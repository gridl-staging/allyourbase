import { test as base, expect, type Page, type Route } from "@playwright/test";

export interface MockEmailPreviewRequest {
  key: string;
  subjectTemplate: string;
  htmlTemplate: string;
  variables: Record<string, string>;
}

interface PreviewResponse {
  status: number;
  body: unknown;
}

export interface EmailTemplateMockState {
  previewCalls: number;
  previewRequests: MockEmailPreviewRequest[];
  resetPreviewCalls: () => void;
}

export interface EmailTemplateMockOptions {
  previewResponder?: (request: MockEmailPreviewRequest) => PreviewResponse;
}

function json(route: Route, status: number, body: unknown): Promise<void> {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

const defaultList = {
  items: [
    {
      templateKey: "auth.password_reset",
      source: "builtin",
      subjectTemplate: "Reset your password",
      enabled: true,
      updatedAt: "2026-02-22T12:00:00Z",
    },
    {
      templateKey: "app.club_invite",
      source: "custom",
      subjectTemplate: "You're invited to {{.ClubName}}",
      enabled: true,
      updatedAt: "2026-02-22T12:05:00Z",
    },
  ],
  count: 2,
};

const builtinReset = {
  source: "builtin",
  templateKey: "auth.password_reset",
  subjectTemplate: "Reset your password",
  htmlTemplate: "<p>Hello {{.AppName}}: <a href=\"{{.ActionURL}}\">Reset</a></p>",
  enabled: true,
  variables: ["AppName", "ActionURL"],
};

const appInvite = {
  source: "custom",
  templateKey: "app.club_invite",
  subjectTemplate: "You're invited to {{.ClubName}}",
  htmlTemplate: "<p>{{.Inviter}} invited you to {{.ClubName}}</p>",
  enabled: true,
  variables: ["ClubName", "Inviter"],
};

function defaultPreviewResponder(request: MockEmailPreviewRequest): PreviewResponse {
  if (!request.variables.ActionURL) {
    return {
      status: 400,
      body: { message: "missing variable ActionURL" },
    };
  }

  return {
    status: 200,
    body: {
      subject: `Preview for ${request.variables.AppName || "App"}`,
      html: "<p>Preview HTML</p>",
      text: "Preview text",
    },
  };
}

export async function bootstrapMockedAdminApp(page: Page): Promise<void> {
  await page.addInitScript(() => {
    window.localStorage.setItem("ayb_admin_token", "mock-admin-token");
  });
}

export async function mockAdminEmailTemplateApis(
  page: Page,
  options: EmailTemplateMockOptions = {},
): Promise<EmailTemplateMockState> {
  const previewRequests: MockEmailPreviewRequest[] = [];
  let previewCalls = 0;
  const previewResponder = options.previewResponder || defaultPreviewResponder;

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (method === "GET" && path === "/api/admin/status") {
      return json(route, 200, { auth: true });
    }

    if (method === "GET" && path === "/api/schema") {
      return json(route, 200, {
        tables: {},
        schemas: ["public"],
        builtAt: "2026-02-22T12:00:00Z",
      });
    }

    if (method === "GET" && path === "/api/admin/email/templates") {
      return json(route, 200, defaultList);
    }

    if (method === "GET" && path === "/api/admin/email/templates/auth.password_reset") {
      return json(route, 200, builtinReset);
    }

    if (method === "GET" && path === "/api/admin/email/templates/app.club_invite") {
      return json(route, 200, appInvite);
    }

    if (method === "POST" && path === "/api/admin/email/templates/auth.password_reset/preview") {
      const data = request.postDataJSON() as {
        subjectTemplate: string;
        htmlTemplate: string;
        variables: Record<string, string>;
      };
      const previewRequest: MockEmailPreviewRequest = {
        key: "auth.password_reset",
        subjectTemplate: data.subjectTemplate,
        htmlTemplate: data.htmlTemplate,
        variables: data.variables || {},
      };
      previewRequests.push(previewRequest);
      previewCalls += 1;
      const response = previewResponder(previewRequest);
      return json(route, response.status, response.body);
    }

    if (method === "POST" && path === "/api/admin/email/templates/app.club_invite/preview") {
      const data = request.postDataJSON() as {
        subjectTemplate: string;
        htmlTemplate: string;
        variables: Record<string, string>;
      };
      const previewRequest: MockEmailPreviewRequest = {
        key: "app.club_invite",
        subjectTemplate: data.subjectTemplate,
        htmlTemplate: data.htmlTemplate,
        variables: data.variables || {},
      };
      previewRequests.push(previewRequest);
      previewCalls += 1;
      const response = previewResponder(previewRequest);
      return json(route, response.status, response.body);
    }

    return json(route, 500, {
      message: `Unhandled mocked API route: ${method} ${path}`,
    });
  });

  return {
    get previewCalls() {
      return previewCalls;
    },
    previewRequests,
    resetPreviewCalls() {
      previewCalls = 0;
      previewRequests.length = 0;
    },
  };
}

export const test = base;
export { expect };
