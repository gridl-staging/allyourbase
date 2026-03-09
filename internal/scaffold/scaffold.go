// Package scaffold generates project boilerplate for new AYB-backed apps.
// It creates configuration, schema, SDK client, and context files.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Template represents a project template type.
type Template string

const (
	TemplateReact   Template = "react"
	TemplateNext    Template = "next"
	TemplateExpress Template = "express"
	TemplatePlain   Template = "plain"
)

// ValidTemplates returns all valid template names.
func ValidTemplates() []Template {
	return []Template{TemplateReact, TemplateNext, TemplateExpress, TemplatePlain}
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

func aybToml(opts Options) string {
	return `[server]
host = "0.0.0.0"
port = 8090

[database]
# Leave empty for managed Postgres (zero-config dev mode)
# url = "postgresql://user:pass@localhost:5432/mydb"

[auth]
enabled = true

[storage]
enabled = true
backend = "local"

[admin]
enabled = true
`
}

func schemaSQLFile() string {
	return `-- AYB Schema
-- Run with: psql $DATABASE_URL -f schema.sql
-- Or paste into the admin SQL editor at http://localhost:8090/admin

-- Example: users table with RLS
CREATE TABLE IF NOT EXISTS items (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    description TEXT,
    owner_id   UUID REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enable Row-Level Security
ALTER TABLE items ENABLE ROW LEVEL SECURITY;

-- Users can only see their own items
CREATE POLICY items_select ON items FOR SELECT
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);

-- Users can only insert items they own
CREATE POLICY items_insert ON items FOR INSERT
    WITH CHECK (owner_id = current_setting('ayb.user_id', true)::uuid);

-- Users can only update their own items
CREATE POLICY items_update ON items FOR UPDATE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);

-- Users can only delete their own items
CREATE POLICY items_delete ON items FOR DELETE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);
`
}

func envFile() string {
	return `# AYB environment variables
# Copy to .env.local for overrides

# Server
AYB_SERVER_PORT=8090

# Database (leave empty for managed Postgres)
# AYB_DATABASE_URL=postgresql://user:pass@localhost:5432/mydb

# Auth
AYB_AUTH_ENABLED=true
# AYB_AUTH_JWT_SECRET=  # auto-generated if not set

# Admin
AYB_ADMIN_ENABLED=true
# AYB_ADMIN_PASSWORD=  # set for admin dashboard access
`
}

func gitignoreFile(tmpl Template) string {
	base := `node_modules/
dist/
.env.local
.env.*.local
*.log
.DS_Store
`
	switch tmpl {
	case TemplateNext:
		base += ".next/\n"
	}
	return base
}

func claudeMD(opts Options) string {
	return fmt.Sprintf(`# %s

Built with [Allyourbase](https://allyourbase.io) — Backend-as-a-Service for PostgreSQL.

## Quick Start

`+"```"+`bash
# Start AYB (managed Postgres, zero config)
ayb start

# Apply schema
ayb sql < schema.sql

# Generate TypeScript types
ayb types typescript -o src/types/ayb.d.ts
`+"```"+`

## API Reference

- **REST API**: http://localhost:8090/api
- **Admin Dashboard**: http://localhost:8090/admin
- **API Schema**: http://localhost:8090/api/schema

## AYB SDK

`+"```"+`ts
import { AYBClient } from "@allyourbase/js";
const ayb = new AYBClient("http://localhost:8090");

// List records
const { items } = await ayb.records.list("items", { filter: "published=true" });

// CRUD
const item = await ayb.records.create("items", { name: "New Item" });
await ayb.records.update("items", item.id, { name: "Updated" });
await ayb.records.delete("items", item.id);

// Auth
await ayb.auth.login("user@example.com", "password");
const me = await ayb.auth.me();
`+"```"+`
`, opts.Name)
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

// --- Shared content generators ---

func packageJSON(opts Options, tmpl string) string {
	name := strings.ToLower(opts.Name)

	switch tmpl {
	case "react":
		return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "@allyourbase/js": "^0.1.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.0.0",
    "vite": "^6.0.0"
  }
}
`, name)
	case "next":
		return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start"
  },
  "dependencies": {
    "@allyourbase/js": "^0.1.0",
    "next": "^15.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "typescript": "^5.0.0"
  }
}
`, name)
	case "express":
		return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "tsx watch src/index.ts",
    "build": "tsc",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "@allyourbase/js": "^0.1.0"
  },
  "devDependencies": {
    "tsx": "^4.0.0",
    "typescript": "^5.0.0"
  }
}
`, name)
	default: // plain
		return fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "tsx watch src/index.ts",
    "build": "tsc",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "@allyourbase/js": "^0.1.0"
  },
  "devDependencies": {
    "tsx": "^4.0.0",
    "typescript": "^5.0.0"
  }
}
`, name)
	}
}

func aybClient() string {
	return `import { AYBClient } from "@allyourbase/js";

const AYB_URL = import.meta.env.VITE_AYB_URL || "http://localhost:8090";

export const ayb = new AYBClient(AYB_URL);

// Persist auth tokens to localStorage
const TOKENS_KEY = "ayb_tokens";

export function persistTokens(token: string, refreshToken: string) {
  localStorage.setItem(TOKENS_KEY, JSON.stringify({ token, refreshToken }));
  ayb.setToken(token);
}

export function clearPersistedTokens() {
  localStorage.removeItem(TOKENS_KEY);
  ayb.setToken("");
}

export function isLoggedIn(): boolean {
  const stored = localStorage.getItem(TOKENS_KEY);
  if (!stored) return false;
  try {
    const { token } = JSON.parse(stored);
    ayb.setToken(token);
    return true;
  } catch {
    return false;
  }
}
`
}

func aybClientNode() string {
	return `import { AYBClient } from "@allyourbase/js";

const AYB_URL = process.env.AYB_URL || "http://localhost:8090";

export const ayb = new AYBClient(AYB_URL);
`
}

func tsConfigJSON() string {
	return `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"]
}
`
}

func viteConfig() string {
	return `import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
});
`
}

func indexHTML(opts Options) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`, opts.Name)
}

func reactMain() string {
	return `import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
`
}

func reactApp() string {
	return `import { useEffect, useState } from "react";
import { ayb } from "./lib/ayb";

function App() {
  const [items, setItems] = useState<any[]>([]);
  const [status, setStatus] = useState("loading...");

  useEffect(() => {
    ayb.health()
      .then(() => setStatus("connected"))
      .catch(() => setStatus("disconnected — run 'ayb start'"));

    ayb.records
      .list("items")
      .then((res) => setItems(res.items))
      .catch(() => {});
  }, []);

  return (
    <div style={{ maxWidth: 600, margin: "2rem auto", fontFamily: "system-ui" }}>
      <h1>Welcome to your AYB app</h1>
      <p>
        Server: <strong>{status}</strong>
      </p>
      <h2>Items ({items.length})</h2>
      <ul>
        {items.map((item: any) => (
          <li key={item.id}>{item.name}</li>
        ))}
      </ul>
      <p style={{ color: "#888", fontSize: "0.9rem" }}>
        Edit <code>src/App.tsx</code> to get started.
        <br />
        Admin dashboard: <a href="http://localhost:8090/admin">localhost:8090/admin</a>
      </p>
    </div>
  );
}

export default App;
`
}

func minimalCSS() string {
	return `body {
  margin: 0;
  -webkit-font-smoothing: antialiased;
}
`
}

func nextTSConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2017",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
`
}

func nextConfig() string {
	return `/** @type {import('next').NextConfig} */
const nextConfig = {};
module.exports = nextConfig;
`
}

func nextLayout(opts Options) string {
	return fmt.Sprintf(`export const metadata = {
  title: "%s",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
`, opts.Name)
}

func nextPage() string {
	return `"use client";

import { useEffect, useState } from "react";
import { ayb } from "@/lib/ayb";

export default function Home() {
  const [items, setItems] = useState<any[]>([]);
  const [status, setStatus] = useState("loading...");

  useEffect(() => {
    ayb.health()
      .then(() => setStatus("connected"))
      .catch(() => setStatus("disconnected — run 'ayb start'"));

    ayb.records
      .list("items")
      .then((res) => setItems(res.items))
      .catch(() => {});
  }, []);

  return (
    <main style={{ maxWidth: 600, margin: "2rem auto", fontFamily: "system-ui" }}>
      <h1>Welcome to your AYB app</h1>
      <p>Server: <strong>{status}</strong></p>
      <h2>Items ({items.length})</h2>
      <ul>
        {items.map((item: any) => (
          <li key={item.id}>{item.name}</li>
        ))}
      </ul>
    </main>
  );
}
`
}

func expressTSConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "resolveJsonModule": true
  },
  "include": ["src"]
}
`
}

func expressMain() string {
	return `import { ayb } from "./lib/ayb";

async function main() {
  // Check AYB connection
  try {
    const health = await ayb.health();
    console.log("AYB server:", health.status);
  } catch (err) {
    console.error("Cannot connect to AYB. Run 'ayb start' first.");
    process.exit(1);
  }

  // List items
  const { items } = await ayb.records.list("items");
  console.log("Items:", items.length);
  for (const item of items) {
    console.log(" -", item.name);
  }
}

main();
`
}

func plainMain() string {
	return `import { ayb } from "./lib/ayb";

async function main() {
  // Check AYB connection
  try {
    const health = await ayb.health();
    console.log("AYB server:", health.status);
  } catch (err) {
    console.error("Cannot connect to AYB. Run 'ayb start' first.");
    process.exit(1);
  }

  // Example: list records
  const { items } = await ayb.records.list("items");
  console.log("Items:", items.length);
}

main();
`
}
