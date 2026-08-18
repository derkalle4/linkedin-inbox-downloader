package pdfhtml

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// Thread is the extracted conversation data (mirrors EXTRACT_JS output).
type Thread struct {
	Name       string `json:"name"`
	Headline   string `json:"headline"`
	Degree     string `json:"degree"`
	Photo      string `json:"photo"`
	ProfileURL string `json:"profileUrl"`
	URL        string `json:"url"`
	Items      []Item `json:"items"`
}

// Item is a day heading or a message.
type Item struct {
	Type        string   `json:"type"`
	Heading     string   `json:"heading"`
	Sender      string   `json:"sender"`
	Time        string   `json:"time"`
	Subject     string   `json:"subject"`
	HTML        string   `json:"html"`
	Text        string   `json:"text"`
	Self        bool     `json:"self"`
	Images      []string `json:"images"`
	SenderPhoto string   `json:"senderPhoto"`
}

var (
	commentRE = regexp.MustCompile(`(?s)<!--.*?-->`)
	// RE2 has no backreferences — strip each dangerous tag family separately.
	dangerREs = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`),
		regexp.MustCompile(`(?is)<button\b[^>]*>.*?</button>`),
		regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg>`),
		regexp.MustCompile(`(?is)<input\b[^>]*/?>`),
	}
	onAttrRE   = regexp.MustCompile(`(?i)\son\w+="[^"]*"`)
	spaceSplit = regexp.MustCompile(`[\s,]+`)
)

func initials(name string) string {
	parts := spaceSplit.Split(name, -1)
	var letters []rune
	for _, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)[0]
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters = append(letters, r)
		}
	}
	if len(letters) == 0 {
		return "?"
	}
	out := string(letters[0])
	if len(letters) > 1 {
		out += string(letters[len(letters)-1])
	}
	return strings.ToUpper(out)
}

func sanitizeFragment(fragment string) string {
	fragment = commentRE.ReplaceAllString(fragment, "")
	for _, re := range dangerREs {
		fragment = re.ReplaceAllString(fragment, "")
	}
	fragment = onAttrRE.ReplaceAllString(fragment, "")
	return strings.TrimSpace(fragment)
}

func avatarHTML(src, label, class string) string {
	if src != "" {
		return fmt.Sprintf(`<img class='%s' src='%s' alt=''>`, class, html.EscapeString(src))
	}
	return fmt.Sprintf(`<div class='%s fallback'>%s</div>`, class, html.EscapeString(initials(label)))
}

// Build returns the full HTML document for a conversation PDF (same design as the Python exporter).
func Build(data Thread, shortThreadID string) string {
	name := data.Name
	if name == "" {
		name = "LinkedIn conversation"
	}
	nMsg := 0
	for _, i := range data.Items {
		if i.Type == "msg" {
			nMsg++
		}
	}
	exported := time.Now().Format("02 Jan 2006 · 15:04")

	var chips strings.Builder
	if deg := strings.TrimLeft(strings.TrimSpace(data.Degree), "·• \t"); deg != "" {
		chips.WriteString(fmt.Sprintf(`<span class='chip'>%s</span>`, html.EscapeString(deg)))
	}
	chips.WriteString(fmt.Sprintf(`<span class='chip'>%d messages</span>`, nMsg))
	if shortThreadID != "" {
		chips.WriteString(fmt.Sprintf(`<span class='chip mono'>#%s</span>`, html.EscapeString(shortThreadID)))
	}
	profileLink := ""
	if data.ProfileURL != "" {
		profileLink = fmt.Sprintf(`<a class='profile-link' href='%s'>View profile</a>`, html.EscapeString(data.ProfileURL))
	}

	var parts strings.Builder
	parts.WriteString("<!doctype html><html><head><meta charset='utf-8'>")
	parts.WriteString("<title>" + html.EscapeString(name) + "</title>")
	parts.WriteString(css)
	parts.WriteString("</head><body>")
	// Repeating <thead> draws the compact header on every page; the hero
	// pulls up over it on page 1 so only the full header is visible there.
	parts.WriteString("<table class='sheet'><thead><tr><td>")
	parts.WriteString("<div class='run-header'><div class='run-card'>")
	parts.WriteString(avatarHTML(data.Photo, name, "run-pic"))
	parts.WriteString("<div class='run-id'>")
	parts.WriteString("<div class='run-name'>" + html.EscapeString(name) + "</div>")
	if data.Headline != "" {
		parts.WriteString("<div class='run-headline'>" + html.EscapeString(data.Headline) + "</div>")
	}
	parts.WriteString("</div></div></div>")
	parts.WriteString("<div class='run-gap'></div>")
	parts.WriteString("</td></tr></thead><tbody><tr><td>")
	parts.WriteString("<header class='hero'>")
	parts.WriteString("<div class='hero-top'><span>LinkedIn conversation</span>")
	parts.WriteString("<span>" + html.EscapeString(exported) + "</span></div>")
	parts.WriteString("<div class='hero-card'>")
	parts.WriteString(avatarHTML(data.Photo, name, "avatar"))
	parts.WriteString("<div class='identity'>")
	parts.WriteString("<h1>" + html.EscapeString(name) + "</h1>")
	if data.Headline != "" {
		parts.WriteString("<p class='headline'>" + html.EscapeString(data.Headline) + "</p>")
	}
	parts.WriteString("<div class='chips'>" + chips.String() + profileLink + "</div>")
	parts.WriteString("</div></div></header>")
	parts.WriteString("<main class='thread'>")

	for _, item := range data.Items {
		if item.Heading != "" {
			parts.WriteString(fmt.Sprintf(`<div class='day'><span>%s</span></div>`, html.EscapeString(item.Heading)))
		}
		if item.Type != "msg" {
			continue
		}
		senderRaw := item.Sender
		if senderRaw == "" {
			if item.Self {
				senderRaw = "You"
			} else {
				senderRaw = name
			}
		}
		sender := html.EscapeString(senderRaw)
		timeS := html.EscapeString(item.Time)
		subject := ""
		if item.Subject != "" {
			subject = fmt.Sprintf(`<div class='subject'>%s</div>`, html.EscapeString(item.Subject))
		}
		body := sanitizeFragment(item.HTML)
		if body == "" {
			body = html.EscapeString(item.Text)
		}
		var images strings.Builder
		for _, src := range item.Images {
			images.WriteString(fmt.Sprintf(`<img class='pic' src='%s' alt=''>`, html.EscapeString(src)))
		}
		kind := "other"
		if item.Self {
			kind = "self"
		}
		parts.WriteString("<article class='msg " + kind + "'>")
		parts.WriteString(avatarHTML(item.SenderPhoto, senderRaw, "sender-pic"))
		parts.WriteString("<div class='msg-body'><div class='who'><b>" + sender + "</b>  ·  " + timeS + "</div>")
		parts.WriteString("<div class='bubble'>" + subject + body + images.String() + "</div></div></article>")
	}

	parts.WriteString("</main></td></tr></tbody></table></body></html>")
	return parts.String()
}

const css = `
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700;1,9..40,400&family=Fraunces:opsz,wght@9..144,560;9..144,650&display=swap" rel="stylesheet">
<style>
  :root {
    --ink: #12202b;
    --muted: #5c6b76;
    --page: #ffffff;
    --white: #ffffff;
    --line: #e4e7eb;
    --accent: #0a66c2;
    --accent-soft: #e7f0fa;
    --self: #173a63;
    --hero-grad: linear-gradient(135deg, #0c1422 0%, #163152 58%, #0a66c2 130%);
    /* Compact header (25+44+25) + gap below (20) — must match .run-header/.run-gap. */
    --run-block: 114px;
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; }
  body {
    font-family: "DM Sans", "Segoe UI", Helvetica, Arial, sans-serif;
    color: var(--ink);
    background: var(--page);
    font-size: 13.5px;
    line-height: 1.55;
    -webkit-print-color-adjust: exact;
    print-color-adjust: exact;
  }
  .sheet {
    width: 100%; border-collapse: collapse; border-spacing: 0;
  }
  .sheet > thead > tr > td,
  .sheet > tbody > tr > td {
    padding: 0; vertical-align: top;
  }
  .run-header {
    background: var(--hero-grad);
    color: #fff;
    padding: 25px 40px;
  }
  .run-card {
    display: flex; gap: 12px; align-items: center; min-width: 0;
  }
  .run-pic {
    width: 44px; height: 44px; border-radius: 50%; object-fit: cover;
    flex-shrink: 0; border: 2px solid rgba(255,255,255,.88); background: #ddd6c8;
  }
  .run-pic.fallback {
    display: flex; align-items: center; justify-content: center;
    background: rgba(255,255,255,.14); color: #fff; font-weight: 700;
    font-size: 14px; letter-spacing: .04em;
    font-family: Fraunces, Georgia, serif;
  }
  .run-id { min-width: 0; flex: 1; }
  .run-name {
    font-family: Fraunces, Georgia, serif; font-size: 16px; font-weight: 650;
    line-height: 1.2; letter-spacing: -.01em;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .run-headline {
    margin-top: 2px; color: rgba(255,255,255,.86); font-size: 12px; font-weight: 400;
    line-height: 1.25; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .run-gap { height: 20px; background: var(--page); }
  .hero {
    background: var(--hero-grad);
    color: #fff;
    /* Cover the repeating thead on page 1 (compact header + gap). */
    margin-top: calc(-1 * var(--run-block));
    position: relative; z-index: 2;
    padding: 25px 40px 36px;
    break-inside: avoid;
  }
  .hero-top {
    display: flex; justify-content: space-between; align-items: center;
    font-size: 10px; letter-spacing: .16em; text-transform: uppercase;
    color: rgba(255,255,255,.62); font-weight: 600; margin-bottom: 22px;
  }
  .hero-card { display: flex; gap: 22px; align-items: center; }
  .avatar {
    width: 92px; height: 92px; border-radius: 50%; object-fit: cover;
    flex-shrink: 0; border: 3px solid rgba(255,255,255,.88);
    box-shadow: 0 10px 30px rgba(0,0,0,.28);
  }
  .avatar.fallback, .sender-pic.fallback {
    display: flex; align-items: center; justify-content: center;
    background: #d9e6f5; color: #163152; font-weight: 700; letter-spacing: .04em;
  }
  .hero .avatar.fallback { background: rgba(255,255,255,.14); color: #fff; font-size: 28px;
    font-family: Fraunces, Georgia, serif; }
  .identity { min-width: 0; }
  h1 {
    font-family: Fraunces, Georgia, serif; font-size: 30px; font-weight: 650;
    line-height: 1.15; margin: 0 0 8px; letter-spacing: -.02em;
  }
  .headline { margin: 0 0 12px; color: rgba(255,255,255,.86); font-size: 14px; font-weight: 400; }
  .chips { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .chip {
    display: inline-block; padding: 4px 10px; border-radius: 999px;
    background: rgba(255,255,255,.12); border: 1px solid rgba(255,255,255,.16);
    font-size: 11px; font-weight: 500; color: rgba(255,255,255,.9);
  }
  .chip.mono { font-variant-numeric: tabular-nums; letter-spacing: .04em; }
  .profile-link { color: #fff; font-size: 12px; font-weight: 600; margin-left: 6px; }
  .thread { padding: 28px 40px 48px; max-width: 780px; background: var(--page); }
  .day {
    display: flex; align-items: center; gap: 14px; margin: 26px 0 16px;
    color: var(--muted); font-size: 11px; font-weight: 650;
    letter-spacing: .14em; text-transform: uppercase;
    break-inside: avoid; break-after: avoid;
  }
  .day::before, .day::after {
    content: ""; flex: 1; height: 1px; background: var(--line);
  }
  .day span {
    background: #f3f4f6; padding: 5px 12px; border-radius: 999px;
    border: 1px solid var(--line);
  }
  .msg {
    display: block; position: relative; margin: 0 0 16px;
    padding-left: 46px;
  }
  .msg.self { padding-left: 0; padding-right: 46px; }
  .sender-pic {
    position: absolute; left: 0; top: 18px;
    width: 34px; height: 34px; border-radius: 50%; object-fit: cover;
    background: #d8dde3;
  }
  .msg.self .sender-pic { left: auto; right: 0; }
  .msg-body { min-width: 0; max-width: 86%; }
  .msg.self .msg-body { margin-left: auto; }
  .who {
    font-size: 11px; color: var(--muted); margin: 0 0 5px; padding: 0 4px;
    break-after: avoid;
  }
  .who b { color: var(--ink); font-weight: 650; }
  .msg.self .who { text-align: right; }
  .bubble {
    background: var(--white); border: 1px solid var(--line);
    border-radius: 18px 18px 18px 6px; padding: 12px 16px 13px;
    box-decoration-break: slice; -webkit-box-decoration-break: slice;
  }
  .msg.self .bubble {
    background: var(--accent-soft); border-color: #d3e4f5;
    border-radius: 18px 18px 6px 18px;
  }
  .subject {
    font-weight: 700; font-size: 13px; margin: 0 0 8px; color: var(--self);
  }
  .bubble p { margin: 0 0 .55em; }
  .bubble p:last-child { margin-bottom: 0; }
  .bubble a { color: var(--accent); text-decoration: none; font-weight: 500; }
  img.pic {
    max-width: 100%; height: auto; display: block; margin-top: 10px;
    border-radius: 12px; break-inside: avoid;
  }
</style>
`
