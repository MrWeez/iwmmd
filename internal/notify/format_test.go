package notify

import (
	"strings"
	"testing"

	"github.com/mrweez/iwmmd/internal/alert"
)

func TestFormatExpiryMessage(t *testing.T) {
	payload := alert.Payload{
		Event:    "domain.expiring",
		Kind:     "expiry",
		Domain:   "example.com",
		Expiry:   "2026-11-03",
		DaysLeft: 14,
		Rule:     "14 days before",
		URL:      "https://iwmmd.example.com/domains/dom_1",
	}
	subject, text, body := FormatMessage(payload)

	if subject != "[IWMMD] example.com expires in 14 days" {
		t.Fatalf("subject = %q", subject)
	}
	if strings.Contains(text, subject) {
		t.Fatalf("text repeats subject: %q", text)
	}
	for _, want := range []string{
		"Domain: example.com",
		"Status: Expires in 14 days",
		"Expiry date: Nov 3, 2026",
		"Alert rule: 14 days before",
		"Open domain: https://iwmmd.example.com/domains/dom_1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text missing %q:\n%s", want, text)
		}
	}
	for _, want := range []string{
		"<h1",
		"example.com",
		"Expires in 14 days",
		"Nov 3, 2026",
		"14 days before",
		`href="https://iwmmd.example.com/domains/dom_1"`,
		"Open domain",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("html missing %q", want)
		}
	}
	if strings.Contains(body, "<h2") {
		t.Fatal("html still repeats the subject as a heading")
	}
}

func TestFormatIncidentMessages(t *testing.T) {
	tests := []struct {
		name        string
		event       string
		wantSubject string
		wantBody    []string
	}{
		{
			name:        "open",
			event:       "opened",
			wantSubject: "[IWMMD] Action needed: example.com DNS incident",
			wantBody:    []string{"DNS incident open", "Incident", "Detected", "Review domain"},
		},
		{
			name:        "resolved",
			event:       "resolved",
			wantSubject: "[IWMMD] Recovered: example.com DNS incident",
			wantBody:    []string{"Incident recovered", "Recovered", "DNS", "Review domain"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := alert.Payload{
				Event:        "system.incident",
				Kind:         "system",
				Domain:       "example.com",
				IncidentKind: "dns",
				SystemEvent:  tt.event,
				Message:      "dns incident " + tt.event + " for example.com",
				DueAt:        "2026-10-20T10:00:00Z",
				URL:          "https://iwmmd.example.com/domains/dom_1",
			}
			subject, text, body := FormatMessage(payload)
			if subject != tt.wantSubject {
				t.Fatalf("subject = %q", subject)
			}
			if strings.Contains(text, payload.Message) {
				t.Fatalf("text repeats payload message: %q", text)
			}
			if !strings.Contains(text, "Domain: example.com") {
				t.Fatalf("text missing domain: %q", text)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Fatalf("html missing %q", want)
				}
			}
		})
	}
}

func TestFormatTestMessageEscapesHTML(t *testing.T) {
	payload := alert.Payload{
		Event:   "test",
		Message: `<strong>bold</strong> & "quoted"`,
	}
	_, text, body := FormatMessage(payload)
	if !strings.Contains(text, payload.Message) {
		t.Fatalf("text missing message: %q", text)
	}
	if strings.Contains(body, "<strong>bold</strong>") {
		t.Fatal("html contains unescaped message markup")
	}
	if !strings.Contains(body, "&lt;strong&gt;bold&lt;/strong&gt;") {
		t.Fatal("html does not contain escaped message")
	}
}

func TestExpiryWhen(t *testing.T) {
	tests := map[int]string{
		-3: "expired 3 days ago",
		-1: "expired 1 day ago",
		0:  "expires today",
		1:  "expires tomorrow",
		14: "expires in 14 days",
	}
	for days, want := range tests {
		if got := expiryWhen(days); got != want {
			t.Fatalf("expiryWhen(%d) = %q, want %q", days, got, want)
		}
	}
}
