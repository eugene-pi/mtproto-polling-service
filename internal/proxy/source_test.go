package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceDetectsUpdates(t *testing.T) {
	body := "https://t.me/proxy?server=a.example&port=443&secret=ee01\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := NewSource(srv.URL, srv.Client())
	ctx := context.Background()

	// First fetch is always reported as updated.
	res, err := s.Fetch(ctx)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if !res.Updated || len(res.Proxies) != 1 {
		t.Fatalf("first fetch: updated=%v proxies=%d", res.Updated, len(res.Proxies))
	}

	// Same content -> not updated, but still returns the cached list.
	res, err = s.Fetch(ctx)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if res.Updated {
		t.Fatalf("second fetch reported updated for unchanged content")
	}
	if len(res.Proxies) != 1 {
		t.Fatalf("second fetch should return cached proxies, got %d", len(res.Proxies))
	}

	// Change the content -> updated again.
	body = "https://t.me/proxy?server=a.example&port=443&secret=ee01\n" +
		"https://t.me/proxy?server=b.example&port=8443&secret=ee02\n"
	res, err = s.Fetch(ctx)
	if err != nil {
		t.Fatalf("third fetch: %v", err)
	}
	if !res.Updated || len(res.Proxies) != 2 {
		t.Fatalf("third fetch: updated=%v proxies=%d", res.Updated, len(res.Proxies))
	}
}
