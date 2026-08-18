package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/pdfhtml"
)

const maxImageBytes = 8 << 20 // 8 MiB

// embedThreadImages turns photo URLs into JPEG data URLs so the PDF (printed
// from about:blank) can render them. LinkedIn CDN URLs are signed, so a
// normal HTTP GET works; Chrome's resource cache is only a fallback.
func (s *Session) embedThreadImages(data *pdfhtml.Thread) {
	if data == nil {
		return
	}
	cache := make(map[string]string)
	resolve := func(src string) string {
		src = strings.TrimSpace(src)
		if src == "" {
			return ""
		}
		if got, ok := cache[src]; ok {
			return got
		}
		got := s.imageToJPEGDataURL(src)
		cache[src] = got
		return got
	}
	data.Photo = resolve(data.Photo)
	for i := range data.Items {
		data.Items[i].SenderPhoto = resolve(data.Items[i].SenderPhoto)
		for j, img := range data.Items[i].Images {
			data.Items[i].Images[j] = resolve(img)
		}
	}
}

func (s *Session) imageToJPEGDataURL(src string) string {
	raw := decodeDataURL(src)
	if len(raw) == 0 && !strings.HasPrefix(src, "data:") {
		raw, _ = s.httpImageBytes(src)
		if len(raw) == 0 {
			raw, _ = s.cachedResourceBytes(src)
		}
	}
	if len(raw) < 24 {
		return ""
	}
	if sniffImageMIME(raw) == "image/jpeg" {
		return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
	}
	if d, err := s.rasterToJPEG(raw); err == nil && strings.HasPrefix(d, "data:image/jpeg") {
		return d
	}
	if mime := sniffImageMIME(raw); mime == "image/png" || mime == "image/gif" {
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
	}
	return ""
}

func (s *Session) httpImageBytes(imgURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(s.runCtx(), 15*time.Second)
	defer cancel()

	var cookies []*network.Cookie
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(ctx)
		return err
	}))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}
	if s.userAgent != "" {
		req.Header.Set("User-Agent", s.userAgent)
	}
	req.Header.Set("Referer", "https://www.linkedin.com/")
	req.Header.Set("Accept", "image/jpeg,image/png,image/webp,image/*;q=0.8")
	for _, c := range cookies {
		if c == nil {
			continue
		}
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxImageBytes {
		return nil, fmt.Errorf("image too large")
	}
	return raw, nil
}

func (s *Session) cachedResourceBytes(target string) ([]byte, error) {
	var raw []byte
	err := chromedp.Run(s.runCtx(), chromedp.ActionFunc(func(ctx context.Context) error {
		tree, err := page.GetResourceTree().Do(ctx)
		if err != nil {
			return err
		}
		frameID, resourceURL, ok := findImageResource(tree, target)
		if !ok {
			return fmt.Errorf("not in resource tree")
		}
		content, err := page.GetResourceContent(frameID, resourceURL).Do(ctx)
		if err != nil {
			return err
		}
		raw = content
		return nil
	}))
	return raw, err
}

func findImageResource(tree *page.FrameResourceTree, target string) (cdp.FrameID, string, bool) {
	if tree == nil {
		return "", "", false
	}
	want := resourceKey(target)
	var hitID cdp.FrameID
	var hitURL string
	var found bool
	var walk func(t *page.FrameResourceTree)
	walk = func(t *page.FrameResourceTree) {
		if t == nil || t.Frame == nil || found {
			return
		}
		for _, r := range t.Resources {
			if r == nil || r.Failed || r.Canceled || r.URL == "" {
				continue
			}
			if resourceKey(r.URL) != want && r.URL != target {
				continue
			}
			hitID, hitURL, found = t.Frame.ID, r.URL, true
			return
		}
		for _, child := range t.ChildFrames {
			walk(child)
		}
	}
	walk(tree)
	return hitID, hitURL, found
}

func resourceKey(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	if i := strings.IndexByte(u, '#'); i >= 0 {
		u = u[:i]
	}
	return u
}

// rasterToJPEG re-encodes webp/avif/png bytes as JPEG via Chrome (not the live <img>).
func (s *Session) rasterToJPEG(raw []byte) (string, error) {
	if s == nil || s.ctx == nil {
		return "", fmt.Errorf("no browser")
	}
	expr := fmt.Sprintf(`(async () => {
		const bin = Uint8Array.from(atob(%q), c => c.charCodeAt(0));
		const bmp = await createImageBitmap(new Blob([bin]));
		const max = 1600;
		const scale = Math.min(1, max / bmp.width, max / bmp.height);
		const c = document.createElement('canvas');
		c.width = Math.max(1, Math.round(bmp.width * scale));
		c.height = Math.max(1, Math.round(bmp.height * scale));
		const ctx = c.getContext('2d', { alpha: false });
		ctx.fillStyle = '#ffffff';
		ctx.fillRect(0, 0, c.width, c.height);
		ctx.drawImage(bmp, 0, 0, c.width, c.height);
		if (bmp.close) bmp.close();
		return c.toDataURL('image/jpeg', 0.9);
	})()`, base64.StdEncoding.EncodeToString(raw))

	out, err := s.evalJSONString(expr)
	if err != nil {
		return "", err
	}
	var dataURL string
	if err := json.Unmarshal(out, &dataURL); err != nil {
		dataURL = strings.Trim(string(out), `"`)
	}
	if !strings.HasPrefix(dataURL, "data:image/jpeg") {
		return "", fmt.Errorf("raster produced no jpeg")
	}
	return dataURL, nil
}

func decodeDataURL(src string) []byte {
	if !strings.HasPrefix(src, "data:") {
		return nil
	}
	comma := strings.IndexByte(src, ',')
	if comma < 0 {
		return nil
	}
	payload := src[comma+1:]
	if strings.Contains(strings.ToLower(src[:comma]), ";base64") {
		raw, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil
		}
		return raw
	}
	return []byte(payload)
}

func sniffImageMIME(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(raw, []byte{0x89, 0x50, 0x4e, 0x47}):
		return "image/png"
	case bytes.HasPrefix(raw, []byte("GIF87a")) || bytes.HasPrefix(raw, []byte("GIF89a")):
		return "image/gif"
	case len(raw) >= 12 && bytes.Equal(raw[:4], []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return ""
	}
}

// waitPrintImagesJS waits until every <img> in the print document has
// finished loading (or failed) so PrintToPDF captures decoded pixels.
const waitPrintImagesJS = `(async () => {
  const wait = (img) => {
    if (img.complete) {
      return img.decode ? img.decode().catch(() => {}) : Promise.resolve();
    }
    return new Promise((resolve) => {
      img.addEventListener('load', () => {
        if (img.decode) img.decode().then(resolve).catch(resolve);
        else resolve();
      }, { once: true });
      img.addEventListener('error', resolve, { once: true });
    });
  };
  const ready = (async () => {
    await Promise.all([...document.images].map(wait));
    if (document.fonts && document.fonts.ready) await document.fonts.ready;
  })();
  await Promise.race([ready, new Promise((r) => setTimeout(r, 8000))]);
  return true;
})()`
