"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.NabuGateClient = void 0;
class NabuGateClient {
    baseUrl;
    apiKey;
    defaultModel;
    constructor(config) {
        this.baseUrl = config.baseUrl || 'https://gate.nabuxai.com/v1';
        this.apiKey = config.apiKey;
        this.defaultModel = config.defaultModel || 'nabu-smart';
    }
    async chat(messages, model, temperature = 0.7) {
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
    async completeText(messages, model, temperature = 0.7) {
        const data = await this.chat(messages, model, temperature);
        return data.choices?.[0]?.message?.content?.trim() || '';
    }
}
exports.NabuGateClient = NabuGateClient;
