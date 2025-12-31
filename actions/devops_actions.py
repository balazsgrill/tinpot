"""
Example DevOps actions that run on the devops queue.
These actions have access to Docker, Git, and SSH tools.
"""
import time
from tinpot import action


@action(
    group="DevOps", 
    queue="devops",
    description="Deploy application using Docker"
)
def docker_deploy(image: str = "nginx:latest", port: int = 8080):
    """Deploy a Docker container."""
    print(f"🐳 Deploying Docker image: {image}")
    
    try:
        import docker
        client = docker.from_env()
        print(f"✓ Docker client connected")
        
        # Simulate pulling image
        print(f"Pulling image {image}...")
        time.sleep(1)
        
        # Simulate running container
        print(f"Starting container on port {port}...")
        time.sleep(1)
        
        print(f"✓ Container deployed successfully")
        return {
            "image": image,
            "port": port,
            "status": "running"
        }
    except ImportError:
        print("⚠️ Docker library not available (install with: pip install docker)")
        return {"status": "simulated", "message": "Docker not available"}


@action(
    group="DevOps",
    queue="devops", 
    description="Clone and update Git repository"
)
def git_update(repo_url: str, branch: str = "main"):
    """Update a Git repository."""
    print(f"📦 Updating Git repository: {repo_url}")
    
    try:
        import git
        print(f"Branch: {branch}")
        
        # Simulate git operations
        print("Fetching latest changes...")
        time.sleep(1)
        
        print("Pulling updates...")
        time.sleep(1)
        
        print(f"✓ Repository updated to latest {branch}")
        return {
            "repo": repo_url,
            "branch": branch,
            "status": "updated"
        }
    except ImportError:
        print("⚠️ GitPython not available (install with: pip install gitpython)")
        return {"status": "simulated", "message": "Git not available"}


@action(
    group="Database",
    queue="devops",
    description="Run database migration"
)
def db_migrate(environment: str = "staging", dry_run: bool = False):
    """Run database migrations."""
    print(f"🗄️ Running database migration on {environment}")
    
    if dry_run:
        print("🔍 DRY RUN - No changes will be made")
    
    migrations = [
        "001_create_users_table",
        "002_add_email_index",
        "003_create_sessions_table"
    ]
    
    for migration in migrations:
        print(f"Applying {migration}...")
        time.sleep(0.5)
        if not dry_run:
            print(f"  ✓ Applied")
        else:
            print(f"  ✓ Would apply")
    
    print(f"✓ Migration complete ({len(migrations)} migrations)")
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
    print(f"🔐 Executing SSH command on {host}")
    
    try:
        import paramiko
        print(f"Command: {command}")
        
        # Simulate SSH connection
        print(f"Connecting to {host}...")
        time.sleep(1)
        
        print(f"Executing command...")
        time.sleep(1)
        
        # Simulate output
        output = f"Command executed successfully on {host}"
        print(f"Output: {output}")
        
        print("✓ SSH command completed")
        return {
            "host": host,
            "command": command,
            "output": output,
            "status": "success"
        }
    except ImportError:
        print("⚠️ Paramiko not available (install with: pip install paramiko)")
        return {"status": "simulated", "message": "SSH not available"}
