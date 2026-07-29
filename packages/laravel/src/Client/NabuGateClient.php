<?php

namespace NabuGate\Client;

use Generator;
use Illuminate\Http\Client\PendingRequest;
use Illuminate\Support\Facades\Http;
use RuntimeException;

/**
 * Client for the NabuGate AI gateway.
 *
 * The gateway is OpenAI-wire compatible and passes request bodies straight
 * through to the upstream provider, so every method accepts an $extra array
 * that is merged into the body untouched. Parameters this SDK has never heard
 * of — new tool formats, provider-specific flags — reach the provider anyway.
 */
class NabuGateClient
{
    public function __construct(
        private readonly string $baseUrl,
        private readonly string $apiKey,
        private readonly string $defaultModel = 'nabu-smart',
        private readonly int $timeout = 120,
        private readonly array $headers = [],
    ) {
    }

    /**
     * Chat completion.
     *
     * @param  array<int, array<string, mixed>>  $messages
     * @param  array<string, mixed>  $extra
     * @return array<string, mixed>
     */
    public function chat(array $messages, ?string $model = null, array $extra = []): array
    {
        $response = $this->request()->post($this->url('/chat/completions'), array_merge([
            'model' => $model ?? $this->defaultModel,
            'messages' => $messages,
        ], $extra, ['stream' => false]));

        $this->assertOk($response->status(), $response->body());

        return $response->json();
    }

    /**
     * Chat completion, returning only the assistant's text.
     *
     * @param  array<int, array<string, mixed>>  $messages
     * @param  array<string, mixed>  $extra
     */
    public function completeText(array $messages, ?string $model = null, array $extra = []): string
    {
        return trim((string) data_get($this->chat($messages, $model, $extra), 'choices.0.message.content', ''));
    }

    /**
     * Streams a chat completion, invoking $onChunk with each text delta.
     *
     * @param  array<int, array<string, mixed>>  $messages
     * @param  callable(string): void  $onChunk
     * @param  array<string, mixed>  $extra
     */
    public function stream(array $messages, callable $onChunk, ?string $model = null, array $extra = []): void
    {
        foreach ($this->streamChunks($messages, $model, $extra) as $chunk) {
            $delta = data_get($chunk, 'choices.0.delta.content');
            if (is_string($delta) && $delta !== '') {
                $onChunk($delta);
            }
        }
    }

    /**
     * Streams a chat completion, yielding whole SSE payloads. Use this when you
     * need tool-call deltas or finish reasons rather than plain text.
     *
     * @param  array<int, array<string, mixed>>  $messages
     * @param  array<string, mixed>  $extra
     * @return Generator<int, array<string, mixed>>
     */
    public function streamChunks(array $messages, ?string $model = null, array $extra = []): Generator
    {
        // withOptions(['stream' => true]) hands back an unread PSR-7 body. The
        // body must actually be read here — issuing the request and returning
        // was the old bug: the callback never fired and the response was
        // discarded whole.
        $response = $this->request()
            ->withOptions(['stream' => true])
            // A stream is not timed out: a long generation is the normal case,
            // and cutting it off discards tokens that were already paid for.
            ->timeout(0)
            ->post($this->url('/chat/completions'), array_merge([
                'model' => $model ?? $this->defaultModel,
                'messages' => $messages,
            ], $extra, ['stream' => true]));

        $this->assertOk($response->status(), $response->body());

        $body = $response->toPsrResponse()->getBody();
        $buffer = '';

        while (! $body->eof()) {
            $buffer .= $body->read(8192);

            // Keep the trailing partial line for the next read; parsing it
            // early is how streaming clients lose the tail of a chunk.
            while (($newline = strpos($buffer, "\n")) !== false) {
                $line = trim(substr($buffer, 0, $newline));
                $buffer = substr($buffer, $newline + 1);

                if (! str_starts_with($line, 'data:')) {
                    continue;
                }
                $payload = trim(substr($line, 5));
                if ($payload === '[DONE]') {
                    return;
                }
                $decoded = json_decode($payload, true);
                if (is_array($decoded)) {
                    yield $decoded;
                }
            }
        }
    }

    /**
     * Creates embeddings.
     *
     * Pass $dimensions whenever the vectors are being stored: a consumer that
     * writes to a fixed-width column cannot accept whatever width the provider
     * happens to default to. Leave it null for ad-hoc search, since providers
     * without the field reject it.
     *
     * @param  string|array<int, string>  $input
     * @return array<string, mixed>
     */
    public function embeddings(string|array $input, string $model = 'nabu-embed', ?int $dimensions = null): array
    {
        $body = ['model' => $model, 'input' => $input];
        if ($dimensions !== null) {
            $body['dimensions'] = $dimensions;
        }

        $response = $this->request()->post($this->url('/embeddings'), $body);
        $this->assertOk($response->status(), $response->body());

        return $response->json();
    }

    /**
     * Generates images. Results carry base64 data in data[].b64_json.
     *
     * @param  array<string, mixed>  $extra
     * @return array<string, mixed>
     */
    public function images(string $prompt, string $model = 'nabu-image', array $extra = []): array
    {
        $response = $this->request()->post($this->url('/images/generations'), array_merge([
            'model' => $model,
            'prompt' => $prompt,
        ], $extra));

        $this->assertOk($response->status(), $response->body());

        return $response->json();
    }

    /**
     * Synthesises speech and returns the raw audio bytes.
     *
     * @param  array<string, mixed>  $extra
     */
    public function speech(string $input, string $model = 'nabu-voice', ?string $voice = null, array $extra = []): string
    {
        $body = array_merge(['model' => $model, 'input' => $input], $extra);
        if ($voice !== null) {
            $body['voice'] = $voice;
        }

        $response = $this->request()->post($this->url('/audio/speech'), $body);
        $this->assertOk($response->status(), $response->body());

        return $response->body();
    }

    /**
     * Every model, alias and agent this key may call.
     *
     * @return array<int, array<string, mixed>>
     */
    public function models(): array
    {
        $response = $this->request()->get($this->url('/models'));
        $this->assertOk($response->status(), $response->body());

        return $response->json('data', []);
    }

    /**
     * Token and cost usage for this key.
     *
     * @return array<string, mixed>
     */
    public function usage(): array
    {
        $response = $this->request()->get($this->url('/usage'));
        $this->assertOk($response->status(), $response->body());

        return $response->json();
    }

    private function request(): PendingRequest
    {
        return Http::withToken($this->apiKey)
            ->withHeaders($this->headers)
            ->timeout($this->timeout)
            ->acceptJson();
    }

    private function url(string $path): string
    {
        return rtrim($this->baseUrl, '/').$path;
    }

    private function assertOk(int $status, string $body): void
    {
        if ($status >= 400) {
            throw new RuntimeException("NabuGate request failed ({$status}): {$body}");
        }
    }
}
