package export_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/export"
)

func TestSidecarImageName(t *testing.T) {
	got := export.SidecarImageName("2026-08-18_Qabcdefghijk_Jane_Doe.pdf", 1)
	want := "2026-08-18_Qabcdefghijk_Jane_Doe_img_01.jpg"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := export.SidecarImageName("thread.pdf", 12); got != "thread_img_12.jpg" {
		t.Errorf("got %q", got)
	}
}

func TestWriteSidecarJPEGsNumberingAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	pdf := "2026-08-18_abc_Ada.pdf"
	first := mustJPEGDataURL(t, color.RGBA{R: 200, A: 255})
	second := mustJPEGDataURL(t, color.RGBA{G: 200, A: 255})
	third := mustPNGDataURL(t, color.RGBA{B: 200, A: 255})

	if err := export.WriteSidecarJPEGs(dir, pdf, []string{first, second}); err != nil {
		t.Fatal(err)
	}
	path01 := filepath.Join(dir, "2026-08-18_abc_Ada_img_01.jpg")
	path02 := filepath.Join(dir, "2026-08-18_abc_Ada_img_02.jpg")
	path03 := filepath.Join(dir, "2026-08-18_abc_Ada_img_03.jpg")

	orig01, err := os.ReadFile(path01)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path02); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path03); !os.IsNotExist(err) {
		t.Fatalf("img_03 should not exist yet: %v", err)
	}

	marker := []byte("keep-me")
	if err := os.WriteFile(path01, marker, 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-export with a new trailing image: keep 01/02, write 03.
	if err := export.WriteSidecarJPEGs(dir, pdf, []string{first, second, third}); err != nil {
		t.Fatal(err)
	}
	got01, err := os.ReadFile(path01)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got01, marker) {
		t.Fatalf("img_01 was overwritten")
	}
	if _, err := os.Stat(path03); err != nil {
		t.Fatalf("img_03 missing after new message: %v", err)
	}
	if bytes.Equal(orig01, marker) {
		t.Fatal("test setup failed: marker equals original jpeg")
	}
}

func mustJPEGDataURL(t *testing.T, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, c)
	img.Set(1, 0, c)
	img.Set(0, 1, c)
	img.Set(1, 1, c)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func mustPNGDataURL(t *testing.T, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, c)
	img.Set(1, 0, c)
	img.Set(0, 1, c)
	img.Set(1, 1, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
