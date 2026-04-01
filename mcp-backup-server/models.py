from pydantic import BaseModel
from typing import Optional
from datetime import datetime
from enum import Enum


class BackupStatus(Enum):
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"


class Backup(BaseModel):
    id: Optional[int] = None
    label: str
    source_path: str
    target: str
    backup_path: str
    size_bytes: Optional[int] = None
    file_count: Optional[int] = None
    status: BackupStatus = BackupStatus.PENDING
    created_at: datetime
    completed_at: Optional[datetime] = None
    error_message: Optional[str] = None


class Schedule(BaseModel):
    id: Optional[int] = None
    label: str
    source_path: str
    target: str
    cron_expression: str
    next_run: Optional[datetime] = None
    last_run: Optional[datetime] = None
    last_status: Optional[str] = None
    is_active: bool = True


class BackupFile(BaseModel):
    path: str
    size: int
    modification_time: datetime


class Target(BaseModel):
    name: str
    type: str
    path: str
