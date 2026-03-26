export interface AuthSettings {
  magic_link_enabled: boolean;
  sms_enabled: boolean;
  email_mfa_enabled: boolean;
  anonymous_auth_enabled: boolean;
  totp_enabled: boolean;
}

export interface OAuthProviderInfo {
  name: string;
  type: "builtin" | "oidc";
  enabled: boolean;
  client_id_configured: boolean;
}

export interface OAuthProviderListResponse {
  providers: OAuthProviderInfo[];
}

export interface UpdateAuthProviderRequest {
  enabled?: boolean;
  client_id?: string;
  client_secret?: string;
  tenant_id?: string;
  team_id?: string;
  key_id?: string;
  private_key?: string;
  facebook_api_version?: string;
  gitlab_base_url?: string;
  issuer_url?: string;
  scopes?: string[];
  display_name?: string;
}

export interface TestProviderResult {
  success: boolean;
  provider: string;
  message?: string;
  error?: string;
}

export interface AuthUser {
  id: string;
  email: string;
  phone?: string;
  is_anonymous?: boolean;
  linked_at?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AuthTokens {
  token: string;
  refreshToken: string;
  user: AuthUser;
}

export interface TOTPEnrollment {
  factor_id: string;
  uri: string;
  secret: string;
}

export interface MFAFactor {
  id: string;
  method: string;
  label: string;
  phone?: string;
  email?: string;
}
