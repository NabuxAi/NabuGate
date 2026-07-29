use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct Message {
    pub role: String,
    pub content: String,
}

#[derive(Debug, Serialize)]
pub struct ChatRequest {
    pub model: String,
    pub messages: Vec<Message>,
    pub temperature: f32,
}

#[derive(Debug, Deserialize)]
pub struct ChatChoice {
    pub message: Message,
}

#[derive(Debug, Deserialize)]
pub struct ChatResponse {
    pub choices: Vec<ChatChoice>,
}

#[derive(Clone)]
pub struct NabuGateClient {
    pub api_key: String,
    pub base_url: String,
    pub default_model: String,
    client: reqwest::Client,
}

impl NabuGateClient {
    pub fn new(api_key: String) -> Self {
        Self {
            api_key,
            base_url: "https://gate.nabuxai.com/v1".to_string(),
            default_model: "nabu-smart".to_string(),
            client: reqwest::Client::new(),
        }
    }

    pub async fn chat(
        &self,
        messages: Vec<Message>,
        model: Option<String>,
        temperature: Option<f32>,
    ) -> Result<ChatResponse, reqwest::Error> {
        let url = format!("{}/chat/completions", self.base_url.trim_end_matches('/'));
        let req = ChatRequest {
            model: model.unwrap_or_else(|| self.default_model.clone()),
            messages,
            temperature: temperature.unwrap_or(0.7),
        };

        let res = self
            .client
            .post(&url)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .json(&req)
            .send()
            .await?
            .json::<ChatResponse>()
            .await?;

        Ok(res)
    }
}
