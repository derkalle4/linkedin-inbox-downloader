package browser

import (
	"encoding/json"
	"os"
	"time"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/paths"
)

// Cookie is a serializable browser cookie.
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

// LoadCookies reads linkedin_cookies.json next to the executable.
func LoadCookies() ([]Cookie, error) {
	path, err := paths.CookiesFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cookies []Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, err
	}
	return cookies, nil
}

// SaveCookies writes linkedin_cookies.json next to the executable.
func SaveCookies(cookies []Cookie) error {
	path, err := paths.CookiesFile()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// CookiesExist reports whether a cookie file is present.
func CookiesExist() bool {
	path, err := paths.CookiesFile()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// FilterLive drops expired cookies.
func FilterLive(cookies []Cookie) []Cookie {
	now := float64(time.Now().Unix())
	out := make([]Cookie, 0, len(cookies))
	for _, c := range cookies {
		if c.Expires > 0 && c.Expires < now {
			continue
		}
		out = append(out, c)
	}
	return out
}
