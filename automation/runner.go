// Package automation implements documentation generation and maintenance workflows.
package automation

import (
	"errors"
	"fmt"
	"io"

	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/parser"
)

type trackingWriter struct {
	writer io.Writer
	err    error
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

// Runner executes documentation workflows using the supplied output streams.
type Runner struct {
	out             *trackingWriter
	errOut          *trackingWriter
	verbose         bool
	resolveRevision revisionResolver
	parseRole       func(roleName, repoType, path string) (*parser.RoleInfo, error)
	saveDocument    func(manager *document.Manager, doc *document.Document) error
}

// NewRunner creates a documentation workflow runner.
func NewRunner(out, errOut io.Writer, verbose bool) *Runner {
	return &Runner{
		out:             &trackingWriter{writer: out},
		errOut:          &trackingWriter{writer: errOut},
		verbose:         verbose,
		resolveRevision: gitRevision,
		parseRole: func(roleName, repoType, path string) (*parser.RoleInfo, error) {
			return parser.New(roleName, repoType).ParseFile(path)
		},
		saveDocument: func(manager *document.Manager, doc *document.Document) error {
			return manager.SaveDocument(doc)
		},
	}
}

func (r *Runner) result(err error) error {
	return errors.Join(err, r.out.err, r.errOut.err)
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
