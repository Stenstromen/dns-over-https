package main

import (
	"testing"

	"github.com/miekg/dns"
)

func TestParseCIDR(t *testing.T) {
	t.Parallel()
	for _, ednsClientSubnet := range []string{
		"2001:db8::/0",
		"2001:db8::/56",
		"2001:db8::/129",
		"2001:db8::",

		"127.0.0.1/0",
		"127.0.0.1/24",
		"127.0.0.1/33",
		"127.0.0.1",

		"::ffff:7f00:1/0",
		"::ffff:7f00:1/120",
		"::ffff:7f00:1",
		"127.0.0.1/0",
		"127.0.0.1/24",
		"127.0.0.1",
	} {
		_, ip, ipNet, err := parseSubnet(ednsClientSubnet)
		if err != nil {
			t.Errorf("ecs:%s ip:[%v]  ipNet:[%v]  err:[%v]", ednsClientSubnet, ip, ipNet, err)
		}
	}
}

func TestParseInvalidCIDR(t *testing.T) {
	t.Parallel()

	for _, ip := range []string{
		"test",
		"test/0",
		"test/24",
		"test/34",
		"test/56",
		"test/129",
	} {
		_, _, _, err := parseSubnet(ip)
		if err == nil {
			t.Errorf("expected error for %q", ip)
		}
	}
}

func TestEdns0SubnetParseCIDR(t *testing.T) {
	t.Parallel()
	// init dns Msg
	msg := new(dns.Msg)
	msg.Id = dns.Id()
	msg.SetQuestion(dns.Fqdn("example.com"), 1)

	// init edns0Subnet
	edns0Subnet := new(dns.EDNS0_SUBNET)
	edns0Subnet.Code = dns.EDNS0SUBNET
	edns0Subnet.SourceScope = 0

	// init opt
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(dns.DefaultMsgSize)

	opt.Option = append(opt.Option, edns0Subnet)
	msg.Extra = append(msg.Extra, opt)

	for _, subnet := range []string{"::ffff:7f00:1/120", "127.0.0.1/24"} {
		var err error
		edns0Subnet.Family, edns0Subnet.Address, edns0Subnet.SourceNetmask, err = parseSubnet(subnet)
		if err != nil {
			t.Error(err)
			continue
		}
		t.Log(msg.Pack())
	}

	// ------127.0.0.1/24-----
	// [143 29 1 0 0 1 0 0 0 0 0 1 7 101 120 97 109 112 108 101 3 99 111 109 0 0 1 0 1 0
	// opt start   0 41 16 0 0 0 0 0 0 11
	// subnet start 0 8 0 7 0 1 24 0
	// client subnet start 127 0 0]

	// -----::ffff:7f00:1/120----
	// [111 113 1 0 0 1 0 0 0 0 0 1 7 101 120 97 109 112 108 101 3 99 111 109 0 0 1 0 1 0
	// opt start  0 41 16 0 0 0 0 0 0 23
	// subnet start  0 8 0 19 0 2 120 0
	// client subnet start 0 0 0 0 0 0 0 0 0 0 255 255 127 0 0]
}
