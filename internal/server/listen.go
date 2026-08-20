package server

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// ListenFirstAvailable listens on the first candidate port that is available.
func ListenFirstAvailable(host string, ports []int) (net.Listener, int, error) {
	if len(ports) == 0 {
		return nil, 0, errors.New("no candidate ports supplied")
	}

	errs := make([]error, 0, len(ports))
	for _, port := range ports {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			errs = append(errs, fmt.Errorf("listen on %s port %d: %w", host, port, err))
			continue
		}

		actualPort := listener.Addr().(*net.TCPAddr).Port
		return listener, actualPort, nil
	}

	return nil, 0, fmt.Errorf("no candidate port available: %w", errors.Join(errs...))
}
