# Tinpot

🥫 **Tinpot** - A lightweight execution platform for Python automation, built on a robust Go/MQTT architecture. It replaces complex stacks like Python/Celery with a efficient, modular system.

## Features

- **Decoupled Architecture**: Go-native Coordinator and Worker communicating via MQTT.
- **Python-Native Actions**: Define automations as simple Python functions with the `@action` decorator.
- **Embedded Execution**: Go Worker embeds Python (via C-API) for low-latency execution and direct stdout/stderr capture.
- **Real-Time Feedback**: Stream logs from workers to the UI via Server-Sent Events (SSE).
- **Synchronous & Asynchronous**: Trigger actions and wait for results, or fire-and-forget.
- **Simple UI**: Clean web interface for triggering actions and viewing logs.

## Architecture

```
┌─────────────┐       ┌───────────────┐       ┌─────────────┐
│   Browser   │──────▶│  Coordinator  │◀─────▶│    MQTT     │
│             │◀──SSE─│  (Go API)     │       │   Broker    │
└─────────────┘       └───────────────┘       └─────────────┘
                                                      ▲
                                                      │
                                                      ▼
                                              ┌─────────────┐
                                              │   Worker    │
                                              │ (Go+Python) │
                                              └─────────────┘
```

## Quick Start

### Prerequisites

- Go 1.25+ (for building)
- Python 3.11 or 3.12 (for worker runtime)
- MQTT Broker (e.g., Mosquitto)

### Building

```bash
mkdir -p bin
go build -o bin/coordinator ./coordinator
go build -o bin/worker ./worker
```

### Running Locally

1. **Start MQTT Broker**
   ```bash
   # Ensure mosquitto is running on localhost:1883
   docker run -p 1883:1883 eclipse-mosquitto
   ```

2. **Start Coordinator**
   ```bash
   export MQTT_BROKER=tcp://localhost:1883
   ./bin/coordinator
   ```
   The API will be available at `http://localhost:8000`.

3. **Start Worker**
   ```bash
   export MQTT_BROKER=tcp://localhost:1883
   export ACTIONS_DIR=$(pwd)/actions
   ./bin/worker
   ```

## Creating Actions

Actions are standard Python functions located in the `actions/` directory. They utilize the lightweight `tinpot` library.

```python
# actions/my_ops.py
from tinpot import action, action_print
import time

@action(group="Maintenance", description="Clean up old files")
def cleanup(days: int = 7):
    """Delete files older than N days."""
    action_print(f"Cleaning files older than {days} days...")
    # Your logic here
    time.sleep(2)
    action_print("✓ Cleanup complete!")
    return {"files_deleted": 42}
```

## Python Dependencies & Virtual Environments

Tinpot embeds the Python runtime but does not automatically activate virtual environments. To use external libraries (e.g., `requests`, `pandas`) installed in a `venv`, you must add the venv's `site-packages` to the `PYTHONPATH` before running the worker.

**Example:**

```bash
# 1. Create and populate venv
python3 -m venv venv
source venv/bin/activate
pip install requests pandas

# 2. Run worker with PYTHONPATH
export PYTHONPATH=$(pwd)/venv/lib/python3.11/site-packages:$PYTHONPATH
./bin/worker
```

> **Note**: Ensure the Python version of the venv matches the version the worker was linked against (usually system Python 3.11/3.12).

## API Endpoints

- `GET /api/actions`: List all discovered actions.
- `POST /api/actions/{name}/execute`: Trigger an action asynchronously (returns execution ID).
- `POST /api/actions/{name}/sync_execute`: Trigger an action and wait for the result.
- `GET /api/executions/{id}/stream`: Stream logs and status via SSE.
- `GET /api/executions/{id}/status`: Get execution status.

## Configuration

Environment variables:

| Variable | Component | Description | Default |
|----------|-----------|-------------|---------|
| `MQTT_BROKER` | Both | URL of the MQTT broker | `tcp://localhost:1883` |
| `PORT` | Coordinator | HTTP API Port | `8000` |
| `ACTIONS_DIR` | Worker | Path to actions directory | `../actions` |

## Project Structure

```
tinpot/
├── actions/                  # User-defined Python actions
├── bin/                      # Compiled binaries
├── coordinator/              # Go Coordinator (API & MQTT Client)
├── worker/                   # Go Worker (Embedded Python)
├── integration/              # Integration tests (Go + Mochi MQTT)
└── README.md
```

## License

GPL3
