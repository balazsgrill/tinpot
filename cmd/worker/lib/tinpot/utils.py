import subprocess
import os
from .decorators import action_print

def run_command(command: str, cwd: str = None) -> str:
    """
    Run a shell command and capture output while streaming it to action logs.
    
    Args:
        command: Shell command to execute
        cwd: Working directory (optional)
        
    Returns:
        Combined stdout/stderr output as string
        
    Raises:
        Exception: If command fails (non-zero exit code)
    """
    action_print(f"$ {command}")
    
    process = subprocess.Popen(
        command,
        shell=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1, # Line buffered
        cwd=cwd
    )
    
    output = []
    
    # Read output line by line
    if process.stdout:
        for line in process.stdout:
            line = line.rstrip()
            action_print(line)
            output.append(line)
            
    rc = process.wait()
    
    if rc != 0:
        raise Exception(f"Command failed with exit code {rc}")
        
    return "\n".join(output)
