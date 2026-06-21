package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type nopLogger struct{}

func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// proxyListServer serves a one-proxy list pointing at a fresh local listener and
// returns the listener's proxy. Both are cleaned up with the test.
func proxyListServer(t *testing.T) (*httptest.Server, Proxy) {
	t.Helper()
	lp := newListener(t)
	link := fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=ee00", lp.Server, lp.Port)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, link)
	}))
	t.Cleanup(srv.Close)
	return srv, lp
}

func newManager(t *testing.T, srv *httptest.Server, validate time.Duration) *Manager {
	source := NewSource(srv.URL, srv.Client())
	checker := NewChecker(time.Second, time.Second, 4) // no verifier: connectable == usable
	return NewManager(Config{
		PollInterval:     time.Hour,
		RetryInterval:    time.Hour,
		ValidateInterval: validate,
	}, source, checker, nopLogger{})
}

// TestManagerOpensProxy verifies OpenProxy is invoked with the served proxy.
func TestManagerOpensProxy(t *testing.T) {
	srv, lp := proxyListServer(t)
	m := newManager(t, srv, time.Hour)

	got := make(chan Proxy, 1)
	m.OpenProxy = func(p Proxy) error {
		select {
		case got <- p:
		default:
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	select {
	case p := <-got:
		if p.Server != lp.Server || p.Port != lp.Port {
			t.Fatalf("unexpected proxy: %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OpenProxy was not called")
	}
}

// TestManagerRetriesOpenUntilSession verifies that when OpenProxy keeps failing
// (e.g. no user is logged in), the manager retries on each health check and
// stops once it finally succeeds.
func TestManagerRetriesOpenUntilSession(t *testing.T) {
	srv, _ := proxyListServer(t)
	m := newManager(t, srv, 15*time.Millisecond)

	var (
		mu       sync.Mutex
		calls    int
		openedAt int
	)
	m.OpenProxy = func(Proxy) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls < 3 {
			return errors.New("no active user session")
		}
		openedAt = calls
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	// Wait until the open finally succeeds.
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		done := openedAt > 0
		mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			t.Fatal("OpenProxy never succeeded after retries")
		case <-time.After(5 * time.Millisecond):
		}
	}

	mu.Lock()
	callsAtSuccess := calls
	mu.Unlock()

	// After success it must not be called again, even across several rechecks.
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	finalCalls := calls
	mu.Unlock()
	if finalCalls != callsAtSuccess {
		t.Fatalf("OpenProxy called again after success: %d -> %d", callsAtSuccess, finalCalls)
	}
}
