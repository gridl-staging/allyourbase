# GitHub Actions Version Bump Research — Findings

**Date**: 2026-03-30
**Purpose**: Verify target versions for all action references and identify breaking input changes before applying mechanical version bumps across 6 workflow files.
**Method**: Queried GitHub Releases API (`gh api repos/<owner>/<repo>/releases/latest` and selected `releases/tags/<tag>`) on 2026-03-30 for version truth and changelog sources.

## 1. GitHub-Owned Actions — Latest Versions Confirmed

All target versions from the stage overview are confirmed as the current latest majors.

| Action | Current | Target | Latest Release (source) | Occurrences | Target Correct? |
|--------|---------|--------|-------------------------|-------------|-----------------|
| `actions/checkout` | v4 | v6 | [`v6.0.2`](https://github.com/actions/checkout/releases/tag/v6.0.2) | 20 | ✅ |
| `actions/setup-node` | v4 | v6 | [`v6.3.0`](https://github.com/actions/setup-node/releases/tag/v6.3.0) | 11 | ✅ |
| `actions/setup-go` | v5 | v6 | [`v6.4.0`](https://github.com/actions/setup-go/releases/tag/v6.4.0) | 8 | ✅ |
| `actions/upload-artifact` | v4 | v7 | [`v7.0.0`](https://github.com/actions/upload-artifact/releases/tag/v7.0.0) | 5 | ✅ |
| `actions/setup-python` | v5 | v6 | [`v6.2.0`](https://github.com/actions/setup-python/releases/tag/v6.2.0) | 1 | ✅ |
| `actions/setup-java` | v4 | v5 | [`v5.2.0`](https://github.com/actions/setup-java/releases/tag/v5.2.0) | 1 | ✅ |
| `actions/download-artifact` | v4 | v8 | [`v8.0.1`](https://github.com/actions/download-artifact/releases/tag/v8.0.1) | 1 | ✅ |

**Total GitHub-owned references: 47** (matches checklist)

## 2. Third-Party Actions — Latest Versions

| Action | Current | Latest Release (source) | New Major? | Target |
|--------|---------|--------------------------|------------|--------|
| `golangci/golangci-lint-action` | v7 | [`v9.2.0`](https://github.com/golangci/golangci-lint-action/releases/tag/v9.2.0) | ✅ v7→v9 | **v9** |
| `docker/setup-buildx-action` | v3 | [`v4.0.0`](https://github.com/docker/setup-buildx-action/releases/tag/v4.0.0) | ✅ v3→v4 | **v4** |
| `docker/login-action` | v3 | [`v4.0.0`](https://github.com/docker/login-action/releases/tag/v4.0.0) | ✅ v3→v4 | **v4** |
| `docker/metadata-action` | v5 | [`v6.0.0`](https://github.com/docker/metadata-action/releases/tag/v6.0.0) | ✅ v5→v6 | **v6** |
| `docker/build-push-action` | v6 | [`v7.0.0`](https://github.com/docker/build-push-action/releases/tag/v7.0.0) | ✅ v6→v7 | **v7** |
| `goreleaser/goreleaser-action` | v6 | [`v7.0.0`](https://github.com/goreleaser/goreleaser-action/releases/tag/v7.0.0) | ✅ v6→v7 | **v7** |
| `softprops/action-gh-release` | v2 | [`v2.6.1`](https://github.com/softprops/action-gh-release/releases/tag/v2.6.1) | ❌ still v2 | no bump |
| `cloudflare/wrangler-action` | v3 | [`v3.14.1`](https://github.com/cloudflare/wrangler-action/releases/tag/v3.14.1) | ❌ still v3 | no bump |
| `dart-lang/setup-dart` | v1 | [`v1.7.2`](https://github.com/dart-lang/setup-dart/releases/tag/v1.7.2) | ❌ still v1 | no bump |

**Third-party references: 9 total** (6 to bump, 3 stay as-is)

## 3. Breaking Changes Analysis (Input Compatibility)

Breaking-change and behavior notes below are sourced from major-release notes for each action.

### 3.1 actions/checkout v4→v6

Sources: [`v5.0.0`](https://github.com/actions/checkout/releases/tag/v5.0.0), [`v6.0.0`](https://github.com/actions/checkout/releases/tag/v6.0.0)

- v5: runtime move to Node 24; runner minimum is raised.
- v6: credential persistence internals changed to separate-file storage.
- `persist-credentials` remains a supported input in the action interface.

**Our usage**: many steps use `persist-credentials: false`. **No input changes needed.**

### 3.2 actions/setup-node v4→v6

Sources: [`v5.0.0`](https://github.com/actions/setup-node/releases/tag/v5.0.0), [`v6.0.0`](https://github.com/actions/setup-node/releases/tag/v6.0.0)

- v5 introduced broader automatic package-manager caching behavior.
- v6 limited automatic caching behavior to npm.

**Our usage**: 11 steps use `node-version: "20"`; one step uses explicit `cache: "npm"` and `cache-dependency-path`. Repository search found no `"packageManager"` key in `package.json`, so the auto-caching behavior does not trigger. **No input changes needed.**

### 3.3 actions/setup-go v5→v6

Source: [`v6.0.0`](https://github.com/actions/setup-go/releases/tag/v6.0.0)

- v6 updated toolchain handling semantics and runtime.

**Our usage**: 8 steps use `go-version: "1.25"`. Input name remains valid. **No input changes needed.**

### 3.4 actions/upload-artifact v4→v7

Source: [`v7.0.0`](https://github.com/actions/upload-artifact/releases/tag/v7.0.0)

- v7 adds optional archive behavior and runtime/module updates.

**Our usage**: 5 steps use `name` and `path`. Existing inputs remain valid. **No input changes needed.**

### 3.5 actions/download-artifact v4→v8

Source: [`v8.0.0`](https://github.com/actions/download-artifact/releases/tag/v8.0.0)

- v8 changes digest-mismatch behavior to fail by default and adds `skip-decompress`.

**Our usage**: 1 step uses only `path: dist/pg-binaries`; no `artifact-ids`. New digest default is acceptable safety hardening. **No input changes needed.**

### 3.6 actions/setup-python v5→v6

Source: [`v6.0.0`](https://github.com/actions/setup-python/releases/tag/v6.0.0)

- v6 runtime update; adds optional `pip-version`.

**Our usage**: 1 step uses `python-version: "3.12"`. **No input changes needed.**

### 3.7 actions/setup-java v4→v5

Source: [`v5.0.0`](https://github.com/actions/setup-java/releases/tag/v5.0.0)

- v5 runtime update and bug fixes.

**Our usage**: 1 step uses `distribution: temurin` and `java-version: "17"`. **No input changes needed.**

### 3.8 golangci/golangci-lint-action v7→v9

Sources: [`v8.0.0`](https://github.com/golangci/golangci-lint-action/releases/tag/v8.0.0), [`v9.0.0`](https://github.com/golangci/golangci-lint-action/releases/tag/v9.0.0)

- v8 requires golangci-lint v2 and updates path handling behavior.
- v9 updates runtime and adds optional install-only behavior.

**Our usage**: workflow uses `version: latest`; repository `.golangci.yml` is already v2 format. **No input changes needed.**

### 3.9 docker/setup-buildx-action v3→v4

Source: [`v4.0.0`](https://github.com/docker/setup-buildx-action/releases/tag/v4.0.0)

- v4 removed deprecated parameters and updated runtime.

**Our usage**: no custom inputs. **No input changes needed.**

### 3.10 docker/login-action v3→v4

Source: [`v4.0.0`](https://github.com/docker/login-action/releases/tag/v4.0.0)

- v4 runtime/module update.

**Our usage**: `registry`, `username`, `password` remain valid. **No input changes needed.**

### 3.11 docker/metadata-action v5→v6

Source: [`v6.0.0`](https://github.com/docker/metadata-action/releases/tag/v6.0.0)

- v6 changes parsing around `#` in list values and updates runtime.

**Our usage**: `images` and `tags` patterns contain no `#`. **No input changes needed.**

### 3.12 docker/build-push-action v6→v7

Source: [`v7.0.0`](https://github.com/docker/build-push-action/releases/tag/v7.0.0)

- v7 removed deprecated legacy summary env vars and updated runtime/module.

**Our usage**: action inputs only (`context`, `push`, `tags`, `labels`, `cache-from`, `cache-to`) and no removed env vars. **No input changes needed.**

### 3.13 goreleaser/goreleaser-action v6→v7

Source: [`v7.0.0`](https://github.com/goreleaser/goreleaser-action/releases/tag/v7.0.0)

- v7 runtime/module update; no required input migration.

**Our usage**: `version: latest`, `args: release --clean`. **No input changes needed.**

## 4. Summary — Actions Requiring Input Changes

**None.** All 13 version bumps (7 GitHub-owned + 6 third-party) are safe drop-in replacements. No `with:` block modifications needed.

## 5. Final Target Version Map for Implementation

### GitHub-Owned (sed replacements)
```text
actions/checkout@v4          -> actions/checkout@v6
actions/setup-node@v4        -> actions/setup-node@v6
actions/setup-go@v5          -> actions/setup-go@v6
actions/upload-artifact@v4   -> actions/upload-artifact@v7
actions/setup-python@v5      -> actions/setup-python@v6
actions/setup-java@v4        -> actions/setup-java@v5
actions/download-artifact@v4 -> actions/download-artifact@v8
```

### Third-Party (sed replacements)
```text
golangci/golangci-lint-action@v7 -> golangci/golangci-lint-action@v9
docker/setup-buildx-action@v3    -> docker/setup-buildx-action@v4
docker/login-action@v3           -> docker/login-action@v4
docker/metadata-action@v5        -> docker/metadata-action@v6
docker/build-push-action@v6      -> docker/build-push-action@v7
goreleaser/goreleaser-action@v6  -> goreleaser/goreleaser-action@v7
```

### No Bump Needed
```text
softprops/action-gh-release@v2 — latest major still v2
cloudflare/wrangler-action@v3  — latest major still v3
dart-lang/setup-dart@v1        — latest major still v1
```

## 6. Input Compatibility Verification (action.yml Metadata)

Every input our workflows use was verified to exist in the target-version `action.yml` by fetching it
from GitHub at the target tag: `gh api 'repos/<owner>/<repo>/contents/action.yml?ref=<target-tag>'`.
Collected on 2026-03-30.

### 6.1 GitHub-Owned Actions

| Action@Target | Our Input(s) | Present in action.yml? | Evidence |
|---------------|-------------|------------------------|----------|
| `checkout@v6` | `persist-credentials` | Yes | `persist-credentials:` with description "Whether to configure the token or SSH key with the local git config", default: `true` |
| `setup-node@v6` | `node-version`, `cache`, `cache-dependency-path` | Yes (all three) | `node-version:` (description: "Version Spec…"), `cache-dependency-path:` (description: "Used to specify the path to a dependency file…") |
| `setup-go@v6` | `go-version` | Yes | `go-version:` with description "The Go version to download (if necessary) and use." |
| `setup-python@v6` | `python-version` | Yes | `python-version:` with description "Version range or exact version of Python or PyPy…" |
| `setup-java@v5` | `distribution`, `java-version` | Yes (both) | `distribution:` (required: true, description: "Java distribution…"), `java-version:` (description: "The Java version to set up…") |
| `upload-artifact@v7` | `name`, `path` | Yes (both) | `name:` (default: "artifact"), `path:` (required: true, description: "A file, directory or wildcard pattern…") |
| `download-artifact@v8` | `path` | Yes | `path:` (required: false, description: "Destination path. Supports basic tilde expansion.") |

### 6.2 Third-Party Actions

| Action@Target | Our Input(s) | Present in action.yml? | Evidence |
|---------------|-------------|------------------------|----------|
| `golangci-lint-action@v9` | `version`, `args` | Yes (both) | `version:` (description: "The version of golangci-lint to use."), `args:` (description: "golangci-lint command line arguments.") |
| `docker/login-action@v4` | `registry`, `username`, `password` | Yes (all three) | `registry:` (required: false), `username:` (required: false), `password:` (required: false) |
| `docker/setup-buildx-action@v4` | _(none — no `with:` block)_ | N/A | `inputs:` section exists; no custom inputs used |
| `docker/metadata-action@v6` | `images`, `tags` | Yes (both) | `images:` (required: false, description: "List of Docker images…"), `tags:` (required: false, description: "List of tags as key-value pair attributes") |
| `docker/build-push-action@v7` | `context`, `push`, `tags`, `labels`, `cache-from`, `cache-to` | Yes (all six) | All present as optional inputs with descriptions matching v6 semantics |
| `goreleaser/goreleaser-action@v7` | `version`, `args` | Yes (both) | `version:` (default: "~> v2"), `args:` (required: false) |

**Method**: `gh api 'repos/<owner>/<repo>/contents/action.yml?ref=<tag>' --jq '.content' | base64 -d | grep -A2 '<input-name>'`

**Conclusion**: All 13 bumped actions retain every input our workflows reference. Zero `with:` block changes required.

## 7. Old-Version Tag Elimination Proof

Verified at HEAD (`baeb4126`, clean working tree) on 2026-03-30 that zero old-version tags remain in workflow YAML files.

**Command** (action-scoped grep covering all 13 pre-bump version tags):
```bash
grep -RInE \
  'uses:\s*actions/checkout@v4|uses:\s*actions/setup-node@v4|uses:\s*actions/setup-go@v5|uses:\s*actions/upload-artifact@v4|uses:\s*actions/setup-python@v5|uses:\s*actions/setup-java@v4|uses:\s*actions/download-artifact@v4|uses:\s*golangci/golangci-lint-action@v7|uses:\s*docker/setup-buildx-action@v3|uses:\s*docker/login-action@v3|uses:\s*docker/metadata-action@v5|uses:\s*docker/build-push-action@v6|uses:\s*goreleaser/goreleaser-action@v6' \
  .github/workflows/*.yml
```

**Result**: Exit code 1 (no matches). All 53 `uses:` lines in the 6 workflow files reference the target versions only.

## 8. Open Questions

None.

## 9. Evidence Snippets (API)

Collected on 2026-03-30 via GitHub CLI:

```bash
gh api repos/actions/setup-go/releases/latest --jq '.tag_name, .html_url'
# v6.4.0
# https://github.com/actions/setup-go/releases/tag/v6.4.0

gh api repos/docker/build-push-action/releases/latest --jq '.tag_name, .html_url'
# v7.0.0
# https://github.com/docker/build-push-action/releases/tag/v7.0.0

gh api repos/actions/download-artifact/releases/tags/v8.0.0 --jq '.html_url'
# https://github.com/actions/download-artifact/releases/tag/v8.0.0
```
