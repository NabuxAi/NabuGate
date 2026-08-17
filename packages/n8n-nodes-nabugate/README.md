# n8n-nodes-nabugate

Official n8n community node package for **[NabuGate](https://github.com/NabuxAi/NabuGate)** — the central AI gateway and multi-agent execution engine.

Connect your n8n automation workflows directly to NabuGate's sub-agents, multi-agent flows (`seo-audit-team`, `sales-team`, etc.), routing aliases (`nabu-smart`, `nabu-fast`), embeddings, image generation, and audio synthesis.

---

## 🚀 Features

- **Multi-Agent Flows:** Run collaborative pipelines (e.g. `seo-audit-team` for On-Page audit + JSON-LD schema generation + quality scoring).
- **Sub-Agent Specialists:** Execute single-purpose sub-agents (`seo-content-auditor`, `seo-schema-engineer`, `agent-architect`, `write-composer`, `sales-drafter`).
- **Standard Chat Completions:** Query aliases (`nabu-smart`, `nabu-fast`, `nabu-cheap`) with automated upstream fallbacks.
- **Embeddings:** Vectorize text with `nabu-embed`, `write-embed`, `chat-embed`, `desk-embed`.
- **Image & Audio Generation:** Generate images and synthesize speech directly inside n8n.
- **Registry Inspection:** Fetch real-time lists of all active models, agents, and flows.

---

## 📦 Installation in n8n

### Community Nodes (Recommended)
1. In your n8n instance, go to **Settings > Community Nodes**.
2. Click **Install a community node**.
3. Enter `n8n-nodes-nabugate` and click **Install**.

### Manual / Docker Installation
```bash
npm install -g n8n-nodes-nabugate
```

---

## 🔑 Credentials Setup

1. In n8n, create a new credential of type **NabuGate API**.
2. Set **Base URL**:
   - Local: `http://localhost:8080/v1` (or your host IP in Docker: `http://host.docker.internal:8080/v1`)
   - Production / Remote: `https://your-nabugate-domain.com/v1`
3. Enter your **API Key** (`NABU_API_KEY` or project key).

---

## 💡 Example Workflows

### 1. Automated WordPress SEO Reviewer
`WordPress Trigger (New Post)` ➔ `NabuGate (Flow: seo-audit-team)` ➔ `WordPress (Update Post with JSON-LD Schema)` ➔ `Telegram Notification`

### 2. Multi-Agent Lead Qualification
`Webhook / Form Trigger` ➔ `NabuGate (Flow: sales-team)` ➔ `HubSpot / CRM Create Deal`

---

## 📄 License
MIT © [NabuX Team](https://nabuxai.com)
