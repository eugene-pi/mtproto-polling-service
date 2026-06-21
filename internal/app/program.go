// Package app wires the proxy manager and HTTP API into a kardianos service so
// the same binary can run as a Windows service or as an interactive console
// application.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/kardianos/service"

	"github.com/eugene-pi/mtproto-polling-service/internal/browser"
	"github.com/eugene-pi/mtproto-polling-service/internal/httpapi"
	"github.com/eugene-pi/mtproto-polling-service/internal/proxy"
)

// Config holds everything needed to run the service.
type Config struct {
	ListURL          string
	HTTPAddr         string
	PollInterval     time.Duration
	RetryInterval    time.Duration
	ValidateInterval time.Duration
	DialTimeout      time.Duration
	FastThreshold    time.Duration
	Concurrency      int

	// Telegram API credentials (required). They gate the second-stage check
	// that confirms a real Telegram client can use the proxy.
	TGAPIID       int
	TGAPIHash     string
	VerifyTimeout time.Duration

	// OpenBrowser opens each newly selected proxy URL in the default browser.
	OpenBrowser bool
}

// Program implements service.Interface.
type Program struct {
	cfg     Config
	log     *logger
	manager *proxy.Manager
	api     *httpapi.Server

	cancel context.CancelFunc
	done   chan struct{}
}

// NewProgram builds a Program from cfg.
func NewProgram(cfg Config) *Program {
	return &Program{cfg: cfg, done: make(chan struct{})}
}

// Start is called by kardianos when the service starts. It must not block.
func (p *Program) Start(s service.Service) error {
	svcLogger, err := s.Logger(nil)
	if err != nil {
		return err
	}
	p.log = newLogger(svcLogger)

	verifier, err := proxy.NewTelegramVerifier(p.cfg.TGAPIID, p.cfg.TGAPIHash, p.cfg.VerifyTimeout)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	source := proxy.NewSource(p.cfg.ListURL, httpClient)
	checker := proxy.NewChecker(p.cfg.DialTimeout, p.cfg.FastThreshold, p.cfg.Concurrency)
	checker.Verifier = verifier
	p.manager = proxy.NewManager(proxy.Config{
		PollInterval:     p.cfg.PollInterval,
		RetryInterval:    p.cfg.RetryInterval,
		ValidateInterval: p.cfg.ValidateInterval,
	}, source, checker, p.log)

	if p.cfg.OpenBrowser {
		interactive := service.Interactive()
		p.manager.OpenProxy = func(px proxy.Proxy) error {
			// Returns quickly (it only spawns the browser). The manager logs the
			// outcome and retries on the next health check if this fails.
			return browser.Open(px.URL, interactive)
		}
	}

	p.api = httpapi.New(p.cfg.HTTPAddr, p.manager)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go p.run(ctx)
	return nil
}

func (p *Program) run(ctx context.Context) {
	defer close(p.done)

	go func() {
		p.log.Infof("serving proxy API on http://%s/proxy", p.api.Addr())
		if err := p.api.Start(); err != nil {
			p.log.Errorf("http server stopped: %v", err)
		}
	}()

	if err := p.manager.Run(ctx); err != nil && err != context.Canceled {
		p.log.Errorf("proxy manager stopped: %v", err)
	}
}

// Stop is called by kardianos on shutdown.
func (p *Program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.api != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.api.Shutdown(shutdownCtx)
	}

	// Give run() a moment to unwind so logs flush before the process exits.
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
	}
	return nil
}
