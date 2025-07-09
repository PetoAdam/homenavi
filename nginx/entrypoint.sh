#!/bin/sh
set -e

# Default to dev mode (http only)
NGINX_MODE=${NGINX_MODE:-dev}

if [ "$NGINX_MODE" = "prod" ]; then
  TEMPLATE=/etc/nginx/nginx.conf.prod.template
  # Only substitute SSL_CERT_PATH and SSL_KEY_PATH, leave $host etc. intact
  envsubst '${SSL_CERT_PATH} ${SSL_KEY_PATH}' < "$TEMPLATE" > /etc/nginx/nginx.conf
else
  TEMPLATE=/etc/nginx/nginx.conf.dev.template
  # No custom variables, so don't substitute anything
  cp "$TEMPLATE" /etc/nginx/nginx.conf
fi

exec nginx -g 'daemon off;'
