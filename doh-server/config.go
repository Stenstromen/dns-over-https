package main

import (
	"regexp"
	"strings"
)

type config struct {
	TLSClientAuthCA     string   `toml:"tls_client_auth_ca"`
	LocalAddr           string   `toml:"local_addr"`
	Cert                string   `toml:"cert"`
	Key                 string   `toml:"key"`
	Path                string   `toml:"path"`
	DebugHTTPHeaders    []string `toml:"debug_http_headers"`
	Listen              []string `toml:"listen"`
	Upstream            []string `toml:"upstream"`
	BackupUpstream      []string `toml:"backup_upstream"`
	Timeout             uint     `toml:"timeout"`
	Tries               uint     `toml:"tries"`
	BackupRetryInterval uint     `toml:"backup_retry_interval"`
	Verbose             bool     `toml:"verbose"`
	LogGuessedIP        bool     `toml:"log_guessed_client_ip"`
	ECSAllowNonGlobalIP bool     `toml:"ecs_allow_non_global_ip"`
	ECSUsePreciseIP     bool     `toml:"ecs_use_precise_ip"`
	TLSClientAuth       bool     `toml:"tls_client_auth"`
}

var rxUpstreamWithTypePrefix = regexp.MustCompile("^[a-z-]+(:)")

func addressAndType(us string) (string, string) {
	p := rxUpstreamWithTypePrefix.FindStringSubmatchIndex(us)
	if len(p) != 4 {
		return "", ""
	}

	return us[p[2]+1:], us[:p[2]]
}

func splitUpstreamList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type configError struct {
	err string
}

func (e *configError) Error() string {
	return e.err
}
