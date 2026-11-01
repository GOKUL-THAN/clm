# CLM-SPEC-13: PROCUREMENT & FINANCIAL MANAGEMENT SPECIFICATION
## Three-Way Matching (PO/Receipt/Invoice), Supplier Governance, and Leakage Detection

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-FIN-013  
**Classification:** Enterprise Finance & Sourcing Standard  

---

### 1. Automated Three-Way Matching Engine

To prevent procurement fraud, duplicate payments, and unauthorized price variances, the platform implements automated **Three-Way Matching**:

```
+-----------------------------------+     +-----------------------------------+
|      Purchase Order (PO)          |     |        Goods Receipt (GR)         |
|  - Authorized Amount: $87,500.00  |     |  - Accepted Quantity: 100%        |
|  - Rate Schedule: MSA Terms       |     |  - Inspection Status: Passed      |
+-----------------+-----------------+     +-----------------+-----------------+
                  |                                         |
                  +--------------------+--------------------+
                                       |
                                       v
                     +-----------------------------------+
                     |       Supplier Invoice (INV)      |
                     |  - Billed Amount: $87,500.00      |
                     |  - Currency: USD                  |
                     +-----------------+-----------------+
                                       |
                                       v
                     +-----------------------------------+
                     |      Three-Way Matching Check     |
                     |  - Price Variance: $0.00 (0.0%)   |
                     |  - Tolerance Threshold: 2.0%      |
                     |  - Result: PERFECT MATCH          |
                     |  - Status: APPROVED FOR DISBURSE  |
                     +-----------------------------------+
```

#### Variance Exceptions:
If the variance between PO and Invoice exceeds the tolerance threshold (default 2.0%), the invoice is flagged as `variance_exception` and held in escrow pending CFO override.

---

### 2. Corporate Supplier Lifecycle Management

Tracks vendor profiles in `procurement_suppliers`:
* **Onboarding Stages:** `prequalification`, `due_diligence`, `onboarding`, `active`, `offboarding`, `suspended`.
* **Risk Tiering:** `Tier-1` (Mission-critical infrastructure), `Tier-2` (Operational SaaS), `Tier-3` (Commodity services).
* **Compliance Checks:** Mandatory insurance certificate tracking (`insurance_cert_expires`) with automated renewal notices.
* **Performance Scoring:** Dynamic composite score (0-100) based on SLA adherence, defect rate, and delivery timeliness.

---

### 3. Contract & Revenue Leakage Detection

The financial analyzer scans `contract_financials` for:
1. **Unbilled Milestone Commitments:** Earned revenue not yet invoiced.
2. **Pricing Discrepancies:** Supplier charging higher rates than negotiated MSA rate cards.
3. **Unclaimed Service Credits:** Missed SLA penalties that were not deducted from payments.

