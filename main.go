// Command mtproto-polling-service keeps a working Telegram MTProto proxy on hand
// and serves it over a local HTTP endpoint. It can run as a Windows service or
// as an interactive console application.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
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
		verifyTimeout    = flag.Duration("verify-timeout", 15*time.Second, "timeout for the Telegram client verification of a proxy")
		tgAPIID          = flag.Int("tg-api-id", envInt("TG_API_ID"), "Telegram api_id (required; defaults to $TG_API_ID)")
		tgAPIHash        = flag.String("tg-api-hash", os.Getenv("TG_API_HASH"), "Telegram api_hash (required; defaults to $TG_API_HASH)")
	)
	flag.Parse()

	// Telegram credentials are required for the second-stage proxy verification.
	if *control == "" && (*tgAPIID == 0 || *tgAPIHash == "") {
		fatalf("Telegram api_id/api_hash are required: set -tg-api-id/-tg-api-hash " +
			"or the TG_API_ID/TG_API_HASH environment variables (get them from https://my.telegram.org)")
	}

	prg := app.NewProgram(app.Config{
		ListURL:          *listURL,
		HTTPAddr:         *httpAddr,
		PollInterval:     *pollInterval,
		RetryInterval:    *retryInterval,
		ValidateInterval: *validateInterval,
		DialTimeout:      *dialTimeout,
		Concurrency:      *concurrency,
		TGAPIID:          *tgAPIID,
		TGAPIHash:        *tgAPIHash,
		VerifyTimeout:    *verifyTimeout,
	})

	svcConfig := &service.Config{
		Name:        "MTProtoPollingService",
		DisplayName: "MTProto Polling Service",
		Description: "Finds a working Telegram MTProto proxy and serves it over a local HTTP endpoint.",
		// Note: Telegram credentials are deliberately NOT baked into the service
		// arguments (they would be stored in the registry in clear text). Set
		// TG_API_ID / TG_API_HASH as machine environment variables instead so
		// the service account picks them up.
		Arguments: []string{
			"-list-url", *listURL,
			"-http-addr", *httpAddr,
			"-poll-interval", pollInterval.String(),
			"-retry-interval", retryInterval.String(),
			"-validate-interval", validateInterval.String(),
			"-dial-timeout", dialTimeout.String(),
			"-concurrency", strconv.Itoa(*concurrency),
			"-verify-timeout", verifyTimeout.String(),
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

// envInt reads an integer environment variable, returning 0 when unset or
// unparsable so it can serve as a flag default.
func envInt(name string) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return 0
	}
	return n
}
