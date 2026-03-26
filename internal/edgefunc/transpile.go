package edgefunc

import (
	"fmt"

	"github.com/evanw/esbuild/pkg/api"
)

// entryPointExportBridge returns JS that exports a named function to global scope
// after IIFE execution, so the Goja runtime can discover it.
func entryPointExportBridge(entryPoint string) string {
	return fmt.Sprintf(`
;if (typeof %s !== "undefined") {
  if (typeof globalThis !== "undefined") {
    globalThis.%s = %s;
  } else {
    Function("return this")().%s = %s;
  }
}
`, entryPoint, entryPoint, entryPoint, entryPoint, entryPoint)
}

// Transpile converts TypeScript source into ES2015 JavaScript for Goja execution.
// Uses the esbuild Go API directly (no subprocess). JavaScript input is returned
// unchanged. entryPoint is the function name to export to global scope (defaults
// to "handler" if empty).
func Transpile(source string, isTS bool, entryPoint string) (string, error) {
	if !isTS {
		return source, nil
	}

	if entryPoint == "" {
		entryPoint = "handler"
	}

	input := source + entryPointExportBridge(entryPoint)

	result := api.Transform(input, api.TransformOptions{
		Loader:            api.LoaderTS,
		Target:            api.ES2015,
		Format:            api.FormatIIFE,
		TreeShaking:       api.TreeShakingFalse,
		Sourcefile:        "edge-function.ts",
		LogLevel:          api.LogLevelSilent,
		LegalComments:     api.LegalCommentsNone,
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
	})

	if len(result.Errors) > 0 {
		msg := result.Errors[0]
		text := msg.Text
		if msg.Location != nil {
			text = fmt.Sprintf("%s (%d:%d)", text, msg.Location.Line, msg.Location.Column)
		}
		return "", fmt.Errorf("transpile failed: %s", text)
	}

	return string(result.Code), nil
}
