package main

import (
	"log"

	"github.com/cubbit/ercubbit/config"
	"github.com/cubbit/ercubbit/db"
	"github.com/cubbit/ercubbit/mcp"
	"github.com/cubbit/ercubbit/storage"
	"github.com/cubbit/ercubbit/tools"
)

func main() {
	cfg := config.Load()

	store, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	rclone := storage.NewRcloneClient(cfg.RcloneConfig)

	server := mcp.NewServer(store, rclone, cfg)
	tools.Register(server)

	if err := server.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
