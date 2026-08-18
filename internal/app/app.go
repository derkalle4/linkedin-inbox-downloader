package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/browser"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/config"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/paths"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/state"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/ui"
)

// Run is the main entry for the TUI application.
func Run() error {
	accepted, cfg, err := config.Accepted()
	if err != nil {
		return err
	}

	downloadPath := config.DefaultDownloadPath
	if cfg != nil && cfg.DownloadPath != "" {
		downloadPath = cfg.DownloadPath
	}
	absDownload, err := paths.ResolveDownloadPath(downloadPath)
	if err != nil {
		absDownload = downloadPath
	}

	m := ui.New(!accepted, absDownload)
	p := tea.NewProgram(m, tea.WithAltScreen())

	runner := &Runner{
		program:  p,
		cfg:      cfg,
		accepted: accepted,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runner.loop(ctx)

	final, err := p.Run()
	runner.cleanup()
	if err != nil {
		return err
	}
	if fm, ok := final.(ui.Model); ok && fm.DeclineRequested {
		_ = config.WipeSession()
	}
	return nil
}

// Runner coordinates browser work with the TUI.
type Runner struct {
	program  *tea.Program
	cfg      *config.Config
	accepted bool
	session  *browser.Session
	download string
	st       *state.State
	// convos is the last fetched inbox list; only refreshed on explicit reload.
	convos []browser.Conversation
}

func (r *Runner) cleanup() {
	if r.session != nil {
		r.session.Close()
		r.session = nil
	}
}

func (r *Runner) fatal(err error) {
	r.program.Send(ui.ClassifyError(err))
}

func (r *Runner) progress(msg ui.LoadProgressMsg) {
	r.program.Send(msg)
}

func (r *Runner) loop(ctx context.Context) {
	// Wait until disclaimer is accepted (or already was).
	if !r.accepted {
		for {
			select {
			case <-ctx.Done():
				_ = config.WipeSession()
				r.program.Quit()
				return
			case msg := <-ui.ActionBus:
				switch msg.(type) {
				case ui.AcceptedMsg:
					cfg, err := config.AcceptAndSave()
					if err != nil {
						r.fatal(err)
						return
					}
					r.cfg = cfg
					r.accepted = true
				case ui.DeclinedMsg:
					_ = config.WipeSession()
					return
				default:
					continue
				}
			}
			if r.accepted {
				break
			}
		}
	} else {
		// Wait for ReadyMsg from Init when skipping disclaimer.
		select {
		case <-ctx.Done():
			return
		case msg := <-ui.ActionBus:
			if _, ok := msg.(ui.DeclinedMsg); ok {
				return
			}
			_ = msg
		case <-time.After(500 * time.Millisecond):
			// proceed even if ReadyMsg was missed
		}
	}

	if r.cfg == nil {
		r.cfg = &config.Config{DisclaimerAccepted: true, DownloadPath: config.DefaultDownloadPath}
	}

	r.progress(ui.LoadProgressMsg{
		Title:   "Starting",
		Status:  "Preparing download folder…",
		Percent: 5,
	})

	dir, err := config.EnsureDownloadDir(r.cfg)
	if err != nil {
		r.fatal(fmt.Errorf("download folder: %w", err))
		return
	}
	r.download = dir
	r.program.Send(ui.PathSavedMsg{Path: dir})

	st, err := state.Load(dir)
	if err != nil {
		r.fatal(fmt.Errorf("backup state: %w", err))
		return
	}
	r.st = st

	if err := r.connect(ctx); err != nil {
		r.fatal(err)
		return
	}

	r.loadList()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ui.ActionBus:
			switch m := msg.(type) {
			case ui.ReloadMsg:
				r.loadList()
			case ui.BackupRequestMsg:
				if r.runBackup(ctx, m.Convos) {
					return
				}
			case ui.PathEditSubmitMsg:
				r.changePath(m.Path)
			case ui.DeclinedMsg:
				_ = config.WipeSession()
				return
			}
		}
	}
}

// ErrCookiesInvalid means saved cookies are missing or no longer open the inbox.
var ErrCookiesInvalid = fmt.Errorf("saved cookies missing or expired")

// ConnectWithCookies starts a headless browser using saved cookies and opens messaging.
// Returns ErrCookiesInvalid when cookies are missing or the inbox never becomes ready.
// On success the session has refreshed cookies via DumpCookies.
func ConnectWithCookies() (*browser.Session, error) {
	cookies, err := browser.LoadCookies()
	if err != nil {
		return nil, err
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("%w: linkedin_cookies.json missing or empty", ErrCookiesInvalid)
	}

	sess, err := browser.Start(browser.Options{Headless: true, Cookies: cookies})
	if err != nil {
		return nil, err
	}
	if err := sess.NavigateMessaging(); err != nil {
		sess.Close()
		return nil, err
	}
	if err := sess.WaitForInbox(20 * time.Second); err != nil {
		sess.Close()
		return nil, fmt.Errorf("%w: did not open the inbox", ErrCookiesInvalid)
	}
	if err := sess.CheckSessionHealthy(); err != nil {
		sess.Close()
		return nil, fmt.Errorf("%w: %v", ErrCookiesInvalid, err)
	}
	_ = sess.DumpCookies()
	return sess, nil
}

func (r *Runner) connect(ctx context.Context) error {
	r.progress(ui.LoadProgressMsg{
		Title:   "Loading session",
		Status:  "Reading saved cookies…",
		Percent: 15,
	})

	cookies, err := browser.LoadCookies()
	if err != nil {
		return fmt.Errorf("cookie file: %w", err)
	}

	needLogin := len(cookies) == 0
	if !needLogin {
		r.progress(ui.LoadProgressMsg{
			Title:   "Checking login",
			Status:  "Using saved cookies…",
			Percent: 30,
		})
		sess, err := ConnectWithCookies()
		if err == nil {
			r.progress(ui.LoadProgressMsg{
				Title:   "Connected",
				Status:  "Inbox ready",
				Percent: 65,
			})
			r.session = sess
			return nil
		}
		if !errors.Is(err, ErrCookiesInvalid) {
			return err
		}
		r.progress(ui.LoadProgressMsg{
			Title:   "Session expired",
			Status:  "Saved login no longer works — opening browser…",
			Percent: 35,
		})
	} else {
		r.progress(ui.LoadProgressMsg{
			Title:   "No saved session",
			Status:  "Opening browser for login…",
			Percent: 25,
		})
	}

	r.progress(ui.LoadProgressMsg{
		Title:   "Starting browser",
		Status:  "Opening browser for login…",
		Percent: 40,
	})
	sess, err := browser.Start(browser.Options{Headless: false})
	if err != nil {
		return err
	}
	browserName := sess.Browser.DisplayName()
	r.progress(ui.LoadProgressMsg{
		Title:       "Waiting for LinkedIn login",
		Status:      fmt.Sprintf("Log in to LinkedIn in the %s window, then wait…", browserName),
		Percent:     50,
		LoginWait:   true,
		BrowserName: browserName,
	})
	if err := sess.NavigateMessaging(); err != nil {
		sess.Close()
		return fmt.Errorf("navigate messaging: %w", err)
	}
	loginCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	err = sess.WaitUntilInbox(loginCtx, 2*time.Second)
	cancel()
	if err != nil {
		sess.Close()
		return fmt.Errorf("login timed out or was cancelled: %w", err)
	}
	if err := sess.DumpCookies(); err != nil {
		sess.Close()
		return fmt.Errorf("cookie file: %w", err)
	}
	sess.Close()

	r.progress(ui.LoadProgressMsg{
		Title:   "Login OK",
		Status:  "Starting headless session…",
		Percent: 60,
	})
	sess, err = ConnectWithCookies()
	if err != nil {
		return err
	}
	r.progress(ui.LoadProgressMsg{
		Title:   "Connected",
		Status:  "Inbox ready",
		Percent: 65,
	})
	r.session = sess
	return nil
}

func (r *Runner) loadList() {
	r.progress(ui.LoadProgressMsg{
		Title:            "Loading conversations",
		Status:           "Scanning LinkedIn inbox…",
		Percent:          70,
		SubLabel:         "Conversations found",
		SubIndeterminate: true,
	})
	convos, err := r.session.ListConversations(func(p browser.ListProgress) {
		pct := 70
		if p.MaxRounds > 0 && p.Round > 0 {
			pct = 70 + (p.Round*25)/p.MaxRounds
			if pct > 95 {
				pct = 95
			}
		}
		r.progress(ui.LoadProgressMsg{
			Title:            "Loading conversations",
			Status:           "Scanning LinkedIn inbox…",
			Percent:          pct,
			SubLabel:         "Conversations found",
			SubCurrent:       p.Found,
			SubIndeterminate: true,
		})
	})
	if err != nil {
		r.fatal(fmt.Errorf("conversation list: %w", err))
		return
	}
	r.convos = convos
	r.progress(ui.LoadProgressMsg{
		Title:      "Loading conversations",
		Status:     "Finishing…",
		Percent:    100,
		SubLabel:   "Conversations found",
		SubCurrent: len(convos),
		SubTotal:   len(convos),
	})
	r.publishCachedList()
}

// publishCachedList rebuilds inbox rows from the cached conversation list and
// current backup state — no LinkedIn fetch.
func (r *Runner) publishCachedList() {
	rows := make([]ui.ConvoRow, len(r.convos))
	for i, c := range r.convos {
		rows[i] = ui.ConvoRow{
			Convo:    c,
			BackedUp: r.st.IsBackedUp("", c.NameStr(), c.SnippetStr(), c.TimeStr()),
		}
	}
	r.program.Send(ui.ConvListMsg{Rows: rows})
}

func (r *Runner) changePath(path string) {
	if path == "" {
		path = config.DefaultDownloadPath
	}
	r.cfg.DownloadPath = path
	if err := config.Save(r.cfg); err != nil {
		r.fatal(err)
		return
	}
	dir, err := config.EnsureDownloadDir(r.cfg)
	if err != nil {
		r.fatal(fmt.Errorf("download folder: %w", err))
		return
	}
	r.download = dir
	st, err := state.Load(dir)
	if err != nil {
		r.fatal(fmt.Errorf("backup state: %w", err))
		return
	}
	r.st = st
	r.program.Send(ui.PathSavedMsg{Path: dir})
	if len(r.convos) == 0 {
		r.loadList()
		return
	}
	// Recompute backed-up flags against the new folder; keep cached inbox rows.
	r.publishCachedList()
}

// runBackup exports conversations. Returns true when the app should exit
// (parent context cancelled or disclaimer declined). "q" during backup
// cancels the job and returns to the inbox.
func (r *Runner) runBackup(ctx context.Context, convos []browser.Conversation) bool {
	r.program.Send(ui.BackupStartMsg{Total: len(convos)})
	backupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		restore := r.session.SetOpContext(backupCtx)
		defer restore()
		r.BackupConversations(backupCtx, convos)
	}()
	for {
		select {
		case <-done:
			if backupCtx.Err() != nil && ctx.Err() == nil {
				_ = r.session.ResetInbox()
			}
			return false
		case <-ctx.Done():
			cancel()
			<-done
			return true
		case msg := <-ui.ActionBus:
			switch msg.(type) {
			case ui.BackupCancelMsg:
				cancel()
			case ui.DeclinedMsg:
				cancel()
				<-done
				_ = config.WipeSession()
				return true
			}
		}
	}
}

// BackupConversations exports the given conversation rows.
func (r *Runner) BackupConversations(ctx context.Context, rows []browser.Conversation) {
	total := len(rows)
	r.program.Send(ui.BackupStartMsg{Total: total})
	saved := 0
	var failed []browser.Conversation

	send := func(done int, name, sub, subDetail string, subCur, subTot int, indeterminate bool, phaseFrac float64, errMsg string) {
		pct := 0
		if total > 0 {
			pct = int((float64(done) + phaseFrac) * 100 / float64(total))
			if pct > 100 {
				pct = 100
			}
		}
		msg := ui.BackupProgressMsg{
			Done:             done,
			Total:            total,
			Current:          name,
			SubLabel:         sub,
			SubDetail:        subDetail,
			SubCurrent:       subCur,
			SubTotal:         subTot,
			SubIndeterminate: indeterminate,
			Percent:          pct,
			Err:              errMsg,
		}
		r.program.Send(msg)
	}

	const threadPhases = 5 // open + load + extract + build + save
	runOne := func(c browser.Conversation, progressDone int) bool {
		name := c.NameStr()
		send(progressDone, name, "Opening conversation", "", 1, threadPhases, false, 0, "")

		res := backupOne(r.session, c, r.download, r.st, r.cfg != nil && r.cfg.DownloadImages, func(p browser.ExportProgress) {
			step := p.PhaseStep + 1 // 0 (open) → 1; 1..4 → 2..5
			if step < 1 {
				step = 1
			}
			if step > threadPhases {
				step = threadPhases
			}
			phaseFrac := float64(step) / float64(threadPhases)
			if p.Phase == "Loading messages" && p.MsgMaxRounds > 0 && p.MsgRound > 0 {
				loadFrac := float64(p.MsgRound) / float64(p.MsgMaxRounds)
				phaseFrac = (1.0 + loadFrac) / float64(threadPhases)
			}
			if p.MsgCount > 0 {
				send(progressDone, name, p.Phase, "", p.MsgCount, 0, true, phaseFrac, "")
				return
			}
			send(progressDone, name, p.Phase, "", step, threadPhases, false, phaseFrac, "")
		})
		if res.Err != nil {
			if ctx.Err() != nil || errors.Is(res.Err, context.Canceled) {
				return false
			}
			if errors.Is(res.Err, browser.ErrSessionChallenge) {
				r.fatal(res.Err)
				return false // abort
			}
			send(progressDone, name, "Failed", "", 0, threadPhases, false, 0, res.Err.Error())
			browser.BetweenConversations()
			failed = append(failed, c)
			return true
		}
		saved++
		send(progressDone+1, name, "Saving PDF", filepath.Base(res.OutPath), threadPhases, threadPhases, false, 1, "")
		browser.BetweenConversations()
		return true
	}

	finishCancel := func() {
		r.publishCachedList()
		r.program.Send(ui.BackupDoneMsg{Flash: fmt.Sprintf("Cancelled — saved %d PDF(s)", saved)})
	}

	for i, c := range rows {
		select {
		case <-ctx.Done():
			finishCancel()
			return
		default:
		}
		if !runOne(c, i) {
			if ctx.Err() != nil {
				finishCancel()
				return
			}
			r.publishCachedList()
			return
		}
		if ctx.Err() != nil {
			finishCancel()
			return
		}
	}

	// Second pass: retry conversations that failed in the first pass.
	if len(failed) > 0 {
		if ctx.Err() != nil {
			finishCancel()
			return
		}
		_ = r.session.ResetInbox()
		retry := failed
		failed = nil
		for i, c := range retry {
			select {
			case <-ctx.Done():
				finishCancel()
				return
			default:
			}
			doneBase := total - len(retry) + i
			if doneBase < saved {
				doneBase = saved
			}
			if !runOne(c, doneBase) {
				if ctx.Err() != nil {
					finishCancel()
					return
				}
				r.publishCachedList()
				return
			}
			if ctx.Err() != nil {
				finishCancel()
				return
			}
		}
	}

	flash := fmt.Sprintf("Saved %d PDF(s)", saved)
	if n := len(failed); n > 0 {
		flash = fmt.Sprintf("Saved %d PDF(s), %d failed", saved, n)
	}

	// Update backed-up flags from local state — do not re-fetch LinkedIn.
	r.publishCachedList()
	r.program.Send(ui.BackupDoneMsg{Flash: flash})
}
