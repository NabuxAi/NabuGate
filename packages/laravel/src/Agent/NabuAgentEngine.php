<?php

namespace NabuGate\Agent;

use NabuGate\Client\NabuGateClient;
use RuntimeException;

class NabuAgentEngine
{
    public function __construct(private readonly NabuGateClient $client)
    {
    }

    /**
     * Run an agent execution loop with provided tools and callback registry.
     *
     * @param list<array{role: string, content: string}> $messages
     * @param list<array> $tools
     * @param array<string, callable> $toolHandlers
     */
    public function run(
        array $messages,
        array $tools,
        array $toolHandlers,
        int $maxSteps = 5
    ): string {
        $history = $messages;

        for ($step = 0; $step < $maxSteps; $step++) {
            $response = $this->client->chat($history);
            $choice = data_get($response, 'choices.0.message');

            if (! $choice) {
                throw new RuntimeException('Agent received empty response from NabuGate.');
            }

            $content = $choice['content'] ?? '';
            $toolCalls = $choice['tool_calls'] ?? [];

            if (empty($toolCalls)) {
                return $content;
            }

            // Append assistant message with tool calls
            $history[] = [
                'role' => 'assistant',
                'content' => $content,
                'tool_calls' => $toolCalls,
            ];

            // Execute registered tool handlers
            foreach ($toolCalls as $call) {
                $toolName = $call['function']['name'] ?? '';
                $rawArgs = $call['function']['arguments'] ?? '{}';
                $args = json_decode($rawArgs, true) ?? [];

                if (isset($toolHandlers[$toolName])) {
                    $result = call_user_func($toolHandlers[$toolName], $args);
                    $history[] = [
                        'role' => 'tool',
                        'tool_call_id' => $call['id'] ?? '',
                        'content' => is_string($result) ? $result : json_encode($result),
                    ];
                } else {
                    $history[] = [
                        'role' => 'tool',
                        'tool_call_id' => $call['id'] ?? '',
                        'content' => json_encode(['error' => "Tool {$toolName} not registered"]),
                    ];
                }
            }
        }

        return $content;
    }
}
