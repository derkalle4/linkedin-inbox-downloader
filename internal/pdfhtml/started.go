package pdfhtml

import (
	"strings"
	"time"
	"unicode"
)

var (
	weekdays = map[string]time.Weekday{
		"sunday": time.Sunday, "sonntag": time.Sunday,
		"monday": time.Monday, "montag": time.Monday,
		"tuesday": time.Tuesday, "dienstag": time.Tuesday,
		"wednesday": time.Wednesday, "mittwoch": time.Wednesday,
		"thursday": time.Thursday, "donnerstag": time.Thursday,
		"friday": time.Friday, "freitag": time.Friday,
		"saturday": time.Saturday, "samstag": time.Saturday, "sonnabend": time.Saturday,
	}
	deMonths = strings.NewReplacer(
		"januar", "january", "februar", "february", "maerz", "march", "marz", "march",
		"oktober", "october", "dezember", "december", "juni", "june", "juli", "july",
		"mai", "may", "okt", "oct", "dez", "dec", "mrz", "mar",
	)
	umlauts = strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "ß", "ss")
	dateLayouts = []string{
		"2 January 2006", "2 Jan 2006", "January 2 2006", "Jan 2 2006", "2 1 2006",
		"2 January", "2 Jan", "January 2", "Jan 2", "2 1",
	}
	clockLayouts = []string{"15:04", "3:04 PM"}
)

// StartedAt is the first message time (conversation start). Falls back to now.
func (t Thread) StartedAt(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	heading := ""
	for _, item := range t.Items {
		if h := strings.TrimSpace(item.Heading); h != "" {
			heading = h
		}
		if item.Type != "msg" {
			continue
		}
		if at := parseMessageTime(heading, item.Time, now); !at.IsZero() {
			return at
		}
		break
	}
	return now
}

func parseMessageTime(heading, clock string, now time.Time) time.Time {
	day := parseHeadingDate(heading, now)
	if day.IsZero() {
		return time.Time{}
	}
	clock = strings.ToUpper(strings.Join(strings.Fields(clock), " "))
	for _, layout := range clockLayouts {
		t, err := time.Parse(layout, clock)
		if err != nil {
			continue
		}
		return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
	}
	return day
}

func parseHeadingDate(heading string, now time.Time) time.Time {
	key := normalizeHeading(heading)
	if key == "" {
		return time.Time{}
	}
	switch key {
	case "today", "heute":
		return truncateDay(now)
	case "yesterday", "gestern":
		return truncateDay(now.AddDate(0, 0, -1))
	}
	if wd, ok := weekdays[key]; ok {
		delta := int(now.Weekday() - wd)
		if delta < 0 {
			delta += 7
		}
		return truncateDay(now.AddDate(0, 0, -delta))
	}

	key = deMonths.Replace(key)
	for _, layout := range dateLayouts {
		t, err := time.ParseInLocation(layout, key, now.Location())
		if err != nil {
			continue
		}
		if t.Year() < 1900 {
			t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
			if t.After(truncateDay(now)) {
				t = t.AddDate(-1, 0, 0)
			}
			return t
		}
		return truncateDay(t)
	}
	return time.Time{}
}

func truncateDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func normalizeHeading(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = umlauts.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
