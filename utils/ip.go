package utils

import (
	"net"
	"strings"
)

// IpUtil IP 工具集。
var IpUtil = ipUtil{}

type ipUtil struct{}

func (r ipUtil) IsIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func (r ipUtil) IsIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil && strings.Contains(s, ":")
}

func (r ipUtil) IsPrivate(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

func (r ipUtil) IsLoopback(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.IsLoopback()
}

func (r ipUtil) IsPublic(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified()
}

func (r ipUtil) InCIDR(ipStr, cidr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return network.Contains(ip)
}

func (r ipUtil) Parse(s string) net.IP {
	return net.ParseIP(s)
}
