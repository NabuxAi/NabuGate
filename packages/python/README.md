# nabugate

Official Python client for **NabuGate**, the organisation's OpenAI-compatible AI
gateway. Projects call NabuGate with an alias such as `nabu-fast`; the gateway
picks the provider, falls back on failure, holds the secrets and meters the cost.

```bash
pip install nabugate
```

## Use

```python
from nabugate import NabuGateClient

nabu = NabuGateClient(api_key=os.environ["NABUGATE_API_KEY"])

# Chat
answer = nabu.complete_text([{"role": "user", "content": "Summarise this quarter."}], model="nabu-fast")

# Streaming
for delta in nabu.stream([{"role": "user", "content": "Write a haiku."}]):
    print(delta, end="", flush=True)

# Embeddings — pin `dimensions` whenever you store the vectors
vectors = nabu.embeddings(["a", "b"], model="write-embed", dimensions=1536)

# Images, speech, catalogue
image = nabu.images("a lighthouse at dusk")      # data[].b64_json
audio = nabu.speech("Welcome back.", voice="alloy")
models = nabu.models()
```

## Sub-agents

A named assistant is called exactly like a model:

```python
nabu.chat([{"role": "user", "content": "Storyboard this."}], model="cine-motion-designer")
```

## Everything passes through

The gateway forwards request bodies to the provider untouched, so any keyword
argument you pass reaches the provider whether or not this SDK names it:

```python
nabu.chat(
    messages,
    tools=[{"type": "function", "function": {"name": "get_weather", "parameters": {}}}],
    response_format={"type": "json_object"},
    seed=7,
)
```

## Options

| Argument | Default | Notes |
|---|---|---|
| `base_url` | `https://gate.nabuxai.com/v1` | Point at your own deployment |
| `default_model` | `nabu-smart` | Used when a request names none |
| `timeout` | `120` | Streaming requests are never read-timed-out |
| `headers` | `{}` | Sent with every request |
| `session` | new session | Supply your own `requests.Session` |

Failures raise `NabuGateError` carrying `status_code`, `body` and `code`.

MIT licensed.
