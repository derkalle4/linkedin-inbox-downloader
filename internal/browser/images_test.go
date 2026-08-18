package browser

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
)

func TestResourceKeyStripsQueryAndFragment(t *testing.T) {
	got := resourceKey("https://media.licdn.com/dms/image/abc?e=1&v=beta#x")
	want := "https://media.licdn.com/dms/image/abc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSniffImageMIME(t *testing.T) {
	if sniffImageMIME([]byte{0xff, 0xd8, 0xff, 0xe0}) != "image/jpeg" {
		t.Fatal("jpeg")
	}
	if sniffImageMIME([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d}) != "image/png" {
		t.Fatal("png")
	}
	if sniffImageMIME(append([]byte("GIF89a"), make([]byte, 20)...)) != "image/gif" {
		t.Fatal("gif")
	}
	webp := make([]byte, 16)
	copy(webp, "RIFF")
	copy(webp[8:], "WEBP")
	if sniffImageMIME(webp) != "image/webp" {
		t.Fatal("webp")
	}
	if sniffImageMIME([]byte("<html>not an image at all!!")) != "" {
		t.Fatal("html should not sniff as image")
	}
}

func TestDecodeDataURL(t *testing.T) {
	raw := bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0}, 8)
	src := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
	got := decodeDataURL(src)
	if !bytes.Equal(got, raw) {
		t.Fatal("roundtrip")
	}
	if decodeDataURL("https://example.com/x.jpg") != nil {
		t.Fatal("https is not a data URL")
	}
}

func TestFindImageResource(t *testing.T) {
	target := "https://media.licdn.com/dms/image/abc?e=99"
	tree := &page.FrameResourceTree{
		Frame: &cdp.Frame{ID: "frame-1"},
		Resources: []*page.FrameResource{
			{URL: "https://media.licdn.com/dms/image/abc?e=99", MimeType: "image/jpeg"},
		},
	}
	id, u, ok := findImageResource(tree, target)
	if !ok {
		t.Fatal("expected match")
	}
	if id != "frame-1" || u != target {
		t.Fatalf("id=%q u=%q", id, u)
	}
}

func TestImageToJPEGDataURLKeepsJPEG(t *testing.T) {
	jpeg := bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0}, 8)
	src := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)
	var s Session
	got := s.imageToJPEGDataURL(src)
	if !strings.HasPrefix(got, "data:image/jpeg;base64,") {
		t.Fatalf("got %q", got[:min(40, len(got))])
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, jpeg) {
		t.Fatal("roundtrip")
	}
}

func TestImageToJPEGDataURLFallsBackToPNG(t *testing.T) {
	png := append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{1}, 24)...)
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	var s Session
	got := s.imageToJPEGDataURL(src)
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("got %q", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
