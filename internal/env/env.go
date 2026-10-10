// Package env reads configuration from environment variables.
//
// Every service gets its ports, database credentials and resilience settings
// from the environment (set in docker-compose.yml), never from constants in
// the code.
package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Loader collects missing or malformed variables so a service can report all
// configuration problems at once instead of failing on the first one.
type Loader struct {
	errs []string
}

// Required returns the value of key, recording an error if it is unset.
func (l *Loader) Required(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		l.errs = append(l.errs, fmt.Sprintf("%s is required", key))
	}
	return v
}

// Int returns key parsed as an int, or def when unset.
func (l *Loader) Int(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		l.errs = append(l.errs, fmt.Sprintf("%s must be a positive integer, got %q", key, v))
		return def
	}
	return n
}

// Duration returns key parsed as a Go duration (e.g. "1s", "250ms"), or def when unset.
func (l *Loader) Duration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.errs = append(l.errs, fmt.Sprintf("%s must be a positive duration like 500ms or 2s, got %q", key, v))
		return def
	}
	return d
}

// Err returns a combined error for every problem found, or nil.
func (l *Loader) Err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration: %s", strings.Join(l.errs, "; "))
}
