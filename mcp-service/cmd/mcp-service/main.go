package main

import (
	"log/slog"
	"os"

	"github.com/PetoAdam/homenavi/mcp-service/internal/app"
)

func main() {
	application, err := app.New(app.LoadConfig(), slog.Default())
	if err != nil {
		slog.Error("configure mcp service", "error", err)
		os.Exit(1)
	}
	if err := application.Run(); err != nil {
		slog.Error("run mcp service", "error", err)
		os.Exit(1)
	}
}
