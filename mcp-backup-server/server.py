from fastmcp import FastMCP
from datetime import datetime
import models
import store
import rclone_client
import scheduler
from config import Config

mcp = FastMCP("RcloneBackup")

# Initialize components
backup_store = store.BackupStore(Config.DATABASE_PATH)
rclone = rclone_client.RcloneClient(config_path=Config.RCLONE_CONFIG)
sched = scheduler.BackupScheduler(backup_store, rclone)

# ============== TOOLS ==============

@mcp.tool()
def backup_path(source_path: str, target: str, label: str) -> dict:
    """
    Create a full backup of source_path to target.
    
    Args:
        source_path: Local path to backup (e.g., "/home/user/data")
        target: Destination (e.g., "s3:bucket/backups" or "sftp:user@host/path")
        label: Human-readable label for the backup
    
    Returns:
        Backup result with ID, size, file count
    """
    # Generate backup path with timestamp
    timestamp = datetime.now().strftime("%Y-%m-%d_%H%M%S")
    backup_path = f"{target}/{timestamp}"
    
    # Create backup record
    backup = models.Backup(
        label=label,
        source_path=source_path,
        target=target,
        backup_path=backup_path,
        status=models.BackupStatus.RUNNING,
        created_at=datetime.now()
    )
    backup_id = backup_store.create_backup(backup)
    
    try:
        # Execute backup
        result = rclone.sync(source_path, backup_path)
        
        # Get stats
        size = rclone.size(backup_path)
        files = rclone.ls(backup_path)
        
        # Update backup record
        backup_store.update_backup_status(
            backup_id,
            models.BackupStatus.COMPLETED,
            size_bytes=size,
            file_count=len(files)
        )
        
        return {
            "backup_id": backup_id,
            "backup_path": backup_path,
            "size": result["transferred_text"],
            "files": len(files),
            "status": "completed"
        }
    except Exception as e:
        backup_store.update_backup_status(
            backup_id,
            models.BackupStatus.FAILED,
            error_message=str(e)
        )
        raise


@mcp.tool()
def list_backups(target: str = None, limit: int = 10) -> list:
    """
    List available backups.
    
    Args:
        target: Filter by target (optional)
        limit: Maximum number of backups to return
    
    Returns:
        List of backups with metadata
    """
    backups = backup_store.list_backups(target=target, limit=limit)
    return [
        {
            "id": b.id,
            "label": b.label,
            "source": b.source_path,
            "target": b.target,
            "backup_path": b.backup_path,
            "size": f"{b.size_bytes / (1024**3):.2f} GB" if b.size_bytes else "Unknown",
            "files": b.file_count or 0,
            "status": b.status.value,
            "created": b.created_at.isoformat()
        }
        for b in backups
    ]


@mcp.tool()
def schedule_backup(source_path: str, target: str, cron_expression: str, label: str) -> dict:
    """
    Schedule a recurring backup.
    
    Args:
        source_path: Local path to backup
        target: Destination
        cron_expression: Cron schedule (e.g., "0 2 * * *" for daily at 2 AM)
        label: Human-readable label
    
    Returns:
        Schedule ID and next run time
    """
    schedule = models.Schedule(
        label=label,
        source_path=source_path,
        target=target,
        cron_expression=cron_expression
    )
    schedule_id = backup_store.create_schedule(schedule)
    schedule.id = schedule_id

    # Add to scheduler
    updated_schedule = sched.add_schedule(schedule)
    
    return {
        "schedule_id": schedule_id,
        "label": label,
        "cron": cron_expression,
        "next_run": updated_schedule.next_run.isoformat() if updated_schedule.next_run else None
    }


@mcp.tool()
def preview_restore(backup_id: int) -> list:
    """
    Preview files in a backup before restoring.
    
    Args:
        backup_id: ID of the backup to preview
    
    Returns:
        List of files with paths and sizes
    """
    backup = backup_store.get_backup(backup_id)
    if not backup:
        raise ValueError(f"Backup {backup_id} not found")
    
    files = rclone.ls(backup.backup_path)
    return [
        {
            "path": f.path,
            "size": f"{f.size / (1024**2):.2f} MB" if f.size > 1024**2 else f"{f.size / 1024:.2f} KB",
            "modified": f.modification_time.isoformat()
        }
        for f in files[:100]  # Limit to first 100 files for preview
    ]


@mcp.tool()
def restore_backup(backup_id: int, restore_path: str) -> dict:
    """
    Restore a backup to specified path.
    
    Args:
        backup_id: ID of the backup to restore
        restore_path: Local path to restore to
    
    Returns:
        Restore result with file count
    """
    backup = backup_store.get_backup(backup_id)
    if not backup:
        raise ValueError(f"Backup {backup_id} not found")
    
    # Execute restore
    result = rclone.copy(backup.backup_path, restore_path)
    
    return {
        "restored_from": backup.backup_path,
        "restored_to": restore_path,
        "size": result["transferred_text"],
        "status": "completed"
    }


@mcp.tool()
def list_schedules(active_only: bool = True) -> list:
    """List all backup schedules"""
    schedules = backup_store.list_schedules(active_only=active_only)
    return [
        {
            "id": s.id,
            "label": s.label,
            "source": s.source_path,
            "target": s.target,
            "cron": s.cron_expression,
            "next_run": s.next_run.isoformat() if s.next_run else None,
            "last_run": s.last_run.isoformat() if s.last_run else None,
            "last_status": s.last_status,
            "active": s.is_active
        }
        for s in schedules
    ]


@mcp.tool()
def cancel_schedule(schedule_id: int) -> dict:
    """Cancel a scheduled backup"""
    sched.remove_schedule(schedule_id)
    backup_store.update_schedule(schedule_id, is_active=False)
    return {"schedule_id": schedule_id, "status": "cancelled"}


# ============== REMOTE CONFIG TOOLS ==============

@mcp.tool()
def configure_s3_remote(
    name: str,
    access_key_id: str,
    secret_access_key: str,
    region: str,
    provider: str = "AWS",
) -> dict:
    """
    Configure an S3 remote in rclone.

    Args:
        name: Remote name (e.g. "s3" or "my-bucket")
        access_key_id: AWS access key ID
        secret_access_key: AWS secret access key
        region: AWS region (e.g. "eu-west-1")
        provider: S3 provider (default: "AWS"; also "Minio", "Wasabi", "Cloudflare", etc.)

    Returns:
        Confirmation with remote name
    """
    rclone.create_remote(name, "s3", {
        "provider": provider,
        "access_key_id": access_key_id,
        "secret_access_key": secret_access_key,
        "region": region,
    })
    return {"remote": name, "type": "s3", "provider": provider, "status": "configured"}


@mcp.tool()
def configure_sftp_remote(
    name: str,
    host: str,
    user: str,
    port: int = 22,
    key_file: str = None,
) -> dict:
    """
    Configure an SFTP/SSH remote in rclone.

    Args:
        name: Remote name (e.g. "myserver")
        host: SSH hostname or IP
        user: SSH username
        port: SSH port (default: 22)
        key_file: Path to private key file (optional; uses SSH agent if omitted)

    Returns:
        Confirmation with remote name
    """
    params = {"host": host, "user": user, "port": str(port)}
    if key_file:
        params["key_file"] = key_file
    rclone.create_remote(name, "sftp", params)
    return {"remote": name, "type": "sftp", "host": host, "user": user, "status": "configured"}


@mcp.tool()
def show_remote(name: str) -> dict:
    """
    Show configuration for a specific rclone remote (secrets redacted by rclone).

    Args:
        name: Remote name

    Returns:
        Remote configuration dict
    """
    config = rclone.show_remote(name)
    if not config:
        raise ValueError(f"Remote '{name}' not found")
    return {"remote": name, "config": config}


@mcp.tool()
def delete_remote(name: str) -> dict:
    """
    Delete a configured rclone remote.

    Args:
        name: Remote name to delete

    Returns:
        Confirmation
    """
    rclone.delete_remote(name)
    return {"remote": name, "status": "deleted"}


# ============== RESOURCES ==============

@mcp.resource("backups://list")
def get_all_backups() -> str:
    """Get all backups across all targets"""
    backups = backup_store.list_backups(limit=50)
    if not backups:
        return "No backups found"
    
    lines = ["All Backups:", "=" * 60]
    for b in backups:
        status_icon = "✓" if b.status == models.BackupStatus.COMPLETED else "✗"
        size_str = f"{b.size_bytes / (1024**3):.2f} GB" if b.size_bytes else "Unknown"
        lines.append(f"{status_icon} [{b.id}] {b.label}")
        lines.append(f"   Source: {b.source_path}")
        lines.append(f"   Target: {b.backup_path}")
        lines.append(f"   Size: {size_str} | Files: {b.file_count or 0}")
        lines.append(f"   Created: {b.created_at.isoformat()}")
        lines.append("")
    return "\n".join(lines)


@mcp.resource("schedules://list")
def get_all_schedules() -> str:
    """Get all active schedules"""
    schedules = backup_store.list_schedules(active_only=True)
    if not schedules:
        return "No active schedules"
    
    lines = ["Active Backup Schedules:", "=" * 60]
    for s in schedules:
        lines.append(f"[{s.id}] {s.label}")
        lines.append(f"   {s.source_path} → {s.target}")
        lines.append(f"   Schedule: {s.cron_expression}")
        lines.append(f"   Next run: {s.next_run.isoformat() if s.next_run else 'N/A'}")
        lines.append(f"   Last run: {s.last_run.isoformat() if s.last_run else 'Never'}")
        lines.append("")
    return "\n".join(lines)


@mcp.resource("targets://list")
def get_all_targets() -> str:
    """Get all configured rclone remotes"""
    remotes = rclone.list_remotes()
    if not remotes:
        return "No remotes configured. Run 'rclone config' first."
    
    lines = ["Configured Rclone Remotes:", "=" * 60]
    for remote in remotes:
        lines.append(f"• {remote}")
    return "\n".join(lines)


# ============== PROMPTS ==============

@mcp.prompt()
def backup_strategy_prompt() -> str:
    """Prompt for planning backup strategy"""
    return """Analyze the backup needs and recommend a strategy:

1. Review current backups using the backups://list resource
2. Review configured targets using targets://list resource
3. Consider:
   - How often should backups run? (daily, weekly, hourly)
   - What retention policy makes sense?
   - Are critical paths being backed up?
4. Recommend specific backup_path and schedule_backup commands

Provide concrete recommendations with example commands."""


@mcp.prompt()
def restore_decision_prompt(backup_id: int) -> str:
    """Prompt for deciding which backup to restore"""
    return f"""Help decide on restoring backup #{backup_id}:

1. First, preview the backup contents using preview_restore({backup_id})
2. Verify this is the correct backup by checking:
   - Creation date
   - File count and size
   - Sample file paths
3. Confirm the restore destination path
4. Warn about potential data loss if destination exists

Ask clarifying questions before proceeding with restore_backup."""


# ============== MAIN ==============

if __name__ == "__main__":
    # Start scheduler
    sched.start()
    
    # Run MCP server
    mcp.run()
