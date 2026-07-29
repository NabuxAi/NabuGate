export declare function streamChat(baseUrl: string, apiKey: string, messages: Array<{
    role: string;
    content: string;
}>, model?: string): AsyncGenerator<string, void, unknown>;
