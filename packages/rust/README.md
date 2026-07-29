# nabugate

Official Rust client for **NabuGate**, the organisation's OpenAI-compatible AI
gateway. Projects call NabuGate with an alias such as `nabu-fast`; the gateway
picks the provider, falls back on failure, holds the secrets and meters the cost.

```toml
[dependencies]
nabugate = "1"
```

## Use

```rust
use nabugate::{ChatRequest, Message, NabuGateClient};

let client = NabuGateClient::new(std::env::var("NABUGATE_API_KEY")?);

let text = client
    .complete_text(ChatRequest::new(vec![Message::user("Summarise this quarter.")]).model("nabu-fast"))
    .await?;

// Streaming
client
    .stream_text(ChatRequest::new(vec![Message::user("Write a haiku.")]), |delta| {
        print!("{delta}");
        true
    })
    .await?;

// Embeddings — pin the width whenever you store the vectors
let vectors = client.embeddings(json!(["a", "b"]), Some("write-embed"), Some(1536)).await?;

// Images, speech, catalogue
let image = client.images("a lighthouse at dusk", None).await?;
let audio = client.speech("Welcome back.", None, None).await?;
let models = client.models().await?;
```

## Sub-agents

A named assistant is called exactly like a model — pass its name to `.model()`.

## Everything passes through

The gateway forwards request bodies to the provider untouched. `ChatRequest`
names the common fields and `.param()` sets anything else:

```rust
ChatRequest::new(messages)
    .param("response_format", json!({"type": "json_object"}))
    .param("frequency_penalty", json!(0.5));
```

## Options

`NabuGateClient::new` is chained with `.base_url()`, `.default_model()` and
`.http_client()`. Failures return `nabugate::Error`, whose `Api` variant carries
the status and body.

MIT licensed.
