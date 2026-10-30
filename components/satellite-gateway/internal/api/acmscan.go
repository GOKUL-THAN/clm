package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	acmscanner "github.com/m2p/clm/internal/satellite/discovery/scanners/cloud"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"go.uber.org/zap"
)

// AWS ACM discovery.
//
// The ACM scanner existed but was never wired to anything, so certificates
// managed by AWS were invisible to this gateway. They cannot be found by probing
// an internal range either: an ACM certificate lives in the AWS control plane and
// is attached to a load balancer, so a TLS sweep of private addresses will never
// see one. Cloud-managed certificates are usually the ones nobody is watching,
// which makes this the gap most likely to produce a surprise outage.

// scanACM lists ACM certificates across regions and converts them to the shared
// discovered-certificate shape.
func (s *Server) scanACM(ctx context.Context, regions []string) ([]*scanners.DiscoveredCert, error) {
	scanner := acmscanner.NewACMScanner(regions, s.logger)

	acmCerts, err := scanner.ScanAllRegions(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]*scanners.DiscoveredCert, 0, len(acmCerts))
	for _, c := range acmCerts {
		converted := c.ToDiscoveredCert()
		if converted == nil {
			continue
		}
		// Mark the origin so the inventory and the reports can tell an ACM record
		// apart from something observed on the wire.
		converted.DiscoveryMethod = "aws-acm"
		out = append(out, converted)
	}

	s.logger.Info("acm discovery complete",
		zap.Int("regions", len(regions)), zap.Int("certificates", len(out)))
	return out, nil
}

// handleDiscoverACM runs an ACM scan on demand.
//
// Also had no role check: unlike a network sweep this makes real outbound calls
// to a customer's AWS account with whatever credentials the gateway holds, so a
// viewer being able to trigger it at will is a real gap, not merely a read.
// POST /api/v1/discover/acm
func (s *Server) handleDiscoverACM(w http.ResponseWriter, r *http.Request) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot run an ACM discovery scan")
		return
	}

	var req struct {
		Regions []string `json:"regions"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	certs, err := s.scanACM(ctx, req.Regions)
	if err != nil {
		// Credentials are the usual cause, so say so rather than returning a bare
		// AWS error string.
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "credential") ||
			strings.Contains(strings.ToLower(msg), "no ec2imds") ||
			strings.Contains(strings.ToLower(msg), "not authorized") {
			msg += " — the gateway needs AWS credentials with acm:ListCertificates " +
				"and acm:DescribeCertificate, supplied through an instance role or the environment"
		}
		s.recordAudit(r, "discovery.acm", "discovery", "", strings.Join(req.Regions, ", "),
			audit.ResultFailure, msg)
		writeErr(w, http.StatusBadGateway, "ACM scan failed: "+msg)
		return
	}

	recorded := s.recordDiscovered(certs)
	s.recordAudit(r, "discovery.acm", "discovery", "", strings.Join(req.Regions, ", "),
		audit.ResultSuccess, fmt.Sprintf("certs_found=%d", recorded))
	writeJSON(w, http.StatusOK, map[string]any{
		"certs_found": recorded,
		"regions":     req.Regions,
		"source":      "aws-acm",
		"note": fmt.Sprintf("%d ACM certificate(s) folded into the inventory", recorded),
	})
}
