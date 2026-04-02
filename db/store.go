package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type BackupStatus string

const (
	StatusRunning   BackupStatus = "running"
	StatusCompleted BackupStatus = "completed"
	StatusFailed    BackupStatus = "failed"
)

type Backup struct {
	ID           int64
	Label        string
	SourcePath   string
	Target       string
	BackupPath   string
	Status       BackupStatus
	SizeBytes    int64
	FileCount    int
	ErrorMessage string
	CreatedAt    time.Time
}

type Schedule struct {
	ID             int64
	Label          string
	SourcePath     string
	Target         string
	CronExpression string
	IsActive       bool
	NextRun        *time.Time
	LastRun        *time.Time
	LastStatus     string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS backups (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			label         TEXT NOT NULL,
			source_path   TEXT NOT NULL,
			target        TEXT NOT NULL,
			backup_path   TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'running',
			size_bytes    INTEGER NOT NULL DEFAULT 0,
			file_count    INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS schedules (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			label           TEXT NOT NULL,
			source_path     TEXT NOT NULL,
			target          TEXT NOT NULL,
			cron_expression TEXT NOT NULL,
			is_active       INTEGER NOT NULL DEFAULT 1,
			next_run        DATETIME,
			last_run        DATETIME,
			last_status     TEXT NOT NULL DEFAULT ''
		);
	`)
	return err
}

// --- Backups ---

func (s *Store) CreateBackup(b *Backup) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO backups (label, source_path, target, backup_path, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		b.Label, b.SourcePath, b.Target, b.BackupPath, b.Status, b.CreatedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateBackup(id int64, status BackupStatus, sizeBytes int64, fileCount int, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE backups SET status=?, size_bytes=?, file_count=?, error_message=? WHERE id=?`,
		status, sizeBytes, fileCount, errMsg, id,
	)
	return err
}

func (s *Store) GetBackup(id int64) (*Backup, error) {
	row := s.db.QueryRow(
		`SELECT id, label, source_path, target, backup_path, status, size_bytes, file_count, error_message, created_at
		 FROM backups WHERE id=?`, id)
	return scanBackup(row)
}

func (s *Store) ListBackups(target string, limit int) ([]*Backup, error) {
	var rows *sql.Rows
	var err error
	if target != "" {
		rows, err = s.db.Query(
			`SELECT id, label, source_path, target, backup_path, status, size_bytes, file_count, error_message, created_at
			 FROM backups WHERE target=? ORDER BY created_at DESC LIMIT ?`, target, limit)
	} else {
		rows, err = s.db.Query(
			`SELECT id, label, source_path, target, backup_path, status, size_bytes, file_count, error_message, created_at
			 FROM backups ORDER BY created_at DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []*Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBackup(s scanner) (*Backup, error) {
	var b Backup
	var createdAt string
	err := s.Scan(
		&b.ID, &b.Label, &b.SourcePath, &b.Target, &b.BackupPath,
		&b.Status, &b.SizeBytes, &b.FileCount, &b.ErrorMessage, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	b.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
	if b.CreatedAt.IsZero() {
		b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	return &b, nil
}

// --- Schedules ---

func (s *Store) CreateSchedule(sch *Schedule) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO schedules (label, source_path, target, cron_expression, is_active)
		 VALUES (?, ?, ?, ?, 1)`,
		sch.Label, sch.SourcePath, sch.Target, sch.CronExpression,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetSchedule(id int64) (*Schedule, error) {
	row := s.db.QueryRow(
		`SELECT id, label, source_path, target, cron_expression, is_active, next_run, last_run, last_status
		 FROM schedules WHERE id=?`, id)
	return scanSchedule(row)
}

func (s *Store) ListSchedules(activeOnly bool) ([]*Schedule, error) {
	var rows *sql.Rows
	var err error
	if activeOnly {
		rows, err = s.db.Query(
			`SELECT id, label, source_path, target, cron_expression, is_active, next_run, last_run, last_status
			 FROM schedules WHERE is_active=1 ORDER BY id`)
	} else {
		rows, err = s.db.Query(
			`SELECT id, label, source_path, target, cron_expression, is_active, next_run, last_run, last_status
			 FROM schedules ORDER BY id`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []*Schedule
	for rows.Next() {
		sch, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, sch)
	}
	return schedules, rows.Err()
}

func (s *Store) UpdateScheduleNextRun(id int64, next time.Time) error {
	_, err := s.db.Exec(`UPDATE schedules SET next_run=? WHERE id=?`, next.UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) UpdateScheduleLastRun(id int64, last time.Time, status string) error {
	_, err := s.db.Exec(
		`UPDATE schedules SET last_run=?, last_status=? WHERE id=?`,
		last.UTC().Format(time.RFC3339), status, id,
	)
	return err
}

func (s *Store) DeactivateSchedule(id int64) error {
	_, err := s.db.Exec(`UPDATE schedules SET is_active=0 WHERE id=?`, id)
	return err
}

func scanSchedule(sc scanner) (*Schedule, error) {
	var sch Schedule
	var nextRun, lastRun sql.NullString
	var isActive int
	err := sc.Scan(
		&sch.ID, &sch.Label, &sch.SourcePath, &sch.Target,
		&sch.CronExpression, &isActive, &nextRun, &lastRun, &sch.LastStatus,
	)
	if err != nil {
		return nil, err
	}
	sch.IsActive = isActive == 1
	if nextRun.Valid && nextRun.String != "" {
		t, _ := time.Parse(time.RFC3339, nextRun.String)
		sch.NextRun = &t
	}
	if lastRun.Valid && lastRun.String != "" {
		t, _ := time.Parse(time.RFC3339, lastRun.String)
		sch.LastRun = &t
	}
	return &sch, nil
}
