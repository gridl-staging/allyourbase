/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/types/ai.ts.
 */
export interface CallLog {
  id: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  duration_ms: number;
  status: string;
  error_message?: string;
  edge_function_id?: string;
  created_at: string;
}

export interface CallLogListResponse {
  logs: CallLog[];
  total: number;
}

export interface ProviderUsage {
  calls: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_usd: number;
  error_count: number;
}

export interface UsageSummary {
  total_calls: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  total_cost_usd: number;
  error_count: number;
  by_provider: Record<string, ProviderUsage>;
}

export interface DailyUsage {
  day: string;
  provider: string;
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_usd: number;
}

export interface PromptVariable {
  name: string;
  type: string;
  required: boolean;
  default?: string;
}

export interface Prompt {
  id: string;
  name: string;
  version: number;
  template: string;
  variables: PromptVariable[];
  model?: string;
  provider?: string;
  max_tokens?: number;
  temperature?: number;
  created_at: string;
  updated_at: string;
}

export interface PromptListResponse {
  prompts: Prompt[];
  total: number;
}

export interface PromptVersion {
  id: string;
  prompt_id: string;
  version: number;
  template: string;
  variables: PromptVariable[];
  created_at: string;
}

export interface CreatePromptRequest {
  name: string;
  template: string;
  variables?: PromptVariable[];
  model?: string;
  provider?: string;
  max_tokens?: number;
  temperature?: number;
}

export interface UpdatePromptRequest {
  template?: string;
  variables?: PromptVariable[];
  model?: string;
  provider?: string;
  max_tokens?: number;
  temperature?: number;
}

export interface AssistantRequest {
  mode: string;
  query: string;
  provider?: string;
  model?: string;
}

/**
 * TODO: Document AssistantResponse.
 */
export interface AssistantResponse {
  history_id?: string;
  mode?: string;
  status?: string;
  query?: string;
  text?: string;
  sql?: string;
  explanation?: string;
  warning?: string;
  provider?: string;
  model?: string;
  duration_ms?: number;
  input_tokens?: number;
  output_tokens?: number;
  created_at?: string;
  finished_at?: string;
}

/**
 * TODO: Document AssistantHistoryEntry.
 */
export interface AssistantHistoryEntry {
  id: string;
  mode: string;
  query_text: string;
  response_text: string;
  sql: string;
  explanation: string;
  warning: string;
  provider: string;
  model: string;
  status: string;
  duration_ms: number;
  input_tokens: number;
  output_tokens: number;
  created_at: string;
}

export interface AssistantHistoryListResponse {
  history: AssistantHistoryEntry[];
  total: number;
}

export interface PromptRenderResponse {
  rendered: string;
  prompt: Prompt;
}
