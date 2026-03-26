export interface SAMLProvider {
  name: string;
  entity_id: string;
  idp_metadata_xml?: string;
  attribute_mapping?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface SAMLUpsertRequest {
  name: string;
  entity_id: string;
  idp_metadata_url?: string;
  idp_metadata_xml?: string;
  attribute_mapping?: Record<string, string>;
}
