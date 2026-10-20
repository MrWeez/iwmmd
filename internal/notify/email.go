package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/settings"
)

// DeliveryError classifies a channel failure.
type DeliveryError struct {
	Status    int
	Retryable bool
	Err       error
}

func (e *DeliveryError) Error() string {
	if e.Err == nil {
		return "delivery failed"
	}
	return e.Err.Error()
}

func (e *DeliveryError) Unwrap() error { return e.Err }

func sendEmail(ctx context.Context, snap settings.SMTP, cfg EmailConfig, payload alert.Payload, notificationID string) (int, error) {
	if snap.Host == "" || snap.From == "" {
		return 0, &DeliveryError{Retryable: false, Err: errors.New("SMTP is not configured")}
	}
	subject, text, html := FormatMessage(payload)
	boundary := "iwmmd-" + notificationID
	message := buildEmail(snap.From, cfg.Recipients, subject, text, html, boundary, time.Now())
	address := net.JoinHostPort(snap.Host, fmt.Sprintf("%d", snap.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	defer conn.Close()
	deadline := time.Now().Add(30 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	tlsConfig := &tls.Config{ServerName: snap.Host, MinVersion: tls.VersionTLS12}
	if snap.TLSMode == "implicit" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return 0, &DeliveryError{Retryable: true, Err: err}
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, snap.Host)
	if err != nil {
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	defer client.Close()
	if snap.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return 0, &DeliveryError{Retryable: false, Err: errors.New("SMTP server does not support STARTTLS")}
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return 0, &DeliveryError{Retryable: true, Err: err}
		}
	}
	if snap.Username != "" {
		auth := smtp.PlainAuth("", snap.Username, snap.Password, snap.Host)
		if err := client.Auth(auth); err != nil {
			return 0, &DeliveryError{Retryable: false, Err: fmt.Errorf("SMTP auth: %w", err)}
		}
	}
	if err := client.Mail(snap.From); err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	for _, recipient := range cfg.Recipients {
		if err := client.Rcpt(recipient); err != nil {
			return 0, &DeliveryError{Retryable: false, Err: err}
		}
	}
	writer, err := client.Data()
	if err != nil {
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	if err := writer.Close(); err != nil {
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	_ = client.Quit()
	return 250, nil
}

// SendTestEmail delivers a synthetic message using the current SMTP settings.
func SendTestEmail(ctx context.Context, snap settings.SMTP, recipients []string, payload alert.Payload) error {
	_, err := sendEmail(ctx, snap, EmailConfig{Recipients: recipients}, payload, "test-smtp")
	return err
}

func buildEmail(from string, recipients []string, subject, text, html, boundary string, now time.Time) []byte {
	var buf bytes.Buffer
	writeHeader := func(key, value string) {
		_, _ = fmt.Fprintf(&buf, "%s: %s\r\n", key, value)
	}
	writeHeader("From", from)
	writeHeader("To", strings.Join(recipients, ", "))
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", subject))
	writeHeader("Date", now.Format(time.RFC1123Z))
	writeHeader("MIME-Version", "1.0")
	writeHeader("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	_, _ = buf.WriteString("\r\n")
	_, _ = fmt.Fprintf(&buf, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, text)
	_, _ = fmt.Fprintf(&buf, "--%s\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, html)
	_, _ = fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	return buf.Bytes()
}
