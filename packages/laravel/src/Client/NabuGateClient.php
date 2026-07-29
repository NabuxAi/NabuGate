<?php

namespace NabuGate\Client;

use Illuminate\Support\Facades\Http;
use RuntimeException;

class NabuGateClient
{
    public function __construct(
        private readonly string $baseUrl,
        private readonly string $apiKey,
        private readonly string $defaultModel = 'nabu-smart',
    ) {
    }

    public function chat(array $messages, ?string $model = null, float $temperature = 0.7): array
    {
        $response = Http::withToken($this->apiKey)
            ->timeout(120)
            ->acceptJson()
            ->post(rtrim($this->baseUrl, '/') . '/chat/completions', [
                'model' => $model ?? $this->defaultModel,
                'messages' => $messages,
                'temperature' => $temperature,
            ]);

        if ($response->failed()) {
            throw new RuntimeException("NabuGate request failed ({$response->status()}): " . $response->body());
        }

        return $response->json();
    }

    public function completeText(array $messages, ?string $model = null, float $temperature = 0.7): string
    {
        $json = $this->chat($messages, $model, $temperature);
        return trim((string) data_get($json, 'choices.0.message.content', ''));
    }
}
