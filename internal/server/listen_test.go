package server

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestListenFirstAvailableFallsBackWhenFirstPortIsOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	firstPort := occupied.Addr().(*net.TCPAddr).Port
	listener, port, err := ListenFirstAvailable("127.0.0.1", []int{firstPort, 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if port == firstPort {
		t.Fatalf("port = %d, want fallback instead of occupied port %d", port, firstPort)
	}
}

func TestListenFirstAvailableReportsEveryUnavailablePort(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	firstPort := first.Addr().(*net.TCPAddr).Port
	secondPort := second.Addr().(*net.TCPAddr).Port
	_, _, err = ListenFirstAvailable("127.0.0.1", []int{firstPort, secondPort})
	if err == nil {
		t.Fatal("ListenFirstAvailable() error = nil, want diagnostic error")
	}
	if !strings.Contains(err.Error(), "port "+strconv.Itoa(firstPort)) || !strings.Contains(err.Error(), "port "+strconv.Itoa(secondPort)) {
		t.Fatalf("error = %q, want both unavailable ports", err)
	}
}
