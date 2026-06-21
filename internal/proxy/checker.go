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

// Checker tests proxies for connectability. A proxy is considered usable if a
// TCP connection to its host:port can be established within DialTimeout.
type Checker struct {
	// DialTimeout bounds each individual connection attempt.
	DialTimeout time.Duration
	// Concurrency bounds how many proxies are dialed at once.
	Concurrency int
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

// FindFirstWorking dials the proxies in parallel (bounded by Concurrency) and
// returns the first one that connects. Remaining attempts are cancelled as soon
// as a winner is found. It returns nil if none connect or ctx is cancelled.
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
				if c.Connectable(ctx, p) {
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
