package main

import (
	"log/slog"
	"os"

	"github.com/aggregateway/agw"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := agw.Run(os.Args[1:]); err != nil {
		logger.Error("fatal", "error", err.Error())
		os.Exit(1)
	}
}
