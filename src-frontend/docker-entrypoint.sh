#!/bin/sh
set -eu

# Render runtime config from env (PRD Stage B: identical image, config via env).
# Only these two vars are substituted; anything else in the template is left alone.
export API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
export OTLP_URL="${OTLP_URL:-http://localhost:4318/v1/traces}"
envsubst '${API_BASE_URL} ${OTLP_URL}' \
  < /etc/frontend/config.js.template \
  > /usr/share/nginx/html/config.js

exec nginx -g 'daemon off;'
