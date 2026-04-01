import os
from pathlib import Path


class Config:
    RCLONE_CONFIG = os.environ.get("RCLONE_CONFIG", None)
    DEFAULT_TARGET = os.environ.get("DEFAULT_BACKUP_TARGET", "s3:my-bucket/backups")
    DATABASE_PATH = Path(
        os.environ.get("BACKUP_DB_PATH", "~/.mcp-backup/backups.db")
    ).expanduser()
    BACKUP_RETENTION_DAYS = int(os.environ.get("BACKUP_RETENTION_DAYS", "30"))
    LOG_LEVEL = os.environ.get("LOG_LEVEL", "INFO")
    SCHEDULER_TIMEZONE = os.environ.get("SCHEDULER_TIMEZONE", "UTC")
