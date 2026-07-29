<?php

namespace NabuGate\Auth;

use Illuminate\Support\Facades\Http;
use RuntimeException;

class NabuAuthClient
{
    public function __construct(
        private readonly string $authUrl = 'https://auth.nabuxai.com',
        private readonly ?string $clientId = null,
        private readonly ?string $clientSecret = null,
    ) {
    }

    public function getAuthorizeUrl(string $redirectUri, string $state = '', array $scopes = ['openid', 'profile', 'email', 'wallet']): string
    {
        $params = http_build_query([
            'client_id' => $this->clientId,
            'redirect_uri' => $redirectUri,
            'response_type' => 'code',
            'scope' => implode(' ', $scopes),
            'state' => $state,
        ]);

        return rtrim($this->authUrl, '/') . '/oauth/authorize?' . $params;
    }

    public function handleCallback(string $code, string $redirectUri): array
    {
        $response = Http::asForm()->post(rtrim($this->authUrl, '/') . '/oauth/token', [
            'grant_type' => 'authorization_code',
            'client_id' => $this->clientId,
            'client_secret' => $this->clientSecret,
            'redirect_uri' => $redirectUri,
            'code' => $code,
        ]);

        if ($response->failed()) {
            throw new RuntimeException("NabuAuth token exchange failed ({$response->status()}): " . $response->body());
        }

        return $response->json();
    }

    public function getUser(string $accessToken): array
    {
        $response = Http::withToken($accessToken)
            ->get(rtrim($this->authUrl, '/') . '/api/v1/user');

        if ($response->failed()) {
            throw new RuntimeException("NabuAuth user lookup failed ({$response->status()}): " . $response->body());
        }

        return $response->json();
    }
}
