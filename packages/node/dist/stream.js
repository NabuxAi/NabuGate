"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.streamChat = streamChat;
// Stream response helper for Node.js/TypeScript SDK
async function* streamChat(baseUrl, apiKey, messages, model = 'nabu-smart') {
    const res = await fetch(`${baseUrl.replace(/\/$/, '')}/chat/completions`, {
        method: 'POST',
        headers: {
            'Authorization': `Bearer ${apiKey}`,
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            model,
            messages,
            stream: true,
        }),
    });
    if (!res.ok || !res.body) {
        throw new Error(`NabuGate stream failed (${res.status})`);
    }
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
        const { done, value } = await reader.read();
        if (done)
            break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';
        for (const line of lines) {
            const trimmed = line.trim();
            if (trimmed.startsWith('data: ')) {
                const dataStr = trimmed.slice(6);
                if (dataStr === '[DONE]')
                    return;
                try {
                    const parsed = JSON.parse(dataStr);
                    const chunk = parsed.choices?.[0]?.delta?.content;
                    if (chunk)
                        yield chunk;
                }
                catch (e) {
                    // ignore chunk parse errors
                }
            }
        }
    }
}
