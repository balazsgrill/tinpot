import inspect
import sys
from typing import Any, Callable, Dict, Optional, get_type_hints

# Global registry for discovered actions
ACTION_REGISTRY: Dict[str, Dict[str, Any]] = {}


def action(
    name: Optional[str] = None,
    group: Optional[str] = "General",
    description: Optional[str] = None,
    queue: str = "default", 
):
    """
    Decorator to mark a function as a Tinpot action.
    """
    def decorator(func: Callable):
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
        
        return func
    
    return decorator

def action_print(*args, **kwargs):
    """
    Helper to print messages that will be captured as logs.
    Go worker captures stdout, so this is just print.
    """
    print(*args, **kwargs)
    sys.stdout.flush() # Ensure it goes to pipe immediately
