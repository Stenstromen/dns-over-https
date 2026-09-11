package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/miekg/dns"
)

func (s *Server) performDNSQuery(req *DNSRequest) error {
	if len(s.conf.BackupUpstream) > 0 && s.primaryDown.Load() {
		err := s.queryServers(req, s.conf.BackupUpstream)
		if err == nil {
			return nil
		}
		// Backup failed too; try the preferred pool in case it recovered early.
		if primaryErr := s.queryServers(req, s.conf.Upstream); primaryErr == nil {
			log.Printf("Primary upstream recovered, switching back from backup")
			s.primaryDown.Store(false)
			return nil
		}
		return err
	}

	err := s.queryServers(req, s.conf.Upstream)
	if err == nil {
		return nil
	}
	if len(s.conf.BackupUpstream) == 0 {
		return err
	}

	log.Printf("Primary upstream failed (%v), failing over to backup", err)
	s.primaryDown.Store(true)
	return s.queryServers(req, s.conf.BackupUpstream)
}

func (s *Server) queryServers(req *DNSRequest, servers []string) error {
	if len(servers) == 0 {
		return fmt.Errorf("no upstream servers configured")
	}

	tries := s.conf.Tries
	if tries == 0 {
		tries = 1
	}

	var lastErr error
	for i := uint(0); i < tries; i++ {
		req.currentUpstream = servers[rand.Intn(len(servers))]
		resp, err := s.exchange(req.request, req.currentUpstream)
		if err == nil && resp != nil {
			req.response = resp
			return nil
		}
		if _, ok := err.(*configError); ok {
			return err
		}
		if err != nil {
			lastErr = err
			log.Printf("DNS error from upstream %s: %s\n", req.currentUpstream, err.Error())
		} else {
			lastErr = fmt.Errorf("empty response")
			log.Printf("DNS error from upstream %s: empty response\n", req.currentUpstream)
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("all upstream servers failed")
}

func (s *Server) exchange(msg *dns.Msg, upstreamWithType string) (*dns.Msg, error) {
	upstream, t := addressAndType(upstreamWithType)
	if t == "" {
		return nil, &configError{"invalid DNS type"}
	}

	switch t {
	case "tcp-tls":
		resp, _, err := s.tcpClientTLS.ExchangeContext(context.Background(), msg, upstream)
		return resp, err
	case "tcp", "udp":
		if t == "tcp" || s.indexQuestionType(msg, dns.TypeAXFR) > -1 {
			resp, _, err := s.tcpClient.ExchangeContext(context.Background(), msg, upstream)
			return resp, err
		}
		resp, _, err := s.udpClient.ExchangeContext(context.Background(), msg, upstream)
		if err == nil && resp != nil && resp.Truncated {
			resp, _, err = s.tcpClient.ExchangeContext(context.Background(), msg, upstream)
		}
		return resp, err
	default:
		return nil, &configError{"invalid DNS type"}
	}
}

func (s *Server) startPrimaryProbe() {
	if len(s.conf.BackupUpstream) == 0 {
		return
	}

	interval := time.Duration(s.conf.BackupRetryInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	s.probeStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !s.primaryDown.Load() {
					continue
				}
				if s.probePrimary() {
					log.Printf("Primary upstream recovered, switching back from backup")
					s.primaryDown.Store(false)
				} else if s.conf.Verbose {
					log.Printf("Primary upstream still unavailable, staying on backup")
				}
			case <-s.probeStop:
				return
			}
		}
	}()
}

// probePrimary reports whether the preferred upstream is reachable.
// Any DNS response (including SERVFAIL/NXDOMAIN/REFUSED) counts as healthy;
// only transport failures keep the primary marked down, so content filtering
// is not treated as an outage.
func (s *Server) probePrimary() bool {
	if len(s.conf.Upstream) == 0 {
		return false
	}

	m := new(dns.Msg)
	m.SetQuestion(".", dns.TypeNS)
	m.RecursionDesired = true

	upstream := s.conf.Upstream[rand.Intn(len(s.conf.Upstream))]
	resp, err := s.exchange(m, upstream)
	if err != nil || resp == nil {
		if s.conf.Verbose {
			log.Printf("Primary upstream probe failed (%s): %v", upstream, err)
		}
		return false
	}
	return true
}
