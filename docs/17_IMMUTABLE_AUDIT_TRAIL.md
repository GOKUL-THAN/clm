# CLM-SPEC-17: IMMUTABLE AUDIT TRAIL SPECIFICATION
## WORM Protection, PostgreSQL Trigger Enforcement, Non-Repudiation, and SOX

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-AUD-017  
**Classification:** Enterprise Compliance & Security Standard  

---

### 1. Compliance Requirements & WORM Rationale

Regulatory frameworks including **Sarbanes-Oxley (SOX) Section 404**, **SOC 2 Trust Services Criterion CC6.8**, and **ISO/IEC 27001 Control A.8.15** mandate that audit evidence must be:
1. **Complete:** Recording actor identity, action type, resource affected, timestamp, source IP, and before/after diffs.
2. **Immutable (WORM - Write Once, Read Many):** Protected against alteration, truncation, or deletion by any user, including database superusers.

---

### 2. Database Trigger Enforcement Mechanism

In `components/saas-control-plane/deploy/migrations/006_contract_security_and_procurement_rls.sql`, the platform binds database-level exception triggers directly to `contract_audit_logs`:

```sql
CREATE OR REPLACE FUNCTION reject_contract_audit_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'Contract audit log records are immutable and append-only under compliance requirements (SOX / SOC 2 / ISO 27001)';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contract_audit_no_update ON contract_audit_logs;
CREATE TRIGGER contract_audit_no_update
BEFORE UPDATE ON contract_audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_contract_audit_mutation();

DROP TRIGGER IF EXISTS contract_audit_no_delete ON contract_audit_logs;
CREATE TRIGGER contract_audit_no_delete
BEFORE DELETE ON contract_audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_contract_audit_mutation();
```

---

### 3. Captured Audit Telemetry

Each record in `contract_audit_logs` captures:
* `tenant_id`: UUID of the enterprise tenant.
* `contract_id`: UUID of the associated agreement.
* `actor_email`: Verified corporate identity of the user or service account.
* `action_type`: `create`, `edit`, `approve`, `redline_proposed`, `signed`, `obligation_fulfilled`, `risk_flagged`.
* `description`: Plain-language explanation of the event.
* `payload_diff`: JSONB capturing field-level before/after changes.
* `ip_address`: Forensic IPv4/IPv6 address of the calling client.
* `created_at`: Hardware-synchronized UTC timestamp.

Verified in **Scenario H**.

