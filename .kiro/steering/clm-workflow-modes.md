---
inclusion: auto
name: clm-workflow-modes
description: Activated when the user mentions feature development, planning, bug fixing, or spec generation for CLM
---

# CLM Workflow Modes — Quick Reference

Four structured development modes are available for the CLM project. Start your message with the trigger phrase to activate the matching workflow automatically.

## 1. Structured Feature Development
**Trigger:** `feature: <description>` or `/feature <description>`  
**Workflow:** `.kiro/workflows/clm-feature-dev.workflow.json`  
**Pipeline:**
```
design → design-review loop (up to 3x) → plan → implement+code-review loop (up to 5x) → validate
```
- Produces: `feature-design.md`, `feature-plan.md`, `feature-summary.md`
- Runs `make test` + `make lint` before closing
- CLM constraints enforced: multi-tenant isolation, audit logging, compliance

## 2. Plan-Only Mode
**Trigger:** `plan: <idea>` or `/plan <idea>`  
**Workflow:** `.kiro/workflows/clm-plan-only.workflow.json`  
**Pipeline:**
```
investigate (read-only) → structured plan document
```
- **No code changes made**
- Produces: `plan-output.md` with ordered tasks, complexity estimate, open questions
- Use this to evaluate feasibility or break down ideas before committing to implementation

## 3. Structured Bug-Fix
**Trigger:** `bug: <description>` or `/bug <description>`  
**Workflow:** `.kiro/workflows/clm-bugfix.workflow.json`  
**Pipeline:**
```
investigate → diagnose (root cause + fix plan) → fix + tests
```
- Produces: `bug-investigation.md`, `bug-diagnosis.md`, `bug-fix-summary.md`
- Checks compliance/audit/tenant-isolation blast radius as part of investigation
- Runs `make test` + `make lint` to verify fix

## 4. SpecFast
**Trigger:** `spec: <idea>` or `/spec <idea>`  
**Workflow:** `.kiro/workflows/clm-specfast.workflow.json`  
**Pipeline:**
```
clarify → requirements (FR/NFR) → technical design → implementation task list
```
- Produces: `spec-clarifications.md`, `spec-requirements.md`, `spec-design.md`, `spec-tasks.md`, `spec-index.md`
- All artifacts written to `d:\cmd project\M2P-Git\CLM\.agents\tasks\`
- Flags compliance documentation requirements if approval workflow or e-signature path is touched

## Component Names
When a component is needed, use one of:
- `saas-control-plane` — central orchestration, multi-tenant control
- `satellite-gateway` — customer in-VPC gateway, zero-egress AI
- `vsatellite` — cloud-native virtual satellite variant
- `shared` — shared libraries and types

If not specified, the workflow will infer from context or default to `saas-control-plane`.

## Output Location
All workflow artifacts land in:
```
d:\cmd project\M2P-Git\CLM\.agents\tasks\
```
