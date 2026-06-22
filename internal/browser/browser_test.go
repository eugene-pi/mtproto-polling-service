package browser

import (
	"strings"
	"testing"
)

func TestConsoleCommandIncludesURL(t *testing.T) {
	url := "tg://proxy?server=example.com&port=443&secret=ee00"
	name, args := consoleCommand(url)

	if name == "" {
		t.Fatal("empty command name")
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, url) {
		t.Fatalf("command %q %v does not include the URL", name, args)
	}
}
