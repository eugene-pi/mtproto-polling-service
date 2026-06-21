package proxy

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// verifierFunc adapts a function to the Verifier interface.
type verifierFunc func(ctx context.Context, p Proxy) error

func (f verifierFunc) Verify(ctx context.Context, p Proxy) error { return f(ctx, p) }

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

// TestVerifierRunsSerially proves the second-stage verification is never run
// concurrently, even when many proxies pass the parallel TCP stage at once.
func TestVerifierRunsSerially(t *testing.T) {
	var proxies []Proxy
	for i := 0; i < 8; i++ {
		proxies = append(proxies, newListener(t))
	}

	var (
		mu        sync.Mutex
		active    int
		maxActive int
		count     int
	)
	verifier := verifierFunc(func(_ context.Context, _ Proxy) error {
		mu.Lock()
		active++
		count++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond) // window in which any overlap would show

		mu.Lock()
		active--
		mu.Unlock()
		return errors.New("never usable") // force every proxy to be verified
	})

	checker := NewChecker(time.Second, 16)
	checker.Verifier = verifier

	if got := checker.FindFirstWorking(context.Background(), proxies); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("verification must run one-by-one; observed %d concurrent", maxActive)
	}
	if count != len(proxies) {
		t.Fatalf("expected all %d proxies verified, got %d", len(proxies), count)
	}
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
