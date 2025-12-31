"""
Redis-based logging handler for streaming logs from workers to API.
"""
import logging
import sys
import os
import redis
import subprocess
from typing import Optional, Union, List
from contextlib import contextmanager
import json
from datetime import datetime
import threading


# Thread-local storage for execution context
_context = threading.local()


def set_execution_context(execution_id: str, call_depth: int = 0):
    """Set the current execution context (called by worker before running action)."""
    _context.execution_id = execution_id
    _context.call_depth = call_depth


def get_execution_context() -> tuple[Optional[str], int]:
    """Get current execution ID and call depth."""
    execution_id = getattr(_context, 'execution_id', None)
    call_depth = getattr(_context, 'call_depth', 0)
    return execution_id, call_depth


class RedisLogHandler(logging.Handler):
    """
    Custom logging handler that publishes log records to Redis pub/sub.
    The API server subscribes to these channels to stream logs via SSE.
    """
    
    def __init__(self, redis_url: str = "redis://localhost:6379"):
        super().__init__()
        self.redis_client = redis.from_url(redis_url)
        
    def emit(self, record: logging.LogRecord):
        """Publish log record to Redis channel."""
        try:
            execution_id, call_depth = get_execution_context()
            
            if not execution_id:
                # No execution context, skip Redis publishing
                return
            
            # Format the log message
            log_entry = {
                "timestamp": datetime.utcnow().isoformat(),
                "level": record.levelname,
                "message": self.format(record),
                "call_depth": call_depth,
            }
            
            # Publish to Redis channel specific to this execution
            channel = f"tinpot:logs:{execution_id}"
            log_key = f"tinpot:logs:{execution_id}:history"
            self.redis_client.publish(channel, json.dumps(log_entry))
            # Also store in a list for late subscribers (with TTL)
            self.redis_client.rpush(log_key, json.dumps(log_entry))
            self.redis_client.expire(log_key, 3600)  # Keep for 1 hour
            
        except Exception as e:
            # Don't let logging errors break the application
            print(f"Error publishing log to Redis: {e}", file=sys.stderr)


class StdoutRedirector:
    """
    Redirect stdout/stderr to both console and Redis pub/sub.
    """
    
    def __init__(self, redis_url: str, original_stream):
        self.redis_client = redis.from_url(redis_url)
        self.original_stream = original_stream
        
    def write(self, message: str):
        """Write to both original stream and Redis."""
        # Write to original stream
        self.original_stream.write(message)
        self.original_stream.flush()
        
        # Skip empty messages
        if not message or message.isspace():
            return
        
        # Publish to Redis
        execution_id, call_depth = get_execution_context()
        if execution_id:
            log_entry = {
                "timestamp": datetime.utcnow().isoformat(),
                "level": "INFO",
                "message": message.rstrip(),
                "call_depth": call_depth,
            }
            channel = f"tinpot:logs:{execution_id}"
            log_key = f"tinpot:logs:{execution_id}:history"
            try:
                # Publish to channel (for live subscribers)
                self.redis_client.publish(channel, json.dumps(log_entry))
                # Also store in a list for late subscribers (with TTL)
                self.redis_client.rpush(log_key, json.dumps(log_entry))
                self.redis_client.expire(log_key, 3600)  # Keep for 1 hour
            except Exception as e:
                print(f"Error publishing stdout to Redis: {e}", file=sys.stderr)
    
    def flush(self):
        """Flush the original stream."""
        self.original_stream.flush()


def setup_logging(redis_url: str = "redis://localhost:6379"):
    """
    Configure logging to stream to Redis.
    Should be called once at worker startup.
    """
    # Add Redis handler to root logger
    redis_handler = RedisLogHandler(redis_url)
    redis_handler.setLevel(logging.INFO)
    
    formatter = logging.Formatter('%(message)s')
    redis_handler.setFormatter(formatter)
    
    root_logger = logging.getLogger()
    root_logger.addHandler(redis_handler)
    root_logger.setLevel(logging.INFO)
    
    # Redirect stdout and stderr
    sys.stdout = StdoutRedirector(redis_url, sys.stdout)
    sys.stderr = StdoutRedirector(redis_url, sys.stderr)


def action_print(*args, **kwargs):
    """
    Print function for use in actions - ensures output is captured and streamed.
    Use this instead of regular print() in action functions.
    
    Example:
        from tinpot import action_print
        
        @action(group="DevOps")
        def deploy_app():
            action_print("Starting deployment...")
            action_print(f"Status: {status}")
    """
    # Use regular print which is already redirected to Redis by setup_logging()
    print(*args, **kwargs)
    # Force flush to ensure immediate delivery
    sys.stdout.flush()


def run_command(
    cmd: Union[str, List[str]],
    shell: bool = True,
    check: bool = True,
    capture_output: bool = True,
    **kwargs
) -> subprocess.CompletedProcess:
    """
    Run a shell command and capture its output to the action log stream.
    
    Args:
        cmd: Command to run (string or list of arguments)
        shell: Whether to run through shell (default: True)
        check: Raise exception on non-zero exit code (default: True)
        capture_output: Capture stdout/stderr (default: True)
        **kwargs: Additional arguments passed to subprocess.run()
    
    Returns:
        CompletedProcess instance with stdout/stderr as strings
        
    Example:
        from tinpot import action_print, run_command
        
        @action(group="DevOps")
        def git_status():
            action_print("Checking git status...")
            result = run_command("git status")
            action_print(f"Exit code: {result.returncode}")
    """
    execution_id, call_depth = get_execution_context()
    
    # Log the command being executed
    cmd_str = cmd if isinstance(cmd, str) else ' '.join(cmd)
    action_print(f"$ {cmd_str}")
    
    try:
        # Run the command
        result = subprocess.run(
            cmd,
            shell=shell,
            check=check,
            capture_output=capture_output,
            text=True,
            **kwargs
        )
        
        # Log stdout if present
        if result.stdout:
            for line in result.stdout.rstrip().split('\n'):
                if line:
                    action_print(f"  {line}")
        
        # Log stderr if present (even on success, some commands use stderr for info)
        if result.stderr:
            for line in result.stderr.rstrip().split('\n'):
                if line:
                    action_print(f"  [stderr] {line}")
        
        return result
        
    except subprocess.CalledProcessError as e:
        # Log error output
        action_print(f"✗ Command failed with exit code {e.returncode}")
        if e.stdout:
            action_print("Output:")
            for line in e.stdout.rstrip().split('\n'):
                if line:
                    action_print(f"  {line}")
        if e.stderr:
            action_print("Error output:")
            for line in e.stderr.rstrip().split('\n'):
                if line:
                    action_print(f"  {line}")
        raise


@contextmanager
def capture_subprocess_output():
    """
    Context manager to ensure subprocess output is properly captured.
    Use when running commands with subprocess.Popen or similar.
    
    Example:
        from tinpot import action_print, capture_subprocess_output
        
        @action(group="DevOps")
        def long_running_task():
            action_print("Starting long task...")
            with capture_subprocess_output():
                proc = subprocess.Popen(
                    ["./long_script.sh"],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True
                )
                for line in proc.stdout:
                    action_print(line.rstrip())
                proc.wait()
    """
    # Nothing special needed - our redirected stdout will handle it
    # This is just a convenience for clarity
    try:
        yield
    finally:
        sys.stdout.flush()
        sys.stderr.flush()
