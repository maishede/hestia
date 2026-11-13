//go:build !windows

package server

import (
	"errors"
	"net"
	"syscall"
)

func listenTCP(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}

func portUnavailable(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
