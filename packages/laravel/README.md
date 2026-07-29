# nabux/nabugate-laravel

Official Laravel package for **NabuGate**, the organisation's OpenAI-compatible
AI gateway, and **NabuAuth**, its single sign-on server. Projects call NabuGate
with an alias such as `nabu-fast`; the gateway picks the provider, falls back on
failure, holds the secrets and meters the cost.

```bash
composer require nabux/nabugate-laravel
php artisan vendor:publish --tag=nabugate-config
```

## Configure

```env
NABUGATE_API_KEY=...
NABUGATE_BASE_URL=https://gate.nabuxai.com/v1
NABUGATE_MODEL=nabu-smart

NABUAUTH_URL=https://auth.nabuxai.com
NABUAUTH_CLIENT_ID=yourapp
NABUAUTH_CLIENT_SECRET=...
NABUAUTH_REDIRECT_URI=https://yourapp.example.com/auth/nabu/callback
```

## Chat, streaming, embeddings, media

```php
use NabuGate\Client\NabuGateClient;

$nabu = app(NabuGateClient::class);

$answer = $nabu->completeText([['role' => 'user', 'content' => 'Summarise this quarter.']], 'nabu-fast');

$nabu->stream(
    [['role' => 'user', 'content' => 'Write a haiku.']],
    fn (string $delta) => print($delta),
);

// Pin `dimensions` whenever you store the vectors: a fixed-width column cannot
// take whatever the provider defaults to.
$vectors = $nabu->embeddings(['a', 'b'], 'write-embed', 1536);

$image  = $nabu->images('a lighthouse at dusk');        // data[].b64_json
$audio  = $nabu->speech('Welcome back.', voice: 'alloy');
$models = $nabu->models();
```

### Sub-agents

A named assistant is called exactly like a model:

```php
$nabu->chat($messages, 'cine-motion-designer');
```

### Everything passes through

The gateway forwards request bodies to the provider untouched, so anything in
`$extra` reaches the provider whether or not this package names it:

```php
$nabu->chat($messages, 'nabu-fast', [
    'tools' => [$tool],
    'response_format' => ['type' => 'json_object'],
    'seed' => 7,
]);
```

## Single sign-on with NabuAuth

```php
use NabuGate\Auth\NabuAuthClient;

// Redirect the browser
$pkce = NabuAuthClient::pkce();
session(['nabu_verifier' => $pkce['verifier'], 'nabu_state' => $state = Str::random(32)]);

return redirect(app(NabuAuthClient::class)->getAuthorizeUrl(
    state: $state,
    scopes: ['openid', 'profile', 'email', 'wallet'],
    codeChallenge: $pkce['challenge'],
));

// Handle the callback
$auth = app(NabuAuthClient::class);
abort_unless($request->get('state') === session('nabu_state'), 403);
$tokens = $auth->handleCallback($request->get('code'), codeVerifier: session('nabu_verifier'));
$profile = $auth->getUser($tokens['access_token']);
```

NabuAuth rotates refresh tokens, so `refresh()` returns a new one and the old one
stops working — persist the replacement or the next refresh fails.

### Wallet

```php
$auth->getWalletBalance($accessToken);

// Charge for usage. The idempotency key must be stable per unit of work, so a
// retried call cannot bill the customer twice.
$auth->debitWallet($serviceToken, 750, 'nabu-fast completion',
    userId: $user->nabu_id, idempotencyKey: $requestId);
```

## Metering middleware

`NabuGate\Middleware\NabuGateMeteringMiddleware` refuses a request when the
wallet is empty and debits it from `usage.total_tokens` on the way back out.

MIT licensed.
