export interface SecretMetadata {
  name: string;
  created_at: string;
  updated_at: string;
}

export interface SecretValue {
  name: string;
  value: string;
}

export interface CreateSecretRequest {
  name: string;
  value: string;
}

export interface UpdateSecretRequest {
  value: string;
}
