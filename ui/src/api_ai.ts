/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_ai.ts.
 */
import { fetchAdmin, request, requestNoBody } from "./api_client";
import type {
  CallLogListResponse,
  UsageSummary,
  DailyUsage,
  AssistantRequest,
  AssistantResponse,
  AssistantHistoryListResponse,
  Prompt,
  PromptListResponse,
  PromptVersion,
  CreatePromptRequest,
  UpdatePromptRequest,
  PromptRenderResponse,
} from "./types/ai";

interface ListAILogsParams {
  page?: number;
  per_page?: number;
  provider?: string;
  status?: string;
  from?: string;
  to?: string;
}

function stringField(payload: Record<string, unknown>, key: string): string | undefined {
  const value = payload[key];
  return typeof value === "string" ? value : undefined;
}

function numberField(payload: Record<string, unknown>, key: string): number | undefined {
  const value = payload[key];
  return typeof value === "number" ? value : undefined;
}

export function listAILogs(params?: ListAILogsParams): Promise<CallLogListResponse> {
  const qs = new URLSearchParams();
  if (params?.page) qs.set("page", String(params.page));
  if (params?.per_page) qs.set("per_page", String(params.per_page));
  if (params?.provider) qs.set("provider", params.provider);
  if (params?.status) qs.set("status", params.status);
  if (params?.from) qs.set("from", params.from);
  if (params?.to) qs.set("to", params.to);
  const query = qs.toString();
  return request<CallLogListResponse>(`/api/admin/ai/logs${query ? `?${query}` : ""}`);
}

export function getAIUsage(): Promise<UsageSummary> {
  return request<UsageSummary>("/api/admin/ai/usage");
}

export function getDailyUsage(): Promise<DailyUsage[]> {
  return request<DailyUsage[]>("/api/admin/ai/usage/daily");
}

export function sendAssistantQuery(req: AssistantRequest): Promise<AssistantResponse> {
  return request<AssistantResponse>("/api/admin/ai/assistant", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

/**
 * TODO: Document streamAssistant.
 */
export async function streamAssistant(
  req: AssistantRequest,
  onChunk: (text: string) => void,
): Promise<AssistantResponse | null> {
  const res = await fetchAdmin("/api/admin/ai/assistant/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new Error(body.message || res.statusText);
  }
  const reader = res.body?.getReader();
  if (!reader) return null;

  const decoder = new TextDecoder();
  let buffer = "";
  let finalResponse: AssistantResponse | null = null;

  const processEvent = (rawEvent: string) => {
    if (!rawEvent.trim()) return;

    let eventName = "message";
    const dataLines: string[] = [];

    for (const line of rawEvent.split("\n")) {
      if (line.startsWith("event:")) {
        eventName = line.slice("event:".length).trim();
        continue;
      }
      if (line.startsWith("data:")) {
        dataLines.push(line.slice("data:".length).trimStart());
      }
    }

    if (dataLines.length === 0) return;

    const payloadRaw = dataLines.join("\n");
    const payload = JSON.parse(payloadRaw) as Record<string, unknown>;

    if (eventName === "chunk" && typeof payload.text === "string") {
      onChunk(payload.text);
      return;
    }

    if (eventName === "done") {
      finalResponse = {
        history_id: stringField(payload, "history_id"),
        mode: stringField(payload, "mode"),
        status: stringField(payload, "status"),
        query: stringField(payload, "query"),
        text: stringField(payload, "text"),
        sql: stringField(payload, "sql"),
        explanation: stringField(payload, "explanation"),
        warning: stringField(payload, "warning"),
        provider: stringField(payload, "provider"),
        model: stringField(payload, "model"),
        duration_ms: numberField(payload, "duration_ms"),
        input_tokens: numberField(payload, "input_tokens"),
        output_tokens: numberField(payload, "output_tokens"),
        created_at: stringField(payload, "created_at"),
        finished_at: stringField(payload, "finished_at"),
      };
      return;
    }

    if (eventName === "error") {
      const message =
        typeof payload.message === "string"
          ? payload.message
          : "AI assistant stream failed";
      throw new Error(message);
    }
  };

  let streamDone = false;
  while (!streamDone) {
    const result = await reader.read();
    streamDone = result.done;

    if (result.value) {
      buffer += decoder.decode(result.value, { stream: !streamDone });

      let boundaryIndex = buffer.indexOf("\n\n");
      while (boundaryIndex >= 0) {
        const rawEvent = buffer.slice(0, boundaryIndex);
        processEvent(rawEvent);
        buffer = buffer.slice(boundaryIndex + 2);
        boundaryIndex = buffer.indexOf("\n\n");
      }
    }
  }

  if (buffer.trim()) {
    processEvent(buffer);
  }

  return finalResponse;
}

export function listAssistantHistory(): Promise<AssistantHistoryListResponse> {
  return request<AssistantHistoryListResponse>("/api/admin/ai/assistant/history");
}

export function listPrompts(): Promise<PromptListResponse> {
  return request<PromptListResponse>("/api/admin/ai/prompts");
}

export function createPrompt(req: CreatePromptRequest): Promise<Prompt> {
  return request<Prompt>("/api/admin/ai/prompts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function getPrompt(id: string): Promise<Prompt> {
  return request<Prompt>(`/api/admin/ai/prompts/${id}`);
}

export function updatePrompt(id: string, req: UpdatePromptRequest): Promise<Prompt> {
  return request<Prompt>(`/api/admin/ai/prompts/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function deletePrompt(id: string): Promise<void> {
  return requestNoBody(`/api/admin/ai/prompts/${id}`, { method: "DELETE" });
}

export function getPromptVersions(id: string): Promise<PromptVersion[]> {
  return request<PromptVersion[]>(`/api/admin/ai/prompts/${id}/versions`);
}

export function renderPrompt(
  id: string,
  variables: Record<string, unknown>,
): Promise<PromptRenderResponse> {
  return request<PromptRenderResponse>(`/api/admin/ai/prompts/${id}/render`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ variables }),
  });
}
