package config

import (
	"fmt"
	"net"
	"strings"
)

// ParseTrustedProxies converts configured CIDRs and bare IPs into networks.
func ParseTrustedProxies(values []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(value); err == nil {
			nets = append(nets, network)
			continue
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy %q", value)
		}
		bits := 128
		if v4 := ip.To4(); v4 != nil {
			ip = v4
			bits = 32
		}
		nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return nets, nil
}
