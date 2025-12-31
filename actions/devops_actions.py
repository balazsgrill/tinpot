"""
Example DevOps actions that run on the devops queue.
These actions have access to Docker, Git, and SSH tools.
"""
import time
from tinpot import action, action_print, run_command


@action(
    group="DevOps", 
    queue="devops",
    description="Deploy application using Docker"
)
def docker_deploy(image: str = "nginx:latest", port: int = 8080):
    """Deploy a Docker container."""
    action_print(f"🐳 Deploying Docker image: {image}")
    
    try:
        import docker
        client = docker.from_env()
        action_print(f"✓ Docker client connected")
        
        # Simulate pulling image
        action_print(f"Pulling image {image}...")
        time.sleep(1)
        
        # Simulate running container
        action_print(f"Starting container on port {port}...")
        time.sleep(1)
        
        action_print(f"✓ Container deployed successfully")
        return {
            "image": image,
            "port": port,
            "status": "running"
        }
    except ImportError:
        action_print("⚠️ Docker library not available (install with: pip install docker)")
        return {"status": "simulated", "message": "Docker not available"}


@action(
    group="DevOps",
    queue="devops", 
    description="Clone and update Git repository"
)
def git_update(repo_url: str, branch: str = "main"):
    """Update a Git repository."""
    action_print(f"📦 Updating Git repository: {repo_url}")
    
    try:
        action_print(f"Branch: {branch}")
        
        # Use run_command to execute git
        run_command("git --version")
        action_print("Fetching latest changes...")
        time.sleep(1)
        
        action_print("Pulling updates...")
        time.sleep(1)
        
        action_print(f"✓ Repository updated to latest {branch}")
        return {
            "repo": repo_url,
            "branch": branch,
            "status": "updated"
        }
    except Exception as e:
        action_print(f"⚠️ Git command failed: {e}")
        return {"status": "simulated", "message": str(e)}


@action(
    group="Database",
    queue="devops",
    description="Run database migration"
)
def db_migrate(environment: str = "staging", dry_run: bool = False):
    """Run database migrations."""
    action_print(f"🗄️ Running database migration on {environment}")
    
    if dry_run:
        action_print("🔍 DRY RUN - No changes will be made")
    
    migrations = [
        "001_create_users_table",
        "002_add_email_index",
        "003_create_sessions_table"
    ]
    
    for migration in migrations:
        action_print(f"Applying {migration}...")
        time.sleep(0.5)
        if not dry_run:
            action_print(f"  ✓ Applied")
        else:
            action_print(f"  ✓ Would apply")
    
    action_print(f"✓ Migration complete ({len(migrations)} migrations)")
    return {
        "environment": environment,
        "migrations_applied": len(migrations),
        "dry_run": dry_run
    }


@action(
    group="DevOps",
    queue="devops",
    description="Execute remote SSH command"
)
def ssh_execute(host: str, command: str = "uptime"):
    """Execute command on remote host via SSH."""
    action_print(f"🔐 Executing SSH command on {host}")
    
    try:
        import paramiko
        action_print(f"Command: {command}")
        
        # Simulate SSH connection
        action_print(f"Connecting to {host}...")
        time.sleep(1)
        
        action_print(f"Executing command...")
        time.sleep(1)
        
        # Simulate output
        output = f"Command executed successfully on {host}"
        action_print(f"Output: {output}")
        
        action_print("✓ SSH command completed")
        return {
            "host": host,
            "command": command,
            "output": output,
            "status": "success"
        }
    except ImportError:
        action_print("⚠️ Paramiko not available (install with: pip install paramiko)")
        return {"status": "simulated", "message": "SSH not available"}
