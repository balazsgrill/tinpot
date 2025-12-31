# Multiple Workers Architecture

## Overview

You can deploy multiple specialized workers, each handling different types of actions with different dependencies.

## Architecture Options

### Option 1: Queue-Based Routing (Recommended)

Different workers listen to different Celery queues based on action groups.

**Benefits:**
- Separate dependencies per worker type
- Better resource isolation
- Scale workers independently
- Different concurrency settings per queue

### Option 2: Worker Pools

Single worker type with multiple pools for different priorities/resources.

## Implementation Example

### 1. Update Action Decorator to Support Queues

```python
# app/tinpot/decorators.py
def action(
    name: Optional[str] = None,
    group: Optional[str] = "General",
    description: Optional[str] = None,
    queue: str = "default",  # NEW: specify queue
):
    # ... existing code ...
    ACTION_REGISTRY[action_name] = {
        "name": action_name,
        "group": group,
        "description": action_desc.strip(),
        "function": func,
        "parameters": parameters,
        "module": func.__module__,
        "queue": queue,  # Store queue info
    }
```

### 2. Create Specialized Worker Configurations

**File: `app/worker_devops.py`** (DevOps-specific worker)
```python
import os
import sys
from celery import Celery

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

REDIS_URL = os.environ.get("REDIS_URL", "redis://localhost:6379")

celery_app = Celery("tinpot-devops", broker=REDIS_URL, backend=REDIS_URL)

celery_app.conf.update(
    task_serializer='json',
    task_routes={
        'tinpot.execute_action': {'queue': 'devops'}
    },
)

# Only load DevOps actions
os.environ['ACTION_FILTER'] = 'DevOps'

from worker import *  # Import common worker setup
```

### 3. Docker Compose with Multiple Workers

```yaml
services:
  # General worker - handles Maintenance, General actions
  worker-general:
    build: .
    container_name: tinpot-worker-general
    environment:
      - REDIS_URL=redis://redis:6379
      - ACTIONS_DIR=/opt/tinpot/actions
      - ACTION_GROUPS=Maintenance,General  # Filter by group
      - QUEUE_NAME=default
    volumes:
      - ./actions:/opt/tinpot/actions
    depends_on:
      redis:
        condition: service_healthy
    command: celery -A app.worker worker --loglevel=info --concurrency=4 -Q default
    restart: unless-stopped

  # DevOps worker - handles DevOps, Database actions
  worker-devops:
    build:
      context: .
      dockerfile: Dockerfile.worker-devops  # Custom Dockerfile
    container_name: tinpot-worker-devops
    environment:
      - REDIS_URL=redis://redis:6379
      - ACTIONS_DIR=/opt/tinpot/actions
      - ACTION_GROUPS=DevOps,Database
      - QUEUE_NAME=devops
    volumes:
      - ./actions:/opt/tinpot/actions
    depends_on:
      redis:
        condition: service_healthy
    command: celery -A app.worker worker --loglevel=info --concurrency=2 -Q devops
    restart: unless-stopped

  # ML worker - handles ML/AI actions with GPU support
  worker-ml:
    build:
      context: .
      dockerfile: Dockerfile.worker-ml
    container_name: tinpot-worker-ml
    environment:
      - REDIS_URL=redis://redis:6379
      - ACTIONS_DIR=/opt/tinpot/actions
      - ACTION_GROUPS=MachineLearning
      - QUEUE_NAME=ml
    volumes:
      - ./actions:/opt/tinpot/actions
    depends_on:
      redis:
        condition: service_healthy
    deploy:
      resources:
        reservations:
          devices:
            - capabilities: [gpu]
    command: celery -A app.worker worker --loglevel=info --concurrency=1 -Q ml
    restart: unless-stopped
```

### 4. Custom Dockerfiles with Different Dependencies

**Dockerfile.worker-devops:**
```dockerfile
FROM python:3.11-slim

WORKDIR /opt/tinpot

# Base dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# DevOps-specific dependencies
RUN pip install --no-cache-dir \
    ansible==8.5.0 \
    kubernetes==28.1.0 \
    boto3==1.34.0

COPY app/ ./app/
COPY actions/ ./actions/

ENV PYTHONUNBUFFERED=1
ENV REDIS_URL=redis://redis:6379
ENV ACTIONS_DIR=/opt/tinpot/actions

CMD ["celery", "-A", "app.worker", "worker", "--loglevel=info", "--concurrency=2", "-Q", "devops"]
```

**Dockerfile.worker-ml:**
```dockerfile
FROM pytorch/pytorch:2.1.0-cuda12.1-cudnn8-runtime

WORKDIR /opt/tinpot

# Base dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# ML-specific dependencies
RUN pip install --no-cache-dir \
    torch==2.1.0 \
    transformers==4.35.0 \
    scikit-learn==1.3.2

COPY app/ ./app/
COPY actions/ ./actions/

ENV PYTHONUNBUFFERED=1
ENV REDIS_URL=redis://redis:6379
ENV ACTIONS_DIR=/opt/tinpot/actions

CMD ["celery", "-A", "app.worker", "worker", "--loglevel=info", "--concurrency=1", "-Q", "ml"]
```

### 5. Update Worker to Filter Actions by Group

```python
# app/worker.py
def init_worker_process(**kwargs):
    """Called when each worker process starts (after fork)."""
    action_groups = os.environ.get('ACTION_GROUPS', '').split(',')
    action_groups = [g.strip() for g in action_groups if g.strip()]
    
    # Discover all actions
    discover_actions(ACTIONS_DIR)
    
    # Filter actions if ACTION_GROUPS is set
    if action_groups:
        filtered_registry = {
            name: info 
            for name, info in ACTION_REGISTRY.items()
            if info['group'] in action_groups
        }
        ACTION_REGISTRY.clear()
        ACTION_REGISTRY.update(filtered_registry)
        print(f"Filtered to groups: {action_groups}")
        print(f"Loaded {len(ACTION_REGISTRY)} actions")
    
    # Setup logging in each worker process
    setup_logging(REDIS_URL)
```

### 6. Update API to Route to Correct Queue

```python
# app/main.py
@app.post("/api/actions/{action_name}/execute", response_model=ExecutionResponse)
async def execute_action(action_name: str, request: ExecuteActionRequest):
    if action_name not in ACTION_REGISTRY:
        raise HTTPException(status_code=404, detail=f"Action not found: {action_name}")
    
    # Get the queue for this action
    action_info = ACTION_REGISTRY[action_name]
    queue_name = action_info.get('queue', 'default')
    
    # Submit task to appropriate queue
    task = celery_app.send_task(
        'tinpot.execute_action',
        args=[action_name, request.parameters],
        queue=queue_name,  # Route to correct queue
    )
    
    return ExecutionResponse(
        execution_id=task.id,
        action_name=action_name,
        status="submitted",
        stream_url=f"/api/executions/{task.id}/stream"
    )
```

### 7. Example Actions with Queue Assignment

```python
# actions/devops_actions.py
from tinpot import action

@action(group="DevOps", queue="devops", description="Deploy with Kubernetes")
def k8s_deploy(namespace: str = "default"):
    import kubernetes
    # Requires kubernetes library only installed on devops worker
    print(f"Deploying to {namespace}")
    

# actions/ml_actions.py
from tinpot import action

@action(group="MachineLearning", queue="ml", description="Run ML inference")
def run_inference(model: str = "bert-base"):
    import torch
    from transformers import AutoModel
    # Requires PyTorch + transformers only on ML worker
    print(f"Loading model {model}")
    

# actions/general_actions.py
from tinpot import action

@action(group="General", queue="default", description="Simple task")
def simple_task():
    # No special dependencies
    print("Running simple task")
```

## Scaling Examples

### Scale by Load
```bash
# Run 4 general workers
docker-compose up -d --scale worker-general=4

# Run 2 DevOps workers
docker-compose up -d --scale worker-devops=2

# Single GPU worker
docker-compose up -d worker-ml
```

### Resource Limits
```yaml
worker-general:
  deploy:
    resources:
      limits:
        cpus: '2'
        memory: 2G
      reservations:
        cpus: '1'
        memory: 1G

worker-devops:
  deploy:
    resources:
      limits:
        cpus: '4'
        memory: 8G
```

## Monitoring

```bash
# Check worker status
docker-compose exec worker-general celery -A app.worker inspect active
docker-compose exec worker-devops celery -A app.worker inspect active

# View queue lengths
docker-compose exec redis redis-cli LLEN celery
docker-compose exec redis redis-cli LLEN devops
docker-compose exec redis redis-cli LLEN ml
```

## Benefits Summary

✅ **Dependency Isolation** - Each worker has only its required packages
✅ **Resource Optimization** - Allocate resources per workload type  
✅ **Independent Scaling** - Scale workers based on queue depth
✅ **Fault Isolation** - Failures in one worker don't affect others
✅ **Security** - Sensitive operations can run in isolated workers
✅ **Cost Efficiency** - GPU workers only for GPU tasks

## Trade-offs

⚠️ **Complexity** - More services to manage
⚠️ **Resource Overhead** - Multiple containers consume more resources
⚠️ **Deployment** - More Docker images to build and maintain
