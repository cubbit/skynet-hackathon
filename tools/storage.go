package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cubbit/ercubbit/db"
	"github.com/cubbit/ercubbit/mcp"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerStorageTools(s *mcp.Server) {
	s.MCP.AddTool(mcpgo.NewTool("backup_path",
		mcpgo.WithDescription("Back up a local or remote path to a target destination using rclone sync."),
		mcpgo.WithString("source_path", mcpgo.Required(), mcpgo.Description("Local path to back up (e.g. /home/user/documents)")),
		mcpgo.WithString("target", mcpgo.Required(), mcpgo.Description("Destination (e.g. cubbit:my-bucket/backups)")),
		mcpgo.WithString("label", mcpgo.Required(), mcpgo.Description("Human-readable name for this backup")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleBackupPath(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("list_backups",
		mcpgo.WithDescription("List completed and running backups, optionally filtered by target."),
		mcpgo.WithString("target", mcpgo.Description("Filter by target destination (optional)")),
		mcpgo.WithNumber("limit", mcpgo.Description("Maximum number of results (default: 10)")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleListBackups(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("preview_restore",
		mcpgo.WithDescription("List the files inside a backup before restoring it, so you can verify it's the right one."),
		mcpgo.WithNumber("backup_id", mcpgo.Required(), mcpgo.Description("Backup ID from list_backups")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handlePreviewRestore(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("restore_backup",
		mcpgo.WithDescription("Restore a backup to a local path."),
		mcpgo.WithNumber("backup_id", mcpgo.Required(), mcpgo.Description("Backup ID from list_backups")),
		mcpgo.WithString("restore_path", mcpgo.Required(), mcpgo.Description("Local path to restore files into")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleRestoreBackup(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("show_remote",
		mcpgo.WithDescription("Show the configuration for a specific rclone remote. Secrets are redacted by rclone."),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Remote name")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleShowRemote(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("delete_remote",
		mcpgo.WithDescription("Remove a configured rclone remote. This does not delete any data, only the local configuration."),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Remote name to delete")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleDeleteRemote(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("configure_sftp_remote",
		mcpgo.WithDescription("Configure an SSH/SFTP remote in rclone for backing up to a remote server."),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Remote name (e.g. myserver)")),
		mcpgo.WithString("host", mcpgo.Required(), mcpgo.Description("SSH hostname or IP address")),
		mcpgo.WithString("user", mcpgo.Required(), mcpgo.Description("SSH username")),
		mcpgo.WithNumber("port", mcpgo.Description("SSH port (default: 22)")),
		mcpgo.WithString("key_file", mcpgo.Description("Path to private key file (uses SSH agent if omitted)")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleConfigureSFTP(ctx, s, req)
	}))
}

func handleBackupPath(ctx context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sourcePath, _ := args["source_path"].(string)
	target, _ := args["target"].(string)
	label, _ := args["label"].(string)

	timestamp := time.Now().Format("2006-01-02_150405")
	backupPath := fmt.Sprintf("%s/%s", strings.TrimRight(target, "/"), timestamp)

	backup := &db.Backup{
		Label:      label,
		SourcePath: sourcePath,
		Target:     target,
		BackupPath: backupPath,
		Status:     db.StatusRunning,
		CreatedAt:  time.Now(),
	}
	id, err := s.Store.CreateBackup(backup)
	if err != nil {
		return toolError(fmt.Sprintf("Failed to create backup record: %v", err))
	}

	stats, syncErr := s.Rclone.Sync(ctx, sourcePath, backupPath)
	if syncErr != nil {
		_ = s.Store.UpdateBackup(id, db.StatusFailed, 0, 0, syncErr.Error())
		return toolError(fmt.Sprintf("Backup failed: %v", syncErr))
	}

	// Count files
	files, _ := s.Rclone.Ls(backupPath)
	fileCount := len(files)
	sizeBytes, _ := s.Rclone.Size(backupPath)

	_ = s.Store.UpdateBackup(id, db.StatusCompleted, sizeBytes, fileCount, "")

	return toolText(fmt.Sprintf(
		"Backup completed successfully.\n\nID: %d\nLabel: %s\nSource: %s\nDestination: %s\nTransferred: %s\nFiles: %d",
		id, label, sourcePath, backupPath, stats.TransferredText, fileCount,
	))
}

func handleListBackups(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	target, _ := args["target"].(string)
	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	backups, err := s.Store.ListBackups(target, limit)
	if err != nil {
		return toolError(fmt.Sprintf("Failed to list backups: %v", err))
	}
	if len(backups) == 0 {
		return toolText("No backups found.")
	}

	var lines []string
	for _, b := range backups {
		size := formatBytes(b.SizeBytes)
		status := statusIcon(string(b.Status))
		lines = append(lines, fmt.Sprintf("%s [%d] %s", status, b.ID, b.Label))
		lines = append(lines, fmt.Sprintf("   Source:  %s", b.SourcePath))
		lines = append(lines, fmt.Sprintf("   Stored:  %s", b.BackupPath))
		lines = append(lines, fmt.Sprintf("   Size:    %s  |  Files: %d  |  Status: %s", size, b.FileCount, b.Status))
		lines = append(lines, fmt.Sprintf("   Created: %s", b.CreatedAt.Format("2006-01-02 15:04:05")))
		lines = append(lines, "")
	}
	return toolText(strings.Join(lines, "\n"))
}

func handlePreviewRestore(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	idF, _ := req.GetArguments()["backup_id"].(float64)
	id := int64(idF)

	backup, err := s.Store.GetBackup(id)
	if err != nil {
		return toolError(fmt.Sprintf("Backup %d not found.", id))
	}

	files, err := s.Rclone.Ls(backup.BackupPath)
	if err != nil {
		return toolError(fmt.Sprintf("Cannot list backup contents: %v", err))
	}

	if len(files) == 0 {
		return toolText(fmt.Sprintf("Backup %d is empty.", id))
	}

	limit := 100
	if len(files) > limit {
		files = files[:limit]
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Backup %d — %s (%s)", id, backup.Label, backup.BackupPath))
	lines = append(lines, strings.Repeat("-", 60))
	for _, f := range files {
		if !f.IsDir {
			lines = append(lines, fmt.Sprintf("  %s  (%s)", f.Path, formatBytes(f.Size)))
		}
	}
	if len(files) == limit {
		lines = append(lines, fmt.Sprintf("  ... (showing first %d files)", limit))
	}
	return toolText(strings.Join(lines, "\n"))
}

func handleRestoreBackup(ctx context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	idF, _ := args["backup_id"].(float64)
	id := int64(idF)
	restorePath, _ := args["restore_path"].(string)

	backup, err := s.Store.GetBackup(id)
	if err != nil {
		return toolError(fmt.Sprintf("Backup %d not found.", id))
	}

	stats, err := s.Rclone.Copy(ctx, backup.BackupPath, restorePath)
	if err != nil {
		return toolError(fmt.Sprintf("Restore failed: %v", err))
	}

	return toolText(fmt.Sprintf(
		"Restore completed.\n\nBackup: %s\nRestored to: %s\nTransferred: %s",
		backup.BackupPath, restorePath, stats.TransferredText,
	))
}

func handleShowRemote(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	name, _ := req.GetArguments()["name"].(string)

	cfg, err := s.Rclone.ShowRemote(name)
	if err != nil {
		return toolError(fmt.Sprintf("Remote %q not found. Run cubbit_setup_status to see configured remotes.", name))
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Remote: %s", name))
	for k, v := range cfg {
		lines = append(lines, fmt.Sprintf("  %s = %s", k, v))
	}
	return toolText(strings.Join(lines, "\n"))
}

func handleDeleteRemote(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	name, _ := req.GetArguments()["name"].(string)

	if err := s.Rclone.DeleteRemote(name); err != nil {
		return toolError(fmt.Sprintf("Failed to delete remote %q: %v", name, err))
	}
	return toolText(fmt.Sprintf("Remote %q has been removed from rclone config. No data was deleted.", name))
}

func handleConfigureSFTP(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	name, _ := args["name"].(string)
	host, _ := args["host"].(string)
	user, _ := args["user"].(string)
	port := 22
	if v, ok := args["port"].(float64); ok && v > 0 {
		port = int(v)
	}
	keyFile, _ := args["key_file"].(string)

	params := map[string]string{
		"host": host,
		"user": user,
		"port": fmt.Sprintf("%d", port),
	}
	if keyFile != "" {
		params["key_file"] = keyFile
	}

	if err := s.Rclone.CreateRemote(name, "sftp", params); err != nil {
		return toolError(fmt.Sprintf("Failed to configure SFTP remote: %v", err))
	}
	return toolText(fmt.Sprintf(
		"SSH/SFTP remote %q configured.\n\nHost: %s\nUser: %s\nPort: %d\n\nUse %s:<path> as target in backup commands.",
		name, host, user, port, name,
	))
}

// --- helpers ---

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func statusIcon(status string) string {
	switch status {
	case "completed":
		return "[OK]"
	case "failed":
		return "[FAIL]"
	default:
		return "[...]"
	}
}
