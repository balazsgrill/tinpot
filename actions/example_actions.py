"""
Example actions demonstrating Tinpot features.
"""
import time
import os
from tinpot import action, action_print


@action(group="Maintenance", description="Clean up temporary files older than specified days")
def clean_cache(days: int = 7):
    """Clean cache files older than N days."""
    action_print(f"Starting cache cleanup (files older than {days} days)...")
    
    # Simulate finding and deleting files
    cache_files = [
        "/tmp/cache_file_1.tmp",
        "/tmp/cache_file_2.tmp",
        "/tmp/cache_file_3.tmp",
    ]
    
    for file_path in cache_files:
        action_print(f"Deleted {file_path}")
        time.sleep(0.5)  # Simulate work
    
    action_print(f"✓ Cache cleanup complete! Removed {len(cache_files)} files.")
    return {"files_deleted": len(cache_files)}


@action(group="DevOps", description="Deploy application to specified environment")
def deploy_app(environment: str = "staging", skip_tests: bool = False):
    """Deploy the application to an environment."""
    action_print(f"🚀 Starting deployment to {environment}...")
    
    if not skip_tests:
        action_print("Running tests...")
        time.sleep(1)
        action_print("✓ All tests passed")
    
    action_print("Building application...")
    time.sleep(1.5)
    action_print("✓ Build complete")
    
    action_print(f"Deploying to {environment}...")
    time.sleep(2)
    action_print(f"✓ Successfully deployed to {environment}!")
    
    return {
        "environment": environment,
        "version": "1.0.0",
        "timestamp": time.time()
    }


@action(group="Database", description="Backup database to storage")
def db_backup(target_path: str = "/backups"):
    """Perform a database backup."""
    action_print(f"Starting database backup to {target_path}...")
    
    action_print("Locking tables...")
    time.sleep(0.5)
    
    action_print("Dumping database...")
    time.sleep(2)
    
    backup_file = f"{target_path}/db_backup_{int(time.time())}.sql"
    action_print(f"✓ Backup saved to {backup_file}")
    
    action_print("Unlocking tables...")
    time.sleep(0.3)
    
    action_print("✓ Database backup complete!")
    return {"backup_file": backup_file, "size_mb": 145}


@action(group="DevOps", description="Full deployment with backup - demonstrates nested calls")
def full_deploy(environment: str = "staging"):
    """
    Complete deployment workflow with database backup.
    Demonstrates nested action calls.
    """
    action_print(f"🎯 Starting full deployment workflow for {environment}")
    
    # Nested call: backup database first
    action_print("\n→ Step 1: Backing up database...")
    backup_result = db_backup()
    action_print(f"  Backup file: {backup_result['backup_file']}")
    
    # Nested call: deploy application
    action_print("\n→ Step 2: Deploying application...")
    deploy_result = deploy_app(environment=environment, skip_tests=False)
    
    action_print("\n✓ Full deployment workflow complete!")
    
    return {
        "backup": backup_result,
        "deployment": deploy_result,
        "workflow": "success"
    }


@action(group="Monitoring", description="Long-running health check simulation")
def health_check(duration: int = 30):
    """Simulate a long-running health check."""
    action_print(f"Starting health check (will run for {duration} seconds)...")
    
    for i in range(duration):
        action_print(f"[{i+1}/{duration}] Checking system health...")
        time.sleep(1)
        
        if i % 5 == 4:
            action_print(f"  ✓ All systems operational at {i+1}s")
    
    action_print("✓ Health check complete - all systems healthy!")
    return {"status": "healthy", "duration": duration}
