package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Writer handles all CLI output, switching between JSON and human-readable modes.
type Writer struct {
	w      io.Writer
	isTTY  bool
	pretty bool // explicit --pretty flag
	json   bool // explicit --json flag
	quiet  bool
}

// New creates a Writer. isTTY should be the result of detecting whether stdout
// is a terminal. The pretty/jsonFlag arguments honour explicit CLI flags.
func New(w io.Writer, isTTY, prettyFlag, jsonFlag, quiet bool) *Writer {
	return &Writer{
		w:      w,
		isTTY:  isTTY,
		pretty: prettyFlag,
		json:   jsonFlag,
		quiet:  quiet,
	}
}

// IsJSON returns true when output should be machine-readable JSON.
func (wr *Writer) IsJSON() bool {
	if wr.json {
		return true
	}
	if wr.pretty {
		return false
	}
	return !wr.isTTY
}

// JSON encodes v as JSON to the writer. In pretty mode the same value is
// rendered as indented JSON — the CLI always produces valid JSON so that
// human and machine consumers both work.
func (wr *Writer) JSON(v any) error {
	if wr.quiet {
		return nil
	}
	enc := json.NewEncoder(wr.w)
	if wr.IsJSON() {
		enc.SetEscapeHTML(false)
	} else {
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
	}
	return enc.Encode(v)
}

// Error writes a structured error envelope and returns the DavitError so the
// caller can use its exit code.
func (wr *Writer) Error(code ErrorCode, message string, ctx map[string]any) *DavitError {
	e := NewError(code, message, ctx)
	if !wr.quiet {
		envelope := map[string]any{
			"status":     "error",
			"error_code": e.Code,
			"message":    e.Message,
			"docs_url":   e.DocsURL,
		}
		if len(e.Context) > 0 {
			envelope["context"] = e.Context
		}
		enc := json.NewEncoder(wr.w)
		enc.SetEscapeHTML(false)
		if !wr.IsJSON() {
			enc.SetIndent("", "  ")
		}
		_ = enc.Encode(envelope)
	}
	return e
}

// IsTTY returns whether stdout is a terminal.
func IsTTY() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Stderr writes a plain message to stderr (for internal diagnostics only).
func Stderr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
