package browser

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// productVersion runs browserPath --product-version and returns a trimmed version
// string like "141.0.7390.122". Empty on failure.
func productVersion(browserPath string) string {
	if browserPath == "" {
		return ""
	}
	out, err := exec.Command(browserPath, "--product-version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// chromeMajor extracts the major Chrome/Chromium version from a product-version
// string. Returns "0" if unknown.
func chromeMajor(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "0"
	}
	if i := strings.IndexByte(version, '.'); i > 0 {
		return version[:i]
	}
	return version
}

// headedUserAgent builds a normal (non-HeadlessChrome) User-Agent for the given
// browser binary. Edge keeps an Edg/ suffix; Chrome/Chromium do not.
func headedUserAgent(found *Found, version string) string {
	if version == "" {
		version = "141.0.0.0"
	}
	// Normalize to N.0.0.0 style often seen in UAs when only a full product
	// version is available — keep the real version string when present.
	uaVersion := version
	if parts := strings.Split(version, "."); len(parts) >= 1 && parts[0] != "" {
		// Prefer major.0.0.0 for stability with Client Hints; full version is fine too.
		uaVersion = parts[0] + ".0.0.0"
	}

	osToken := uaOSToken()
	base := fmt.Sprintf(
		"Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36",
		osToken, uaVersion,
	)
	if found != nil && found.IsEdge() {
		return base + " Edg/" + uaVersion
	}
	return base
}

func uaOSToken() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows NT 10.0; Win64; x64"
	case "darwin":
		return "Macintosh; Intel Mac OS X 10_15_7"
	default:
		return "X11; Linux x86_64"
	}
}

// stripHeadlessChrome replaces HeadlessChrome brand tokens with Chrome so a
// leftover headless UA looks headed.
func stripHeadlessChrome(ua string) string {
	return strings.ReplaceAll(ua, "HeadlessChrome", "Chrome")
}

// stealthInitJS runs on every new document: hide navigator.webdriver and fix
// any HeadlessChrome leftover in navigator.userAgent.
const stealthInitJS = `(function () {
  try {
    Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
  } catch (e) {}
  try {
    const ua = navigator.userAgent || '';
    if (ua.indexOf('HeadlessChrome') !== -1) {
      const fixed = ua.replace(/HeadlessChrome/g, 'Chrome');
      Object.defineProperty(navigator, 'userAgent', { get: () => fixed });
    }
  } catch (e) {}
})();`

// HumanPause sleeps a random duration in [min, max]. If max <= min, sleeps min.
func HumanPause(min, max time.Duration) {
	if min < 0 {
		min = 0
	}
	if max <= min {
		time.Sleep(min)
		return
	}
	d := min + time.Duration(randInt63n(int64(max-min+1)))
	time.Sleep(d)
}

// BetweenConversations is the shared pause after finishing one thread backup.
func BetweenConversations() {
	HumanPause(2*time.Second, 5*time.Second)
}
