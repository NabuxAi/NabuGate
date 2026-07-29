<?php

namespace NabuGate\Services;

use NabuGate\Client\NabuGateClient;

class StoryWriter
{
    public function __construct(private readonly NabuGateClient $client)
    {
    }

    public function narrative(string $poiName, string $locale = 'fa', string $brief = '', string $existing = ''): string
    {
        $language = match ($locale) {
            'fa' => 'Persian (Farsi)',
            'ar' => 'Arabic',
            default => 'English',
        };

        $user = "Write a spoken-word audio-tour narration for a point of interest"
            . ($poiName !== '' ? " called \"{$poiName}\"" : '')
            . ". Language: {$language}. 90-160 words."
            . " Tell it as a story: open with a hook that pulls the listener in, build a small narrative arc"
            . " through the place's history or life, and close with an image or thought that lingers."
            . " Warm and vivid, address the listener directly in the second person, natural spoken pacing."
            . " Return only the narration text.";

        if ($brief !== '') {
            $user .= "\n\nFacts / brief to base it on:\n{$brief}";
        }

        return $this->client->completeText([
            ['role' => 'system', 'content' => "You are a master storyteller and audio-tour narrator. You write only in {$language}."],
            ['role' => 'user', 'content' => $user],
        ]);
    }
}
