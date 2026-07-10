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

## A note on client IPs

The auth endpoints are rate-limited per client IP. Behind a reverse proxy every request
arrives from the proxy's address unless the proxy sets `X-Forwarded-For` **and** the app
is told to trust it — which ContactsHQ does not do by default, so the limit is currently
shared across all clients behind the proxy. The limits are generous enough that normal use
is unaffected; this only matters if you expect many legitimate logins from behind a single
proxy. The nginx and Caddy examples above already forward the header, ready for when
per-client limiting is added.

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
