package grpc

import (
	"context"

	"google.golang.org/grpc/peer"
)

func GetClientIpAddress(c context.Context) string {
	if p, ok := peer.FromContext(c); ok {
		return p.Addr.String()
	} else {
		return ""
	}
}