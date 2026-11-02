# DEMO-GUIDE.md
# CLM Enterprise Platform — Local Demo Guide

## Overview

This guide walks you through running a complete demonstration of the Enterprise Contract &  
Certificate Lifecycle Management (CLM) platform locally. No Docker, PostgreSQL, or cloud  
services are required — the demo runs entirely in-memory using Node.js.

---

## Prerequisites

| Requirement | Version | Check |
|---|---|---|
| Node.js | >= 18.0.0 | `node --version` |
| npm | >= 8.0.0 | `npm --version` |
| Browser | Chrome / Firefox / Edge (modern) | — |

**No other dependencies required for demo mode.**

---

## Installation

```bash
# Clone or open the repository in VS Code
cd "d:\400 clm\CLM"

# Verify setup (checks all required files)
node scripts/setup-demo.js
```

---

## Environment Setup

Copy the example environment file (optional — demo runs without it):
```bash
copy .env.example .env.local
```

No changes to `.env.local` are needed for demo mode.

---

## Database Initialization

**Not required for demo mode.** The demo uses in-memory mock data with:

- Pre-seeded contracts (10+ contracts across all lifecycle stages)
- Pre-seeded tenants, departments, and users
- Pre-seeded approval workflows
- Pre-seeded obligations, compliance records, and audit logs
- Pre-seeded certificate inventory

---

## Starting the Demo

### Option 1: Direct start (recommended)
```bash
node dev-server.js
```

### Option 2: Using npm
```bash
npm start
```

### Option 3: Using the demo script
```bash
node scripts/start-demo.js
```

Both servers start automatically:
- **SaaS Admin Portal**: http://localhost:3000
- **Gateway Portal**: http://localhost:8080

---

## Frontend URL

| Portal | URL |
|---|---|
| SaaS Control Plane Admin Portal | http://localhost:3000 |
| Satellite Gateway Portal | http://localhost:8080 |

---

## Backend URL

| Service | URL |
|---|---|
| API Base | http://localhost:3000 |
| Health Check | http://localhost:3000/api/v1/health |

---

## Health Check Endpoints

```bash
# Full health check
node scripts/health-check.js

# Individual checks
curl http://localhost:3000/api/v1/health
curl http://localhost:3000/api/v1/contracts
curl http://localhost:3000/api/v1/tenants
```

---

## Demo Credentials

> These are **demo-only** credentials. Do not use in production.

| Role | Email | Password |
|---|---|---|
| Super Admin | admin@enterprise.internal | Demo@2024! |
| Legal Counsel | legal-lead@enterprise.internal | Demo@2024! |
| Auditor | auditor@enterprise.internal | Demo@2024! |
| Finance Approver | finance@enterprise.internal | Demo@2024! |

---

## Complete Demonstration Workflow

### 1. Login
1. Open http://localhost:3000
2. Enter email: `admin@enterprise.internal`
3. Enter password: `Demo@2024!`
4. Click **Sign In**

### 2. Dashboard
- View the Dashboard showing:
  - Total contracts count
  - Active contracts
  - Pending approvals
  - Upcoming expirations
  - CLM health metrics

### 3. Contract Repository
- Navigate to **Contracts** in the sidebar
- View the contract portfolio with 10+ pre-loaded contracts
- Filter by type, status, stage, or counterparty

### 4. Create a Contract
1. Click **New Contract** button
2. Fill in: Title, Type (NDA/MSA/SOW), Counterparty, Value
3. Select a template from the Template Library
4. Submit the form

### 5. Contract Workflow (26 Stages)
1. Navigate to any contract
2. Click **Advance Stage** to move through:
   - Draft → Authoring → Negotiation → Review → Approval → Signing → Executed

### 6. Approval Workflow
1. Navigate to **Approvals** in the sidebar
2. View pending approvals (L1, L2, L3 tiers)
3. Click **Approve** or **Reject** with comments
4. Observe multi-tier approval progression

### 7. Negotiation & Redlines
1. Open a contract in negotiation stage
2. Navigate to **Negotiations** tab
3. Add a redline/markup
4. Accept or reject counterparty redlines
5. View version history

### 8. E-Signature (Demo)
1. Open an executed or signing-ready contract
2. Navigate to **Signatures** tab
3. Click **Sign Contract**
4. Complete the demo signing process
5. View the cryptographic evidence bundle

### 9. Obligation Tracking
1. Navigate to **Obligations** in the sidebar
2. View active obligations with due dates
3. Mark an obligation as fulfilled
4. Check the compliance calendar

### 10. AI Contract Analysis
1. Navigate to **AI Contracts** in the sidebar
2. Select a contract for analysis
3. Click **Run AI Analysis**
4. View: Risk Detection, Clause Extraction, Compliance Check
5. Ask the AI Legal Assistant a question

### 11. Compliance & Risk
1. Navigate to **Risk Posture**
2. View the enterprise risk register
3. Navigate to **CLM Reports** for analytics dashboard
4. Export a compliance report

### 12. Audit Trail
1. Navigate to **Activity** or **Audit Logs**
2. View immutable audit events for any contract
3. Filter by user, action, or date range

### 13. Certificate Management (Gateway Portal)
1. Open http://localhost:8080
2. View certificate inventory
3. Check expiring certificates
4. Review satellite gateway status

### 14. 400 Features Explorer
1. Navigate to **400 Features** in the sidebar
2. Browse all 400 enterprise CLM features organized by domain
3. Click any feature to view its implementation details

---

## Running Tests

```bash
# Run all tests
node scripts/run-demo-tests.js

# Individual test suites
node scripts/verify_scenarios_a_to_h.js   # 40 enterprise scenarios
node scripts/test_lifecycle_26_stages.js   # 26 CLM lifecycle stages
node scripts/test_400_features.js          # 400 feature registry
node scripts/test_corporate_modules.js     # 10 corporate module suites

# Via npm
npm test
npm run test:lifecycle
npm run test:400
npm run test:corporate
```

---

## Stopping the Demo

```bash
# Option 1: Demo script
node scripts/stop-demo.js

# Option 2: Press Ctrl+C in the terminal running dev-server.js

# Option 3: Manual (Windows)
taskkill /F /IM node.exe
```

---

## Resetting Demo Data

All data is in-memory and resets automatically on server restart:
```bash
node scripts/stop-demo.js
node dev-server.js
```

---

## Troubleshooting

### Port already in use
```bash
# Windows
netstat -ano | findstr :3000
taskkill /PID <pid> /F

# macOS/Linux
lsof -ti:3000 | xargs kill -9
```

### Server fails to start
```bash
# Check Node.js version (must be >= 18)
node --version

# Check syntax
node -c dev-server.js

# Check setup
node scripts/setup-demo.js
```

### Frontend shows blank page
- Clear browser cache (Ctrl+Shift+Delete)
- Open http://localhost:3000 (not file://)
- Check browser console for errors

### API returns HTML instead of JSON
- Ensure the server is running: `curl http://localhost:3000/api/v1/health`
- Check the correct URL prefix (`/api/v1/`, not `/api/v1/clm/`)

---

## Architecture Notes

This demo uses a **unified Node.js development server** (`dev-server.js`) that:
- Serves the SaaS Admin Portal on port 3000
- Serves the Satellite Gateway Portal on port 8080
- Implements all API endpoints with in-memory mock data
- Requires no external services (no DB, no Redis, no Docker)

In production, the real architecture uses:
- Go backend services (saas-control-plane, satellite-gateway, vsatellite)
- PostgreSQL with Row Level Security for multi-tenant isolation
- Keycloak for authentication
- Redis for caching
- AWS/GCP/Azure infrastructure via Terraform

---

## Shutdown Instructions

1. Press `Ctrl+C` in the terminal running `node dev-server.js`
2. Or run: `node scripts/stop-demo.js`

All in-memory data is discarded on shutdown.