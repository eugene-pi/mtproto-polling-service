// Command mtproto-polling-service keeps a working Telegram MTProto proxy on hand
// and serves it over a local HTTP endpoint. It can run as a Windows service or
// as an interactive console application.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kardianos/service"

	"github.com/eugene-pi/mtproto-polling-service/internal/app"
	"github.com/eugene-pi/mtproto-polling-service/internal/proxy"
)

func main() {
	var (
		control          = flag.String("service", "", "service control action: install, uninstall, start, stop, restart")
		listURL          = flag.String("list-url", proxy.DefaultListURL, "URL of the proxy list to poll")
		httpAddr         = flag.String("http-addr", "127.0.0.1:8080", "address for the local proxy API")
		pollInterval     = flag.Duration("poll-interval", 30*time.Minute, "wait between checks when no proxy works")
		retryInterval    = flag.Duration("retry-interval", time.Minute, "backoff when the list cannot be downloaded")
		validateInterval = flag.Duration("validate-interval", 2*time.Minute, "how often to re-verify the current proxy")
		dialTimeout      = flag.Duration("dial-timeout", 5*time.Second, "per-proxy TCP connect timeout")
		concurrency      = flag.Int("concurrency", 200, "max proxies dialed in parallel")
	)
	flag.Parse()

	prg := app.NewProgram(app.Config{
		ListURL:          *listURL,
		HTTPAddr:         *httpAddr,
		PollInterval:     *pollInterval,
		RetryInterval:    *retryInterval,
		ValidateInterval: *validateInterval,
		DialTimeout:      *dialTimeout,
		Concurrency:      *concurrency,
	})

	svcConfig := &service.Config{
		Name:        "MTProtoPollingService",
		DisplayName: "MTProto Polling Service",
		Description: "Finds a working Telegram MTProto proxy and serves it over a local HTTP endpoint.",
		Arguments: []string{
			"-list-url", *listURL,
			"-http-addr", *httpAddr,
			"-poll-interval", pollInterval.String(),
			"-retry-interval", retryInterval.String(),
			"-validate-interval", validateInterval.String(),
			"-dial-timeout", dialTimeout.String(),
			"-concurrency", fmt.Sprintf("%d", *concurrency),
		},
	}

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		fatalf("failed to initialise service: %v", err)
	}

	if *control != "" {
		if err := service.Control(svc, *control); err != nil {
			fatalf("service %s failed: %v\nvalid actions: %s",
				*control, err, strings.Join(service.ControlAction[:], ", "))
		}
		fmt.Printf("service action %q completed\n", *control)
		return
	}

	// service.Run blocks; it runs the service when launched by the service
	// manager and runs interactively (console mode) otherwise.
	if err := svc.Run(); err != nil {
		fatalf("service exited with error: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
