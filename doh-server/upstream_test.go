package main

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestSplitUpstreamList(t *testing.T) {
	t.Parallel()

	got := splitUpstreamList(" udp:1.1.1.1:53, tcp:8.8.8.8:53 , ")
	if len(got) != 2 || got[0] != "udp:1.1.1.1:53" || got[1] != "tcp:8.8.8.8:53" {
		t.Fatalf("splitUpstreamList = %#v", got)
	}
}

func TestPerformDNSQueryUsesPrimaryWhenHealthy(t *testing.T) {
	t.Parallel()

	primaryHits := &atomic.Int32{}
	backupHits := &atomic.Int32{}
	primary := startUDPServer(t, countingHandler(primaryHits, answerA(net.IPv4(192, 0, 2, 10))))
	backup := startUDPServer(t, countingHandler(backupHits, answerA(net.IPv4(192, 0, 2, 20))))

	s := newTestServer(&config{
		Upstream:       []string{"udp:" + primary},
		BackupUpstream: []string{"udp:" + backup},
		Timeout:        1,
		Tries:          1,
	})

	req := newARequest("example.com.")
	if err := s.performDNSQuery(req); err != nil {
		t.Fatalf("performDNSQuery: %v", err)
	}
	assertA(t, req, "192.0.2.10")
	if primaryHits.Load() == 0 {
		t.Fatal("expected primary to be queried")
	}
	if backupHits.Load() != 0 {
		t.Fatal("backup must not be used while primary is healthy")
	}
	if s.primaryDown.Load() {
		t.Fatal("primary should stay marked healthy")
	}
}

func TestPerformDNSQueryFailsOverToBackupOnTimeout(t *testing.T) {
	t.Parallel()

	backupHits := &atomic.Int32{}
	primary := startUDPServer(t, dropHandler)
	backup := startUDPServer(t, countingHandler(backupHits, answerA(net.IPv4(192, 0, 2, 20))))

	s := newTestServer(&config{
		Upstream:       []string{"udp:" + primary},
		BackupUpstream: []string{"udp:" + backup},
		Timeout:        1,
		Tries:          1,
	})

	req := newARequest("example.com.")
	if err := s.performDNSQuery(req); err != nil {
		t.Fatalf("performDNSQuery: %v", err)
	}
	assertA(t, req, "192.0.2.20")
	if backupHits.Load() == 0 {
		t.Fatal("expected backup to be queried")
	}
	if !s.primaryDown.Load() {
		t.Fatal("primary should be marked down after timeout")
	}
}

func TestPerformDNSQueryDoesNotFailOverOnNXDOMAIN(t *testing.T) {
	t.Parallel()

	backupHits := &atomic.Int32{}
	primary := startUDPServer(t, rcodeHandler(dns.RcodeNameError))
	backup := startUDPServer(t, countingHandler(backupHits, answerA(net.IPv4(192, 0, 2, 20))))

	s := newTestServer(&config{
		Upstream:       []string{"udp:" + primary},
		BackupUpstream: []string{"udp:" + backup},
		Timeout:        1,
		Tries:          1,
	})

	req := newARequest("blocked.example.")
	if err := s.performDNSQuery(req); err != nil {
		t.Fatalf("performDNSQuery: %v", err)
	}
	if req.response.Rcode != dns.RcodeNameError {
		t.Fatalf("Rcode = %d, want NXDOMAIN", req.response.Rcode)
	}
	if backupHits.Load() != 0 {
		t.Fatal("NXDOMAIN must not fail over; that would bypass filtering")
	}
	if s.primaryDown.Load() {
		t.Fatal("primary should stay healthy after NXDOMAIN")
	}
}

func TestPerformDNSQueryDoesNotFailOverOnSERVFAIL(t *testing.T) {
	t.Parallel()

	backupHits := &atomic.Int32{}
	primary := startUDPServer(t, rcodeHandler(dns.RcodeServerFailure))
	backup := startUDPServer(t, countingHandler(backupHits, answerA(net.IPv4(192, 0, 2, 20))))

	s := newTestServer(&config{
		Upstream:       []string{"udp:" + primary},
		BackupUpstream: []string{"udp:" + backup},
		Timeout:        1,
		Tries:          1,
	})

	req := newARequest("example.com.")
	if err := s.performDNSQuery(req); err != nil {
		t.Fatalf("performDNSQuery: %v", err)
	}
	if req.response.Rcode != dns.RcodeServerFailure {
		t.Fatalf("Rcode = %d, want SERVFAIL", req.response.Rcode)
	}
	if backupHits.Load() != 0 {
		t.Fatal("SERVFAIL must not fail over; that would bypass filtering")
	}
}

func TestPerformDNSQueryStaysOnBackupUntilProbeSucceeds(t *testing.T) {
	t.Parallel()

	primaryHits := &atomic.Int32{}
	backupHits := &atomic.Int32{}
	primary := startUDPServer(t, countingHandler(primaryHits, dropHandler))
	backup := startUDPServer(t, countingHandler(backupHits, answerA(net.IPv4(192, 0, 2, 20))))

	s := newTestServer(&config{
		Upstream:       []string{"udp:" + primary},
		BackupUpstream: []string{"udp:" + backup},
		Timeout:        1,
		Tries:          1,
	})
	s.primaryDown.Store(true)

	req := newARequest("example.com.")
	if err := s.performDNSQuery(req); err != nil {
		t.Fatalf("performDNSQuery: %v", err)
	}
	assertA(t, req, "192.0.2.20")
	if primaryHits.Load() != 0 {
		t.Fatal("sticky backup mode must skip the primary")
	}
	if backupHits.Load() == 0 {
		t.Fatal("expected backup to be queried")
	}

	if s.probePrimary() {
		t.Fatal("probe should fail while primary drops queries")
	}
	if !s.primaryDown.Load() {
		t.Fatal("primary should remain down after a failed probe")
	}
}

func TestProbePrimarySucceedsOnAnyDNSResponse(t *testing.T) {
	t.Parallel()

	primary := startUDPServer(t, rcodeHandler(dns.RcodeRefused))
	s := newTestServer(&config{
		Upstream: []string{"udp:" + primary},
		Timeout:  1,
		Tries:    1,
	})

	if !s.probePrimary() {
		t.Fatal("REFUSED still means the server is reachable")
	}
}

func TestPerformDNSQueryOpportunisticFailbackWhenBackupFails(t *testing.T) {
	t.Parallel()

	primary := startUDPServer(t, answerA(net.IPv4(192, 0, 2, 10)))
	backup := startUDPServer(t, dropHandler)

	s := newTestServer(&config{
		Upstream:       []string{"udp:" + primary},
		BackupUpstream: []string{"udp:" + backup},
		Timeout:        1,
		Tries:          1,
	})
	s.primaryDown.Store(true)

	req := newARequest("example.com.")
	if err := s.performDNSQuery(req); err != nil {
		t.Fatalf("performDNSQuery: %v", err)
	}
	assertA(t, req, "192.0.2.10")
	if s.primaryDown.Load() {
		t.Fatal("primary should be marked healthy after opportunistic failback")
	}
}

func TestPerformDNSQueryNoBackupReturnsError(t *testing.T) {
	t.Parallel()

	primary := startUDPServer(t, dropHandler)
	s := newTestServer(&config{
		Upstream: []string{"udp:" + primary},
		Timeout:  1,
		Tries:    1,
	})

	req := newARequest("example.com.")
	if err := s.performDNSQuery(req); err == nil {
		t.Fatal("expected error when primary fails and no backup is configured")
	}
}

func TestStartPrimaryProbeFailsBack(t *testing.T) {
	t.Parallel()

	primary := startUDPServer(t, answerA(net.IPv4(192, 0, 2, 10)))
	backup := startUDPServer(t, answerA(net.IPv4(192, 0, 2, 20)))

	s := newTestServer(&config{
		Upstream:            []string{"udp:" + primary},
		BackupUpstream:      []string{"udp:" + backup},
		BackupRetryInterval: 1,
		Timeout:             1,
		Tries:               1,
	})
	s.primaryDown.Store(true)
	s.startPrimaryProbe()
	t.Cleanup(func() {
		close(s.probeStop)
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !s.primaryDown.Load() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("probe loop should fail back to primary within 3s")
}

func TestNewServerReadsBackupEnv(t *testing.T) {
	t.Setenv("UPSTREAM_DNS_SERVER", "udp:192.0.2.1:53")
	t.Setenv("BACKUP_UPSTREAM_DNS_SERVER", "udp:192.0.2.2:53, tcp:192.0.2.3:53")
	t.Setenv("BACKUP_RETRY_INTERVAL", "15")
	t.Setenv("DOH_HTTP_PREFIX", "/dns-query")
	t.Setenv("DOH_SERVER_LISTEN", "")
	t.Setenv("DOH_SERVER_TIMEOUT", "")
	t.Setenv("DOH_SERVER_TRIES", "")
	t.Setenv("DOH_SERVER_VERBOSE", "")
	t.Setenv("REDIS_URL", "")

	conf := &config{
		Path:     "/dns-query",
		Listen:   []string{"127.0.0.1:0"},
		Upstream: []string{"udp:8.8.8.8:53"},
		Timeout:  10,
		Tries:    3,
	}
	s, err := NewServer(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(conf.BackupUpstream) != 2 || conf.BackupUpstream[0] != "udp:192.0.2.2:53" || conf.BackupUpstream[1] != "tcp:192.0.2.3:53" {
		t.Fatalf("BackupUpstream = %#v", conf.BackupUpstream)
	}
	if conf.BackupRetryInterval != 15 {
		t.Fatalf("BackupRetryInterval = %d", conf.BackupRetryInterval)
	}
	if len(conf.Upstream) != 1 || conf.Upstream[0] != "udp:192.0.2.1:53" {
		t.Fatalf("Upstream = %#v", conf.Upstream)
	}
	_ = s
}

func TestPerformDNSQueryInvalidType(t *testing.T) {
	t.Parallel()

	s := newTestServer(&config{
		Upstream: []string{"not-a-valid-upstream"},
		Timeout:  1,
		Tries:    1,
	})

	req := newARequest("example.com.")
	err := s.performDNSQuery(req)
	var cfgErr *configError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err = %v, want configError", err)
	}
}

func newTestServer(conf *config) *Server {
	timeout := time.Duration(conf.Timeout) * time.Second
	return &Server{
		conf: conf,
		udpClient: &dns.Client{
			Net:     "udp",
			UDPSize: dns.DefaultMsgSize,
			Timeout: timeout,
		},
		tcpClient: &dns.Client{
			Net:     "tcp",
			Timeout: timeout,
		},
		tcpClientTLS: &dns.Client{
			Net:     "tcp-tls",
			Timeout: timeout,
		},
	}
}

func newARequest(name string) *DNSRequest {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	m.RecursionDesired = true
	return &DNSRequest{request: m}
}

func assertA(t *testing.T, req *DNSRequest, want string) {
	t.Helper()
	if req.response == nil || len(req.response.Answer) == 0 {
		t.Fatalf("no answer from %s", req.currentUpstream)
	}
	a, ok := req.response.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("answer type %T", req.response.Answer[0])
	}
	if got := a.A.String(); got != want {
		t.Fatalf("A = %s, want %s", got, want)
	}
}

func startUDPServer(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ready := make(chan struct{})
	srv := &dns.Server{
		PacketConn:        pc,
		Handler:           handler,
		NotifyStartedFunc: func() { close(ready) },
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ActivateAndServe()
	}()

	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("dns server: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("dns server start timeout")
	}

	t.Cleanup(func() {
		_ = srv.Shutdown()
	})
	return pc.LocalAddr().String()
}

func countingHandler(n *atomic.Int32, next dns.HandlerFunc) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		n.Add(1)
		next(w, r)
	}
}

func dropHandler(dns.ResponseWriter, *dns.Msg) {}

func answerA(ip net.IP) dns.HandlerFunc {
	ip = ip.To4()
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		if len(r.Question) == 0 {
			_ = w.WriteMsg(m)
			return
		}
		q := r.Question[0]
		switch q.Qtype {
		case dns.TypeA:
			m.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
				A:   ip,
			}}
		case dns.TypeNS:
			m.Answer = []dns.RR{&dns.NS{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 30},
				Ns:  "ns.example.com.",
			}}
		}
		_ = w.WriteMsg(m)
	}
}

func rcodeHandler(rcode int) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = rcode
		_ = w.WriteMsg(m)
	}
}
