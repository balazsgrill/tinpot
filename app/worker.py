"""
Celery worker configuration and task execution.
"""
import os
import sys
import uuid
from celery import Celery, Task
from typing import Any, Dict

# Add app directory to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from tinpot.decorators import ACTION_REGISTRY, get_action
from tinpot.loader import discover_actions
from tinpot.logging import setup_logging, set_execution_context, action_print


# Redis configuration
REDIS_URL = os.environ.get("REDIS_URL", "redis://localhost:6379")
ACTIONS_DIR = os.environ.get("ACTIONS_DIR", "/opt/tinpot/actions")


# Create Celery app
celery_app = Celery(
    "tinpot",
    broker=REDIS_URL,
    backend=REDIS_URL,
)

celery_app.conf.update(
    task_serializer='json',
    accept_content=['json'],
    result_serializer='json',
    timezone='UTC',
    enable_utc=True,
    task_track_started=True,
    task_send_sent_event=True,
    worker_send_task_events=True,
)

# Process-local flag to track if logging has been set up
_logging_setup = False


class ActionTask(Task):
    """
    Custom Celery task that wraps action execution with logging setup.
    """
    
    def __call__(self, *args, **kwargs):
        """Execute the task with proper logging context."""
        global _logging_setup
        
        # Ensure logging is set up in this worker process
        if not _logging_setup:
            setup_logging(REDIS_URL)
            _logging_setup = True
        
        execution_id = self.request.id
        call_depth = kwargs.pop('_call_depth', 0)
        
        # Set up execution context for logging
        set_execution_context(execution_id, call_depth)
        
        return self.run(*args, **kwargs)


@celery_app.task(base=ActionTask, bind=True, name='tinpot.execute_action')
def execute_action(self, action_name: str, params: Dict[str, Any]):
    """
    Execute a registered action with given parameters.
    
    Args:
        action_name: Name of the action to execute
        params: Dictionary of parameters to pass to the action
    """
    action_func = get_action(action_name)
    
    if not action_func:
        raise ValueError(f"Action not found: {action_name}")
    
    action_print(f"→ Starting action: {action_name}")
    
    try:
        result = action_func(**params)
        action_print(f"✓ Completed action: {action_name}")
        return {"status": "success", "result": result}
    except Exception as e:
        action_print(f"✗ Failed action: {action_name} - {str(e)}")
        raise


# Worker startup - discover actions
def init_worker(**kwargs):
    """Called when worker starts up."""
    print(f"Initializing Tinpot worker...")
    print(f"Redis URL: {REDIS_URL}")
    print(f"Actions directory: {ACTIONS_DIR}")
    
    # Setup Redis logging
    setup_logging(REDIS_URL)
    
    # Discover and load actions
    modules = discover_actions(ACTIONS_DIR)
    print(f"Loaded {len(ACTION_REGISTRY)} actions from {len(modules)} modules")
    
    for action_name, action_info in ACTION_REGISTRY.items():
        print(f"  - {action_name} [{action_info['group']}]")


def init_worker_process(**kwargs):
    """Called when each worker process starts (after fork)."""
    global _logging_setup
    
    # Get action group filter from environment
    action_groups_env = os.environ.get('ACTION_GROUPS', '')
    action_groups = [g.strip() for g in action_groups_env.split(',') if g.strip()]
    
    # Re-discover actions in each worker process
    discover_actions(ACTIONS_DIR)
    
    # Filter actions by group if specified
    if action_groups:
        from tinpot.decorators import ACTION_REGISTRY
        filtered_registry = {
            name: info 
            for name, info in ACTION_REGISTRY.items()
            if info['group'] in action_groups
        }
        ACTION_REGISTRY.clear()
        ACTION_REGISTRY.update(filtered_registry)
        
        worker_name = os.environ.get('WORKER_NAME', 'worker')
        print(f"[{worker_name}] Filtered to groups: {action_groups}")
        print(f"[{worker_name}] Loaded {len(ACTION_REGISTRY)} actions: {list(ACTION_REGISTRY.keys())}")
    
    # Setup logging in each worker process
    setup_logging(REDIS_URL)
    _logging_setup = True


# Connect signals
from celery.signals import worker_ready, worker_process_init
worker_ready.connect(init_worker)
worker_process_init.connect(init_worker_process)


if __name__ == '__main__':
    # Run worker
    celery_app.worker_main([
        'worker',
        '--loglevel=info',
        '--concurrency=4',
    ])
