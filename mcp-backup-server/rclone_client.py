import subprocess
import json
from typing import List, Optional
from datetime import datetime

from models import BackupFile


class RcloneClient:
    def __init__(self, config_path: Optional[str] = None):
        self.config_path = config_path

    def _run(self, args: List[str]) -> subprocess.CompletedProcess:
        cmd = ["rclone"]
        if self.config_path:
            cmd.extend(["--config", self.config_path])
        cmd.extend(args)
        return subprocess.run(cmd, capture_output=True, text=True)

    def list_remotes(self) -> List[str]:
        result = self._run(["listremotes"])
        return [
            r.rstrip() for r in result.stdout.strip().split("\n") if r.strip()
        ]

    def sync(self, source: str, dest: str, progress: bool = True) -> dict:
        args = ["sync", source, dest]
        if progress:
            args.append("--progress")
        result = self._run(args)
        if result.returncode != 0:
            raise RuntimeError(f"rclone sync failed: {result.stderr}")
        return self._parse_stats(result.stderr)

    def copy(self, source: str, dest: str, progress: bool = True) -> dict:
        args = ["copy", source, dest]
        if progress:
            args.append("--progress")
        result = self._run(args)
        if result.returncode != 0:
            raise RuntimeError(f"rclone copy failed: {result.stderr}")
        return self._parse_stats(result.stderr)

    def ls(self, path: str, recursive: bool = True) -> List[BackupFile]:
        args = ["lsjson", path]
        if not recursive:
            args.append("--no-recurse")
        result = self._run(args)
        if result.returncode != 0:
            raise RuntimeError(f"rclone lsjson failed: {result.stderr}")
        try:
            items = json.loads(result.stdout)
        except json.JSONDecodeError:
            return []
        files = []
        for item in items:
            if not item.get("IsDir", False):
                # rclone ModTime may have nanoseconds; truncate to microseconds
                mod_time_str = item.get("ModTime", datetime.now().isoformat())[:26]
                try:
                    mod_time = datetime.fromisoformat(mod_time_str)
                except ValueError:
                    mod_time = datetime.now()
                files.append(
                    BackupFile(
                        path=item.get("Path", ""),
                        size=item.get("Size", 0),
                        modification_time=mod_time,
                    )
                )
        return files

    def lsd(self, path: str) -> List[str]:
        result = self._run(["lsd", path])
        return [
            d.split("\t", 1)[1] if "\t" in d else d
            for d in result.stdout.strip().split("\n")
            if d.strip()
        ]

    def size(self, path: str) -> int:
        result = self._run(["size", "--json", path])
        if result.returncode != 0:
            return 0
        try:
            data = json.loads(result.stdout)
            return data.get("bytes", 0)
        except (json.JSONDecodeError, KeyError):
            return 0

    def create_remote(self, name: str, remote_type: str, params: dict) -> None:
        args = ["config", "create", name, remote_type]
        for key, value in params.items():
            args.append(f"{key}={value}")
        result = self._run(args)
        if result.returncode != 0:
            raise RuntimeError(f"rclone config create failed: {result.stderr}")

    def update_remote(self, name: str, params: dict) -> None:
        args = ["config", "update", name]
        for key, value in params.items():
            args.append(f"{key}={value}")
        result = self._run(args)
        if result.returncode != 0:
            raise RuntimeError(f"rclone config update failed: {result.stderr}")

    def delete_remote(self, name: str) -> None:
        result = self._run(["config", "delete", name])
        if result.returncode != 0:
            raise RuntimeError(f"rclone config delete failed: {result.stderr}")

    def show_remote(self, name: str) -> dict:
        result = self._run(["config", "dump"])
        if result.returncode != 0:
            raise RuntimeError(f"rclone config dump failed: {result.stderr}")
        try:
            all_config = json.loads(result.stdout)
            return all_config.get(name, {})
        except json.JSONDecodeError:
            return {}

    def _parse_stats(self, stderr: str) -> dict:
        stats = {"transferred": 0, "errors": 0, "transferred_text": "0B"}
        for line in stderr.split("\n"):
            if "Transferred:" in line:
                parts = line.split()
                stats["transferred_text"] = parts[1] if len(parts) > 1 else "0B"
            if "Errors:" in line:
                parts = line.split()
                stats["errors"] = int(parts[1]) if len(parts) > 1 else 0
        return stats
