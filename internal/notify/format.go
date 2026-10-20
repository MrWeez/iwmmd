package notify

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode"

	"github.com/mrweez/iwmmd/internal/alert"
)

type messageTone struct {
	accent string
	soft   string
}

var (
	toneAccent  = messageTone{accent: "#0b6e69", soft: "#d4ece8"}
	toneSignal  = messageTone{accent: "#b45309", soft: "#fef3c7"}
	toneDanger  = messageTone{accent: "#b42318", soft: "#fee4e2"}
	toneSuccess = messageTone{accent: "#15803d", soft: "#dcfce7"}
)

type messageFact struct {
	label string
	value string
}

type message struct {
	subject     string
	preheader   string
	kicker      string
	heading     string
	domain      string
	status      string
	summary     string
	facts       []messageFact
	actionLabel string
	actionURL   string
	tone        messageTone
}

// FormatMessage returns a subject plus plain-text and HTML bodies.
func FormatMessage(payload alert.Payload) (string, string, string) {
	msg := buildMessage(payload)
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(msg.subject)
	return subject, msg.plainText(), msg.htmlBody(subject)
}

func buildMessage(payload alert.Payload) message {
	switch {
	case payload.Event == "test":
		summary := payload.Message
		if summary == "" {
			summary = "IWMMD successfully delivered a test email. Your SMTP settings are working."
		}
		return message{
			subject:     "[IWMMD] Test notification",
			preheader:   "SMTP delivery is working.",
			kicker:      "Delivery test",
			heading:     "Test notification",
			status:      "Delivered",
			summary:     summary,
			actionLabel: "Open IWMMD",
			actionURL:   payload.URL,
			tone:        toneAccent,
		}
	case payload.Kind == "system":
		domain := valueOr(payload.Domain, "Domain")
		kind := incidentKindLabel(payload.IncidentKind)
		if payload.SystemEvent == "resolved" {
			return message{
				subject:   fmt.Sprintf("[IWMMD] Recovered: %s %s incident", domain, kind),
				preheader: fmt.Sprintf("%s incident recovered for %s.", kind, domain),
				kicker:    "Incident recovered",
				heading:   domain,
				domain:    domain,
				status:    "Recovered",
				summary:   "The latest checks no longer show this incident. No action is needed unless it returns.",
				facts: []messageFact{
					{label: "Incident", value: kind},
					{label: "Recovered", value: formatEventTime(payload.DueAt)},
				},
				actionLabel: "Review domain",
				actionURL:   payload.URL,
				tone:        toneSuccess,
			}
		}
		return message{
			subject:   fmt.Sprintf("[IWMMD] Action needed: %s %s incident", domain, kind),
			preheader: fmt.Sprintf("Open %s incident detected for %s.", kind, domain),
			kicker:    "Incident alert",
			heading:   domain,
			domain:    domain,
			status:    fmt.Sprintf("%s incident open", kind),
			summary:   "Review the domain for details and next steps.",
			facts: []messageFact{
				{label: "Incident", value: kind},
				{label: "Detected", value: formatEventTime(payload.DueAt)},
			},
			actionLabel: "Review domain",
			actionURL:   payload.URL,
			tone:        toneDanger,
		}
	default:
		domain := valueOr(payload.Domain, "Domain")
		when := expiryWhen(payload.DaysLeft)
		expiryDate := formatExpiryDate(payload.Expiry)
		summary := "Renew the registration before the expiry date to avoid losing control of the domain."
		preheader := "Domain registration expiry reminder."
		if payload.DaysLeft < 0 {
			summary = "The registration has expired. Renew it as soon as possible to regain control of the domain."
			if expiryDate != "" {
				preheader = fmt.Sprintf("Registration expired on %s.", expiryDate)
			}
		} else if payload.DaysLeft == 0 {
			summary = "The registration expires today. Renew it now to avoid losing control of the domain."
			if expiryDate != "" {
				preheader = fmt.Sprintf("Registration expires today, %s.", expiryDate)
			}
		} else if expiryDate != "" {
			preheader = fmt.Sprintf("Registration expires on %s.", expiryDate)
		}
		return message{
			subject:   fmt.Sprintf("[IWMMD] %s %s", domain, when),
			preheader: preheader,
			kicker:    "Domain expiry",
			heading:   domain,
			domain:    domain,
			status:    sentenceCase(when),
			summary:   summary,
			facts: []messageFact{
				{label: "Expiry date", value: expiryDate},
				{label: "Alert rule", value: payload.Rule},
			},
			actionLabel: "Open domain",
			actionURL:   payload.URL,
			tone:        expiryTone(payload.DaysLeft),
		}
	}
}

func (m message) plainText() string {
	var b strings.Builder
	if m.domain != "" {
		fmt.Fprintf(&b, "Domain: %s\n", m.domain)
		fmt.Fprintf(&b, "Status: %s\n\n", m.status)
	}
	b.WriteString(m.summary)
	b.WriteString("\n")

	facts := m.visibleFacts()
	if len(facts) > 0 {
		b.WriteString("\n")
		for _, fact := range facts {
			fmt.Fprintf(&b, "%s: %s\n", fact.label, fact.value)
		}
	}
	if m.actionURL != "" {
		label := valueOr(m.actionLabel, "Open in IWMMD")
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s: %s\n", label, m.actionURL)
	}
	b.WriteString("\nSent by IWMMD.\n")
	return b.String()
}

func (m message) htmlBody(subject string) string {
	var b strings.Builder
	preheader := valueOr(m.preheader, m.summary)

	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light"><title>`)
	b.WriteString(html.EscapeString(subject))
	b.WriteString(`</title></head><body style="margin:0;padding:0;background-color:#e9eef0;color:#10212b;">`)
	b.WriteString(`<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">`)
	b.WriteString(html.EscapeString(preheader))
	b.WriteString(`</div>`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#e9eef0;"><tr><td align="center" style="padding:28px 12px;">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:600px;background-color:#f8faf9;border:1px solid #cbd8db;border-radius:12px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">`)

	b.WriteString(`<tr><td style="background-color:#0d1d25;padding:16px 28px;">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr><td style="font-size:15px;font-weight:700;letter-spacing:0.08em;color:#5eead4;">IWMMD</td>`)
	b.WriteString(`<td align="right" style="font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:#8ea6ae;">`)
	b.WriteString(html.EscapeString(m.kicker))
	b.WriteString(`</td></tr></table></td></tr>`)

	b.WriteString(`<tr><td style="padding:28px 28px 0;">`)
	fmt.Fprintf(&b, `<h1 style="margin:0;font-size:24px;line-height:1.25;font-weight:600;color:#10212b;word-break:break-word;">%s</h1>`, html.EscapeString(m.heading))
	if m.status != "" {
		fmt.Fprintf(&b, `<span style="display:inline-block;margin-top:12px;padding:4px 9px;border-radius:999px;background-color:%s;color:%s;font-size:11px;font-weight:700;">%s</span>`, m.tone.soft, m.tone.accent, html.EscapeString(m.status))
	}
	b.WriteString(`</td></tr>`)

	b.WriteString(`<tr><td style="padding:20px 28px 28px;">`)
	fmt.Fprintf(&b, `<p style="margin:0;font-size:14px;line-height:1.6;color:#5a6f79;">%s</p>`, html.EscapeString(m.summary))

	facts := m.visibleFacts()
	if len(facts) > 0 {
		b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-top:20px;border-collapse:collapse;">`)
		for _, fact := range facts {
			fmt.Fprintf(&b, `<tr><td style="padding:9px 0;border-top:1px solid #cbd8db;font-size:12px;line-height:1.4;color:#5a6f79;">%s</td><td align="right" style="padding:9px 0;border-top:1px solid #cbd8db;font-size:12px;line-height:1.4;font-weight:600;color:#10212b;">%s</td></tr>`, html.EscapeString(fact.label), html.EscapeString(fact.value))
		}
		b.WriteString(`</table>`)
	}

	if m.actionURL != "" {
		label := valueOr(m.actionLabel, "Open in IWMMD")
		b.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin-top:24px;"><tr><td style="border-radius:6px;background-color:`)
		b.WriteString(m.tone.accent)
		b.WriteString(`;"><a href="`)
		b.WriteString(html.EscapeString(m.actionURL))
		b.WriteString(`" style="display:inline-block;padding:11px 18px;font-size:13px;font-weight:700;color:#ffffff;text-decoration:none;border-radius:6px;">`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</a></td></tr></table>`)
		b.WriteString(`<p style="margin:12px 0 0;font-size:11px;line-height:1.5;color:#5a6f79;word-break:break-all;">`)
		b.WriteString(html.EscapeString(m.actionURL))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</td></tr>`)

	b.WriteString(`<tr><td style="padding:18px 28px 22px;border-top:1px solid #cbd8db;background-color:#eef3f3;font-size:11px;line-height:1.5;color:#5a6f79;">Sent by IWMMD. This is an automated message.</td></tr>`)
	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}

func (m message) visibleFacts() []messageFact {
	facts := make([]messageFact, 0, len(m.facts))
	for _, fact := range m.facts {
		if strings.TrimSpace(fact.value) != "" {
			facts = append(facts, fact)
		}
	}
	return facts
}

func expiryWhen(days int) string {
	switch {
	case days < 0:
		count := -days
		if count == 1 {
			return "expired 1 day ago"
		}
		return fmt.Sprintf("expired %d days ago", count)
	case days == 0:
		return "expires today"
	case days == 1:
		return "expires tomorrow"
	default:
		return fmt.Sprintf("expires in %d days", days)
	}
}

func expiryTone(days int) messageTone {
	switch {
	case days <= 7:
		return toneDanger
	case days <= 30:
		return toneSignal
	default:
		return toneAccent
	}
}

func incidentKindLabel(kind string) string {
	switch kind {
	case "dns":
		return "DNS"
	case "tls":
		return "TLS"
	case "http":
		return "HTTP"
	}
	words := strings.Fields(strings.ReplaceAll(kind, "_", " "))
	for i, word := range words {
		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	if len(words) == 0 {
		return "System"
	}
	return strings.Join(words, " ")
}

func formatExpiryDate(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("Jan 2, 2006")
}

func formatEventTime(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("Jan 2, 2006 at 15:04 UTC")
}

func sentenceCase(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
