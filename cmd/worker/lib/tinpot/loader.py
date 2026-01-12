import importlib
import os
import sys
import pkgutil

def discover_actions(directory: str):
    """
    Recursively find and import Python modules in the given directory
    to trigger the @action decorators.
    """
    directory = os.path.abspath(directory)
    if directory not in sys.path:
        sys.path.append(directory)

    # Walk directory
    for root, dirs, files in os.walk(directory):
        for file in files:
            if file.endswith(".py") and not file.startswith("__"):
                module_name = file[:-3]
                
                # Calculate relative path for package import if needed
                # But typically we just add directory to sys.path and import module
                # If actions are flat or simple packages.
                
                try:
                    importlib.import_module(module_name)
                except Exception as e:
                    print(f"WARNING: Failed to load action module '{module_name}': {e}", file=sys.stderr)
