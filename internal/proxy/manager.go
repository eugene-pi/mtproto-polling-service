package proxy

import (
	"context"
	"sync"
	"time"
)

// Logger is the minimal logging surface the manager needs. It is satisfied by
// the application's logger as well as by a simple stdlib-backed adapter.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Config tunes the manager's behaviour.
type Config struct {
	// PollInterval is how long to wait before re-checking the list when no
	// proxy currently works (the issue specifies 30 minutes).
	PollInterval time.Duration
	// RetryInterval is the (shorter) backoff used when the list cannot be
	// downloaded at all.
	RetryInterval time.Duration
	// ValidateInterval is how often the current working proxy is re-verified.
	ValidateInterval time.Duration
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = 30 * time.Minute
	}
	if c.RetryInterval <= 0 {
		c.RetryInterval = time.Minute
	}
	if c.ValidateInterval <= 0 {
		c.ValidateInterval = 2 * time.Minute
	}
	return c
}

// Manager keeps a working proxy on hand: it downloads the list, finds a
// connectable proxy, serves it, and re-searches whenever the current one dies
// or — when nothing works — whenever the published list changes.
type Manager struct {
	cfg     Config
	source  *Source
	checker *Checker
	log     Logger

	// OnNewProxy, when set, is called whenever the served proxy changes to a
	// different one (including the first proxy found). It runs on the manager's
	// goroutine, so it must not block.
	OnNewProxy func(Proxy)

	mu      sync.RWMutex
	current *Proxy
}

// NewManager wires a manager together.
func NewManager(cfg Config, source *Source, checker *Checker, log Logger) *Manager {
	return &Manager{
		cfg:     cfg.withDefaults(),
		source:  source,
		checker: checker,
		log:     log,
	}
}

// Current returns the proxy currently being served, if any.
func (m *Manager) Current() (Proxy, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return Proxy{}, false
	}
	return *m.current, true
}

func (m *Manager) setCurrent(p *Proxy) {
	m.mu.Lock()
	m.current = p
	m.mu.Unlock()
}

// Run blocks until ctx is cancelled, continuously ensuring a working proxy is
// available. It returns ctx.Err() on shutdown.
func (m *Manager) Run(ctx context.Context) error {
	var lastURL string
	for {
		p, err := m.findWorking(ctx)
		if err != nil {
			return err
		}

		m.setCurrent(p)
		m.log.Infof("serving working proxy %s", p.Address())

		if p.URL != lastURL {
			lastURL = p.URL
			if m.OnNewProxy != nil {
				m.OnNewProxy(*p)
			}
		}

		// Hold this proxy until it stops connecting or we are shut down.
		if err := m.monitor(ctx, *p); err != nil {
			return err
		}

		m.setCurrent(nil)
		m.log.Warnf("proxy %s stopped responding; searching for a replacement", p.Address())
	}
}

// findWorking downloads the list and returns the first connectable proxy. When
// nothing works it waits PollInterval and keeps waiting until the published
// list changes, then re-checks the fresh list — exactly the behaviour the issue
// describes.
func (m *Manager) findWorking(ctx context.Context) (*Proxy, error) {
	res, err := m.fetch(ctx)
	if err != nil {
		return nil, err
	}

	for {
		m.log.Infof("checking %d proxies for connectivity", len(res.Proxies))
		if p := m.checker.FindFirstWorking(ctx, res.Proxies); p != nil {
			return p, nil
		}

		m.log.Warnf("no working proxy among %d; waiting %s before checking for an updated list",
			len(res.Proxies), m.cfg.PollInterval)

		// Wait PollInterval, then keep waiting until the list actually changes.
		for {
			if err := sleep(ctx, m.cfg.PollInterval); err != nil {
				return nil, err
			}
			res, err = m.fetch(ctx)
			if err != nil {
				return nil, err
			}
			if res.Updated {
				m.log.Infof("proxy list updated; re-checking")
				break
			}
			m.log.Infof("proxy list unchanged; waiting another %s", m.cfg.PollInterval)
		}
	}
}

// monitor re-verifies the current proxy on an interval and returns nil once it
// stops connecting, or ctx.Err() on shutdown.
func (m *Manager) monitor(ctx context.Context, p Proxy) error {
	for {
		if err := sleep(ctx, m.cfg.ValidateInterval); err != nil {
			return err
		}
		if !m.checker.Usable(ctx, p) {
			return nil
		}
	}
}

// fetch downloads the list, retrying transient failures with RetryInterval
// until it succeeds or ctx is cancelled.
func (m *Manager) fetch(ctx context.Context) (*FetchResult, error) {
	for {
		res, err := m.source.Fetch(ctx)
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		m.log.Errorf("failed to download proxy list: %v; retrying in %s", err, m.cfg.RetryInterval)
		if err := sleep(ctx, m.cfg.RetryInterval); err != nil {
			return nil, err
		}
	}
}

// sleep waits for d or returns ctx.Err() if cancelled first.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
