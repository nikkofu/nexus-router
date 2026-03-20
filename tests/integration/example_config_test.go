package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func TestExampleConfigBoots(t *testing.T) {
	data, err := os.ReadFile("../../configs/nexus-router.example.yaml")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	cfg, err := config.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	srv, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
