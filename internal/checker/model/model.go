// Package model contains shared checker results.
package model

import "time"

// Result is the outcome of one lookup.
type Result struct {
	Source       string
	Status       string // ok, error, unsupported
	HTTPStatus   *int
	ExpiresAt    *time.Time
	Registrar    string
	Confidence   string // high, medium, low, unknown
	ErrorCode    string
	ErrorMessage string
	Duration     time.Duration
	Raw          []byte
	ContentType  string
}
