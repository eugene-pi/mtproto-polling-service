// Package proxy parses, validates and serves Telegram MTProto proxies.
package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Proxy is a single Telegram MTProto proxy parsed from a t.me/proxy link.
type Proxy struct {
	Server string `json:"server"`
	Port   int    `json:"port"`
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

// Address returns the host:port the proxy listens on, suitable for net.Dial.
func (p Proxy) Address() string {
	return net.JoinHostPort(p.Server, strconv.Itoa(p.Port))
}

// TelegramURL returns the proxy as a tg:// deep link. Opening it hands the proxy
// straight to the Telegram desktop app, which applies it to its settings —
// unlike the https://t.me/proxy form, which opens a web page first.
func (p Proxy) TelegramURL() string {
	return fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s",
		url.QueryEscape(p.Server), p.Port, url.QueryEscape(p.Secret))
}

// ParseLine parses a single `https://t.me/proxy?server=...&port=...&secret=...`
// link into a Proxy. It returns an error for blank/comment lines or links that
// are missing a server or a valid port.
func ParseLine(line string) (Proxy, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Proxy{}, fmt.Errorf("empty line")
	}

	u, err := url.Parse(line)
	if err != nil {
		return Proxy{}, fmt.Errorf("parse url: %w", err)
	}

	q := u.Query()
	server := strings.TrimSuffix(strings.TrimSpace(q.Get("server")), ".")
	if server == "" {
		return Proxy{}, fmt.Errorf("missing server in %q", line)
	}

	port, err := strconv.Atoi(strings.TrimSpace(q.Get("port")))
	if err != nil {
		return Proxy{}, fmt.Errorf("invalid port in %q: %w", line, err)
	}
	if port < 1 || port > 65535 {
		return Proxy{}, fmt.Errorf("port out of range in %q: %d", line, port)
	}

	return Proxy{
		Server: server,
		Port:   port,
		Secret: strings.TrimSpace(q.Get("secret")),
		URL:    line,
	}, nil
}

// ParseList parses a proxy list file, one link per line. Blank lines, lines
// starting with '#' and any links that fail to parse are skipped so a few bad
// entries never discard the whole list.
func ParseList(data []byte) []Proxy {
	var out []Proxy
	sc := bufio.NewScanner(bytes.NewReader(data))
	// Links can be long (long hex secrets); raise the line limit from 64KiB.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if p, err := ParseLine(line); err == nil {
			out = append(out, p)
		}
	}
	return out
}
