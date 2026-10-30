package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"go.uber.org/zap"
)

// Background processes: automated discovery, expiry evaluation and renewal.
//
// Discovery, risk assessment and renewal all existed as things a human could
// trigger. None of them ran on their own, which meant the inventory was only as
// fresh as the last time somebody remembered to scan, and `discovery.schedule`
// and `discovery.networks` sat in the config file being parsed and never read.
//
// Three loops run here. Each records its outcome so a job that has been failing
// for a week is visible in the portal rather than only in the logs — a scheduled
// job whose failures are invisible is worse than no job, because it creates
// confidence that something is being handled when it is not.

// JobRun records one execution.
type JobRun struct {
	Job        string        `json:"job"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   string        `json:"duration"`
	DurationMS int64         `json:"duration_ms"`
	OK         bool          `json:"ok"`
	Summary    string        `json:"summary"`
	Errors     []string      `json:"errors,omitempty"`
	Trigger    string        `json:"trigger"` // schedule, manual, startup
}

// SchedulerConfig configures the background loops.
type SchedulerConfig struct {
	DiscoveryEnabled  bool
	DiscoveryInterval time.Duration
	Networks          []string
	Domains           []string
	DNSServer         string
	Enumerate         bool

	MaxNames   int
	ScanACM    bool
	ACMRegions []string

	AlertInterval time.Duration
	AlertEnabled  bool

	RenewInterval time.Duration
	RenewEnabled  bool

	// CA order polling collects asynchronous commercial orders once the CA has
	// finished validating them.
	CAOrderPollEnabled bool
	CAOrderInterval    time.Duration
}

// Scheduler owns the periodic jobs.
type Scheduler struct {
	cfg    SchedulerConfig
	srv    *Server
	logger *zap.Logger

	mu      sync.RWMutex
	history []*JobRun
	running map[string]bool
	next    map[string]time.Time
	// baseCtx is the process lifetime context handed to Start. A manually
	// triggered run derives from it rather than from context.Background(), so
	// shutdown can actually stop it; a detached run was being killed at an
	// arbitrary point instead, which for renewal means a new private key on disk
	// that no inventory record references.
	baseCtx context.Context
}

const maxJobHistory = 100

// NewScheduler wires a scheduler to a server.
func NewScheduler(cfg SchedulerConfig, srv *Server, logger *zap.Logger) *Scheduler {
	if cfg.DiscoveryInterval <= 0 {
		cfg.DiscoveryInterval = 6 * time.Hour
	}
	if cfg.AlertInterval <= 0 {
		cfg.AlertInterval = 1 * time.Hour
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = 12 * time.Hour
	}
	if cfg.CAOrderInterval <= 0 {
		// More frequent than renewal: a DV order can validate within minutes, and
		// leaving a paid certificate uncollected for hours is a poor experience.
		cfg.CAOrderInterval = 15 * time.Minute
	}
	return &Scheduler{
		cfg:     cfg,
		srv:     srv,
		logger:  logger,
		history: make([]*JobRun, 0, 16),
		running: map[string]bool{},
		next:    map[string]time.Time{},
	}
}

// Start launches the loops and returns immediately.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	s.baseCtx = ctx
	s.mu.Unlock()

	if s.cfg.DiscoveryEnabled {
		go s.loop(ctx, "discovery", s.cfg.DiscoveryInterval, s.runDiscovery)
		s.logger.Info("scheduled discovery enabled",
			zap.Duration("interval", s.cfg.DiscoveryInterval),
			zap.Strings("networks", s.cfg.Networks),
			zap.Strings("domains", s.cfg.Domains))
	} else {
		s.logger.Info("scheduled discovery is disabled (set discovery.enabled)")
	}

	if s.cfg.AlertEnabled {
		go s.loop(ctx, "expiry-alerts", s.cfg.AlertInterval, s.runAlerts)
		s.logger.Info("scheduled expiry alerting enabled",
			zap.Duration("interval", s.cfg.AlertInterval))
	}

	if s.cfg.RenewEnabled {
		go s.loop(ctx, "renewal", s.cfg.RenewInterval, s.runRenewals)
		s.logger.Info("scheduled renewal enabled",
			zap.Duration("interval", s.cfg.RenewInterval))
	}

	if s.cfg.CAOrderPollEnabled {
		go s.loop(ctx, "ca-orders", s.cfg.CAOrderInterval, s.runCAOrders)
		s.logger.Info("scheduled CA order polling enabled",
			zap.Duration("interval", s.cfg.CAOrderInterval))
	}
}

// runCAOrders polls commercial CA orders that are still pending.
//
// OV and EV issuance involves a human vetting the organisation at the CA, so an
// order can sit for days. Without this loop a completed order would never be
// collected and the certificate would simply never appear.
func (s *Scheduler) runCAOrders(ctx context.Context) *JobRun {
	run := &JobRun{OK: true}

	checked, completed, errs := s.srv.runCAOrderPoll(ctx)
	if len(errs) > 0 {
		run.OK = false
		run.Errors = errs
	}
	run.Summary = fmt.Sprintf("%d pending CA order(s) polled, %d completed", checked, completed)
	return run
}

// loop runs job on a ticker until the context ends.
//
// The first run is delayed rather than immediate: at startup the gateway is still
// registering with the control plane and loading state, and a discovery sweep
// competing with that makes both slower and startup logs harder to read.
func (s *Scheduler) loop(ctx context.Context, name string, interval time.Duration, job func(context.Context) *JobRun) {
	const startupDelay = 90 * time.Second

	s.setNext(name, time.Now().Add(startupDelay))
	select {
	case <-ctx.Done():
		return
	case <-time.After(startupDelay):
	}

	for {
		s.execute(ctx, name, job, "schedule")
		s.setNext(name, time.Now().Add(interval))

		select {
		case <-ctx.Done():
			s.logger.Info("scheduled job stopping", zap.String("job", name))
			return
		case <-time.After(interval):
		}
	}
}

// lifetime returns the process context, or Background if Start was never called
// (every scheduled job disabled, manual triggers still allowed).
func (s *Scheduler) lifetime() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.baseCtx != nil {
		return s.baseCtx
	}
	return context.Background()
}

// runGuarded runs a job, containing a panic and always clearing the overlap flag.
//
// These jobs run on bare goroutines, where a panic is not contained the way
// net/http contains one in a request handler: it unwinds past the goroutine and
// takes the process down. A single malformed certificate met during a discovery
// sweep, or a nil map in a CA provider, would stop the gateway entirely —
// including the ACME HTTP-01 responder, failing validations that were in flight.
//
// Clearing s.running is deferred for the same reason. It used to be cleared on
// the line after the job returned, so once a panic was recovered anywhere up the
// stack the flag stayed true and that job was skipped, with only a warning, for
// the rest of the process lifetime.
func (s *Scheduler) runGuarded(ctx context.Context, name string, job func(context.Context) *JobRun) (run *JobRun) {
	defer func() {
		s.mu.Lock()
		s.running[name] = false
		s.mu.Unlock()

		if p := recover(); p != nil {
			stack := string(debug.Stack())
			s.logger.Error("scheduled job panicked and was contained",
				zap.String("job", name), zap.Any("panic", p), zap.String("stack", stack))
			run = &JobRun{
				Job: name, OK: false,
				Summary: fmt.Sprintf("panicked: %v", p),
				Errors:  []string{fmt.Sprintf("panic: %v", p)},
			}
		}
	}()
	return job(ctx)
}

// execute runs a job once, guarding against overlap.
func (s *Scheduler) execute(ctx context.Context, name string, job func(context.Context) *JobRun, trigger string) *JobRun {
	s.mu.Lock()
	if s.running[name] {
		s.mu.Unlock()
		// Overlap means the previous run is still going. Skipping is the right
		// call: two concurrent discovery sweeps would double the scan load and
		// race on the same inventory entries.
		s.logger.Warn("skipping scheduled job because the previous run is still in progress",
			zap.String("job", name))
		return &JobRun{Job: name, StartedAt: time.Now(), OK: false,
			Summary: "skipped: previous run still in progress", Trigger: trigger}
	}
	s.running[name] = true
	s.mu.Unlock()

	started := time.Now()
	run := s.runGuarded(ctx, name, job)

	if run == nil {
		run = &JobRun{Job: name, OK: false, Summary: "job returned no result"}
	}
	run.Job = name
	run.StartedAt = started
	run.Trigger = trigger
	run.DurationMS = time.Since(started).Milliseconds()
	run.Duration = time.Since(started).Round(time.Millisecond).String()

	s.record(run)

	level := s.logger.Info
	if !run.OK {
		level = s.logger.Warn
	}
	level("scheduled job finished",
		zap.String("job", name), zap.Bool("ok", run.OK),
		zap.String("summary", run.Summary), zap.String("took", run.Duration))

	return run
}

// runDiscovery sweeps the configured networks and domains.
func (s *Scheduler) runDiscovery(ctx context.Context) *JobRun {
	run := &JobRun{OK: true}
	if s.srv.scanner == nil {
		run.OK = false
		run.Summary = "no network scanner configured"
		return run
	}

	// Bounded well inside the interval so a slow sweep cannot pile up.
	scanCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	found, names := 0, 0

	for _, cidr := range s.cfg.Networks {
		certs, err := s.srv.scanner.ScanNetwork(scanCtx, cidr)
		if err != nil {
			run.Errors = append(run.Errors, cidr+": "+err.Error())
			run.OK = false
			continue
		}
		found += s.srv.recordDiscovered(certs)
	}

	// AWS ACM. Certificates issued by ACM never appear in a TLS probe of an
	// internal range — they live in the AWS control plane and are attached to
	// load balancers — so without this they are simply absent from the inventory.
	if s.cfg.ScanACM {
		acmCerts, err := s.srv.scanACM(scanCtx, s.cfg.ACMRegions)
		if err != nil {
			run.Errors = append(run.Errors, "acm: "+err.Error())
			run.OK = false
		} else {
			found += s.srv.recordDiscovered(acmCerts)
		}
	}

	if len(s.cfg.Domains) > 0 && s.srv.dnsScanner != nil {
		scanner := s.srv.dnsScanner.WithDNSServer(s.cfg.DNSServer)
		for _, domain := range s.cfg.Domains {
			result, err := scanner.ScanDomain(scanCtx, domain, scanners.DNSScanOptions{
				Enumerate:  s.cfg.Enumerate,
				FollowSANs: true,
				MaxNames:   s.cfg.MaxNames,
			})
			if err != nil {
				run.Errors = append(run.Errors, domain+": "+err.Error())
				run.OK = false
				continue
			}
			found += s.srv.recordDiscovered(result.Certs)
			names += len(result.NamesResolved)
		}
	}

	run.Summary = fmt.Sprintf("%d certificate(s) recorded from %d network(s) and %d domain(s); %d name(s) resolved",
		found, len(s.cfg.Networks), len(s.cfg.Domains), names)
	return run
}

// runAlerts evaluates the inventory and delivers notifications.
func (s *Scheduler) runAlerts(ctx context.Context) *JobRun {
	run := &JobRun{OK: true}
	if s.srv.alerter == nil {
		run.OK = false
		run.Summary = "alerting is not configured"
		return run
	}

	evalCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	alerts := s.srv.alerter.Evaluate(evalCtx, s.srv.inventory.ListCerts())

	delivered, suppressed, failed := 0, 0, 0
	for _, a := range alerts {
		switch {
		case a.Delivered:
			delivered++
		case len(a.Channels) == 1 && a.Channels[0] == "suppressed (cooldown)":
			suppressed++
		default:
			failed++
		}
	}
	if failed > 0 {
		run.OK = false
		run.Errors = append(run.Errors,
			fmt.Sprintf("%d alert(s) could not be delivered to any channel", failed))
	}

	run.Summary = fmt.Sprintf("%d condition(s) raised: %d delivered, %d suppressed by cooldown, %d undeliverable",
		len(alerts), delivered, suppressed, failed)
	return run
}

// runRenewals reissues managed certificates approaching expiry.
func (s *Scheduler) runRenewals(ctx context.Context) *JobRun {
	run := &JobRun{OK: true}
	if s.srv.acme == nil {
		run.OK = false
		run.Summary = "ACME is not configured, so nothing can be renewed automatically"
		return run
	}

	window := s.srv.acme.RenewBefore()
	due := make([]*Certificate, 0)
	for _, c := range s.srv.inventory.ListCerts() {
		// Only certificates this gateway issued can be renewed: anything merely
		// discovered has no private key here.
		if c.ManagedBy != "acme" || !c.RevokedAt.IsZero() || c.NotAfter.IsZero() {
			continue
		}
		if time.Until(c.NotAfter) <= window {
			due = append(due, c)
		}
	}

	renewed := 0
	for _, cert := range due {
		renewCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		err := s.srv.renewCertificate(renewCtx, cert)
		cancel()
		if err != nil {
			run.Errors = append(run.Errors, cert.Domain+": "+err.Error())
			run.OK = false
			continue
		}
		renewed++
	}

	run.Summary = fmt.Sprintf("%d managed certificate(s) within %d day(s) of expiry, %d renewed",
		len(due), int(window.Hours()/24), renewed)
	return run
}

func (s *Scheduler) record(run *JobRun) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, run)
	if len(s.history) > maxJobHistory {
		s.history = s.history[len(s.history)-maxJobHistory:]
	}
}

func (s *Scheduler) setNext(job string, at time.Time) {
	s.mu.Lock()
	s.next[job] = at
	s.mu.Unlock()
}

// Status describes the jobs for the portal.
func (s *Scheduler) Status() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := make([]*JobRun, len(s.history))
	copy(history, s.history)
	sort.Slice(history, func(i, j int) bool { return history[i].StartedAt.After(history[j].StartedAt) })

	last := map[string]*JobRun{}
	for _, r := range history {
		if _, seen := last[r.Job]; !seen {
			last[r.Job] = r
		}
	}

	jobs := []map[string]any{}
	add := func(name string, enabled bool, interval time.Duration, detail string) {
		entry := map[string]any{
			"job": name, "enabled": enabled,
			"interval": interval.String(), "detail": detail,
			"running": s.running[name],
		}
		if t, ok := s.next[name]; ok && enabled {
			entry["next_run"] = t
		}
		if r, ok := last[name]; ok {
			entry["last_run"] = r
		}
		jobs = append(jobs, entry)
	}

	add("discovery", s.cfg.DiscoveryEnabled, s.cfg.DiscoveryInterval,
		fmt.Sprintf("%d network(s), %d domain(s)", len(s.cfg.Networks), len(s.cfg.Domains)))
	add("expiry-alerts", s.cfg.AlertEnabled, s.cfg.AlertInterval, "evaluates the inventory and notifies")
	add("renewal", s.cfg.RenewEnabled, s.cfg.RenewInterval, "reissues managed certificates near expiry")
	add("ca-orders", s.cfg.CAOrderPollEnabled, s.cfg.CAOrderInterval,
		"collects asynchronous commercial CA orders once validated")

	if len(history) > 20 {
		history = history[:20]
	}
	return map[string]any{
		"jobs":    jobs,
		"history": history,
	}
}

// --- HTTP handlers ---

// handleSchedulerStatus reports the background jobs and their recent runs.
// GET /api/v1/scheduler
func (s *Server) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"jobs": []any{}, "history": []any{},
			"reason": "no scheduler is running on this gateway",
		})
		return
	}
	writeJSON(w, http.StatusOK, s.scheduler.Status())
}

// handleRunJob triggers a scheduled job immediately.
// POST /api/v1/scheduler/run/{job}
func (s *Server) handleRunJob(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeErr(w, http.StatusServiceUnavailable, "no scheduler is running on this gateway")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "running a job on demand requires an admin role")
		return
	}

	name := r.PathValue("job")
	var job func(context.Context) *JobRun
	switch name {
	case "discovery":
		job = s.scheduler.runDiscovery
	case "expiry-alerts":
		job = s.scheduler.runAlerts
	case "renewal":
		job = s.scheduler.runRenewals
	case "ca-orders":
		job = s.scheduler.runCAOrders
	default:
		writeErr(w, http.StatusBadRequest,
			"job must be discovery, expiry-alerts, renewal or ca-orders")
		return
	}

	// Detached from the request so a long sweep is not cancelled if the browser
	// gives up, but rooted in the process lifetime so shutdown can stop it, and
	// still bounded.
	ctx, cancel := context.WithTimeout(s.scheduler.lifetime(), 25*time.Minute)
	go func() {
		defer cancel()
		s.scheduler.execute(ctx, name, job, "manual")
	}()

	s.logger.Info("scheduled job triggered manually",
		zap.String("job", name), zap.String("by", callerName(r)))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":     name,
		"started": true,
		"note":    "running in the background; poll GET /api/v1/scheduler for the result",
	})
}
