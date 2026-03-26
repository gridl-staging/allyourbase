package ai

import (
	"fmt"
	"regexp"
	"strings"
)

var templateVarRe = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// RenderPrompt substitutes {{variable}} placeholders in template with values from variables.
func RenderPrompt(template string, variables map[string]any) (string, error) {
	var missing []string

	result := templateVarRe.ReplaceAllStringFunc(template, func(match string) string {
		name := match[2 : len(match)-2] // strip {{ and }}
		val, ok := variables[name]
		if !ok {
			missing = append(missing, name)
			return match
		}
		return fmt.Sprint(val)
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("missing required template variables: %s", strings.Join(missing, ", "))
	}

	return result, nil
}

// ValidatePromptVariables checks that all required variables (from the prompt spec) are present.
func ValidatePromptVariables(spec []PromptVariable, provided map[string]any) error {
	for _, v := range spec {
		if !v.Required {
			continue
		}
		if _, ok := provided[v.Name]; !ok {
			if v.Default == "" {
				return fmt.Errorf("required variable %q is missing", v.Name)
			}
		}
	}
	return nil
}

// ApplyDefaults fills in default values for missing variables.
func ApplyDefaults(spec []PromptVariable, provided map[string]any) map[string]any {
	result := make(map[string]any, len(provided))
	for k, v := range provided {
		result[k] = v
	}
	for _, v := range spec {
		if _, ok := result[v.Name]; !ok && v.Default != "" {
			result[v.Name] = v.Default
		}
	}
	return result
}
