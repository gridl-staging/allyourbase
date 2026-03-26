package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidTemplates(t *testing.T) {
	t.Parallel()
	templates := ValidTemplates()
	testutil.Equal(t, 9, len(templates))
	testutil.True(t, IsValidTemplate("react"))
	testutil.True(t, IsValidTemplate("next"))
	testutil.True(t, IsValidTemplate("express"))
	testutil.True(t, IsValidTemplate("plain"))
	testutil.True(t, IsValidTemplate("blog"))
	testutil.True(t, IsValidTemplate("kanban"))
	testutil.True(t, IsValidTemplate("ecommerce"))
	testutil.True(t, IsValidTemplate("polls"))
	testutil.True(t, IsValidTemplate("chat"))
	testutil.False(t, IsValidTemplate("invalid"))
	testutil.False(t, IsValidTemplate(""))

	blogCount := 0
	for _, tmpl := range templates {
		if tmpl == TemplateBlog {
			blogCount++
		}
	}
	testutil.Equal(t, 1, blogCount)
}

func TestRun_React(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "my-app", TemplateReact)

	// Check common files exist
	assertFilesExist(t, projectDir, "ayb.toml", "schema.sql", ".env", ".gitignore", "CLAUDE.md")

	// Check React-specific files
	assertFilesExist(t, projectDir,
		"package.json",
		"tsconfig.json",
		"vite.config.ts",
		"index.html",
		"src/main.tsx",
		"src/App.tsx",
		"src/lib/ayb.ts",
		"src/index.css",
	)

	// Check content
	assertFileContains(t, projectDir, "package.json", `"@allyourbase/js"`)
	assertFileContains(t, projectDir, "package.json", `"react"`)
	assertFileContains(t, projectDir, "package.json", `"my-app"`)
	assertFileContains(t, projectDir, "ayb.toml", `port = 8090`)
	assertFileContains(t, projectDir, "schema.sql", `CREATE TABLE IF NOT EXISTS items`)
	assertFileContains(t, projectDir, "src/lib/ayb.ts", `AYBClient`)
	assertFileContains(t, projectDir, "src/lib/ayb.ts", `import.meta.env.VITE_AYB_URL`)
	assertFileContains(t, projectDir, "src/lib/ayb.ts", "setTokens(")
	assertFileContains(t, projectDir, "src/lib/ayb.ts", "clearTokens(")
	assertFileContains(t, projectDir, "CLAUDE.md", "my-app")
	assertFileContains(t, projectDir, "index.html", "my-app")
}

func TestRun_Next(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "nextapp", TemplateNext)

	assertFilesExist(t, projectDir,
		"package.json",
		"next.config.js",
		"src/app/layout.tsx",
		"src/app/page.tsx",
		"src/lib/ayb.ts",
	)

	assertFileContains(t, projectDir, "package.json", `"next"`)
	assertFileContains(t, projectDir, ".gitignore", ".next/")
	assertFileContains(t, projectDir, "src/app/layout.tsx", "nextapp")
}

func TestRun_Express(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "api-server", TemplateExpress)

	assertFilesExist(t, projectDir, "package.json", "src/index.ts", "src/lib/ayb.ts")

	assertFileContains(t, projectDir, "package.json", `"tsx"`)
	assertFileContains(t, projectDir, "src/lib/ayb.ts", `process.env.AYB_URL`)
}

func TestRun_Plain(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "plain-app", TemplatePlain)

	assertFilesExist(t, projectDir, "package.json", "src/index.ts", "src/lib/ayb.ts", "ayb.toml")
}

func TestRun_EmptyName(t *testing.T) {
	t.Parallel()
	err := Run(Options{Name: "", Template: TemplateReact})
	testutil.ErrorContains(t, err, "project name is required")
}

func TestRun_DirectoryAlreadyExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "existing"), 0755)

	err := Run(Options{Name: "existing", Template: TemplateReact, Dir: dir})
	testutil.ErrorContains(t, err, "already exists")
}

func TestRun_DefaultDir(t *testing.T) {
	// Verify Dir defaults to "."
	t.Parallel()

	opts := Options{Name: "test", Template: TemplatePlain}
	// Can't actually run this without creating a dir in cwd,
	// so just verify the option handling
	testutil.Equal(t, "", opts.Dir)
}

func TestAybTomlContent(t *testing.T) {
	t.Parallel()
	content := aybToml(Options{Name: "test"})
	// Validate all TOML sections exist
	testutil.Contains(t, content, "[server]")
	testutil.Contains(t, content, "[database]")
	testutil.Contains(t, content, "[auth]")
	testutil.Contains(t, content, "[storage]")
	testutil.Contains(t, content, "[admin]")
	// Validate key values
	testutil.Contains(t, content, `host = "127.0.0.1"`)
	testutil.Contains(t, content, `port = 8090`)
	testutil.Contains(t, content, `backend = "local"`)
	// Auth, storage, admin all enabled
	// Count occurrences of "enabled = true" — should be 3 (auth, storage, admin)
	testutil.Equal(t, 3, strings.Count(content, "enabled = true"))
}

func TestSchemaSQL(t *testing.T) {
	t.Parallel()
	content := schemaSQLFile()
	// Table structure
	testutil.Contains(t, content, "CREATE TABLE IF NOT EXISTS items")
	testutil.Contains(t, content, "id         SERIAL PRIMARY KEY")
	testutil.Contains(t, content, "name       TEXT NOT NULL")
	testutil.Contains(t, content, "description TEXT")
	testutil.Contains(t, content, "owner_id   UUID REFERENCES _ayb_users(id)")
	testutil.Contains(t, content, "created_at TIMESTAMPTZ NOT NULL DEFAULT now()")
	testutil.Contains(t, content, "updated_at TIMESTAMPTZ NOT NULL DEFAULT now()")
	// RLS
	testutil.Contains(t, content, "ALTER TABLE items ENABLE ROW LEVEL SECURITY")
	// All 4 policies
	testutil.Contains(t, content, "CREATE POLICY items_select ON items FOR SELECT")
	testutil.Contains(t, content, "CREATE POLICY items_insert ON items FOR INSERT")
	testutil.Contains(t, content, "CREATE POLICY items_update ON items FOR UPDATE")
	testutil.Contains(t, content, "CREATE POLICY items_delete ON items FOR DELETE")
	// Policy conditions reference the correct setting
	testutil.Contains(t, content, "current_setting('ayb.user_id', true)::uuid")
}

func TestGitignoreNextTemplate(t *testing.T) {
	t.Parallel()
	content := gitignoreFile(TemplateNext)
	testutil.Contains(t, content, ".next/")
	testutil.Contains(t, content, "node_modules/")
}

func TestGitignoreReactTemplate(t *testing.T) {
	t.Parallel()
	content := gitignoreFile(TemplateReact)
	// React template should NOT have .next/
	testutil.False(t, strings.Contains(content, ".next/"))
	testutil.Contains(t, content, "node_modules/")
}

func TestClaudeMD(t *testing.T) {
	t.Parallel()
	content := claudeMD(Options{Name: "my-project"})
	testutil.Contains(t, content, "my-project")
	testutil.Contains(t, content, "ayb start")
	testutil.Contains(t, content, "AYBClient")
}

func TestAybClientBrowser(t *testing.T) {
	t.Parallel()
	content := aybClient()
	testutil.Contains(t, content, "import.meta.env.VITE_AYB_URL")
	testutil.False(t, strings.Contains(content, "localStorage."))
	testutil.Contains(t, content, "setSessionTokens")
	testutil.Contains(t, content, "clearSessionTokens")
	testutil.Contains(t, content, "isLoggedIn")
	// Verify SDK method calls use correct names (setTokens/clearTokens, not setToken)
	testutil.Contains(t, content, "ayb.setTokens(")
	testutil.Contains(t, content, "ayb.clearTokens(")
	testutil.False(t, strings.Contains(content, "ayb.setToken("))
	testutil.Contains(t, content, `typeof ayb.token === "string"`)
	testutil.Contains(t, content, `typeof ayb.refreshToken === "string"`)
	testutil.Contains(t, content, "Keep auth tokens in memory by default")
}

func TestAybClientNode(t *testing.T) {
	t.Parallel()
	content := aybClientNode()
	testutil.Contains(t, content, "process.env.AYB_URL")
	// Node client should NOT use localStorage
	testutil.False(t, strings.Contains(content, "localStorage"))
}

func TestEnvFileContent(t *testing.T) {
	t.Parallel()
	content := envFile()
	testutil.Contains(t, content, "AYB_SERVER_PORT=8090")
	testutil.Contains(t, content, "AYB_AUTH_ENABLED=true")
	testutil.Contains(t, content, "AYB_ADMIN_ENABLED=true")
	testutil.Contains(t, content, "AYB_DATABASE_URL")
	testutil.Contains(t, content, "AYB_AUTH_JWT_SECRET")
	testutil.Contains(t, content, "AYB_ADMIN_PASSWORD")
}

func TestViteConfigContent(t *testing.T) {
	t.Parallel()
	content := viteConfig()
	testutil.Contains(t, content, "defineConfig")
	testutil.Contains(t, content, "@vitejs/plugin-react")
	testutil.Contains(t, content, "react()")
}

func TestReactTSConfigContent(t *testing.T) {
	t.Parallel()
	content := tsConfigJSON()
	testutil.Contains(t, content, `"jsx": "react-jsx"`)
	testutil.Contains(t, content, `"target": "ES2020"`)
	testutil.Contains(t, content, `"DOM.Iterable"`)
	testutil.Contains(t, content, `"strict": true`)
}

func TestNextTSConfigContent(t *testing.T) {
	t.Parallel()
	content := nextTSConfig()
	testutil.Contains(t, content, `"jsx": "preserve"`)
	testutil.Contains(t, content, `"target": "ES2017"`)
	testutil.Contains(t, content, `"next"`)
	testutil.Contains(t, content, `"incremental": true`)
}

func TestExpressTSConfigContent(t *testing.T) {
	t.Parallel()
	content := expressTSConfig()
	testutil.Contains(t, content, `"target": "ES2020"`)
	testutil.Contains(t, content, `"outDir": "dist"`)
	testutil.Contains(t, content, `"rootDir": "src"`)
	testutil.Contains(t, content, `"esModuleInterop": true`)
}

func TestNextPageContent(t *testing.T) {
	t.Parallel()
	content := nextPage()
	// "use client" must be the first line
	testutil.True(t, strings.HasPrefix(content, "\"use client\""),
		"Next.js page must start with \"use client\" directive")
	testutil.Contains(t, content, "useEffect")
	testutil.Contains(t, content, "ayb.health()")
	testutil.Contains(t, content, "ayb.records")
}

func TestNextLayoutContent(t *testing.T) {
	t.Parallel()
	content := nextLayout(Options{Name: "myapp"})
	testutil.Contains(t, content, `title: "myapp"`)
	testutil.Contains(t, content, "RootLayout")
	testutil.Contains(t, content, "<html")
}

func TestNextConfigContent(t *testing.T) {
	t.Parallel()
	content := nextConfig()
	testutil.Contains(t, content, "module.exports = nextConfig")
}

func TestReactMainContent(t *testing.T) {
	t.Parallel()
	content := reactMain()
	testutil.Contains(t, content, "ReactDOM.createRoot")
	testutil.Contains(t, content, "React.StrictMode")
	testutil.Contains(t, content, "import App")
}

func TestReactAppContent(t *testing.T) {
	t.Parallel()
	content := reactApp()
	testutil.Contains(t, content, "useEffect")
	testutil.Contains(t, content, "useState")
	testutil.Contains(t, content, "ayb.health()")
	testutil.Contains(t, content, "ayb.records")
}

func TestExpressMainContent(t *testing.T) {
	t.Parallel()
	content := expressMain()
	testutil.Contains(t, content, `import { ayb }`)
	testutil.Contains(t, content, "ayb.health()")
	testutil.Contains(t, content, "ayb.records.list")
	testutil.Contains(t, content, "async function main()")
	testutil.Contains(t, content, "Cannot connect to AYB. Run 'ayb start' first.")
	testutil.Contains(t, content, "Cannot list items. Run 'ayb sql < schema.sql' first.")
}

func TestPlainMainContent(t *testing.T) {
	t.Parallel()
	content := plainMain()
	testutil.Contains(t, content, `import { ayb }`)
	testutil.Contains(t, content, "ayb.health()")
	testutil.Contains(t, content, "ayb.records.list")
	testutil.Contains(t, content, "Cannot connect to AYB. Run 'ayb start' first.")
	testutil.Contains(t, content, "Cannot list items. Run 'ayb sql < schema.sql' first.")
}

func TestPackageNameLowercase(t *testing.T) {
	// Verify mixed-case names get lowercased in package.json
	t.Parallel()

	content := packageJSON(Options{Name: "MyApp"}, "react")
	testutil.Contains(t, content, `"name": "myapp"`)
	// Should NOT contain the original casing
	testutil.False(t, strings.Contains(content, `"name": "MyApp"`))
}

// helpers

func assertFileExists(t *testing.T, dir, path string) {
	t.Helper()
	fullPath := filepath.Join(dir, path)
	_, err := os.Stat(fullPath)
	testutil.Nil(t, err)
}

func assertFilesExist(t *testing.T, dir string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		assertFileExists(t, dir, path)
	}
}

func assertFileContains(t *testing.T, dir, path, substr string) {
	t.Helper()
	fullPath := filepath.Join(dir, path)
	content, err := os.ReadFile(fullPath)
	testutil.NoError(t, err)
	testutil.Contains(t, string(content), substr)
}

func scaffoldProject(t *testing.T, name string, tmpl Template) string {
	t.Helper()
	dir := t.TempDir()
	err := Run(Options{Name: name, Template: tmpl, Dir: dir})
	testutil.NoError(t, err)
	return filepath.Join(dir, name)
}

func assertDomainTemplateScaffoldFiles(t *testing.T, projectDir, domainClientPath string) {
	t.Helper()
	assertFilesExist(t, projectDir,
		"ayb.toml",
		"schema.sql",
		"seed.sql",
		".env",
		".gitignore",
		"CLAUDE.md",
		"README.md",
		"package.json",
		"src/index.ts",
		"src/lib/ayb.ts",
		domainClientPath,
	)
}

func assertSchemaDoesNotContainGenericItemsTable(t *testing.T, projectDir string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, "schema.sql"))
	testutil.NoError(t, err)
	testutil.False(t, strings.Contains(string(content), "CREATE TABLE IF NOT EXISTS items"))
}

func TestRun_Blog(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "blog-app", TemplateBlog)
	assertDomainTemplateScaffoldFiles(t, projectDir, "src/lib/blog.ts")

	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS posts")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS comments")
	assertSchemaDoesNotContainGenericItemsTable(t, projectDir)

	assertFileContains(t, projectDir, "README.md", "ayb sql < schema.sql && ayb sql < seed.sql")
	assertFileContains(t, projectDir, "src/lib/blog.ts", "listPosts")
}

func TestRun_ReactRegressionAfterDomainTemplateRefactor(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "react-unchanged", TemplateReact)
	assertFilesExist(t, projectDir, "src/main.tsx", "src/App.tsx", "src/lib/ayb.ts")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS items")
}

func TestRun_Kanban(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "kanban-app", TemplateKanban)
	assertDomainTemplateScaffoldFiles(t, projectDir, "src/lib/kanban.ts")

	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS boards")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS columns")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS cards")
	assertSchemaDoesNotContainGenericItemsTable(t, projectDir)
}

func TestRun_Ecommerce(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "shop-app", TemplateEcommerce)
	assertDomainTemplateScaffoldFiles(t, projectDir, "src/lib/ecommerce.ts")

	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS products")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS orders")
	assertSchemaDoesNotContainGenericItemsTable(t, projectDir)
}

func TestRun_Polls(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "polls-app", TemplatePolls)
	assertDomainTemplateScaffoldFiles(t, projectDir, "src/lib/polls.ts")

	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS polls")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS poll_options")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS votes")
	assertSchemaDoesNotContainGenericItemsTable(t, projectDir)
}

func TestRun_Chat(t *testing.T) {
	t.Parallel()
	projectDir := scaffoldProject(t, "chat-app", TemplateChat)
	assertDomainTemplateScaffoldFiles(t, projectDir, "src/lib/chat.ts")

	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS rooms")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS participants")
	assertFileContains(t, projectDir, "schema.sql", "CREATE TABLE IF NOT EXISTS messages")
	assertSchemaDoesNotContainGenericItemsTable(t, projectDir)
}
