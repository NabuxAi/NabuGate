<?php

return [
    'base_url' => env('NABUGATE_BASE_URL', 'https://gate.nabuxai.com/v1'),
    'api_key' => env('NABUGATE_API_KEY'),
    'default_model' => env('NABUGATE_MODEL', 'nabu-smart'),
    
    // Central SSO for Nabu Ecosystem apps (NabuDesk, NabuGen, NabuGate, NabuVoice)
    // Consumer platforms like Biko use independent dedicated authentication.
    'auth' => [
        'url' => env('NABUAUTH_URL', 'https://auth.nabuxai.com'),
        'client_id' => env('NABUAUTH_CLIENT_ID'),
        'client_secret' => env('NABUAUTH_CLIENT_SECRET'),
        'redirect_uri' => env('NABUAUTH_REDIRECT_URI'),
        'ecosystem_apps' => ['NabuDesk', 'NabuGen', 'NabuGate', 'NabuVoice', 'NabuBot'],
    ],

    'tts' => [
        'base_url' => env('ELEVENLABS_BASE_URL', 'https://api.elevenlabs.io/v1'),
        'api_key' => env('ELEVENLABS_API_KEY'),
        'voice_id' => env('ELEVENLABS_VOICE_ID'),
        'model' => env('ELEVENLABS_MODEL', 'eleven_multilingual_v2'),
        'format' => env('ELEVENLABS_OUTPUT_FORMAT', 'mp3_44100_128'),
    ],
];
