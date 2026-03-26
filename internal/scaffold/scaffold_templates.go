// Package scaffold Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_4_scaffold_sdk_first_run_fix/allyourbase_dev/internal/scaffold/scaffold_templates.go.
package scaffold

import (
	"fmt"
	"strings"
)

// aybToml returns the default ayb.toml configuration file content with server, database, auth, storage, and admin settings.
func aybToml(opts Options) string {
	return `[server]
host = "127.0.0.1"
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

// schemaSQLFile returns the default PostgreSQL schema with an example items table and row-level security policies that restrict access by owner.
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

// envFile returns the .env template documenting AYB environment variables for server port, database URL, authentication, and admin settings.
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

// claudeMD returns the project CLAUDE.md documentation with quick start instructions, API reference links, and SDK usage examples.
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

// TODO: Document packageJSON.
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
	default: // express and plain
		return nodePackageJSON(name)
	}
}

// TODO: Document nodePackageJSON.
func nodePackageJSON(name string) string {
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

// TODO: Document aybClient.
func aybClient() string {
	return `import { AYBClient } from "@allyourbase/js";

const AYB_URL = import.meta.env.VITE_AYB_URL || "http://localhost:8090";

export const ayb = new AYBClient(AYB_URL);

// Keep auth tokens in memory by default. Persisting bearer tokens in
// localStorage makes XSS impact much worse for scaffolded browser apps.
export function setSessionTokens(token: string, refreshToken: string) {
  ayb.setTokens(token, refreshToken);
}

export function clearSessionTokens() {
  ayb.clearTokens();
}

export function isLoggedIn(): boolean {
  return typeof ayb.token === "string" && typeof ayb.refreshToken === "string";
}
`
}

func aybClientNode() string {
	return `import { AYBClient } from "@allyourbase/js";

const AYB_URL = process.env.AYB_URL || "http://localhost:8090";

export const ayb = new AYBClient(AYB_URL);
`
}

// tsConfigJSON returns the TypeScript compiler configuration for React and Vite projects, configured for ES2020 target with strict type checking and JSX support.
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

// reactApp returns the root React component with server health display and example items listing from the items table.
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

// nextTSConfig returns the TypeScript compiler configuration for Next.js projects, configured for ES2017 target with strict type checking and the Next.js plugin.
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

// nextPage returns the root page component for Next.js projects with server connection status display and example items listing.
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

// expressTSConfig returns the TypeScript compiler configuration for Express projects, configured for ES2020 target with strict type checking and output to dist directory.
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

// TODO: Document nodeMain.
func nodeMain(listItemsBody string) string {
	return fmt.Sprintf(`import { ayb } from "./lib/ayb";

async function main() {
  try {
    const health = await ayb.health();
    console.log("AYB server:", health.status);
  } catch {
    console.error("Cannot connect to AYB. Run 'ayb start' first.");
    process.exit(1);
  }

  try {
    const { items } = await ayb.records.list("items");
%s
  } catch {
    console.error("Cannot list items. Run 'ayb sql < schema.sql' first.");
    process.exit(1);
  }
}

main();
`, listItemsBody)
}

// expressMain returns the entry point source code for Express template projects, including health check and example record listing from the AYB API.
func expressMain() string {
	return nodeMain(`    console.log("Items:", items.length);
    for (const item of items) {
      console.log(" -", item.name);
    }`)
}

// plainMain returns the entry point source code for plain Node.js template projects, with AYB health check and example record listing.
func plainMain() string {
	return nodeMain(`    console.log("Items:", items.length);`)
}
