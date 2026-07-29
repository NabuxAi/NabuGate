export interface NabuGateConfig {
  baseUrl?: string;
  apiKey: string;
  defaultModel?: string;
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

export class NabuGateClient {
  private baseUrl: string;
  private apiKey: string;
  private defaultModel: string;

  constructor(config: NabuGateConfig) {
    this.baseUrl = config.baseUrl || 'https://gate.nabuxai.com/v1';
    this.apiKey = config.apiKey;
    this.defaultModel = config.defaultModel || 'nabu-smart';
  }

  async chat(messages: ChatMessage[], model?: string, temperature = 0.7): Promise<any> {
    const res = await fetch(`${this.baseUrl.replace(/\/$/, '')}/chat/completions`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.apiKey}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: model || this.defaultModel,
        messages,
        temperature,
      }),
    });

    if (!res.ok) {
      throw new Error(`NabuGate request failed (${res.status}): ${await res.text()}`);
    }

    return await res.json();
  }

  async completeText(messages: ChatMessage[], model?: string, temperature = 0.7): Promise<string> {
    const data = await this.chat(messages, model, temperature);
    return data.choices?.[0]?.message?.content?.trim() || '';
  }
}
