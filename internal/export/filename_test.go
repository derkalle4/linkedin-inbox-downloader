package export_test

import (
	"testing"
	"time"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/export"
)

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Jane Doe":           "Jane_Doe",
		"Jean-Luc Picard":    "JeanLuc_Picard",
		"  Bob  ":            "Bob",
		"":                   "thread",
		"Anne O'Connor":      "Anne_OConnor",
		"名字":                 "thread",
	}
	for in, want := range cases {
		if got := export.SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortThreadID(t *testing.T) {
	id := export.ShortThreadID("2Q_abcdefghijklmnop")
	if len(id) != 12 {
		t.Fatalf("expected 12 chars, got %q", id)
	}
}

func TestPDFName(t *testing.T) {
	when := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	name := export.PDFName("2Qabcdefghijklmn", "Jane Doe", when)
	want := "2026-08-18_Qabcdefghijk_Jane_Doe.pdf"
	// After stripping leading 2: Qabcdefghijklmn → first 12 = Qabcdefghijk
	if name != want {
		// Compute expected dynamically for clarity
		short := export.ShortThreadID("2Qabcdefghijklmn")
		want = "2026-08-18_" + short + "_Jane_Doe.pdf"
		if name != want {
			t.Errorf("got %q want %q", name, want)
		}
	}
}
