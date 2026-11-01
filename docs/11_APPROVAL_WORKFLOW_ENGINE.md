# CLM-SPEC-11: APPROVAL WORKFLOW & DOA ENGINE SPECIFICATION
## Delegation of Authority (DOA), Multi-Tier Progression, and Segregation of Duties

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-APP-011  
**Classification:** Enterprise Governance & Compliance Standard  

---

### 1. Delegation of Authority (DOA) Threshold Matrix

Approval routing is governed by financial value, risk category, and department:

| Total Contract Value (USD) | Approval Tier | Required Approver Roles | SLA Turnaround |
| :--- | :---: | :--- | :---: |
| **\$0 - \$50,000** | **L1** | Department Manager / Cost Center Owner | 24 Hours |
| **\$50,001 - \$250,000** | **L2** | Department VP + Legal Counsel + Finance Controller | 48 Hours |
| **\$250,001 - \$1,000,000** | **L3** | CFO + Chief Legal Officer (CLO) + CISO | 72 Hours |
| **> \$1,000,000** | **Executive**| Chief Executive Officer (CEO) + Board of Directors | 5 Days |

---

### 2. Multi-Tier Stage Progression & State Machine

```
[Contract Submitted for Approval]
                |
                v
        [Evaluate DOA Rule]
                |
    +-----------+-----------+
    |                       |
[Value <= $50K]      [Value > $50K]
    |                       |
    v                       v
[L1 Queue] --------> [L1 Approved?] 
                            |
                     (Cost >= $100 or Cert Purchase)
                            |
                            v
                     [Promote to L2 Queue]
                            |
                     (Cost >= $500 or CISO Required)
                            |
                            v
                     [Promote to L3 Executive Queue]
                            |
                            v
                     [Final Executive Signoff]
```

---

### 3. Segregation of Duties (SoD) Rule ARC-004

To comply with **Sarbanes-Oxley (SOX) Section 404** and **SOC 2 CC6.3**, the approval engine implements strict segregation of duties:
* **The Rule:** An individual who authored, requested, or modified a contract or purchase order is cryptographically and logically barred from approving it.
* **Engine Implementation (`approval/engine.go`):**
  ```go
  if req.RequestedBy != uuid.Nil && req.RequestedBy == approverID {
      return nil, ErrSegregationOfDuties // Returns 403 Forbidden
  }
  ```
* **Verification:** Tested and certified in **Scenario D**.

