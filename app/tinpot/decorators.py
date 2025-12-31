"""
@action decorator for Tinpot.
Wraps Python functions as Celery tasks and extracts metadata for UI generation.
"""
import functools
import inspect
import uuid
from typing import Any, Callable, Dict, Optional, get_type_hints
from celery import Task


# Global registry for discovered actions
ACTION_REGISTRY: Dict[str, Dict[str, Any]] = {}


def action(
    name: Optional[str] = None,
    group: Optional[str] = "General",
    description: Optional[str] = None,
    queue: str = "celery",  # Default Celery queue name
):
    """
    Decorator to mark a function as a Tinpot action.
    
    Args:
        name: Display name (defaults to function name)
        group: Category/group for UI organization
        description: Help text for the action
        queue: Celery queue name for routing to specific workers (default: "default")
        
    Example:
        @action(group="DevOps", queue="devops", description="Deploy the application")
        def deploy_app(environment: str = "staging"):
            print(f"Deploying to {environment}")
    """
    def decorator(func: Callable) -> Task:
        # Extract metadata
        action_name = name or func.__name__
        action_desc = description or func.__doc__ or ""
        
        # Get function signature for form generation
        sig = inspect.signature(func)
        type_hints = get_type_hints(func)
        
        parameters = {}
        for param_name, param in sig.parameters.items():
            param_type = type_hints.get(param_name, str)
            param_default = param.default if param.default != inspect.Parameter.empty else None
            
            parameters[param_name] = {
                "type": param_type.__name__ if hasattr(param_type, '__name__') else str(param_type),
                "default": param_default,
                "required": param.default == inspect.Parameter.empty
            }
        
        # Store metadata in registry
        ACTION_REGISTRY[action_name] = {
            "name": action_name,
            "group": group,
            "description": action_desc.strip(),
            "function": func,
            "parameters": parameters,
            "module": func.__module__,
            "queue": queue,
        }
        
        # Return the original function (Celery task wrapping happens later in worker.py)
        return func
    
    return decorator


def get_action(name: str) -> Optional[Callable]:
    """Get an action function by name."""
    action_info = ACTION_REGISTRY.get(name)
    return action_info["function"] if action_info else None


def list_actions() -> Dict[str, Dict[str, Any]]:
    """Return all registered actions."""
    return {
        name: {k: v for k, v in info.items() if k != "function"}
        for name, info in ACTION_REGISTRY.items()
    }
