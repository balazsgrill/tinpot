# Subpath Deployment - Quick Start

Deploy Tinpot under a subpath (e.g., `https://example.com/tinpot/`) in 3 easy steps.

## Quick Setup

### Step 1: Set Environment Variable

```bash
export ROOT_PATH=/tinpot
```

### Step 2: Start with Root Path Parameter

```bash
uvicorn app.main:app --host 0.0.0.0 --port 8000 --root-path /tinpot
```

### Step 3: Configure Nginx

```nginx
location /tinpot/ {
    rewrite ^/tinpot/(.*)$ /$1 break;
    proxy_pass http://localhost:8000;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Prefix /tinpot;
}

location /tinpot/api/executions/ {
    rewrite ^/tinpot/(.*)$ /$1 break;
    proxy_pass http://localhost:8000;
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
}
```

## Verify

```bash
# Check BASE_PATH injection
curl http://localhost:8000/ | grep 'BASE_PATH = "/tinpot"'

# Test API
curl http://localhost:8000/api/actions

# Through nginx (after nginx config)
curl https://example.com/tinpot/
```

## Docker Compose

```bash
ROOT_PATH=/tinpot docker-compose up
```

## Systemd Service

Update `/opt/tinpot/api.env`:
```bash
ROOT_PATH=/tinpot
```

Update `/etc/systemd/system/tinpot-api.service`:
```ini
ExecStart=/opt/tinpot/venv/bin/uvicorn app.main:app \
    --host 0.0.0.0 \
    --port 8000 \
    --root-path ${ROOT_PATH}
```

## That's It!

Access your application at: `https://example.com/tinpot/`

For detailed documentation, see:
- [SUBPATH_DEPLOYMENT.md](SUBPATH_DEPLOYMENT.md) - Full deployment guide
- [SUBPATH_IMPLEMENTATION.md](SUBPATH_IMPLEMENTATION.md) - Technical details
- [DEPLOYMENT.md](DEPLOYMENT.md) - Production deployment guide

## Troubleshooting

**API returns 404?**
- Verify `ROOT_PATH` env var matches `--root-path` parameter
- Check nginx rewrite rule is correct

**SSE not streaming?**
- Ensure SSE location has `proxy_buffering off`
- Check browser DevTools for EventSource errors

**Static assets not loading?**
- Verify BASE_PATH is injected: view page source, look for `window.BASE_PATH`
- Check browser console for 404 errors

**Health check fails?**
- Health checks work with/without subpath
- Test directly: `curl http://localhost:8000/health`
