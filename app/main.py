"""
FastAPI server with SSE streaming for real-time log delivery.
"""
import os
import sys
import asyncio
import json
from typing import Dict, Any, Optional
from datetime import datetime

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import StreamingResponse, HTMLResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import redis.asyncio as aioredis
from celery.result import AsyncResult

# Add app directory to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from tinpot.decorators import list_actions, ACTION_REGISTRY
from tinpot.loader import discover_actions


# Configuration
REDIS_URL = os.environ.get("REDIS_URL", "redis://localhost:6379")
ACTIONS_DIR = os.environ.get("ACTIONS_DIR", "/opt/tinpot/actions")


# Initialize FastAPI app
app = FastAPI(title="Tinpot", version="1.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# Models
class ExecuteActionRequest(BaseModel):
    parameters: Dict[str, Any] = {}


class ExecutionResponse(BaseModel):
    execution_id: str
    action_name: str
    status: str
    stream_url: str


# Import Celery app from worker (for task submission)
from worker import celery_app


@app.on_event("startup")
async def startup_event():
    """Initialize actions on startup."""
    print("Starting Tinpot API...")
    print(f"Redis URL: {REDIS_URL}")
    print(f"Actions directory: {ACTIONS_DIR}")
    
    # Discover actions (read metadata only, worker will execute)
    modules = discover_actions(ACTIONS_DIR)
    print(f"Discovered {len(ACTION_REGISTRY)} actions from {len(modules)} modules")


@app.get("/")
async def root():
    """Root endpoint - redirect to UI."""
    return HTMLResponse("""
        <html>
            <head><meta http-equiv="refresh" content="0; url=/static/index.html"></head>
            <body>Redirecting to Tinpot UI...</body>
        </html>
    """)


@app.get("/health")
async def health_check():
    """
    Health check endpoint for load balancers and monitoring.
    Returns 200 if service is healthy, 503 if unhealthy.
    """
    try:
        # Check Redis connection
        import redis as redis_lib
        redis_client = redis_lib.from_url(REDIS_URL)
        redis_client.ping()
        return {
            "status": "healthy",
            "redis": "connected",
            "timestamp": datetime.utcnow().isoformat()
        }
    except Exception as e:
        raise HTTPException(
            status_code=503,
            detail=f"Service unhealthy: {str(e)}"
        )


@app.get("/ready")
async def readiness_check():
    """
    Readiness check - ensures actions are loaded and service is ready to serve requests.
    """
    if len(ACTION_REGISTRY) == 0:
        raise HTTPException(
            status_code=503,
            detail="Not ready: No actions loaded"
        )
    
    return {
        "status": "ready",
        "actions_count": len(ACTION_REGISTRY),
        "actions": list(ACTION_REGISTRY.keys()),
        "timestamp": datetime.utcnow().isoformat()
    }


@app.get("/api/actions")
async def get_actions():
    """List all available actions with their metadata."""
    return list_actions()


@app.post("/api/actions/{action_name}/execute", response_model=ExecutionResponse)
async def execute_action(action_name: str, request: ExecuteActionRequest):
    """
    Trigger an action to run asynchronously.
    
    Returns execution ID and SSE stream URL.
    """
    if action_name not in ACTION_REGISTRY:
        raise HTTPException(status_code=404, detail=f"Action not found: {action_name}")
    
    # Get action metadata including queue
    action_info = ACTION_REGISTRY[action_name]
    queue_name = action_info.get('queue', 'default')
    
    # Submit task to Celery with queue routing
    task = celery_app.send_task(
        'tinpot.execute_action',
        args=[action_name, request.parameters],
        queue=queue_name,
    )
    
    return ExecutionResponse(
        execution_id=task.id,
        action_name=action_name,
        status="submitted",
        stream_url=f"/api/executions/{task.id}/stream"
    )


@app.get("/api/executions/{execution_id}/status")
async def get_execution_status(execution_id: str):
    """Get the current status of an execution."""
    result = AsyncResult(execution_id, app=celery_app)
    
    return {
        "execution_id": execution_id,
        "state": result.state,
        "info": result.info if result.info else {},
        "ready": result.ready(),
        "successful": result.successful() if result.ready() else None,
    }


@app.post("/api/executions/{execution_id}/cancel")
async def cancel_execution(execution_id: str):
    """Cancel a running execution."""
    celery_app.control.revoke(execution_id, terminate=True, signal='SIGTERM')
    
    return {
        "execution_id": execution_id,
        "status": "cancelled"
    }


@app.get("/api/executions/{execution_id}/stream")
async def stream_logs(execution_id: str, request: Request):
    """
    Stream real-time logs via Server-Sent Events (SSE).
    
    Subscribes to the Redis pub/sub channel for this execution
    and forwards log messages to the client.
    """
    
    async def event_generator():
        """Generate SSE events from Redis pub/sub."""
        redis_client = await aioredis.from_url(REDIS_URL)
        pubsub = redis_client.pubsub()
        
        channel = f"tinpot:logs:{execution_id}"
        await pubsub.subscribe(channel)
        
        try:
            # Send initial connection message
            yield f"data: {json.dumps({'type': 'connected', 'execution_id': execution_id})}\n\n"
            
            # Check task status
            result = AsyncResult(execution_id, app=celery_app)
            
            # Stream logs from Redis
            async for message in pubsub.listen():
                if await request.is_disconnected():
                    break
                
                if message['type'] == 'message':
                    # Forward the log entry
                    log_data = json.loads(message['data'])
                    yield f"data: {json.dumps({'type': 'log', 'data': log_data})}\n\n"
                
                # Check if task is complete
                if result.ready():
                    # Send completion status
                    final_status = {
                        'type': 'complete',
                        'state': result.state,
                        'successful': result.successful(),
                    }
                    
                    if result.successful():
                        final_status['result'] = result.result
                    elif result.failed():
                        final_status['error'] = str(result.info)
                    
                    yield f"data: {json.dumps(final_status)}\n\n"
                    break
                
                # Small delay to prevent tight loop
                await asyncio.sleep(0.1)
        
        finally:
            await pubsub.unsubscribe(channel)
            await redis_client.close()
    
    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",  # Disable nginx buffering
        }
    )


# Mount static files (UI)
import os
static_dir = os.path.join(os.path.dirname(os.path.dirname(__file__)), "static")
app.mount("/static", StaticFiles(directory=static_dir), name="static")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
