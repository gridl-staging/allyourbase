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
          { text: "Authentication", link: "/guide/authentication" },
          { text: "File Storage", link: "/guide/file-storage" },
          { text: "Realtime", link: "/guide/realtime" },
          { text: "Database RPC", link: "/guide/database-rpc" },
          { text: "Email", link: "/guide/email" },
          { text: "Email Templates", link: "/guide/email-templates" },
          { text: "Job Queue", link: "/guide/job-queue" },
          { text: "Admin Dashboard", link: "/guide/admin-dashboard" },
        ],
      },
      {
        text: "SDK & Tutorials",
        items: [
          { text: "JavaScript SDK", link: "/guide/javascript-sdk" },
          { text: "Quickstart: Todo App", link: "/guide/quickstart" },
          { text: "Tutorial: Kanban Board", link: "/guide/tutorial-kanban" },
        ],
      },
      {
        text: "Operations",
        items: [
          { text: "Deployment", link: "/guide/deployment" },
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
