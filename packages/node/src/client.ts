/**
 * NabuGate client.
 *
 * The gateway is OpenAI-wire compatible and passes request bodies through to the
 * upstream provider untouched, so every method here takes the full request
 * object rather than a fixed set of arguments. That is deliberate: parameters
 * this SDK has never heard of — new tool formats, provider-specific flags —
 * reach the provider anyway, with no SDK release needed.
 */

export interface NabuGateConfig {
  /** Gateway base URL, including the /v1 prefix. */
  baseUrl?: string;
  apiKey: string;
  /** Model or agent used when a request does not name one. */
  defaultModel?: string;
  /** Request timeout in milliseconds. Streaming requests are not timed out. */
  timeoutMs?: number;
  /** Extra headers sent with every request, e.g. a project identifier. */
  headers?: Record<string, string>;
  fetch?: typeof fetch;
}

export type Role = 'system' | 'user' | 'assistant' | 'tool';

export interface ChatMessage {
  role: Role;
  content: string | unknown[] | null;
  name?: string;
  tool_call_id?: string;
  tool_calls?: unknown[];
  [key: string]: unknown;
}

export interface ChatRequest {
  model?: string;
  messages: ChatMessage[];
  temperature?: number;
  max_tokens?: number;
  top_p?: number;
  stop?: string | string[];
  seed?: number;
  tools?: unknown[];
  tool_choice?: unknown;
  response_format?: unknown;
  /** Server-side conversation memory; the gateway replays this history. */
  conversation_id?: string;
  [key: string]: unknown;
}

export interface Usage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface ChatResponse {
  id: string;
  model: string;
  choices: Array<{
    index: number;
    message: ChatMessage;
    finish_reason: string | null;
  }>;
  usage?: Usage;
  [key: string]: unknown;
}

export interface EmbeddingsRequest {
  input: string | string[];
  model?: string;
  /**
   * Vector width. Leave unset unless you are storing the vectors: a consumer
   * that writes them to a fixed-width column cannot accept whatever the
   * provider happens to default to.
   */
  dimensions?: number;
  [key: string]: unknown;
}

export interface EmbeddingsResponse {
  data: Array<{ index: number; embedding: number[] }>;
  model: string;
  usage?: Usage;
}

export interface ImageRequest {
  prompt: string;
  model?: string;
  n?: number;
  size?: string;
  [key: string]: unknown;
}

export interface ImageResponse {
  created: number;
  data: Array<{ b64_json?: string; url?: string; revised_prompt?: string }>;
}

export interface SpeechRequest {
  input: string;
  model?: string;
  voice?: string;
  response_format?: string;
  speed?: number;
  [key: string]: unknown;
}

export interface ModelInfo {
  id: string;
  object: string;
  owned_by?: string;
  [key: string]: unknown;
}

/** Thrown for any non-2xx gateway response. */
export class NabuGateError extends Error {
  constructor(
    readonly status: number,
    readonly body: string,
    /** The gateway's error code when it sent one, e.g. "all targets failed". */
    readonly code?: string,
  ) {
    super(`NabuGate request failed (${status}): ${body}`);
    this.name = 'NabuGateError';
  }
}

const DEFAULT_BASE_URL = 'https://gate.nabuxai.com/v1';

export class NabuGateClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly defaultModel: string;
  private readonly timeoutMs: number;
  private readonly extraHeaders: Record<string, string>;
  private readonly fetchImpl: typeof fetch;

  constructor(config: NabuGateConfig) {
    if (!config?.apiKey) {
      throw new Error('NabuGate: apiKey is required');
    }
    this.baseUrl = (config.baseUrl || DEFAULT_BASE_URL).replace(/\/+$/, '');
    this.apiKey = config.apiKey;
    this.defaultModel = config.defaultModel || 'nabu-smart';
    this.timeoutMs = config.timeoutMs ?? 120_000;
    this.extraHeaders = config.headers || {};
    this.fetchImpl = config.fetch || globalThis.fetch;
    if (!this.fetchImpl) {
      throw new Error('NabuGate: no fetch implementation available; pass one via config.fetch');
    }
  }

  /** Chat completion. Every OpenAI parameter is passed through unchanged. */
  async chat(request: ChatRequest): Promise<ChatResponse> {
    return this.post<ChatResponse>('/chat/completions', {
      ...request,
      model: request.model || this.defaultModel,
      stream: false,
    });
  }

  /** Chat completion, returning only the assistant's text. */
  async completeText(request: ChatRequest): Promise<string> {
    const data = await this.chat(request);
    const content = data.choices?.[0]?.message?.content;
    return typeof content === 'string' ? content.trim() : '';
  }

  /** Streaming chat completion, yielding text deltas as they arrive. */
  async *stream(request: ChatRequest): AsyncGenerator<string, void, unknown> {
    for await (const chunk of this.streamChunks(request)) {
      const delta = chunk?.choices?.[0]?.delta?.content;
      if (typeof delta === 'string' && delta.length > 0) yield delta;
    }
  }

  /**
   * Streaming chat completion, yielding whole SSE payloads. Use this when you
   * need tool-call deltas or finish reasons rather than plain text.
   */
  async *streamChunks(request: ChatRequest): AsyncGenerator<any, void, unknown> {
    const res = await this.send(
      '/chat/completions',
      { ...request, model: request.model || this.defaultModel, stream: true },
      { stream: true },
    );

    if (!res.body) {
      throw new NabuGateError(res.status, 'the gateway returned no response body for a stream');
    }
    const reader = (res.body as ReadableStream<Uint8Array>).getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        // The trailing partial line is carried into the next read; parsing it
        // early is how streamed SDKs drop the last token of a chunk.
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed.startsWith('data:')) continue;
          const payload = trimmed.slice(5).trim();
          if (payload === '[DONE]') return;
          try {
            yield JSON.parse(payload);
          } catch {
            // Ignore an unparseable payload rather than killing the stream.
          }
        }
      }
    } finally {
      await reader.cancel().catch(() => {});
    }
  }

  /** Embeddings. Set `dimensions` when the vectors are being stored. */
  async embeddings(request: EmbeddingsRequest): Promise<EmbeddingsResponse> {
    return this.post<EmbeddingsResponse>('/embeddings', {
      ...request,
      model: request.model || 'nabu-embed',
    });
  }

  /** Image generation. Responses carry base64 data in `data[].b64_json`. */
  async images(request: ImageRequest): Promise<ImageResponse> {
    return this.post<ImageResponse>('/images/generations', {
      ...request,
      model: request.model || 'nabu-image',
    });
  }

  /** Text to speech. Returns the raw audio bytes. */
  async speech(request: SpeechRequest): Promise<ArrayBuffer> {
    const res = await this.send('/audio/speech', {
      ...request,
      model: request.model || 'nabu-voice',
    });
    return res.arrayBuffer();
  }

  /** Every model, alias and agent this key may call. */
  async models(): Promise<ModelInfo[]> {
    const data = await this.get<{ data: ModelInfo[] }>('/models');
    return data.data ?? [];
  }

  /** Token and cost usage for this key. */
  async usage(): Promise<Record<string, unknown>> {
    return this.get<Record<string, unknown>>('/usage');
  }

  // ------------------------------------------------------------------ internals

  private headers(): Record<string, string> {
    return {
      Authorization: `Bearer ${this.apiKey}`,
      'Content-Type': 'application/json',
      ...this.extraHeaders,
    };
  }

  private async send(
    path: string,
    body?: unknown,
    opts: { method?: string; stream?: boolean } = {},
  ): Promise<Response> {
    const method = opts.method ?? (body === undefined ? 'GET' : 'POST');

    // A stream is not timed out: a long generation is the normal case, and an
    // abort mid-stream throws away tokens that were already paid for.
    const controller = opts.stream ? undefined : new AbortController();
    const timer = controller ? setTimeout(() => controller.abort(), this.timeoutMs) : undefined;

    try {
      const res = await this.fetchImpl(`${this.baseUrl}${path}`, {
        method,
        headers: this.headers(),
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller?.signal,
      });
      if (!res.ok) {
        const text = await res.text();
        throw new NabuGateError(res.status, text, errorCode(text));
      }
      return res;
    } finally {
      if (timer) clearTimeout(timer);
    }
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    return (await this.send(path, body)).json() as Promise<T>;
  }

  private async get<T>(path: string): Promise<T> {
    return (await this.send(path)).json() as Promise<T>;
  }
}

/** Pulls the gateway's error code out of an error body, when it sent one. */
function errorCode(body: string): string | undefined {
  try {
    const parsed = JSON.parse(body);
    return parsed?.error?.code ?? parsed?.error?.message ?? parsed?.error;
  } catch {
    return undefined;
  }
}
