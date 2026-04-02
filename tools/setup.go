package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cubbit/ercubbit/mcp"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSetupTools(s *mcp.Server) {
	s.MCP.AddTool(mcpgo.NewTool("cubbit_setup",
		mcpgo.WithDescription("Configure Cubbit DS3 storage credentials and verify the connection. Run this first to get started."),
		mcpgo.WithString("access_key", mcpgo.Required(), mcpgo.Description("Cubbit access key from console.cubbit.eu")),
		mcpgo.WithString("secret_key", mcpgo.Required(), mcpgo.Description("Cubbit secret key from console.cubbit.eu")),
		mcpgo.WithString("bucket", mcpgo.Required(), mcpgo.Description("Bucket name (e.g. my-backups)")),
		mcpgo.WithString("remote_name", mcpgo.Description("Name for the rclone remote (default: cubbit)")),
		mcpgo.WithString("endpoint", mcpgo.Description("Custom S3 endpoint (leave empty for standard Cubbit: s3.cubbit.eu)")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleCubbitSetup(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("cubbit_setup_status",
		mcpgo.WithDescription("Check whether rclone is installed and Cubbit is configured. Call this before any backup operation if unsure."),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleCubbitSetupStatus(ctx, s, req)
	}))

	s.MCP.AddTool(mcpgo.NewTool("cubbit_doctor",
		mcpgo.WithDescription("Run a full diagnostic: checks rclone installation, config file, remote connectivity, and bucket access."),
		mcpgo.WithString("remote_name", mcpgo.Description("Remote name to diagnose (default: cubbit)")),
	), server.ToolHandlerFunc(func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return handleCubbitDoctor(ctx, s, req)
	}))
}

func handleCubbitSetup(ctx context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	accessKey, _ := args["access_key"].(string)
	secretKey, _ := args["secret_key"].(string)
	bucket, _ := args["bucket"].(string)
	remoteName, _ := args["remote_name"].(string)
	endpoint, _ := args["endpoint"].(string)

	if remoteName == "" {
		remoteName = "cubbit"
	}

	params := map[string]string{
		"provider":          "Cubbit",
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"region":            "eu-west-1",
	}
	if endpoint != "" {
		params["endpoint"] = endpoint
	}

	if err := s.Rclone.CreateRemote(remoteName, "s3", params); err != nil {
		return toolError(fmt.Sprintf("Failed to write rclone config: %v\n\nMake sure rclone is installed. Run: brew install rclone", err))
	}

	// Verify connection by listing the bucket
	target := fmt.Sprintf("%s:%s", remoteName, bucket)
	_, err := s.Rclone.Ls(target)
	if err != nil {
		return toolText(fmt.Sprintf(
			"Cubbit remote %q configured, but could not connect to bucket %q.\n\nError: %v\n\nPlease check:\n- Your access key and secret key are correct\n- The bucket %q exists in your Cubbit account (create it at console.cubbit.eu)\n- Your internet connection is working",
			remoteName, bucket, err, bucket,
		))
	}

	configPath, _ := s.Rclone.ConfigPath()
	return toolText(fmt.Sprintf(
		"Cubbit is configured and working!\n\nRemote: %s\nBucket: %s\nConfig file: %s\n\nYou can now back up files. Try: \"Back up /path/to/folder to %s\"",
		remoteName, bucket, configPath, target,
	))
}

func handleCubbitSetupStatus(_ context.Context, s *mcp.Server, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var lines []string

	// Check rclone is in PATH
	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		return toolText(
			"rclone is not installed or not in PATH.\n\n" +
				"Install it:\n" +
				"  macOS:  brew install rclone\n" +
				"  Linux:  sudo apt install rclone\n" +
				"  Windows: https://rclone.org/downloads/\n\n" +
				"After installing, run cubbit_setup to configure your credentials.",
		)
	}
	lines = append(lines, fmt.Sprintf("rclone: installed (%s)", rclonePath))

	// Check config file
	configPath, err := s.Rclone.ConfigPath()
	if err != nil {
		lines = append(lines, "rclone config: could not determine config path")
	} else {
		if _, err := os.Stat(configPath); err == nil {
			lines = append(lines, fmt.Sprintf("rclone config: found (%s)", configPath))
		} else {
			lines = append(lines, fmt.Sprintf("rclone config: not found (expected at %s)", configPath))
		}
	}

	// Check remotes
	remotes, err := s.Rclone.ListRemotes()
	if err != nil || len(remotes) == 0 {
		lines = append(lines, "configured remotes: none")
		lines = append(lines, "\nRun cubbit_setup to configure Cubbit credentials.")
	} else {
		lines = append(lines, fmt.Sprintf("configured remotes: %s", strings.Join(remotes, ", ")))

		// Check for cubbit-style remote
		hasCubbit := false
		for _, r := range remotes {
			if strings.Contains(strings.ToLower(r), "cubbit") {
				hasCubbit = true
				break
			}
		}
		if !hasCubbit {
			lines = append(lines, "\nNo Cubbit remote found. Run cubbit_setup to add one.")
		} else {
			lines = append(lines, "\nSetup looks good. You're ready to back up files.")
		}
	}

	return toolText(strings.Join(lines, "\n"))
}

func handleCubbitDoctor(_ context.Context, s *mcp.Server, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	remoteName, _ := req.GetArguments()["remote_name"].(string)
	if remoteName == "" {
		remoteName = "cubbit"
	}

	var report []string
	allOK := true

	// 1. rclone binary
	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		report = append(report, "[ FAIL ] rclone not found in PATH")
		report = append(report, "         Install: brew install rclone  (macOS) or apt install rclone  (Linux)")
		allOK = false
	} else {
		ver, _ := s.Rclone.Version()
		report = append(report, fmt.Sprintf("[  OK  ] rclone found: %s (%s)", ver, rclonePath))
	}

	// 2. Config file
	configPath, err := s.Rclone.ConfigPath()
	if err != nil {
		report = append(report, "[ FAIL ] could not determine rclone config path")
		allOK = false
	} else {
		if _, err := os.Stat(configPath); err == nil {
			report = append(report, fmt.Sprintf("[  OK  ] config file: %s", configPath))
		} else {
			report = append(report, fmt.Sprintf("[ WARN ] config file not found: %s", configPath))
			report = append(report, "         Run cubbit_setup to create it.")
		}
	}

	// 3. Remote exists
	remotes, err := s.Rclone.ListRemotes()
	if err != nil {
		report = append(report, fmt.Sprintf("[ FAIL ] could not list remotes: %v", err))
		allOK = false
	} else {
		found := false
		for _, r := range remotes {
			if strings.TrimSuffix(r, ":") == remoteName {
				found = true
				break
			}
		}
		if found {
			report = append(report, fmt.Sprintf("[  OK  ] remote %q exists", remoteName))
		} else {
			report = append(report, fmt.Sprintf("[ FAIL ] remote %q not configured (available: %s)", remoteName, strings.Join(remotes, ", ")))
			report = append(report, "         Run cubbit_setup to configure it.")
			allOK = false
		}
	}

	// 4. Connectivity: list top-level of remote
	if allOK {
		_, err := s.Rclone.Ls(remoteName + ":")
		if err != nil {
			report = append(report, fmt.Sprintf("[ FAIL ] cannot connect to %s: %v", remoteName, err))
			report = append(report, "         Check your credentials and network connection.")
			allOK = false
		} else {
			report = append(report, fmt.Sprintf("[  OK  ] connected to %s successfully", remoteName))
		}
	}

	summary := "All checks passed. Cubbit is ready to use."
	if !allOK {
		summary = "Some checks failed. See details above."
	}

	return toolText(strings.Join(report, "\n") + "\n\n" + summary)
}

