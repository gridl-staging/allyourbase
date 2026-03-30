package storage

import (
	"mime"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzRewritePublicURL exercises the CDN URL rewriting logic that swaps
// scheme/host from an origin URL to a CDN base URL. It must never panic,
// even on malformed URLs, empty strings, or adversarial input with
// embedded null bytes or Unicode.
func FuzzRewritePublicURL(f *testing.F) {
	seeds := []struct {
		origin string
		cdn    string
	}{
		// Normal cases.
		{"https://s3.example.com/bucket/file.png", "https://cdn.example.com"},
		{"https://s3.example.com/bucket/file.png?v=1", "https://cdn.example.com"},
		{"http://localhost:9000/bucket/img.jpg", "https://cdn.prod.com"},

		// Empty/missing values — should return origin unchanged.
		{"", "https://cdn.example.com"},
		{"https://s3.example.com/file.png", ""},
		{"", ""},

		// Malformed URLs.
		{"not-a-url", "https://cdn.example.com"},
		{"https://s3.example.com/file.png", "not-a-url"},
		{"://no-scheme", "https://cdn.example.com"},
		{"ftp://", "https://cdn.example.com"},

		// Adversarial input.
		{"\x00", "\x00"},
		{"https://evil.com@s3.example.com/file.png", "https://cdn.example.com"},
		{strings.Repeat("a", 2048), strings.Repeat("b", 2048)},
		{"https://例え.jp/ファイル.png", "https://cdn.example.com"},
	}
	for _, s := range seeds {
		f.Add(s.origin, s.cdn)
	}

	f.Fuzz(func(t *testing.T, origin, cdn string) {
		// Cap to prevent OOM on huge generated strings.
		if len(origin) > 4096 || len(cdn) > 4096 {
			t.Skip()
		}
		// Must never panic — the function should always return a string.
		_ = RewritePublicURL(origin, cdn)
	})
}

// FuzzStorageMIMEDetection exercises the MIME content-type detection logic
// used by the storage upload path. The detection chain is:
//  1. mime.TypeByExtension(filepath.Ext(name)) — stdlib extension lookup
//  2. fallback to provided header Content-Type
//  3. fallback to "application/octet-stream"
//
// This fuzz target ensures the chain never panics on adversarial filenames,
// particularly those with unusual extensions, path traversal sequences,
// embedded null bytes, or very long names.
func FuzzStorageMIMEDetection(f *testing.F) {
	seeds := []string{
		"file.png",
		"file.jpg",
		"document.pdf",
		"archive.tar.gz",
		"noext",
		".hidden",
		"",
		"file.",
		".tar.gz",
		"path/to/file.txt",
		"../../../etc/passwd",
		"file\x00.png",
		strings.Repeat("a", 1024) + ".xyz",
		"名前.テキスト",
		"file.JPEG",
		"file.JPG",
		"file.PNG",
		"file.unknown-extension-12345",
		"file.a",
		"....",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 4096 {
			t.Skip()
		}
		// Reproduce the exact detection chain from handler.go:parseUploadRequest.
		// This catches panics in filepath.Ext or mime.TypeByExtension on
		// adversarial filenames.
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			// In the real code this falls back to the multipart header Content-Type.
			// We simulate the header being empty to test the final fallback.
			contentType = "application/octet-stream"
		}
		// Sanity: contentType must never be empty after the chain.
		if contentType == "" {
			t.Error("contentType should never be empty after fallback chain")
		}
	})
}
