package scaffold

import "testing"

// ---------------------------------------------------------------------------
// appendUniqueTemplate
// ---------------------------------------------------------------------------

func TestAppendUniqueTemplate(t *testing.T) {
	t.Parallel()

	t.Run("adds new template", func(t *testing.T) {
		t.Parallel()
		existing := []Template{TemplateReact, TemplateNext}
		got := appendUniqueTemplate(existing, TemplateExpress)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		if got[2] != TemplateExpress {
			t.Errorf("got[2] = %q, want %q", got[2], TemplateExpress)
		}
	})

	t.Run("skips duplicate", func(t *testing.T) {
		t.Parallel()
		existing := []Template{TemplateReact, TemplateNext}
		got := appendUniqueTemplate(existing, TemplateReact)
		// Should return the same slice unchanged.
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (duplicate should be skipped)", len(got))
		}
	})

	t.Run("empty slice accepts any candidate", func(t *testing.T) {
		t.Parallel()
		got := appendUniqueTemplate(nil, TemplatePlain)
		if len(got) != 1 || got[0] != TemplatePlain {
			t.Fatalf("got %v, want [plain]", got)
		}
	})

	t.Run("preserves order", func(t *testing.T) {
		t.Parallel()
		var list []Template
		list = appendUniqueTemplate(list, TemplateReact)
		list = appendUniqueTemplate(list, TemplateNext)
		list = appendUniqueTemplate(list, TemplateExpress)
		// Inserting duplicates should not change order.
		list = appendUniqueTemplate(list, TemplateReact)
		list = appendUniqueTemplate(list, TemplateNext)
		if len(list) != 3 {
			t.Fatalf("len = %d, want 3", len(list))
		}
		if list[0] != TemplateReact || list[1] != TemplateNext || list[2] != TemplateExpress {
			t.Errorf("order not preserved: %v", list)
		}
	})
}

// ---------------------------------------------------------------------------
// Template constants — guard against accidental renames
// ---------------------------------------------------------------------------

func TestTemplateConstants(t *testing.T) {
	t.Parallel()

	// These values are used in CLI args and config files — renaming breaks UX.
	consts := map[Template]string{
		TemplateReact:     "react",
		TemplateNext:      "next",
		TemplateExpress:   "express",
		TemplatePlain:     "plain",
		TemplateBlog:      "blog",
		TemplateKanban:    "kanban",
		TemplateEcommerce: "ecommerce",
		TemplatePolls:     "polls",
		TemplateChat:      "chat",
	}
	for c, want := range consts {
		if string(c) != want {
			t.Errorf("Template constant = %q, want %q", c, want)
		}
	}
}
