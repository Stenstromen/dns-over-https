package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/miekg/dns"
	"github.com/redis/go-redis/v9"

	jsondns "github.com/stenstromen/dns-over-https/json-dns"
)

type Server struct {
	conf         *config
	udpClient    *dns.Client
	tcpClient    *dns.Client
	tcpClientTLS *dns.Client
	servemux     *http.ServeMux
	redis        *redis.Client
}

type DNSRequest struct {
	request         *dns.Msg
	response        *dns.Msg
	currentUpstream string
	errtext         string
	errcode         int
	transactionID   uint16
	isTailored      bool
	fromCache       bool
}

func NewServer(conf *config) (*Server, error) {
	// Override config with environment variables if present
	if upstreamDNS := os.Getenv("UPSTREAM_DNS_SERVER"); upstreamDNS != "" {
		conf.Upstream = []string{upstreamDNS}
	}

	if prefix := os.Getenv("DOH_HTTP_PREFIX"); prefix != "" {
		conf.Path = prefix
	}

	if listen := os.Getenv("DOH_SERVER_LISTEN"); listen != "" {
		conf.Listen = []string{listen}
	}

	if timeout := os.Getenv("DOH_SERVER_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			conf.Timeout = uint(t)
		}
	}

	if tries := os.Getenv("DOH_SERVER_TRIES"); tries != "" {
		if t, err := strconv.Atoi(tries); err == nil {
			conf.Tries = uint(t)
		}
	}

	if verbose := os.Getenv("DOH_SERVER_VERBOSE"); verbose != "" {
		conf.Verbose = verbose == "true"
	}

	server := &Server{
		conf: conf,
	}

	// Initialize Redis if available
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		server.redis = redis.NewClient(&redis.Options{
			Addr: redisURL,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := server.redis.Ping(ctx).Result()
		if err != nil {
			log.Printf("Failed to connect to Redis at %s: %v", redisURL, err)
			server.redis = nil
		} else {
			log.Printf("Successfully connected to Redis at %s", redisURL)
		}
	}

	server.udpClient = &dns.Client{
		Net:     "udp",
		UDPSize: dns.DefaultMsgSize,
		Timeout: time.Duration(conf.Timeout) * time.Second,
	}
	server.tcpClient = &dns.Client{
		Net:     "tcp",
		Timeout: time.Duration(conf.Timeout) * time.Second,
	}
	server.tcpClientTLS = &dns.Client{
		Net:     "tcp-tls",
		Timeout: time.Duration(conf.Timeout) * time.Second,
	}
	server.servemux = http.NewServeMux()
	server.servemux.HandleFunc(conf.Path, server.handlerFunc)
	return server, nil
}

func (s *Server) Start() error {
	servemux := http.Handler(s.servemux)
	if s.conf.Verbose {
		servemux = handlers.CombinedLoggingHandler(os.Stdout, servemux)
	}

	var clientCAPool *x509.CertPool
	if s.conf.TLSClientAuth {
		if s.conf.TLSClientAuthCA != "" {
			clientCA, err := os.ReadFile(s.conf.TLSClientAuthCA)
			if err != nil {
				log.Fatalf("Reading certificate for client authentication has failed: %v", err)
			}
			clientCAPool = x509.NewCertPool()
			clientCAPool.AppendCertsFromPEM(clientCA)
			log.Println("Certificate loaded for client TLS authentication")
		} else {
			log.Fatalln("TLS client authentication requires both tls_client_auth and tls_client_auth_ca, exiting.")
		}
	}

	results := make(chan error, len(s.conf.Listen))
	for _, addr := range s.conf.Listen {
		go func(addr string) {
			var err error
			if s.conf.Cert != "" || s.conf.Key != "" {
				if clientCAPool != nil {
					srvtls := &http.Server{
						Handler: servemux,
						Addr:    addr,
						TLSConfig: &tls.Config{
							ClientCAs:  clientCAPool,
							ClientAuth: tls.RequireAndVerifyClientCert,
							GetCertificate: func(info *tls.ClientHelloInfo) (certificate *tls.Certificate, e error) {
								c, err := tls.LoadX509KeyPair(s.conf.Cert, s.conf.Key)
								if err != nil {
									fmt.Printf("Error loading server certificate key pair: %v\n", err)
									return nil, err
								}
								return &c, nil
							},
						},
					}
					err = srvtls.ListenAndServeTLS("", "")
				} else {
					err = http.ListenAndServeTLS(addr, s.conf.Cert, s.conf.Key, servemux)
				}
			} else {
				err = http.ListenAndServe(addr, servemux)
			}
			if err != nil {
				log.Println(err)
			}
			results <- err
		}(addr)
	}
	// wait for all handlers
	for i := 0; i < cap(results); i++ {
		err := <-results
		if err != nil {
			return err
		}
	}
	close(results)
	return nil
}

func (s *Server) handlerFunc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		if strings.ContainsRune(realIP, ':') {
			r.RemoteAddr = "[" + realIP + "]:0"
		} else {
			r.RemoteAddr = realIP + ":0"
		}
		_, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			r.RemoteAddr = realIP
		}
	}

	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Max-Age", "3600")
	w.Header().Set("Server", USER_AGENT)
	w.Header().Set("X-Powered-By", USER_AGENT)

	if r.Method == "OPTIONS" {
		w.Header().Set("Content-Length", "0")
		return
	}

	if r.Form == nil {
		const maxMemory = 32 << 20 // 32 MB
		r.ParseMultipartForm(maxMemory)
	}

	for _, header := range s.conf.DebugHTTPHeaders {
		if value := r.Header.Get(header); value != "" {
			log.Printf("%s: %s\n", header, value)
		}
	}

	contentType := r.Header.Get("Content-Type")
	if ct := r.FormValue("ct"); ct != "" {
		contentType = ct
	}
	if contentType == "" {
		// Guess request Content-Type based on other parameters
		if r.FormValue("name") != "" {
			contentType = "application/dns-json"
		} else if r.FormValue("dns") != "" {
			contentType = "application/dns-message"
		}
	}
	var responseType string
	for _, responseCandidate := range strings.Split(r.Header.Get("Accept"), ",") {
		responseCandidate = strings.SplitN(responseCandidate, ";", 2)[0]
		if responseCandidate == "application/json" {
			responseType = "application/json"
			break
		} else if responseCandidate == "application/dns-udpwireformat" {
			responseType = "application/dns-message"
			break
		} else if responseCandidate == "application/dns-message" {
			responseType = "application/dns-message"
			break
		}
	}
	if responseType == "" {
		// Guess response Content-Type based on request Content-Type
		if contentType == "application/dns-json" {
			responseType = "application/json"
		} else if contentType == "application/dns-message" {
			responseType = "application/dns-message"
		} else if contentType == "application/dns-udpwireformat" {
			responseType = "application/dns-message"
		}
	}

	var req *DNSRequest
	if contentType == "application/dns-json" {
		req = s.parseRequestGoogle(ctx, w, r)
	} else if contentType == "application/dns-message" {
		req = s.parseRequestIETF(ctx, w, r)
	} else if contentType == "application/dns-udpwireformat" {
		req = s.parseRequestIETF(ctx, w, r)
	} else {
		jsondns.FormatError(w, fmt.Sprintf("Invalid argument value: \"ct\" = %q", contentType), 415)
		return
	}
	if req.errcode == 444 {
		return
	}
	if req.errcode != 0 {
		jsondns.FormatError(w, req.errtext, req.errcode)
		return
	}

	req = s.patchRootRD(req)

	err := s.doDNSQuery(ctx, req)
	if err != nil {
		jsondns.FormatError(w, fmt.Sprintf("DNS query failure (%s)", err.Error()), 503)
		return
	}

	if responseType == "application/json" {
		s.generateResponseGoogle(ctx, w, r, req)
	} else if responseType == "application/dns-message" {
		s.generateResponseIETF(ctx, w, r, req)
	} else {
		panic("Unknown response Content-Type")
	}
}

func (s *Server) findClientIP(r *http.Request) net.IP {
	noEcs := r.URL.Query().Get("no_ecs")
	if strings.EqualFold(noEcs, "true") {
		return nil
	}

	XForwardedFor := r.Header.Get("X-Forwarded-For")
	if XForwardedFor != "" {
		for _, addr := range strings.Split(XForwardedFor, ",") {
			addr = strings.TrimSpace(addr)
			ip := net.ParseIP(addr)
			if jsondns.IsGlobalIP(ip) {
				return ip
			}
		}
	}
	XRealIP := r.Header.Get("X-Real-IP")
	if XRealIP != "" {
		addr := strings.TrimSpace(XRealIP)
		ip := net.ParseIP(addr)
		if s.conf.ECSAllowNonGlobalIP || jsondns.IsGlobalIP(ip) {
			return ip
		}
	}

	remoteAddr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err != nil {
		return nil
	}
	ip := remoteAddr.IP
	if s.conf.ECSAllowNonGlobalIP || jsondns.IsGlobalIP(ip) {
		return ip
	}
	return nil
}

// Workaround a bug causing Unbound to refuse returning anything about the root.
func (s *Server) patchRootRD(req *DNSRequest) *DNSRequest {
	for _, question := range req.request.Question {
		if question.Name == "." {
			req.request.RecursionDesired = true
		}
	}
	return req
}

// Return the position index for the question of qtype from a DNS msg, otherwise return -1.
func (s *Server) indexQuestionType(msg *dns.Msg, qtype uint16) int {
	for i, question := range msg.Question {
		if question.Qtype == qtype {
			return i
		}
	}
	return -1
}

func createCacheKey(req *DNSRequest) string {
	if len(req.request.Question) == 0 {
		return ""
	}
	q := req.request.Question[0]
	return fmt.Sprintf("dns:%s:%d", strings.ToLower(q.Name), q.Qtype)
}

func (s *Server) doDNSQuery(ctx context.Context, req *DNSRequest) (err error) {
	cacheKey := createCacheKey(req)
	if cacheKey == "" {
		return fmt.Errorf("invalid DNS request: no question")
	}

	const cacheTTL = 300 // 5 minutes fixed TTL

	// Try to get from cache first if Redis is available
	if s.redis != nil {
		// Try to get fresh cache entry
		if cachedResponse, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
			msg := new(dns.Msg)
			if err := msg.Unpack(cachedResponse); err == nil {
				req.response = msg
				req.fromCache = true
				return nil
			}
		}

		// Check for stale entry
		if cachedResponse, err := s.redis.Get(ctx, cacheKey+":stale").Bytes(); err == nil {
			msg := new(dns.Msg)
			if err := msg.Unpack(cachedResponse); err == nil {
				req.response = msg
				req.fromCache = true
				// Trigger background refresh
				go s.refreshCache(cacheKey, req.request)
				return nil
			}
		}
	}

	// Cache miss - perform DNS query
	if err := s.performDNSQuery(req); err != nil {
		return err
	}

	// Cache successful response if Redis is available
	if s.redis != nil && req.response != nil && len(req.response.Answer) > 0 {
		// Store the exact response
		if responseBinary, err := req.response.Pack(); err == nil {
			// Store in both current and stale cache
			s.redis.Set(ctx, cacheKey, responseBinary, time.Duration(cacheTTL)*time.Second)
			s.redis.Set(ctx, cacheKey+":stale", responseBinary, time.Duration(cacheTTL*2)*time.Second)
		}
	}

	return nil
}

func (s *Server) refreshCache(cacheKey string, request *dns.Msg) {
	ctx := context.Background()
	req := &DNSRequest{
		request: request,
	}

	if err := s.performDNSQuery(req); err != nil {
		return
	}

	if req.response != nil {
		if responseBinary, err := req.response.Pack(); err == nil {
			const cacheTTL = 300
			s.redis.Set(ctx, cacheKey, responseBinary, time.Duration(cacheTTL)*time.Second)
			s.redis.Set(ctx, cacheKey+":stale", responseBinary, time.Duration(cacheTTL*2)*time.Second)
		}
	}
}

func (s *Server) performDNSQuery(req *DNSRequest) error {
	numServers := len(s.conf.Upstream)
	for i := uint(0); i < s.conf.Tries; i++ {
		req.currentUpstream = s.conf.Upstream[rand.Intn(numServers)]
		upstream, t := addressAndType(req.currentUpstream)

		var err error
		switch t {
		case "tcp-tls":
			req.response, _, err = s.tcpClientTLS.ExchangeContext(context.Background(), req.request, upstream)
		case "tcp", "udp":
			if t == "tcp" || (s.indexQuestionType(req.request, dns.TypeAXFR) > -1) {
				req.response, _, err = s.tcpClient.ExchangeContext(context.Background(), req.request, upstream)
			} else {
				req.response, _, err = s.udpClient.ExchangeContext(context.Background(), req.request, upstream)
				if err == nil && req.response != nil && req.response.Truncated {
					req.response, _, err = s.tcpClient.ExchangeContext(context.Background(), req.request, upstream)
				}
			}
		default:
			return &configError{"invalid DNS type"}
		}

		if err == nil && req.response != nil {
			return nil
		}
		log.Printf("DNS error from upstream %s: %s\n", req.currentUpstream, err.Error())
	}
	return fmt.Errorf("all upstream servers failed")
}
