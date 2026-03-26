package templates

import "testing"

func mustTemplate(t *testing.T, name string) DomainTemplate {
	t.Helper()
	dt, ok := Get(name)
	if !ok {
		t.Fatalf("expected template %q to be registered", name)
	}
	return dt
}
