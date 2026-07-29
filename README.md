# 🌌 NabuGate AI & Voice Gateway SDK Monorepo

Official multi-language SDK suite for **NabuGate AI Gateway** (`gate.nabuxai.com`).  
Provides unified OpenAI-compatible Chat Completions, Text-to-Speech (TTS), Tour Story Generation, and Model Context Protocol (MCP) integrations for Laravel, PHP, Node.js/TypeScript, Python, and Dart/Flutter.

---

## 📦 Packages Included

- **`packages/laravel`**: Laravel Package (`nabux/nabugate-laravel`) with ServiceProvider, Facades, StoryWriter, TTS, and Filament integrations.
- **`packages/php`**: Standalone PHP 8.2+ SDK (`nabux/nabugate-php`).
- **`packages/node`**: Node.js & TypeScript SDK (`@nabugate/sdk`).
- **`packages/python`**: Python SDK (`nabugate`).
- **`packages/dart`**: Dart & Flutter SDK (`nabugate_sdk`).

---

## ⚡ Quickstart

### Default Endpoint & Authentication
- **Gateway Base URL**: `https://gate.nabuxai.com/v1`
- **Default Models**: `nabu-smart`, `nabu-fast`, `nabu-story`, `nabu-voice`, `nabu-vision`

---

## 🚀 Laravel Package Example

```php
use NabuGate\Facades\NabuGate;

// Generate tour narration story in Persian
$story = NabuGate::story()->narrative('Qurum Natural Park', 'fa', 'Scenic park with lake');

// Chat completions
$response = NabuGate::chat()->complete([
    'model' => 'nabu-smart',
    'messages' => [
        ['role' => 'user', 'content' => 'Hello NabuGate!']
    ]
]);
```
