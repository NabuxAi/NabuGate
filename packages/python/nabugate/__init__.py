"""Official Python SDK for NabuGate, the OpenAI-compatible AI gateway."""

from .client import DEFAULT_BASE_URL, NabuGateClient, NabuGateError

__all__ = ["NabuGateClient", "NabuGateError", "DEFAULT_BASE_URL"]
__version__ = "1.0.0"
