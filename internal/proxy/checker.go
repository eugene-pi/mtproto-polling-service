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

// FindFirstWorking checks the proxies in parallel (bounded by Concurrency) and
// returns the first usable one (see Usable). Remaining attempts are cancelled as
// soon as a winner is found. It returns nil if none are usable or ctx is
// cancelled.
func (c *Checker) FindFirstWorking(ctx context.Context, proxies []Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, c.Concurrency)
	winner := make(chan Proxy, 1)
	var wg sync.WaitGroup

	// The launcher itself is tracked by wg so the closer below cannot observe a
	// zero count and close `winner` before any worker has been registered.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, p := range proxies {
			select {
			case <-ctx.Done():
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
				if c.Usable(ctx, p) {
					select {
					case winner <- p:
						cancel() // stop the rest
					default: // someone already won
					}
				}
			}(p)
		}
	}()

	go func() {
		wg.Wait()
		close(winner)
	}()

	if p, ok := <-winner; ok {
		return &p
	}
	return nil
}
