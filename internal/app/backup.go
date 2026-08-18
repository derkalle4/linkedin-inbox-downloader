package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/browser"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/export"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/pdfhtml"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/state"
)

// backupResult is the outcome of backing up one conversation.
type backupResult struct {
	OutPath string
	Thread  *pdfhtml.Thread
	Err     error
}

// backupOne opens a conversation, exports a PDF, and records state.
// onProgress receives ExportProgress; PhaseStep 0 is "Opening conversation",
// then ExportOpenThread reports steps 1..4. Up to 3 attempts with an inbox
// reset between failures (not on session challenge errors).
func backupOne(
	sess *browser.Session,
	c browser.Conversation,
	dir string,
	st *state.State,
	downloadImages bool,
	onProgress func(browser.ExportProgress),
) backupResult {
	report := func(p browser.ExportProgress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	const totalSteps = 1 + browser.ExportPhaseCount // open + 4 export phases
	const maxAttempts = 3

	tryOnce := func() backupResult {
		if err := sess.CheckSessionHealthy(); err != nil {
			return backupResult{Err: err}
		}

		report(browser.ExportProgress{
			Phase:      "Opening conversation",
			PhaseStep:  0,
			PhaseCount: totalSteps,
		})

		ok, err := sess.OpenConversation(c)
		if err != nil {
			return backupResult{Err: err}
		}
		if !ok {
			return backupResult{Err: fmt.Errorf("could not open conversation")}
		}

		outPath, data, err := sess.ExportOpenThread(dir, downloadImages, func(p browser.ExportProgress) {
			p.PhaseCount = totalSteps
			report(p)
		})
		if err != nil {
			return backupResult{Err: err}
		}

		tid := browser.ThreadIDFromURL(data.URL)
		if tid == "" {
			tid = export.ShortThreadID(c.NameStr())
		}
		st.Record(tid, c.NameStr(), c.SnippetStr(), c.TimeStr())
		if err := state.Save(dir, st); err != nil {
			return backupResult{OutPath: outPath, Thread: data, Err: fmt.Errorf("save backup state: %w", err)}
		}
		return backupResult{OutPath: outPath, Thread: data}
	}

	var res backupResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res = tryOnce()
		if res.Err == nil {
			return res
		}
		if errors.Is(res.Err, browser.ErrSessionChallenge) {
			return res
		}
		if attempt == maxAttempts {
			break
		}
		_ = sess.ResetInbox()
		browser.HumanPause(2*time.Second, 4*time.Second)
	}
	return res
}
