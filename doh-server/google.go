package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/idna"

	jsondns "github.com/stenstromen/dns-over-https/json-dns"
)

func (s *Server) parseRequestGoogle(_ context.Context, _ http.ResponseWriter, r *http.Request) *DNSRequest {
	name := r.FormValue("name")
	if name == "" {
		return &DNSRequest{
			errcode: 400,
			errtext: "Invalid argument value: \"name\"",
		}
	}
	if punycode, err := idna.ToASCII(name); err == nil {
		name = punycode
	} else {
		return &DNSRequest{
			errcode: 400,
			errtext: fmt.Sprintf("Invalid argument value: \"name\" = %q (%s)", name, err.Error()),
		}
	}

	rrTypeStr := r.FormValue("type")
	rrType := uint16(1)
	if rrTypeStr == "" {
	} else if v, err := strconv.ParseUint(rrTypeStr, 10, 16); err == nil {
		rrType = uint16(v)
	} else if v, ok := dns.StringToType[strings.ToUpper(rrTypeStr)]; ok {
		rrType = v
	} else {
		return &DNSRequest{
			errcode: 400,
			errtext: fmt.Sprintf("Invalid argument value: \"type\" = %q", rrTypeStr),
		}
	}

	cdStr := r.FormValue("cd")
	cd := false
	if cdStr == "1" || strings.EqualFold(cdStr, "true") {
		cd = true
	} else if cdStr == "0" || strings.EqualFold(cdStr, "false") || cdStr == "" {
	} else {
		return &DNSRequest{
			errcode: 400,
			errtext: fmt.Sprintf("Invalid argument value: \"cd\" = %q", cdStr),
		}
	}

	ednsClientSubnet := r.FormValue("edns_client_subnet")
	ednsClientFamily := uint16(0)
	ednsClientAddress := net.IP(nil)
	ednsClientNetmask := uint8(255)
	if ednsClientSubnet != "" {
		if ednsClientSubnet == "0/0" {
			ednsClientSubnet = "0.0.0.0/0"
		}

		var err error
		ednsClientFamily, ednsClientAddress, ednsClientNetmask, err = parseSubnet(ednsClientSubnet)
		if err != nil {
			return &DNSRequest{
				errcode: 400,
				errtext: err.Error(),
			}
		}
	} else {
		ednsClientAddress = s.findClientIP(r)
		if ednsClientAddress == nil {
			ednsClientNetmask = 0
		} else if ipv4 := ednsClientAddress.To4(); ipv4 != nil {
			ednsClientFamily = 1
			ednsClientAddress = ipv4
			ednsClientNetmask = 24
		} else {
			ednsClientFamily = 2
			ednsClientNetmask = 56
		}
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), rrType)
	msg.CheckingDisabled = cd
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(dns.DefaultMsgSize)
	opt.SetDo(true)
	if ednsClientAddress != nil {
		edns0Subnet := new(dns.EDNS0_SUBNET)
		edns0Subnet.Code = dns.EDNS0SUBNET
		edns0Subnet.Family = ednsClientFamily
		edns0Subnet.SourceNetmask = ednsClientNetmask
		edns0Subnet.SourceScope = 0
		edns0Subnet.Address = ednsClientAddress
		opt.Option = append(opt.Option, edns0Subnet)
	}
	msg.Extra = append(msg.Extra, opt)

	return &DNSRequest{
		request:    msg,
		isTailored: ednsClientSubnet == "",
	}
}

func parseSubnet(ednsClientSubnet string) (ednsClientFamily uint16, ednsClientAddress net.IP, ednsClientNetmask uint8, err error) {
	slash := strings.IndexByte(ednsClientSubnet, '/')
	if slash < 0 {
		ednsClientAddress = net.ParseIP(ednsClientSubnet)
		if ednsClientAddress == nil {
			err = fmt.Errorf("invalid argument value: \"edns_client_subnet\" = %q", ednsClientSubnet)
			return
		}
		if ipv4 := ednsClientAddress.To4(); ipv4 != nil {
			ednsClientFamily = 1
			ednsClientAddress = ipv4
			ednsClientNetmask = 24
		} else {
			ednsClientFamily = 2
			ednsClientNetmask = 56
		}
	} else {
		ednsClientAddress = net.ParseIP(ednsClientSubnet[:slash])
		if ednsClientAddress == nil {
			err = fmt.Errorf("invalid argument value: \"edns_client_subnet\" = %q", ednsClientSubnet)
			return
		}
		if ipv4 := ednsClientAddress.To4(); ipv4 != nil {
			ednsClientFamily = 1
			ednsClientAddress = ipv4
		} else {
			ednsClientFamily = 2
		}
		netmask, err1 := strconv.ParseUint(ednsClientSubnet[slash+1:], 10, 8)
		if err1 != nil {
			err = fmt.Errorf("invalid argument value: \"edns_client_subnet\" = %q", ednsClientSubnet)
			return
		}
		ednsClientNetmask = uint8(netmask)
	}

	return
}

func (s *Server) generateResponseGoogle(_ context.Context, w http.ResponseWriter, _ *http.Request, req *DNSRequest) {
	respJSON := jsondns.Marshal(req.response)
	respStr, err := json.Marshal(respJSON)
	if err != nil {
		log.Println(err)
		jsondns.FormatError(w, fmt.Sprintf("DNS packet parse failure (%s)", err.Error()), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	now := time.Now().UTC().Format(http.TimeFormat)
	w.Header().Set("Date", now)
	w.Header().Set("Last-Modified", now)
	w.Header().Set("Vary", "Accept")
	if respJSON.HaveTTL {
		if req.isTailored {
			w.Header().Set("Cache-Control", "private, max-age="+strconv.FormatUint(uint64(respJSON.LeastTTL), 10))
		} else {
			w.Header().Set("Cache-Control", "public, max-age="+strconv.FormatUint(uint64(respJSON.LeastTTL), 10))
		}
		w.Header().Set("Expires", respJSON.EarliestExpires.Format(http.TimeFormat))
	}
	if respJSON.Status == dns.RcodeServerFailure {
		w.WriteHeader(503)
	}

	// Add cache status header
	if req.fromCache {
		w.Header().Set("X-Cache-Status", "HIT")
	} else {
		w.Header().Set("X-Cache-Status", "MISS")
	}

	w.Write(respStr)
}
