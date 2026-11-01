# CLM-SPEC-26: DEVSECOPS & CI/CD PIPELINE SPECIFICATION
## Automated Quality Gates, Static Analysis, SBOM Generation, and Container Security

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-CI-026  
**Classification:** Enterprise Engineering Pipeline Standard  

---

### 1. DevSecOps Pipeline Architecture

Security is integrated into every stage of the software delivery lifecycle using automated GitHub Actions pipelines (`.github/workflows/ci.yml`):

```
+-----------------------------------------------------------------------------------------+
|                                CI/CD SECURITY PIPELINE                                  |
+-----------------------------------------------------------------------------------------+
| [1. Commit & PR] ----> [2. Static Analysis] --> [3. Unit/Integration] -> [4. Security]  |
|  - Conventional         - Gosec AST Scanner      - Scenarios A - H        - Trivy Vuln  |
|    Commit Checks        - Semgrep Rules          - 14 Dept Test Suites    - CycloneDX   |
|  - Go Module Verify     - Go Vet & Lint          - Code Coverage > 85%      SBOM Export |
|                                                                                         |
| [8. Production] <----- [7. Cluster Deploy] <-- [6. Image Sign] <-------- [5. Container] |
|  - Canary Rollout       - Restricted K8s         - Cosign PKI Sig         - Distroless  |
|  - Telemetry Probes     - NetworkPolicy Apply    - Attestation Ledger     - Non-Root    |
+-----------------------------------------------------------------------------------------+
```

---

### 2. Automated Quality Gates

Every pull request must pass four mandatory gates before merge approval is unlocked:
1. **Gate 1 - Code Quality:** Zero linting errors via `golangci-lint` with strict type checking.
2. **Gate 2 - Static Security Analysis:** Zero HIGH or CRITICAL issues reported by `gosec` and `semgrep`.
3. **Gate 3 - Scenario Verification:** Automated test runner must achieve 100% pass rate on Scenarios A through H with zero stubs.
4. **Gate 4 - Container Vulnerability Scanning:** Docker container image scanned by Trivy must contain 0 unpatched CVEs.

---

### 3. Supply Chain Security (SLSA Level 3)

* **Provenance Attestation:** Releases are signed using Sigstore Cosign with cryptographic key pairs generated in CI.
* **Software Bill of Materials (SBOM):** Standard CycloneDX JSON SBOM generated during compilation and attached to release artifacts.

