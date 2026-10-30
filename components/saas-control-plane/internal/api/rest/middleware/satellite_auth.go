package middleware

import (
	"context"
	"crypto/x509"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/models"
)

const (
	maxAuthorizationHeaderBytes = 1024
	maxSatelliteCredentialBytes = 256
	maxConcurrentSatelliteAuth  = 8
)

type SatelliteAuthenticator interface {
	AuthenticateSatellite(ctx context.Context, satelliteID uuid.UUID, plaintext string) (*models.SatelliteIdentity, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Satellite, error)
}

// SatelliteAuth authenticates the bearer credential against the satellite ID in
// the route. If a client certificate is present its SAN/URI is also bound to the
// same tenant and satellite. Legacy unauthenticated access is possible only when
// explicitly enabled in a non-production configuration.
func SatelliteAuth(auth SatelliteAuthenticator, server config.ServerConfig) gin.HandlerFunc {
	authSlots := make(chan struct{}, maxConcurrentSatelliteAuth)
	return func(c *gin.Context) {
		satelliteID, err := uuid.Parse(c.Param("id"))
		if err != nil || satelliteID == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid satellite ID"})
			return
		}
		authorization := c.GetHeader("Authorization")
		credential, present := bearerCredential(authorization)
		if authorization != "" && !present {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid satellite credential"})
			return
		}
		var identity *models.SatelliteIdentity
		if present {
			select {
			case authSlots <- struct{}{}:
				identity, err = auth.AuthenticateSatellite(c.Request.Context(), satelliteID, credential)
				<-authSlots
			case <-c.Request.Context().Done():
				c.AbortWithStatusJSON(http.StatusRequestTimeout, gin.H{"error": "satellite authentication cancelled"})
				return
			default:
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "satellite authentication capacity exceeded"})
				return
			}
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid satellite credential"})
				return
			}
		} else if server.Gateway.AllowInsecureLegacy && !isProductionEnv(server.Env) {
			satellite, lookupErr := auth.GetByID(c.Request.Context(), satelliteID)
			if lookupErr != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "satellite authentication required"})
				return
			}
			identity = &models.SatelliteIdentity{SatelliteID: satellite.ID, TenantID: satellite.TenantID}
		} else {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "satellite authentication required"})
			return
		}

		cert := peerCertificate(c)
		if strings.EqualFold(server.TLS.ClientAuth, "required") && cert == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "gateway client certificate required"})
			return
		}
		if cert != nil && !certificateMatches(cert, identity.TenantID, identity.SatelliteID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "gateway certificate identity mismatch"})
			return
		}
		c.Set("satellite_id", identity.SatelliteID)
		c.Set("tenant_id", identity.TenantID)
		c.Next()
	}
}

func bearerCredential(header string) (string, bool) {
	if len(header) == 0 || len(header) > maxAuthorizationHeaderBytes {
		return "", false
	}
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", false
	}
	credential := strings.TrimSpace(parts[1])
	if credential == "" || len(credential) > maxSatelliteCredentialBytes {
		return "", false
	}
	return credential, true
}

func peerCertificate(c *gin.Context) *x509.Certificate {
	if c.Request.TLS == nil || len(c.Request.TLS.VerifiedChains) == 0 || len(c.Request.TLS.VerifiedChains[0]) == 0 {
		return nil
	}
	return c.Request.TLS.VerifiedChains[0][0]
}

func certificateMatches(cert *x509.Certificate, tenantID, satelliteID uuid.UUID) bool {
	tenant := tenantID.String()
	satellite := satelliteID.String()
	for _, uri := range cert.URIs {
		path := strings.TrimSuffix(uri.EscapedPath(), "/")
		if (uri.Scheme == "spiffe" || uri.Scheme == "clm") &&
			strings.HasSuffix(path, "/tenants/"+tenant+"/satellites/"+satellite) {
			return true
		}
	}
	for _, dnsName := range cert.DNSNames {
		if strings.EqualFold(dnsName, satellite) || strings.EqualFold(dnsName, "satellite-"+satellite) {
			return true
		}
	}
	return false
}

func isProductionEnv(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "production") || strings.EqualFold(strings.TrimSpace(env), "prod")
}
