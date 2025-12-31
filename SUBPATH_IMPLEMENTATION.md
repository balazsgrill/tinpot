# Subpath Deployment Implementation

## Changes Summary

This implementation adds full support for deploying Tinpot under a URL subpath (e.g., `https://example.com/tinpot/`).

## Modified Files

### Backend Changes

**`app/main.py`**
- Added `ROOT_PATH` environment variable support
- Updated FastAPI initialization to use `root_path` parameter
- Modified root endpoint to inject `BASE_PATH` into HTML
- Routes automatically work with subpath through FastAPI's built-in support

```python
ROOT_PATH = os.environ.get("ROOT_PATH", "")

app = FastAPI(
    title="Tinpot",
    version="1.0",
    root_path=ROOT_PATH
)
```

### Frontend Changes

**`static/index.html`**
- Added `<!-- BASE_PATH_INJECTION -->` marker in HTML head
- Backend injects `<script>window.BASE_PATH = "/tinpot";</script>`
- Updated all API calls to use dynamic `BASE_PATH`:
  - `fetch(\`${BASE_PATH}/api/actions\`)`
  - `fetch(\`${BASE_PATH}/api/actions/${actionName}/execute\`)`
  - `new EventSource(\`${BASE_PATH}/api/executions/${executionId}/stream\`)`

### Docker Changes

**`docker-compose.yml`**
- Added `ROOT_PATH` environment variable support
- Updated `command` to include `--root-path` parameter
- Can be set via environment: `ROOT_PATH=/tinpot docker-compose up`

### Documentation

**`DEPLOYMENT.md`**
- Added reference to SUBPATH_DEPLOYMENT.md
- Updated nginx configuration with three deployment options:
  1. Root path deployment (original)
  2. Subpath deployment (new)
  3. Multiple instances on different subpaths

**`SUBPATH_DEPLOYMENT.md`** (NEW)
- Complete guide for subpath deployment
- Configuration examples for:
  - Environment variables
  - Uvicorn startup
  - Nginx reverse proxy
  - Multiple instance deployment
- Troubleshooting section
- Testing instructions
- Best practices

**`.env.example`** (NEW)
- Documented all environment variables
- Includes ROOT_PATH configuration
- Separated sections for API, Worker, Docker, Production

### Testing

**`test_subpath.sh`** (NEW)
- Automated test script
- Tests both root path and subpath deployment
- Verifies:
  - BASE_PATH injection
  - API endpoint accessibility
  - Health check functionality
  - OpenAPI docs generation

## How It Works

### Request Flow (Subpath Deployment)

```
1. User accesses: https://example.com/tinpot/
   ↓
2. Nginx receives: /tinpot/
   ↓
3. Nginx rewrites: / (strips prefix)
   ↓
4. Proxies to: http://localhost:8000/
   ↓
5. FastAPI serves: index.html with BASE_PATH="/tinpot" injected
   ↓
6. Browser loads: JavaScript with window.BASE_PATH = "/tinpot"
   ↓
7. User clicks action: fetch(`${BASE_PATH}/api/actions/example/execute`)
   ↓
8. Browser requests: https://example.com/tinpot/api/actions/example/execute
   ↓
9. Nginx rewrites: /api/actions/example/execute
   ↓
10. FastAPI handles: /api/actions/example/execute
```

### Key Components

| Component | Responsibility |
|-----------|---------------|
| **ROOT_PATH env var** | Stores the deployment subpath |
| **FastAPI root_path** | Configures route generation and OpenAPI schema |
| **BASE_PATH injection** | Provides subpath to frontend JavaScript |
| **Nginx rewrite** | Strips subpath prefix before proxying |
| **X-Forwarded-Prefix** | Header for debugging (optional) |

## Configuration Examples

### Root Path (Default)

```bash
# No configuration needed
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

Access at: `http://localhost:8000/`

### Subpath Deployment

```bash
# Set environment variable
export ROOT_PATH=/tinpot

# Start with root-path parameter
uvicorn app.main:app --host 0.0.0.0 --port 8000 --root-path /tinpot
```

Nginx configuration:
```nginx
location /tinpot/ {
    rewrite ^/tinpot/(.*)$ /$1 break;
    proxy_pass http://localhost:8000;
    proxy_set_header X-Forwarded-Prefix /tinpot;
}
```

Access at: `https://example.com/tinpot/`

### Docker Compose

```bash
# Root path
docker-compose up

# Subpath
ROOT_PATH=/tinpot docker-compose up
```

### Multiple Instances

Deploy multiple environments on different subpaths:

```bash
# Production
ROOT_PATH=/tinpot uvicorn app.main:app --port 8000 --root-path /tinpot

# Staging  
ROOT_PATH=/tinpot-staging uvicorn app.main:app --port 8001 --root-path /tinpot-staging

# Development
ROOT_PATH=/tinpot-dev uvicorn app.main:app --port 8002 --root-path /tinpot-dev
```

## Testing

### Manual Test

```bash
# Start services
export ROOT_PATH=/tinpot
uvicorn app.main:app --root-path /tinpot &

# Test endpoints (direct access, no nginx)
curl http://localhost:8000/ | grep 'BASE_PATH = "/tinpot"'
curl http://localhost:8000/api/actions
curl http://localhost:8000/health
```

### Automated Test

```bash
./test_subpath.sh
```

Expected output:
```
Test 1: Root path deployment (ROOT_PATH not set)
✓ Index page served
✓ API endpoint accessible
✓ Health check passed

Test 2: Subpath deployment (ROOT_PATH=/tinpot)
✓ BASE_PATH injected correctly
✓ API endpoint accessible at root
✓ OpenAPI docs accessible
✓ Health check passed
```

### Integration Test with Nginx

See [SUBPATH_DEPLOYMENT.md](SUBPATH_DEPLOYMENT.md#testing) for full nginx testing setup.

## Benefits

1. **Flexibility**: Deploy at root or any subpath without code changes
2. **Multi-tenancy**: Run multiple instances on different subpaths
3. **Integration**: Easy integration with existing web infrastructure
4. **Standards**: Uses FastAPI's built-in subpath support
5. **SSE Support**: Server-Sent Events work correctly with subpaths
6. **OpenAPI**: Swagger/ReDoc documentation automatically adjusted

## Compatibility

- ✅ Works with root path deployment (backward compatible)
- ✅ Works with subpath deployment (new feature)
- ✅ Works with Docker and systemd
- ✅ Works with single and multiple workers
- ✅ SSE streaming fully functional
- ✅ Static assets properly served
- ✅ Health checks accessible
- ✅ OpenAPI docs correctly generated

## Migration

Existing deployments at root path continue to work without changes:
- No environment variables needed
- No nginx changes needed  
- No code changes needed

To migrate to subpath:
1. Set `ROOT_PATH` environment variable
2. Add `--root-path` to uvicorn command
3. Update nginx configuration
4. Restart services

## Security Considerations

- Subpath doesn't add security (use authentication/authorization separately)
- Nginx should validate/sanitize paths
- Use HTTPS for production deployments
- Consider rate limiting at nginx level
- Set appropriate CORS headers if needed

## Performance

- No performance impact from subpath support
- BASE_PATH injection is minimal overhead
- Nginx rewrite is fast
- All caching strategies still work
- SSE streaming performance unchanged

## Future Enhancements

Possible improvements:
- [ ] Auto-detect subpath from X-Forwarded-Prefix header
- [ ] Support multiple subpaths with single instance
- [ ] Configuration UI for changing subpath
- [ ] Health check that validates subpath configuration
- [ ] Middleware for path prefix handling
- [ ] Automatic redirect from wrong paths

## References

- [FastAPI Sub Applications](https://fastapi.tiangolo.com/advanced/sub-applications/)
- [FastAPI Behind a Proxy](https://fastapi.tiangolo.com/advanced/behind-a-proxy/)
- [Nginx Rewrite Rules](http://nginx.org/en/docs/http/ngx_http_rewrite_module.html)
- [Server-Sent Events Specification](https://html.spec.whatwg.org/multipage/server-sent-events.html)

---

**Status**: ✅ Complete and tested
**Version**: 1.0  
**Date**: 2025-12-31
