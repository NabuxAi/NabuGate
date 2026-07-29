import requests
from typing import List, Dict, Any, Optional

class NabuGateClient:
    def __init__(self, api_key: str, base_url: str = "https://gate.nabuxai.com/v1", default_model: str = "nabu-smart"):
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")
        self.default_model = default_model

    def chat(self, messages: List[Dict[str, str]], model: Optional[str] = None, temperature: float = 0.7) -> Dict[str, Any]:
        url = f"{self.base_url}/chat/completions"
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json"
        }
        payload = {
            "model": model or self.default_model,
            "messages": messages,
            "temperature": temperature
        }
        response = requests.post(url, json=payload, headers=headers, timeout=120)
        response.raise_for_status()
        return response.json()

    def complete_text(self, messages: List[Dict[str, str]], model: Optional[str] = None, temperature: float = 0.7) -> str:
        data = self.chat(messages, model=model, temperature=temperature)
        choices = data.get("choices", [])
        if choices and "message" in choices[0]:
            return choices[0]["message"].get("content", "").strip()
        return ""
