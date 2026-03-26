export interface ExtensionInfo {
  name: string;
  installed: boolean;
  available: boolean;
  installed_version?: string;
  default_version?: string;
  comment?: string;
}

export interface ExtensionListResponse {
  extensions: ExtensionInfo[];
  total: number;
}
