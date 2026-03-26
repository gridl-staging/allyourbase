// Package scaffold generates boilerplate configuration and code files for new AYB-backed projects across multiple template types including React, Next.js, Express, and plain Node.js.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allyourbase/ayb/internal/scaffold/templates"
)

// Template represents a project template type.
type Template string

const (
	TemplateReact     Template = "react"
	TemplateNext      Template = "next"
	TemplateExpress   Template = "express"
	TemplatePlain     Template = "plain"
	TemplateBlog      Template = "blog"
	TemplateKanban    Template = "kanban"
	TemplateEcommerce Template = "ecommerce"
	TemplatePolls     Template = "polls"
	TemplateChat      Template = "chat"
)

// ValidTemplates returns all valid template names.
func ValidTemplates() []Template {
	valid := []Template{TemplateReact, TemplateNext, TemplateExpress, TemplatePlain}
	for _, name := range templates.Names() {
		valid = appendUniqueTemplate(valid, Template(name))
	}
	return valid
}

// IsValidTemplate checks if a template name is valid.
func IsValidTemplate(name string) bool {
	for _, t := range ValidTemplates() {
		if string(t) == name {
			return true
		}
	}
	return false
}

func appendUniqueTemplate(existing []Template, candidate Template) []Template {
	for _, t := range existing {
		if t == candidate {
			return existing
		}
	}
	return append(existing, candidate)
}

// Options configures project scaffolding.
type Options struct {
	// Name is the project directory name.
	Name string
	// Template is the project template to use.
	Template Template
	// Dir is the parent directory (defaults to ".").
	Dir string
}

// Run creates the scaffolded project.
func Run(opts Options) error {
	if opts.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if opts.Dir == "" {
		opts.Dir = "."
	}

	projectDir := filepath.Join(opts.Dir, opts.Name)
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("directory %q already exists", projectDir)
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	files := generateFiles(opts)
	for path, content := range files {
		fullPath := filepath.Join(projectDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return nil
}

// generateFiles returns the file tree for a given template.
func generateFiles(opts Options) map[string]string {
	files := make(map[string]string)

	// Common files for all templates
	files["ayb.toml"] = aybToml(opts)
	files["schema.sql"] = schemaSQLFile()
	files[".env"] = envFile()
	files[".gitignore"] = gitignoreFile(opts.Template)
	files["CLAUDE.md"] = claudeMD(opts)

	if dt, ok := templates.Get(string(opts.Template)); ok {
		files["schema.sql"] = dt.Schema()
		files["seed.sql"] = dt.SeedData()
		for path, content := range dt.ClientCode() {
			files[path] = content
		}
		files["README.md"] = dt.Readme()
		addPlainFiles(files, opts)
		return files
	}

	// Template-specific files
	switch opts.Template {
	case TemplateReact:
		addReactFiles(files, opts)
	case TemplateNext:
		addNextFiles(files, opts)
	case TemplateExpress:
		addExpressFiles(files, opts)
	case TemplatePlain:
		addPlainFiles(files, opts)
	}

	return files
}

// --- Template-specific file generators ---

func addReactFiles(files map[string]string, opts Options) {
	files["package.json"] = packageJSON(opts, "react")
	files["tsconfig.json"] = tsConfigJSON()
	files["vite.config.ts"] = viteConfig()
	files["index.html"] = indexHTML(opts)
	files["src/main.tsx"] = reactMain()
	files["src/App.tsx"] = reactApp()
	files["src/lib/ayb.ts"] = aybClient()
	files["src/index.css"] = minimalCSS()
}

func addNextFiles(files map[string]string, opts Options) {
	files["package.json"] = packageJSON(opts, "next")
	files["tsconfig.json"] = nextTSConfig()
	files["next.config.js"] = nextConfig()
	files["src/app/layout.tsx"] = nextLayout(opts)
	files["src/app/page.tsx"] = nextPage()
	files["src/lib/ayb.ts"] = aybClient()
}

func addExpressFiles(files map[string]string, opts Options) {
	files["package.json"] = packageJSON(opts, "express")
	files["tsconfig.json"] = expressTSConfig()
	files["src/index.ts"] = expressMain()
	files["src/lib/ayb.ts"] = aybClientNode()
}

func addPlainFiles(files map[string]string, opts Options) {
	files["package.json"] = packageJSON(opts, "plain")
	files["src/index.ts"] = plainMain()
	files["src/lib/ayb.ts"] = aybClientNode()
}
