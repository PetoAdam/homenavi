# Nginx Configuration Guide for Homenavi

This guide explains how to use and configure the Nginx reverse proxy for Homenavi in both development and production environments.

---

## 🚀 Overview
- Nginx acts as the public entrypoint for all HTTP(S) traffic.
- Routes `/api` requests to the API Gateway, all other requests to the frontend.
- Handles HTTPS termination in production.
- Can optionally proxy internal tools (Grafana, Prometheus, Jaeger) for convenience (not recommended for public exposure).

---

## 🧑‍💻 Local Development (HTTP Only)
- Uses `nginx.conf.dev.template` (no HTTPS, no certs needed).
- All requests to `http://localhost`:
  - `/api` and `/api/*` → API Gateway
  - `/` → Frontend
- Internal tools (Grafana, Prometheus, Jaeger) are accessed directly via their ports (see README).

**No environment variables or certs required.**

---

## 🛡️ Production (HTTPS, Secure)
- Uses `nginx.conf.prod.template` (HTTPS enforced, HTTP redirected to HTTPS).
- Requires real SSL certificates (e.g., from Let's Encrypt).
- Set `SSL_CERT_PATH` and `SSL_KEY_PATH` environment variables to point to your cert and key.
- Only expose Nginx (ports 80/443) to the public. All other services should be internal.

**Example Docker Compose Nginx service for production:**
```yaml
nginx:
  image: nginx:1.25-alpine
  ports:
    - "80:80"
    - "443:443"
  volumes:
    - ./nginx/nginx.conf.prod.template:/etc/nginx/nginx.conf.prod.template:ro
    - ./nginx/entrypoint.sh:/entrypoint.sh:ro
    - ./nginx/certs:/etc/nginx/certs:ro
  environment:
    - NGINX_MODE=prod
    - SSL_CERT_PATH=/etc/nginx/certs/fullchain.pem
    - SSL_KEY_PATH=/etc/nginx/certs/privkey.pem
  entrypoint: ["/bin/sh", "/entrypoint.sh"]
  depends_on:
    - frontend
    - api-gateway
  restart: unless-stopped
```

---

## 🔒 Security Best Practices
- **Never expose internal tools (Grafana, Prometheus, Jaeger, Redis) to the public.**
- If you must proxy them through Nginx, use authentication (basic auth, OAuth) and restrict by IP.
- Use strong SSL certificates and keep them secure.
- Regularly update Nginx and all dependencies.

---

## 🧩 Example: Proxying Internal Tools (Optional, Not for Public Use)
Add to your Nginx config (dev or prod):
```nginx
location /grafana/ {
    proxy_pass http://grafana:3000/;
}
location /jaeger/ {
    proxy_pass http://jaeger:16686/;
}
location /prometheus/ {
    proxy_pass http://prometheus:9090/;
}
```
**For production, always add authentication and/or IP allowlisting!**

---

## 📝 Switching Between Dev and Prod
- The entrypoint script (`nginx/entrypoint.sh`) selects the config based on the `NGINX_MODE` environment variable (`dev` or `prod`).
- For local dev, use `dev` (default). For production, set `NGINX_MODE=prod` and provide certs.

---

## 📚 See Also
- [README.md](../README.md)
- [Local Build Guide](local_build.md)

---
