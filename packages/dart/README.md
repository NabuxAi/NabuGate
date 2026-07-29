# nabugate_sdk

Official Dart and Flutter client for **NabuGate**, the organisation's
OpenAI-compatible AI gateway. Projects call NabuGate with an alias such as
`nabu-fast`; the gateway picks the provider, falls back on failure, holds the
secrets and meters the cost.

```bash
dart pub add nabugate_sdk
```

## Use

```dart
final nabu = NabuGateClient(apiKey: Platform.environment['NABUGATE_API_KEY']!);

// Chat
final answer = await nabu.completeText(
  [const Message.user('Summarise this quarter.')],
  model: 'nabu-fast',
);

// Streaming
await for (final delta in nabu.stream([const Message.user('Write a haiku.')])) {
  stdout.write(delta);
}

// Embeddings — pin `dimensions` whenever you store the vectors
final vectors = await nabu.embeddings(['a', 'b'], model: 'write-embed', dimensions: 1536);

// Images, speech, catalogue
final image = await nabu.images('a lighthouse at dusk');   // data[].b64_json
final audio = await nabu.speech('Welcome back.', voice: 'alloy');
final models = await nabu.models();
```

## Sub-agents

A named assistant is called exactly like a model — pass its name as `model`.

## Everything passes through

The gateway forwards request bodies to the provider untouched. Anything in
`extra` reaches the provider whether or not this SDK names it:

```dart
await nabu.chat(messages, extra: {
  'tools': [tool],
  'response_format': {'type': 'json_object'},
});
```

## Options

`NabuGateClient` takes `baseUrl`, `defaultModel`, `timeout`, `headers` and an
optional `httpClient`. Streams are never timed out. Failures throw
`NabuGateException` carrying `statusCode`, `body` and `code`.

MIT licensed.
