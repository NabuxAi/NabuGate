/**
 * Standalone streaming helper, kept for callers that hold a base URL and key
 * rather than a client instance. New code should prefer
 * `new NabuGateClient(...).stream(...)`, which shares the same parsing.
 */
import { NabuGateClient, type ChatMessage } from './client';

export async function* streamChat(
  baseUrl: string,
  apiKey: string,
  messages: ChatMessage[],
  model = 'nabu-smart',
): AsyncGenerator<string, void, unknown> {
  const client = new NabuGateClient({ baseUrl, apiKey, defaultModel: model });
  yield* client.stream({ messages, model });
}
