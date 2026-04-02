package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cubbit/ercubbit/db"
	"github.com/cubbit/ercubbit/mcp"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/robfig/cron/v3"
)

// scheduler is a package-level cron runner shared across tool calls.
var (
	cronRunner  *cron.Cron
	cronEntries = map[int64]cron.EntryID{} // schedule DB id -> cron entry id
	cronMu      sync.Mutex
)

func initScheduler(s *mcp.Server) {
	cronRunner = cron.New(cron.WithLocation(time.UTC))

	// Reload active schedules from DB on startup
	schedules, err := s.Store.ListSchedules(true)
	if err == nil {
		for _, sch := range schedules {
			addCronEntry(s, sch)
		}
	}
	cronRunner.Start()
}

func addCronEntry(s *mcp.Server, sch *db.Schedule) {
	entryID, err := cronRunner.AddFunc(sch.CronExpression, func() {
		runScheduledBackup(s, sch)
	})
	if err == nil {
		cronMu.Lock()
		cronEntries[sch.ID] = entryID
		cronMu.Unlock()

		// Update next_run in DB
		entry := cronRunner.Entry(entryID)
		_ = s.Store.UpdateScheduleNextRun(sch.ID, entry.Next)
	}
}

func runScheduledBackup(s *mcp.Server, sch *db.Schedule) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	timestamp := time.Now().Format("2006-01-02_150405")
	backupPath := fmt.Sprintf("%s/%s", strings.TrimRight(sch.Target, "/"), timestamp)

	backup := &db.Backup{
		Label:      sch.Label,
		SourcePath: sch.SourcePath,
		Target:     sch.Target,
		BackupPath: backupPath,
		Status:     db.StatusRunning,
		CreatedAt:  time.Now(),
	}
	id, err := s.Store.CreateBackup(backup)
	status := "completed"
	errMsg := ""

	if err == nil {
		_, syncErr := s.Rclone.Sync(ctx, sch.SourcePath, backupPath)
		if syncErr != nil {
			status = "failed"
			errMsg = syncErr.Error()
			_ = s.Store.UpdateBackup(id, db.StatusFailed, 0, 0, errMsg)
		} else {
			files, _ := s.Rclone.Ls(backupPath)
			sizeBytes, _ := s.Rclone.Size(backupPath)
			_ = s.Store.UpdateBackup(id, db.StatusCompleted, sizeBytes, len(files), "")
		}
	} else {
		status = "failed"
		errMsg = err.Error()
	}

	_ = s.Store.UpdateScheduleLastRun(sch.ID, time.Now(), status)

	// Update next_run
	cronMu.Lock()
	entryID, ok := cronEntries[sch.ID]
	cronMu.Unlock()
	if ok {
		entry := cronRunner.Entry(entryID)
		_ = s.Store.UpdateScheduleNextRun(sch.ID, entry.Next)
	}

	_ = errMsg // suppress unused warning when status is used
}

func registerScheduleTools(s *mcp.Server) {
	initScheduler(s)

	s.MCP.AddTool(mcpgo.NewTool("schedule_backup",
		mcpgo.WithDescription("Schedule a recurring backup using a cron expression."),
		mcpgo.WithString("source_path", mcpgo.Required(), mcpgo.Description("Local path to back up")),
		mcpgo.WithString("target", mcpgo.Required(), mcpgo.Description("Destination (e.g. cubbit:my-bucket/backups)")),
		mcpgo.WithString("cron_expression", mcpgo.Required(), mcpgo.Description("Cron schedule (e.g. \"0 2 * * *\" for daily at 2 AM)")),
		mcpgo.WithString("label", mcpgo.Required(), mcpgo.Description("Human-readable name for this schedule")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleScheduleBackup(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("list_schedules",
		mcpgo.WithDescription("List all active backup schedules with their next and last run times."),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleListSchedules(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("cancel_schedule",
		mcpgo.WithDescription("Cancel a scheduled backup. Does not delete any existing backup data."),
		mcpgo.WithNumber("schedule_id", mcpgo.Required(), mcpgo.Description("Schedule ID from list_schedules")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleCancelSchedule(ctx, s, req)
	}))
}

func handleScheduleBackup(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sourcePath, _ := args["source_path"].(string)
	target, _ := args["target"].(string)
	cronExpr, _ := args["cron_expression"].(string)
	label, _ := args["label"].(string)

	// Validate cron expression before saving
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return toolError(fmt.Sprintf("Invalid cron expression %q: %v\n\nExamples:\n  \"0 2 * * *\"   — daily at 2:00 AM\n  \"0 */6 * * *\" — every 6 hours\n  \"0 9 * * 1\"   — every Monday at 9 AM", cronExpr, err))
	}

	schedule := &db.Schedule{
		Label:          label,
		SourcePath:     sourcePath,
		Target:         target,
		CronExpression: cronExpr,
	}
	id, err := s.Store.CreateSchedule(schedule)
	if err != nil {
		return toolError(fmt.Sprintf("Failed to save schedule: %v", err))
	}
	schedule.ID = id

	addCronEntry(s, schedule)

	nextRun := sched.Next(time.Now())
	return toolText(fmt.Sprintf(
		"Backup scheduled.\n\nID: %d\nLabel: %s\nSource: %s\nDestination: %s\nSchedule: %s\nNext run: %s",
		id, label, sourcePath, target, cronExpr, nextRun.Format("2006-01-02 15:04:05 UTC"),
	))
}

func handleListSchedules(_ context.Context, s *mcp.Server, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	schedules, err := s.Store.ListSchedules(true)
	if err != nil {
		return toolError(fmt.Sprintf("Failed to list schedules: %v", err))
	}
	if len(schedules) == 0 {
		return toolText("No active schedules. Use schedule_backup to create one.")
	}

	var lines []string
	for _, sch := range schedules {
		lines = append(lines, fmt.Sprintf("[%d] %s", sch.ID, sch.Label))
		lines = append(lines, fmt.Sprintf("   %s  ->  %s", sch.SourcePath, sch.Target))
		lines = append(lines, fmt.Sprintf("   Schedule: %s", sch.CronExpression))
		if sch.NextRun != nil {
			lines = append(lines, fmt.Sprintf("   Next run: %s", sch.NextRun.Format("2006-01-02 15:04:05 UTC")))
		}
		if sch.LastRun != nil {
			lines = append(lines, fmt.Sprintf("   Last run: %s  (%s)", sch.LastRun.Format("2006-01-02 15:04:05 UTC"), sch.LastStatus))
		} else {
			lines = append(lines, "   Last run: never")
		}
		lines = append(lines, "")
	}
	return toolText(strings.Join(lines, "\n"))
}

func handleCancelSchedule(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	idF, _ := req.GetArguments()["schedule_id"].(float64)
	id := int64(idF)

	// Remove from cron
	cronMu.Lock()
	if entryID, ok := cronEntries[id]; ok {
		cronRunner.Remove(entryID)
		delete(cronEntries, id)
	}
	cronMu.Unlock()

	if err := s.Store.DeactivateSchedule(id); err != nil {
		return toolError(fmt.Sprintf("Failed to cancel schedule %d: %v", id, err))
	}
	return toolText(fmt.Sprintf("Schedule %d cancelled. Existing backup data is not affected.", id))
}
