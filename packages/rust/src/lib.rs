//! Official Rust client for NabuGate, the OpenAI-compatible AI gateway.
//!
//! The gateway passes request bodies through to the upstream provider untouched,
//! so [`ChatRequest`] carries an `extra` map alongside its typed fields. Anything
//! placed there reaches the provider as-is, which means a new provider parameter
//! needs no release of this crate.
//!
//! ```no_run
//! # async fn run() -> Result<(), nabugate::Error> {
//! use nabugate::{ChatRequest, Message, NabuGateClient};
//!
//! let client = NabuGateClient::new(std::env::var("NABUGATE_API_KEY").unwrap());
//! let text = client
//!     .complete_text(ChatRequest::new(vec![Message::user("Summarise this quarter.")]))
//!     .await?;
//! println!("{text}");
//! # Ok(())
//! # }
//! ```

use std::collections::HashMap;

use futures_util::StreamExt;
use serde::{Deserialize, Serialize};
use serde_json::Value;

/// The hosted gateway.
pub const DEFAULT_BASE_URL: &str = "https://gate.nabuxai.com/v1";

/// Anything that can go wrong talking to the gateway.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("nabugate: transport error: {0}")]
    Transport(#[from] reqwest::Error),
    #[error("nabugate: request failed ({status}): {body}")]
    Api { status: u16, body: String },
    #[error("nabugate: could not decode the response: {0}")]
    Decode(#[from] serde_json::Error),
}

/// One chat message. `content` is a [`Value`] so multimodal parts and tool
/// results pass through unchanged.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub role: String,
    pub content: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_call_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Value>,
}

impl Message {
    fn text(role: &str, content: impl Into<String>) -> Self {
        Self {
            role: role.to_string(),
            content: Value::String(content.into()),
            name: None,
            tool_call_id: None,
            tool_calls: None,
        }
    }

    pub fn system(content: impl Into<String>) -> Self {
        Self::text("system", content)
    }

    pub fn user(content: impl Into<String>) -> Self {
        Self::text("user", content)
    }

    pub fn assistant(content: impl Into<String>) -> Self {
        Self::text("assistant", content)
    }

    /// The message text, when the content is a plain string.
    pub fn as_text(&self) -> &str {
        self.content.as_str().unwrap_or_default()
    }
}

/// A chat completion request.
#[derive(Debug, Clone, Default, Serialize)]
pub struct ChatRequest {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    pub messages: Vec<Message>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_p: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stop: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seed: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_choice: Option<Value>,
    /// Asks the gateway to replay a stored conversation.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub conversation_id: Option<String>,

    /// Every parameter this struct does not name, merged into the body as-is.
    #[serde(flatten)]
    pub extra: HashMap<String, Value>,
}

impl ChatRequest {
    pub fn new(messages: Vec<Message>) -> Self {
        Self {
            messages,
            ..Default::default()
        }
    }

    pub fn model(mut self, model: impl Into<String>) -> Self {
        self.model = Some(model.into());
        self
    }

    pub fn temperature(mut self, temperature: f32) -> Self {
        self.temperature = Some(temperature);
        self
    }

    pub fn max_tokens(mut self, max_tokens: u32) -> Self {
        self.max_tokens = Some(max_tokens);
        self
    }

    /// Sets a parameter this struct does not name.
    pub fn param(mut self, key: impl Into<String>, value: Value) -> Self {
        self.extra.insert(key.into(), value);
        self
    }
}

/// Token consumption for a request.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct Usage {
    #[serde(default)]
    pub prompt_tokens: u32,
    #[serde(default)]
    pub completion_tokens: u32,
    #[serde(default)]
    pub total_tokens: u32,
}

/// One completion alternative.
#[derive(Debug, Clone, Deserialize)]
pub struct ChatChoice {
    #[serde(default)]
    pub index: u32,
    pub message: Message,
    #[serde(default)]
    pub finish_reason: Option<String>,
}

/// A chat completion response.
#[derive(Debug, Clone, Deserialize)]
pub struct ChatResponse {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub model: String,
    pub choices: Vec<ChatChoice>,
    #[serde(default)]
    pub usage: Usage,
}

impl ChatResponse {
    /// The first choice's text, when it is a plain string.
    pub fn text(&self) -> &str {
        self.choices
            .first()
            .map(|c| c.message.as_text())
            .unwrap_or_default()
    }
}

/// One server-sent event from a streaming completion.
#[derive(Debug, Clone, Deserialize)]
pub struct StreamChunk {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub choices: Vec<StreamChoice>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct StreamChoice {
    #[serde(default)]
    pub delta: Delta,
    #[serde(default)]
    pub finish_reason: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Delta {
    #[serde(default)]
    pub role: Option<String>,
    #[serde(default)]
    pub content: Option<String>,
    #[serde(default)]
    pub tool_calls: Option<Value>,
}

/// A vector embedding request.
#[derive(Debug, Clone, Serialize)]
pub struct EmbeddingsRequest {
    pub model: String,
    /// A string or an array of strings.
    pub input: Value,
    /// Pins the vector width. Set it whenever the vectors are stored: a
    /// fixed-width column cannot accept whatever the provider defaults to.
    /// Leave it `None` for ad-hoc search, since providers without the field
    /// reject it.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dimensions: Option<u32>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Embedding {
    #[serde(default)]
    pub index: u32,
    pub embedding: Vec<f32>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct EmbeddingsResponse {
    #[serde(default)]
    pub model: String,
    pub data: Vec<Embedding>,
    #[serde(default)]
    pub usage: Usage,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ModelInfo {
    pub id: String,
    #[serde(default)]
    pub object: String,
    #[serde(default)]
    pub owned_by: String,
}

/// Client for a NabuGate deployment.
#[derive(Clone)]
pub struct NabuGateClient {
    api_key: String,
    base_url: String,
    default_model: String,
    http: reqwest::Client,
}

impl NabuGateClient {
    /// Builds a client against the hosted gateway.
    pub fn new(api_key: impl Into<String>) -> Self {
        Self {
            api_key: api_key.into(),
            base_url: DEFAULT_BASE_URL.to_string(),
            default_model: "nabu-smart".to_string(),
            http: reqwest::Client::new(),
        }
    }

    /// Points the client at a different deployment.
    pub fn base_url(mut self, base_url: impl Into<String>) -> Self {
        self.base_url = base_url.into().trim_end_matches('/').to_string();
        self
    }

    /// Sets the alias or agent used when a request names none.
    pub fn default_model(mut self, model: impl Into<String>) -> Self {
        self.default_model = model.into();
        self
    }

    /// Supplies a pre-configured HTTP client (proxies, tracing, timeouts).
    pub fn http_client(mut self, http: reqwest::Client) -> Self {
        self.http = http;
        self
    }

    /// Performs a chat completion.
    pub async fn chat(&self, request: ChatRequest) -> Result<ChatResponse, Error> {
        let body = self.chat_body(request, false)?;
        self.post_json("/chat/completions", &body).await
    }

    /// Performs a chat completion and returns only the text.
    pub async fn complete_text(&self, request: ChatRequest) -> Result<String, Error> {
        Ok(self.chat(request).await?.text().trim().to_string())
    }

    /// Streams a chat completion, invoking `on_chunk` for each event.
    ///
    /// Returning `false` from the callback stops the stream.
    pub async fn stream<F>(&self, request: ChatRequest, mut on_chunk: F) -> Result<(), Error>
    where
        F: FnMut(StreamChunk) -> bool,
    {
        let body = self.chat_body(request, true)?;
        let response = self.send("/chat/completions", &body).await?;
        let mut stream = response.bytes_stream();
        let mut buffer = String::new();

        while let Some(bytes) = stream.next().await {
            buffer.push_str(&String::from_utf8_lossy(&bytes?));

            // Keep the trailing partial line for the next read; parsing it early
            // is how streaming clients drop the tail of a chunk.
            while let Some(newline) = buffer.find('\n') {
                let line = buffer[..newline].trim().to_string();
                buffer.drain(..=newline);

                let Some(payload) = line.strip_prefix("data:") else {
                    continue;
                };
                let payload = payload.trim();
                if payload == "[DONE]" {
                    return Ok(());
                }
                // Skip an unparseable payload rather than failing the stream.
                if let Ok(chunk) = serde_json::from_str::<StreamChunk>(payload) {
                    if !on_chunk(chunk) {
                        return Ok(());
                    }
                }
            }
        }
        Ok(())
    }

    /// Streams a chat completion, reduced to text deltas.
    pub async fn stream_text<F>(&self, request: ChatRequest, mut on_text: F) -> Result<(), Error>
    where
        F: FnMut(&str) -> bool,
    {
        self.stream(request, |chunk| {
            for choice in &chunk.choices {
                if let Some(content) = &choice.delta.content {
                    if !content.is_empty() && !on_text(content) {
                        return false;
                    }
                }
            }
            true
        })
        .await
    }

    /// Creates embeddings.
    pub async fn embeddings(
        &self,
        input: Value,
        model: Option<&str>,
        dimensions: Option<u32>,
    ) -> Result<EmbeddingsResponse, Error> {
        let request = EmbeddingsRequest {
            model: model.unwrap_or("nabu-embed").to_string(),
            input,
            dimensions,
        };
        self.post_json("/embeddings", &request).await
    }

    /// Generates images. Results carry base64 data in `data[].b64_json`.
    pub async fn images(&self, prompt: &str, model: Option<&str>) -> Result<Value, Error> {
        let body = serde_json::json!({
            "model": model.unwrap_or("nabu-image"),
            "prompt": prompt,
        });
        self.post_json("/images/generations", &body).await
    }

    /// Synthesises speech and returns the raw audio bytes.
    pub async fn speech(
        &self,
        input: &str,
        model: Option<&str>,
        voice: Option<&str>,
    ) -> Result<Vec<u8>, Error> {
        let mut body = serde_json::json!({
            "model": model.unwrap_or("nabu-voice"),
            "input": input,
        });
        if let Some(voice) = voice {
            body["voice"] = Value::String(voice.to_string());
        }
        let response = self.send("/audio/speech", &body).await?;
        Ok(response.bytes().await?.to_vec())
    }

    /// Lists every model, alias and agent this key may call.
    pub async fn models(&self) -> Result<Vec<ModelInfo>, Error> {
        #[derive(Deserialize)]
        struct Catalogue {
            #[serde(default)]
            data: Vec<ModelInfo>,
        }
        let catalogue: Catalogue = self.get_json("/models").await?;
        Ok(catalogue.data)
    }

    /// Returns token and cost usage for this key.
    pub async fn usage(&self) -> Result<Value, Error> {
        self.get_json("/usage").await
    }

    // ------------------------------------------------------------ internals

    fn chat_body(&self, mut request: ChatRequest, stream: bool) -> Result<Value, Error> {
        if request.model.is_none() {
            request.model = Some(self.default_model.clone());
        }
        let mut body = serde_json::to_value(request)?;
        if let Some(map) = body.as_object_mut() {
            map.insert("stream".to_string(), Value::Bool(stream));
        }
        Ok(body)
    }

    async fn send<B: Serialize>(&self, path: &str, body: &B) -> Result<reqwest::Response, Error> {
        let response = self
            .http
            .post(format!("{}{}", self.base_url, path))
            .bearer_auth(&self.api_key)
            .json(body)
            .send()
            .await?;
        Self::check(response).await
    }

    async fn check(response: reqwest::Response) -> Result<reqwest::Response, Error> {
        let status = response.status();
        if status.is_success() {
            return Ok(response);
        }
        let body = response.text().await.unwrap_or_default();
        Err(Error::Api {
            status: status.as_u16(),
            body,
        })
    }

    async fn post_json<B: Serialize, T: for<'de> Deserialize<'de>>(
        &self,
        path: &str,
        body: &B,
    ) -> Result<T, Error> {
        let text = self.send(path, body).await?.text().await?;
        Ok(serde_json::from_str(&text)?)
    }

    async fn get_json<T: for<'de> Deserialize<'de>>(&self, path: &str) -> Result<T, Error> {
        let response = self
            .http
            .get(format!("{}{}", self.base_url, path))
            .bearer_auth(&self.api_key)
            .send()
            .await?;
        let text = Self::check(response).await?.text().await?;
        Ok(serde_json::from_str(&text)?)
    }
}
