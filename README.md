# skynet-hackathon — MCP Rclone Backup Server

An [MCP](https://modelcontextprotocol.io) server that wraps [rclone](https://rclone.org) to provide backup management tools to Claude. Back up local paths or remote targets to S3 (or any rclone-supported remote), schedule recurring backups, and restore — all by talking to Claude naturally.

## How it works

```
Claude (client)  ──stdio──►  server.py (FastMCP)  ──subprocess──►  rclone
                                    │
                               SQLite DB
                          (~/.mcp-backup/backups.db)
```

Claude spawns the MCP server automatically on startup. No manual server management needed.

## Prerequisites

### 1. Python 3.10+

```bash
python3 --version
```

### 2. rclone

```bash
# macOS
brew install rclone

# Linux
sudo apt install rclone   # or: curl https://rclone.org/install.sh | sudo bash
```

### 3. Configure an rclone remote

At minimum, configure the S3 (or SSH/SFTP) remote you want to back up to:

```bash
rclone config
```

Example for S3:
```
n  → New remote
name: s3
storage: s3 (Amazon S3)
provider: AWS
env_auth: false
access_key_id: YOUR_KEY
secret_access_key: YOUR_SECRET
region: eu-west-1
```

Verify with:
```bash
rclone listremotes        # should list  s3:
rclone lsd s3:my-bucket   # should list bucket contents
```

## Installation

```bash
cd mcp-backup-server
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

## Configure Claude Code

The project ships a `.mcp.json` at the repo root. Before using it, update the absolute paths to match your machine:

```json
{
  "mcpServers": {
    "rclone-backup": {
      "command": "/absolute/path/to/mcp-backup-server/.venv/bin/python3",
      "args": ["/absolute/path/to/mcp-backup-server/server.py"],
      "env": {
        "BACKUP_DB_PATH": "~/.mcp-backup/backups.db"
      }
    }
  }
}
```

Then reload Claude Code (run `/hooks` or restart). Claude will discover the tools automatically.

### Configure Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "rclone-backup": {
      "command": "/absolute/path/to/mcp-backup-server/.venv/bin/python3",
      "args": ["/absolute/path/to/mcp-backup-server/server.py"],
      "env": {
        "BACKUP_DB_PATH": "~/.mcp-backup/backups.db"
      }
    }
  }
}
```

Restart Claude Desktop.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `BACKUP_DB_PATH` | `~/.mcp-backup/backups.db` | SQLite database path |
| `DEFAULT_BACKUP_TARGET` | `s3:my-bucket/backups` | Default rclone destination |
| `RCLONE_CONFIG` | (rclone default) | Path to a custom rclone config file |
| `SCHEDULER_TIMEZONE` | `UTC` | Timezone for scheduled backups |
| `LOG_LEVEL` | `INFO` | Logging verbosity |

## Available tools

Once connected, Claude can use these tools:

| Tool | Description |
|---|---|
| `configure_s3_remote` | Configure an S3 remote (AWS, Minio, Wasabi, Cloudflare R2, …) |
| `configure_sftp_remote` | Configure an SSH/SFTP remote |
| `show_remote` | Show a remote's config (secrets redacted by rclone) |
| `delete_remote` | Remove a configured remote |
| `backup_path` | Back up a local or remote path to a target |
| `list_backups` | List completed backups, optionally filtered by target |
| `schedule_backup` | Schedule a recurring backup with a cron expression |
| `restore_backup` | Restore a backup to a given path |
| `preview_restore` | List files in a backup before restoring |
| `list_schedules` | Show all active scheduled backups |
| `cancel_schedule` | Cancel a scheduled backup |

## Example usage

A full walkthrough from zero to scheduled backup.

---

### Step 1 — Configure a remote

> "Set up an S3 remote called `s3`. Access key is `AKIAIOSFODNN7EXAMPLE`, secret is `wJalrXUtnFEMI/K7MDENG`, region `eu-west-1`."

```
Claude calls: configure_s3_remote(
  name="s3",
  access_key_id="AKIAIOSFODNN7EXAMPLE",
  secret_access_key="wJalrXUtnFEMI/K7MDENG",
  region="eu-west-1"
)

→ { "remote": "s3", "type": "s3", "provider": "AWS", "status": "configured" }
```

---

### Step 2 — Run a first backup

> "Back up `/home/carlo/documents` to `s3:my-bucket/backups`, label it `documents`."

```
Claude calls: backup_path(
  source_path="/home/carlo/documents",
  target="s3:my-bucket/backups",
  label="documents"
)

→ { "backup_id": 1, "backup_path": "s3:my-bucket/backups/2026-04-01_120000",
    "size": "1.2 GB", "files": 3842, "status": "completed" }
```

---

### Step 3 — Schedule a recurring backup

> "Schedule that same backup to run every day at 2am."

```
Claude calls: schedule_backup(
  source_path="/home/carlo/documents",
  target="s3:my-bucket/backups",
  cron_expression="0 2 * * *",
  label="documents-daily"
)

→ { "schedule_id": 1, "cron": "0 2 * * *", "next_run": "2026-04-02T02:00:00" }
```

---

### Step 4 — Check status and storage usage

> "Show me all my backups and how much space they're using."

```
Claude calls: list_backups()

→ [
    { "id": 1, "label": "documents", "size": "1.2 GB", "files": 3842,
      "status": "completed", "created": "2026-04-01T12:00:00" },
    { "id": 2, "label": "documents-daily (scheduled)", "size": "1.2 GB",
      "files": 3842, "status": "completed", "created": "2026-04-02T02:00:01" }
  ]
```

> "What schedules are active?"

```
Claude calls: list_schedules()

→ [
    { "id": 1, "label": "documents-daily", "cron": "0 2 * * *",
      "next_run": "2026-04-03T02:00:00", "last_run": "2026-04-02T02:00:01",
      "last_status": "completed" }
  ]
```

---

For SSH/SFTP targets the flow is identical, just configure the remote differently:

> "Add an SSH remote called `myserver` for user `carlo` at `192.168.1.10`."

```
Claude calls: configure_sftp_remote(
  name="myserver", host="192.168.1.10", user="carlo"
)
```

Then use `myserver:backups` as the target in any backup or schedule command.

No need to run `rclone config` manually — Claude handles it.

## Project structure

```
mcp-backup-server/
├── server.py         # FastMCP server — tools, resources, prompts
├── rclone_client.py  # rclone subprocess wrapper
├── scheduler.py      # APScheduler-based background scheduler
├── store.py          # SQLite persistence (backups + schedules)
├── models.py         # Pydantic models
├── config.py         # Configuration from env vars
└── requirements.txt
.mcp.json             # Claude Code MCP server config (project-scoped)
flake.nix             # Nix dev shell (includes rclone + python3)
```

## Nix dev shell

If you use Nix, the dev shell provides rclone and Python:

```bash
nix develop   # or: direnv allow
```
