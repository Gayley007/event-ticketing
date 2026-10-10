// Package validate provides a tiny error collector used by each service's
// validation rules, so a client sees every invalid field in one response.
package validate

import (
	"strings"

	"github.com/google/uuid"
)

// Errors collects field-level validation problems.
type Errors struct {
	msgs []string
}

// Add records a problem when cond is true.
func (e *Errors) Add(cond bool, msg string) {
	if cond {
		e.msgs = append(e.msgs, msg)
	}
}

// Err returns nil when no problems were recorded.
func (e *Errors) Err() error {
	if len(e.msgs) == 0 {
		return nil
	}
	return &Error{Fields: e.msgs}
}

// Error is returned when one or more fields are invalid.
type Error struct {
	Fields []string
}

func (e *Error) Error() string {
	return "validation failed: " + strings.Join(e.Fields, "; ")
}

// IsUUID reports whether s is a well-formed UUID.
func IsUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
