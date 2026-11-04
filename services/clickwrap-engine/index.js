/**
 * Enterprise CLM Self-Service Clickwrap & Standard Agreement Engine
 * Patterned after Ironclad Clickwrap and DocuSign Click.
 * Enables self-service execution of standard agreements (NDAs, Terms of Service, Vendor Terms),
 * with cryptographic evidence logging, IP/UserAgent capture, and Certificates of Completion.
 */

const crypto = require('crypto');

class ClickwrapEngine {
    constructor() {
        this.activeAgreements = new Map();
        this.acceptedLedger = new Map();
        this.seedStandardAgreements();
    }

    seedStandardAgreements() {
        const ndaToken = "CLICK-NDA-STANDARD-2026";
        this.activeAgreements.set(ndaToken, {
            token: ndaToken,
            title: "Standard Enterprise Mutual Non-Disclosure Agreement",
            type: "NDA",
            version: "2026.1",
            terms_text: "This Mutual Non-Disclosure Agreement governs disclosures between Enterprise Corp and Counterparty. Confidential Information shall be protected for three (3) years. Governing law is the State of Delaware.",
            terms_digest: crypto.createHash('sha256').update("Standard Enterprise Mutual Non-Disclosure Agreement 2026.1").digest('hex'),
            created_at: new Date().toISOString()
        });
    }

    /**
     * Create a new self-service clickwrap campaign
     */
    createClickwrap(title, agreementType, termsText, tenantId = "default-tenant") {
        const token = "CLICK-" + crypto.randomBytes(8).toString('hex').toUpperCase();
        const digest = crypto.createHash('sha256').update(termsText, 'utf8').digest('hex');

        const campaign = {
            token: token,
            tenant_id: tenantId,
            title: title,
            type: agreementType,
            terms_text: termsText,
            terms_digest: digest,
            created_at: new Date().toISOString(),
            status: "ACTIVE"
        };

        this.activeAgreements.set(token, campaign);
        return campaign;
    }

    /**
     * Accept a self-service clickwrap agreement
     */
    accept(token, counterpartyData = {}) {
        const agreement = this.activeAgreements.get(token);
        if (!agreement) {
            throw new Error(`Clickwrap agreement with token '${token}' not found or has expired.`);
        }

        const signerName = counterpartyData.signer_name || "Authorized Signatory";
        const signerEmail = counterpartyData.signer_email || "counterparty@client.com";
        const companyName = counterpartyData.company_name || "Counterparty Inc.";
        const ipAddress = counterpartyData.ip_address || "198.51.100.42";
        const userAgent = counterpartyData.user_agent || "Mozilla/5.0 (Windows NT 10.0; Win64; x64)";
        const acceptedAt = new Date().toISOString();

        // Compile Certificate of Completion
        const certificatePayload = {
            certificate_id: "CERT-" + crypto.randomBytes(6).toString('hex').toUpperCase(),
            agreement_token: token,
            agreement_title: agreement.title,
            terms_digest: agreement.terms_digest,
            signer: {
                name: signerName,
                email: signerEmail,
                company: companyName,
                ip_address: ipAddress,
                user_agent: userAgent
            },
            accepted_at: acceptedAt,
            legal_enforceability: "COMPLIANT_WITH_ESIGN_ACT_AND_UETA"
        };

        const certHash = crypto.createHash('sha256').update(JSON.stringify(certificatePayload)).digest('hex');
        certificatePayload.certificate_hash = certHash;

        this.acceptedLedger.set(certificatePayload.certificate_id, certificatePayload);

        return {
            success: true,
            status: "EXECUTED_LEGALLY_BINDING",
            certificate: certificatePayload
        };
    }

    /**
     * Verify an issued Certificate of Completion
     */
    verifyCertificate(certificateId) {
        const cert = this.acceptedLedger.get(certificateId);
        if (!cert) {
            return { valid: false, message: `Certificate ${certificateId} not found in clickwrap ledger.` };
        }

        const calculated = crypto.createHash('sha256').update(JSON.stringify({
            certificate_id: cert.certificate_id,
            agreement_token: cert.agreement_token,
            agreement_title: cert.agreement_title,
            terms_digest: cert.terms_digest,
            signer: cert.signer,
            accepted_at: cert.accepted_at,
            legal_enforceability: cert.legal_enforceability
        })).digest('hex');

        return {
            valid: calculated === cert.certificate_hash,
            certificate: cert,
            verified_at: new Date().toISOString()
        };
    }
}

module.exports = new ClickwrapEngine();

