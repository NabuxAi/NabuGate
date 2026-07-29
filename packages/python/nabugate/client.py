"""NabuGate client.

The gateway is OpenAI-wire compatible and passes request bodies straight through
to the upstream provider, so every method here accepts arbitrary keyword
arguments and forwards them. Parameters this SDK has never heard of — new tool
formats, provider-specific flags — reach the provider anyway.
"""

from __future__ import annotations

import json
from typing import Any, Dict, Iterator, List, Optional, Union

import requests

DEFAULT_BASE_URL = "https://gate.nabuxai.com/v1"
DEFAULT_TIMEOUT = 120

Messages = List[Dict[str, Any]]


class NabuGateError(RuntimeError):
    """Raised for any non-2xx response from the gateway."""

    def __init__(self, status_code: int, body: str) -> None:
        super().__init__(f"NabuGate request failed ({status_code}): {body}")
        self.status_code = status_code
        self.body = body

    @property
    def code(self) -> Optional[str]:
        """The gateway's error code, when the body carried one."""
        try:
            error = json.loads(self.body).get("error")
        except (ValueError, AttributeError):
            return None
        if isinstance(error, dict):
            return error.get("code") or error.get("message")
        return error if isinstance(error, str) else None


class NabuGateClient:
    """Client for the NabuGate AI gateway."""

    def __init__(
        self,
        api_key: str,
        base_url: str = DEFAULT_BASE_URL,
        default_model: str = "nabu-smart",
        timeout: int = DEFAULT_TIMEOUT,
        headers: Optional[Dict[str, str]] = None,
        session: Optional[requests.Session] = None,
    ) -> None:
        if not api_key:
            raise ValueError("api_key is required")
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")
        self.default_model = default_model
        self.timeout = timeout
        self.extra_headers = headers or {}
        # A shared session keeps the TLS connection alive between calls, which
        # matters as soon as a loop issues hundreds of completions.
        self.session = session or requests.Session()

    # ------------------------------------------------------------------ chat

    def chat(self, messages: Messages, model: Optional[str] = None, **params: Any) -> Dict[str, Any]:
        """Chat completion. Extra keyword arguments are passed through untouched."""
        body = {"model": model or self.default_model, "messages": messages, **params}
        body["stream"] = False
        return self._post("/chat/completions", body).json()

    def complete_text(self, messages: Messages, model: Optional[str] = None, **params: Any) -> str:
        """Chat completion, returning only the assistant's text."""
        data = self.chat(messages, model=model, **params)
        choices = data.get("choices") or []
        if not choices:
            return ""
        content = (choices[0].get("message") or {}).get("content")
        return content.strip() if isinstance(content, str) else ""

    def stream(self, messages: Messages, model: Optional[str] = None, **params: Any) -> Iterator[str]:
        """Stream a chat completion, yielding text deltas as they arrive."""
        for chunk in self.stream_chunks(messages, model=model, **params):
            choices = chunk.get("choices") or []
            if not choices:
                continue
            delta = (choices[0].get("delta") or {}).get("content")
            if delta:
                yield delta

    def stream_chunks(
        self, messages: Messages, model: Optional[str] = None, **params: Any
    ) -> Iterator[Dict[str, Any]]:
        """Stream a chat completion, yielding whole SSE payloads.

        Use this when you need tool-call deltas or finish reasons rather than
        plain text.
        """
        body = {"model": model or self.default_model, "messages": messages, **params}
        body["stream"] = True
        # No read timeout on a stream: a long generation is the normal case, and
        # timing out mid-stream discards tokens that were already paid for.
        response = self._post("/chat/completions", body, stream=True, timeout=(self.timeout, None))
        try:
            for line in response.iter_lines(decode_unicode=True):
                if not line:
                    continue
                line = line.strip()
                if not line.startswith("data:"):
                    continue
                payload = line[5:].strip()
                if payload == "[DONE]":
                    return
                try:
                    yield json.loads(payload)
                except ValueError:
                    # Skip an unparseable payload rather than killing the stream.
                    continue
        finally:
            response.close()

    # ------------------------------------------------------------ embeddings

    def embeddings(
        self,
        input: Union[str, List[str]],
        model: str = "nabu-embed",
        dimensions: Optional[int] = None,
        **params: Any,
    ) -> Dict[str, Any]:
        """Create embeddings.

        Pass ``dimensions`` whenever the vectors are being stored: a consumer
        that writes to a fixed-width column cannot accept whatever width the
        provider happens to default to. Leave it unset for ad-hoc search, since
        providers that do not support the field reject it outright.
        """
        body: Dict[str, Any] = {"model": model, "input": input, **params}
        if dimensions is not None:
            body["dimensions"] = dimensions
        return self._post("/embeddings", body).json()

    # ----------------------------------------------------------------- media

    def images(self, prompt: str, model: str = "nabu-image", **params: Any) -> Dict[str, Any]:
        """Generate images. Results carry base64 data in ``data[].b64_json``."""
        return self._post("/images/generations", {"model": model, "prompt": prompt, **params}).json()

    def speech(
        self, input: str, model: str = "nabu-voice", voice: Optional[str] = None, **params: Any
    ) -> bytes:
        """Synthesise speech and return the raw audio bytes."""
        body: Dict[str, Any] = {"model": model, "input": input, **params}
        if voice:
            body["voice"] = voice
        return self._post("/audio/speech", body).content

    # ------------------------------------------------------------- catalogue

    def models(self) -> List[Dict[str, Any]]:
        """Every model, alias and agent this key may call."""
        return self._get("/models").json().get("data", [])

    def usage(self) -> Dict[str, Any]:
        """Token and cost usage for this key."""
        return self._get("/usage").json()

    # -------------------------------------------------------------- internals

    def _headers(self) -> Dict[str, str]:
        return {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            **self.extra_headers,
        }

    def _post(self, path: str, body: Dict[str, Any], **kwargs: Any) -> requests.Response:
        kwargs.setdefault("timeout", self.timeout)
        response = self.session.post(
            f"{self.base_url}{path}", json=body, headers=self._headers(), **kwargs
        )
        return self._check(response)

    def _get(self, path: str) -> requests.Response:
        response = self.session.get(
            f"{self.base_url}{path}", headers=self._headers(), timeout=self.timeout
        )
        return self._check(response)

    @staticmethod
    def _check(response: requests.Response) -> requests.Response:
        if response.status_code >= 400:
            raise NabuGateError(response.status_code, response.text)
        return response

    def close(self) -> None:
        self.session.close()

    def __enter__(self) -> "NabuGateClient":
        return self

    def __exit__(self, *exc: Any) -> None:
        self.close()
