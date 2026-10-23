# Iran mirrors (`gate`, `pilot`, `chat` `.nabuxai.ir`)

One mirror, one Coolify resource per product. The resource's environment picks the product:

| Resource | `UPSTREAM_HOST` | `MIRROR_HOST` | `NGINX_ENVSUBST_TEMPLATE_DIR` |
| --- | --- | --- | --- |
| NabuGate | `gate.nabuxai.com` (default) | `gate.nabuxai.ir` (default) | `/etc/nginx/templates` (default) |
| NabuPilot | `pilot.nabuxai.com` | `pilot.nabuxai.ir` | `/etc/nginx/templates` |
| NabuChat | `chat.nabuxai.com` | `chat.nabuxai.ir` | `/etc/nginx/templates-rewrite` |

`templates/` passes requests through. It is for a product that builds its links from the request: NabuGate reads `X-Forwarded-Host`, and NabuPilot builds sign-in and payment return URLs from the page's own origin. `templates-rewrite/` also replaces `https://UPSTREAM_HOST` with `https://MIRROR_HOST` in pages, scripts, JSON, redirects and cookies. It is for a product that pins its public URL at build time: NabuChat bakes `NEXT_PUBLIC_DASHBOARD_URL` into its JavaScript. URL-encoded copies are not rewritten, because they are OAuth `redirect_uri` values and the product exchanges the code with the URL it was built with. So NabuChat's dashboard sign-in still round-trips through `chat.nabuxai.com`. Everything else (pages, the chat widget, the API) is served through the mirror.

Each mirror host needs its callbacks registered on NabuAuth (`apps.yaml`), and an A record on ArvanCloud pointing at the Iran server, like every other `nabuxai.ir` name.

The rest of this file describes the NabuGate mirror. The others differ only in the table above.


This directory is a reverse proxy for an Iran-hosted machine. It does not run NabuGate. Every request — pages, static files, `/v1/*`, the console API, server-sent events, and websocket upgrades — is forwarded to `https://gate.nabuxai.com`. Browsers in Iran talk only to `gate.nabuxai.ir`. The connection to the upstream leaves from this server.

The machine runs **Coolify**, and Coolify’s proxy (`coolify-proxy`, Traefik v3) already owns ports 80 and 443 and issues the Let’s Encrypt certificate per host. The file to deploy is [`docker-compose.yml`](docker-compose.yml). It starts one nginx container that listens on container port 80 only: no TLS, no host port binding. Traefik routes `gate.nabuxai.ir` to that port. The domain is set in Coolify (`docker_compose_domains`), not in the compose file.

[`nginx.conf`](nginx.conf) and [`Caddyfile`](Caddyfile) are optional, for a host where nothing else is using 80/443. Do not install them on the Coolify server.

Nothing in this directory is applied automatically.

## What the upstream already does

The gateway trusts `X-Forwarded-Host` and `X-Forwarded-Proto` over the `Host` header the proxy uses for the upstream certificate. After this nginx, a request arrives at `gate.nabuxai.com` with:

| Header | Value |
| --- | --- |
| `Host` | `gate.nabuxai.com` |
| `X-Forwarded-Host` | `gate.nabuxai.ir` |
| `X-Forwarded-Proto` | `https` |
| `X-Forwarded-For` | the browser, then Traefik |

`X-Forwarded-Proto` is the literal `https`. Inside the container `$scheme` is `http`, because Traefik already terminated TLS. Using `$scheme` would build `http://` return URLs and omit `Secure` on cookies.

From those headers the gateway:

- Builds payment return URLs and the NabuAuth `redirect_uri` as `https://gate.nabuxai.ir/...`, so the browser comes back to the mirror even when `NABU_PUBLIC_URL` is `https://gate.nabuxai.com`.
- Sets session cookies with an empty `Domain` and `Secure`, so the browser stores them for `gate.nabuxai.ir`.
- Rate-limits the first `X-Forwarded-For` hop, which is the browser. Traefik wrote that hop; this nginx appends its own address after it.
- Flushes streamed responses and sets `X-Accel-Buffering: no`. This nginx also turns `proxy_buffering` and `proxy_request_buffering` off.

The console and the docs copy the base URL from the page’s own origin, so a sample taken on the mirror says `https://gate.nabuxai.ir/v1`. API calls from the page are relative. `server.cors_origins` stays empty: the mirror is same-origin, and listing `gate.nabuxai.ir` would only let that page call `.com` directly.

## Before you start

1. DNS: an A/AAAA record for `gate.nabuxai.ir` pointing at the Coolify server, not at the foreign host.
2. That server can open outbound TCP 443 to `gate.nabuxai.com`.
3. On the **upstream** deployment, leave `NABUAUTH_REDIRECT_URI` unset so each host derives its own callback. Register both of these redirect URIs on NabuAuth:

   - `https://gate.nabuxai.com/api/nabu/callback`
   - `https://gate.nabuxai.ir/api/nabu/callback`

   If `NABUAUTH_REDIRECT_URI` is pinned to the `.com` URL, sign-in from the mirror returns to `.com`.
4. A forwarded host of `gate.nabuxai.ir` or `gate.nabuxai.com` wins over `NABU_PUBLIC_URL`. Any other forwarded host does not.

## Coolify

1. In Coolify, create a **Docker Compose** resource from this repository. Set the **Base Directory** to `deploy/iran` so the build context is this directory. Leave the compose file as `docker-compose.yml`.
2. Coolify parses a service named `mirror`. Open **Domains** and add one domain for that service:
   - Protocol: `https`
   - Domain: `gate.nabuxai.ir`
   - Port: `80` (the port inside the container, not a host port)
   Coolify stores this on `docker_compose_domains` and generates the Traefik router, the Let’s Encrypt certificate, and the HTTP→HTTPS redirect. Do not add a `Host()` label or a `ports:` entry to the compose file; a published port would skip Traefik.
3. Turn **Gzip compression off** for this resource. Coolify’s compress middleware buffers `text/event-stream`, so a streamed chat would arrive in one piece. nginx in this container already has `gzip off` and `proxy_buffering off`.
4. Deploy. The container healthcheck is `GET /healthz` on port 80 inside the container (a local `200`, not a call to the upstream). Traefik is what answers `https://gate.nabuxai.ir/healthz`.

The service has to stay named `mirror`. `SERVICE_FQDN_MIRROR_80` is how Coolify knows to route the domain to port 80; renaming the service without renaming that variable leaves the container off the proxy.

## Check

```bash
curl -fsSI https://gate.nabuxai.ir/ | head
curl -fsS https://gate.nabuxai.ir/v1/models -H "Authorization: Bearer $KEY" | head
# A streamed completion should print chunks as they arrive, not after the call ends.
curl -N https://gate.nabuxai.ir/v1/chat/completions \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"ping"}]}'
```

Open `https://gate.nabuxai.ir/` and confirm the copied base URL in the hero is `https://gate.nabuxai.ir/v1`. Sign-in should set a cookie for `gate.nabuxai.ir` with `Secure` and no `Domain`. A top-up and a NabuAuth sign-in should return to `https://gate.nabuxai.ir`, not to `.com`.

## Optional: a host that is not running Coolify

Use one of these, not both, and not on the Coolify server (they both want ports 80 and 443, which Traefik already has).

### nginx

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

`certbot renew` keeps the certificate current. The port-80 server in `nginx.conf` already serves `/.well-known/acme-challenge/` from `/var/www/certbot` and redirects every other path to HTTPS, so webroot renewal works with the file as committed. This nginx is the edge, so it sets `X-Forwarded-For` to `$remote_addr` and `X-Forwarded-Proto` to `$scheme`.

### Caddy

Caddy issues the certificate itself. Install Caddy, then:

```bash
sudo cp Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```
