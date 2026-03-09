//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestEmailTemplatesMigrationConstraintsAndUniqueness(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	// Table exists
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_email_templates'
		)`).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_email_templates table should exist")

	// Valid insert
	var id string
	err = sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ('auth.password_reset', 'Reset: {{.AppName}}', '<p>Hello</p>')
		 RETURNING id`).Scan(&id)
	testutil.NoError(t, err)
	testutil.True(t, id != "", "insert should return a UUID")

	// Uniqueness: duplicate key rejected
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ('auth.password_reset', 'Dup', '<p>Dup</p>')`)
	testutil.True(t, err != nil, "duplicate template_key should be rejected")

	// Key format: valid multi-segment keys
	for _, validKey := range []string{
		"app.club_invite",
		"auth.email_verification",
		"notify.event_reminder",
		"a.b",
	} {
		_, err = sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
			 VALUES ($1, 'Subject', '<p>Body</p>')`, validKey)
		if err != nil {
			t.Fatalf("valid key %q should be accepted: %v", validKey, err)
		}
	}

	// Key format: invalid keys rejected
	for _, invalidKey := range []string{
		"singleword",    // no dot separator
		"",              // empty
		".leading.dot",  // starts with dot
		"UPPER.case",    // uppercase
		"has space.key", // space
		"1starts.num",   // starts with number
		"a.",            // trailing dot
		".a",            // leading dot only
	} {
		_, err = sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
			 VALUES ($1, 'Subject', '<p>Body</p>')`, invalidKey)
		testutil.True(t, err != nil, "invalid key %q should be rejected", invalidKey)
	}

	// Subject size limit: 1000 chars OK, 1001 rejected
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ('test.subject_ok', $1, '<p>Body</p>')`, strings.Repeat("a", 1000))
	if err != nil {
		t.Fatalf("1000-char subject should be accepted: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ('test.subject_too_long', $1, '<p>Body</p>')`, strings.Repeat("a", 1001))
	testutil.True(t, err != nil, "1001-char subject should be rejected")

	// HTML size limit: 256000 OK, 256001 rejected
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ('test.html_ok', 'Subject', $1)`, strings.Repeat("x", 256000))
	if err != nil {
		t.Fatalf("256000-char HTML should be accepted: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ('test.html_too_big', 'Subject', $1)`, strings.Repeat("x", 256001))
	testutil.True(t, err != nil, "256001-char HTML should be rejected")

	// Enabled defaults to true
	var enabled bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT enabled FROM _ayb_email_templates WHERE template_key = 'auth.password_reset'`).Scan(&enabled)
	testutil.NoError(t, err)
	testutil.True(t, enabled, "enabled should default to true")

	// Can toggle enabled
	_, err = sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_email_templates SET enabled = false WHERE template_key = 'auth.password_reset'`)
	testutil.NoError(t, err)
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT enabled FROM _ayb_email_templates WHERE template_key = 'auth.password_reset'`).Scan(&enabled)
	testutil.NoError(t, err)
	testutil.True(t, !enabled, "enabled should be togglable to false")
}
