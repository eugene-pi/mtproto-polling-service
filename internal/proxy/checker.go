package proxy

import (
	"context"
	"net"
	"sort"
	"sync"
	"time"
)

const (
	defaultDialTimeout   = 5 * time.Second
	defaultFastThreshold = 1 * time.Second
	defaultConcurrency   = 200
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
	// FastThreshold is the connect-time boundary between "fast" proxies (tried
	// first) and "slow" ones (tried only if no fast proxy is usable).
	FastThreshold time.Duration
	// Concurrency bounds how many proxies are dialed at once.
	Concurrency int
	// Verifier, when set, is run after a successful TCP connection. It is the
	// second-stage check (e.g. a real Telegram client connection).
	Verifier Verifier

	// connect measures the TCP connect time of a proxy; ok is false when it does
	// not connect within DialTimeout. It is a field so tests can substitute a
	// deterministic probe; nil means use the real TCP dialer.
	connect func(ctx context.Context, p Proxy) (d time.Duration, ok bool)
}

// NewChecker returns a Checker with sensible defaults applied for any zero
// fields.
func NewChecker(dialTimeout, fastThreshold time.Duration, concurrency int) *Checker {
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	if fastThreshold <= 0 {
		fastThreshold = defaultFastThreshold
	}
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	return &Checker{
		DialTimeout:   dialTimeout,
		FastThreshold: fastThreshold,
		Concurrency:   concurrency,
	}
}

// measure returns the proxy's TCP connect time, using the injected probe when
// present and the real dialer otherwise.
func (c *Checker) measure(ctx context.Context, p Proxy) (time.Duration, bool) {
	if c.connect != nil {
		return c.connect(ctx, p)
	}

	dialCtx, cancel := context.WithTimeout(ctx, c.DialTimeout)
	defer cancel()

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", p.Address())
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, false
	}
	_ = conn.Close()
	return elapsed, true
}

// Connectable reports whether a TCP connection to the proxy can be established.
func (c *Checker) Connectable(ctx context.Context, p Proxy) bool {
	_, ok := c.measure(ctx, p)
	return ok
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
// It measures each proxy's connect time in parallel (bounded by Concurrency) and
// verifies candidates in two passes:
//
//   - First pass: "fast" proxies (connect time <= FastThreshold) are verified —
//     strictly one-by-one, since the Verifier shares a single set of credentials.
//     They are streamed in as soon as they connect, so verification can start
//     before the whole list has been dialed.
//   - Second pass: only if no fast proxy was usable, the "slow" proxies
//     (FastThreshold < connect time <= DialTimeout) are verified one-by-one,
//     fastest first. Fast proxies that already failed verification are not
//     retried.
//
// The first proxy that passes verification wins and all remaining work is
// cancelled.
func (c *Checker) FindFirstWorking(ctx context.Context, proxies []Proxy) *Proxy {
	if len(proxies) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // returning cancels the parallel dial stage below

	fast, slowResult := c.streamCandidates(ctx, proxies)

	// First pass: fast proxies, streamed and verified one at a time.
	for p := range fast {
		if ctx.Err() != nil {
			return nil
		}
		if c.verify(ctx, p) {
			return &p
		}
	}

	// Second pass: slow proxies (fastest first), only reached when no fast proxy
	// was usable. The dial stage is complete once `fast` is closed, so the full
	// slow list is ready.
	for _, p := range <-slowResult {
		if ctx.Err() != nil {
			return nil
		}
		if c.verify(ctx, p) {
			return &p
		}
	}
	return nil
}

// verify runs the configured Verifier, returning true when the proxy is usable.
// With no Verifier, a connectable proxy is considered usable.
func (c *Checker) verify(ctx context.Context, p Proxy) bool {
	if c.Verifier == nil {
		return true
	}
	return c.Verifier.Verify(ctx, p) == nil
}

// timedProxy pairs a proxy with its measured connect time.
type timedProxy struct {
	proxy   Proxy
	connect time.Duration
}

// streamCandidates dials every proxy in parallel (bounded by Concurrency). Fast
// proxies are emitted on the returned channel as they connect; that channel is
// closed once every proxy has been dialed (or ctx is cancelled). Slow proxies
// are collected and delivered, sorted fastest-first, on the second channel after
// dialing completes. A fast worker holds its concurrency slot until its result
// is consumed, which paces dialing to the serial verification downstream.
func (c *Checker) streamCandidates(ctx context.Context, proxies []Proxy) (<-chan Proxy, <-chan []Proxy) {
	fast := make(chan Proxy)
	slowResult := make(chan []Proxy, 1)

	go func() {
		sem := make(chan struct{}, c.Concurrency)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var slow []timedProxy

	launch:
		for _, p := range proxies {
			select {
			case <-ctx.Done():
				break launch
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(p Proxy) {
				defer wg.Done()
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}
				d, ok := c.measure(ctx, p)
				if !ok {
					return
				}
				if d <= c.FastThreshold {
					select {
					case fast <- p:
					case <-ctx.Done():
					}
					return
				}
				mu.Lock()
				slow = append(slow, timedProxy{proxy: p, connect: d})
				mu.Unlock()
			}(p)
		}

		wg.Wait()
		close(fast)

		sort.Slice(slow, func(i, j int) bool { return slow[i].connect < slow[j].connect })
		ordered := make([]Proxy, len(slow))
		for i, tp := range slow {
			ordered[i] = tp.proxy
		}
		slowResult <- ordered
	}()

	return fast, slowResult
}
