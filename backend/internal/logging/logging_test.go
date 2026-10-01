package logging

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/config"
	"github.com/gin-gonic/gin"
)

func TestFromContextFallsBackToDefault(t *testing.T) {
	if FromContext(context.Background()) != slog.Default() {
		t.Error("expected slog.Default() when no logger in context")
	}
}

func TestWithLoggerRoundTrip(t *testing.T) {
	l := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if got := FromContext(WithLogger(context.Background(), l)); got != l {
		t.Error("expected the stored logger back")
	}
}

func TestNewLogger(t *testing.T) {
	orig := slog.Default()
	t.Cleanup(func() { slog.SetDefault(orig) })

	prod := NewLogger(&config.Config{Environment: "prod"})
	if prod.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("prod logger should not emit debug")
	}
	if !prod.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("prod logger should emit info")
	}

	dev := NewLogger(&config.Config{Environment: "development"})
	if !dev.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("dev logger should emit debug")
	}
	if slog.Default() != dev {
		t.Error("NewLogger should install itself as the default")
	}
}

func TestGinRequestLoggerMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	var inHandler *slog.Logger
	r := gin.New()
	r.Use(GinRequestLoggerMiddleware(base))
	r.GET("/ping", func(c *gin.Context) {
		inHandler = FromContext(c.Request.Context())
		c.Status(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	id := w.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("expected X-Request-ID header")
	}
	if inHandler == nil || inHandler == base {
		t.Error("handler should receive the enriched logger")
	}
	out := buf.String()
	for _, want := range []string{"request started", "request completed", id, `"status":418`, `"path":"/ping"`, `"method":"GET"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
}
