package rpc

import (
	"context"
	"net"
)

// DialInProc attaches an in-process connection to the given RPC server.
func DialInProc(handler *Server) *Client {
	initCtx := context.Background()
	c, _ := newClient(initCtx, func(context.Context) (ServerCodec, error) {
		p1, p2 := net.Pipe()
		go handler.ServeCodec(NewJSONCodec(p1), OptionMethodInvocation|OptionSubscriptions)
		return NewJSONCodec(p2), nil
	})
	return c
}
