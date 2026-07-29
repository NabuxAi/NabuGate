# nabugate-go

Official Go client for **NabuGate**, the organisation's OpenAI-compatible AI
gateway. Projects call NabuGate with an alias such as `nabu-fast`; the gateway
picks the provider, falls back on failure, holds the secrets and meters the cost.

```bash
go get github.com/nabuxai/nabugate-go
```

## Use

```go
client := nabugate.New(os.Getenv("NABUGATE_API_KEY"))

text, err := client.CompleteText(ctx, nabugate.ChatRequest{
    Model:    "nabu-fast",
    Messages: []nabugate.Message{nabugate.Text("user", "Summarise this quarter.")},
})

// Streaming
err = client.StreamText(ctx, nabugate.ChatRequest{
    Messages: []nabugate.Message{nabugate.Text("user", "Write a haiku.")},
}, func(delta string) error {
    fmt.Print(delta)
    return nil
})

// Embeddings — pin Dimensions whenever you store the vectors
vectors, err := client.Embeddings(ctx, nabugate.EmbeddingsRequest{
    Model: "write-embed", Input: []string{"a", "b"}, Dimensions: nabugate.Int(1536),
})

// Images, speech, catalogue
image, err := client.Images(ctx, nabugate.ImageRequest{Prompt: "a lighthouse at dusk"})
audio, err := client.Speech(ctx, nabugate.SpeechRequest{Input: "Welcome back."})
models, err := client.Models(ctx)
```

## Sub-agents

A named assistant is called exactly like a model — set `Model` to its name.

## Everything passes through

The gateway forwards request bodies to the provider untouched. `ChatRequest`
names the common fields and carries an `Extra` map for everything else, so a new
provider parameter needs no release of this package:

```go
req := nabugate.ChatRequest{
    Messages: messages,
    Tools:    []any{tool},
    Extra: map[string]any{
        "response_format":   map[string]string{"type": "json_object"},
        "frequency_penalty": 0.5,
    },
}
```

## Options

`New` takes `WithBaseURL`, `WithDefaultModel`, `WithHTTPClient` and `WithHeader`.
There is no client-level timeout — streams are long-lived by design, so pass a
deadline on the context when you want one.

Failures return `*nabugate.Error` carrying `StatusCode` and `Body`.

MIT licensed.
