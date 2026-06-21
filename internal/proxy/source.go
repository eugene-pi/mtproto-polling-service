package proxy

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultListURL is the raw URL of the public proxy list referenced by the
// project issue. The issue links to the GitHub "blob" page; the raw URL below
// serves the file contents directly.
const DefaultListURL = "https://raw.githubusercontent.com/SoliSpirit/mtproto/master/all_proxies.txt"

// Source downloads the proxy list and tracks whether it changed between
// fetches. It uses HTTP conditional requests (ETag) when the server supports
// them and falls back to comparing a SHA-256 of the body, so "was the list
// updated?" is answered reliably either way.
type Source struct {
	url    string
	client *http.Client

	etag    string
	hash    [32]byte
	hasHash bool
	last    []Proxy
}

// FetchResult is the outcome of a single fetch.
type FetchResult struct {
	// Proxies is the parsed list. On an unchanged (304) response it is the
	// previously parsed list, so callers always get a usable slice.
	Proxies []Proxy
	// Updated reports whether the content differs from the previous fetch.
	// The first successful fetch is always reported as updated.
	Updated bool
}

// NewSource creates a Source for the given list URL. An empty url falls back to
// DefaultListURL.
func NewSource(url string, client *http.Client) *Source {
	if url == "" {
		url = DefaultListURL
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Source{url: url, client: client}
}

// Fetch downloads the proxy list and reports whether it changed since the last
// call. It is not safe for concurrent use.
func (s *Source) Fetch(ctx context.Context) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	if s.etag != "" {
		req.Header.Set("If-None-Match", s.etag)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download proxy list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &FetchResult{Proxies: s.last, Updated: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download proxy list: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read proxy list: %w", err)
	}

	sum := sha256.Sum256(body)
	updated := !s.hasHash || sum != s.hash

	s.hash = sum
	s.hasHash = true
	s.etag = resp.Header.Get("ETag")
	if updated {
		s.last = ParseList(body)
	}

	return &FetchResult{Proxies: s.last, Updated: updated}, nil
}
