package jsonwriter

import (
	"encoding/json"
	"fmt"
	"io"
)

// Writer writes JSON arrays to an output stream.
type Writer struct {
	w        io.Writer
	headers  []string
	firstRow bool
	started  bool
	err      error
}

// NewWriter returns a new Writer that writes to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:        w,
		firstRow: true,
		started:  false,
	}
}

// Write writes a single JSON object to the output stream.
// The first call sets the headers (field names) and begins the array with "[".
// Subsequent calls write objects using the headers as keys, separated by commas.
func (w *Writer) Write(row []string) error {
	if !w.started {
		if len(row) == 0 {
			return nil // No-op for empty headers
		}
		w.headers = make([]string, len(row))
		copy(w.headers, row)
		_, _ = w.w.Write([]byte("[\n"))
		w.started = true
		return nil
	}

	if len(row) != len(w.headers) {
		return &FieldCountError{Expected: len(w.headers), Got: len(row)}
	}

	// Build JSON object
	obj := make(map[string]string, len(w.headers))
	for i, key := range w.headers {
		obj[key] = row[i]
	}

	// Encode JSON object
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	// Write comma and newline before object (except for first row)
	if !w.firstRow {
		_, _ = w.w.Write([]byte(",\n"))
	}
	w.firstRow = false
	_, _ = w.w.Write(data)

	return nil
}

// Flush writes the closing "]" and newline to the output stream.
func (w *Writer) Flush() {
	if !w.started {
		_, w.err = w.w.Write([]byte("[]\n"))
		return
	}
	_, w.err = w.w.Write([]byte("\n]\n"))
}

func (w *Writer) Error() error {
	return w.err
}

// FieldCountError is returned when the number of fields in a row doesn't match the headers.
type FieldCountError struct {
	Expected int
	Got      int
}

func (e *FieldCountError) Error() string {
	return fmt.Sprintf("jsonwriter: expected '%d' fields, got \"%d\"", e.Expected, e.Got)
}
