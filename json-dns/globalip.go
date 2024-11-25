package jsondns

import (
	"net"

	"github.com/infobloxopen/go-trees/iptree"
)

var defaultFilter *iptree.Tree

func init() {
	defaultFilter = iptree.NewTree()

	// RFC6890
	// This host on this network
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{0, 0, 0, 0},
		Mask: net.IPMask{255, 0, 0, 0},
	}, struct{}{})

	// Private-Use Networks
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{10, 0, 0, 0},
		Mask: net.IPMask{255, 0, 0, 0},
	}, struct{}{})

	// Shared Address Space
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{100, 64, 0, 0},
		Mask: net.IPMask{255, 192, 0, 0},
	}, struct{}{})

	// Loopback
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{127, 0, 0, 0},
		Mask: net.IPMask{255, 0, 0, 0},
	}, struct{}{})

	// Link Local
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{169, 254, 0, 0},
		Mask: net.IPMask{255, 255, 0, 0},
	}, struct{}{})

	// Private-Use Networks
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{172, 16, 0, 0},
		Mask: net.IPMask{255, 240, 0, 0},
	}, struct{}{})

	// DS-Lite
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{192, 0, 0, 0},
		Mask: net.IPMask{255, 255, 255, 248},
	}, struct{}{})

	// 6to4 Relay Anycast
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{192, 88, 99, 0},
		Mask: net.IPMask{255, 255, 255, 0},
	}, struct{}{})

	// Private-Use Networks
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{192, 168, 0, 0},
		Mask: net.IPMask{255, 255, 0, 0},
	}, struct{}{})

	// Reserved for Future Use & Limited Broadcast
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{240, 0, 0, 0},
		Mask: net.IPMask{240, 0, 0, 0},
	}, struct{}{})

	// RFC6890
	// Unspecified & Loopback Address
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Mask: net.IPMask{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe},
	}, struct{}{})

	// Discard-Only Prefix
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Mask: net.IPMask{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}, struct{}{})

	// Unique-Local
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{0xfc, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Mask: net.IPMask{0xfe, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}, struct{}{})

	// Linked-Scoped Unicast
	defaultFilter.InplaceInsertNet(&net.IPNet{
		IP:   net.IP{0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Mask: net.IPMask{0xff, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}, struct{}{})
}

func IsGlobalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	_, contained := defaultFilter.GetByIP(ip)
	return !contained
}
