# CLM-SPEC-09: NEGOTIATION & REDLINING SPECIFICATION
## External Workspace, Cryptographic Tokens, Redline Diffing, and Resolution

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-NEG-009  
**Classification:** Enterprise Legal Operations Standard  

---

### 1. External Collaboration Architecture

External negotiations avoid insecure email attachments (e.g. circulating untracked `.docx` files) through an ephemeral, tokenized **Counterparty Negotiation Workspace**:
* **Secure Guest Token:** High-entropy cryptographically random tokens (`tok-guest-<hash>`) generated with 72-hour expiration windows.
* **Isolated Perimeter:** Counterparties access a restricted view showing only the negotiated agreement, proposed redlines, and comments thread, with no visibility into internal corporate data or other contracts.

---

### 2. Redline Diffing & State Progression

Proposed contract modifications are tracked discretely in `contract_redlines`:
* **Change Types:** `addition`, `deletion`, `modification`.
* **Attributes Tracked:** `original_text`, `proposed_text`, `clause_title`, `author_type` (`internal` vs `counterparty`), `author_name`.
* **State Machine:**
  ```
  [proposed] ---> [accepted]   (Incorporates proposed text into active working draft)
             ---> [rejected]   (Preserves original standard language)
             ---> [superseded] (Replaced by alternative counter-proposal)
  ```

---

### 3. Resolution & Audit Ledger

1. Every redline acceptance or rejection records the deciding user, timestamp, and justification.
2. Once all redlines are resolved, the negotiation session is closed, generating an immutable revision record in `contract_versions` with an updated SHA-256 digest.

