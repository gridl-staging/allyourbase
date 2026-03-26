import { defineConfig } from "vitepress";

export default defineConfig({
  title: "👾 Allyourbase",
  description: "Backend-as-a-Service for PostgreSQL",
  head: [["link", { rel: "icon", href: "/favicon.ico" }]],

  themeConfig: {
    nav: [
      { text: "Guide", link: "/guide/getting-started" },
      { text: "API Reference", link: "/guide/api-reference" },
      { text: "OpenAPI", link: "/guide/openapi" },
      { text: "GitHub", link: "https://github.com/gridlhq/allyourbase" },
    ],

    sidebar: [
      {
        text: "Introduction",
        items: [
          { text: "Getting Started", link: "/guide/getting-started" },
          { text: "Configuration", link: "/guide/configuration" },
        ],
      },
      {
        text: "Features",
        items: [
          { text: "REST API", link: "/guide/api-reference" },
          { text: "GraphQL", link: "/guide/graphql" },
          { text: "Authentication", link: "/guide/authentication" },
          { text: "OAuth Provider", link: "/guide/oauth-provider" },
          { text: "Organizations", link: "/guide/organizations" },
          { text: "SAML SSO", link: "/guide/saml" },
          { text: "AI and Vector Search", link: "/guide/ai-vector" },
          { text: "Security", link: "/guide/security" },
          { text: "Custom Domains", link: "/guide/custom-domains" },
          { text: "Static Hosting", link: "/guide/static-hosting" },
          { text: "Log Drains", link: "/guide/log-drains" },
          { text: "File Storage", link: "/guide/file-storage" },
          { text: "Realtime", link: "/guide/realtime" },
          { text: "Database RPC", link: "/guide/database-rpc" },
          { text: "Email", link: "/guide/email" },
          { text: "Email Templates", link: "/guide/email-templates" },
          { text: "Push Notifications", link: "/guide/push-notifications" },
          { text: "Job Queue", link: "/guide/job-queue" },
          { text: "Edge Functions", link: "/guide/edge-functions" },
          { text: "Webhooks", link: "/guide/webhooks" },
          { text: "Materialized Views", link: "/guide/materialized-views" },
          { text: "PostGIS", link: "/guide/postgis" },
          { text: "Admin Dashboard", link: "/guide/admin-dashboard" },
          { text: "Backups", link: "/guide/backups" },
          { text: "Replicas", link: "/guide/replicas" },
          { text: "Branching", link: "/guide/branching" },
        ],
      },
      {
        text: "SDK & Tutorials",
        items: [
          { text: "JavaScript SDK", link: "/guide/javascript-sdk" },
          { text: "React SDK", link: "/guide/react-sdk" },
          { text: "SSR SDK", link: "/guide/ssr-sdk" },
          { text: "Flutter SDK", link: "/guide/flutter-sdk" },
          { text: "Python SDK", link: "/guide/python-sdk" },
          { text: "Swift SDK", link: "/guide/swift-sdk" },
          { text: "Kotlin SDK", link: "/guide/kotlin-sdk" },
          { text: "Go SDK", link: "/guide/go-sdk" },
          { text: "Patterns", link: "/guide/patterns" },
          { text: "Quickstart: Todo App", link: "/guide/quickstart" },
          { text: "Tutorial: Kanban Board", link: "/guide/tutorial-kanban" },
        ],
      },
      {
        text: "Tools",
        items: [
          { text: "CLI Reference", link: "/guide/cli" },
          { text: "Migrations", link: "/guide/migrations" },
          { text: "MCP Server", link: "/guide/mcp" },
        ],
      },
      {
        text: "Operations",
        items: [
          { text: "Deployment", link: "/guide/deployment" },
          { text: "Performance", link: "/guide/performance" },
          { text: "Comparison", link: "/guide/comparison" },
        ],
      },
      {
        text: "Reference",
        items: [
          { text: "OpenAPI Spec", link: "/guide/openapi" },
        ],
      },
    ],

    socialLinks: [
      { icon: "github", link: "https://github.com/gridlhq/allyourbase" },
    ],

    footer: {
      message: "Released under the MIT License.",
    },

    search: {
      provider: "local",
    },
  },
});
