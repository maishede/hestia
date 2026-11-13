package server

import (
	"context"
	"errors"
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

func listenTCP(address string) (net.Listener, error) {
	config := net.ListenConfig{
		Control: func(_, _ string, conn syscall.RawConn) error {
			var optionErr error
			err := conn.Control(func(fd uintptr) {
				// Winsock defines SO_EXCLUSIVEADDRUSE as the complement of SO_REUSEADDR.
				optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, ^syscall.SO_REUSEADDR, 1)
			})
			if err != nil {
				return err
			}
			return optionErr
		},
	}
	return config.Listen(context.Background(), "tcp", address)
}

func portUnavailable(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE) || errors.Is(err, windows.WSAEACCES)
}
