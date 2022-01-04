package net

import (
	"net"
	"strings"
)

/*
Finds the ip with the specified prefix
*/
func GetOutboundIp(prefixIp string) string {
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && strings.HasPrefix(ipnet.IP.String(), prefixIp) {
				return ipnet.IP.String()
			}
		}
	}

	return ""
}
