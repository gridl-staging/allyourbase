import { describe, expect, it } from "vitest";
import { AuthSettingsToggles } from "../AuthSettingsToggles";
import { AuthSettingsProviders } from "../AuthSettingsProviders";
import { AuthSettingsOIDCForm } from "../AuthSettingsOIDCForm";
import { ApiExplorerRequest } from "../ApiExplorerRequest";
import { ApiExplorerResponse } from "../ApiExplorerResponse";
import { ApiExplorerHistory } from "../ApiExplorerHistory";
import { OAuthClientsTable } from "../OAuthClientsTable";
import { OAuthClientsModals } from "../OAuthClientsModals";
import { ApiKeysTable } from "../ApiKeysTable";
import { ApiKeysModals } from "../ApiKeysModals";
import { Sidebar } from "../Sidebar";
import { ContentRouter } from "../ContentRouter";

describe("stage 5 split modules", () => {
  it("exports all extracted components", () => {
    expect(AuthSettingsToggles).toBeTypeOf("function");
    expect(AuthSettingsProviders).toBeTypeOf("function");
    expect(AuthSettingsOIDCForm).toBeTypeOf("function");
    expect(ApiExplorerRequest).toBeTypeOf("function");
    expect(ApiExplorerResponse).toBeTypeOf("function");
    expect(ApiExplorerHistory).toBeTypeOf("function");
    expect(OAuthClientsTable).toBeTypeOf("function");
    expect(OAuthClientsModals).toBeTypeOf("function");
    expect(ApiKeysTable).toBeTypeOf("function");
    expect(ApiKeysModals).toBeTypeOf("function");
    expect(Sidebar).toBeTypeOf("function");
    expect(ContentRouter).toBeTypeOf("function");
  });
});
