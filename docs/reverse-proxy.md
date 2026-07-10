# Reverse Proxy Configuration

HTTPS is required for mobile CardDAV clients (iOS, macOS Contacts). Use a reverse proxy to terminate TLS in front of ContactsHQ.

## Caddy (recommended)

Caddy automatically provisions HTTPS certificates via Let's Encrypt. This is the simplest option.

```
your-domain.com {
    reverse_proxy localhost:8080
}
```

That's it — Caddy handles certificate issuance, renewal, and HTTPS redirection automatically.

## nginx

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}
```

Use [certbot](https://certbot.eff.org/) to obtain certificates from Let's Encrypt:

```bash
sudo certbot certonly --nginx -d your-domain.com
```

## Traefik

If you're using Traefik with Docker Compose, add labels to the ContactsHQ service:

```yaml
services:
  contactshq:
    image: ghcr.io/gumeniukcom/contactshq:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.contactshq.rule=Host(`your-domain.com`)"
      - "traefik.http.routers.contactshq.tls.certresolver=letsencrypt"
      - "traefik.http.services.contactshq.loadbalancer.server.port=8080"
```

## Client IPs behind a proxy

The auth endpoints are rate-limited per client IP. Behind a reverse proxy every request
arrives from the proxy's own address, so without further configuration the limit is shared
across everyone behind the proxy.

To restore per-client limiting, tell ContactsHQ which proxies may set `X-Forwarded-For`.
List the proxy's address (or CIDR range) — never `0.0.0.0/0`, which would let any client
spoof the header:

```yaml
server:
  trusted_proxies:
    - 127.0.0.1        # proxy on the same host
    - 10.0.0.0/8       # or a private range
```

or via environment variable (comma-separated):

```bash
CHQ_SERVER_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8
```

The server validates every entry at startup and refuses to boot on a malformed IP or CIDR.
When the request's direct peer matches a trusted entry, the client IP is taken from the
leftmost `X-Forwarded-For` value; otherwise the header is ignored and the direct peer is
used, so a spoofed header from an untrusted source cannot forge a new rate-limit bucket.
The nginx and Caddy examples above already forward the header.

## Health checks

`GET /health` returns `200` when the database is reachable and `503` with
`"status":"degraded"` when it is not. Point your uptime monitor at it — an unreachable
database is exactly the failure a plain "is the port open" check misses. The Docker image's
`HEALTHCHECK` already uses it.

## Verifying

After setting up your reverse proxy, verify CardDAV auto-discovery:

```bash
curl -I https://your-domain.com/.well-known/carddav
# Expected: 301 Moved Permanently → /dav/
```
