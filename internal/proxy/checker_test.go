package proxy

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestFindFirstWorking(t *testing.T) {
	reachable := newListener(t)

	checker := NewChecker(time.Second, 10)
	proxies := []Proxy{
		// Unreachable: TEST-NET-1 address that should not connect quickly.
		{Server: "192.0.2.1", Port: 9},
		reachable,
	}

	got := checker.FindFirstWorking(context.Background(), proxies)
	if got == nil {
		t.Fatal("expected a working proxy, got nil")
	}
	if got.Address() != reachable.Address() {
		t.Fatalf("unexpected winner: %+v", got)
	}
}

// newListener starts a TCP listener that accepts and immediately closes
// connections, and returns a Proxy pointing at it. The listener is closed when
// the test finishes.
func newListener(t *testing.T) Proxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	return Proxy{Server: host, Port: atoiOrZero(portStr)}
}

func TestFindFirstWorkingNone(t *testing.T) {
	checker := NewChecker(200*time.Millisecond, 10)
	proxies := []Proxy{{Server: "192.0.2.1", Port: 9}}
	if got := checker.FindFirstWorking(context.Background(), proxies); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func atoiOrZero(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
