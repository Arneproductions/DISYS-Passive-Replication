package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/peer"
)

func GetClientIpAddress(c context.Context) string {
	if p, ok := peer.FromContext(c); ok {
		return strings.Split(p.Addr.String(), ":")[0]
	} else {
		return ""
	}
}