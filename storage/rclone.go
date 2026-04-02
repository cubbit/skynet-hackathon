package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Minute

// RcloneClient wraps rclone as a subprocess.
type RcloneClient struct {
	configPath string // empty = rclone default (~/.config/rclone/rclone.conf)
}

func NewRcloneClient(configPath string) *RcloneClient {
	return &RcloneClient{configPath: configPath}
}

// run executes rclone with the given arguments and a default timeout.
func (r *RcloneClient) run(args ...string) (stdout, stderr string, err error) {
	return r.runCtx(context.Background(), args...)
}

func (r *RcloneClient) runCtx(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := []string{"rclone"}
	if r.configPath != "" {
		cmd = append(cmd, "--config", r.configPath)
	}
	cmd = append(cmd, args...)

	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf

	runErr := c.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr != nil {
		err = fmt.Errorf("%w\n%s", runErr, strings.TrimSpace(stderr))
	}
	return
}

// --- Remote management ---

func (r *RcloneClient) ListRemotes() ([]string, error) {
	stdout, _, err := r.run("listremotes")
	if err != nil {
		return nil, err
	}
	var remotes []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes, nil
}

func (r *RcloneClient) CreateRemote(name, remoteType string, params map[string]string) error {
	args := []string{"config", "create", name, remoteType}
	for k, v := range params {
		args = append(args, k+"="+v)
	}
	_, _, err := r.run(args...)
	return err
}

func (r *RcloneClient) DeleteRemote(name string) error {
	_, _, err := r.run("config", "delete", name)
	return err
}

func (r *RcloneClient) ShowRemote(name string) (map[string]string, error) {
	stdout, _, err := r.run("config", "dump")
	if err != nil {
		return nil, err
	}
	var all map[string]map[string]string
	if err := json.Unmarshal([]byte(stdout), &all); err != nil {
		return nil, fmt.Errorf("parsing rclone config dump: %w", err)
	}
	cfg, ok := all[name]
	if !ok {
		return nil, fmt.Errorf("remote %q not found", name)
	}
	return cfg, nil
}

// --- Transfer operations ---

type TransferStats struct {
	TransferredText string
	Errors          int
}

func (r *RcloneClient) Sync(ctx context.Context, src, dst string) (*TransferStats, error) {
	_, stderr, err := r.runCtx(ctx, "sync", src, dst, "--stats-one-line")
	if err != nil {
		return nil, err
	}
	return parseStats(stderr), nil
}

func (r *RcloneClient) Copy(ctx context.Context, src, dst string) (*TransferStats, error) {
	_, stderr, err := r.runCtx(ctx, "copy", src, dst, "--stats-one-line")
	if err != nil {
		return nil, err
	}
	return parseStats(stderr), nil
}

// --- Listing ---

type RemoteFile struct {
	Path     string
	Size     int64
	ModTime  time.Time
	IsDir    bool
}

func (r *RcloneClient) Ls(path string) ([]RemoteFile, error) {
	stdout, _, err := r.run("lsjson", path)
	if err != nil {
		return nil, err
	}
	var items []struct {
		Path    string `json:"Path"`
		Size    int64  `json:"Size"`
		ModTime string `json:"ModTime"`
		IsDir   bool   `json:"IsDir"`
	}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		return nil, fmt.Errorf("parsing lsjson output: %w", err)
	}
	files := make([]RemoteFile, 0, len(items))
	for _, item := range items {
		mod, _ := time.Parse(time.RFC3339Nano, item.ModTime)
		files = append(files, RemoteFile{
			Path:    item.Path,
			Size:    item.Size,
			ModTime: mod,
			IsDir:   item.IsDir,
		})
	}
	return files, nil
}

func (r *RcloneClient) Size(path string) (int64, error) {
	stdout, _, err := r.run("size", "--json", path)
	if err != nil {
		return 0, err
	}
	var data struct {
		Bytes int64 `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		return 0, nil
	}
	return data.Bytes, nil
}

// --- Diagnostics ---

// Version returns the rclone version string, or an error if rclone is not found.
func (r *RcloneClient) Version() (string, error) {
	stdout, _, err := r.run("version", "--check=false")
	if err != nil {
		return "", err
	}
	lines := strings.SplitN(strings.TrimSpace(stdout), "\n", 2)
	if len(lines) > 0 {
		return strings.TrimPrefix(lines[0], "rclone "), nil
	}
	return strings.TrimSpace(stdout), nil
}

// ConfigPath returns the path rclone is using for its config file.
func (r *RcloneClient) ConfigPath() (string, error) {
	stdout, _, err := r.run("config", "file")
	if err != nil {
		return "", err
	}
	// output: "Configuration file is stored at:\n/path/to/rclone.conf\n"
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	return strings.TrimSpace(lines[len(lines)-1]), nil
}

// --- helpers ---

func parseStats(stderr string) *TransferStats {
	stats := &TransferStats{TransferredText: "0 B"}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "Transferred:") {
			parts := strings.Fields(line)
			// "Transferred:  X.XXX GiB, 100%, ..."
			if len(parts) >= 2 {
				stats.TransferredText = parts[1] + " " + parts[2]
			}
		}
		if strings.Contains(line, "Errors:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fmt.Sscanf(parts[1], "%d", &stats.Errors)
			}
		}
	}
	return stats
}
