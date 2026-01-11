# Tinpot

🥫 **Tinpot** - A lightweight Python automation platform that replaces shell-script wrappers like OliveTin.

## Features

- **Python-Native Actions**: Define automations as simple Python functions with the `@action` decorator
- **Real-Time Feedback**: Stream logs from workers to the UI via Server-Sent Events (SSE)
- **Distributed Architecture**: Separate control plane (API) and execution plane (Celery workers)
- **Nested Procedures**: Actions can call other actions, with proper call-stack tracking
- **Simple UI**: Clean web interface with action cards and live log streaming
- **Task Cancellation**: Stop running actions with SIGTERM

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

Access the UI at http://localhost:8000

### Running Tests

```bash
# Run the complete test suite (API + UI tests)
./run_tests.sh

# Or manually
docker-compose --profile test run --rm test-runner

# Run specific test suites
docker-compose --profile test run --rm test-runner pytest tests/test_api.py -v
docker-compose --profile test run --rm test-runner pytest tests/test_ui.py -v
```

See [TESTING.md](TESTING.md) for detailed testing documentation.

### Local Development

```bash
# Install dependencies
pip install -r requirements.txt

# Terminal 1: Start Redis
docker run -p 6379:6379 redis:7-alpine

# Terminal 2: Start Worker
cd app
celery -A worker worker --loglevel=info

# Terminal 3: Start API
cd app
uvicorn main:app --reload --port 8000
```

## Creating Actions

Actions are Python functions decorated with `@action`:

```python
# actions/my_actions.py
from tinpot import action
import time

@action(group="Maintenance", description="Clean up old files")
def cleanup(days: int = 7):
    """Delete files older than N days."""
    print(f"Cleaning files older than {days} days...")
    # Your logic here
    time.sleep(2)
    print("✓ Cleanup complete!")
    return {"files_deleted": 42}

@action(group="DevOps")
def deploy(environment: str = "staging"):
    """Deploy to specified environment."""
    print(f"Deploying to {environment}...")
    # Nested call
    cleanup(days=30)
    print("✓ Deploy complete!")
```

## Architecture

```
┌─────────────┐      ┌──────────────┐      ┌────────────┐
│   Browser   │─────▶│  FastAPI     │◀────▶│   Redis    │
│             │◀─SSE─│  (API)       │      │  (Broker)  │
└─────────────┘      └──────────────┘      └────────────┘
                            │                      ▲
                            │                      │
                            ▼                      │
                     ┌──────────────┐             │
                     │   Celery     │─────────────┘
                     │   Worker     │
                     └──────────────┘
```

## API Endpoints

- `GET /api/actions` - List all available actions
- `POST /api/actions/{name}/execute` - Trigger an action
- `POST /api/actions/{name}/sync_execute` - Trigger an action synchronously and wait for result
- `GET /api/executions/{id}/stream` - Stream logs via SSE
- `GET /api/executions/{id}/status` - Get execution status
- `POST /api/executions/{id}/cancel` - Cancel execution

## Configuration

Environment variables:

- `REDIS_URL`: Redis connection URL (default: `redis://localhost:6379`)
- `ACTIONS_DIR`: Path to actions directory (default: `/opt/tinpot/actions`)
- `ROOT_PATH`: Subpath for deployment (default: empty for root path, e.g., `/tinpot`)

### Deployment

- **Production**: See [DEPLOYMENT.md](DEPLOYMENT.md) for systemd-based deployment
- **Subpath**: See [SUBPATH_QUICKSTART.md](SUBPATH_QUICKSTART.md) for deploying under a URL subpath
- **Multi-Worker**: See [MULTI_WORKER_GUIDE.md](MULTI_WORKER_GUIDE.md) for scaling workers

## Project Structure

```
tinpot/
├── actions/                  # User-defined actions
│   ├── __init__.py
│   └── example_actions.py
├── app/
│   ├── main.py              # FastAPI server
│   ├── worker.py            # Celery worker
│   └── tinpot/              # Core library
│       ├── __init__.py
│       ├── decorators.py    # @action decorator
│       ├── logging.py       # Redis log streaming
│       └── loader.py        # Action discovery
├── static/
│   └── index.html           # Web UI
├── Dockerfile
├── docker-compose.yml
└── requirements.txt
```

## License

MIT
