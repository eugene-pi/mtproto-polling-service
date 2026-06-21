package proxy

import "testing"

func TestParseLine(t *testing.T) {
	got, err := ParseLine("https://t.me/proxy?server=example.com&port=443&secret=eeABCD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Server != "example.com" || got.Port != 443 || got.Secret != "eeABCD" {
		t.Fatalf("unexpected parse result: %+v", got)
	}
	if got.Address() != "example.com:443" {
		t.Fatalf("unexpected address: %q", got.Address())
	}
}

func TestParseLineTrailingDot(t *testing.T) {
	got, err := ParseLine("https://t.me/proxy?server=host.example.site.&port=44300&secret=ee00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Server != "host.example.site" {
		t.Fatalf("trailing dot not trimmed: %q", got.Server)
	}
}

func TestParseLineErrors(t *testing.T) {
	cases := []string{
		"",
		"# a comment",
		"https://t.me/proxy?server=&port=443",
		"https://t.me/proxy?server=example.com&port=notaport",
		"https://t.me/proxy?server=example.com&port=99999",
	}
	for _, c := range cases {
		if _, err := ParseLine(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestParseList(t *testing.T) {
	data := []byte(`
# header comment
https://t.me/proxy?server=a.example&port=443&secret=ee01

garbage line that is not a url-with-fields
https://t.me/proxy?server=b.example&port=8443&secret=ee02
`)
	got := ParseList(data)
	if len(got) != 2 {
		t.Fatalf("expected 2 proxies, got %d: %+v", len(got), got)
	}
	if got[0].Server != "a.example" || got[1].Port != 8443 {
		t.Fatalf("unexpected proxies: %+v", got)
	}
}
