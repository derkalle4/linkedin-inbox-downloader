package export

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/png"
)

// SidecarImageName returns {pdf-stem}_img_{XX}.jpg (XX is 1-based, two digits).
func SidecarImageName(pdfName string, n int) string {
	base := filepath.Base(pdfName)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return fmt.Sprintf("%s_img_%02d.jpg", stem, n)
}

// WriteSidecarJPEGs writes message images next to the PDF.
// Numbering always starts at 01 for the first image in conversation order so
// a later message's new picture becomes the next index. Existing files are
// left unchanged.
func WriteSidecarJPEGs(dir, pdfName string, images []string) error {
	n := 0
	for _, src := range images {
		if strings.TrimSpace(src) == "" {
			continue
		}
		n++
		path := filepath.Join(dir, SidecarImageName(pdfName, n))
		jpegBytes, err := jpegFromDataURL(src)
		if err != nil {
			// Leave a hole for this index; a later backup can fill it.
			continue
		}
		if err := writeNewFile(path, jpegBytes); err != nil {
			return err
		}
	}
	return nil
}

func writeNewFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

func jpegFromDataURL(src string) ([]byte, error) {
	src = strings.TrimSpace(src)
	if !strings.HasPrefix(src, "data:") {
		return nil, fmt.Errorf("not a data URL")
	}
	comma := strings.IndexByte(src, ',')
	if comma < 0 {
		return nil, fmt.Errorf("malformed data URL")
	}
	meta := strings.ToLower(src[len("data:"):comma])
	payload := src[comma+1:]
	isBase64 := strings.Contains(meta, ";base64")
	mime := meta
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		mime = meta[:i]
	}
	mime = strings.TrimSpace(mime)

	var raw []byte
	if isBase64 {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, err
		}
		raw = decoded
	} else {
		raw = []byte(payload)
	}

	if mime == "image/jpeg" || mime == "image/jpg" {
		return raw, nil
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	flat := flatten(img)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func flatten(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, &image.Uniform{C: image.White}, image.Point{}, draw.Src)
	draw.Draw(out, b, img, b.Min, draw.Over)
	return out
}
