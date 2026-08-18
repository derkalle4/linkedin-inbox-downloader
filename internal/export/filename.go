package export

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	nonAlnum     = regexp.MustCompile(`[^A-Za-z0-9]`)
	oldSuffixRE  = regexp.MustCompile(`_old\d+\.pdf$`)
	threadIDInFN = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_([A-Za-z0-9]+)_.+\.pdf$`)
)

// ShortThreadID returns a short unique prefix from the LinkedIn thread id.
func ShortThreadID(raw string) string {
	compact := nonAlnum.ReplaceAllString(raw, "")
	if strings.HasPrefix(strings.ToLower(compact), "2") && len(compact) > 12 {
		compact = compact[1:]
	}
	if len(compact) >= 12 {
		return compact[:12]
	}
	if compact != "" {
		return compact
	}
	sum := sha1.Sum([]byte(raw))
	if raw == "" {
		sum = sha1.Sum([]byte("thread"))
	}
	return hex.EncodeToString(sum[:])[:10]
}

// SanitizeName turns a participant name into a filesystem-safe alphanumeric token.
// Spaces become underscores; everything else non-alphanumeric is stripped.
func SanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "_")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "thread"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// PDFName builds {YYYY-MM-DD}_{shortID}_{Name}.pdf
func PDFName(threadID, person string, when time.Time) string {
	if when.IsZero() {
		when = time.Now()
	}
	return fmt.Sprintf("%s_%s_%s.pdf",
		when.Format("2006-01-02"),
		ShortThreadID(threadID),
		SanitizeName(person),
	)
}

// RotateExisting renames any current PDF for this thread id to *_oldN.pdf.
// Returns the path that was rotated (empty if none).
func RotateExisting(downloadDir, threadID string) (string, error) {
	short := ShortThreadID(threadID)
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var current string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
			continue
		}
		if oldSuffixRE.MatchString(name) {
			continue
		}
		m := threadIDInFN.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		if m[1] == short {
			current = filepath.Join(downloadDir, name)
			break
		}
	}
	if current == "" {
		return "", nil
	}

	stem := strings.TrimSuffix(filepath.Base(current), ".pdf")
	n := 1
	for {
		candidate := filepath.Join(downloadDir, fmt.Sprintf("%s_old%d.pdf", stem, n))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			if err := os.Rename(current, candidate); err != nil {
				return "", err
			}
			return candidate, nil
		}
		n++
	}
}
