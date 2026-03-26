# Packaging Workflow

This directory contains reference manifests for package manager distribution.

## Versioning and Placeholders

- Replace `0.0.0` with the release version when preparing a manual submission.
- Replace placeholder checksums and Nix hashes with real values from release artifacts.
- Public release artifact URLs follow:
  - `https://github.com/gridlhq-staging/allyourbase/releases/download/vVERSION/ayb_VERSION_OS_ARCH.tar.gz`
  - Windows archives use `.zip`.

## winget (manual submit)

1. Update `winget/allyourbase.ayb.yaml` placeholders for the target release.
2. Generate or validate installer SHA256 values.
3. Submit with `wingetcreate submit`.

## Scoop (bucket submit)

1. Keep `scoop/ayb.json` as a manual reference template.
2. Update `version`, architecture URLs, and hashes.
3. Open a PR against the Scoop bucket repository (for example `gridlhq/scoop-bucket`).

Note: goreleaser also auto-generates Scoop metadata during release via `.goreleaser.yaml`.

## Nix (nixpkgs PR)

1. Update `nix/default.nix` version and hash placeholders.
2. Compute `vendorHash` and source hash from the target release source.
3. Submit a PR to nixpkgs with the derivation update.

## Homebrew

Homebrew publishing is already automated via goreleaser/release automation and the tap workflow.
