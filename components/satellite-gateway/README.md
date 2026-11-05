# Component 2: Satellite Gateway

## Role

Per-client gateway deployed in the customer's cloud or data centre. Bridges private infrastructure to the SaaS control plane, and owns everything certificate-related — private keys are generated and stored here and never leave.

One gateway per location. The gateway always dials the control plane; the control plane never opens a connection to it.

---

## Status Legend

| Status | Meaning |
|--------|---------|
| **Done** | Implemented and verified against the live test gateway |
| **In-memory** | Works, but state is lost on restart (no persistence layer) |
| **Code Ready** | Code exists but is not wired up or not exercised against a live target |
| **Planned** | Not implemented |

---

## Capabilities

| Capability | Status | Notes |
|------------|--------|-------|
| Gateway Portal REST API (`:8080`) | Done | Backs the Gateway Portal UI; validates Keycloak RS256 and internal HS256 tokens |
| vSatellite coordinator API (`:9090`) | Done | Token-gated enrolment, authenticated heartbeat, command dispatch, result reporting |
| vSatellite deployment engine | Partial | Single-use enrolment and per-agent secrets exist, but certificate execution is disabled until Release 2 adds durable lease/ack/dedup and key-local execution |
| SaaS connector | Code Ready | Outbound HTTPS client loads a server CA and client certificate/key, verifies server names, sends a unique bearer credential, and refuses HTTP or insecure TLS in production. Live mTLS/PKI validation has not been performed |
| Durable SaaS command execution | Code Ready | Claims are acked before execution, leases renew during work, command timeouts/full-envelope result limits apply, final outcomes persist under `state.dir`, duplicates replay the saved outcome by command ID + execution generation (not delivery attempt), confirmed outcomes are cleaned up, unknown types fail without execution, and result transport retries network failures |
| Certificate discovery — network | Done | Concurrent TLS handshake probing across CIDRs and hosts |
| Certificate discovery — DNS | Done | Record walking, subdomain enumeration and SNI-correct probing by name |
| Certificate discovery — scheduled | Done | Background sweep of the configured networks and domains; `discovery.enabled` + `discovery.interval` |
| Certificate lifecycle assessment | Done | Graded expiry risk, hygiene findings, per-issuer exposure and a prioritised action list |
| Durable state | Done | Atomic JSON snapshots under `state.dir`; inventory, approvals and fleet survive restart |
| Certificate issuance — ACME | Done | HTTP-01 remains supported; provider-neutral DNS-01 supports wildcard and DNS-selected multi-SAN orders. RFC2136 and AWS Route 53 are live adapters; Azure DNS, GCP Cloud DNS, and OCI DNS remain planned |
| Renewal — automated | Code Ready | Scheduler/manual renewal creates immutable versions, replays sanitized active bindings, requires verified endpoint results before commit, compensates partial rollout with the prior version, and exposes explicit rollback recovery after indeterminate restart. Live ACME/provider rollout validation remains outstanding |
| Revocation | Done | Signed with the certificate's own key, so only certificates issued here |
| Expiry monitoring & alerting | Done | Webhook and Slack channels, graded severity, cooldown with escalation override |
| Compliance reporting & export | Done | Six scored controls, weighted grade, CSV and JSON export |
| Certificate deployment — Linux | Done | Release 1 verified apply, strict verified-only commit, injected verification failure, automatic rollback, restored file hash, durable stages and restart persistence on the test gateway |
| Certificate deployment — PostgreSQL | Code Ready | Adapter builds and rollback errors propagate; no live PostgreSQL target was exercised in Release 1 |
| Crypto engine | Done | Key generation, CSR, chain assembly (RSA 2048/4096, ECDSA P-256/P-384) |
| Certificate inventory | Done | Persisted to disk; statuses recomputed on read |
| Approval records | Done | Persisted; a decision is written synchronously |
| Certificate deployment — AWS ACM + ELB | Code Ready | Listener-aware verification and rollback exist; real ACM/ELB mutation remains unverified because the test role lacks permissions |
| Certificate deployment — GCP load balancer | Code Ready | Compute SSL certificate + proxy swap exists; no live project validation and rollback is unsupported |
| Certificate deployment — Azure Key Vault | Code Ready | Key Vault import exists; no live tenant validation and rollback is unsupported |
| Per-service deployment credentials | Done | Schema-validated `deploy` Credential Sets use canonical target fields, durable atomic writes, write-only values, conflict-safe item GET/PUT, kind/service enforcement, and transactionally guarded endpoint references/deletion. Saved endpoints use the same canonical target schema with durable, conflict-safe POST-create/PUT-update writes. Deploy requests accept `credential_ref` only. Snapshot files are mode `0600` but are not encrypted at rest |
| Choosing which vSatellite executes | Planned | Disabled in Release 1; the API returns 501 and does not queue certificate/private-key material until authenticated lease/ack and key-local execution exist |
| Certificate discovery — AWS ACM | Done | `POST /api/v1/discover/acm` and the scheduled sweep when `discovery.scan_acm` is set |
| Secrets storage — AWS Secrets Manager | Done | Only provider implemented; the config switch falls back to AWS for every value |
| Secrets storage — GCP SM / Azure KV / Vault | **Planned** | No providers exist |
| Multi-vendor CA integration | Done | ACME, GoDaddy and Namecheap behind one interface; routing by account, capability enforcement, async orders. Verified against a protocol mock, **not** against the live vendor APIs |
| CA pricing / quoting | Partial | Namecheap pricing is read live; GoDaddy exposes no pricing endpoint, so it returns nothing rather than invented numbers |
| Multi-level approval engine (L1/L2/L3) | **Planned** | Only a flat pending/decided model exists |
| Audit trail | Done | Append-only JSONL event log for every mutating action, with an in-memory query index and CSV/JSONL export. Portal has a dedicated Audit Log page. 12 unit tests under `-race`; verified live (entries appear after real actions, correct actor recorded, viewer refused query access) and in a real browser |
| Certificate policy engine | Done | Rule-based enforcement at issuance time — min key size, max validity, allowed CAs/cert types per environment, wildcard allow/deny, max SAN count, denied domain patterns, and require-approval-for-cert-types (auto-creates a pending approval instead of a hard block). Portal has a dedicated Certificate Policy page with a dry-run evaluator. 25 unit tests under `-race`; verified live (blocks a real issuance with violations, approval flow completes issuance) and in a real browser |

### Release 1 — Deploy Engine correctness and durability

**Deployment scope:** existing test gateway `3.7.21.28` only. The SaaS host was not modified. This release does **not** make the gateway or CLM platform production-ready.

Implemented and measured:

- `POST /api/v1/deploy` accepts a gateway-managed `cert_id` and optional target `credential_ref`; inline `cert_pem`, `key_pem`, and target credential maps are rejected with HTTP 400.
- Deployment success is committed only when the adapter returns `success=true` and verification returns `verified=true`. Unverified apply returns HTTP 502.
- Every attempt is persisted before apply and carries attempt/correlation IDs plus stage evidence for accepted, apply, verify, rollback, and commit.
- Failed verification automatically invokes rollback and records `rollback_attempted`, `rollback_success`, and `rollback_error` in response, inventory, state, logs, and audit.
- Existing same-target deployment is superseded only after a new verified commit; a failed attempt leaves the prior verified record active.
- The portal never labels an unverified result deployed and shows verification, rollback, attempt, and correlation evidence.
- vSatellite certificate execution is deliberately disabled with HTTP 501 in this release; the old HTTP command queue no longer receives certificate/private-key payloads.

Live evidence:

| Check | Result |
|---|---|
| Verified apply | Correlation `release1-success-20260815`; HTTP 200; `success=true`, `verified=true`; stage `committed` |
| Verification failure | Correlation `release1-failure-20260815`; HTTP 502; `success=false`; stage `rolled_back` |
| Automatic rollback | Attempted and completed; certificate hash after rollback matched the pre-attempt hash |
| Supersession safety | After the failed attempt: `deployed=1`, `failed=1`, `superseded=0` |
| Persistence | Both attempt records and both audit events remained after gateway restart |
| Authorization | Viewer deploy returned 403; operator positive control succeeded |
| Secret boundary | Inline PEM/credentials returned 400; no private-key markers found in deployment API output, logs, audit, or inventory state |
| Validation | Go format, build, binary build, vet, existing race-enabled tests, JS syntax, health, live API/UI asset, and restart checks passed |

Rollback source backup: `/opt/clm-gateway/backups/release1-20260815T082500Z`.

At the Release 1 evidence cutoff, the following were still incomplete: serving-listener fingerprint/application-health verification for Linux/PostgreSQL, independent recovery verification after rollback, real AWS/GCP/Azure mutation tests, signed SaaS commands, automatic renewal redeployment, vSatellite coordinator lease/ack execution, live mTLS/PKI validation, encrypted deployment-credential storage, HA, CI/security scans, and external audit retention.

**Task 4 code update (offline-validated):** renewal now retains immutable certificate versions and version-specific key files, replays sanitized active deployment targets, commits the candidate only after every deployment reports independent verification, and compensates partial failure by redeploying and verifying the previous version. In-progress work found after restart is marked `recovery_required`; `POST /api/v1/certificates/{id}/renew/recover` deterministically redeploys the last committed version rather than guessing that the candidate is safe. AWS Route 53 and RFC2136 are selectable DNS-01 adapters; Azure DNS, GCP Cloud DNS, and OCI DNS still fail closed as planned. Canonical staged `go test`, `go vet`, and build pass, but no live ACME order, Route 53 mutation, or multi-endpoint renewal rollout has been claimed.

> Earlier revisions of this document listed approval, purchasing, monitoring, and renewal as implemented subsystems with dedicated packages. Those packages do not exist. The table above reflects the code.

---

## Ports & Request Flow

| Port | Exposed by | Purpose |
|------|-----------|---------|
| `80` | nginx | Gateway Portal SPA + reverse proxy (the only port a browser uses) |
| `8080` | `clm-satellite` | Gateway Portal REST API |
| `9090` | `clm-satellite` | vSatellite agent API |

nginx routes:

| Path | Target | Purpose |
|------|--------|---------|
| `/` | static files | Gateway Portal SPA |
| `/api/` | `clm-gateway:8080` | Gateway's own REST API |
| `/health` | `clm-gateway:8080` | Liveness |
| `/saas-api/` | `http://35.154.6.28/api/` | SaaS control plane API, proxied to stay same-origin |
| `/saas-auth/` | `http://35.154.6.28/auth/` | SaaS Keycloak, proxied so the portal can obtain a token same-origin |

The two `saas-*` proxies exist because direct cross-origin requests to the control plane are blocked by the corporate proxy even with correct CORS headers.

---

## Authentication

The portal uses the **Keycloak Direct Grant (password) flow** against the SaaS realm, reached through the same-origin `/saas-auth/` proxy. The Keycloak JavaScript adapter is deliberately not used: the CDN build is blocked by the corporate proxy, and Keycloak 24's bundled `/auth/js/keycloak.js` is a Node UMD module that cannot run in a browser.

The API accepts two token types:

| Type | Verification |
|------|-------------|
| Keycloak RS256 | Fetched from the realm JWKS (`CLM_JWKS_URL`, defaults to `<saas_endpoint>/auth/realms/clm/protocol/openid-connect/certs`), cached with a 1-minute refresh floor on unknown key IDs |
| Internal HS256 | Shared secret via `CLM_JWT_SECRET` |

Roles are read from the token's top-level `roles` claim (this realm's shape) or `realm_access.roles`, with Keycloak's internal roles filtered out. Because both portals use the same realm, a token issued to `clm-admin-portal` is accepted here too — that is intentional SSO.

Authorisation on write paths:

| Operation | Allowed roles |
|-----------|--------------|
| Approve / reject | super_admin, admin, tenant_admin, approver, cert_manager |
| Deploy certificate | the above, plus operator |
| Request certificate, discovery (network/DNS/ACM), CA quote | the above, plus operator — **fixed 2026-08**; a viewer token could previously reach all five |
| Audit query / export | super_admin, admin, tenant_admin, cert_manager (`canManageFleet`) |
| Policy read / write / evaluate | super_admin, admin, tenant_admin, cert_manager (`canManageFleet`) |
| Read endpoints | any authenticated user |

### Fixed: five mutating and fan-out routes had no role gate (2026-08)

Live-probing the viewer role found that `handleRequestCertificate`, `handleDiscover`, `handleDiscoverDNS`, `handleDiscoverACM` and `handleCAQuote` never called an authorization check at all — any authenticated viewer token could request a certificate, kick off a network/DNS/ACM scan, or fan out a pricing request to every configured CA vendor. `handleCAQuote` was the sharpest edge: it made an outbound authenticated call to every vendor with no rate limit, so a viewer account could be used to hammer GoDaddy or Namecheap's API. All five now require `canApprove(role) || role == "operator"`, matching the gate already on `handleDiscoverACM` and deploy. Verified live with viewer-token probes (all five return `403`) and a positive control confirming an operator token still succeeds.

### Two login failures worth knowing about

**1. The login overlay would not go away (fixed).** Sign-in appeared to do nothing: the form stayed on screen no matter which valid credentials were used. Authentication was in fact succeeding — the token request returned `200`, the session was stored and the dashboard rendered underneath — but the overlay never disappeared, so nothing looked like it happened.

The cause was CSS specificity, not auth. `index.html` declared:

```css
.hidden { display: none; }                        /* declared first  */
.login-redirect { ...; display: flex; ... }       /* declared second */
```

Both are single-class selectors, so their specificity is identical and **source order decides the winner — the later rule**. `.login-redirect` came second, so its `display: flex` always beat `.hidden`, and `classList.add('hidden')` in `onAuthenticated()` had no visual effect at all: the element ended up with `class="login-redirect hidden"` while still computing `display: flex`, `position: fixed`, `inset: 0`, `z-index: 1000`.

`.hidden` is now `display: none !important`, which is the right call for a utility class: it must beat whatever `display` a component sets, regardless of declaration order. The Admin Portal was never affected — there `.hidden` happens to be declared *after* `.login-overlay`, which is exactly how fragile this kind of ordering dependency is.

Worth noting for diagnosis: every server-side check passed while this bug was live — the token endpoint, the gateway API and the JWKS validation all returned `200`. Only driving the page in a real browser and reading the *computed* style exposed it.

**2. "Account is not fully set up".** Keycloak refuses a Direct Grant when the account has a pending required action, or when a required profile attribute is missing — the realm requires `firstName` and `lastName`, and that check produces a `VERIFY_PROFILE` action that never appears in `requiredActions`. The portal now surfaces Keycloak's actual message instead of the fallback's generic "Invalid credentials", and an administrator can repair the account with **Fix Login** in Identity Management. See the SaaS Control Plane README for the full account of that one.

JWT payloads are decoded with a base64url-safe helper. A bare `atob()` throws on `-`/`_` and on missing padding, and `isTokenExpired()` treats a decode failure as expired — so an unlucky token would have signed the user out on the next page load.

---

## API Reference

`/health` and `/api/v1/health` are unauthenticated so nginx and uptime checks can use them. Everything else requires `Authorization: Bearer <token>`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health`, `/api/v1/health` | Liveness: id, name, version, uptime |
| GET | `/api/v1/status` | Dashboard summary: cert counts, expiring, pending approvals, agent stats |
| GET | `/api/v1/agents` | Registered vSatellite agents + coordinator stats |
| GET | `/api/v1/certificates` | Certificate inventory with recomputed status and stats |
| POST | `/api/v1/certificates` | Record a certificate request — returns `202` with `status: pending`; does **not** issue |
| GET | `/api/v1/inventory` | Filtered inventory plus facets. See the query parameters below |
| GET | `/api/v1/certificates/{id}` | One record in full, with `can_renew` / `can_revoke` / `key_held` stated |
| DELETE | `/api/v1/certificates/{id}` | Remove a record. `?force=true` required while a managed certificate is still valid |
| POST | `/api/v1/inventory/prune` | Bulk remove failed attempts and/or stale discovered records; supports `dry_run` |
| GET | `/api/v1/approvals/pending` | Approvals awaiting a decision |
| POST | `/api/v1/approvals/{id}/approve` | Approve (role-gated) |
| POST | `/api/v1/approvals/{id}/reject` | Reject (role-gated) |
| POST | `/api/v1/discover` | Real TLS scan of `cidrs[]`, `hosts[]` and/or `domains[]`; folds results into the inventory. 4-minute timeout |
| POST | `/api/v1/discover/dns` | DNS-based discovery: record walking, optional subdomain enumeration, SNI-correct probing |
| POST | `/api/v1/discover/acm` | AWS ACM discovery across regions; needs `acm:ListCertificates` and `acm:DescribeCertificate` |
| GET | `/api/v1/lifecycle` | Lifecycle assessment: risk bands, hygiene findings, per-issuer exposure, prioritised action list |
| POST | `/api/v1/certificates/issue` | Issue over ACME using HTTP-01 or configured DNS-01 (admin/cert_manager) |
| POST | `/api/v1/certificates/{id}/renew` | Issue a candidate, replay active deployment bindings, verify every target, then commit |
| POST | `/api/v1/certificates/{id}/renew/recover` | For `recovery_required`, redeploy and verify the last committed version on every active binding; never promotes the candidate implicitly |
| POST | `/api/v1/certificates/{id}/revoke` | Revoke at the CA; optional `reason` |
| GET | `/api/v1/acme/status` | ACME directory, account state, renewal window |
| GET | `/api/v1/scheduler` | Background jobs, next run, last result per job |
| POST | `/api/v1/scheduler/run/{job}` | Run `discovery`, `expiry-alerts` or `renewal` now |
| GET | `/api/v1/alerts` | Alert history and delivery stats |
| POST | `/api/v1/alerts/test` | Deliver a synthetic alert to prove a channel |
| GET | `/api/v1/reports/compliance` | Scored controls, weighted grade, breakdowns, caveats |
| GET | `/api/v1/reports/export` | `format=csv\|json`; no keys, no chains |
| GET | `/.well-known/acme-challenge/{token}` | HTTP-01 responder. Unauthenticated by necessity; 404 for unknown tokens |
| GET | `/api/v1/vsatellites` | Fleet with per-agent queue depth, plus outstanding enrolments |
| POST | `/api/v1/vsatellites/enroll` | Issue a single-use enrolment token and install command (admin roles) |
| GET | `/api/v1/vsatellites/install.sh?token=` | The generated installer. Authenticated by the enrolment token, not a JWT |
| POST | `/api/v1/vsatellites/{id}/command` | Queue supported `scan_cidr` and `health_check` commands; certificate deployment is disabled until tenant authorization and authenticated deployment transport are available |
| DELETE | `/api/v1/vsatellites/{id}` | Decommission: revokes the agent secret and drops its queue |
| DELETE | `/api/v1/vsatellites/enrollments/{token}` | Revoke an unused enrolment |
| POST | `/api/v1/deploy` | Deploy a gateway-managed certificate by `cert_id`; target credentials are resolved by optional `credential_ref`. Inline PEM/keys and inline credentials are rejected. Returns 200 only after verification; failed verification returns 502 with rollback evidence |
| GET | `/api/v1/deploy/targets` | Target types with their fields. Release 1 exposes only the gateway executor; vSatellite execution is disabled until Release 2 |
| GET | `/api/v1/deploy/endpoints` | List durable saved deployment endpoints and their Credential Set references |
| POST | `/api/v1/deploy/endpoints` | Create a schema-validated saved endpoint; duplicate names and unsupported properties are rejected |
| PUT | `/api/v1/deploy/endpoints/{id}` | Conflict-safe saved-endpoint update requiring the previously read `updated_at` |
| DELETE | `/api/v1/deploy/endpoints/{id}` | Remove a saved endpoint |
| GET | `/api/v1/ca/vendors` | Supported CA vendors and the credential fields each needs |
| POST | `/api/v1/ca/accounts` | Register a schema-validated CA account; duplicate names and unknown credential fields are rejected |
| PUT | `/api/v1/ca/accounts/{name}` | Conflict-safe CA credential update requiring `updated_at`; blank values retain stored fields and `remove_keys` clears optional fields |
| POST | `/api/v1/ca/accounts/{name}/test` | Make a real authenticated call to prove the credentials |
| POST | `/api/v1/ca/accounts/{name}/default` | Move the default account |
| DELETE | `/api/v1/ca/accounts/{name}` | Remove an account and its credentials |
| GET | `/api/v1/credentials` | List deployment Credential Set metadata; every value is write-only/masked and saved-endpoint usage references are included. CA credential metadata is never exposed here |
| POST | `/api/v1/credentials` | Create a schema-validated deployment Credential Set; duplicate names, unsupported services/keys and incomplete required fields are rejected |
| GET | `/api/v1/credentials/{name}` | Get one deployment Credential Set with every value masked |
| PUT | `/api/v1/credentials/{name}` | Conflict-safe update requiring the previously read `updated_at`; blank fields retain values and `remove_keys` explicitly removes optional values |
| DELETE | `/api/v1/credentials/{name}` | Remove an unreferenced deployment Credential Set; in-use sets and CA-owned credentials are rejected |
| POST | `/api/v1/purchases/quote` | Deprecated alias of `/api/v1/ca/quote`, kept for existing callers. Delegates to the same registry and adds the old `ca`/`total_price` field names alongside the real ones |
| GET | `/api/v1/ca/providers` | Configured CA accounts and their capabilities |
| POST | `/api/v1/ca/quote` | Live pricing from every account whose API exposes it |
| GET | `/api/v1/ca/orders` | Orders still waiting on a CA, with the validation steps |
| POST | `/api/v1/ca/orders/{id}/refresh` | Re-read one order from the CA now |
| GET | `/api/v1/audit` | Query the audit trail: filter by `actor`, `action`, `target_type`, `result`, `since`, `until` (role-gated) |
| GET | `/api/v1/audit/export` | `format=csv\|jsonl`; streams the full log, not just the in-memory query window |
| GET | `/api/v1/policies` | List configured certificate policies |
| POST | `/api/v1/policies` | Create or update a policy (role-gated) |
| DELETE | `/api/v1/policies/{id}` | Remove a policy (role-gated) |
| POST | `/api/v1/policies/evaluate` | Dry-run: evaluate a hypothetical request against all policies without issuing anything, returns the violation list |

vSatellite agent API on `:9090` (agent credentials, not a user JWT): `/vsatellite/enroll` (alias `/vsatellite/register`), `/vsatellite/heartbeat`, `/vsatellite/result`. Every route authenticates — see below.

### DNS certificate discovery

A CIDR sweep dials addresses, which misses two common cases. Name-based virtual hosts choose their certificate from the SNI value in the handshake, so dialling the bare address only ever returns the default certificate. And anything behind a CDN, a load balancer or a third-party SaaS resolves outside every internal range, so a sweep never reaches it.

`POST /api/v1/discover/dns` starts from a domain instead:

1. **Zone records** — `CNAME`, `MX`, `NS` and a list of `SRV` service names are read for hostnames actually published in the zone.
2. **Subdomain enumeration** (`enumerate: true`) — a built-in wordlist of 221 base names weighted toward infrastructure that terminates TLS in an enterprise estate rather than generic web names, **expanded up to `max_names` (default and maximum 2000)**.
3. **Wildcard guard** — two improbable labels are resolved first. If both answer, the zone is wildcard and enumeration is skipped, because every guess would appear to resolve and the results would be noise.
4. **SNI probing** — each resolved name is dialled at its resolved address with `ServerName` set to the name.
5. **SAN expansion** (`follow_sans: true`) — names in the certificates found are re-probed, restricted to the target domain so a shared certificate naming hundreds of unrelated hosts does not widen the scan.

`dns_server` sends every lookup to a specific resolver, which is how an internal zone is scanned from the gateway; `CLM_DNS_SERVER` sets the default.

#### Enumeration breadth

`max_names` caps candidate names per domain. The default and the hard ceiling are both **2000**; a larger request is clamped and the response says so, because every candidate costs a DNS query and an unbounded scan would run past the handler timeout and return nothing at all.

The 221-entry wordlist alone could never reach that, so it is expanded with the name shapes that actually recur in an estate — numbered instances (`api1`, `api2`) and environment-qualified forms (`api-prod`, `dev-api`, `k8s-dev`) — rather than random character mutations, which cost the same query and almost never resolve. Plain base names are emitted first, so a small `max_names` still spends its budget on the highest-value candidates.

Measured against a real non-wildcard zone: `max_names=200` tried 200 names in 5s and resolved 19; `max_names=2000` tried 2000 in **33s** and resolved 21, the two extra being `ad.` and `k8s-dev.` — both produced by the expansion, not the base list. `dns_concurrency` (default 64) is what keeps a full-width scan inside the budget; at the previous 25 the worst case ran past four minutes.

A wildcard zone skips enumeration entirely regardless of `max_names`, since every guess would appear to resolve.

Measured against `badssl.com`, one address (`104.154.89.105`) returned **three different certificates** depending on the SNI value. A network scan of that address reports one.

Zone transfer (AXFR) is not attempted; it needs a DNS library the module does not carry, and enumeration plus record walking covers the same ground without one.

### Centralized inventory

The certificate list was a flat seven-column table with no search, no filters and no way to open a single record. Discovery had been collecting fingerprints, serial numbers, SANs, key sizes and the method each certificate was found by, and none of it was viewable — gathered, then hidden. Nothing could be removed either, so failed issuance attempts and hosts that no longer exist accumulated permanently.

`GET /api/v1/inventory` returns the filtered set plus **facets** — the values actually present, with counts — which is what the portal builds its dropdowns from, so the options can never disagree with the data. Filtering is server-side; doing it in the browser would mean downloading the whole inventory to narrow it, and the same filter logic would then exist twice and drift.

| Parameter | Effect |
|-----------|--------|
| `search` | Substring across domain, host, issuer, serial, fingerprint, environment and SANs |
| `status`, `source`, `issuer`, `environment` | Exact match on a facet value |
| `managed` | `managed` (key held here) or `observed` (found on the network) |
| `expiring_within` | Days of remaining validity, inclusive |
| `self_signed`, `weak_key`, `stale` | Boolean narrowing |
| `sort` | `domain`, `issuer`, `last_seen`; default is soonest expiry |
| `limit` | Cap the rows; the response sets `truncated` |

The facets are always computed over the **whole** inventory rather than the filtered subset. Narrowing them to the current filter would make every other option disappear as soon as one was applied, which makes a filter bar impossible to use.

**Staleness.** `last_seen_at` is refreshed each time discovery observes a certificate again. A discovered record unseen for more than 30 days is flagged stale: usually the host is gone or has stopped serving it. Managed certificates are excluded from that check — the gateway owns them, so not having seen one on the wire says nothing about whether it exists.

**Removal is guarded.** Deleting the record for a managed certificate that is still valid is refused with `409` unless `?force=true`, because removing the record does not revoke anything: clients would keep accepting the certificate with nothing tracking it. Revoking first is the right order. When a record is removed its private key is deleted too — an orphaned key with no record pointing at it is worse than no key, since nothing will ever rotate or revoke it.

**Pruning** removes failed issuance attempts and stale discovered records, with `dry_run` to see the list first.

### Certificate deployment

Before Release 1, the deploy endpoint accepted certificate/private-key material and a free-form target. It could return HTTP 200 when adapter verification failed, did not automatically roll back, and recorded only a final summary after execution.

Release 1 makes the gateway-local boundary explicit: callers send `cert_id`, target metadata, and optional `credential_ref`. The gateway resolves certificate/key material and credentials locally, persists an attempt before apply, runs the adapter, verifies, and either commits or attempts rollback. Provider/file write success alone is not enough for API success.

`GET /api/v1/deploy/targets` returns every target type with its fields, so the portal builds the form and the API can reject a request that could not have worked:

| Target | Category | Credentials | Rollback |
|--------|----------|-------------|----------|
| `linux` | os | SSH user + private key (only for a remote host) | yes |
| `postgresql` | data | SSH user + private key | yes |
| `aws-acm` | cloud | access key/secret/session token, an assumed role, or the instance role | yes (listener) |
| `gcp-lb` | cloud | service account JSON, project ID | no |
| `azure-keyvault` | cloud | tenant, client ID, client secret, vault name | no |

Required fields are checked before anything reaches a vendor and reported together — `gcp-lb requires: service_account_json`, `azure-keyvault requires: tenant_id, client_id, client_secret` — rather than one per attempt.

**Credentials never cross the deployment API inline and never leak back.** `target.credentials` is rejected. `credential_ref` names a stored set, which is resolved inside the gateway and stripped from the `DeployResult` target before response, logging, audit, or persistence. A set records the service it belongs to, and using an Azure set against a GCP target is refused before dispatch.

Credential registration is still an HTTP API and the stored JSON is only mode `0600`, not encrypted. Release 1 closes private-key and credential transfer on the deploy call; TLS and a KMS/Vault-backed write-only credential backend remain P0 work.

#### AWS: choosing ACM, the load balancer, or both

Importing a certificate into ACM and putting it in front of traffic on a load balancer are two separate decisions, and rotating one already on a listener is a third. The deployer collapsed all three into one path — import into ACM, then, only if `target.host` happened to hold a listener ARN, call `ModifyListener` with a single certificate — and got two of them wrong:

- `resource_id` was passed as `ImportCertificate`'s `CertificateArn` while the schema documents that field as the **listener** ARN, so a request built from the schema asked ACM to re-import over a load balancer ARN.
- `ModifyListener` sets only the listener's **default** certificate and detaches the one it replaces, so there was no way to keep the outgoing certificate attached and therefore no way back except deploying again.
- The access key, secret and session token the schema advertises were **never read**. Every deployment silently used the gateway's own instance role.

The three decisions are now separate fields:

| Field | Values | Meaning |
|-------|--------|---------|
| `apply_to` | `acm`, `alb`, `acm+alb` (default) | `acm` imports and stops — nothing serves it yet. `alb` attaches an existing `certificate_arn` and imports nothing. `acm+alb` does both. |
| `version_policy` | `new-version` (default), `replace-in-place` | `new-version` imports alongside, so the old ARN keeps working and a rollback is possible. `replace-in-place` re-imports over `certificate_arn`, so consumers need no repointing — but the old material is gone. |
| `listener_policy` | `keep-previous` (default), `replace`, `add-only` | `keep-previous` makes the new certificate the default and leaves the outgoing one attached, so reverting is one call. `replace` leaves only the new certificate. `add-only` adds to the SNI list without touching the default. |
| `retain_versions` | integer, default `2` | Superseded CLM-imported certificates for the same name beyond this many are deleted after a successful deploy. `0` keeps every version. |

`listener_arn` and `certificate_arn` are now separate fields rather than both being crammed into `resource_id`. The old spellings are still read (`resource_id`, then `host`) because requests exist in both shapes, and an ACM ARN found in the listener field is treated as the certificate rather than being sent to ELBv2 to fail obscurely.

Cross-field requirements cannot be expressed in a static field schema, so they are checked before any AWS call and named precisely: `apply_to "alb" needs listener_arn`, `apply_to "alb" attaches an existing certificate, so certificate_arn is required; use "acm+alb" to import this certificate first`, `version_policy "replace-in-place" re-imports over an existing certificate, so certificate_arn is required`. Unknown enum values are rejected with the valid set rather than silently defaulting.

**On keeping the old and the new attached to one listener:** when two certificates cover the same name, which one a client gets is **not defined by AWS**. `keep-previous` exists so a revert is a single `ModifyListener` call, not so both serve traffic simultaneously. This is stated in the target's notes in the API response too, because it is the kind of thing that looks like a feature and behaves like a coin toss.

Retention never deletes a certificate this gateway did not import (it checks its own `clm-managed` tag) and never deletes one AWS still reports in `InUseBy`, since the reason to keep old versions is precisely that something still references them. The selection logic is a pure function with the AWS calls kept out of it, and it is unit tested — deleting the wrong certificate is not a thing to discover in production.

Credentials resolve in order: static access key and secret when both are given (supplying only one is refused rather than silently ignored), then `role_arn` assumed on top for a cross-account deploy, then the gateway's ambient role.

Verification is listener-aware. It used to ask ACM whether a certificate existed and was `ISSUED`, which is true of every successful import whether or not anything serves it — so it reported success for a deployment that changed nothing about the traffic path. It now also confirms the certificate is attached to the listener and, unless the policy was `add-only`, that it is the default.

Rollback used to call `DeleteCertificate`, which is not a rollback: it destroyed a certificate and left the listener pointing at the one being rolled back, unrecoverably for imported material because the private key cannot be read back out of AWS. It now promotes the newest other issued CLM-managed certificate on the listener back to default, and when there is nothing to revert to it says so and names the policy that would have kept one.

Two AWS quirks the live test surfaced: `CertificateChain` must be **absent** rather than empty (ACM rejects a zero-length chain as a validation error, and a self-signed or root-issued certificate legitimately has none), and `Certificate` must be a single certificate — callers routinely pass a fullchain, and the deploy handler sets `cert_pem` and `chain_pem` to the same fullchain for a gateway-issued certificate, so the leaf is split out and any copy of it is filtered from the chain.

**Verified:** the option schema as served, every cross-field rejection, enum validation, credential-pairing rejection, PEM leaf/chain splitting, retention selection across seven cases including in-use skipping and listing-order independence, and that the portal renders all three selectors and sends them (with `retain_versions` as a number). 26 test cases pass under `-race`. **Not verified:** the ACM import, listener attach, retention delete and rollback against real ACM and ELBv2 — the test gateway runs on a Lightsail instance role with no ACM permissions, so requests reach AWS and return `AccessDeniedException`. That proves credential resolution and payload shaping; it does not prove the mutations.

#### GCP and Azure

Both are implemented with the standard library. Pulling in the Google or Azure SDK for a handful of REST calls would add a large dependency tree to a binary that is otherwise stdlib plus the AWS SDK.

- **GCP** signs a service account JWT with RS256 and exchanges it for an access token, then creates a Compute SSL certificate and repoints the target HTTPS proxy. A Compute SSL certificate is **immutable**, so rotation is always create-then-swap; the previous resource is left in place because deleting one a proxy still references breaks the listener.
- **Azure** uses the client credentials flow — no JWT signing needed — then imports into Key Vault. Key Vault is the right target because it is the one place a certificate can be put once and consumed by App Service, Application Gateway and Front Door. The import policy sets issuer `Unknown` so Key Vault does not try to renew the certificate itself and fight with this gateway's renewal, and the token is requested for the **Key Vault audience** rather than the ARM one, which is a token that otherwise authenticates and is then refused by the vault.

Neither implements rollback, and both say so rather than silently doing nothing: reverting means repointing at a previous resource or version, which the gateway does not record.

#### vSatellite execution status

Release 1 exposes only `gateway` from `GET /api/v1/deploy/targets`. Supplying a vSatellite executor returns HTTP 501 before certificate material is resolved. The prior path embedded `cert_pem` and `key_pem` in the coordinator's HTTP command queue even though the shipped agent did not execute them; that path is disabled rather than being reported as queued or complete.

Release 2 must add authenticated scoped commands, durable lease/renew/ack, deduplication/idempotency, capability and assignment enforcement, encrypted local spool, key-local material resolution, result signatures, and mTLS before the vSatellite executor can be enabled.

### Multi-vendor certificate authorities

Issuance used to be one hardcoded ACME directory, and the two fields that accepted a CA choice — `ca_provider` on a certificate request and `ca` on the issue endpoint — were parsed and then **silently discarded**. Asking for DigiCert produced a Let's Encrypt certificate with no warning, which is worse than refusing the request.

Every CA now sits behind one interface (`internal/satellite/ca`), ACME included, and accounts are registered by name so several can be configured at once:

| Account | Vendor | Types | Wildcard | Issuance | Pricing API | DCV |
|---------|--------|-------|----------|----------|-------------|-----|
| `acme` | RFC 8555 (Let's Encrypt, ZeroSSL, step-ca, ...) | DV | with live DNS-01 | immediate | no | HTTP-01, DNS-01 when configured |
| `godaddy` | GoDaddy Certificates API | DV, OV, EV | yes | asynchronous | no | HTTP, DNS, email |
| `namecheap` | Namecheap SSL API | DV, OV, EV | yes | asynchronous | yes | HTTP, DNS, email |

Routing is by **account name**, not vendor, so a sandbox and a production account of the same vendor can coexist. An unknown name is refused with the list of what exists:

```
POST /certificates/issue {"ca_provider":"digicert"}
  -> 400  unknown CA "digicert"; configured accounts are: acme, godaddy, namecheap
```

Capabilities are enforced before anything reaches a vendor, so a request that cannot succeed fails immediately and says why — `account "acme" issues dv certificates; it cannot issue ov`, `account "acme" cannot issue wildcard certificates`. OV and EV require an `organization` object, checked in the handler so a missing one is a `400` rather than a vendor error.

**Credentials come from the environment only.** A CA API key in a config file ends up in a backup, a container image or a support bundle, and none of those should carry the ability to buy certificates. GoDaddy defaults to the **OTE sandbox** and Namecheap to **sandbox**: a production default would let a misconfiguration spend money.

| Variable | Purpose |
|----------|---------|
| `CLM_GODADDY_KEY`, `CLM_GODADDY_SECRET` | GoDaddy API credentials |
| `CLM_GODADDY_ENV` | `ote` (default) or `production` |
| `CLM_GODADDY_SHOPPER_ID` | only for reseller keys acting for a customer |
| `CLM_NAMECHEAP_API_USER`, `CLM_NAMECHEAP_API_KEY` | Namecheap credentials |
| `CLM_NAMECHEAP_CLIENT_IP` | **must be allow-listed on the account** or every call is rejected |
| `CLM_NAMECHEAP_SANDBOX` | `false` for production |

#### Registering a vendor from the portal

Accounts could only come from environment variables read once at startup, so onboarding a CA meant editing a compose file and restarting the gateway. `POST /api/v1/ca/accounts` registers one at runtime, and `GET /api/v1/ca/vendors` returns each vendor's credential fields **as data** — which is why the portal renders GoDaddy's key/secret and Namecheap's IP allow-list field without containing any vendor-specific code.

Credentials go to the credential store, never into the config file, and no read path returns any credential value: stored fields are write-only and represented only by key presence. `PUT /api/v1/ca/accounts/{name}` requires the previously read `credential_updated_at`, so a stale form cannot overwrite a newer rotation. Blank fields retain stored values; explicit `remove_keys` entries clear optional fields.

Registration only proves the fields parse, so **`POST /api/v1/ca/accounts/{name}/test` makes a real authenticated call**. Credentials that are merely well-formed are the usual cause of a first order failing hours later. For a vendor with pricing that is a quote request — the cheapest authenticated round trip that spends nothing; for the others it is a deliberately non-existent order lookup, where an auth failure and an unknown order are told apart rather than both reported as broken.

Accounts registered this way are rebuilt from stored credentials at startup, after the environment-configured ones so an explicitly configured account keeps priority for the default. A stored account that no longer builds is logged and skipped: one bad account must not stop the gateway from starting.

#### Asynchronous orders

ACME returns a certificate inside the request. A commercial OV or EV order does not: a human at the CA validates the organisation, which takes hours to days. So `pending` is a normal successful outcome, not a failure:

```
POST /certificates/issue {"ca_account":"godaddy",...}  -> 202 Accepted
  order_status: pending, order_id: gd-order-0001
  validation: [{type: dns, name: _godaddy-verify.mock.example.com, value: gd-dcv-token-abc123}]
```

The response carries the **domain control validation steps**, because without them a pending order can never progress and the operator has nothing to act on. The `ca-orders` scheduler job then polls every pending order every 15 minutes and completes the record when the CA issues; `GET /api/v1/ca/orders` shows what is outstanding and for how long.

Two deliberate refusals:

- **Renewal of a commercial certificate is refused.** At a commercial CA renewal means buying another certificate, and doing that from a button labelled "renew" would spend money without the operator intending to. The error points at the explicit order path.
- **Namecheap purchase and activation are separate calls** (`ssl.create` then `ssl.activate`). If activation fails the credit has already been bought, so the error says so and gives the certificate ID to retry against — otherwise an operator retries and pays twice.

#### The two vendor APIs are shaped very differently

This is what the interface earns its keep for. GoDaddy is REST/JSON with an `Authorization: sso-key KEY:SECRET` header — not a bearer token. Namecheap is a single endpoint taking a `Command` parameter, credentials as **form parameters** rather than headers, XML responses, and **failures arrive as HTTP 200 with `Status="ERROR"`** in the body, so checking the status code alone would treat every rejection as a success.

Two Go-specific traps were found by testing rather than by reading:

- `encoding/xml` cannot combine a nested path with an attribute in one tag. `xml:"SSLActivateResult>ID,attr"` is invalid and makes `Unmarshal` fail for the **whole document**, so every field came back empty — indistinguishable from a CA that returned no validation instructions. The nesting is now spelled out as separate structs, and the decode error is reported instead of ignored.
- The Namecheap IP allow-list error (`1011150`) is the most common first-time failure, so it is annotated with the actual client IP and the fix rather than passed through raw.

#### What is verified, and what is not

Verified end to end against a **mock that speaks both vendors' real protocols**, asserting on what the gateway sends rather than only on what it accepts back:

- GoDaddy: order creation with a PEM CSR and the correct `productType`/`period`, `sso-key` auth shaping, status polling, chain download, `domainVerifications` parsed into DCV instructions, revocation with a valid reason enum. Five API calls, all with the right method and path.
- Namecheap: `users.getPricing` parsed into comparable quotes, `ssl.create` then `ssl.activate` with the CSR and `CNAME_CSR_HASH` DCV, `ssl.getInfo` collecting the issued chain, `ssl.revokecertificate`. Credentials confirmed to travel as form parameters.
- Chain parsing exercised with a **real** certificate generated by openssl, because with fake PEM the parse fails silently and the dates come from order metadata instead — the parsing path would never have been tested.
- Error paths: missing credentials (account not registered, then `unknown CA`), non-allow-listed Namecheap IP, OV without organisation details, unknown CA name, capability violations.

**Not verified: the real GoDaddy and Namecheap APIs.** No vendor credentials were available, so field names, product identifiers and state strings come from the published API documentation and have not been exercised against a live account. The parts most likely to need adjustment on first contact with a real sandbox are the product names (`goDaddyProductFor`, `namecheapProductName`), Namecheap's `CertificateType` on revoke, and the exact DCV element names. Get an OTE key and a Namecheap sandbox key and this can be confirmed in an afternoon; until then treat both providers as **integration-complete but unproven against the vendor**.

#### One page, not two

The portal had a **CA Providers** page and a separate **CA Purchasing** page. Buying a certificate always begins by choosing an account, so the split forced operators between two entries for one task — and the two pages disagreed. Purchasing called the older `/api/v1/purchases/quote`, which answered *"No CA accounts are configured on this gateway"* while the providers page listed four of them.

They are now a single **Certificate Authorities** page with three tabs over one pricing source:

| Tab | Contents |
|-----|----------|
| Accounts | The account table with test / set-default / edit / remove, and the vendor registration form |
| Pricing & Purchase | Live quotes from every account with a pricing API, each row offering **Order** |
| Orders | Pending commercial orders with their DCV instructions and a per-order re-check |

Two details worth recording:

- **The old `purchasing` route still works** and opens the merged page on the Pricing tab. Deleting it would break saved links for no benefit.
- **Wildcard is chosen through the domain, not a separate product type.** The old form offered `DV / OV / EV / Wildcard` as one dropdown, which conflated validation level with name scope — a wildcard OV certificate was not expressible. The merged form keeps validation level on its own and adds a wildcard toggle that rewrites the domain to `*.example.com`, matching how the CAs actually infer wildcard cover (`ca.HasWildcard` reads the names).

`/api/v1/purchases/quote` was left in place for existing callers but no longer reports an emptiness that is not there: it delegates to the same registry as `/api/v1/ca/quote` and emits the old `ca` / `total_price` keys next to the real ones, plus a `deprecated` field. Its free-text `validity` ("1y", "24m") is read as whole years, defaulting to 1 so a vendor is never asked for a zero-year certificate.

### Automated issuance, renewal and revocation

`POST /api/v1/certificates` records an intent and issues nothing — it always did. Actual issuance is `POST /api/v1/certificates/issue`, which runs a full ACME order using `golang.org/x/crypto/acme` (already in the module, so no new dependency).

HTTP-01 remains the automatic default for ordinary names, and nginx continues to forward `/.well-known/acme-challenge/` unauthenticated and un-redirected. DNS-01 is selected automatically for wildcard authorizations when a live provider is configured; setting `acme.challenge_type: dns-01` uses DNS-01 for every authorization in a multi-SAN order. Each TXT value is polled until visible, bounded by `propagation_timeout`, and removed on success, provider/CA error, timeout, or request cancellation. Cleanup is separately bounded and detached from request cancellation.

The provider contract is dependency-light. RFC2136 is live through the host's `nsupdate`. AWS Route 53 is live through the pinned AWS SDK and accepts only the default workload identity chain or `aws-profile://<name>`; raw AWS credentials are rejected. Within one gateway/provider instance its serialized read/merge/write sequence preserves other TXT values and cleanup removes only the challenge value owned by that authorization. Route 53 offers no cross-writer compare-and-swap, so `_acme-challenge` must have a single CLM writer; external writers or another gateway can still race an UPSERT. Azure DNS, GCP Cloud DNS, and OCI DNS are accurately reported as **planned** and cannot be selected. DNS credentials are references and metadata, never raw inventory values. RFC2136 requires a `file-ref://` path so the TSIG key stays outside config resolution and is read directly by `nsupdate`:

```yaml
acme:
  enabled: true
  challenge_type: auto # auto | http-01 | dns-01
  dns01:
    provider: rfc2136
    zone: example.com
    nameserver: 192.0.2.53:53
    credential_ref: file-ref:///etc/clm/dns/acme-nsupdate.key
    resolvers: [192.0.2.53:53]
    ttl: 60
    poll_interval: 2s
    propagation_timeout: 2m
    cleanup_timeout: 15s
```

Route 53 uses the hosted-zone ID in `zone` (not a domain name):

```yaml
acme:
  enabled: true
  challenge_type: dns-01
  dns01:
    provider: aws-route53
    zone: Z0123456789EXAMPLE
    credential_ref: workload-identity://default # or aws-profile://test
    resolvers: [1.1.1.1:53, 8.8.8.8:53]
```

A renewal requires durable `state.dir`. Before issuance, the gateway checkpoints the attempt and current active version. The issued candidate gets a unique key filename, remains non-active while every previously verified binding is replayed, and becomes active only after all endpoint checks pass. If one binding fails, earlier candidate deployments are compensated in reverse order with the prior version and independently verified. A restart or non-durable commit outcome becomes `recovery_required`; the recovery API always restores the last committed version to all active bindings. Deployment snapshots retain public target metadata and `credential_ref`, never resolved credentials or private-key bytes.

Production rejects `acme.insecure_skip_verify: true` both while validating startup config and while constructing runtime-added ACME accounts.

Only certificates issued here can be renewed or revoked. A discovered certificate has no private key on this gateway, so there is nothing to renew with and nothing to sign a revocation — the API says so explicitly instead of returning a generic error, and the portal shows those rows as *report only*.

**Private keys never leave the gateway.** They are written to `<state.dir>/keys/<id>.key` at mode `0600` and referenced by path; they are absent from the inventory, from every API response, from the JSON snapshots and from the CSV export. The key is written only after issuance succeeds, so a failed order leaves no orphan. A failed attempt is recorded in the inventory with its reason, because an issuance failure that exists only in the logs will not be noticed.

The default directory is Let's Encrypt **staging**. Production has strict rate limits and issues publicly trusted certificates, so using it has to be a deliberate choice. `acme.accept_tos` must be set explicitly — agreeing to a CA's terms on someone's behalf silently would be wrong.

Two behaviours worth knowing, both found by testing against a real CA:

- **Asynchronous finalization.** A CA may accept the CSR, move the order to `processing`, and publish the certificate URL only on a later poll. The immediate fetch then has no URL and fails while issuance is actually succeeding, so the client polls the order and fetches explicitly instead of treating that as an error.
- **`Order.URI` is only populated by `AuthorizeOrder`.** It comes from the response's `Location` header, which later polls do not carry. Re-reading it from a refreshed order yields an empty string and any subsequent request silently posts to `""`. The order URL is kept separately.

### Background processes

Discovery, risk assessment and renewal all existed as things a human could trigger. None ran on their own, and `discovery.schedule` / `discovery.networks` sat in the config being parsed and never read. Three loops now run:

| Job | Default | What it does |
|-----|---------|--------------|
| `discovery` | `discovery.interval`, 6h | Sweeps the configured networks and domains, and ACM when `discovery.scan_acm` is set |
| `expiry-alerts` | `alerting.interval`, 1h | Evaluates the inventory and delivers notifications |
| `renewal` | `acme.renew_interval`, 12h | Reissues managed certificates inside the renewal window |

Each records its outcome, and `GET /api/v1/scheduler` exposes the last run per job including failures — a scheduled job whose failures are invisible is worse than no job, because it creates confidence that something is handled when it is not. Overlapping runs are skipped rather than queued: two concurrent sweeps would double the scan load and race on the same inventory entries. The first run is delayed 90s so a sweep does not compete with startup. `POST /api/v1/scheduler/run/{job}` runs one now.

### Proactive alerting

Severity follows the expiry bands, plus optional self-signed and weak-key conditions. Delivery is to a generic JSON webhook and/or a Slack incoming webhook; both can be set, and an alert counts as delivered if any channel accepts it. A failure is recorded on the alert with its reason, because a silently dropped alert is worse than none — the absence of a message reads as "nothing is wrong".

Repeats are suppressed by a per-certificate, per-kind cooldown. Without it a 15-minute evaluation would send the same "expires in 20 days" message 96 times a day and the channel would be filtered within a week. **Escalation bypasses the cooldown**: warning → critical is new information and must not be swallowed by a cooldown started while the certificate was merely a warning.

### Reporting and compliance

`GET /api/v1/reports/compliance` scores six controls the gateway can actually evidence from what it holds, weighted by severity, and grades the result. Inventing a control that cannot be evidenced would raise the score without meaning anything.

Revoked certificates and failed issuance attempts are **excluded** from scoring — one has been dealt with, the other never existed — and the report states how many were excluded and why. Controls with nothing to check are marked not-applicable rather than counted as perfect, and an empty inventory gets no grade rather than an A. `GET /api/v1/reports/export?format=csv|json` exports the inventory; neither the private key nor the chain is included, since an export is the artefact most likely to be emailed around.

### Audit trail

Nothing recorded who did what. An approval, a revocation, a CA account change and a credential deletion all left the same trace: none. That is not an auditable system — a customer or an auditor asking "who issued this wildcard certificate and when" had no answer beyond whatever happened to be in the container logs.

`internal/audit` is an append-only event log, deliberately **not** built on the same pattern as `state.Store` or the credential store. Those are small, mutable config that gets rewritten in full on every change, which is the right shape for a CA account list. An audit trail must never be rewritable — a store that overwrites its own history on every write is the wrong data structure for evidence. So the log is:

- **Append-only JSONL on disk** (`<state.dir>/audit/`) — each event is one line, opened for append, never rewritten or truncated.
- **An in-memory ring buffer** for the query API, so `GET /api/v1/audit` does not re-read the file on every request. Export (`GET /api/v1/audit/export`) streams the file directly instead, so it is not limited to the ring buffer's window.

Every mutating handler that changes state now writes an event: certificate issue/renew/revoke/request, approval approve/reject, discovery (network/DNS/ACM), deployment, CA account register/remove/set-default, and credential store/delete. Each event records the actor (email or username from the token), their role, the action, the target type and ID, the result (`success`/`failure`/`denied`), and action-specific detail.

Query supports filtering by `actor`, `action`, `target_type`, `result`, and a `since`/`until` time range. Access is role-gated via `canManageFleet` (super_admin, admin, tenant_admin, cert_manager) — an audit trail that any authenticated viewer can read is itself an information leak (who requested what certificate, when, and from where).

**Verified:** 12 unit tests under `-race`. Live: an audit entry appears with the correct actor immediately after a real issuance; a viewer token is refused query access (`403`); CSV and JSONL export both return real rows. Browser-verified: the Audit Log portal page renders the table, the actor/action/target/result/since/until filters round-trip against the live API, and both export buttons trigger a download.

### Certificate policy engine

Compliance reporting (above) grades what already happened. Nothing stopped a non-compliant certificate from being issued in the first place — a wildcard certificate in a locked-down environment, a 4096-day validity period, or an OV certificate issued without anyone signing off would all succeed silently and only show up later as a lower compliance score.

`internal/policy` is a rule engine evaluated **before** `provider.Issue` is called, inside `handleCAIssue`, not a post-hoc check. A policy can constrain:

- Minimum key size (RSA and ECDSA checked separately)
- Maximum validity period in days
- Allowed CA accounts and allowed certificate types, per environment
- Wildcard allow/deny
- Maximum SAN count
- Denied domain patterns
- Certificate types that require approval before issuance, regardless of the other rules

A violation is returned as a **list**, not a single error string, so the UI renders every failed rule rather than only the first one found. `require_approval_for_cert_types` is handled differently from every other rule: instead of a hard `403` block, it auto-creates a pending `Approval` (extended with `Domain`/`CertType`/`Environment`/`CAAccount` fields for exact-match lookup) and returns `202 awaiting_approval`. Once approved, resubmitting the same request finds the approval via `Inventory.HasApprovedIssuance()` and proceeds to the CA layer. Treating `require_approval` the same as a hard block would make the approval workflow pointless — nothing could ever get approved into existence.

`POST /api/v1/policies/evaluate` runs the same engine as a dry run, with no side effects, so an operator can check a hypothetical request against every configured policy before submitting it for real.

Policies are persisted through the same `state.Store`-backed mechanism as everything else in `internal/state`, and restored at startup.

**Verified:** 25 unit tests under `-race`. Live: a wildcard request against a policy-restricted account is blocked with a `403` and the specific violation listed; `require_approval_for_cert_types` creates a pending approval (`status: awaiting_approval`), and approving it then resubmitting the same request reaches the CA and fully issues. Browser-verified: the Certificate Policy portal page supports add/edit/remove, the dry-run evaluator reports violations against a real policy, and the Issue Certificate flow shows "Blocked by certificate policy" inline with the violation list when a request is rejected.

### Durable state

Everything the gateway tracked lived in Go maps, so a restart wiped the inventory, every approval decision, and the whole vSatellite fleet including agent credentials — meaning every agent had to be re-enrolled by hand. `state.dir` enables JSON snapshots written to a temporary file and renamed over the target, with an `fsync` before the rename so a crash cannot leave a truncated file that fails to parse. Writes are debounced, because a discovery scan upserts hundreds of certificates in a burst; an approval decision is written synchronously, being the least recoverable record here. A corrupt snapshot is set aside and startup continues rather than failing.

Agent secrets are stored as SHA-256 hashes, never in the clear, so a snapshot cannot be replayed as a credential. Enrolment tokens are keyed by hash for the same reason. A restored agent is marked offline until it heartbeats, rather than claimed as online.

### Certificate lifecycle assessment

`GET /api/v1/lifecycle` answers "what needs doing first", which the certificate list does not. It is derived from the in-memory inventory, so it needs no extra scan.

- **Graded expiry** — `expired`, `critical` (≤7d), `warning` (≤30d), `notice` (≤60d), `healthy`, and `unknown` for records with no expiry. A single 30-day cliff treats a 5-day and a 29-day certificate as the same problem; they are not.
- **Hygiene** — self-signed, unknown issuer, missing SANs, and validity longer than 398 days, which public CAs have not issued since 2020 and so marks a private or pre-2020 certificate.
- **Per-issuer exposure** — totals and at-risk counts per CA, which is what surfaces an unexpected issuer or everything concentrated on one CA.
- **Action list** — every certificate with a problem, ordered by urgency, each with the specific issues and a recommended action. `renewal_queue` is the subset that expiry alone puts at risk.

All array fields are emitted as `[]` rather than `null` when empty, so clients need no null guard.

### vSatellite deployment engine

Deployment is **pull-based**: the gateway issues a single-use enrolment token and serves an installer that the target fetches itself. That works through NAT and outbound-only firewalls — the normal shape of the segments a vSatellite exists to reach — and the gateway never holds SSH credentials for the estate.

1. An operator creates an enrolment (`POST /api/v1/vsatellites/enroll`) with a name, optional VPC/subnet, the CIDRs the agent will own, and a token lifetime (24h default). The response contains a one-line install command; the token is shown **only** at creation.
2. The target host runs `curl -fsSL '<install_url>' | sudo bash`. The script detects the platform by **package manager** rather than distribution name — `apt-get`, `dnf`, `yum`, `zypper`, `apk` — so derivatives work without a per-distro branch. It installs dependencies, checks it can reach the gateway before installing anything, enrols, and registers a systemd unit (or an OpenRC service on Alpine).
3. Enrolment exchanges the short-lived token for a **long-lived per-agent secret**, stored `0600` root-only. Every later heartbeat and result call presents it as `X-Agent-Secret`.
4. The agent appears in the portal within ~30s. Commands are queued by the portal and collected on the next heartbeat.
5. Decommissioning revokes the secret and drops the queue, so a removed agent cannot collect work.

Enrolment tokens are single use, expire, and can be revoked while unused. `install.sh` verifies the token before emitting anything, so an invalid token cannot even read the script.

---

## Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `CLM_SATELLITE_CONFIG` | `configs/satellite.yaml` | Config path |
| `CLM_SATELLITE_TOKEN` | — | Bearer token presented to the SaaS control plane |
| `CLM_JWT_SECRET` | — | HS256 secret for internal tokens |
| `CLM_GATEWAY_API_ADDR` | `:8080` | Portal API listen address |
| `CLM_JWKS_URL` | derived from `saas_endpoint` | Keycloak JWKS for RS256 verification |

Key `satellite.yaml` fields: `id`, `name`, `env`, `saas_endpoint`, `connector_mode`, `tls_cert_path`, `tls_key_path`, `tls_ca_path`, `tls_server_name`, `insecure_skip_verify`, `state.dir`, `commands.{poll_interval,timeout,lease_renewal,max_result_bytes,network_retries}`, `acme.challenge_type`, `acme.dns01.{provider,zone,nameserver,credential_ref,resolvers,ttl,poll_interval,propagation_timeout,cleanup_timeout}`, `secrets.provider`/`region`/`prefix`, `discovery.ports`/`concurrency`/`timeout`, and `crypto.default_algorithm`.

The production host installer does not register or self-sign a satellite. Register through the authenticated control-plane API first, then run `components/shared/scripts/install-satellite.sh` with the returned UUID and one-time credential plus a CA-issued client certificate/key and server CA bundle. The installer validates UUID/key/SAN binding, writes durable state and command bounds, rejects unsupported proxy/offline modes, and never calls a bootstrap-token activation route.

---

## Directory Structure

```
satellite-gateway/
├── cmd/clm-satellite/main.go        # Entry: coordinator API, portal API, runtime, connector
├── internal/
│   ├── api/                         # Gateway Portal REST API (:8080)
│   │   ├── auth.go                  #   dual RS256 (JWKS) + HS256 validation, role extraction
│   │   ├── server.go                #   Go 1.22 ServeMux routing, middleware, JSON helpers
│   │   ├── handlers.go              #   health, status, agents, certs, approvals, discover, deploy, quote
│   │   ├── audit_handlers.go        #   GET/POST /audit, /audit/export
│   │   ├── policy_handlers.go       #   GET/POST/DELETE /policies, /policies/evaluate
│   │   └── inventory.go             #   in-memory certificate + approval store
│   ├── audit/audit.go               # Append-only JSONL event log + in-memory ring buffer for query
│   ├── policy/policy.go             # Policy rule engine, evaluated before every issuance
│   ├── connector/                   # SaaS outbound connector (heartbeat + command pull)
│   ├── crypto/                      # Key generation, CSR, chain assembly
│   ├── deployment/
│   │   ├── executor.go              #   deployer registry + sequential/parallel dispatch
│   │   └── deployers/
│   │       ├── os/linux.go          #   registered
│   │       ├── data/postgresql.go   #   registered
│   │       └── cloud/aws_acm.go     #   NOT registered in main.go
│   ├── discovery/scanners/
│   │   ├── network.go               #   registered — concurrent TLS probing
│   │   └── cloud/acm.go             #   not wired into the discovery endpoint
│   ├── runtime/                     # Satellite runtime loop + auth helpers
│   ├── secrets/providers/aws.go     # AWS Secrets Manager (only provider)
│   └── vsatellite-coordinator/      # Agent registry, dispatch, health monitor
├── configs/                         # satellite.yaml, satellite-test.yaml
├── web/gateway-ui/                  # Portal SPA (index.html + app.js, no build step)
├── nginx.conf                       # Reverse proxy: SPA, /api, /saas-api, /saas-auth
└── deploy/
    ├── Dockerfile.satellite         # Multi-stage build with monorepo module-path rewrite
    └── build-src-bundle.sh          # Produces the deployable source bundle — see below
```

---

## Building & Deploying

**Always use `deploy/build-src-bundle.sh`.** The gateway is developed inside a monorepo but compiled as a single Go module on the host, which requires two transformations that a plain `tar` of this directory will not perform:

1. `internal/` is re-rooted under `internal/satellite/` to match the `github.com/m2p/clm/internal/satellite/...` import paths — `runtime/*.go` moves up to `internal/satellite/`, and `vsatellite-coordinator` is renamed `vsatellite` so the directory matches its package name.
2. `internal/core/interfaces.go` and `pkg/` are copied in from the control plane, which owns them.

`Dockerfile.satellite` then rewrites the module path with `sed` before `go mod tidy`. Removing that line breaks the build.

```bash
bash deploy/build-src-bundle.sh /tmp/clm-gateway-src.tar.gz
scp -i ~/.ssh/clm-test-lightsail.pem /tmp/clm-gateway-src.tar.gz ubuntu@3.7.21.28:/tmp/
ssh -i ~/.ssh/clm-test-lightsail.pem ubuntu@3.7.21.28 \
  "cd /opt/clm-gateway && sudo rm -rf cmd internal pkg go.mod go.sum \
   && sudo tar xzf /tmp/clm-gateway-src.tar.gz -C /opt/clm-gateway \
   && sudo chown -R ubuntu:ubuntu . \
   && sudo docker compose build clm-gateway && sudo docker compose up -d"
```

> **Do not `rm -rf web/` or `nginx.conf` on the host.** Both are bind-mounted into the nginx container; deleting the directory breaks the mount and nginx then returns **500 for every static file**. Extract in place, or follow with `docker compose up -d --force-recreate nginx`.

**UI-only changes need no image rebuild.** `docker-compose.yml` bind-mounts `web/gateway-ui` straight into the nginx container (`/opt/clm-gateway/web/gateway-ui:/usr/share/nginx/html:ro`), and `Dockerfile.satellite` never copies the web UI into the `clm-gateway` image at all. A change to `index.html` or `app.js` only needs the files copied to that host path and nginx reloaded — `sudo docker exec clm-gateway-nginx-1 nginx -s reload` — not a rebuild of the Go binary or the `clm-gateway` container. Bump the cache-bust query string on `app.js` (`<script src="app.js?v=...">` in `index.html`) so browsers with the old file cached pick up the change.

---

## Test Deployment

| Property | Value |
|----------|-------|
| Portal | http://3.7.21.28 |
| Instance | `clm-gw-dc-mumbai-01-fbbc77fd` (Lightsail medium_3_1, 2 vCPU, 4 GB RAM) |
| Static IP | `clm-ip-dc-mumbai-01-fbbc77fd` (3.7.21.28) |
| Region | ap-south-1a |
| Gateway ID | `c13fcc26-36d4-45f6-a556-e724dd99ebd0` (`dc-mumbai-01`) |
| SSH | `ssh -i ~/.ssh/clm-test-lightsail.pem ubuntu@3.7.21.28` |
| Remote path | `/opt/clm-gateway/` |
| Containers | `clm-gateway` (satellite binary), `nginx` |

Provision isolated test identities in Keycloak at runtime and supply credentials through local environment variables; no test password is tracked in this repository.

---

## Security

Accurate as of this revision. Several of these are weaker than a production deployment should accept.

| Property | Current state |
|----------|--------------|
| Private key custody | **Strong.** Keys are generated and stored on the gateway and never sent to the control plane; CA flows are CSR-only |
| SaaS connection identity | **Code Ready.** Unique rotatable per-satellite bearer credential (scrypt hash server-side) plus a verified client certificate in production; both bind to the satellite identity |
| Transport encryption | **Code Ready, not live-verified.** The connector requires HTTPS, verifies the configured server CA/name, presents its client cert/key, and refuses insecure HTTP/TLS in production. No live PKI handshake is claimed |
| Portal authentication | Keycloak-issued JWT, signature-verified against the realm JWKS |
| vSatellite enrolment | Single-use token plus per-agent secret, still over HTTP; command leasing/ack and executable certificate work remain incomplete |
| Deploy endpoint | Resolves certificate/key locally by `cert_id` and target credentials by `credential_ref`; inline PEM/private keys and inline credentials are rejected |
| Inbound exposure | Ports 80 / 8080 / 9090 open; only 80 is needed by browsers. `9090` should be firewalled to the segments that host agents |

### Fixed: the coordinator port accepted anyone (2026-08)

The vSatellite coordinator on `:9090` had **no authentication on any route**, and the port is published on `0.0.0.0` with the Lightsail firewall allowing it. Reproduced from the public internet against the test gateway:

```
POST http://<gateway>:9090/vsatellite/register   -> 200 {"agent_id":"rogue-probe-001","status":"registered"}
GET  http://<gateway>:9090/vsatellite/agents     -> 200 (full fleet inventory)
```

Three distinct problems:

1. **Open registration.** Any reachable client could join the fleet with an arbitrary ID and claim responsibility for any CIDR.
2. **Command theft.** A heartbeat carrying an unknown agent ID *auto-registered* it, and a heartbeat **drains that agent's queued commands**. Heartbeating with a legitimate agent's ID therefore collected work queued for it — including deployment instructions — and denied it to the real agent.
3. **Fleet disclosure.** `/vsatellite/agents` published every agent, its host, its CIDRs and its certificate counts without credentials.

Now: joining requires a single-use enrolment token an operator creates in the portal; that token is exchanged once for a per-agent secret which every later call must present; auto-registration on heartbeat is gone; and the unauthenticated agent listing was removed, since the portal API already exposes the fleet behind a JWT check. Comparisons on both credentials are constant-time, and tokens come from `crypto/rand`.

Worth stating plainly: closing this is necessary but not sufficient. The credentials still cross the network over **plain HTTP**, so anyone able to observe the traffic can replay them. Terminating TLS on the coordinator port and restricting `9090` to the agent segments are both still outstanding.

---

## Known Limitations

- **State is durable but single-node.** Inventory, approvals, deployment attempts, fleet state, and the SaaS command-result journal use atomic JSON snapshots and survive restart. Command journal writes are serialized through durable replacement, but the broader store still has no shared multi-replica/HA semantics.
- **Release 1 deployment verification is not full serving-endpoint proof for every adapter.** Linux currently verifies the deployed certificate file; PostgreSQL checks SSL configuration; full fingerprint/chain and application-health probing remains required before production use.
- **The GCP deployer is unproven against a live project.** The token exchange and error handling are exercised, but no GCP credentials were available, so the Compute API calls have not been run against a real project. Azure got further: its token request reaches Microsoft's real endpoint and returns a genuine `AADSTS` error, so that path is confirmed end to end up to the point of needing a valid tenant.
- **Credential storage is not encrypted at rest.** Sets are written `0600` in the state directory. Anything with root on the host, or a copy of the volume, can read them. A KMS- or Vault-backed store is the right answer and is not implemented.
- **vSatellite certificate execution is disabled.** Release 1 refuses a vSatellite executor with HTTP 501 so no private key is queued over HTTP; authenticated lease/ack/dedup and key-local execution are Release 2 work.
- **The GoDaddy and Namecheap integrations are unproven against the live vendors.** They are verified against a mock speaking both real protocols, but no vendor credentials were available, so product identifiers and state strings come from documentation. Expect to adjust product names on first contact with a real sandbox.
- **No approval gate before spending money.** A `cert_manager` can place a paid commercial order directly; the approval model exists in the inventory but is not wired to CA purchasing, and there is no budget cap.
- **Commercial-CA DCV is reported, not performed.** The gateway shows the DNS record or HTTP file a commercial CA wants and polls for completion. ACME is the exception: HTTP-01 is served automatically, and RFC2136 or Route 53 DNS-01 creates, polls, and cleans up its TXT value automatically.
- **Cloud DNS-01 coverage is incomplete and not live-validated.** AWS Route 53 is selectable with workload identity or an `aws-profile://` reference, but no real hosted-zone mutation has been run. Its TXT merge lock is process-local, so the challenge RRset requires a single CLM writer. Azure DNS, GCP Cloud DNS, and OCI DNS remain planned and fail during configuration. HTTP-01 remains available for ordinary names.
- **Renewal only covers certificates issued here.** Anything discovered has no private key on the gateway; the action list flags it, but a human has to renew it wherever it actually lives.
- **Renewal redeployment is code-ready, not live-proven.** Managed bindings are replayed and endpoint verification gates activation; partial failure compensates with the prior version. Indeterminate restart or persistence outcomes block further renewal until the explicit recovery action restores the last committed version. A real ACME multi-endpoint rollout and recovery drill remain release gates.
- **`schedule` is not a cron expression.** The scheduler uses `interval` (a Go duration); the cron string is kept for display only, because parsing cron would mean another dependency. There is no "run at 02:00" support.
- **Alert delivery is fire-and-forget.** There is no retry queue: if a webhook is down at the moment of evaluation, that alert is recorded as undeliverable and the next attempt waits for the next evaluation.
- **vSatellite scan/deploy/verify command execution is not implemented.** Enrolment and heartbeat exist, but Release 2 must replace destructive command delivery with scoped lease/ack execution before certificate work is enabled.
- **No HSM.** `crypto.hsm_enabled` exists in the config and nothing reads it. PKCS#11 needs CGO, and the binary is built `CGO_ENABLED=0`, so an HSM backend would require changing the build. Keys are generated in software and held on the gateway's disk; AWS KMS is used only to encrypt values in Secrets Manager, not to hold certificate keys.
- **State is a JSON snapshot, not a database.** It is loaded in full at startup and rewritten in full on change, which is fine at the scale this gateway holds and would not be at tens of thousands of certificates. There is no concurrent-writer safety across replicas, and the snapshot itself carries no version history — the *event* history is covered separately by the append-only audit trail (see above), but the *config* state (inventory, policies, CA accounts) has no history of its own, only its current shape.
- **The AWS mutations are unproven against real AWS.** The `aws-acm` deployer is registered and its option handling is tested, but the test gateway's Lightsail instance role has no ACM or ELBv2 permissions, so import, listener attach, retention delete and rollback have only been exercised as far as an `AccessDeniedException` from the real API.
- **Control-plane command delivery retries are lease/idempotency guarded, but deployment adapter reconciliation is still incomplete.** SaaS commands have tenant idempotency keys, bounded retry/dead-letter state and attempt-scoped duplicate suppression. A process crash during an external adapter side effect still needs adapter-specific observed-state reconciliation.
- **AWS is the only secrets provider**; the config switch defaults every other value to AWS.
- **Host sizing** is 4 GB RAM. The control plane host was lost to an out-of-memory condition under a similar profile and has since been given swap; confirm this host has swap before running builds on it.

---

## High Availability & Autoscaling

> **Status: Planned.** The gateway is currently a **single stateful node** and cannot be replicated
> as-is. Full topology, autoscaling signals, and per-platform detail are in the
> [control plane README](../saas-control-plane/README.md#high-availability--autoscaling); this
> section covers what must change *inside this component*.

Running more than one replica today would break in four specific ways:

| Blocker | Consequence of ignoring it |
|---------|---------------------------|
| Certificate and approval inventory is a Go map (`internal/api/inventory.go`) | Replicas hold divergent inventories; an approval created on one is invisible to the others |
| `satellite.yaml` carries a single `id` | N replicas heartbeating with the same ID overwrite each other's status in the fleet view |
| Control-plane command delivery is leased but gateway-local state is not shared | Multiple replicas can claim SaaS commands exclusively, but they do not share execution journals or lifecycle state, so failover cannot yet guarantee safe continuation |
| No leader election | Renewal scheduling and expiry monitoring (once built) would run N times, producing N renewal attempts per certificate |

**Not a blocker:** private key custody. Keys already go to AWS Secrets Manager rather than local disk, so replicas can share key material with no change. This is the part of the design that is already HA-ready.

### Required changes in this component

| # | Change | Detail |
|---|--------|--------|
| 1 | **Replace `Inventory` with a database-backed store** | Keep the same interface; swap the maps for PostgreSQL (or SQLite for single-node installs). This is the prerequisite for everything else |
| 2 | **Add leader election** | `pg_try_advisory_lock` or a TTL lease row. PostgreSQL is already required by change 1, so this adds no new infrastructure — prefer it over etcd/Consul |
| 3 | **Gate leader-only work** | Only the leader runs the SaaS heartbeat, command pull, renewal scheduling, and expiry monitoring. All replicas continue serving the Portal API |
| 4 | **Retain leased claims and add shared execution state** | SaaS delivery already uses claim/ack/renew/result tokens and does not clear work on pull. Move the command journal and lifecycle checkpoints to shared transactional state before enabling multiple replicas |
| 5 | **Report cluster health** | Add `replica_id`, `replica_count`, and `healthy_replicas` to the heartbeat so the fleet view shows cluster state, not one node's |
| 6 | **Partition discovery work** | Two replicas told to scan `10.0.0.0/16` both scan all of it. Split CIDRs across replicas, or delegate to vSatellite agents |
| 7 | **Drain on shutdown** | Finish in-flight scans and deployments on `SIGTERM` before exiting, so scale-in and rolling upgrades do not abandon work |
| 8 | **Expose scaling metrics** | Publish scan-queue and deploy-queue depth for the autoscaler. Certificate count is not a load signal — an idle gateway holding 10,000 certificates is doing no work |

### Deployment shape once complete

- **EKS/GKE/AKS** — `Deployment` with `replicas: N`, a PodDisruptionBudget, node anti-affinity, HPA on CPU plus queue depth via KEDA. Lowest-effort correct HA, so validate the model here first.
- **EC2** — Auto Scaling Group across two AZs behind the ALB that the deploy engine already provisions; health checks on `/health`.
- **On-premise** — N hosts behind keepalived VIP or an existing load balancer. Fixed capacity, no autoscaling; alert on sustained queue depth instead.
- **Lightsail** — **not supported.** No load balancer integration and no scaling group. HA on this platform requires migrating to EC2 or EKS.

Never scale below 2 replicas once HA is enabled, and keep scale-in cooldowns long (~10 minutes) — discovery schedules are bursty and aggressive scale-in causes flapping.

---

## Recommended Improvements

Ordered by risk. See the [control plane README](../saas-control-plane/README.md#recommended-improvements) for platform-wide items.

| # | Item | Why |
|---|------|-----|
| 1 | **Move state from JSON snapshots to a real store** | Durable snapshots landed (`internal/state`), so a restart no longer loses inventory or approvals. The remaining gap is scale and concurrency: the file is read and rewritten whole, config state carries no version history, and there is no concurrent-writer safety across replicas. **Still the hard prerequisite for HA** — see above |
| 2 | **Live-validate transport and finish credential hardening** | Task 3 adds production fail-closed HTTPS/mTLS, unique hashed bearer credentials, rotation/revocation, and leased command delivery. Remaining: exercise a real PKI handshake, rotate any deployed legacy secrets, secure the separate gateway↔vSatellite channel, and move credential storage behind KMS/Vault/workload identity |
| 3 | **Move command execution state to a shared transactional store** | The local journal prevents duplicate side effects on one durable gateway and distinguishes delivery attempts, but HA replicas need one shared journal/checkpoint store before leader failover is safe |
| 4 | **Prove the AWS mutations against a real account** | The `aws-acm` deployer is registered and its option handling is unit tested, but the test gateway's Lightsail role has no ACM or ELBv2 permissions. An OTE-style account with `acm:ImportCertificate` and the ELBv2 listener permissions would confirm import, attach, retention and rollback in an afternoon |
| 5 | **Serialise renewal per certificate** | Issuance, renewal and revocation are implemented and verified against Pebble, but a scheduled renewal and a clicked renewal of the same certificate can run at once and write a chain and a key from different orders to the same paths |
| 6 | **Extend test coverage past the AWS deployer** | `internal/deployment/deployers/cloud` has tests and they run under `-race`. The scanner, inventory status transitions, CIDR expansion and the expiry arithmetic are all cheap to cover and all have known defects |
| 7 | **Make `saas_endpoint` and the nginx SaaS proxies configurable** | `35.154.6.28` is hardcoded in `nginx.conf`; a second gateway against a different control plane needs a template |
| 8 | **Confirm swap on the host** | Same failure profile that took down the control plane |
| 9 | **Leader election + shared execution state for HA** | Control-plane command leasing is complete; leader-only polling and shared gateway journals/lifecycle state remain prerequisites before any multi-replica deployment |
| 10 | **Graceful drain on `SIGTERM`** | Without it, autoscaling scale-in and rolling upgrades abandon in-flight scans and deployments |
| 11 | **Multi-level approval engine (L1/L2/L3)** | The policy engine's `require_approval_for_cert_types` creates a single flat approval; a real OV/EV workflow at scale often needs sequential sign-off (e.g. security then finance) rather than one decision |
| 12 | **Move the audit log off local disk** | JSONL-on-disk is durable against a process crash but not against losing the volume. Shipping events to an external sink (e.g. a log pipeline or object storage) would survive instance loss, which local append-only files do not |

> Item 6 in the previous revision of this list ("add role checks to the nine mutating routes that have none") is resolved: the five confirmed gaps (`handleRequestCertificate`, `handleDiscover`, `handleDiscoverDNS`, `handleDiscoverACM`, `handleCAQuote`) are fixed and verified live via viewer-token probes — see the Authentication section above.
