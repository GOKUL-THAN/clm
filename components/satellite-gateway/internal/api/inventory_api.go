package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/securefs"
	"go.uber.org/zap"
)

// The certificate inventory as a first-class thing.
//
// The certificate list was a flat table of seven columns with no search, no
// filter and no way to look at a single record. Discovery had been collecting
// fingerprints, serial numbers, SANs, key sizes and the method each certificate
// was found by, and none of it was viewable — the data was gathered and then
// hidden. There was also no way to remove anything, so failed issuance attempts
// and hosts that no longer exist accumulated permanently.
//
// This adds the operations an inventory actually needs: facets to slice it,
// per-record detail, removal, and pruning of records that are no longer real.

// InventorySummary describes the estate and supplies the values the UI filters
// on, so the portal does not have to derive them and then disagree with the API.
type InventorySummary struct {
	Total    int `json:"total"`
	Managed  int `json:"managed"`
	Observed int `json:"observed"`
	Expired  int `json:"expired"`
	Expiring int `json:"expiring_30d"`
	Revoked  int `json:"revoked"`
	Failed   int `json:"failed"`
	// Stale counts records not observed by discovery for over 30 days. Managed
	// certificates are excluded: the gateway owns them, so not having seen one on
	// the wire says nothing about whether it still exists.
	Stale int `json:"stale"`

	Statuses     []NameCount `json:"statuses"`
	Sources      []NameCount `json:"sources"`
	Issuers      []NameCount `json:"issuers"`
	Environments []NameCount `json:"environments"`
	Methods      []NameCount `json:"discovery_methods"`
}

// StaleAfter is how long a discovered certificate can go unseen before the
// inventory flags it.
const StaleAfter = 30 * 24 * time.Hour

// buildInventorySummary computes the facets.
func buildInventorySummary(certs []*Certificate) InventorySummary {
	s := InventorySummary{Total: len(certs)}

	statuses := map[string]int{}
	sources := map[string]int{}
	issuers := map[string]int{}
	envs := map[string]int{}
	methods := map[string]int{}

	for _, c := range certs {
		statuses[nonEmpty(c.Status, "unknown")]++
		sources[nonEmpty(c.Source, "unknown")]++
		issuers[nonEmpty(c.Issuer, "(unknown)")]++
		envs[nonEmpty(c.Environment, "unknown")]++
		if c.DiscoveryMethod != "" {
			methods[c.DiscoveryMethod]++
		}

		if c.ManagedBy != "" {
			s.Managed++
		} else {
			s.Observed++
		}
		switch {
		case c.Revoked:
			s.Revoked++
		case c.Status == "failed":
			s.Failed++
		case !c.NotAfter.IsZero() && time.Until(c.NotAfter) < 0:
			s.Expired++
		case !c.NotAfter.IsZero() && time.Until(c.NotAfter) <= 30*24*time.Hour:
			s.Expiring++
		}

		if c.ManagedBy == "" && !c.LastSeenAt.IsZero() &&
			time.Since(c.LastSeenAt) > StaleAfter {
			s.Stale++
		}
	}

	s.Statuses = toNameCounts(statuses)
	s.Sources = toNameCounts(sources)
	s.Issuers = toNameCounts(issuers)
	s.Environments = toNameCounts(envs)
	s.Methods = toNameCounts(methods)
	return s
}

// matchesFilters applies the query parameters to one record.
//
// Filtering server-side matters because the portal would otherwise download the
// whole inventory to narrow it, and the same filter logic would then exist twice
// and drift.
func matchesFilters(c *Certificate, q inventoryQuery) bool {
	if q.Search != "" {
		hay := strings.ToLower(strings.Join([]string{
			c.Domain, c.Host, c.Issuer, c.SerialNumber, c.Fingerprint,
			c.Environment, strings.Join(c.SANs, " "),
		}, " "))
		if !strings.Contains(hay, q.Search) {
			return false
		}
	}
	if q.Status != "" && !strings.EqualFold(c.Status, q.Status) {
		return false
	}
	if q.Source != "" && !strings.EqualFold(c.Source, q.Source) {
		return false
	}
	if q.Issuer != "" && !strings.EqualFold(nonEmpty(c.Issuer, "(unknown)"), q.Issuer) {
		return false
	}
	if q.Environment != "" && !strings.EqualFold(nonEmpty(c.Environment, "unknown"), q.Environment) {
		return false
	}
	switch q.Managed {
	case "managed":
		if c.ManagedBy == "" {
			return false
		}
	case "observed":
		if c.ManagedBy != "" {
			return false
		}
	}
	if q.SelfSigned && !c.SelfSigned {
		return false
	}
	if q.WeakKey && !(c.KeySize > 0 && c.KeySize < 2048) {
		return false
	}
	if q.Stale {
		if c.ManagedBy != "" || c.LastSeenAt.IsZero() ||
			time.Since(c.LastSeenAt) <= StaleAfter {
			return false
		}
	}
	// ExpiringWithin filters on remaining validity. A negative value is not the
	// same as "no filter", so the flag is explicit.
	if q.HasExpiringWithin {
		if c.NotAfter.IsZero() {
			return false
		}
		days := int(time.Until(c.NotAfter).Hours() / 24)
		if days > q.ExpiringWithin {
			return false
		}
	}
	return true
}

type inventoryQuery struct {
	Search            string
	Status            string
	Source            string
	Issuer            string
	Environment       string
	Managed           string
	SelfSigned        bool
	WeakKey           bool
	Stale             bool
	ExpiringWithin    int
	HasExpiringWithin bool
	Sort              string
	Limit             int
}

func parseInventoryQuery(r *http.Request) inventoryQuery {
	v := r.URL.Query()
	q := inventoryQuery{
		Search:      strings.ToLower(strings.TrimSpace(v.Get("search"))),
		Status:      strings.TrimSpace(v.Get("status")),
		Source:      strings.TrimSpace(v.Get("source")),
		Issuer:      strings.TrimSpace(v.Get("issuer")),
		Environment: strings.TrimSpace(v.Get("environment")),
		Managed:     strings.TrimSpace(v.Get("managed")),
		SelfSigned:  v.Get("self_signed") == "true",
		WeakKey:     v.Get("weak_key") == "true",
		Stale:       v.Get("stale") == "true",
		Sort:        strings.TrimSpace(v.Get("sort")),
	}
	if raw := v.Get("expiring_within"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			q.ExpiringWithin = n
			q.HasExpiringWithin = true
		}
	}
	if raw := v.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.Limit = n
		}
	}
	return q
}

// handleInventory returns the filtered inventory plus the facets.
// GET /api/v1/inventory
func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	all := s.inventory.ListCerts()
	q := parseInventoryQuery(r)

	matched := make([]*Certificate, 0, len(all))
	for _, c := range all {
		if matchesFilters(c, q) {
			matched = append(matched, c)
		}
	}

	switch q.Sort {
	case "domain":
		sort.Slice(matched, func(i, j int) bool { return matched[i].Domain < matched[j].Domain })
	case "issuer":
		sort.Slice(matched, func(i, j int) bool { return matched[i].Issuer < matched[j].Issuer })
	case "last_seen":
		sort.Slice(matched, func(i, j int) bool { return matched[i].LastSeenAt.After(matched[j].LastSeenAt) })
	default:
		// Soonest to expire first, which is the order that matters operationally.
		sort.Slice(matched, func(i, j int) bool {
			if matched[i].NotAfter.IsZero() != matched[j].NotAfter.IsZero() {
				return matched[j].NotAfter.IsZero()
			}
			return matched[i].NotAfter.Before(matched[j].NotAfter)
		})
	}

	truncated := false
	if q.Limit > 0 && len(matched) > q.Limit {
		matched = matched[:q.Limit]
		truncated = true
	}

	// The summary is computed over everything, not the filtered subset: the
	// facets are what a caller filters *by*, so narrowing them to the current
	// filter would make the other options disappear as soon as one was applied.
	writeJSON(w, http.StatusOK, map[string]any{
		"certificates": matched,
		"returned":     len(matched),
		"total":        len(all),
		"truncated":    truncated,
		"summary":      buildInventorySummary(all),
		"stale_after":  StaleAfter.String(),
	})
}

// handleGetCertificate returns one record in full.
// GET /api/v1/certificates/{id}
func (s *Server) handleGetCertificate(w http.ResponseWriter, r *http.Request) {
	cert, ok := s.inventory.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate not found")
		return
	}

	// Recompute the derived fields so a single-record fetch agrees with the list,
	// which recomputes them on read.
	view := *cert
	view.Revoked = !view.RevokedAt.IsZero()
	if !view.Revoked && view.Status != "failed" && !view.NotAfter.IsZero() {
		view.Status, view.DaysLeft = statusFor(view.NotAfter)
	}

	stale := view.ManagedBy == "" && !view.LastSeenAt.IsZero() &&
		time.Since(view.LastSeenAt) > StaleAfter

	writeJSON(w, http.StatusOK, map[string]any{
		"certificate": view,
		// Stated rather than left for the caller to infer from managed_by.
		"can_renew":  view.ManagedBy == "acme" && !view.Revoked && view.Status != "failed",
		"can_revoke": view.ManagedBy == "acme" && !view.Revoked && view.SecretRef != "",
		"key_held":   view.SecretRef != "",
		"stale":      stale,
		"note":       "the private key is never returned; secret_ref is its path on the gateway",
	})
}

// handleDeleteCertificate removes a record from the inventory.
// DELETE /api/v1/certificates/{id}?force=true
func (s *Server) handleDeleteCertificate(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "removing an inventory record requires an admin or cert_manager role")
		return
	}

	id := r.PathValue("id")
	unlockCertificate := s.lockCertificateOperation(id)
	defer unlockCertificate()
	cert, ok := s.inventory.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate not found")
		return
	}
	if cert.TenantID != strings.TrimSpace(callerClaims(r).TenantID) &&
		!isGlobalAdmin(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "certificate is not owned by the authenticated tenant")
		return
	}

	force := r.URL.Query().Get("force") == "true"
	live := cert.ManagedBy == "acme" && !cert.Revoked && cert.Status != "failed" &&
		!cert.NotAfter.IsZero() && time.Until(cert.NotAfter) > 0

	// Deleting the record for a certificate that is still valid at the CA loses
	// the only note that it exists, while the certificate carries on being
	// accepted by clients. Revoke it first, or say explicitly that you mean it.
	if live && !force {
		writeErr(w, http.StatusConflict,
			"this certificate is managed here and still valid until "+
				cert.NotAfter.Format(time.RFC3339)+
				". Removing the record does not revoke it: clients would keep accepting it "+
				"with nothing tracking it. Revoke it first, or repeat with ?force=true")
		return
	}

	keyRefs := make(map[string]struct{})
	if cert.SecretRef != "" {
		keyRefs[cert.SecretRef] = struct{}{}
	}
	for _, version := range cert.Versions {
		if version != nil && version.SecretRef != "" {
			keyRefs[version.SecretRef] = struct{}{}
		}
	}
	removedKeys := 0
	removalFailed := false
	for keyRef := range keyRefs {
		if s.filePolicy == nil {
			removalFailed = true
			s.logger.Warn("secure file storage policy is unavailable; private key was retained",
				zap.String("path", keyRef))
			continue
		}
		if err := s.filePolicy.Remove(keyRef, cert.TenantID, securefs.PrivateKey); err == nil {
			removedKeys++
		} else if !errors.Is(err, os.ErrNotExist) {
			removalFailed = true
			s.logger.Warn("could not remove a private key for a deleted certificate lineage",
				zap.String("path", keyRef), zap.Error(err))
		}
	}
	if removalFailed {
		writeErr(w, http.StatusInternalServerError,
			"could not remove every private key in the certificate lineage; the inventory record was retained")
		return
	}

	s.inventory.Delete(id)
	s.recordAudit(r, "inventory.certificate.delete", "certificate", id, cert.Domain,
		audit.ResultSuccess, fmt.Sprintf("removed_keys=%d", removedKeys))
	s.logger.Info("inventory record removed",
		zap.String("id", id), zap.String("domain", cert.Domain),
		zap.Int("keys_removed", removedKeys), zap.String("by", callerName(r)))

	writeJSON(w, http.StatusOK, map[string]any{
		"removed":      id,
		"domain":       cert.Domain,
		"key_removed":  removedKeys > 0,
		"keys_removed": removedKeys,
		"warning": func() string {
			if live {
				return "the certificate is still valid at the CA and was NOT revoked"
			}
			return ""
		}(),
	})
}

// handlePruneInventory removes records that no longer describe anything real.
// POST /api/v1/inventory/prune
func (s *Server) handlePruneInventory(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "pruning the inventory requires an admin or cert_manager role")
		return
	}

	var req struct {
		// Failed removes recorded issuance attempts that never produced a
		// certificate. Stale removes discovered records not seen for StaleDays.
		Failed    bool `json:"failed"`
		Stale     bool `json:"stale"`
		StaleDays int  `json:"stale_days"`
		// DryRun reports what would go without removing anything, so a bulk
		// delete can be checked first.
		DryRun bool `json:"dry_run"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !req.Failed && !req.Stale {
		writeErr(w, http.StatusBadRequest,
			"set failed and/or stale to choose what to prune")
		return
	}

	staleWindow := StaleAfter
	if req.StaleDays > 0 {
		staleWindow = time.Duration(req.StaleDays) * 24 * time.Hour
	}

	type victim struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
		Reason string `json:"reason"`
	}
	victims := []victim{}

	claims := callerClaims(r)
	for _, c := range s.inventory.ListCerts() {
		if c.TenantID != strings.TrimSpace(claims.TenantID) && !isGlobalAdmin(claims.EffectiveRole()) {
			continue
		}
		switch {
		case req.Failed && c.Status == "failed":
			victims = append(victims, victim{c.ID, c.Domain, "issuance never succeeded"})
		case req.Stale && c.ManagedBy == "" && !c.LastSeenAt.IsZero() &&
			time.Since(c.LastSeenAt) > staleWindow:
			victims = append(victims, victim{c.ID, c.Domain,
				"not observed since " + c.LastSeenAt.Format(time.RFC3339)})
		}
	}

	removed := 0
	removalErrors := []string{}
	if !req.DryRun {
		for _, v := range victims {
			if cert, ok := s.inventory.Get(v.ID); ok {
				if s.filePolicy == nil {
					if cert.SecretRef != "" || len(cert.Versions) > 0 {
						removalErrors = append(removalErrors, cert.ID+": secure file storage policy is unavailable")
						continue
					}
				}
				keyRefs := map[string]struct{}{}
				if cert.SecretRef != "" {
					keyRefs[cert.SecretRef] = struct{}{}
				}
				for _, version := range cert.Versions {
					if version != nil && version.SecretRef != "" {
						keyRefs[version.SecretRef] = struct{}{}
					}
				}
				failed := false
				for keyRef := range keyRefs {
					if err := s.filePolicy.Remove(keyRef, cert.TenantID, securefs.PrivateKey); err != nil && !errors.Is(err, os.ErrNotExist) {
						s.logger.Warn("could not remove a private key while pruning",
							zap.String("path", keyRef), zap.Error(err))
						removalErrors = append(removalErrors, cert.ID+": "+err.Error())
						failed = true
					}
				}
				if failed {
					continue
				}
			}
			if s.inventory.Delete(v.ID) {
				removed++
			}
		}
		s.logger.Info("inventory pruned",
			zap.Int("removed", removed), zap.String("by", callerName(r)))
		s.recordAudit(r, "inventory.prune", "inventory", "", "",
			audit.ResultSuccess, fmt.Sprintf("removed=%d failures=%d", removed, len(removalErrors)))
	}

	status := http.StatusOK
	if len(removalErrors) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, map[string]any{
		"dry_run":     req.DryRun,
		"matched":     len(victims),
		"removed":     removed,
		"candidates":  victims,
		"stale_after": staleWindow.String(),
		"errors":      removalErrors,
	})
}

// ---------------------------------------------------------------------------
// Deployment tracking.
//
// The deploy endpoint fires and forgets: a certificate is pushed to a target
// and the result vanishes from memory the moment the HTTP response is written.
// The inventory now records where each managed certificate was deployed so an
// operator can see which endpoint is running which version.
// ---------------------------------------------------------------------------

// RecordDeployment attaches a new deployment attempt to a certificate. It never
// supersedes an existing verified deployment: an attempt is not committed until
// UpdateDeployment records a verified success.
func (i *Inventory) RecordDeployment(certID string, rec *DeploymentRecord) bool {
	if rec == nil {
		return false
	}
	if rec.ID == "" {
		return false
	}
	if rec.AttemptID == "" {
		rec.AttemptID = rec.ID
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = time.Now().UTC()
	}
	if rec.DeployedAt.IsZero() {
		rec.DeployedAt = rec.StartedAt
	}
	rec.BindingID = deploymentBindingID(rec.Target, rec.TargetType, rec.TargetHost, rec.Region)

	i.mu.Lock()
	cert, ok := i.certs[certID]
	if !ok {
		i.mu.Unlock()
		return false
	}
	cert.Deployments = append(cert.Deployments, rec)
	i.mu.Unlock()

	// Deployment attempts are operational evidence, not a cache. Persist the
	// accepted attempt synchronously before apply starts.
	i.FlushState()
	return true
}

// UpdateDeployment replaces an existing attempt with its latest durable state.
// Only a verified committed result supersedes the previous good deployment;
// failed or rolled-back attempts remain evidence without changing current state.
func (i *Inventory) UpdateDeployment(certID, deploymentID string, rec *DeploymentRecord) bool {
	if rec == nil {
		return false
	}

	i.mu.Lock()
	cert, ok := i.certs[certID]
	if !ok {
		i.mu.Unlock()
		return false
	}

	index := -1
	for n, existing := range cert.Deployments {
		if existing.ID == deploymentID {
			index = n
			break
		}
	}
	if index < 0 {
		i.mu.Unlock()
		return false
	}

	rec.BindingID = deploymentBindingID(rec.Target, rec.TargetType, rec.TargetHost, rec.Region)
	if rec.Status == "deployed" && rec.Verified {
		for _, existing := range cert.Deployments {
			if existing == nil || existing.ID == deploymentID {
				continue
			}
			existingBindingID := deploymentBindingID(existing.Target, existing.TargetType, existing.TargetHost, existing.Region)
			existing.BindingID = existingBindingID
			if existingBindingID == rec.BindingID && existing.Status == "deployed" && existing.Verified {
				existing.Status = "superseded"
				existing.Stage = "superseded"
			}
		}
	}
	cert.Deployments[index] = rec
	i.mu.Unlock()

	i.FlushState()
	return true
}

// AllDeployments returns a flat list of every active deployment across all certs.
func (i *Inventory) AllDeployments() []deploymentView {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var out []deploymentView
	for _, c := range i.certs {
		for _, d := range c.Deployments {
			out = append(out, deploymentView{
				CertID:     c.ID,
				Domain:     c.Domain,
				Status:     c.Status,
				DaysLeft:   c.DaysLeft,
				Issuer:     c.Issuer,
				Deployment: d,
			})
		}
	}
	return out
}

type deploymentView struct {
	CertID     string            `json:"cert_id"`
	Domain     string            `json:"domain"`
	Status     string            `json:"cert_status"`
	DaysLeft   int               `json:"days_left"`
	Issuer     string            `json:"issuer"`
	Deployment *DeploymentRecord `json:"deployment"`
}

// handleListDeployments returns all recorded deployments across the inventory.
// GET /api/v1/inventory/deployments
func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	all := s.inventory.AllDeployments()

	// Filter: ?status=deployed|failed|superseded|queued
	if qs := strings.TrimSpace(r.URL.Query().Get("status")); qs != "" {
		filtered := make([]deploymentView, 0, len(all))
		for _, d := range all {
			if strings.EqualFold(d.Deployment.Status, qs) {
				filtered = append(filtered, d)
			}
		}
		all = filtered
	}

	// Sort by most recently deployed first.
	sort.Slice(all, func(i, j int) bool {
		return all[i].Deployment.DeployedAt.After(all[j].Deployment.DeployedAt)
	})

	// Summary counts.
	deployed, failed, superseded, queued, deploying := 0, 0, 0, 0, 0
	for _, d := range all {
		switch d.Deployment.Status {
		case "deployed":
			deployed++
		case "failed":
			failed++
		case "superseded":
			superseded++
		case "queued":
			queued++
		case "deploying":
			deploying++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deployments": all,
		"total":       len(all),
		"summary": map[string]int{
			"deployed":   deployed,
			"failed":     failed,
			"superseded": superseded,
			"queued":     queued,
			"deploying":  deploying,
		},
	})
}

// deploymentOwner returns the certificate that owns an attempt. The caller
// acquires that certificate's operation lock and then revalidates ownership
// before mutation, preventing status edits from racing rollout target selection.
func (i *Inventory) deploymentOwner(deploymentID string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for certID, cert := range i.certs {
		for _, deployment := range cert.Deployments {
			if deployment != nil && deployment.ID == deploymentID {
				return certID, true
			}
		}
	}
	return "", false
}

func (i *Inventory) updateDeploymentStatus(certID, deploymentID, status, message string) (bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	cert, ok := i.certs[certID]
	if !ok {
		return false, nil
	}
	for _, deployment := range cert.Deployments {
		if deployment == nil || deployment.ID != deploymentID {
			continue
		}
		oldStatus, oldMessage := deployment.Status, deployment.Message
		deployment.Status = status
		if message != "" {
			deployment.Message = message
		}
		if err := i.persistLocked(); err != nil {
			deployment.Status, deployment.Message = oldStatus, oldMessage
			return true, err
		}
		return true, nil
	}
	return false, nil
}

// handleUpdateDeploymentStatus allows updating deployment status (e.g. marking
// one as superseded or failed after a manual check).
// POST /api/v1/inventory/deployments/{id}/status
func (s *Server) handleUpdateDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "modifying a deployment status requires an admin or cert_manager role")
		return
	}

	depID := r.PathValue("id")
	if depID == "" {
		writeErr(w, http.StatusBadRequest, "deployment id is required in the path")
		return
	}

	var req struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	switch strings.ToLower(req.Status) {
	case "failed", "superseded", "queued":
	default:
		writeErr(w, http.StatusBadRequest,
			"status must be failed, superseded or queued; only the verified deployment engine can set deployed")
		return
	}

	certID, found := s.inventory.deploymentOwner(depID)
	if !found {
		writeErr(w, http.StatusNotFound, "deployment record not found")
		return
	}
	unlockCertificate := s.lockCertificateOperation(certID)
	defer unlockCertificate()
	found, err := s.inventory.updateDeploymentStatus(certID, depID, strings.ToLower(req.Status), req.Message)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "persisting deployment status failed: "+err.Error())
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "deployment record not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"updated": true, "id": depID, "status": req.Status})
}
