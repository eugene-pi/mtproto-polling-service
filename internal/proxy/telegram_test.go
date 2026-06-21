package proxy

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewTelegramVerifierRequiresCredentials(t *testing.T) {
	if _, err := NewTelegramVerifier(0, "", 0); !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("expected ErrMissingCredentials, got %v", err)
	}
	if _, err := NewTelegramVerifier(123, "", 0); !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("expected ErrMissingCredentials for missing hash, got %v", err)
	}
	if _, err := NewTelegramVerifier(123, "hash", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyRejectsInvalidSecret(t *testing.T) {
	v, err := NewTelegramVerifier(123, "hash", time.Second)
	if err != nil {
		t.Fatalf("build verifier: %v", err)
	}
	// "zz" is not valid hex, so verification fails before any network use.
	p := Proxy{Server: "example.com", Port: 443, Secret: "zz"}
	if err := v.Verify(context.Background(), p); err == nil {
		t.Fatal("expected error for invalid secret hex")
	}
}

// stubVerifier lets checker tests exercise the second stage without a network.
type stubVerifier struct {
	allow map[string]bool
}

func (s stubVerifier) Verify(_ context.Context, p Proxy) error {
	if s.allow[p.Address()] {
		return nil
	}
	return errors.New("not allowed")
}

func TestCheckerUsesVerifier(t *testing.T) {
	// Two real listeners so both pass the TCP stage; the verifier only allows
	// the second one.
	a := newListener(t)
	b := newListener(t)

	checker := NewChecker(time.Second, time.Second, 10)
	checker.Verifier = stubVerifier{allow: map[string]bool{b.Address(): true}}

	got := checker.FindFirstWorking(context.Background(), []Proxy{a, b})
	if got == nil {
		t.Fatal("expected a usable proxy")
	}
	if got.Address() != b.Address() {
		t.Fatalf("verifier ignored: winner %s, wanted %s", got.Address(), b.Address())
	}
}
