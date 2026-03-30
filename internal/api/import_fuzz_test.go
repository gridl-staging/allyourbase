package api

import (
	"testing"
)

// FuzzImportContentType exercises the Content-Type validation path used by
// the import handler. The function strips charset parameters and rejects
// anything that isn't text/csv or application/json. The fuzz target ensures
// it never panics on adversarial MIME strings, including embedded null bytes,
// extremely long types, and malformed parameter separators.
func FuzzImportContentType(f *testing.F) {
	seeds := []string{
		// Valid types the handler accepts.
		"text/csv",
		"application/json",
		"text/csv; charset=utf-8",
		"application/json; charset=utf-8",

		// Types the handler should reject without panicking.
		"application/xml",
		"text/html",
		"multipart/form-data",
		"image/png",
		"application/octet-stream",

		// Edge cases: semicolons, whitespace, empty, null bytes.
		"",
		";",
		";;",
		"; charset=utf-8",
		"text/csv;",
		"text/csv;;extra",
		"text/csv; charset=utf-8; boundary=something",
		"text/csv\x00",
		"\x00text/csv",
		"text\x00/csv",

		// Adversarial: embedded null bytes, Unicode, whitespace tricks.
		// make([]byte, 1024) produces 1024 zero bytes — tests null-heavy MIME strings.
		"text/" + "x" + string(make([]byte, 1024)),
		"текст/csv",
		" text/csv ",
		"\ttext/csv\t",
		"TEXT/CSV",
		"Text/Csv",

		// Malformed MIME patterns.
		"/",
		"text/",
		"/csv",
		"text",
		"text/csv/extra",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Cap length to avoid OOM.
		if len(input) > 4096 {
			t.Skip()
		}
		// importContentType must never panic regardless of input.
		_, _ = importContentType(input)
	})
}
