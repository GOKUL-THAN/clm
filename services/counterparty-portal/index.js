/**
 * Enterprise CLM External Counterparty Negotiation Workspace Engine
 * Patterned after Ironclad External Collaboration and Agiloft Counterparty Portal.
 * Enables zero-trust, tokenized negotiation with external counterparties without requiring
 * internal enterprise logins. Manages HMAC-SHA256 signed access tokens, external draft redlining,
 * revision rounds, and cryptographic audit hashing.
 */

const crypto = require('crypto');

class CounterpartyPortalEngine {
    constructor() {
        this.secret = process.env.CLM_PORTAL_SECRET || "enterprise-clm-counterparty-portal-hmac-secret-2026";
        this.activeSessions = new Map(); // token -> session data
        this.negotiationRounds = new Map(); // contract_id -> array of rounds
    }

    /**
     * Creates a secure, time-bounded negotiation token for an external counterparty.
     * @param {Object} params Contract ID, counterparty email, permissions, and duration
     * @returns {Object} Token metadata, secure link, and digest
     */
    generateNegotiationToken(params) {
        const contractId = params.contract_id || "CNT-" + Math.floor(1000 + Math.random() * 9000);
        const counterpartyEmail = params.counterparty_email || "legal@vendor-global.com";
        const counterpartyOrg = params.counterparty_org || "Vendor International Corp";
        const permissions = params.permissions || ["VIEW", "COMMENT", "SUBMIT_REDLINE"];
        const expiryDays = params.expiry_days || 14;
        const expiresAt = new Date(Date.now() + expiryDays * 86400000).toISOString();
        const tokenId = "PORTAL-" + crypto.randomBytes(8).toString('hex');

        // Create HMAC signature
        const payloadToSign = `${tokenId}:${contractId}:${counterpartyEmail}:${expiresAt}`;
        const signature = crypto.createHmac('sha256', this.secret)
            .update(payloadToSign)
            .digest('hex');

        const token = `${tokenId}.${signature}`;

        const session = {
            token_id: tokenId,
            full_token: token,
            contract_id: contractId,
            contract_title: params.contract_title || "Enterprise Master Commercial Agreement",
            current_draft: params.initial_draft || "This Master Services Agreement governs the relationship between Company and Counterparty...",
            counterparty_email: counterpartyEmail,
            counterparty_org: counterpartyOrg,
            permissions: permissions,
            created_at: new Date().toISOString(),
            expires_at: expiresAt,
            status: "ACTIVE",
            round_count: 1
        };

        this.activeSessions.set(token, session);

        if (!this.negotiationRounds.has(contractId)) {
            const initialDigest = crypto.createHash('sha256').update(session.current_draft).digest('hex');
            this.negotiationRounds.set(contractId, [
                {
                    round: 1,
                    author: "Enterprise Internal Legal Counsel",
                    author_type: "INTERNAL",
                    summary: "Initial Draft Issued",
                    draft_digest: initialDigest,
                    timestamp: new Date().toISOString()
                }
            ]);
        }

        return {
            token: token,
            token_id: tokenId,
            portal_url: `https://clm.enterprise.internal/external/portal?token=${token}`,
            contract_id: contractId,
            counterparty_email: counterpartyEmail,
            counterparty_org: counterpartyOrg,
            permissions: permissions,
            expires_at: expiresAt,
            status: "ACTIVE"
        };
    }

    /**
     * Validates a counterparty negotiation token.
     * @param {string} token Combined token string (tokenId.signature)
     * @returns {Object} Session info or error
     */
    validateToken(token) {
        if (!token || typeof token !== 'string' || !token.includes('.')) {
            return { valid: false, error: "INVALID_TOKEN_FORMAT" };
        }

        const session = this.activeSessions.get(token);
        if (!session) {
            return { valid: false, error: "TOKEN_NOT_FOUND" };
        }

        if (session.status !== "ACTIVE") {
            return { valid: false, error: "TOKEN_REVOKED_OR_EXPIRED" };
        }

        if (new Date() > new Date(session.expires_at)) {
            session.status = "EXPIRED";
            return { valid: false, error: "TOKEN_EXPIRED" };
        }

        const [tokenId, sig] = token.split('.');
        const payloadToSign = `${tokenId}:${session.contract_id}:${session.counterparty_email}:${session.expires_at}`;
        const expectedSig = crypto.createHmac('sha256', this.secret)
            .update(payloadToSign)
            .digest('hex');

        if (sig !== expectedSig) {
            return { valid: false, error: "TAMPERED_TOKEN_SIGNATURE" };
        }

        const rounds = this.negotiationRounds.get(session.contract_id) || [];

        return {
            valid: true,
            session: {
                token_id: session.token_id,
                contract_id: session.contract_id,
                contract_title: session.contract_title,
                counterparty_email: session.counterparty_email,
                counterparty_org: session.counterparty_org,
                permissions: session.permissions,
                expires_at: session.expires_at,
                current_draft: session.current_draft,
                current_round: rounds.length,
                negotiation_history: rounds
            }
        };
    }

    /**
     * Submits an external counterparty redline revision.
     * @param {string} token Access token
     * @param {string} redlineText New drafted text
     * @param {string} comments Rationale or commentary on changes
     * @returns {Object} Negotiation round details and new draft hash
     */
    submitCounterpartyRedline(token, redlineText, comments = "") {
        const val = this.validateToken(token);
        if (!val.valid) {
            throw new Error(`Counterparty token invalid: ${val.error}`);
        }

        const session = this.activeSessions.get(token);
        if (!session.permissions.includes("SUBMIT_REDLINE")) {
            throw new Error("Counterparty lacks SUBMIT_REDLINE permission");
        }

        const rounds = this.negotiationRounds.get(session.contract_id) || [];
        const nextRoundNum = rounds.length + 1;

        const draftDigest = crypto.createHash('sha256')
            .update(redlineText || "")
            .digest('hex');

        const roundEntry = {
            round: nextRoundNum,
            author: session.counterparty_email,
            author_org: session.counterparty_org,
            author_type: "EXTERNAL_COUNTERPARTY",
            comments: comments || "Proposed revisions submitted via Counterparty Portal",
            draft_digest: draftDigest,
            timestamp: new Date().toISOString()
        };

        rounds.push(roundEntry);
        session.current_draft = redlineText;
        session.round_count = nextRoundNum;

        return {
            success: true,
            contract_id: session.contract_id,
            round: nextRoundNum,
            author: session.counterparty_email,
            draft_digest: draftDigest,
            round_entry: roundEntry,
            total_rounds: rounds.length
        };
    }

    /**
     * Revokes a negotiation token (e.g., when contract moves to signing).
     * @param {string} token Access token
     */
    revokeToken(token) {
        if (this.activeSessions.has(token)) {
            const session = this.activeSessions.get(token);
            session.status = "REVOKED";
            return { revoked: true, token_id: session.token_id };
        }
        return { revoked: false, error: "NOT_FOUND" };
    }
}

module.exports = new CounterpartyPortalEngine();

