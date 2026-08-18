package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/derkalle4/linkedin-inbox-downloader/internal/browser"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/config"
	applog "github.com/derkalle4/linkedin-inbox-downloader/internal/log"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/paths"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/state"
	"github.com/derkalle4/linkedin-inbox-downloader/internal/version"
)

// RunHeadless performs a one-shot backup of every conversation that is not
// currently backed up. Requires an existing accepted config.yaml and cookies.
func RunHeadless() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	applog.Info("starting headless run v%s", version.Version)

	cfgPath, err := paths.ConfigFile()
	if err != nil {
		applog.Error("resolve config path: %v", err)
		return err
	}
	cookiePath, err := paths.CookiesFile()
	if err != nil {
		applog.Error("resolve cookies path: %v", err)
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		applog.Error("load config: %v", err)
		return err
	}
	if cfg == nil {
		applog.Error("config.yaml not found at %s — run the interactive UI once to create it", cfgPath)
		return fmt.Errorf("config.yaml missing")
	}
	if !cfg.DisclaimerAccepted {
		applog.Error("disclaimer_accepted is false in %s — set it to true after accepting in the UI", cfgPath)
		return fmt.Errorf("disclaimer not accepted")
	}
	if cfg.DownloadPath == "" {
		cfg.DownloadPath = config.DefaultDownloadPath
	}

	dir, err := config.EnsureDownloadDir(cfg)
	if err != nil {
		applog.Error("download directory: %v", err)
		return err
	}
	applog.Info("config %s download_path=%s download_images=%v", cfgPath, dir, cfg.DownloadImages)
	applog.Info("cookies %s", cookiePath)

	st, err := state.Load(dir)
	if err != nil {
		applog.Error("load backup state: %v", err)
		return err
	}

	select {
	case <-ctx.Done():
		applog.Error("cancelled before browser start")
		return ctx.Err()
	default:
	}

	applog.Info("starting headless browser with saved cookies…")
	sess, err := ConnectWithCookies()
	if err != nil {
		applog.Error("%v", err)
		return err
	}
	defer sess.Close()
	applog.Info("inbox ready via %s", sess.Browser.DisplayName())

	applog.Info("loading conversation list…")
	convos, err := sess.ListConversations()
	if err != nil {
		applog.Error("list conversations: %v", err)
		return err
	}

	var todo []browser.Conversation
	skipped := 0
	for _, c := range convos {
		if st.IsBackedUp("", c.NameStr(), c.SnippetStr(), c.TimeStr()) {
			skipped++
			applog.Info("skip (already backed up) %q", c.NameStr())
			continue
		}
		todo = append(todo, c)
	}
	applog.Info("listed %d conversations, %d need backup, %d already current", len(convos), len(todo), skipped)

	if len(todo) == 0 {
		applog.Info("nothing to do — saved 0, skipped %d, failed 0", skipped)
		return nil
	}

	saved := 0
	var failed []browser.Conversation
	for i, c := range todo {
		select {
		case <-ctx.Done():
			applog.Error("cancelled during backup (%d/%d done)", saved, len(todo))
			return ctx.Err()
		default:
		}

		name := c.NameStr()
		applog.Info("backing up (%d/%d) %q…", i+1, len(todo), name)

		res := backupOne(sess, c, dir, st, cfg.DownloadImages, nil)
		if res.Err != nil {
			if errors.Is(res.Err, browser.ErrSessionChallenge) {
				applog.Error("session challenge during backup of %q: %v", name, res.Err)
				return res.Err
			}
			applog.Error("backup %q: %v", name, res.Err)
			failed = append(failed, c)
			browser.BetweenConversations()
			continue
		}
		saved++
		applog.Info("saved PDF %s", filepath.Base(res.OutPath))
		browser.BetweenConversations()
	}

	// Second pass over first-pass failures.
	if len(failed) > 0 {
		applog.Info("retrying %d failed conversation(s)…", len(failed))
		_ = sess.ResetInbox()
		retry := failed
		failed = nil
		for i, c := range retry {
			select {
			case <-ctx.Done():
				applog.Error("cancelled during retry (%d saved)", saved)
				return ctx.Err()
			default:
			}
			name := c.NameStr()
			applog.Info("retry (%d/%d) %q…", i+1, len(retry), name)
			res := backupOne(sess, c, dir, st, cfg.DownloadImages, nil)
			if res.Err != nil {
				if errors.Is(res.Err, browser.ErrSessionChallenge) {
					applog.Error("session challenge during retry of %q: %v", name, res.Err)
					return res.Err
				}
				applog.Error("retry %q: %v", name, res.Err)
				failed = append(failed, c)
				browser.BetweenConversations()
				continue
			}
			saved++
			applog.Info("saved PDF %s", filepath.Base(res.OutPath))
			browser.BetweenConversations()
		}
	}

	applog.Info("finished — saved %d, skipped %d, failed %d", saved, skipped, len(failed))
	if len(failed) > 0 {
		return fmt.Errorf("%d conversation backup(s) failed", len(failed))
	}
	return nil
}
