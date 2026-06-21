package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFileConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"tgApiId": 42, "tgApiHash": "abc", "httpAddr": "0.0.0.0:9000", "pollInterval": "10m"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fc, err := LoadFileConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if fc.TGAPIID == nil || *fc.TGAPIID != 42 {
		t.Fatalf("tgApiId not parsed: %+v", fc.TGAPIID)
	}
	if fc.HTTPAddr == nil || *fc.HTTPAddr != "0.0.0.0:9000" {
		t.Fatalf("httpAddr not parsed: %+v", fc.HTTPAddr)
	}
	if fc.RetryInterval != nil {
		t.Fatalf("absent field should be nil, got %v", *fc.RetryInterval)
	}
}

func TestLoadFileConfigMissingIsNotExist(t *testing.T) {
	_, err := LoadFileConfig(filepath.Join(t.TempDir(), "nope.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist, got %v", err)
	}
}

func TestLoadFileConfigRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"nope": 1}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadFileConfig(path); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestResolvePrecedence(t *testing.T) {
	fileVal := "from-file"
	// flag set wins over everything.
	if got := ResolveString(true, "from-flag", "from-env", &fileVal); got != "from-flag" {
		t.Fatalf("flag should win, got %q", got)
	}
	// env wins over file when flag not set.
	if got := ResolveString(false, "default", "from-env", &fileVal); got != "from-env" {
		t.Fatalf("env should win, got %q", got)
	}
	// file wins over default when neither flag nor env set.
	if got := ResolveString(false, "default", "", &fileVal); got != "from-file" {
		t.Fatalf("file should win, got %q", got)
	}
	// default when nothing set.
	if got := ResolveString(false, "default", "", nil); got != "default" {
		t.Fatalf("default should win, got %q", got)
	}
}

func TestResolveDuration(t *testing.T) {
	s := "45s"
	got, err := ResolveDuration(false, time.Minute, &s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 45*time.Second {
		t.Fatalf("got %v", got)
	}

	// Flag set ignores the file value.
	got, err = ResolveDuration(true, time.Minute, &s)
	if err != nil || got != time.Minute {
		t.Fatalf("flag should win: got %v err %v", got, err)
	}

	// Invalid file duration is an error.
	bad := "notaduration"
	if _, err := ResolveDuration(false, time.Minute, &bad); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}
