# Changelog

## 1.0.0

First public release.

Covers the whole NabuGate surface: chat with streaming, embeddings, image
generation, text to speech, the model catalogue and usage. Sub-agents are called
like any other model.

Request bodies pass through to the upstream provider untouched, so parameters
this SDK does not name — `tools`, `tool_choice`, `response_format`, `seed`,
penalties — reach the provider anyway and need no release here to become usable.
