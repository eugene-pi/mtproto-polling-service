package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type nopLogger struct{}

func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// TestManagerOnNewProxy verifies the manager invokes OnNewProxy with the proxy
// it starts serving.
func TestManagerOnNewProxy(t *testing.T) {
	// A connectable proxy backed by a local listener.
	lp := newListener(t)
	link := fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=ee00", lp.Server, lp.Port)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, link)
	}))
	defer srv.Close()

	source := NewSource(srv.URL, srv.Client())
	checker := NewChecker(time.Second, time.Second, 4) // no verifier: connectable == usable
	m := NewManager(Config{
		PollInterval:     time.Hour,
		RetryInterval:    time.Hour,
		ValidateInterval: time.Hour,
	}, source, checker, nopLogger{})

	got := make(chan Proxy, 1)
	m.OnNewProxy = func(p Proxy) {
		select {
		case got <- p:
		default:
		}
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
		t.Fatal("OnNewProxy was not called")
	}
}
