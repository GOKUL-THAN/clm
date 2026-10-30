package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auditservice "github.com/m2p/clm/internal/services/audit"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var safeRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

// MutationAudit buffers protected mutation responses until their immutable
// audit event is durable. If the audit write fails, the success response is
// replaced with 500. The domain write is not rolled back; this is intentionally
// visible and documented rather than pretending cross-table atomicity.
func MutationAudit(service *auditservice.Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		// Never let a caller-controlled identifier violate the audit schema or
		// inject an unsafe value into logs/responses. Invalid values are replaced
		// before the domain handler executes.
		if !safeRequestIDPattern.MatchString(requestID) {
			requestID = uuid.NewString()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		if GetTenantID(c) == uuid.Nil && strings.HasPrefix(c.Request.URL.Path, "/api/v1/identity") {
			target, err := uuid.Parse(c.GetHeader("X-CLM-Tenant-ID"))
			if err != nil || target == uuid.Nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-CLM-Tenant-ID is required for platform identity mutations"})
				return
			}
			c.Set("audit_tenant_id", target)
		}
		original := c.Writer
		buffered := newAuditBufferWriter(original)
		c.Writer = buffered
		c.Next()
		tenantID := GetTenantID(c)
		if value, ok := c.Get("audit_tenant_id"); ok {
			if parsed, ok := value.(uuid.UUID); ok {
				tenantID = parsed
			}
		}
		// A tenantless platform token can be rejected before a handler resolves
		// an explicit target (for example, malformed JSON). No mutation occurred,
		// so preserve that 4xx response rather than converting it into a misleading
		// audit-write 500. Successful mutations must always resolve a tenant and
		// still fail visibly below if they do not.
		if tenantID == uuid.Nil && buffered.status >= http.StatusBadRequest {
			logger.Warn("rejected platform mutation had no tenant audit scope", zap.String("request_id", requestID), zap.String("action", c.Request.Method+" "+c.FullPath()))
			buffered.flush()
			return
		}
		roleValue, _ := c.Get("role")
		role, _ := roleValue.(string)
		outcome := "success"
		if buffered.status >= 400 {
			outcome = "failure"
		}
		resourceType, resourceID := auditResource(c)
		event := &models.AuditEvent{TenantID: tenantID, RequestID: requestID, ActorID: GetUserID(c), ActorRole: role, Action: c.Request.Method + " " + c.FullPath(), ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, SourceIP: c.ClientIP(), Metadata: json.RawMessage(`{}`)}
		if err := service.Record(c.Request.Context(), event); err != nil {
			logger.Error("protected mutation audit write failed", zap.String("request_id", requestID), zap.String("action", event.Action), zap.Error(err))
			original.Header().Set("X-Audit-Write-Failed", "true")
			original.Header().Set("Content-Type", "application/json; charset=utf-8")
			payload, _ := json.Marshal(gin.H{"error": "mutation audit write failed", "request_id": requestID})
			original.WriteHeader(http.StatusInternalServerError)
			_, _ = original.Write(payload)
			return
		}
		buffered.flush()
	}
}
func auditResource(c *gin.Context) (string, string) {
	parts := strings.Split(strings.Trim(c.FullPath(), "/"), "/")
	resource := "control_plane"
	if len(parts) >= 3 {
		resource = parts[2]
	}
	id := ""
	for _, key := range []string{"id", "job_id", "cmd_id", "gid", "sid", "name", "alias"} {
		if value := c.Param(key); value != "" {
			id = value
			break
		}
	}
	return resource, id
}

type auditBufferWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
	size   int
}

func newAuditBufferWriter(base gin.ResponseWriter) *auditBufferWriter {
	return &auditBufferWriter{ResponseWriter: base, status: http.StatusOK}
}
func (w *auditBufferWriter) WriteHeader(code int) {
	if w.size >= 0 && w.status == http.StatusOK {
		w.status = code
	}
}
func (w *auditBufferWriter) WriteHeaderNow() {}
func (w *auditBufferWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}
func (w *auditBufferWriter) WriteString(value string) (int, error) { return w.Write([]byte(value)) }
func (w *auditBufferWriter) Status() int                           { return w.status }
func (w *auditBufferWriter) Size() int                             { return w.size }
func (w *auditBufferWriter) Written() bool                         { return w.size > 0 || w.status != http.StatusOK }
func (w *auditBufferWriter) Flush()                                {}
func (w *auditBufferWriter) CloseNotify() <-chan bool              { return w.ResponseWriter.CloseNotify() }
func (w *auditBufferWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.Hijack()
}
func (w *auditBufferWriter) Pusher() http.Pusher { return w.ResponseWriter.Pusher() }
func (w *auditBufferWriter) flush() {
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(w.body.Bytes())
}
