"""
Redis-based logging handler for streaming logs from workers to API.
"""
import logging
import sys
import redis
from typing import Optional
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
            self.redis_client.publish(channel, json.dumps(log_entry))
            
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
            try:
                self.redis_client.publish(channel, json.dumps(log_entry))
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
