package pdfhtml

import (
	"strings"
	"testing"
	"time"
)

func testNow() time.Time {
	return time.Date(2026, 8, 18, 19, 16, 0, 0, time.FixedZone("CEST", 2*3600)) // Tuesday
}

func TestStartedAt(t *testing.T) {
	now := testNow()
	loc := now.Location()
	tests := []struct {
		name  string
		items []Item
		want  time.Time
	}{
		{"weekday", []Item{{Type: "msg", Heading: "MITTWOCH", Time: "09:02"}}, time.Date(2026, 8, 12, 9, 2, 0, 0, loc)},
		{"day then msg", []Item{{Type: "day", Heading: "Mittwoch"}, {Type: "msg", Time: "09:02"}}, time.Date(2026, 8, 12, 9, 2, 0, 0, loc)},
		{"heute", []Item{{Type: "msg", Heading: "Heute", Time: "18:51"}}, time.Date(2026, 8, 18, 18, 51, 0, 0, loc)},
		{"gestern", []Item{{Type: "msg", Heading: "GESTERN", Time: "09:02"}}, time.Date(2026, 8, 17, 9, 2, 0, 0, loc)},
		{"de date", []Item{{Type: "msg", Heading: "15. Aug. 2024", Time: "09:02"}}, time.Date(2024, 8, 15, 9, 2, 0, 0, loc)},
		{"en am", []Item{{Type: "msg", Heading: "Aug 15, 2024", Time: "9:02 AM"}}, time.Date(2024, 8, 15, 9, 2, 0, 0, loc)},
		{"same year", []Item{{Type: "msg", Heading: "15. Aug.", Time: "09:02"}}, time.Date(2026, 8, 15, 9, 2, 0, 0, loc)},
		{"prev year", []Item{{Type: "msg", Heading: "15. Dez.", Time: "09:02"}}, time.Date(2025, 12, 15, 9, 2, 0, 0, loc)},
		{"numeric", []Item{{Type: "msg", Heading: "13.08.2023", Time: "21:15"}}, time.Date(2023, 8, 13, 21, 15, 0, 0, loc)},
		{"first msg", []Item{
			{Type: "msg", Heading: "15. Aug. 2024", Time: "09:02"},
			{Type: "msg", Heading: "Heute", Time: "18:51"},
		}, time.Date(2024, 8, 15, 9, 2, 0, 0, loc)},
		{"empty", nil, now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (Thread{Items: tt.items}).StartedAt(now)
			if !got.Equal(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestBuildUsesConversationDate(t *testing.T) {
	html := Build(Thread{
		Name:  "Ada",
		Items: []Item{{Type: "msg", Heading: "15. Aug. 2024", Time: "09:02", Text: "hi"}},
	}, "abc")
	if !strings.Contains(html, "15 Aug 2024 · 09:02") {
		t.Fatal("HTML missing conversation start")
	}
}

func TestBuildAvatarUsesPhotoNotInitials(t *testing.T) {
	html := Build(Thread{
		Name:  "Ada Lovelace",
		Photo: "data:image/jpeg;base64,abc",
		Items: []Item{{Type: "msg", Sender: "Ada Lovelace", SenderPhoto: "data:image/jpeg;base64,abc", Text: "hi"}},
	}, "abc")
	if strings.Contains(html, "AL") && strings.Contains(html, "fallback") {
		t.Fatal("expected photo, not initials fallback")
	}
	if !strings.Contains(html, `class='avatar' style='background-image:url("data:image/jpeg;base64,abc")'`) {
		t.Fatal("header avatar missing")
	}
	if !strings.Contains(html, `class='sender-pic' style='background-image:url("data:image/jpeg;base64,abc")'`) {
		t.Fatal("message avatar missing")
	}
}

func TestBuildAvatarFallsBackToInitials(t *testing.T) {
	html := Build(Thread{
		Name:  "Ada Lovelace",
		Items: []Item{{Type: "msg", Sender: "Ada Lovelace", Text: "hi"}},
	}, "abc")
	if !strings.Contains(html, "fallback") {
		t.Fatal("expected initials fallback when photo is missing")
	}
	if !strings.Contains(html, "AL") {
		t.Fatal("expected initials AL")
	}
}
