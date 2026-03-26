package codehealth

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const maxFunctionLines = 100

// Baseline allowlist for existing functions at or above maxFunctionLines.
// Entries should only be removed when functions are shortened.
// Do not add new entries without review.
var functionSizeAllowlist = map[string]int{
	// Batch mutation handlers keep SQL clause assembly and result shaping together
	// to preserve shared transactional behavior and error semantics.
	"api.Handler.execBatchOp":  127,
	"api.Handler.handleBatch":  100,
	"api.Handler.handleUpdate": 100,
	"ai.CompactSchemaContext":  109,
	// Tokenizer remains monolithic to keep lexer state transitions explicit.
	"api.tokenize":                     134,
	"auth.Service.VerifyTOTPChallenge": 108,
	"auth.init":                        111,
	"cli.flyProvider.Deploy":           111,
	"cli.runFunctionsDeploy":           114,
	"cli.runFunctionsInvoke":           119,
	"cli.runPushListDevices":           100,
	"cli.runQuery":                     134,
	"cli.runStartDetached":             113,
	"cli.runStartForeground":           103,
	"cli.runUninstall":                 139,
	"cli.showTableDetail":              104,
	// Default config wiring is an explicit stage non-goal for extraction.
	"config.Default":      185,
	"config.applyAuthEnv": 124,
	"main.run":            103,
	// Recursive GraphQL where-clause lowering is intentionally centralized.
	"graphql.resolveWhere":              148,
	"nhostmigrate.Migrator.buildPlan":   102,
	"pgmanager.Manager.Start":           117,
	"sbmigrate.Migrator.migrateData":    102,
	"sbmigrate.Migrator.migrateSchema":  113,
	"sbmigrate.Migrator.migrateStorage": 116,
	"schemadiff.FromSchemaCache":        101,
	"server.handleEdgeFuncInvoke":       112,
	"storage.Handler.HandleUpload":      114,
	// Template-heavy schema/client generators are kept contiguous for readability
	// of emitted output and to avoid fragmented string assembly wrappers.
	"templates.blogTemplate.Schema":          110,
	"templates.chatTemplate.ClientCode":      139,
	"templates.chatTemplate.Schema":          200,
	"templates.ecommerceTemplate.ClientCode": 141,
	"templates.ecommerceTemplate.Schema":     217,
	"templates.kanbanTemplate.ClientCode":    117,
	"templates.kanbanTemplate.Schema":        161,
	"templates.pollsTemplate.Schema":         111,
}

type oversizedFunction struct {
	Key       string
	LineCount int
	Path      string
}

func TestFunctionSizeAllowlist(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	oversizedFunctions := collectOversizedFunctions(t, repoRoot, maxFunctionLines)

	unallowlisted := make([]oversizedFunction, 0)
	staleCounts := make([]string, 0)
	present := make(map[string]struct{}, len(oversizedFunctions))

	for _, item := range oversizedFunctions {
		present[item.Key] = struct{}{}
		allowlistedCount, exists := functionSizeAllowlist[item.Key]
		if !exists {
			unallowlisted = append(unallowlisted, item)
			continue
		}
		if allowlistedCount != item.LineCount {
			staleCounts = append(staleCounts, fmt.Sprintf("%s has %d lines (allowlist has %d)", item.Key, item.LineCount, allowlistedCount))
		}
	}

	staleAllowlistEntries := make([]string, 0)
	for key := range functionSizeAllowlist {
		if _, exists := present[key]; !exists {
			staleAllowlistEntries = append(staleAllowlistEntries, key)
		}
	}

	if len(unallowlisted) == 0 && len(staleCounts) == 0 && len(staleAllowlistEntries) == 0 {
		return
	}

	sort.Slice(unallowlisted, func(i, j int) bool { return unallowlisted[i].Key < unallowlisted[j].Key })
	sort.Strings(staleCounts)
	sort.Strings(staleAllowlistEntries)

	var message strings.Builder
	message.WriteString("Function size guardrail violations:\n")
	if len(unallowlisted) > 0 {
		message.WriteString("Unallowlisted oversized functions:\n")
		for _, item := range unallowlisted {
			message.WriteString(fmt.Sprintf("- %s (%d lines) [%s]\n", item.Key, item.LineCount, item.Path))
		}
	}
	if len(staleCounts) > 0 {
		message.WriteString("Allowlist entries with stale line counts:\n")
		for _, item := range staleCounts {
			message.WriteString("- " + item + "\n")
		}
	}
	if len(staleAllowlistEntries) > 0 {
		message.WriteString("Allowlist entries no longer oversized:\n")
		for _, item := range staleAllowlistEntries {
			message.WriteString("- " + item + "\n")
		}
	}

	t.Fatal(message.String())
}

func collectOversizedFunctions(t *testing.T, repoRoot string, lineLimit int) []oversizedFunction {
	t.Helper()

	fileSet := token.NewFileSet()
	functions := make([]oversizedFunction, 0)

	if err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)

		if entry.IsDir() {
			if shouldSkipCodehealthDir(relativePath) {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldSkipCodehealthFile(relativePath) {
			return nil
		}

		if !strings.HasSuffix(relativePath, ".go") || strings.HasSuffix(relativePath, "_test.go") {
			return nil
		}

		parsedFile, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relativePath, err)
		}

		for _, declaration := range parsedFile.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}

			startLine := fileSet.Position(function.Body.Lbrace).Line
			endLine := fileSet.Position(function.Body.Rbrace).Line
			functionLineCount := endLine - startLine + 1
			if functionLineCount < lineLimit {
				continue
			}

			functions = append(functions, oversizedFunction{
				Key:       buildFunctionKey(parsedFile.Name.Name, function),
				LineCount: functionLineCount,
				Path:      relativePath,
			})
		}

		return nil
	}); err != nil {
		t.Fatalf("collect oversized functions failed: %v", err)
	}

	return functions
}

func buildFunctionKey(packageName string, function *ast.FuncDecl) string {
	functionName := packageName + "." + function.Name.Name
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return functionName
	}

	receiverName := "Unknown"
	switch receiverType := function.Recv.List[0].Type.(type) {
	case *ast.Ident:
		receiverName = receiverType.Name
	case *ast.StarExpr:
		if receiverIdent, ok := receiverType.X.(*ast.Ident); ok {
			receiverName = receiverIdent.Name
		}
	}

	return packageName + "." + receiverName + "." + function.Name.Name
}

func shouldSkipCodehealthDir(relativePath string) bool {
	if relativePath == "." {
		return false
	}

	skipped := []string{
		".git",
		"_dev",
		"docs-site",
		"examples",
		"sdk",
		"ui",
		"vendor",
	}
	for _, prefix := range skipped {
		if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
			return true
		}
	}
	return false
}

func shouldSkipCodehealthFile(relativePath string) bool {
	return strings.HasPrefix(relativePath, "vendor/")
}
