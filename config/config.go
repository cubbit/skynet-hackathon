package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DatabasePath      string
	RcloneConfig      string // empty = rclone default
	DefaultTarget     string
	SchedulerTimezone string
}

func Load() *Config {
	dbPath := os.Getenv("BACKUP_DB_PATH")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".mcp-backup", "backups.db")
	}

	return &Config{
		DatabasePath:      dbPath,
		RcloneConfig:      os.Getenv("RCLONE_CONFIG"),
		DefaultTarget:     getEnv("DEFAULT_BACKUP_TARGET", "cubbit:my-bucket/backups"),
		SchedulerTimezone: getEnv("SCHEDULER_TIMEZONE", "UTC"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
