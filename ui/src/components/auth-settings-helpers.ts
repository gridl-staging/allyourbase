/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/auth-settings-helpers.ts.
 */
import type {
  OAuthProviderInfo,
  UpdateAuthProviderRequest,
} from "../types";

export const PROVIDER_SETUP_INFO: Record<string, { consoleUrl: string; consoleLabel: string }> = {
  google: {
    consoleUrl: "https://console.cloud.google.com/apis/credentials",
    consoleLabel: "Google Cloud Console",
  },
  github: {
    consoleUrl: "https://github.com/settings/developers",
    consoleLabel: "GitHub Developer Settings",
  },
  microsoft: {
    consoleUrl: "https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps",
    consoleLabel: "Azure App Registrations",
  },
  apple: {
    consoleUrl: "https://developer.apple.com/account/resources/identifiers/list/serviceIds",
    consoleLabel: "Apple Developer Portal",
  },
  discord: {
    consoleUrl: "https://discord.com/developers/applications",
    consoleLabel: "Discord Developer Portal",
  },
  twitter: {
    consoleUrl: "https://developer.twitter.com/en/portal/projects-and-apps",
    consoleLabel: "Twitter Developer Portal",
  },
  facebook: {
    consoleUrl: "https://developers.facebook.com/apps/",
    consoleLabel: "Facebook for Developers",
  },
  linkedin: {
    consoleUrl: "https://www.linkedin.com/developers/apps",
    consoleLabel: "LinkedIn Developer Portal",
  },
  spotify: {
    consoleUrl: "https://developer.spotify.com/dashboard",
    consoleLabel: "Spotify Developer Dashboard",
  },
  twitch: {
    consoleUrl: "https://dev.twitch.tv/console/apps",
    consoleLabel: "Twitch Developer Console",
  },
  gitlab: {
    consoleUrl: "https://gitlab.com/-/user_settings/applications",
    consoleLabel: "GitLab Applications",
  },
  bitbucket: {
    consoleUrl: "https://bitbucket.org/account/settings/app-authorizations/",
    consoleLabel: "Bitbucket App Authorizations",
  },
  slack: {
    consoleUrl: "https://api.slack.com/apps",
    consoleLabel: "Slack API Apps",
  },
  zoom: {
    consoleUrl: "https://marketplace.zoom.us/develop/create",
    consoleLabel: "Zoom App Marketplace",
  },
  notion: {
    consoleUrl: "https://www.notion.so/my-integrations",
    consoleLabel: "Notion Integrations",
  },
  figma: {
    consoleUrl: "https://www.figma.com/developers/apps",
    consoleLabel: "Figma Developer Apps",
  },
};

export interface ProviderFormState {
  enabled: boolean;
  client_id: string;
  client_secret: string;
  tenant_id: string;
  team_id: string;
  key_id: string;
  private_key: string;
  issuer_url: string;
  display_name: string;
  scopes: string;
}

export interface OIDCFormState {
  provider_name: string;
  issuer_url: string;
  client_id: string;
  client_secret: string;
  display_name: string;
  scopes: string;
}

function normalizeScopes(scopes: string): string[] {
  return scopes
    .trim()
    .split(/\s+/)
    .filter((value) => value.length > 0);
}

/**
 * TODO: Document createProviderForm.
 */
export function createProviderForm({
  enabled,
}: OAuthProviderInfo): ProviderFormState {
  return {
    enabled,
    client_id: "",
    client_secret: "",
    tenant_id: "",
    team_id: "",
    key_id: "",
    private_key: "",
    issuer_url: "",
    display_name: "",
    scopes: "",
  };
}

export function createOIDCForm(): OIDCFormState {
  return {
    provider_name: "",
    issuer_url: "",
    client_id: "",
    client_secret: "",
    display_name: "",
    scopes: "openid profile email",
  };
}

function trimToOptional(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

/**
 * Build an update payload with only the fields the user actually entered.
 */
export function buildProviderUpdatePayload(
  providerName: string,
  form: ProviderFormState,
): UpdateAuthProviderRequest {
  const payload: UpdateAuthProviderRequest = { enabled: form.enabled };
  const clientID = trimToOptional(form.client_id);
  const clientSecret = trimToOptional(form.client_secret);
  if (clientID !== undefined) {
    payload.client_id = clientID;
  }
  if (clientSecret !== undefined) {
    payload.client_secret = clientSecret;
  }
  if (providerName === "microsoft") {
    const tenantID = trimToOptional(form.tenant_id);
    if (tenantID !== undefined) {
      payload.tenant_id = tenantID;
    }
  }
  if (providerName === "apple") {
    const teamID = trimToOptional(form.team_id);
    const keyID = trimToOptional(form.key_id);
    const privateKey = trimToOptional(form.private_key);
    if (teamID !== undefined) {
      payload.team_id = teamID;
    }
    if (keyID !== undefined) {
      payload.key_id = keyID;
    }
    if (privateKey !== undefined) {
      payload.private_key = privateKey;
    }
  }
  if (providerName !== "microsoft" && providerName !== "apple") {
    const issuerURL = trimToOptional(form.issuer_url);
    const displayName = trimToOptional(form.display_name);
    const scopes = normalizeScopes(form.scopes);

    if (issuerURL !== undefined) {
      payload.issuer_url = issuerURL;
    }
    if (displayName !== undefined) {
      payload.display_name = displayName;
    }
    if (scopes.length > 0) {
      payload.scopes = scopes;
    }
  }
  return payload;
}

export function buildOIDCUpdatePayload(form: OIDCFormState): UpdateAuthProviderRequest {
  const scopes = normalizeScopes(form.scopes);

  return {
    enabled: true,
    issuer_url: form.issuer_url.trim(),
    client_id: form.client_id.trim(),
    client_secret: form.client_secret.trim(),
    display_name: trimToOptional(form.display_name),
    scopes: scopes.length > 0 ? scopes : undefined,
  };
}
