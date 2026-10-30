package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRequestLoggerAddsCommandFieldsWithoutQuerySecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(&output), zapcore.InfoLevel))
	router := gin.New()
	router.Use(RequestLogger(logger))
	router.GET("/commands", func(c *gin.Context) {
		c.Set("satellite_id", "satellite-1")
		c.Set("command_id", "command-1")
		c.Set("command_action", "claim")
		c.Set("command_count", 2)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commands?credential=must-not-appear", nil))
	logged := output.String()
	for _, expected := range []string{"satellite_id", "command_id", "command_action", "command_count", "query_present"} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log missing %q: %s", expected, logged)
		}
	}
	if strings.Contains(logged, "must-not-appear") || strings.Contains(logged, "credential=") {
		t.Fatalf("raw query secret was logged: %s", logged)
	}
}
