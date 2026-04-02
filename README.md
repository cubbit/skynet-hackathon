# Cubbit Backup for Claude

Back up your files to [Cubbit DS3](https://cubbit.io) (and any rclone-supported storage) by talking to Claude — no terminal needed.

## Install

1. Download the `.dxt` file for your platform from the [Releases](../../releases) page.
2. Double-click the `.dxt` file — Claude Desktop installs it automatically.
3. Restart Claude Desktop.

That's it. Claude now has backup tools available.

## First-time setup

Open Claude Desktop and say:

> "Set up Cubbit backup. My access key is `...`, my secret key is `...`, and my bucket is `my-backups`."

Claude will run `cubbit_setup`, write the rclone config, and verify the connection — all without you opening a terminal.

**Where to find your Cubbit credentials:**
1. Log in at [console.cubbit.eu](https://console.cubbit.eu)
2. Go to **API Keys** → **Create key**
3. Copy the Access Key and Secret Key

## Usage examples

### Back up a folder

> "Back up /Users/me/Documents to Cubbit, label it `documents`."

### Schedule a daily backup

> "Schedule a daily backup of /Users/me/Documents to cubbit:my-backups at 2 AM."

### Restore files

> "Show me the last 5 backups."
> "Restore backup #3 to /tmp/restore."

### Check status

> "Is Cubbit configured correctly?" → runs `cubbit_doctor`

## Available tools

| Tool | What it does |
|---|---|
| `cubbit_setup` | Configure Cubbit credentials |
| `cubbit_setup_status` | Check installation and config |
| `cubbit_doctor` | Full diagnostic |
| `backup_path` | Back up a path |
| `list_backups` | List past backups |
| `preview_restore` | Preview files before restoring |
| `restore_backup` | Restore a backup |
| `schedule_backup` | Schedule recurring backups |
| `list_schedules` | Show active schedules |
| `cancel_schedule` | Cancel a schedule |
| `show_remote` | Show remote config |
| `delete_remote` | Remove a remote |
| `configure_sftp_remote` | Add an SSH/SFTP target |

## Prerequisites

The server needs **rclone** installed on your machine. If it's not installed, `cubbit_setup_status` will tell you exactly how to install it.

Quick install:
- **macOS:** `brew install rclone`
- **Linux:** `sudo apt install rclone`
- **Windows:** [rclone.org/downloads](https://rclone.org/downloads/)

## FAQ

**Can I use this with AWS S3, Backblaze, or other providers?**
Yes — rclone supports 70+ storage providers. Use `configure_sftp_remote` for SSH targets. For other S3-compatible providers, ask Claude to configure a custom remote.

**Where is my data stored?**
On your Cubbit bucket (or whichever remote you configure). The server only stores backup metadata locally in `~/.mcp-backup/backups.db`.

**Does the scheduled backup survive a restart?**
Schedules are persisted in the local SQLite database and reloaded automatically each time Claude Desktop starts the MCP server.

**I already have rclone configured. Will this break it?**
No. `cubbit_setup` only adds a new remote entry to your existing `~/.config/rclone/rclone.conf`. Existing remotes are untouched.

## Build from source

Requires Go 1.22+ and rclone in your PATH.

```bash
git clone https://github.com/cubbit/ercubbit
cd ercubbit
go build -o cubbit-mcp .
```

To build all platforms and package `.dxt` bundles:

```bash
./build-dxt.sh
```

Output will be in `dist/`.
