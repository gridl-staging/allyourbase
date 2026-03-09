feel free to use any pure bash commands for interacting with my filesyste.  and feel free to run multiple bash commands a time like for reading mulitple files or sections of files


## Go workflow rules
- After writing new .go files with new imports, run `go mod tidy` before `go test`
- When reading multiple independent files (e.g. impl files for a stage), read them all in parallel



never sign git commit messages as being from claude or anthropic

no quick fixes! no bandaids! always do the correct and professional DRY appraoch.  refactor as needed 


../allyourbase_dev/_dev/PROJECT_PROPOSAL.md

../allyourbase_dev/_dev/BROWSER_TESTING_STANDARDS_2.md



## Validation commands (run after edits)

You MUST run the relevant checks below after every code change, even for seemingly simple edits:

### Go
```bash
# Vet the package you changed
go vet ./path/to/package/...

# Run related tests (single package, single test)
go test ./path/to/package/... -run TestName -v -count=1

# Check formatting
gofmt -l path/to/file.go
```

### TypeScript (ui/ and sdk/)
```bash
# Type check
cd ui && npx tsc --noEmit
cd sdk && npx tsc --noEmit
```

## Permissions
Allowed without asking: read files, go vet, gofmt, tsc --noEmit, run single package tests
Ask first: go mod tidy, npm install, git push, deleting files, full test suite
