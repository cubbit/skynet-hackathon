import sqlite3
from datetime import datetime
from pathlib import Path
from typing import List, Optional
import json

from models import Backup, BackupStatus, Schedule, BackupFile


class BackupStore:
    def __init__(self, db_path: Path):
        db_path.parent.mkdir(parents=True, exist_ok=True)
        self.db_path = db_path
        self.init_db()

    def init_db(self):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS backups (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    label TEXT NOT NULL,
                    source_path TEXT NOT NULL,
                    target TEXT NOT NULL,
                    backup_path TEXT NOT NULL,
                    size_bytes INTEGER,
                    file_count INTEGER,
                    status TEXT NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    completed_at TIMESTAMP,
                    error_message TEXT
                )
            """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS schedules (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    label TEXT NOT NULL,
                    source_path TEXT NOT NULL,
                    target TEXT NOT NULL,
                    cron_expression TEXT NOT NULL,
                    next_run TIMESTAMP,
                    last_run TIMESTAMP,
                    last_status TEXT,
                    is_active BOOLEAN DEFAULT 1
                )
            """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS backup_files (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    backup_id INTEGER NOT NULL,
                    file_path TEXT NOT NULL,
                    file_size INTEGER,
                    file_hash TEXT,
                    FOREIGN KEY (backup_id) REFERENCES backups(id)
                )
            """
            )

    def create_backup(self, backup: Backup) -> int:
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute(
                """
                INSERT INTO backups (label, source_path, target, backup_path, status, created_at)
                VALUES (?, ?, ?, ?, ?, ?)
            """,
                (
                    backup.label,
                    backup.source_path,
                    backup.target,
                    backup.backup_path,
                    backup.status.value,
                    backup.created_at.isoformat(),
                ),
            )
            conn.commit()
            return cursor.lastrowid

    def get_backup(self, backup_id: int) -> Optional[Backup]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            cursor = conn.execute(
                "SELECT * FROM backups WHERE id = ?", (backup_id,)
            )
            row = cursor.fetchone()
            if not row:
                return None
            return Backup(
                id=row["id"],
                label=row["label"],
                source_path=row["source_path"],
                target=row["target"],
                backup_path=row["backup_path"],
                size_bytes=row["size_bytes"],
                file_count=row["file_count"],
                status=BackupStatus(row["status"]),
                created_at=datetime.fromisoformat(row["created_at"]),
                completed_at=(
                    datetime.fromisoformat(row["completed_at"])
                    if row["completed_at"]
                    else None
                ),
                error_message=row["error_message"],
            )

    def list_backups(
        self, target: Optional[str] = None, limit: int = 10
    ) -> List[Backup]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            if target:
                cursor = conn.execute(
                    """
                    SELECT * FROM backups
                    WHERE target LIKE ?
                    ORDER BY created_at DESC
                    LIMIT ?
                """,
                    (f"{target}%", limit),
                )
            else:
                cursor = conn.execute(
                    """
                    SELECT * FROM backups
                    ORDER BY created_at DESC
                    LIMIT ?
                """,
                    (limit,),
                )
            return [
                Backup(
                    id=row["id"],
                    label=row["label"],
                    source_path=row["source_path"],
                    target=row["target"],
                    backup_path=row["backup_path"],
                    size_bytes=row["size_bytes"],
                    file_count=row["file_count"],
                    status=BackupStatus(row["status"]),
                    created_at=datetime.fromisoformat(row["created_at"]),
                    completed_at=(
                        datetime.fromisoformat(row["completed_at"])
                        if row["completed_at"]
                        else None
                    ),
                    error_message=row["error_message"],
                )
                for row in cursor.fetchall()
            ]

    def update_backup_status(
        self,
        backup_id: int,
        status: BackupStatus,
        size_bytes: Optional[int] = None,
        file_count: Optional[int] = None,
        error_message: Optional[str] = None,
    ):
        with sqlite3.connect(self.db_path) as conn:
            updates = ["status = ?"]
            values = [status.value]

            if size_bytes is not None:
                updates.append("size_bytes = ?")
                values.append(size_bytes)

            if file_count is not None:
                updates.append("file_count = ?")
                values.append(file_count)

            if error_message is not None:
                updates.append("error_message = ?")
                values.append(error_message)
            elif status == BackupStatus.COMPLETED:
                updates.append("completed_at = ?")
                values.append(datetime.now().isoformat())

            values.append(backup_id)
            conn.execute(f"UPDATE backups SET {', '.join(updates)} WHERE id = ?", values)
            conn.commit()

    def delete_backup(self, backup_id: int):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM backups WHERE id = ?", (backup_id,))
            conn.commit()

    def create_schedule(self, schedule: Schedule) -> int:
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute(
                """
                INSERT INTO schedules (label, source_path, target, cron_expression, is_active)
                VALUES (?, ?, ?, ?, ?)
            """,
                (
                    schedule.label,
                    schedule.source_path,
                    schedule.target,
                    schedule.cron_expression,
                    1 if schedule.is_active else 0,
                ),
            )
            conn.commit()
            return cursor.lastrowid

    def get_schedule(self, schedule_id: int) -> Optional[Schedule]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            cursor = conn.execute(
                "SELECT * FROM schedules WHERE id = ?", (schedule_id,)
            )
            row = cursor.fetchone()
            if not row:
                return None
            return Schedule(
                id=row["id"],
                label=row["label"],
                source_path=row["source_path"],
                target=row["target"],
                cron_expression=row["cron_expression"],
                next_run=(
                    datetime.fromisoformat(row["next_run"]) if row["next_run"] else None
                ),
                last_run=(
                    datetime.fromisoformat(row["last_run"]) if row["last_run"] else None
                ),
                last_status=row["last_status"],
                is_active=bool(row["is_active"]),
            )

    def list_schedules(self, active_only: bool = True) -> List[Schedule]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            if active_only:
                cursor = conn.execute(
                    "SELECT * FROM schedules WHERE is_active = 1 ORDER BY id"
                )
            else:
                cursor = conn.execute("SELECT * FROM schedules ORDER BY id")
            return [
                Schedule(
                    id=row["id"],
                    label=row["label"],
                    source_path=row["source_path"],
                    target=row["target"],
                    cron_expression=row["cron_expression"],
                    next_run=(
                        datetime.fromisoformat(row["next_run"])
                        if row["next_run"]
                        else None
                    ),
                    last_run=(
                        datetime.fromisoformat(row["last_run"])
                        if row["last_run"]
                        else None
                    ),
                    last_status=row["last_status"],
                    is_active=bool(row["is_active"]),
                )
                for row in cursor.fetchall()
            ]

    def update_schedule(self, schedule_id: int, **kwargs):
        with sqlite3.connect(self.db_path) as conn:
            updates = []
            values = []
            for key, value in kwargs.items():
                updates.append(f"{key} = ?")
                values.append(value)
            values.append(schedule_id)
            conn.execute(
                f"UPDATE schedules SET {', '.join(updates)} WHERE id = ?", values
            )
            conn.commit()

    def delete_schedule(self, schedule_id: int):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM schedules WHERE id = ?", (schedule_id,))
            conn.commit()

    def list_backup_files(self, backup_id: int) -> List[BackupFile]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            cursor = conn.execute(
                "SELECT * FROM backup_files WHERE backup_id = ?", (backup_id,)
            )
            return [
                BackupFile(
                    path=row["file_path"],
                    size=row["file_size"],
                    modification_time=datetime.now(),
                )
                for row in cursor.fetchall()
            ]

    def add_backup_file(
        self, backup_id: int, file_path: str, file_size: int, file_hash: Optional[str] = None
    ):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                """
                INSERT INTO backup_files (backup_id, file_path, file_size, file_hash)
                VALUES (?, ?, ?, ?)
            """,
                (backup_id, file_path, file_size, file_hash),
            )
            conn.commit()
