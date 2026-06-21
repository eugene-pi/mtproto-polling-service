package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// DefaultConfigPath is loaded automatically when -config is not given and the
// file happens to exist. It is meant as a convenience for local debugging.
const DefaultConfigPath = "config.json"

// FileConfig mirrors the command-line options as an optional JSON config file.
// Every field is a pointer so we can tell "absent" from "set to zero value" and
// apply the correct precedence. Durations are written as strings, e.g. "30m".
type FileConfig struct {
	ListURL          *string `json:"listUrl,omitempty"`
	HTTPAddr         *string `json:"httpAddr,omitempty"`
	PollInterval     *string `json:"pollInterval,omitempty"`
	RetryInterval    *string `json:"retryInterval,omitempty"`
	ValidateInterval *string `json:"validateInterval,omitempty"`
	DialTimeout      *string `json:"dialTimeout,omitempty"`
	FastThreshold    *string `json:"fastThreshold,omitempty"`
	Concurrency      *int    `json:"concurrency,omitempty"`
	VerifyTimeout    *string `json:"verifyTimeout,omitempty"`
	TGAPIID          *int    `json:"tgApiId,omitempty"`
	TGAPIHash        *string `json:"tgApiHash,omitempty"`
}

// LoadFileConfig reads and parses a JSON config file. Unknown fields are
// rejected so typos surface immediately while debugging. The returned error
// satisfies os.IsNotExist when the file is missing, so callers can treat an
// absent default file as "no config".
func LoadFileConfig(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var fc FileConfig
	if err := dec.Decode(&fc); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &fc, nil
}

// The Resolve* helpers implement the precedence
// explicit flag > environment variable > config file > built-in default.
// flagVal already equals the built-in default when the flag was not set, so it
// doubles as the final fallback. Env arguments are passed empty/zero when a
// setting has no environment variable.

// ResolveString resolves a string setting.
func ResolveString(flagSet bool, flagVal, env string, file *string) string {
	switch {
	case flagSet:
		return flagVal
	case env != "":
		return env
	case file != nil:
		return *file
	default:
		return flagVal
	}
}

// ResolveInt resolves an integer setting. An env value of 0 is treated as unset.
func ResolveInt(flagSet bool, flagVal, env int, file *int) int {
	switch {
	case flagSet:
		return flagVal
	case env != 0:
		return env
	case file != nil:
		return *file
	default:
		return flagVal
	}
}

// ResolveDuration resolves a duration setting. The file value is a duration
// string (e.g. "30m") and a parse failure is reported as an error.
func ResolveDuration(flagSet bool, flagVal time.Duration, file *string) (time.Duration, error) {
	if flagSet || file == nil {
		return flagVal, nil
	}
	d, err := time.ParseDuration(*file)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", *file, err)
	}
	return d, nil
}
