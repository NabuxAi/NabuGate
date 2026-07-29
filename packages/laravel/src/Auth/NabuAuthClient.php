<?php

namespace NabuGate\Auth;

use Illuminate\Support\Facades\Http;
use RuntimeException;

/**
 * Client for NabuAuth, the ecosystem's OAuth2 / OpenID Connect server.
 *
 * Drives the authorization code flow with PKCE, exchanges and refreshes tokens,
 * reads the signed-in profile, and reaches the shared wallet.
 */
class NabuAuthClient
{
    public function __construct(
        private readonly string $authUrl = 'https://auth.nabuxai.com',
        private readonly ?string $clientId = null,
        private readonly ?string $clientSecret = null,
        private readonly ?string $redirectUri = null,
    ) {
    }

    /**
     * Builds the URL to send the browser to.
     *
     * @param  array<int, string>  $scopes
     */
    public function getAuthorizeUrl(
        ?string $redirectUri = null,
        string $state = '',
        array $scopes = ['openid', 'profile', 'email', 'wallet'],
        ?string $codeChallenge = null,
        string $nonce = '',
    ): string {
        $params = [
            'client_id' => $this->clientId,
            'redirect_uri' => $redirectUri ?? $this->redirectUri,
            'response_type' => 'code',
            'scope' => implode(' ', $scopes),
        ];
        if ($state !== '') {
            $params['state'] = $state;
        }
        if ($nonce !== '') {
            $params['nonce'] = $nonce;
        }
        if ($codeChallenge !== null) {
            $params['code_challenge'] = $codeChallenge;
            $params['code_challenge_method'] = 'S256';
        }

        return rtrim($this->authUrl, '/').'/oauth/authorize?'.http_build_query($params);
    }

    /**
     * Generates a PKCE verifier and its S256 challenge.
     *
     * Store the verifier in the session and pass it back to handleCallback().
     * PKCE binds the redirect to this browser, so a code intercepted in transit
     * cannot be exchanged by anyone else.
     *
     * @return array{verifier: string, challenge: string}
     */
    public static function pkce(): array
    {
        $verifier = rtrim(strtr(base64_encode(random_bytes(32)), '+/', '-_'), '=');
        $challenge = rtrim(strtr(base64_encode(hash('sha256', $verifier, true)), '+/', '-_'), '=');

        return ['verifier' => $verifier, 'challenge' => $challenge];
    }

    /**
     * Exchanges an authorization code for tokens.
     *
     * @return array<string, mixed>
     */
    public function handleCallback(string $code, ?string $redirectUri = null, ?string $codeVerifier = null): array
    {
        $payload = [
            'grant_type' => 'authorization_code',
            'client_id' => $this->clientId,
            'redirect_uri' => $redirectUri ?? $this->redirectUri,
            'code' => $code,
        ];
        if ($this->clientSecret) {
            $payload['client_secret'] = $this->clientSecret;
        }
        if ($codeVerifier !== null) {
            $payload['code_verifier'] = $codeVerifier;
        }

        return $this->token($payload);
    }

    /**
     * Exchanges a refresh token for a fresh set.
     *
     * NabuAuth rotates refresh tokens, so the response carries a new one and the
     * old one stops working — persist the replacement or the next refresh fails.
     *
     * @return array<string, mixed>
     */
    public function refresh(string $refreshToken): array
    {
        $payload = [
            'grant_type' => 'refresh_token',
            'client_id' => $this->clientId,
            'refresh_token' => $refreshToken,
        ];
        if ($this->clientSecret) {
            $payload['client_secret'] = $this->clientSecret;
        }

        return $this->token($payload);
    }

    /**
     * Requests a service token for this app itself, with no user behind it.
     *
     * @param  array<int, string>  $scopes
     * @return array<string, mixed>
     */
    public function serviceToken(array $scopes = []): array
    {
        $payload = [
            'grant_type' => 'client_credentials',
            'client_id' => $this->clientId,
            'client_secret' => $this->clientSecret,
        ];
        if ($scopes !== []) {
            $payload['scope'] = implode(' ', $scopes);
        }

        return $this->token($payload);
    }

    /**
     * The signed-in user's profile.
     *
     * @return array<string, mixed>
     */
    public function getUser(string $accessToken): array
    {
        return $this->get('/api/v1/user', $accessToken);
    }

    /**
     * The user's wallet balance.
     *
     * @return array<string, mixed>
     */
    public function getWalletBalance(string $accessToken, ?int $userId = null): array
    {
        $path = '/api/v1/wallet/balance';
        if ($userId !== null) {
            $path .= '?user_id='.$userId;
        }

        return $this->get($path, $accessToken);
    }

    /**
     * Charges a wallet for usage.
     *
     * $idempotencyKey must be stable for a given unit of work: NabuAuth turns a
     * repeat into a lookup, so a retried call cannot bill the customer twice.
     *
     * @param  array<string, mixed>  $meta
     * @return array<string, mixed>
     */
    public function debitWallet(
        string $accessToken,
        int $amountCents,
        string $description,
        ?int $userId = null,
        string $idempotencyKey = '',
        array $meta = [],
    ): array {
        $payload = [
            'amount_cents' => $amountCents,
            'description' => $description,
            'meta' => $meta,
        ];
        if ($userId !== null) {
            $payload['user_id'] = $userId;
        }
        if ($idempotencyKey !== '') {
            $payload['idempotency_key'] = $idempotencyKey;
        }

        $response = Http::withToken($accessToken)
            ->acceptJson()
            ->post(rtrim($this->authUrl, '/').'/api/v1/wallet/debit', $payload);

        if ($response->failed()) {
            throw new RuntimeException("NabuAuth wallet debit failed ({$response->status()}): ".$response->body());
        }

        return $response->json();
    }

    /**
     * The apps in the ecosystem launcher.
     *
     * @return array<int, array<string, mixed>>
     */
    public function apps(): array
    {
        $response = Http::acceptJson()->get(rtrim($this->authUrl, '/').'/api/v1/apps');

        if ($response->failed()) {
            throw new RuntimeException("NabuAuth app registry failed ({$response->status()}): ".$response->body());
        }

        return $response->json('data', []);
    }

    /**
     * The URL that ends the NabuAuth session.
     */
    public function logoutUrl(?string $postLogoutRedirectUri = null): string
    {
        $url = rtrim($this->authUrl, '/').'/oauth/logout';
        if ($postLogoutRedirectUri !== null) {
            $url .= '?'.http_build_query(['post_logout_redirect_uri' => $postLogoutRedirectUri]);
        }

        return $url;
    }

    /**
     * @param  array<string, mixed>  $payload
     * @return array<string, mixed>
     */
    private function token(array $payload): array
    {
        $response = Http::asForm()->post(rtrim($this->authUrl, '/').'/oauth/token', $payload);

        if ($response->failed()) {
            throw new RuntimeException("NabuAuth token request failed ({$response->status()}): ".$response->body());
        }

        return $response->json();
    }

    /**
     * @return array<string, mixed>
     */
    private function get(string $path, string $accessToken): array
    {
        $response = Http::withToken($accessToken)
            ->acceptJson()
            ->get(rtrim($this->authUrl, '/').$path);

        if ($response->failed()) {
            throw new RuntimeException("NabuAuth request to {$path} failed ({$response->status()}): ".$response->body());
        }

        return $response->json();
    }
}
