# Action Output and Command Execution Guide

## Overview

Tinpot provides specialized functions for action output and subprocess command execution that ensure all output is properly captured and streamed in real-time to the web UI.

## Why Use These Functions?

Regular `print()` and `subprocess` calls work, but using the Tinpot-specific functions provides:

- **Real-time streaming**: Output appears immediately in the web UI
- **Proper formatting**: Command output is nicely formatted with indentation
- **Error handling**: Failed commands are automatically logged with details
- **Context awareness**: Output is tagged with the correct execution ID

## Core Functions

### `action_print(*args, **kwargs)`

Use this instead of regular `print()` in your actions.

**Example:**

```python
from tinpot import action, action_print

@action(group="DevOps")
def deploy_app(environment: str = "staging"):
    action_print(f"🚀 Deploying to {environment}...")
    action_print("Building application...")
    action_print("✓ Deployment complete!")
```

### `run_command(cmd, shell=True, check=True, capture_output=True, **kwargs)`

Execute shell commands with automatic output capture and streaming.

**Parameters:**
- `cmd` (str | List[str]): Command to run
- `shell` (bool): Run through shell (default: True)
- `check` (bool): Raise exception on non-zero exit (default: True)
- `capture_output` (bool): Capture stdout/stderr (default: True)
- `**kwargs`: Additional arguments passed to `subprocess.run()`

**Returns:** `subprocess.CompletedProcess` with stdout/stderr as strings

**Example:**

```python
from tinpot import action, action_print, run_command

@action(group="DevOps")
def check_disk_space():
    action_print("Checking disk space...")
    result = run_command("df -h")
    action_print(f"Command exited with code: {result.returncode}")
```

**Output formatting:**
```
Checking disk space...
$ df -h
  Filesystem      Size  Used Avail Use% Mounted on
  /dev/sda1       100G   45G   50G  48% /
  /dev/sda2       500G  200G  280G  42% /data
Command exited with code: 0
```

### `capture_subprocess_output()` (Context Manager)

Use when running long commands with `subprocess.Popen` for line-by-line output.

**Example:**

```python
from tinpot import action, action_print, capture_subprocess_output
import subprocess

@action(group="Build")
def long_build():
    action_print("Starting build...")
    
    with capture_subprocess_output():
        proc = subprocess.Popen(
            ["./build.sh"],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True
        )
        
        for line in proc.stdout:
            action_print(line.rstrip())
        
        proc.wait()
    
    action_print("Build complete!")
```

## Complete Examples

### Basic Action with Output

```python
from tinpot import action, action_print

@action(group="Maintenance")
def clean_cache(days: int = 7):
    """Clean cache files older than N days."""
    action_print(f"Starting cache cleanup (files older than {days} days)...")
    
    files = ["/tmp/cache1.tmp", "/tmp/cache2.tmp"]
    
    for file_path in files:
        action_print(f"Deleted {file_path}")
    
    action_print(f"✓ Cleanup complete! Removed {len(files)} files.")
    return {"files_deleted": len(files)}
```

### Running Git Commands

```python
from tinpot import action, action_print, run_command

@action(group="DevOps")
def git_status(repo_path: str = "."):
    """Check Git repository status."""
    action_print(f"Checking status of {repo_path}...")
    
    # Show current branch
    result = run_command(f"git -C {repo_path} branch --show-current")
    
    # Show status
    run_command(f"git -C {repo_path} status --short")
    
    action_print("✓ Git status check complete")
```

**Output:**
```
Checking status of /app...
$ git -C /app branch --show-current
  main
$ git -C /app status --short
  M  app/main.py
  ?? actions/new_action.py
✓ Git status check complete
```

### Docker Container Management

```python
from tinpot import action, action_print, run_command

@action(group="DevOps", queue="devops")
def docker_ps(filter: str = ""):
    """List running Docker containers."""
    action_print("Listing Docker containers...")
    
    cmd = "docker ps"
    if filter:
        cmd += f" --filter {filter}"
    
    try:
        run_command(cmd)
        action_print("✓ Container list retrieved")
    except subprocess.CalledProcessError as e:
        action_print(f"✗ Failed to list containers: {e}")
        raise
```

### Error Handling

```python
from tinpot import action, action_print, run_command
import subprocess

@action(group="DevOps")
def deploy_with_validation(service: str):
    """Deploy service with pre-deployment validation."""
    action_print(f"Validating {service} before deployment...")
    
    try:
        # This will automatically log errors if it fails
        run_command(f"./validate.sh {service}")
        action_print("✓ Validation passed")
        
        action_print("Deploying service...")
        run_command(f"./deploy.sh {service}")
        action_print("✓ Deployment successful")
        
    except subprocess.CalledProcessError as e:
        action_print(f"✗ Deployment failed at validation stage")
        action_print("Rolling back changes...")
        run_command(f"./rollback.sh {service}", check=False)
        raise
```

### Long-Running Command with Progress

```python
from tinpot import action, action_print, capture_subprocess_output
import subprocess

@action(group="Build")
def compile_project():
    """Compile large project with progress output."""
    action_print("Starting compilation...")
    
    with capture_subprocess_output():
        proc = subprocess.Popen(
            ["make", "all", "-j4"],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            cwd="/path/to/project"
        )
        
        for line in proc.stdout:
            # Each line is immediately streamed to UI
            action_print(line.rstrip())
        
        return_code = proc.wait()
    
    if return_code == 0:
        action_print("✓ Compilation successful")
    else:
        action_print(f"✗ Compilation failed with code {return_code}")
        raise subprocess.CalledProcessError(return_code, "make")
```

### System Information Gathering

```python
from tinpot import action, action_print, run_command

@action(group="Monitoring")
def system_info():
    """Gather system information."""
    action_print("Collecting system information...")
    
    action_print("\n=== CPU Info ===")
    run_command("lscpu | grep 'Model name'")
    
    action_print("\n=== Memory Usage ===")
    run_command("free -h")
    
    action_print("\n=== Disk Usage ===")
    run_command("df -h /")
    
    action_print("\n=== Load Average ===")
    run_command("uptime")
    
    action_print("\n✓ System info collected")
```

## Best Practices

### 1. Always Use `action_print()` for User-Facing Output

```python
# ✓ Good
action_print("Processing file...")

# ✗ Avoid (won't stream properly)
print("Processing file...")
```

### 2. Use `run_command()` for Shell Commands

```python
# ✓ Good - automatic output capture and streaming
run_command("git pull origin main")

# ✗ Avoid - output won't be streamed
os.system("git pull origin main")

# ✗ Avoid - requires manual output handling
subprocess.run(["git", "pull", "origin", "main"])
```

### 3. Handle Errors Gracefully

```python
# ✓ Good - catches and logs errors properly
try:
    run_command("risky-command")
except subprocess.CalledProcessError:
    action_print("Command failed, continuing with fallback...")
    # Handle error
```

### 4. Provide Context in Output

```python
# ✓ Good - clear progress indicators
action_print("Step 1/3: Backing up database...")
action_print("Step 2/3: Applying migrations...")
action_print("Step 3/3: Restarting service...")

# ✗ Avoid - unclear what's happening
action_print("Working...")
action_print("Working...")
action_print("Done")
```

### 5. Use Emoji and Formatting for Clarity

```python
action_print("🚀 Starting deployment...")
action_print("  → Checking prerequisites")
action_print("  → Building application")
action_print("  → Running tests")
action_print("✓ Deployment complete!")
```

## Troubleshooting

### Output Not Appearing in UI

**Problem:** Using regular `print()` instead of `action_print()`

**Solution:** Replace all `print()` calls with `action_print()`

### Command Output Not Captured

**Problem:** Using `subprocess` without `run_command()` helper

**Solution:**
```python
# Before
subprocess.run(["ls", "-la"])

# After
run_command("ls -la")
```

### Long Command Hangs

**Problem:** Command produces too much output and fills buffer

**Solution:** Use `capture_subprocess_output()` with line-by-line processing

```python
with capture_subprocess_output():
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, text=True)
    for line in proc.stdout:
        action_print(line.rstrip())
    proc.wait()
```

## API Reference

### `action_print(*args, **kwargs)`

Prints output to action log stream. Signature matches built-in `print()`.

- Ensures output is immediately flushed
- Tagged with execution context
- Streamed via SSE to web UI

### `run_command(cmd, shell=True, check=True, capture_output=True, **kwargs)`

Executes shell command with automatic output capture.

**Arguments:**
- `cmd`: Command string or list of arguments
- `shell`: Whether to use shell (default: True)
- `check`: Raise exception on error (default: True)
- `capture_output`: Capture stdout/stderr (default: True)

**Returns:** `subprocess.CompletedProcess`

**Raises:** `subprocess.CalledProcessError` if `check=True` and command fails

### `capture_subprocess_output()`

Context manager for subprocess output streaming.

Use when you need line-by-line control over subprocess output.

## Summary

| Use Case | Function | Example |
|----------|----------|---------|
| Print messages | `action_print()` | `action_print("Processing...")` |
| Run shell commands | `run_command()` | `run_command("git status")` |
| Long-running commands | `capture_subprocess_output()` | `with capture_subprocess_output(): ...` |
| Multiple commands | `run_command()` multiple times | Sequential command execution |
| Error handling | try/except with `run_command()` | Catch `CalledProcessError` |

All these functions ensure your action output is properly captured and streamed in real-time to the Tinpot web UI!
