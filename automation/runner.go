// Package automation implements documentation generation and maintenance workflows.
package automation

import (
	"fmt"
	"io"
)

// Runner executes documentation workflows using the supplied output streams.
type Runner struct {
	out     io.Writer
	errOut  io.Writer
	verbose bool
}

// NewRunner creates a documentation workflow runner.
func NewRunner(out, errOut io.Writer, verbose bool) *Runner {
	return &Runner{out: out, errOut: errOut, verbose: verbose}
}

func (r *Runner) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.out, format, args...)
}

func (r *Runner) errorf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.errOut, format, args...)
}

func (r *Runner) verbosef(format string, args ...any) {
	if r.verbose {
		r.errorf(format, args...)
	}
}
