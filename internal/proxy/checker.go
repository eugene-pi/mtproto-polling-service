package proxy

import (
	"context"
	"net"
	"sync"
	"time"
)

const (
	defaultDialTimeout = 5 * time.Second
	defaultConcurrency = 200
)

// Verifier performs a deeper, application-level check on a proxy that already
// passed the TCP-connect stage (e.g. confirming a Telegram client can use it).
type Verifier interface {
	Verify(ctx context.Context, p Proxy) error
}

// Checker tests proxies for usability. A proxy is usable if a TCP connection to
// its host:port can be established within DialTimeout and, when a Verifier is
// configured, that verifier also succeeds.
type Checker struct {
	// DialTimeout bounds each individual connection attempt.
	DialTimeout time.Duration
	// Concurrency bounds how many proxies are dialed at once.
	Concurrency int
	// Verifier, when set, is run after a successful TCP connection. It is the
	// second-stage check (e.g. a real Telegram client connection).
	Verifier Verifier
}

// NewChecker returns a Checker with sensible defaults applied for any zero
// fields.
func NewChecker(dialTimeout time.Duration, concurrency int) *Checker {
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	return &Checker{DialTimeout: dialTimeout, Concurrency: concurrency}
}

// Connectable reports whether a TCP connection to the proxy can be established.
func (c *Checker) Connectable(ctx context.Context, p Proxy) bool {
	dialCtx, cancel := context.WithTimeout(ctx, c.DialTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", p.Address())
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Usable reports whether the proxy passes every configured check: it must be
// TCP-connectable and, if a Verifier is set, pass verification.
func (c *Checker) Usable(ctx context.Context, p Proxy) bool {
	if !c.Connectable(ctx, p) {
		return false
	}
	if c.Verifier != nil {
		if err := c.Verifier.Verify(ctx, p); err != nil {
			return false
		}
	}
	return true
}

// FindFirstWorking returns the first usable proxy, or nil if none are usable or
// ctx is cancelled.
//
// The two stages run with different concurrency on purpose. TCP connectivity is
// cheap, so it is checked in parallel (bounded by Concurrency). The Verifier
// stage (e.g. a real Telegram client connection) is expensive and shares one set
// of credentials, so it must not run many at once: connectable proxies are
// funnelled through a single channel and verified strictly one-by-one. The first
// proxy that passes verification wins and all remaining work is cancelled.
func (c *Checker) FindFirstWorking(ctx context.Context, proxies []Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // returning cancels the parallel TCP stage below

	// Stage 1: parallel TCP checks, emitting connectable proxies on a channel.
	connectable := c.streamConnectable(ctx, proxies)

	// Stage 2: verify connectable proxies one at a time. The first that passes
	// is returned; the deferred cancel then unwinds stage 1.
	for p := range connectable {
		if ctx.Err() != nil {
			return nil
		}
		if c.Verifier != nil {
			if err := c.Verifier.Verify(ctx, p); err != nil {
				continue
			}
		}
		return &p
	}
	return nil
}

// streamConnectable checks the proxies for TCP connectivity in parallel (bounded
// by Concurrency) and emits the connectable ones on the returned channel, which
// is closed once every proxy has been checked or ctx is cancelled. A connectable
// worker holds its concurrency slot until its result is consumed, which paces the
// parallel TCP stage to the serial verification stage downstream.
func (c *Checker) streamConnectable(ctx context.Context, proxies []Proxy) <-chan Proxy {
	out := make(chan Proxy)

	go func() {
		defer close(out)

		sem := make(chan struct{}, c.Concurrency)
		var wg sync.WaitGroup

		for _, p := range proxies {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(p Proxy) {
				defer wg.Done()
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}
				if c.Connectable(ctx, p) {
					select {
					case out <- p:
					case <-ctx.Done():
					}
				}
			}(p)
		}
		wg.Wait()
	}()

	return out
}
