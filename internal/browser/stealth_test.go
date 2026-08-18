package browser

import (
	"strings"
	"testing"
)

func TestHeadedUserAgentChrome(t *testing.T) {
	f := &Found{Name: "Google Chrome", Path: "/usr/bin/google-chrome"}
	ua := headedUserAgent(f, "141.0.7390.122")
	if !strings.Contains(ua, "Chrome/141.0.0.0") {
		t.Fatalf("ua=%q missing Chrome major", ua)
	}
	if strings.Contains(ua, "HeadlessChrome") {
		t.Fatalf("ua=%q still headless", ua)
	}
	if strings.Contains(ua, "Edg/") {
		t.Fatalf("ua=%q should not have Edg for Chrome", ua)
	}
	if !strings.HasPrefix(ua, "Mozilla/5.0") {
		t.Fatalf("ua=%q missing Mozilla prefix", ua)
	}
}

func TestHeadedUserAgentEdge(t *testing.T) {
	f := &Found{Name: "Microsoft Edge", Path: "/usr/bin/microsoft-edge"}
	ua := headedUserAgent(f, "141.0.3537.57")
	if !strings.Contains(ua, "Chrome/141.0.0.0") {
		t.Fatalf("ua=%q missing Chrome brand", ua)
	}
	if !strings.Contains(ua, "Edg/141.0.0.0") {
		t.Fatalf("ua=%q missing Edg suffix", ua)
	}
	if strings.Contains(ua, "HeadlessChrome") {
		t.Fatalf("ua=%q still headless", ua)
	}
}

func TestHeadedUserAgentEmptyVersion(t *testing.T) {
	f := &Found{Name: "Chromium", Path: "/usr/bin/chromium"}
	ua := headedUserAgent(f, "")
	if !strings.Contains(ua, "Chrome/") {
		t.Fatalf("ua=%q", ua)
	}
	if strings.Contains(ua, "HeadlessChrome") {
		t.Fatalf("ua=%q", ua)
	}
}

func TestStripHeadlessChrome(t *testing.T) {
	in := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/141.0.0.0 Safari/537.36"
	out := stripHeadlessChrome(in)
	if strings.Contains(out, "HeadlessChrome") {
		t.Fatalf("still headless: %q", out)
	}
	if !strings.Contains(out, "Chrome/141.0.0.0") {
		t.Fatalf("missing Chrome: %q", out)
	}
}

func TestChromeMajor(t *testing.T) {
	if got := chromeMajor("141.0.7390.122"); got != "141" {
		t.Fatalf("got %q", got)
	}
	if got := chromeMajor(""); got != "0" {
		t.Fatalf("got %q", got)
	}
}
