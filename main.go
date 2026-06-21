// Command mtproto-polling-service keeps a working Telegram MTProto proxy on hand
// and serves it over a local HTTP endpoint. It can run as a Windows service or
// as an interactive console application.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
		configPath       = flag.String("config", "", "path to a JSON config file (defaults to ./config.json if present)")
		listURL          = flag.String("list-url", proxy.DefaultListURL, "URL of the proxy list to poll")
		httpAddr         = flag.String("http-addr", "127.0.0.1:8080", "address for the local proxy API")
		pollInterval     = flag.Duration("poll-interval", 30*time.Minute, "wait between checks when no proxy works")
		retryInterval    = flag.Duration("retry-interval", time.Minute, "backoff when the list cannot be downloaded")
		validateInterval = flag.Duration("validate-interval", 2*time.Minute, "how often to re-verify the current proxy")
		dialTimeout      = flag.Duration("dial-timeout", 5*time.Second, "per-proxy TCP connect timeout")
		concurrency      = flag.Int("concurrency", 200, "max proxies dialed in parallel")
		verifyTimeout    = flag.Duration("verify-timeout", 15*time.Second, "timeout for the Telegram client verification of a proxy")
		tgAPIID          = flag.Int("tg-api-id", 0, "Telegram api_id (required; from config file, $TG_API_ID, or this flag)")
		tgAPIHash        = flag.String("tg-api-hash", "", "Telegram api_hash (required; from config file, $TG_API_HASH, or this flag)")
	)
	flag.Parse()

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// configFile is the absolute path of the config file that was loaded, or ""
	// if none. It is baked into the service definition so an installed service
	// reads the same file from its own (system32) working directory.
	fc, configFile := loadConfig(*configPath, set["config"])

	// Resolve every setting with precedence: flag > env var > config file > default.
	cfg := app.Config{
		ListURL:       app.ResolveString(set["list-url"], *listURL, "", fc.ListURL),
		HTTPAddr:      app.ResolveString(set["http-addr"], *httpAddr, "", fc.HTTPAddr),
		Concurrency:   app.ResolveInt(set["concurrency"], *concurrency, 0, fc.Concurrency),
		TGAPIID:       app.ResolveInt(set["tg-api-id"], *tgAPIID, envInt("TG_API_ID"), fc.TGAPIID),
		TGAPIHash:     app.ResolveString(set["tg-api-hash"], *tgAPIHash, os.Getenv("TG_API_HASH"), fc.TGAPIHash),
		PollInterval:  mustDuration("poll-interval", set["poll-interval"], *pollInterval, fc.PollInterval),
		RetryInterval: mustDuration("retry-interval", set["retry-interval"], *retryInterval, fc.RetryInterval),
		ValidateInterval: mustDuration("validate-interval", set["validate-interval"],
			*validateInterval, fc.ValidateInterval),
		DialTimeout:   mustDuration("dial-timeout", set["dial-timeout"], *dialTimeout, fc.DialTimeout),
		VerifyTimeout: mustDuration("verify-timeout", set["verify-timeout"], *verifyTimeout, fc.VerifyTimeout),
	}

	// Telegram credentials are required for the second-stage proxy verification.
	if *control == "" && (cfg.TGAPIID == 0 || cfg.TGAPIHash == "") {
		fatalf("Telegram api_id/api_hash are required: set them in the config file, " +
			"via -tg-api-id/-tg-api-hash, or the TG_API_ID/TG_API_HASH environment " +
			"variables (get them from https://my.telegram.org)")
	}

	prg := app.NewProgram(cfg)

	// Bake the resolved settings into the service definition. A config file (if
	// one was loaded) is referenced by absolute path so the service can read it
	// — including the credentials, which are otherwise NOT stored in the service
	// arguments (they would land in the registry in clear text). Without a config
	// file, set TG_API_ID / TG_API_HASH as machine environment variables instead.
	args := []string{
		"-list-url", cfg.ListURL,
		"-http-addr", cfg.HTTPAddr,
		"-poll-interval", cfg.PollInterval.String(),
		"-retry-interval", cfg.RetryInterval.String(),
		"-validate-interval", cfg.ValidateInterval.String(),
		"-dial-timeout", cfg.DialTimeout.String(),
		"-concurrency", strconv.Itoa(cfg.Concurrency),
		"-verify-timeout", cfg.VerifyTimeout.String(),
	}
	if configFile != "" {
		args = append(args, "-config", configFile)
	}

	svcConfig := &service.Config{
		Name:        "MTProtoPollingService",
		DisplayName: "MTProto Polling Service",
		Description: "Finds a working Telegram MTProto proxy and serves it over a local HTTP endpoint.",
		Arguments:   args,
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
		if *control == "install" {
			if configFile != "" {
				fmt.Printf("service will read configuration (including credentials) from %s\n", configFile)
			} else {
				fmt.Println("note: no config file in use — set TG_API_ID/TG_API_HASH as machine " +
					"environment variables, or reinstall with -config <path>, so the service can " +
					"obtain Telegram credentials.")
			}
		}
		return
	}

	// service.Run blocks; it runs the service when launched by the service
	// manager and runs interactively (console mode) otherwise.
	if err := svc.Run(); err != nil {
		fatalf("service exited with error: %v", err)
	}
}

// loadConfig loads the JSON config file. When -config is given explicitly the
// file must exist; otherwise the default ./config.json is loaded only if present.
// It returns a usable (possibly empty) FileConfig and the absolute path of the
// file that was loaded ("" when none).
func loadConfig(path string, explicit bool) (*app.FileConfig, string) {
	if path == "" {
		path = app.DefaultConfigPath
	}
	fc, err := app.LoadFileConfig(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return &app.FileConfig{}, "" // no default config file: that's fine
		}
		fatalf("config: %v", err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		fatalf("config: resolve path %q: %v", path, err)
	}
	return fc, abs
}

// mustDuration resolves a duration setting or exits with a clear message when
// the config file holds an invalid value.
func mustDuration(name string, flagSet bool, flagVal time.Duration, file *string) time.Duration {
	d, err := app.ResolveDuration(flagSet, flagVal, file)
	if err != nil {
		fatalf("config: %s: %v", name, err)
	}
	return d
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
