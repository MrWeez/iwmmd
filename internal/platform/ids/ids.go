// Package ids creates sortable identifiers.
package ids

import "github.com/google/uuid"

// New returns a UUIDv7 string.
func New() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}
