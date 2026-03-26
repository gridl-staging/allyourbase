/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/browser-tests-mocked/fixtures/email-templates.ts.
 */
import type { Page } from "@playwright/test";
import { json, handleCommonAdminRoutes, unhandledMockedApiRoute, type MockApiResponse } from "./core";

export interface MockEmailPreviewRequest {
  key: string;
  subjectTemplate: string;
  htmlTemplate: string;
  variables: Record<string, string>;
}

export interface EmailTemplateMockState {
  previewCalls: number;
  previewRequests: MockEmailPreviewRequest[];
  resetPreviewCalls: () => void;
}

export interface EmailTemplateMockOptions {
  previewResponder?: (request: MockEmailPreviewRequest) => MockApiResponse;
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
  htmlTemplate: '<p>Hello {{.AppName}}: <a href="{{.ActionURL}}">Reset</a></p>',
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

/**
 * TODO: Document defaultPreviewResponder.
 */
function defaultPreviewResponder(request: MockEmailPreviewRequest): MockApiResponse {
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

/**
 * TODO: Document mockAdminEmailTemplateApis.
 */
export async function mockAdminEmailTemplateApis(
  page: Page,
  options: EmailTemplateMockOptions = {},
): Promise<EmailTemplateMockState> {
  const previewRequests: MockEmailPreviewRequest[] = [];
  let previewCalls = 0;
  const previewResponder = options.previewResponder || defaultPreviewResponder;

  const templateDetails: Record<string, unknown> = {
    "auth.password_reset": builtinReset,
    "app.club_invite": appInvite,
  };

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const method = request.method();
    const url = new URL(request.url());
    const path = url.pathname;

    if (await handleCommonAdminRoutes(route, method, path)) return;

    if (method === "GET" && path === "/api/admin/email/templates") {
      return json(route, 200, defaultList);
    }

    // GET /api/admin/email/templates/:key
    const detailMatch = path.match(/^\/api\/admin\/email\/templates\/([^/]+)$/);
    if (method === "GET" && detailMatch && templateDetails[detailMatch[1]]) {
      return json(route, 200, templateDetails[detailMatch[1]]);
    }

    // POST /api/admin/email/templates/:key/preview
    const previewMatch = path.match(/^\/api\/admin\/email\/templates\/([^/]+)\/preview$/);
    if (method === "POST" && previewMatch && templateDetails[previewMatch[1]]) {
      const data = request.postDataJSON() as {
        subjectTemplate: string;
        htmlTemplate: string;
        variables: Record<string, string>;
      };
      const previewRequest: MockEmailPreviewRequest = {
        key: previewMatch[1],
        subjectTemplate: data.subjectTemplate,
        htmlTemplate: data.htmlTemplate,
        variables: data.variables || {},
      };
      previewRequests.push(previewRequest);
      previewCalls += 1;
      const response = previewResponder(previewRequest);
      return json(route, response.status, response.body);
    }

    return unhandledMockedApiRoute(route, method, path);
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
