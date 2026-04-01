from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.cron import CronTrigger
from datetime import datetime
from typing import Callable, Optional

from models import Schedule
from rclone_client import RcloneClient
from store import BackupStore
from config import Config


class BackupScheduler:
    def __init__(self, store: BackupStore, rclone: RcloneClient):
        self.scheduler = BackgroundScheduler(
            timezone=Config.SCHEDULER_TIMEZONE
        )
        self.store = store
        self.rclone = rclone
        self._backup_callback: Optional[Callable] = None

    def set_backup_callback(self, callback: Callable):
        self._backup_callback = callback

    def start(self):
        self.scheduler.start()
        self._load_schedules()

    def stop(self):
        self.scheduler.shutdown()

    def _load_schedules(self):
        schedules = self.store.list_schedules(active_only=True)
        for schedule in schedules:
            self.add_schedule(schedule)

    def add_schedule(self, schedule: Schedule):
        job = self.scheduler.add_job(
            self._execute_backup,
            trigger=CronTrigger.from_crontab(schedule.cron_expression),
            args=[schedule.id],
            id=f"backup_{schedule.id}",
            replace_existing=True,
        )
        next_run = job.next_run_time
        self.store.update_schedule(schedule.id, next_run=next_run.isoformat())
        schedule.next_run = next_run
        return schedule

    def remove_schedule(self, schedule_id: int):
        self.scheduler.remove_job(f"backup_{schedule_id}")

    def _execute_backup(self, schedule_id: int):
        schedule = self.store.get_schedule(schedule_id)
        if not schedule or not schedule.is_active:
            return

        timestamp = datetime.now().strftime("%Y-%m-%d_%H%M%S")
        backup_path = f"{schedule.target}/{timestamp}"

        try:
            result = self.rclone.sync(schedule.source_path, backup_path)
            size = self.rclone.size(backup_path)
            files = self.rclone.ls(backup_path)

            from models import Backup, BackupStatus

            backup = Backup(
                label=f"{schedule.label} (scheduled)",
                source_path=schedule.source_path,
                target=schedule.target,
                backup_path=backup_path,
                status=BackupStatus.COMPLETED,
                size_bytes=size,
                file_count=len(files),
                created_at=datetime.now(),
            )
            backup_id = self.store.create_backup(backup)

            self.store.update_schedule(
                schedule_id,
                last_run=datetime.now().isoformat(),
                last_status="completed",
            )

            if self._backup_callback:
                self._backup_callback("success", backup_id=backup_id)

            return {"status": "success", "backup_id": backup_id, **result}
        except Exception as e:
            self.store.update_schedule(
                schedule_id,
                last_run=datetime.now().isoformat(),
                last_status="failed",
            )

            if self._backup_callback:
                self._backup_callback("error", error=str(e))

            return {"status": "error", "error": str(e)}
