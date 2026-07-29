<?php

return [
    'base_url' => env('NABUGATE_BASE_URL', 'https://gate.nabuxai.com/v1'),
    'api_key' => env('NABUGATE_API_KEY'),
    'default_model' => env('NABUGATE_MODEL', 'nabu-smart'),
    'tts' => [
        'base_url' => env('ELEVENLABS_BASE_URL', 'https://api.elevenlabs.io/v1'),
        'api_key' => env('ELEVENLABS_API_KEY'),
        'voice_id' => env('ELEVENLABS_VOICE_ID'),
        'model' => env('ELEVENLABS_MODEL', 'eleven_multilingual_v2'),
        'format' => env('ELEVENLABS_OUTPUT_FORMAT', 'mp3_44100_128'),
    ],
];
