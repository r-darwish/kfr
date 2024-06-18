package main

import (
	"log/slog"
	"os"

	"github.com/r-darwish/kfr/kfr"
)

func main() {
	if err := kfr.Purge(); err != nil {
		slog.Error("", "error", err)
		os.Exit(1)
	}
}
