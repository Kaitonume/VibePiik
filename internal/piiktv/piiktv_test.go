package piiktv

import (
	"net"
	"testing"
)

func TestSubnetHosts(t *testing.T) {
	own := net.ParseIP("192.168.5.20")
	for _, test := range []struct {
		name  string
		mask  net.IPMask
		count int
		first string
		last  string
	}{
		// The usual home network: the other 253 addresses of the /24.
		{"/24", net.CIDRMask(24, 32), 253, "192.168.5.1", "192.168.5.254"},
		// A /22 (some mesh routers): own /24 first, then the rest of 192.168.4.0-192.168.7.255.
		{"/22", net.CIDRMask(22, 32), 1021, "192.168.5.0", "192.168.7.254"},
		// Larger networks are limited to the /24 around this computer.
		{"/16", net.CIDRMask(16, 32), 253, "192.168.5.1", "192.168.5.254"},
		// Small subnets stay within themselves.
		{"/29", net.CIDRMask(29, 32), 5, "192.168.5.17", "192.168.5.22"},
		{"/31", net.CIDRMask(31, 32), 0, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			hosts := subnetHosts(own, test.mask)
			if len(hosts) != test.count {
				t.Fatalf("%d hosts, want %d", len(hosts), test.count)
			}
			seen := map[string]bool{}
			for _, host := range hosts {
				if host == own.String() || seen[host] {
					t.Fatalf("own or repeated address %s", host)
				}
				seen[host] = true
			}
			if test.count > 0 && (hosts[0] != test.first || hosts[len(hosts)-1] != test.last) {
				t.Fatalf("first %s last %s", hosts[0], hosts[len(hosts)-1])
			}
		})
	}
	// The /22 lists own /24 before any other block.
	hosts := subnetHosts(own, net.CIDRMask(22, 32))
	for _, host := range hosts[:255] {
		if net.ParseIP(host).To4()[2] != 5 {
			t.Fatalf("%s listed before the own /24 ended", host)
		}
	}
}
