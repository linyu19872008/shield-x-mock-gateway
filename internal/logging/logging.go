// Package logging provides the small logger shared by mock components.
package logging

import (
	"fmt"
	"os"
	"time"
)

// Logger is a printf-style log function.
type Logger func(format string, args ...any)

// New returns a logger that writes to stderr with a local timestamp.
func New() Logger {
	return func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	}
}
