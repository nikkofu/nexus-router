package observability

import (
	"log/slog"
	"os"
)

func NewLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
