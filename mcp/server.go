package mcp

import (
	"github.com/cubbit/ercubbit/config"
	"github.com/cubbit/ercubbit/db"
	"github.com/cubbit/ercubbit/storage"
	mcpgo "github.com/mark3labs/mcp-go/server"
)

// Server holds shared dependencies and the underlying MCP server instance.
type Server struct {
	MCP    *mcpgo.MCPServer
	Store  *db.Store
	Rclone *storage.RcloneClient
	Config *config.Config
}

func NewServer(store *db.Store, rclone *storage.RcloneClient, cfg *config.Config) *Server {
	s := mcpgo.NewMCPServer(
		"cubbit-mcp",
		"0.1.0",
		mcpgo.WithToolCapabilities(true),
	)
	return &Server{
		MCP:    s,
		Store:  store,
		Rclone: rclone,
		Config: cfg,
	}
}

func (s *Server) Run() error {
	return mcpgo.ServeStdio(s.MCP)
}
