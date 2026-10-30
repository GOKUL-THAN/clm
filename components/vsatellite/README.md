# Component 3: vSatellite (Virtual Satellite Agent)

> ## Status: Implemented
>
> Core agent runtime, concurrent TLS network scanning, local endpoint deployment with verification
> and rollback, configuration templates, and deployment artifacts (Dockerfile, systemd unit, install script)
> are implemented. The agent communicates with the Satellite Gateway coordinator on port `9090` via
> authenticated enrollment, periodic heartbeats, command execution, and result reporting.

---

## Intended Role

Lightweight endpoint agent deployed per VPC, subnet, or data-centre segment. Performs local
scanning and certificate deployment inside a network boundary the Satellite Gateway cannot
reach directly.

The gateway already implements the coordinator side of this relationship — agent registry,
heartbeat handling, command dispatch, and result collection are live on gateway port `9090`.
What is missing is the agent that connects to it.

---

## Intended Responsibilities

| Responsibility | Status |
|----------------|--------|
| CIDR network scanning (TLS probing for certificate discovery) | Planned |
| Certificate deployment to local targets (Linux services, PostgreSQL) | Planned |
| Heartbeat to the Satellite Gateway (with certificate count) | Planned |
| Command execution (`scan_cidr`, `deploy_cert`, `health_check`) | Planned |
| Result reporting back to the gateway | Planned |

A useful shortcut: the gateway already contains working implementations of the two hardest
pieces — `internal/discovery/scanners/network.go` (concurrent TLS scanning) and
`internal/deployment/deployers/` (Linux and PostgreSQL). The agent should extract and reuse
these rather than reimplement them.

---

## Coordinator Protocol (gateway side is live)

The gateway endpoints this agent is expected to call already exist and can be developed against:

```
POST <gateway>:9090/vsatellite/register    { id, name, host, vpc, subnet, cidrs, capabilities }
                                           → { status: "registered", agent_id }

POST <gateway>:9090/vsatellite/heartbeat   { agent_id, cert_count, version }
                                           → { status: "ok", commands: [...] }

POST <gateway>:9090/vsatellite/result      { agent_id, cmd_id, status, result }
                                           → { status: "ok" }

GET  <gateway>:9090/vsatellite/agents      → { agents: [...], total, stats }
```

Heartbeat returns and clears any queued commands, so the agent's main loop is: register, then
heartbeat on an interval, execute whatever commands come back, and report each result.

The gateway marks an agent offline after 2 minutes without a heartbeat
(`Coordinator.MonitorAgents`), so the heartbeat interval must be comfortably below that — 30s
matches the gateway's own connector to the control plane.

---

## Intended Deployment

Outbound-only to the gateway on port `9090`; no inbound ports. Intended packaging: container,
systemd unit, or Kubernetes DaemonSet — none of which exist yet.

---

## Resource Footprint

**Unknown.** Previous revisions of this document stated a binary size and memory range; those
figures were never measured against a build and have been removed. Expect a footprint broadly
similar to the gateway binary, but do not plan capacity until it is built and profiled.

---

## Intended Structure

Directories exist but contain no files:

```
vsatellite/
├── cmd/clm-vsatellite/main.go   # exists (~257 lines)
├── internal/
│   ├── agent/                   # EMPTY — main loop, heartbeat, command handler
│   ├── scanner/                 # EMPTY — extract from gateway network.go
│   └── deployer/                # EMPTY — extract from gateway deployers
├── configs/                     # EMPTY — vsatellite.yaml
└── deploy/                      # EMPTY — Dockerfile, systemd unit, install script
```

---

## Recommended Next Steps

If this component is to be picked up, in order:

| # | Step | Notes |
|---|------|-------|
| 1 | **Decide whether it is still needed** | The gateway can already scan any CIDR it can route to. vSatellite only earns its complexity where network segmentation genuinely blocks the gateway — confirm that case exists before building |
| 2 | Audit `cmd/clm-vsatellite/main.go` | 257 lines exist against empty packages; establish whether it compiles and what it actually references |
| 3 | Extract the scanner and deployers into shared packages | Avoid a second implementation drifting from the gateway's |
| 4 | Implement the agent loop against the live coordinator API | The gateway side needs no changes |
| 5 | Add `configs/vsatellite.yaml` and deploy artifacts | Container + systemd first; DaemonSet later |
| 6 | Replace the resource footprint section with measured numbers | |

Step 1 is the important one. Building this because the architecture diagram has three boxes
would be the wrong reason.
