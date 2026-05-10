#!/bin/bash
set -e

# Start virtual display for non-headless mode.
Xvfb :99 -screen 0 1920x1080x24 -nolisten tcp -ac &
sleep 1

# Build args. The stock container serves plain HTTP unless explicit TLS
# certificate paths are provided.
ARGS=(serve --binary ./browser/camoufox --port 8443 --no-tls)

if [ -n "$VULPINE_API_KEY" ]; then
    ARGS+=(--api-key "$VULPINE_API_KEY")
fi

if [ -n "$VULPINE_TLS_CERT" ] && [ -n "$VULPINE_TLS_KEY" ]; then
    ARGS=(serve --binary ./browser/camoufox --port 8443 --tls-cert "$VULPINE_TLS_CERT" --tls-key "$VULPINE_TLS_KEY")
    if [ -n "$VULPINE_API_KEY" ]; then
        ARGS+=(--api-key "$VULPINE_API_KEY")
    fi
fi

# Pass any additional args from docker run
exec ./vulpineos "${ARGS[@]}" "$@"
