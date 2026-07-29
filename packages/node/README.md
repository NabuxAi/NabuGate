# @nabugate/sdk

Official Node.js and TypeScript client for **NabuGate**, the organisation's
OpenAI-compatible AI gateway. Projects call NabuGate with an alias such as
`nabu-fast`; the gateway picks the provider, falls back on failure, holds the
secrets and meters the cost.

```bash
npm install @nabugate/sdk
```

## Use

```ts
import { NabuGateClient } from '@nabugate/sdk';

const nabu = new NabuGateClient({ apiKey: process.env.NABUGATE_API_KEY! });

// Chat
const answer = await nabu.completeText({
  model: 'nabu-fast',
  messages: [{ role: 'user', content: 'Summarise this quarter in one line.' }],
});

// Streaming
for await (const delta of nabu.stream({ messages: [{ role: 'user', content: 'Write a haiku.' }] })) {
  process.stdout.write(delta);
}

// Embeddings — pin `dimensions` whenever you store the vectors
const { data } = await nabu.embeddings({ model: 'write-embed', input: ['a', 'b'], dimensions: 1536 });

// Images, speech, catalogue
const image = await nabu.images({ prompt: 'a lighthouse at dusk' });   // data[].b64_json
const audio = await nabu.speech({ input: 'Welcome back.', voice: 'alloy' });
const models = await nabu.models();
```

## Sub-agents

A named assistant is called exactly like a model:

```ts
await nabu.chat({ model: 'cine-motion-designer', messages: [{ role: 'user', content: 'Storyboard this.' }] });
```

## Everything passes through

The gateway forwards request bodies to the provider untouched, so any parameter
you set — `tools`, `tool_choice`, `response_format`, `seed`, penalties — reaches
the provider whether or not this SDK names it. `tool_calls` come back in the
response.

```ts
await nabu.chat({
  messages,
  tools: [{ type: 'function', function: { name: 'get_weather', parameters: {} } }],
  response_format: { type: 'json_object' },
});
```

## Options

| Option | Default | Notes |
|---|---|---|
| `baseUrl` | `https://gate.nabuxai.com/v1` | Point at your own deployment |
| `defaultModel` | `nabu-smart` | Used when a request names none |
| `timeoutMs` | `120000` | Streaming requests are never timed out |
| `headers` | `{}` | Sent with every request |

Failures throw `NabuGateError` carrying `status`, `body` and the gateway's
`code`.

MIT licensed.
