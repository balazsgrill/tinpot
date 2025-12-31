# Multi-Worker Setup - Quick Reference

## To Answer Your Questions

### ✅ Q1: Requirements are installed on the worker?

**Yes!** Action dependencies are installed on worker containers, not the API.

- **API**: Only needs FastAPI, Redis client, and core Tinpot code
- **Worker**: Needs all action dependencies (Docker, Git, ML libraries, etc.)

### ✅ Q2: Can you deploy multiple workers for different actions?

**Absolutely!** Here's how:

## Quick Start

### Option 1: Single Worker (Current Setup)
```bash
docker-compose up -d
```
- All actions run on one worker type
- All dependencies in one requirements.txt

### Option 2: Multiple Specialized Workers
```bash
./start_multi_worker.sh
```
- Different workers for different action types
- Separate dependencies per worker
- Better resource isolation

## Architecture

### Current: Single Worker
```
API → Redis → Worker (all actions)
                ├─ Maintenance
                ├─ DevOps
                └─ Database
```

### Multi-Worker: Specialized Queues
```
API → Redis → Worker-Default (queue: default)
          │     ├─ Maintenance
          │     └─ General
          │
          └─→ Worker-DevOps (queue: devops)
                ├─ DevOps
                └─ Database
```

## Files Created

### Configuration
- `docker-compose.multi-worker.yml` - Multi-worker setup
- `Dockerfile.worker-devops` - DevOps worker with specialized tools
- `start_multi_worker.sh` - Start script

### Documentation  
- `docs/MULTIPLE_WORKERS.md` - Complete architecture guide
- This file - Quick reference

### Code Updates
- `app/tinpot/decorators.py` - Added `queue` parameter
- `app/worker.py` - Added action filtering by group
- `app/main.py` - Added queue routing
- `actions/devops_actions.py` - Example DevOps actions

## Action Declaration

### Default Queue
```python
@action(group="General")
def my_action():
    pass
```
Routes to: `default` queue → `worker-default`

### Specific Queue
```python
@action(group="DevOps", queue="devops")
def deploy():
    pass
```
Routes to: `devops` queue → `worker-devops`

## Deployment Patterns

### Pattern 1: By Technology
```yaml
worker-python:     # Python data processing
  ACTION_GROUPS: DataScience,Analytics
  
worker-nodejs:     # Node.js actions
  ACTION_GROUPS: WebScraping,API
  
worker-docker:     # Docker operations  
  ACTION_GROUPS: DevOps,Deployment
```

### Pattern 2: By Resource
```yaml
worker-cpu:        # CPU-intensive
  concurrency: 8
  ACTION_GROUPS: DataProcessing,Encoding
  
worker-gpu:        # GPU workloads
  concurrency: 1
  devices: [gpu]
  ACTION_GROUPS: MachineLearning,VideoProcessing
```

### Pattern 3: By Security
```yaml
worker-public:     # Safe actions
  ACTION_GROUPS: General,Reporting
  
worker-privileged: # Sensitive operations
  ACTION_GROUPS: Database,Production
  volumes: [/var/run/docker.sock]
```

## Dependency Management

### Single Worker
**requirements.txt** (everything)
```
fastapi
celery
redis
docker
pytorch
ansible
```

### Multi-Worker
**requirements.txt** (shared/base)
```
fastapi
celery
redis
```

**Dockerfile.worker-devops** (DevOps only)
```dockerfile
RUN pip install docker ansible gitpython
```

**Dockerfile.worker-ml** (ML only)
```dockerfile  
RUN pip install torch transformers scikit-learn
```

## Monitoring

### Check Active Workers
```bash
docker-compose -f docker-compose.multi-worker.yml ps
```

### View Queue Lengths
```bash
# Default queue
docker-compose -f docker-compose.multi-worker.yml exec redis redis-cli LLEN default

# DevOps queue
docker-compose -f docker-compose.multi-worker.yml exec redis redis-cli LLEN devops
```

### Worker Stats
```bash
# Default worker
docker-compose -f docker-compose.multi-worker.yml exec worker-default \
  celery -A app.worker inspect stats

# DevOps worker  
docker-compose -f docker-compose.multi-worker.yml exec worker-devops \
  celery -A app.worker inspect stats
```

### View Loaded Actions
```bash
# Check worker logs to see which actions loaded
docker-compose -f docker-compose.multi-worker.yml logs worker-default | grep "Loaded"
docker-compose -f docker-compose.multi-worker.yml logs worker-devops | grep "Loaded"
```

## Scaling

### Scale a Worker Type
```bash
# Run 4 default workers
docker-compose -f docker-compose.multi-worker.yml up -d --scale worker-default=4

# Run 2 devops workers
docker-compose -f docker-compose.multi-worker.yml up -d --scale worker-devops=2
```

### Auto-Scale Based on Queue Depth
```yaml
# Add Celery autoscaler
worker-default:
  command: celery -A app.worker worker 
    --autoscale=10,2  # max 10, min 2 processes
```

## Troubleshooting

### Actions Not Found
**Problem**: Worker can't find actions
**Solution**: Check `ACTION_GROUPS` environment variable matches action `group`

```bash
# Check what worker loaded
docker-compose -f docker-compose.multi-worker.yml logs worker-devops | grep "Filtered"
```

### Wrong Worker Executing
**Problem**: Action runs on wrong worker
**Solution**: Check queue in `@action` decorator matches worker's `-Q` flag

```python
# This action
@action(queue="devops")  
def my_action(): pass

# Needs this worker
command: celery ... -Q devops
```

### Dependencies Missing
**Problem**: `ImportError` in worker
**Solution**: Install dependencies in worker's Dockerfile

```dockerfile
# Add to Dockerfile.worker-devops
RUN pip install missing-package
```

## Best Practices

✅ **Use queue routing** - Don't rely on action filtering alone
✅ **Resource limits** - Set CPU/memory limits per worker type
✅ **Health checks** - Monitor worker health
✅ **Separate Dockerfiles** - Keep dependencies isolated
✅ **Scale independently** - Scale workers based on queue depth
✅ **Monitor queues** - Alert on queue buildup

## Example: Add New ML Worker

1. Create Dockerfile
```dockerfile
# Dockerfile.worker-ml
FROM pytorch/pytorch:2.1.0
COPY requirements.txt .
RUN pip install -r requirements.txt
RUN pip install transformers scikit-learn
# ... rest of setup
```

2. Add to docker-compose
```yaml
worker-ml:
  build:
    dockerfile: Dockerfile.worker-ml
  environment:
    ACTION_GROUPS: MachineLearning,AI
  command: celery -A app.worker worker -Q ml --concurrency=1
```

3. Create actions
```python
@action(group="MachineLearning", queue="ml")
def run_inference(model: str):
    import torch
    # ML code here
```

4. Start
```bash
docker-compose up -d worker-ml
```

## Summary

| Aspect | Single Worker | Multi-Worker |
|--------|--------------|--------------|
| Setup | Simple | More complex |
| Dependencies | All in one | Isolated |
| Resources | Shared | Dedicated |
| Scaling | All or nothing | Per queue |
| Isolation | Low | High |
| Cost | Lower | Higher |

**Use Multi-Worker When:**
- Actions have conflicting dependencies
- Need different resource profiles (CPU vs GPU)
- Want to isolate sensitive operations
- Need independent scaling per workload

**Use Single Worker When:**
- Few actions with compatible dependencies  
- Tight resource budget
- Simple deployment preferred
- Low concurrent load
