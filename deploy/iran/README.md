# Iran mirror (`gate.nabuxai.ir`)

This directory is a reverse proxy for an Iran-hosted machine. It does not run NabuGate. Every request — pages, static files, `/v1/*`, the console API, server-sent events, and websocket upgrades — is forwarded to `https://gate.nabuxai.com`. Browsers in Iran talk only to `gate.nabuxai.ir`. The connection to the upstream leaves from this server.

Nothing in this directory is applied automatically. Installing it is a manual step on that machine.

## What the upstream already does

The gateway reads `X-Forwarded-Host` and `X-Forwarded-Proto`:

- Payment return URLs and the NabuAuth `redirect_uri` use the mirror’s host, so the browser comes back to `.ir`.
- Session cookies are host-only (no `Domain`) and `Secure` when the forwarded scheme is https, so they stick to `gate.nabuxai.ir`.
- Streamed responses set `X-Accel-Buffering: no` and are flushed as they arrive. The proxy configs below also turn buffering off.
- The console and the docs copy the base URL from the page’s own origin, so a sample taken on the mirror says `https://gate.nabuxai.ir/v1`.

`server.cors_origins` stays empty. The mirror is same-origin; granting `gate.nabuxai.ir` would only let that page call `.com` directly.

## Before you start

1. DNS: an A/AAAA record for `gate.nabuxai.ir` pointing at this server, not at the foreign host.
2. This server can open outbound TCP 443 to `gate.nabuxai.com`.
3. On the **upstream** deployment, leave `NABUAUTH_REDIRECT_URI` unset so each host derives its own callback. Register both of these redirect URIs on NabuAuth:

   - `https://gate.nabuxai.com/api/nabu/callback`
   - `https://gate.nabuxai.ir/api/nabu/callback`

   If `NABUAUTH_REDIRECT_URI` is pinned to the `.com` URL, sign-in from the mirror returns to `.com`.
4. Do not set `NABU_PUBLIC_URL` in a way that surprises you: a forwarded host of `gate.nabuxai.ir` or `gate.nabuxai.com` wins over that variable. Any other forwarded host does not.

## nginx

```bash
sudo apt-get update
sudo apt-get install -y nginx certbot python3-certbot-nginx
sudo mkdir -p /var/www/certbot
# Temporary HTTP-only server so certbot can answer the challenge.
sudo tee /etc/nginx/sites-available/gate.nabuxai.ir <<'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name gate.nabuxai.ir;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / { return 301 https://gate.nabuxai.ir$request_uri; }
}
EOF
sudo ln -sfn /etc/nginx/sites-available/gate.nabuxai.ir /etc/nginx/sites-enabled/gate.nabuxai.ir
sudo nginx -t && sudo systemctl reload nginx
sudo certbot certonly --webroot -w /var/www/certbot -d gate.nabuxai.ir
```

Then replace the temporary file with `nginx.conf` from this directory (the certificate paths match certbot’s layout), test, and reload:

```bash
sudo cp nginx.conf /etc/nginx/sites-available/gate.nabuxai.ir
sudo nginx -t && sudo systemctl reload nginx
```

`certbot renew` keeps the certificate current. The port-80 server in `nginx.conf` already serves `/.well-known/acme-challenge/` from `/var/www/certbot` and redirects every other path to HTTPS, so webroot renewal works with the file as committed.

## Caddy

Caddy issues the certificate itself. Install Caddy, then:

```bash
sudo cp Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Use nginx **or** Caddy, not both, on ports 80 and 443.

## Check

```bash
curl -fsSI https://gate.nabuxai.ir/ | head
curl -fsS https://gate.nabuxai.ir/v1/models -H "Authorization: Bearer $KEY" | head
# A streamed completion should print chunks as they arrive, not after the call ends.
curl -N https://gate.nabuxai.ir/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"ping"}]}'
```

Open `https://gate.nabuxai.ir/` and confirm the copied base URL in the hero is `https://gate.nabuxai.ir/v1`. Sign-in should set a cookie for `gate.nabuxai.ir` with `Secure` and no `Domain`.
