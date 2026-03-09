# Handoff 084 (Review Stage Transition) - Stage 4 verification + Stage 5 checklist review

## What I did

1. **Verified Stage 4 completion**: Re-ran all focused Stage 4 test suites — all pass:
   - `go test ./internal/matview -count=1` → PASS
   - `go test ./internal/server -run '^TestHandleAdmin(.*Matview.*)$' -count=1` → PASS
   - `go test ./internal/cli -run '^TestMatviews' -count=1` → PASS
   - `go test ./internal/migrations -run 'TestMatviewMigrationSQLConstraints|TestMatviewMigrationConstraintsAndUniqueness' -count=1` → PASS

2. **Researched email template industry patterns**: Analyzed how PocketBase (settings-level overrides, Go templates with limited variables), Supabase (config.toml templates, Go Templates), and Appwrite (per-project templates with localization) handle custom email templates. Researched Go template SSTI (Server-Side Template Injection) attack vectors.

3. **Reviewed and rewrote Stage 5 checklist** with 10 corrections:
   - Dropped `is_system` column — system templates are embedded in binary via `//go:embed`, DB stores only custom overrides
   - Dropped `text_template` column — existing `stripHTML()` auto-generates plaintext, no BaaS exposes separate plaintext editors
   - Dropped `description` column — keys are self-describing
   - Added explicit SSTI prevention model — data passed to Execute is `map[string]string` only (no structs with methods), empty FuncMap, `missingkey=error`
   - Added template compilation on save to catch syntax errors immediately
   - Added render timeout (5s) to prevent pathological templates
   - Added `POST /api/admin/email/send` endpoint for arbitrary app emails (required by Sigil for club invites/event reminders)
   - Added "effective template" API pattern — GET returns custom override if exists+enabled, else built-in default with source flag
   - Added graceful degradation requirement — broken custom template falls back to built-in without breaking auth flows
   - Simplified variable model to flat `map[string]string` only (eliminates SSTI surface)

4. **Updated trackers**: input file (`_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md`) and stages file (`.mike/.../stages.md`)

## Key security finding

Go template SSTI is a real attack vector. When user-provided templates are executed with objects that have methods, attackers can chain method calls for RCE/file read. The mitigation is straightforward: pass only `map[string]string` (no methods to call) and register no custom FuncMap. This is safe because:
- `map[string]string` has no methods to exploit
- `html/template` auto-escapes values in HTML context
- `missingkey=error` prevents silent variable omission
- Empty FuncMap means no custom functions to abuse

## Files modified

### Modified
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_05_checklist.md` (full rewrite)
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/stages.md` (Stage 5 description updated)
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` (Stage 5 review note added)

### Created
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_084_review_stage_transition.md` (this file)

## What's next

1. Stage 5 implementation should start from the reviewed checklist at `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_05_checklist.md`
2. First implementation task: migration `026_ayb_email_templates.sql` + migration tests (TDD)
3. Then: `internal/emailtemplates` package with Store + Service (TDD)
4. Then: auth integration refactor to use template service
5. Then: admin API + CLI + dashboard
