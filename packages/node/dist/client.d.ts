export interface NabuGateConfig {
    baseUrl?: string;
    apiKey: string;
    defaultModel?: string;
}
export interface ChatMessage {
    role: 'system' | 'user' | 'assistant';
    content: string;
}
export declare class NabuGateClient {
    private baseUrl;
    private apiKey;
    private defaultModel;
    constructor(config: NabuGateConfig);
    chat(messages: ChatMessage[], model?: string, temperature?: number): Promise<any>;
    completeText(messages: ChatMessage[], model?: string, temperature?: number): Promise<string>;
}
