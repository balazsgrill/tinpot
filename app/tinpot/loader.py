"""
Dynamic action loader - discovers and imports action modules.
"""
import importlib
import os
import sys
from pathlib import Path
from typing import List


def discover_actions(actions_dir: str = "/opt/tinpot/actions") -> List[str]:
    """
    Discover all Python modules in the actions directory.
    
    Returns:
        List of module names that were imported
    """
    actions_path = Path(actions_dir)
    
    if not actions_path.exists():
        print(f"Warning: Actions directory not found: {actions_dir}")
        return []
    
    # Add actions directory to Python path
    if str(actions_path.parent) not in sys.path:
        sys.path.insert(0, str(actions_path.parent))
    
    imported_modules = []
    
    # Find all Python files in actions directory
    for file_path in actions_path.glob("*.py"):
        if file_path.name.startswith("_"):
            continue  # Skip __init__.py and private modules
        
        module_name = f"actions.{file_path.stem}"
        
        try:
            print(f"Loading action module: {module_name}")
            importlib.import_module(module_name)
            imported_modules.append(module_name)
        except Exception as e:
            print(f"Error loading {module_name}: {e}")
    
    return imported_modules


def reload_actions(actions_dir: str = "/opt/tinpot/actions"):
    """
    Reload all action modules (useful for development).
    """
    from tinpot.decorators import ACTION_REGISTRY
    
    # Clear registry
    ACTION_REGISTRY.clear()
    
    # Re-discover
    discover_actions(actions_dir)
