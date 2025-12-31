# Tinpot Production Deployment Guide

This guide covers deploying Tinpot components as separate services on different nodes without Docker.

**For subpath deployment** (e.g., `https://example.com/tinpot/`), see [SUBPATH_DEPLOYMENT.md](SUBPATH_DEPLOYMENT.md).

## Architecture Overview

```
┌─────────────┐
│   Clients   │
│  (Browser)  │
└──────┬──────┘
       │ HTTP/SSE
       ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  API Server │────▶│    Redis    │◀────│   Worker    │
│  (FastAPI)  │     │  (Broker)   │     │  (Celery)   │
└─────────────┘     └─────────────┘     └─────────────┘
   Node 1              Node 2              Node 3+
```

## Components

| Component | Purpose | Port | Scalable |
|-----------|---------|------|----------|
| Redis | Message broker + Pub/Sub | 6379 | Limited |
| API Server | HTTP API + SSE streaming | 8000 | Yes ✅ |
| Worker | Execute actions | - | Yes ✅ |

---

## 1. Redis Server

### Installation

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install redis-server
```

**RHEL/CentOS:**
```bash
sudo yum install redis
```

**From source:**
```bash
wget https://download.redis.io/redis-stable.tar.gz
tar -xzf redis-stable.tar.gz
cd redis-stable
make
sudo make install
```

### Configuration

**File:** `/etc/redis/redis.conf`

```conf
# Network
bind 0.0.0.0                    # Listen on all interfaces (or specific IP)
port 6379
protected-mode yes              # Require password
requirepass YOUR_STRONG_PASSWORD

# Persistence (optional but recommended)
save 900 1                      # Save after 900 sec if 1 key changed
save 300 10                     # Save after 300 sec if 10 keys changed
save 60 10000                   # Save after 60 sec if 10000 keys changed

# Limits
maxmemory 2gb                   # Set based on your needs
maxmemory-policy allkeys-lru    # Eviction policy

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log

# Performance
tcp-backlog 511
timeout 0
tcp-keepalive 300
```

### Systemd Service

**File:** `/etc/systemd/system/redis.service`

```ini
[Unit]
Description=Redis In-Memory Data Store
After=network.target

[Service]
Type=notify
User=redis
Group=redis
ExecStart=/usr/bin/redis-server /etc/redis/redis.conf
ExecStop=/usr/bin/redis-cli shutdown
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Start Redis

```bash
sudo systemctl daemon-reload
sudo systemctl enable redis
sudo systemctl start redis
sudo systemctl status redis
```

### Verify

```bash
redis-cli -h localhost -p 6379 -a YOUR_PASSWORD ping
# Should return: PONG
```

---

## 2. API Server

### System Requirements

- Python 3.11+
- 2GB RAM minimum
- Network access to Redis

### Installation

```bash
# Create user
sudo useradd -r -s /bin/bash -d /opt/tinpot tinpot

# Create directory
sudo mkdir -p /opt/tinpot
sudo chown tinpot:tinpot /opt/tinpot

# Switch to tinpot user
sudo su - tinpot

# Clone or copy code
cd /opt/tinpot
# Copy your code here

# Create virtual environment
python3.11 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r requirements-api.txt

# Copy actions directory
mkdir -p actions
# Copy your actions/*.py files here
```

### Environment Variables

**File:** `/opt/tinpot/api.env`

```bash
# Redis Connection
REDIS_URL=redis://:YOUR_PASSWORD@redis-host:6379/0

# Actions Directory
ACTIONS_DIR=/opt/tinpot/actions

# API Configuration
HOST=0.0.0.0
PORT=8000
WORKERS=4                        # Number of Uvicorn workers
LOG_LEVEL=info

# Optional: Security
# ALLOWED_ORIGINS=https://your-domain.com

# Optional: Authentication headers (if using proxy)
# AUTH_HEADER=X-Remote-User
```

### Systemd Service

**File:** `/etc/systemd/system/tinpot-api.service`

```ini
[Unit]
Description=Tinpot API Server
After=network.target redis.service
Wants=redis.service

[Service]
Type=simple
User=tinpot
Group=tinpot
WorkingDirectory=/opt/tinpot
EnvironmentFile=/opt/tinpot/api.env

# Use virtual environment
ExecStart=/opt/tinpot/venv/bin/uvicorn app.main:app \
    --host ${HOST} \
    --port ${PORT} \
    --workers ${WORKERS} \
    --log-level ${LOG_LEVEL}

# Restart policy
Restart=always
RestartSec=10
StartLimitBurst=5
StartLimitInterval=60

# Security
NoNewPrivileges=true
PrivateTmp=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tinpot-api

[Install]
WantedBy=multi-user.target
```

### Start API Server

```bash
sudo systemctl daemon-reload
sudo systemctl enable tinpot-api
sudo systemctl start tinpot-api
sudo systemctl status tinpot-api
```

### Verify

```bash
curl http://localhost:8000/api/actions
# Should return JSON with actions list
```

### Logs

```bash
# View logs
sudo journalctl -u tinpot-api -f

# View recent logs
sudo journalctl -u tinpot-api -n 100

# View logs since time
sudo journalctl -u tinpot-api --since "2025-12-31 10:00:00"
```

---

## 3. Worker

### System Requirements

- Python 3.11+
- 4GB RAM minimum (more for heavy workloads)
- Network access to Redis
- Dependencies for your actions (Docker, Git, etc.)

### Installation

```bash
# Create user (if not already exists)
sudo useradd -r -s /bin/bash -d /opt/tinpot tinpot

# Create directory
sudo mkdir -p /opt/tinpot
sudo chown tinpot:tinpot /opt/tinpot

# Switch to tinpot user
sudo su - tinpot

cd /opt/tinpot

# Create virtual environment
python3.11 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r requirements-worker.txt

# Copy actions directory
mkdir -p actions
# Copy your actions/*.py files here
```

### Environment Variables

**File:** `/opt/tinpot/worker.env`

```bash
# Redis Connection
REDIS_URL=redis://:YOUR_PASSWORD@redis-host:6379/0

# Actions Directory
ACTIONS_DIR=/opt/tinpot/actions

# Worker Configuration
WORKER_NAME=worker-01
CONCURRENCY=4                    # Number of concurrent tasks
QUEUE_NAME=default              # Queue to consume from
LOG_LEVEL=info

# Optional: Action filtering
# ACTION_GROUPS=General,Maintenance

# Optional: Resource limits
# Set via systemd service (see below)
```

### Systemd Service (Default Worker)

**File:** `/etc/systemd/system/tinpot-worker.service`

```ini
[Unit]
Description=Tinpot Worker (Default Queue)
After=network.target redis.service
Wants=redis.service

[Service]
Type=simple
User=tinpot
Group=tinpot
WorkingDirectory=/opt/tinpot
EnvironmentFile=/opt/tinpot/worker.env

# Use virtual environment
ExecStart=/opt/tinpot/venv/bin/celery -A app.worker worker \
    --loglevel=${LOG_LEVEL} \
    --concurrency=${CONCURRENCY} \
    --queues=${QUEUE_NAME} \
    --hostname=${WORKER_NAME}@%h

# Restart policy
Restart=always
RestartSec=10
StartLimitBurst=5
StartLimitInterval=60

# Graceful shutdown
TimeoutStopSec=300
KillMode=mixed
KillSignal=SIGTERM

# Resource limits (adjust based on your needs)
CPUQuota=200%                    # 2 CPU cores
MemoryMax=4G
MemoryHigh=3G

# Security
NoNewPrivileges=true
PrivateTmp=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tinpot-worker

[Install]
WantedBy=multi-user.target
```

### Start Worker

```bash
sudo systemctl daemon-reload
sudo systemctl enable tinpot-worker
sudo systemctl start tinpot-worker
sudo systemctl status tinpot-worker
```

### Verify

```bash
# Check worker is connected
/opt/tinpot/venv/bin/celery -A app.worker inspect ping
# Should return: pong from worker

# Check active tasks
/opt/tinpot/venv/bin/celery -A app.worker inspect active

# Check worker stats
/opt/tinpot/venv/bin/celery -A app.worker inspect stats
```

### Logs

```bash
# View logs
sudo journalctl -u tinpot-worker -f

# View recent logs
sudo journalctl -u tinpot-worker -n 100
```

---

## 4. Multiple Specialized Workers

### DevOps Worker

**File:** `/opt/tinpot/worker-devops.env`

```bash
REDIS_URL=redis://:YOUR_PASSWORD@redis-host:6379/0
ACTIONS_DIR=/opt/tinpot/actions
WORKER_NAME=worker-devops-01
CONCURRENCY=2
QUEUE_NAME=devops
ACTION_GROUPS=DevOps,Database
LOG_LEVEL=info
```

**File:** `/etc/systemd/system/tinpot-worker-devops.service`

```ini
[Unit]
Description=Tinpot Worker (DevOps Queue)
After=network.target redis.service
Wants=redis.service

[Service]
Type=simple
User=tinpot
Group=tinpot
WorkingDirectory=/opt/tinpot
EnvironmentFile=/opt/tinpot/worker-devops.env

ExecStart=/opt/tinpot/venv/bin/celery -A app.worker worker \
    --loglevel=${LOG_LEVEL} \
    --concurrency=${CONCURRENCY} \
    --queues=${QUEUE_NAME} \
    --hostname=${WORKER_NAME}@%h

Restart=always
RestartSec=10
TimeoutStopSec=300
KillMode=mixed
KillSignal=SIGTERM

CPUQuota=400%                    # 4 CPU cores for DevOps tasks
MemoryMax=8G

StandardOutput=journal
StandardError=journal
SyslogIdentifier=tinpot-worker-devops

[Install]
WantedBy=multi-user.target
```

Install dependencies:
```bash
# Install DevOps-specific dependencies
sudo apt install docker.io
pip install docker gitpython paramiko
```

Start:
```bash
sudo systemctl enable tinpot-worker-devops
sudo systemctl start tinpot-worker-devops
```

---

## 5. Reverse Proxy (Production)

### Nginx Configuration

#### Option 1: Root Path Deployment

**File:** `/etc/nginx/sites-available/tinpot`

```nginx
upstream tinpot_api {
    # Multiple API servers for load balancing
    server api-node1:8000;
    server api-node2:8000;
    server api-node3:8000;
    
    # Load balancing method
    least_conn;
    keepalive 32;
}

server {
    listen 80;
    listen [::]:80;
    server_name tinpot.yourdomain.com;

    # Redirect to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name tinpot.yourdomain.com;

    # SSL configuration
    ssl_certificate /etc/letsencrypt/live/tinpot.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tinpot.yourdomain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # Logging
    access_log /var/log/nginx/tinpot-access.log;
    error_log /var/log/nginx/tinpot-error.log;

    # Client max body size
    client_max_body_size 10M;

    # Proxy to API
    location / {
        proxy_pass http://tinpot_api;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # SSE streaming endpoints (no buffering)
    location /api/executions/ {
        proxy_pass http://tinpot_api;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # SSE specific
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
    }

    # Static files (optional caching)
    location /static/ {
        proxy_pass http://tinpot_api;
        proxy_cache_valid 200 1h;
        expires 1h;
        add_header Cache-Control "public, immutable";
    }
}
```

#### Option 2: Subpath Deployment

Deploy Tinpot under a subpath like `https://yourdomain.com/tinpot/`

**File:** `/etc/nginx/sites-available/yourdomain`

```nginx
upstream tinpot_api {
    server api-node1:8000;
    server api-node2:8000;
    
    least_conn;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name yourdomain.com;

    # SSL configuration
    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;
    
    # ... other locations for other apps ...
    
    # Tinpot on /tinpot/ subpath
    location /tinpot/ {
        # Rewrite path - remove /tinpot prefix before proxying
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
    
    # SSE streaming for subpath
    location /tinpot/api/executions/ {
        rewrite ^/tinpot/(.*)$ /$1 break;
        
        proxy_pass http://tinpot_api;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Prefix /tinpot;
        
        # SSE specific
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
    }
    
    # Static files on subpath
    location /tinpot/static/ {
        rewrite ^/tinpot/(.*)$ /$1 break;
        
        proxy_pass http://tinpot_api;
        proxy_set_header X-Forwarded-Prefix /tinpot;
        proxy_cache_valid 200 1h;
        expires 1h;
        add_header Cache-Control "public, immutable";
    }
}
```

**Configure FastAPI for Subpath:**

Update `/opt/tinpot/api.env`:

```bash
# Add root_path for subpath deployment
ROOT_PATH=/tinpot
```

Update systemd service `/etc/systemd/system/tinpot-api.service`:

```ini
ExecStart=/opt/tinpot/venv/bin/uvicorn app.main:app \
    --host ${HOST} \
    --port ${PORT} \
    --workers ${WORKERS} \
    --log-level ${LOG_LEVEL} \
    --root-path ${ROOT_PATH}
```

Or update `app/main.py` directly:

```python
import os

app = FastAPI(
    title="Tinpot",
    version="1.0",
    root_path=os.environ.get("ROOT_PATH", ""),  # Enable subpath support
)
```

**Update Frontend for Subpath:**

Update `static/index.html` to use relative URLs:

```javascript
// Change absolute paths
fetch('/api/actions')

// To relative paths (automatically includes root_path)
fetch('api/actions')

// Or get root path from window.location
const basePath = window.location.pathname.split('/').slice(0, -1).join('/') || '';
fetch(`${basePath}/api/actions`)
```

#### Option 3: Multiple Instances on Different Subpaths

Run multiple Tinpot instances on different subpaths:

```nginx
# Production instance
location /tinpot/ {
    rewrite ^/tinpot/(.*)$ /$1 break;
    proxy_pass http://tinpot_prod_api;
    proxy_set_header X-Forwarded-Prefix /tinpot;
}

# Staging instance
location /tinpot-staging/ {
    rewrite ^/tinpot-staging/(.*)$ /$1 break;
    proxy_pass http://tinpot_staging_api;
    proxy_set_header X-Forwarded-Prefix /tinpot-staging;
}

# Development instance
location /tinpot-dev/ {
    rewrite ^/tinpot-dev/(.*)$ /$1 break;
    proxy_pass http://tinpot_dev_api;
    proxy_set_header X-Forwarded-Prefix /tinpot-dev;
}
```

Enable and reload:
```bash
sudo ln -s /etc/nginx/sites-available/tinpot /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

## 6. Monitoring & Health Checks

### Health Check Endpoints

Add to `app/main.py`:

```python
@app.get("/health")
async def health_check():
    """Health check endpoint for load balancers."""
    try:
        # Check Redis connection
        redis_client = redis.from_url(REDIS_URL)
        redis_client.ping()
        return {"status": "healthy", "redis": "connected"}
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Unhealthy: {str(e)}")

@app.get("/ready")
async def readiness_check():
    """Readiness check - ensures actions are loaded."""
    if len(ACTION_REGISTRY) == 0:
        raise HTTPException(status_code=503, detail="No actions loaded")
    return {"status": "ready", "actions": len(ACTION_REGISTRY)}
```

### Monitoring Scripts

**Check API Health:**
```bash
#!/bin/bash
# /usr/local/bin/check-tinpot-api.sh

response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8000/health)
if [ "$response" != "200" ]; then
    echo "API health check failed"
    exit 1
fi
```

**Check Worker Health:**
```bash
#!/bin/bash
# /usr/local/bin/check-tinpot-worker.sh

output=$(/opt/tinpot/venv/bin/celery -A app.worker inspect ping 2>&1)
if [[ $output != *"pong"* ]]; then
    echo "Worker not responding"
    exit 1
fi
```

**Check Queue Depth:**
```bash
#!/bin/bash
# /usr/local/bin/check-tinpot-queue.sh

QUEUE=$1
THRESHOLD=100

length=$(redis-cli -h redis-host -a YOUR_PASSWORD LLEN "$QUEUE" 2>/dev/null)
if [ "$length" -gt "$THRESHOLD" ]; then
    echo "Queue $QUEUE has $length items (threshold: $THRESHOLD)"
    exit 1
fi
```

---

## 7. Scaling

### Scale API Servers (Horizontal)

1. Deploy API to multiple nodes
2. Configure load balancer (Nginx, HAProxy, AWS ALB)
3. Use same Redis instance for all APIs
4. Ensure `REDIS_URL` points to same Redis

### Scale Workers (Horizontal)

**Same Queue:**
```bash
# Node 1
sudo systemctl start tinpot-worker

# Node 2  
sudo systemctl start tinpot-worker

# Node 3
sudo systemctl start tinpot-worker

# All consume from 'default' queue
```

**Different Queues:**
```bash
# Node 1 - default queue
sudo systemctl start tinpot-worker

# Node 2 - devops queue
sudo systemctl start tinpot-worker-devops

# Node 3 - ml queue
sudo systemctl start tinpot-worker-ml
```

### Scale Redis (Vertical)

- Single Redis instance (simple, but single point of failure)
- Redis Sentinel (high availability)
- Redis Cluster (sharding, complex)

**For most cases:** Single Redis with enough memory is sufficient

---

## 8. Security Checklist

### Network
- ✅ Firewall rules (Redis only accessible from API/Worker nodes)
- ✅ Redis password authentication
- ✅ TLS/SSL for public API (via Nginx)
- ✅ Private network for internal communication

### Application
- ✅ Run as non-root user (tinpot)
- ✅ Virtual environment for Python dependencies
- ✅ Input validation in actions
- ✅ Rate limiting (via Nginx or API middleware)

### System
- ✅ Keep packages updated
- ✅ Systemd security directives (NoNewPrivileges, PrivateTmp)
- ✅ Resource limits (CPU, Memory)
- ✅ Logging enabled

### Redis
- ✅ `requirepass` enabled
- ✅ `protected-mode yes`
- ✅ Bind to specific interfaces (not 0.0.0.0 in production)
- ✅ Disable dangerous commands: `rename-command CONFIG ""`

---

## 9. Backup & Recovery

### Redis Backup

```bash
# Configure in redis.conf
save 900 1
save 300 10
save 60 10000

# Manual backup
redis-cli -h redis-host -a YOUR_PASSWORD SAVE

# Backup RDB file
cp /var/lib/redis/dump.rdb /backup/redis-$(date +%Y%m%d).rdb
```

### Application Backup

```bash
# Backup actions and configuration
tar -czf /backup/tinpot-$(date +%Y%m%d).tar.gz \
    /opt/tinpot/actions \
    /opt/tinpot/*.env \
    /etc/systemd/system/tinpot-*.service
```

---

## 10. Troubleshooting

### API won't start
```bash
# Check logs
sudo journalctl -u tinpot-api -n 50

# Common issues:
# - Can't connect to Redis: Check REDIS_URL and firewall
# - Port already in use: Check with 'sudo lsof -i :8000'
# - Import errors: Check virtual environment and requirements
```

### Worker not picking up tasks
```bash
# Check worker logs
sudo journalctl -u tinpot-worker -n 50

# Verify worker is registered
/opt/tinpot/venv/bin/celery -A app.worker inspect active_queues

# Check Redis connection
redis-cli -h redis-host -a YOUR_PASSWORD PING

# Check queue has tasks
redis-cli -h redis-host -a YOUR_PASSWORD LLEN default
```

### Actions not loading
```bash
# Check ACTIONS_DIR path
ls -la /opt/tinpot/actions/

# Check file permissions
sudo -u tinpot ls -la /opt/tinpot/actions/

# Check Python can import
sudo -u tinpot /opt/tinpot/venv/bin/python -c "import actions.example_actions"
```

---

## Quick Reference

### Start All Services

```bash
# Redis
sudo systemctl start redis

# API (on API nodes)
sudo systemctl start tinpot-api

# Workers (on worker nodes)
sudo systemctl start tinpot-worker
sudo systemctl start tinpot-worker-devops  # If using multiple queues
```

### Stop All Services

```bash
sudo systemctl stop tinpot-worker
sudo systemctl stop tinpot-api
sudo systemctl stop redis
```

### View Logs

```bash
# API
sudo journalctl -u tinpot-api -f

# Worker
sudo journalctl -u tinpot-worker -f

# Redis
sudo journalctl -u redis -f
```

### Check Status

```bash
# All services
sudo systemctl status redis tinpot-api tinpot-worker

# Worker stats
/opt/tinpot/venv/bin/celery -A app.worker inspect stats

# Queue depths
redis-cli -h redis-host -a YOUR_PASSWORD LLEN default
redis-cli -h redis-host -a YOUR_PASSWORD LLEN devops
```

---

## Environment Variables Summary

### All Components

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `REDIS_URL` | Yes | Redis connection string | `redis://:password@host:6379/0` |
| `ACTIONS_DIR` | Yes | Path to actions directory | `/opt/tinpot/actions` |
| `LOG_LEVEL` | No | Logging level | `info`, `debug`, `warning` |

### API Only

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `HOST` | No | Bind address | `0.0.0.0` (default) |
| `PORT` | No | Listen port | `8000` (default) |
| `WORKERS` | No | Uvicorn workers | `4` |

### Worker Only

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `WORKER_NAME` | No | Worker identifier | `worker-01` |
| `CONCURRENCY` | No | Concurrent tasks | `4` |
| `QUEUE_NAME` | No | Queue to consume | `default`, `devops` |
| `ACTION_GROUPS` | No | Filter actions by group | `DevOps,Database` |

---

## Production Checklist

Before going live:

- [ ] Redis configured with password
- [ ] Redis persistence enabled
- [ ] Firewall rules configured
- [ ] SSL/TLS certificates installed
- [ ] Nginx reverse proxy configured
- [ ] Health check endpoints working
- [ ] Monitoring set up
- [ ] Backup scripts configured
- [ ] Log rotation configured
- [ ] Resource limits set
- [ ] All services enabled to start on boot
- [ ] Documentation updated with your specifics
- [ ] Disaster recovery plan documented
- [ ] Security audit completed
