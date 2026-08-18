package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/embedjs"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/export"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/pdfhtml"
)

const MessagingURL = "https://www.linkedin.com/messaging/"

var (
	threadRE    = regexp.MustCompile(`(?i)/messaging/thread/([^/?#]+)`)
	challengeRE = regexp.MustCompile(`(?i)/(checkpoint|challenge|authwall)(/|$|\?)`)
)

// Conversation is one inbox row.
type Conversation struct {
	Name    SoftString `json:"name"`
	Time    SoftString `json:"time"`
	Snippet SoftString `json:"snippet"`
	Photo   SoftString `json:"photo"`
	Key     SoftString `json:"key"`
	Href    SoftString `json:"href"`
}

// Display helpers used by the UI / open matching.
func (c Conversation) NameStr() string    { return c.Name.String() }
func (c Conversation) TimeStr() string    { return c.Time.String() }
func (c Conversation) SnippetStr() string { return c.Snippet.String() }
func (c Conversation) HrefStr() string    { return c.Href.String() }

// Session owns a chromedp browser instance and a temporary profile directory.
type Session struct {
	Browser     *Found
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
	opMu        sync.Mutex
	opCtx       context.Context // optional child of ctx; used to abort an in-flight backup
	profile     string
	headless    bool
	userAgent   string
}

// SetOpContext binds a cancellable child of the browser context for in-flight
// work (backup). The returned restore function clears it. parent is the
// caller's cancel signal (not a chromedp context); cancelling it aborts page
// operations without closing the browser.
func (s *Session) SetOpContext(parent context.Context) func() {
	if s == nil {
		return func() {}
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	stop := context.AfterFunc(parent, cancel)
	s.opMu.Lock()
	prev := s.opCtx
	s.opCtx = ctx
	s.opMu.Unlock()
	return func() {
		stop()
		cancel()
		s.opMu.Lock()
		s.opCtx = prev
		s.opMu.Unlock()
	}
}

func (s *Session) runCtx() context.Context {
	if s == nil {
		return context.Background()
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.opCtx != nil {
		return s.opCtx
	}
	return s.ctx
}

// Options configures a browser session.
type Options struct {
	Headless bool
	Cookies  []Cookie
}

// Start launches the system browser into a fresh temp profile.
func Start(opts Options) (*Session, error) {
	found, err := Find()
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "linkedin-inbox-profile-*")
	if err != nil {
		return nil, err
	}

	version := productVersion(found.Path)
	ua := headedUserAgent(found, version)

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(found.Path),
		chromedp.UserDataDir(profile),
		chromedp.UserAgent(ua),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("excludeSwitches", "enable-automation"),
		chromedp.WindowSize(1400, 900),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)
	if opts.Headless {
		// New headless keeps normal Chrome/Edge Client Hints (not HeadlessChrome).
		allocOpts = append(allocOpts, chromedp.Flag("headless", "new"))
	} else {
		allocOpts = append(allocOpts, chromedp.Flag("headless", false))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	s := &Session{
		Browser:     found,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         ctx,
		cancel:      cancel,
		profile:     profile,
		headless:    opts.Headless,
		userAgent:   ua,
	}

	if err := chromedp.Run(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("start browser: %w", err)
	}

	if err := s.applyStealth(); err != nil {
		s.Close()
		return nil, fmt.Errorf("stealth: %w", err)
	}

	if len(opts.Cookies) > 0 {
		if err := s.applyCookies(opts.Cookies); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Session) applyStealth() error {
	ua := stripHeadlessChrome(s.userAgent)
	return chromedp.Run(s.runCtx(), chromedp.ActionFunc(func(ctx context.Context) error {
		if err := emulation.SetUserAgentOverride(ua).Do(ctx); err != nil {
			return err
		}
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthInitJS).Do(ctx)
		return err
	}))
}

// Close cancels the browser and deletes the temp profile.
func (s *Session) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.allocCancel != nil {
		s.allocCancel()
	}
	if s.profile != "" {
		_ = os.RemoveAll(s.profile)
		s.profile = ""
	}
}

// Context returns the chromedp context.
func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) applyCookies(cookies []Cookie) error {
	return chromedp.Run(s.runCtx(), chromedp.ActionFunc(func(ctx context.Context) error {
		for _, c := range FilterLive(cookies) {
			expr := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
			p := network.SetCookie(c.Name, c.Value).
				WithDomain(c.Domain).
				WithPath(c.Path).
				WithHTTPOnly(c.HTTPOnly).
				WithSecure(c.Secure)
			if c.Expires > 0 {
				p = p.WithExpires(&expr)
			}
			switch strings.ToLower(c.SameSite) {
			case "strict":
				p = p.WithSameSite(network.CookieSameSiteStrict)
			case "lax":
				p = p.WithSameSite(network.CookieSameSiteLax)
			case "none":
				p = p.WithSameSite(network.CookieSameSiteNone)
			}
			if err := p.Do(ctx); err != nil {
				return err
			}
		}
		return nil
	}))
}

// NavigateMessaging opens the LinkedIn messaging inbox.
func (s *Session) NavigateMessaging() error {
	return chromedp.Run(s.runCtx(), chromedp.Navigate(MessagingURL))
}

// WaitForInbox waits until the messaging UI appears (or timeout).
func (s *Session) WaitForInbox(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(s.runCtx(), timeout)
	defer cancel()
	sel := `.msg-conversations-container__conversations-list, .msg-thread, .msg-title-bar`
	return chromedp.Run(ctx, chromedp.WaitVisible(sel, chromedp.ByQuery))
}

// InboxReady quickly checks whether the inbox UI is already visible.
func (s *Session) InboxReady() bool {
	ctx, cancel := context.WithTimeout(s.runCtx(), 5*time.Second)
	defer cancel()
	var ok bool
	err := chromedp.Run(ctx, chromedp.Evaluate(`!!(
		document.querySelector('.msg-conversations-container__conversations-list') ||
		document.querySelector('.msg-thread') ||
		document.querySelector('.msg-title-bar')
	)`, &ok))
	return err == nil && ok
}

// ErrSessionChallenge means LinkedIn redirected to a login/challenge page.
var ErrSessionChallenge = fmt.Errorf("linkedin session challenge or auth wall")

// CheckSessionHealthy returns ErrSessionChallenge when the current URL looks like
// a checkpoint, challenge, or authwall page.
func (s *Session) CheckSessionHealthy() error {
	var pageURL string
	if err := chromedp.Run(s.runCtx(), chromedp.Location(&pageURL)); err != nil {
		return err
	}
	if challengeRE.MatchString(pageURL) {
		return fmt.Errorf("%w: %s", ErrSessionChallenge, pageURL)
	}
	return nil
}

// DumpCookies saves all cookies for linkedin.com domains.
func (s *Session) DumpCookies() error {
	var cookies []*network.Cookie
	err := chromedp.Run(s.runCtx(), chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(ctx)
		return err
	}))
	if err != nil {
		return err
	}
	out := make([]Cookie, 0, len(cookies))
	for _, c := range cookies {
		if !strings.Contains(c.Domain, "linkedin.com") {
			continue
		}
		out = append(out, Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  c.Expires,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: string(c.SameSite),
		})
	}
	return SaveCookies(out)
}

// ListProgress is reported while harvesting the inbox list.
type ListProgress struct {
	Found     int
	Round     int
	MaxRounds int
}

// LoadProgress is reported while scrolling a thread for older messages.
type LoadProgress struct {
	Count     int
	Round     int
	MaxRounds int
}

type lidJobStatus struct {
	Kind      string          `json:"kind"`
	Found     int             `json:"found"`
	Count     int             `json:"count"`
	Round     int             `json:"round"`
	MaxRounds int             `json:"maxRounds"`
	Done      bool            `json:"done"`
	Error     string          `json:"error"`
	Result    json.RawMessage `json:"result"`
}

const (
	listJobTimeout = 8 * time.Minute
	loadJobTimeout = 8 * time.Minute
)

// ListConversations harvests the conversation list via LIST_JS, optionally
// reporting progress while the in-page job runs.
func (s *Session) ListConversations(onProgress ...func(ListProgress)) ([]Conversation, error) {
	var progress func(ListProgress)
	if len(onProgress) > 0 {
		progress = onProgress[0]
	}
	if err := s.CheckSessionHealthy(); err != nil {
		return nil, err
	}
	if err := s.startLidJob(embedjs.ListJS); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(listJobTimeout)
	for {
		if err := s.runCtx().Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("list conversations: timed out after %s", listJobTimeout)
		}
		st, err := s.pollLidJob()
		if err != nil {
			return nil, err
		}
		if progress != nil {
			progress(ListProgress{Found: st.Found, Round: st.Round, MaxRounds: st.MaxRounds})
		}
		if !st.Done {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if st.Error != "" {
			return nil, fmt.Errorf("%s", st.Error)
		}
		raw := st.Result
		if len(raw) == 0 || string(raw) == "null" {
			return nil, nil
		}
		return decodeConversations(raw)
	}
}

func (s *Session) startLidJob(js string) error {
	var ok bool
	if err := chromedp.Run(s.runCtx(), chromedp.Evaluate(fmt.Sprintf("(%s)()", js), &ok)); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("page job already running")
	}
	return nil
}

func (s *Session) pollLidJob() (lidJobStatus, error) {
	raw, err := s.evalJSONString(`(() => {
		const j = window.__lidJob;
		if (!j) return { done: true, error: "no page job" };
		return {
			kind: j.kind || "",
			found: j.found || 0,
			count: j.count || 0,
			round: j.round || 0,
			maxRounds: j.maxRounds || 0,
			done: !!j.done,
			error: j.error || "",
			result: j.done ? j.result : null
		};
	})()`)
	if err != nil {
		return lidJobStatus{}, err
	}
	var st lidJobStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return lidJobStatus{}, fmt.Errorf("page job status: %w", err)
	}
	return st, nil
}

// LoadMessageHistory scrolls the open thread to load older messages.
func (s *Session) LoadMessageHistory(onProgress ...func(LoadProgress)) (int, error) {
	var progress func(LoadProgress)
	if len(onProgress) > 0 {
		progress = onProgress[0]
	}
	if err := s.startLidJob(embedjs.LoadJS); err != nil {
		return 0, err
	}
	deadline := time.Now().Add(loadJobTimeout)
	for {
		if err := s.runCtx().Err(); err != nil {
			return 0, err
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("load messages: timed out after %s", loadJobTimeout)
		}
		st, err := s.pollLidJob()
		if err != nil {
			return 0, err
		}
		if progress != nil {
			progress(LoadProgress{Count: st.Count, Round: st.Round, MaxRounds: st.MaxRounds})
		}
		if !st.Done {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if st.Error != "" {
			return 0, fmt.Errorf("%s", st.Error)
		}
		return st.Count, nil
	}
}

// ResetInbox navigates back to the messaging inbox and waits until it is ready.
// Used after failed opens/exports so the next attempt can click list rows again.
func (s *Session) ResetInbox() error {
	if err := s.CheckSessionHealthy(); err != nil {
		return err
	}
	if err := s.NavigateMessaging(); err != nil {
		return err
	}
	if err := s.WaitForInbox(20 * time.Second); err != nil {
		return err
	}
	HumanPause(400*time.Millisecond, 900*time.Millisecond)
	return s.CheckSessionHealthy()
}

// EnsureMessaging reloads the inbox when the current page is not LinkedIn messaging.
func (s *Session) EnsureMessaging() error {
	var pageURL string
	if err := chromedp.Run(s.runCtx(), chromedp.Location(&pageURL)); err != nil {
		return err
	}
	low := strings.ToLower(pageURL)
	if strings.Contains(low, "linkedin.com") && strings.Contains(low, "/messaging") {
		return nil
	}
	return s.ResetInbox()
}

// OpenConversation opens a conversation by thread URL when available, otherwise
// a bounded click-in-list fallback (no inbox re-pagination).
func (s *Session) OpenConversation(c Conversation) (bool, error) {
	if err := s.CheckSessionHealthy(); err != nil {
		return false, err
	}

	waitThread := func() error {
		HumanPause(800*time.Millisecond, 1500*time.Millisecond)
		ctx, cancel := context.WithTimeout(s.runCtx(), 30*time.Second)
		defer cancel()
		err := chromedp.Run(ctx,
			chromedp.WaitVisible(`.msg-thread .msg-s-message-list, .msg-s-event-listitem`, chromedp.ByQuery),
		)
		if err != nil {
			if challengeErr := s.CheckSessionHealthy(); challengeErr != nil {
				return challengeErr
			}
			return err
		}
		return nil
	}

	href := strings.TrimSpace(c.HrefStr())
	if href != "" {
		opened, err := s.openByHref(href)
		if err != nil {
			// Navigation failed — reset and try list click.
			if resetErr := s.ResetInbox(); resetErr != nil {
				return false, err
			}
			opened, err = s.openByListClick(c)
			if err != nil {
				return false, err
			}
			if !opened {
				return false, nil
			}
			if err := waitThread(); err != nil {
				return false, err
			}
			return true, nil
		}
		if opened {
			if err := waitThread(); err == nil {
				return true, nil
			} else if errors.Is(err, ErrSessionChallenge) {
				return false, err
			}
			// Thread did not paint — reset inbox and fall through to list click.
			if resetErr := s.ResetInbox(); resetErr != nil {
				return false, err
			}
		}
		// opened == false (href not a thread URL) or wait failed: list click.
	}

	opened, err := s.openByListClick(c)
	if err != nil {
		return false, err
	}
	if !opened {
		return false, nil
	}
	if err := waitThread(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Session) openByHref(href string) (bool, error) {
	u, err := url.Parse(href)
	if err != nil || u.Path == "" {
		return false, nil
	}
	// Prefer absolute LinkedIn messaging URLs.
	target := href
	if u.Host == "" {
		target = "https://www.linkedin.com" + href
	}
	if !strings.Contains(strings.ToLower(target), "/messaging/thread/") {
		return false, nil
	}
	if err := chromedp.Run(s.runCtx(), chromedp.Navigate(target)); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Session) openByListClick(c Conversation) (bool, error) {
	payload, err := json.Marshal(map[string]string{
		"name":    c.NameStr(),
		"time":    c.TimeStr(),
		"snippet": c.SnippetStr(),
		"href":    c.HrefStr(),
	})
	if err != nil {
		return false, err
	}
	js := fmt.Sprintf("(%s)(%s)", embedjs.OpenJS, string(payload))
	var ok bool
	err = chromedp.Run(s.runCtx(), chromedp.ActionFunc(func(ctx context.Context) error {
		res, exception, err := runtime.Evaluate(js).
			WithAwaitPromise(true).
			WithReturnByValue(true).
			Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return exception
		}
		if res == nil || res.Value == nil {
			ok = false
			return nil
		}
		return json.Unmarshal(res.Value, &ok)
	}))
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ExportProgress reports phases while backing up one open thread.
type ExportProgress struct {
	Phase        string // e.g. "Loading messages", "Saving PDF"
	PhaseStep    int    // 1-based step within this thread (open is step 0 from caller)
	PhaseCount   int    // total export steps for this thread (always 4)
	MsgCount     int    // messages loaded (load phase only)
	MsgRound     int
	MsgMaxRounds int
}

// ExportPhaseCount is the number of phases reported by ExportOpenThread.
const ExportPhaseCount = 4

// ExportOpenThread loads history, extracts messages, and writes a PDF.
// When downloadImages is true, message pictures are also written next to the
// PDF as {stem}_img_01.jpg (existing files are not overwritten).
func (s *Session) ExportOpenThread(downloadDir string, downloadImages bool, onProgress ...func(ExportProgress)) (string, *pdfhtml.Thread, error) {
	var progress func(ExportProgress)
	if len(onProgress) > 0 {
		progress = onProgress[0]
	}
	report := func(p ExportProgress) {
		if progress != nil {
			if p.PhaseCount == 0 {
				p.PhaseCount = ExportPhaseCount
			}
			progress(p)
		}
	}

	if err := s.CheckSessionHealthy(); err != nil {
		return "", nil, err
	}

	report(ExportProgress{Phase: "Loading messages", PhaseStep: 1, PhaseCount: ExportPhaseCount})
	if _, err := s.LoadMessageHistory(func(lp LoadProgress) {
		report(ExportProgress{
			Phase:        "Loading messages",
			PhaseStep:    1,
			PhaseCount:   ExportPhaseCount,
			MsgCount:     lp.Count,
			MsgRound:     lp.Round,
			MsgMaxRounds: lp.MaxRounds,
		})
	}); err != nil {
		return "", nil, fmt.Errorf("load messages: %w", err)
	}

	// Let GhostImage / lazy avatars settle after the scroll pass.
	HumanPause(500*time.Millisecond, 1200*time.Millisecond)

	report(ExportProgress{Phase: "Extracting messages", PhaseStep: 2, PhaseCount: ExportPhaseCount})
	raw, err := s.evalJSONString(fmt.Sprintf("(%s)()", embedjs.ExtractJS))
	if err != nil {
		return "", nil, fmt.Errorf("extract: %w", err)
	}
	data, err := decodeThread(raw)
	if err != nil {
		return "", nil, fmt.Errorf("extract decode: %w", err)
	}
	s.embedThreadImages(&data)

	hasMsg := false
	for _, i := range data.Items {
		if i.Type == "msg" {
			hasMsg = true
			break
		}
	}
	if !hasMsg {
		return "", nil, fmt.Errorf("no message content in this thread")
	}

	tid := ThreadIDFromURL(data.URL)
	if tid == "" {
		var pageURL string
		_ = chromedp.Run(s.runCtx(), chromedp.Location(&pageURL))
		tid = ThreadIDFromURL(pageURL)
	}
	person := data.Name
	if person == "" {
		person = "thread"
	}
	short := export.ShortThreadID(tid)

	report(ExportProgress{Phase: "Building PDF", PhaseStep: 3, PhaseCount: ExportPhaseCount})
	htmlDoc := pdfhtml.Build(data, short)

	outName := export.PDFName(tid, person, data.StartedAt(time.Now()))
	outPath := filepath.Join(downloadDir, outName)

	if downloadImages {
		if err := export.WriteSidecarJPEGs(downloadDir, outName, messageImages(data)); err != nil {
			return "", nil, fmt.Errorf("save images: %w", err)
		}
	}

	if _, err := export.RotateExisting(downloadDir, tid); err != nil {
		return "", nil, err
	}

	report(ExportProgress{Phase: "Saving PDF", PhaseStep: 4, PhaseCount: ExportPhaseCount})
	if err := s.printHTMLToPDF(htmlDoc, person, outPath); err != nil {
		return "", nil, err
	}
	// PDF print uses a separate tab; if the main page somehow left messaging, recover.
	_ = s.EnsureMessaging()
	return outPath, &data, nil
}

func (s *Session) printHTMLToPDF(htmlDoc, person, outPath string) error {
	// Use a separate tab so we do not leave the messaging inbox page.
	tabCtx, cancel := chromedp.NewContext(s.runCtx())
	defer cancel()

	footer := fmt.Sprintf(
		`<div style="font-size:9px;color:#7a8490;width:100%%;padding:0 14mm;font-family:DM Sans,Segoe UI,sans-serif;display:flex;justify-content:space-between;align-items:center;"><span>Conversation with %s</span><span><span class="pageNumber"></span> / <span class="totalPages"></span></span></div>`,
		html.EscapeString(person),
	)

	var pdfBuf []byte
	err := chromedp.Run(tabCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetResourceTree().Do(ctx)
			if err != nil {
				return err
			}
			if tree == nil || tree.Frame == nil {
				return fmt.Errorf("print tab: no frame")
			}
			return page.SetDocumentContent(tree.Frame.ID, htmlDoc).Do(ctx)
		}),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, exception, err := runtime.Evaluate(waitPrintImagesJS).
				WithAwaitPromise(true).
				WithReturnByValue(true).
				Do(ctx)
			if err != nil {
				return err
			}
			if exception != nil {
				return exception
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				WithScale(1).
				WithDisplayHeaderFooter(true).
				WithHeaderTemplate("<div></div>").
				WithFooterTemplate(footer).
				WithMarginTop(0).
				WithMarginBottom(0.55).
				WithMarginLeft(0).
				WithMarginRight(0).
				WithPaperWidth(8.27).
				WithPaperHeight(11.69).
				Do(ctx)
			if err != nil {
				return err
			}
			pdfBuf = buf
			return nil
		}),
	)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, pdfBuf, 0o644)
}

// ThreadIDFromURL extracts the LinkedIn messaging thread id from a URL.
func ThreadIDFromURL(u string) string {
	m := threadRE.FindStringSubmatch(u)
	if m == nil {
		return ""
	}
	id, err := url.PathUnescape(m[1])
	if err != nil {
		return strings.Trim(m[1], "/")
	}
	return strings.Trim(id, "/")
}

// WaitUntilInbox polls until the inbox appears (for visible login).
func (s *Session) WaitUntilInbox(ctx context.Context, pollEvery time.Duration) error {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		if s.InboxReady() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// evalJSONString runs an expression, JSON.stringify's the result in-page, and
// returns the raw JSON payload (array/object/null) as bytes.
func (s *Session) evalJSONString(expression string) ([]byte, error) {
	expr := fmt.Sprintf(`(async () => {
		const v = await (%s);
		return JSON.stringify(v === undefined ? null : v);
	})()`, expression)

	var encoded string
	err := chromedp.Run(s.runCtx(), chromedp.ActionFunc(func(ctx context.Context) error {
		res, exception, err := runtime.Evaluate(expr).
			WithAwaitPromise(true).
			WithReturnByValue(true).
			Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return exception
		}
		if res == nil || res.Value == nil {
			encoded = "null"
			return nil
		}
		// JS returned a string → CDP value is a JSON string; unwrap it.
		if err := json.Unmarshal(res.Value, &encoded); err != nil {
			// Already an object/array — treat Value as the payload.
			encoded = string(res.Value)
		}
		return nil
	}))
	if err != nil {
		return nil, err
	}
	if encoded == "" || encoded == "null" {
		return []byte("null"), nil
	}
	return []byte(encoded), nil
}

type wireThread struct {
	Name       SoftString `json:"name"`
	Headline   SoftString `json:"headline"`
	Degree     SoftString `json:"degree"`
	Photo      SoftString `json:"photo"`
	ProfileURL SoftString `json:"profileUrl"`
	URL        SoftString `json:"url"`
	Items      []wireItem `json:"items"`
}

type wireItem struct {
	Type        SoftString  `json:"type"`
	Heading     SoftString  `json:"heading"`
	Sender      SoftString  `json:"sender"`
	Time        SoftString  `json:"time"`
	Subject     SoftString  `json:"subject"`
	HTML        SoftString  `json:"html"`
	Text        SoftString  `json:"text"`
	Self        bool        `json:"self"`
	Images      SoftStrings `json:"images"`
	SenderPhoto SoftString  `json:"senderPhoto"`
}

func decodeThread(raw []byte) (pdfhtml.Thread, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return pdfhtml.Thread{}, fmt.Errorf("empty extract result")
	}
	var w wireThread
	if err := json.Unmarshal(raw, &w); err != nil {
		preview := string(raw)
		if len(preview) > 240 {
			preview = preview[:240] + "…"
		}
		return pdfhtml.Thread{}, fmt.Errorf("%w (payload: %s)", err, preview)
	}
	out := pdfhtml.Thread{
		Name:       w.Name.String(),
		Headline:   w.Headline.String(),
		Degree:     w.Degree.String(),
		Photo:      w.Photo.String(),
		ProfileURL: w.ProfileURL.String(),
		URL:        w.URL.String(),
	}
	for _, it := range w.Items {
		out.Items = append(out.Items, pdfhtml.Item{
			Type:        it.Type.String(),
			Heading:     it.Heading.String(),
			Sender:      it.Sender.String(),
			Time:        it.Time.String(),
			Subject:     it.Subject.String(),
			HTML:        it.HTML.String(),
			Text:        it.Text.String(),
			Self:        it.Self,
			Images:      []string(it.Images),
			SenderPhoto: it.SenderPhoto.String(),
		})
	}
	return out, nil
}

func messageImages(data pdfhtml.Thread) []string {
	var images []string
	for _, item := range data.Items {
		if item.Type != "msg" {
			continue
		}
		for _, src := range item.Images {
			if strings.TrimSpace(src) == "" {
				continue
			}
			images = append(images, src)
		}
	}
	return images
}

func decodeConversations(raw []byte) ([]Conversation, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		var convos []Conversation
		if err := json.Unmarshal(raw, &convos); err != nil {
			return nil, fmt.Errorf("conversation list: %w", err)
		}
		return convos, nil
	}
	// Some CDP paths serialize arrays as objects with numeric keys.
	var asMap map[string]Conversation
	if err := json.Unmarshal(raw, &asMap); err != nil {
		preview := string(raw)
		if len(preview) > 240 {
			preview = preview[:240] + "…"
		}
		return nil, fmt.Errorf("conversation list: expected array, got: %s", preview)
	}
	out := make([]Conversation, 0, len(asMap))
	for _, c := range asMap {
		out = append(out, c)
	}
	return out, nil
}
