# MCP Rclone Backup Server

FastMCP server that wraps rclone to expose backup operations as Claude tools.

See the [project root README](../README.md) for full setup instructions.

## Quick start

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

Configure `.mcp.json` at the project root with the path to `.venv/bin/python3` and `server.py`, then reload Claude Code.

## Tools exposed

**Remote configuration**
- `configure_s3_remote(name, access_key_id, secret_access_key, region, provider?)` — create an S3 remote
- `configure_sftp_remote(name, host, user, port?, key_file?)` — create an SSH/SFTP remote
- `show_remote(name)` — show a remote's config (secrets redacted by rclone)
- `delete_remote(name)` — remove a remote

**Backup operations**
- `backup_path(source_path, target, label)` — run a full backup via `rclone sync`
- `list_backups(target?, limit?)` — query backup history from SQLite
- `schedule_backup(source_path, target, cron_expression, label)` — add a recurring job
- `restore_backup(backup_id, restore_path)` — restore via `rclone copy`
- `preview_restore(backup_id)` — list files before restoring
- `list_schedules(active_only?)` — show scheduled jobs
- `cancel_schedule(schedule_id)` — deactivate a schedule

## Target formats

| Type | Example |
|---|---|
| S3 | `s3:my-bucket/backups` |
| SSH/SFTP | `sftp:user@host/path` |
| Local | `/mnt/backup/external` |
| Any rclone remote | `remote-name:path` |

Remotes must be configured with `rclone config` before use.
