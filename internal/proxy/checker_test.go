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

	checker := NewChecker(time.Second, time.Second, 10)
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

	checker := NewChecker(time.Second, time.Second, 16)
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
	checker := NewChecker(200*time.Millisecond, time.Second, 10)
	proxies := []Proxy{{Server: "192.0.2.1", Port: 9}}
	if got := checker.FindFirstWorking(context.Background(), proxies); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

// fakeConnect builds a connect probe from a fixed per-address timing table.
// Addresses absent from the table are treated as unreachable.
func fakeConnect(times map[string]time.Duration) func(context.Context, Proxy) (time.Duration, bool) {
	return func(_ context.Context, p Proxy) (time.Duration, bool) {
		d, ok := times[p.Address()]
		return d, ok
	}
}

// TestFastTriedBeforeSlow checks that fast proxies are verified before slow ones
// and that a slow proxy can still win when every fast proxy fails verification.
func TestFastTriedBeforeSlow(t *testing.T) {
	fastBad := Proxy{Server: "10.0.0.1", Port: 1}  // connects fast, fails verify
	slowGood := Proxy{Server: "10.0.0.2", Port: 2} // connects slow, passes verify

	checker := NewChecker(5*time.Second, time.Second, 8)
	checker.connect = fakeConnect(map[string]time.Duration{
		fastBad.Address():  100 * time.Millisecond,
		slowGood.Address(): 3 * time.Second,
	})

	var order []string
	var mu sync.Mutex
	checker.Verifier = verifierFunc(func(_ context.Context, p Proxy) error {
		mu.Lock()
		order = append(order, p.Address())
		mu.Unlock()
		if p.Address() == slowGood.Address() {
			return nil
		}
		return errors.New("nope")
	})

	got := checker.FindFirstWorking(context.Background(), []Proxy{slowGood, fastBad})
	if got == nil || got.Address() != slowGood.Address() {
		t.Fatalf("expected slow proxy to win, got %+v", got)
	}
	if len(order) != 2 || order[0] != fastBad.Address() || order[1] != slowGood.Address() {
		t.Fatalf("expected fast verified before slow, got %v", order)
	}
}

// TestSlowSkippedWhenFastWins checks that a fast usable proxy wins and the slow
// proxies are never verified.
func TestSlowSkippedWhenFastWins(t *testing.T) {
	fastGood := Proxy{Server: "10.0.0.1", Port: 1}
	slow := Proxy{Server: "10.0.0.2", Port: 2}
	dead := Proxy{Server: "10.0.0.3", Port: 3}

	checker := NewChecker(5*time.Second, time.Second, 8)
	checker.connect = fakeConnect(map[string]time.Duration{
		fastGood.Address(): 50 * time.Millisecond,
		slow.Address():     3 * time.Second,
		// dead is absent -> never connects
	})

	var verified []string
	var mu sync.Mutex
	checker.Verifier = verifierFunc(func(_ context.Context, p Proxy) error {
		mu.Lock()
		verified = append(verified, p.Address())
		mu.Unlock()
		return nil // anything verified is usable
	})

	got := checker.FindFirstWorking(context.Background(), []Proxy{fastGood, slow, dead})
	if got == nil || got.Address() != fastGood.Address() {
		t.Fatalf("expected fast proxy to win, got %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(verified) != 1 || verified[0] != fastGood.Address() {
		t.Fatalf("slow/dead proxies must not be verified; verified %v", verified)
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
