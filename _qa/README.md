# Allyourbase Exploratory QA Suite

**Created:** 2026-02-21
**Purpose:** Standalone automated manual exploratory QA to validate the entire Allyourbase (AYB) experience from a new user's perspective — before public release.

This is NOT part of the project's test suite. These are standalone, isolated QA scripts that simulate a brand-new user discovering AYB for the first time. They test the CLI, the dashboard, the demo apps, the REST API, and the overall UX.

## Test Categories

### Bash Scripts (`scripts/`)

| Script | What it tests |
|--------|--------------|
| `01_cli_help_and_version.sh` | Every CLI command's `--help` output — formatting, grammar, completeness |
| `02_server_lifecycle.sh` | Start/stop/status cycle, startup timing, health checks, PID management |
| `03_api_crud.sh` | Full CRUD lifecycle via REST API — create table, insert, read, update, delete, filter, sort, paginate |
| `04_auth_flow.sh` | User registration, login, JWT token usage, /me endpoint, refresh tokens |
| `05_dashboard_api.sh` | Admin login, SQL editor, schema inspection, user listing via admin API |
| `06_demo_launch.sh` | Demo app launch, schema application, seed users, demo frontend serving |
| `07_cli_data_commands.sh` | `ayb sql`, `ayb schema`, `ayb query`, `ayb config` — output formatting and accuracy |

### Playwright Scripts (`playwright/`)

| Script | What it tests |
|--------|--------------|
| `dashboard_explore.spec.ts` | Full dashboard walkthrough — every sidebar item, every page, screenshots |
| `demo_live_polls.spec.ts` | Live-polls demo: register, login, create poll, vote, view results |
| `demo_kanban.spec.ts` | Kanban demo: register, login, create board, add cards, drag-drop |

## Running

```bash
# Run all bash QA tests (server should not be running)
cd _qa && bash scripts/run_all.sh

# Run Playwright tests (server must be running)
cd _qa/playwright && npx playwright test

# Run individual scripts
bash scripts/01_cli_help_and_version.sh
```

## Results

All results are written to `results/`:
- `results/*.log` — test output logs
- `results/screenshots/` — Playwright screenshots of every dashboard page and demo app page
- `results/defects.md` — automatically generated defect report
