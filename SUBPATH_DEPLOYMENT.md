# Subpath Deployment Guide

This guide explains how to deploy Tinpot under a URL subpath (e.g., `https://example.com/tinpot/`) instead of at the root path.

## Overview

Tinpot supports deployment under a subpath through:
1. **Backend**: FastAPI's `root_path` parameter handles all route generation
2. **Frontend**: Dynamic `BASE_PATH` injection for API calls and SSE streams
3. **Nginx**: Reverse proxy with path rewriting

## Configuration

### 1. Environment Variable

Set the `ROOT_PATH` environment variable to your desired subpath:

```bash
# For deployment at https://example.com/tinpot/
export ROOT_PATH="/tinpot"

# For root path deployment (default)
export ROOT_PATH=""
# or simply don't set it
```

### 2. Uvicorn Configuration

When starting the API server, pass the `--root-path` parameter:

```bash
uvicorn app.main:app \
    --host 0.0.0.0 \
    --port 8000 \
    --root-path /tinpot
```

Or update the systemd service file:

```ini
[Service]
Environment="ROOT_PATH=/tinpot"
ExecStart=/opt/tinpot/venv/bin/uvicorn app.main:app \
    --host 0.0.0.0 \
    --port 8000 \
    --root-path /tinpot
```

### 3. Nginx Reverse Proxy

Configure nginx to proxy requests to Tinpot:

```nginx
upstream tinpot_api {
    server localhost:8000;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name example.com;

    # ... SSL configuration ...

    # Tinpot application on /tinpot/ subpath
    location /tinpot/ {
        # Strip the /tinpot prefix before proxying
        rewrite ^/tinpot/(.*)$ /$1 break;
        
        proxy_pass http://tinpot_api;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Prefix /tinpot;
        
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
    
    # SSE streaming endpoints (no buffering)
    location /tinpot/api/executions/ {
        rewrite ^/tinpot/(.*)$ /$1 break;
        
        proxy_pass http://tinpot_api;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Prefix /tinpot;
        
        # SSE specific - disable buffering
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
    }
}
```

## How It Works

### Backend (FastAPI)

When `root_path` is set, FastAPI:
- Generates OpenAPI docs with the correct base path
- Prefixes all routes in the OpenAPI schema
- Allows the app to be mounted at any path

The application still listens on the same routes internally (e.g., `/api/actions`), but FastAPI knows it's being served from `/tinpot/api/actions` externally.

### Frontend (JavaScript)

The backend injects a `BASE_PATH` variable into the HTML:

```html
<script>window.BASE_PATH = "/tinpot";</script>
```

All API calls and SSE connections use this base path:

```javascript
// API calls
fetch(`${BASE_PATH}/api/actions`)

// SSE streaming
new EventSource(`${BASE_PATH}/api/executions/${id}/stream`)
```

### Nginx Path Rewriting

Nginx receives requests at `/tinpot/*` and:
1. Strips the `/tinpot` prefix with `rewrite`
2. Proxies the request to the backend at `/*`
3. Adds `X-Forwarded-Prefix` header for logging/debugging

Example flow:
```
User requests: https://example.com/tinpot/api/actions
       ↓
Nginx receives: /tinpot/api/actions
       ↓
Nginx rewrites: /api/actions
       ↓
Proxy to backend: http://localhost:8000/api/actions
       ↓
Backend serves: /api/actions (knows it's at /tinpot externally)
```

## Testing

### Quick Test

Test subpath deployment locally:

```bash
# Start Redis
docker run -d --name redis -p 6379:6379 redis:7-alpine

# Start API with subpath
export ROOT_PATH="/tinpot"
uvicorn app.main:app --host 127.0.0.1 --port 8000 --root-path /tinpot

# Test endpoints (accessed without prefix when testing directly)
curl http://localhost:8000/api/actions
curl http://localhost:8000/health

# Check that BASE_PATH is injected
curl http://localhost:8000/ | grep 'BASE_PATH = "/tinpot"'
```

### Automated Test

Run the provided test script:

```bash
./test_subpath.sh
```

This tests both root path and subpath deployment modes.

### Test with Nginx

Create a test nginx configuration:

```nginx
server {
    listen 8080;
    server_name localhost;

    location /tinpot/ {
        rewrite ^/tinpot/(.*)$ /$1 break;
        proxy_pass http://localhost:8000;
        proxy_set_header X-Forwarded-Prefix /tinpot;
    }
    
    location /tinpot/api/executions/ {
        rewrite ^/tinpot/(.*)$ /$1 break;
        proxy_pass http://localhost:8000;
        proxy_buffering off;
        proxy_cache off;
    }
}
```

Then test:

```bash
# Start services
uvicorn app.main:app --root-path /tinpot &
nginx -c /path/to/test-nginx.conf

# Access via nginx
curl http://localhost:8080/tinpot/
curl http://localhost:8080/tinpot/api/actions
```

## Multiple Instances

Deploy multiple Tinpot instances on different subpaths:

```nginx
# Production instance
location /tinpot/ {
    rewrite ^/tinpot/(.*)$ /$1 break;
    proxy_pass http://tinpot_prod:8000;
    proxy_set_header X-Forwarded-Prefix /tinpot;
}

# Staging instance
location /tinpot-staging/ {
    rewrite ^/tinpot-staging/(.*)$ /$1 break;
    proxy_pass http://tinpot_staging:8001;
    proxy_set_header X-Forwarded-Prefix /tinpot-staging;
}

# Development instance
location /tinpot-dev/ {
    rewrite ^/tinpot-dev/(.*)$ /$1 break;
    proxy_pass http://tinpot_dev:8002;
    proxy_set_header X-Forwarded-Prefix /tinpot-dev;
}
```

Each instance needs its own `ROOT_PATH`:

```bash
# Production
ROOT_PATH=/tinpot uvicorn app.main:app --port 8000 --root-path /tinpot

# Staging
ROOT_PATH=/tinpot-staging uvicorn app.main:app --port 8001 --root-path /tinpot-staging

# Development
ROOT_PATH=/tinpot-dev uvicorn app.main:app --port 8002 --root-path /tinpot-dev
```

## Troubleshooting

### OpenAPI Docs Not Loading

If `/docs` or `/redoc` don't load correctly:

1. Verify `--root-path` matches `ROOT_PATH` environment variable
2. Check nginx is correctly stripping the prefix
3. Access docs at `https://example.com/tinpot/docs` (with subpath)

### API Calls Return 404

Check that:
- `BASE_PATH` is correctly injected in the HTML source
- Browser console shows requests to correct URLs
- Nginx rewrite rule is working: `rewrite ^/tinpot/(.*)$ /$1 break;`

### SSE Streaming Not Working

Ensure:
- SSE location block has `proxy_buffering off` and `proxy_cache off`
- SSE endpoint includes the subpath: `/tinpot/api/executions/`
- Browser DevTools shows EventSource connection to correct URL

### Static Assets Not Loading

If CSS/JS don't load:
- Verify `ROOT_PATH` environment variable is set
- Check that `BASE_PATH` is injected into HTML
- Ensure nginx location blocks cover static asset paths

### Health Check Fails with Subpath

Health checks work with or without subpath. Test directly:

```bash
# Direct to application (no subpath in URL)
curl http://localhost:8000/health

# Through nginx (with subpath)
curl https://example.com/tinpot/health
```

## Best Practices

1. **Consistent Configuration**: Keep `ROOT_PATH` and `--root-path` synchronized
2. **Environment Variables**: Store `ROOT_PATH` in environment files, not hardcoded
3. **Testing**: Test both direct access and nginx proxy access
4. **Logging**: Check nginx access logs to verify path rewriting
5. **Monitoring**: Update health check URLs in monitoring systems
6. **Documentation**: Document the subpath in deployment notes for team members

## Example: Complete Deployment

Full example for deploying at `https://example.com/automation/`:

**1. Environment file (`/opt/tinpot/api.env`):**

```bash
REDIS_URL=redis://redis-server:6379/0
ACTIONS_DIR=/opt/tinpot/actions
ROOT_PATH=/automation
HOST=0.0.0.0
PORT=8000
WORKERS=4
LOG_LEVEL=info
```

**2. Systemd service:**

```ini
[Unit]
Description=Tinpot API Server
After=network.target redis.service

[Service]
Type=simple
User=tinpot
WorkingDirectory=/opt/tinpot
EnvironmentFile=/opt/tinpot/api.env
ExecStart=/opt/tinpot/venv/bin/uvicorn app.main:app \
    --host ${HOST} \
    --port ${PORT} \
    --workers ${WORKERS} \
    --log-level ${LOG_LEVEL} \
    --root-path ${ROOT_PATH}
Restart=always

[Install]
WantedBy=multi-user.target
```

**3. Nginx configuration:**

```nginx
location /automation/ {
    rewrite ^/automation/(.*)$ /$1 break;
    proxy_pass http://localhost:8000;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Prefix /automation;
    proxy_read_timeout 60s;
}

location /automation/api/executions/ {
    rewrite ^/automation/(.*)$ /$1 break;
    proxy_pass http://localhost:8000;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Prefix /automation;
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
}
```

**4. Start and test:**

```bash
sudo systemctl daemon-reload
sudo systemctl start tinpot-api
sudo systemctl reload nginx

# Test
curl https://example.com/automation/health
curl https://example.com/automation/api/actions
```

Access the UI at: `https://example.com/automation/`

## Summary

Tinpot's subpath deployment is achieved through:

| Component | Configuration | Purpose |
|-----------|--------------|---------|
| Environment | `ROOT_PATH=/tinpot` | Sets base path for app |
| Uvicorn | `--root-path /tinpot` | Configures FastAPI routing |
| FastAPI | `root_path` parameter | Generates correct OpenAPI schema |
| Frontend | `window.BASE_PATH` | Makes API calls with prefix |
| Nginx | `rewrite` + `proxy_pass` | Routes and strips prefix |

The application remains unchanged internally—all routing logic is handled through configuration. This allows the same codebase to work at root path or any subpath without modification.
