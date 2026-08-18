package ui

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyErrorBrowser(t *testing.T) {
	msg := ClassifyError(errors.New("No supported browser found. Please install Microsoft Edge"))
	if msg.Title != "Could not find a browser" {
		t.Fatalf("title=%q", msg.Title)
	}
	if !strings.Contains(msg.Hint, "Install") {
		t.Fatalf("hint=%q", msg.Hint)
	}
}

func TestClassifyErrorLoginTimeout(t *testing.T) {
	msg := ClassifyError(errors.New("login timed out or was cancelled: context deadline exceeded"))
	if msg.Title != "Login timed out" {
		t.Fatalf("title=%q", msg.Title)
	}
}

func TestClassifyErrorConversationList(t *testing.T) {
	msg := ClassifyError(errors.New("conversation list: Conversation list not found."))
	if msg.Title != "Could not load the inbox" {
		t.Fatalf("title=%q", msg.Title)
	}
}

func TestClassifyErrorSessionChallenge(t *testing.T) {
	msg := ClassifyError(errors.New("linkedin session challenge or auth wall: https://www.linkedin.com/checkpoint/challenge/"))
	if msg.Title != "LinkedIn asked you to verify again" {
		t.Fatalf("title=%q", msg.Title)
	}
	if !strings.Contains(msg.Hint, "linkedin_cookies.json") {
		t.Fatalf("hint=%q", msg.Hint)
	}
}

func TestClassifyErrorAuthwall(t *testing.T) {
	msg := ClassifyError(errors.New("session challenge: https://www.linkedin.com/authwall"))
	if msg.Title != "LinkedIn asked you to verify again" {
		t.Fatalf("title=%q", msg.Title)
	}
}
