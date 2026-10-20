package notify

import (
	"fmt"
	"html"
	"strings"

	"github.com/mrweez/iwmmd/internal/alert"
)

// FormatMessage returns a subject plus plain-text and HTML bodies.
func FormatMessage(payload alert.Payload) (string, string, string) {
	subject := ""
	lines := []string{}
	if payload.Event == "test" {
		subject = "[IWMMD] Test notification"
		lines = append(lines, "This is a test notification from IWMMD.")
		if payload.Message != "" {
			lines = append(lines, payload.Message)
		}
	} else if payload.Kind == "system" {
		kind := strings.ReplaceAll(payload.IncidentKind, "_", " ")
		switch payload.SystemEvent {
		case "resolved":
			subject = fmt.Sprintf("[IWMMD] Recovered: %s %s", payload.Domain, kind)
			lines = append(lines, fmt.Sprintf("%s recovered from %s.", payload.Domain, kind))
		default:
			subject = fmt.Sprintf("[IWMMD] Alert: %s %s", payload.Domain, kind)
			lines = append(lines, fmt.Sprintf("%s has an open %s incident.", payload.Domain, kind))
		}
		if payload.Message != "" {
			lines = append(lines, "Details: "+payload.Message)
		}
	} else {
		when := "expires"
		switch {
		case payload.DaysLeft < 0:
			when = fmt.Sprintf("expired %d day(s) ago", -payload.DaysLeft)
		case payload.DaysLeft == 0:
			when = "expires today"
		case payload.DaysLeft == 1:
			when = "expires tomorrow"
		default:
			when = fmt.Sprintf("expires in %d days", payload.DaysLeft)
		}
		subject = fmt.Sprintf("[IWMMD] %s %s", payload.Domain, when)
		lines = append(lines, fmt.Sprintf("%s %s.", payload.Domain, when))
		if payload.Expiry != "" {
			lines = append(lines, "Expiry date: "+payload.Expiry)
		}
		if payload.Rule != "" {
			lines = append(lines, "Alert rule: "+payload.Rule)
		}
	}
	if payload.URL != "" {
		lines = append(lines, "Open IWMMD: "+payload.URL)
	}
	text := strings.Join(lines, "\n")
	var htmlBody strings.Builder
	htmlBody.WriteString("<div style=\"font-family:system-ui,-apple-system,Segoe UI,sans-serif;line-height:1.5;color:#111827\">")
	htmlBody.WriteString("<h2 style=\"margin:0 0 12px;font-size:18px\">" + html.EscapeString(subject) + "</h2>")
	for _, line := range lines {
		htmlBody.WriteString("<p style=\"margin:0 0 8px\">" + html.EscapeString(line) + "</p>")
	}
	htmlBody.WriteString("<p style=\"margin-top:16px;color:#6b7280;font-size:12px\">Sent by IWMMD.</p></div>")
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	return subject, text, htmlBody.String()
}
