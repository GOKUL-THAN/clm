/**
 * Enterprise CLM Document Processing & Assembly Engine
 * Provides dynamic contract drafting, token substitution, word-level redline diffing,
 * and document integrity verification.
 */

const crypto = require('crypto');

class DocumentEngine {
    constructor() {
        this.supportedFormats = ['TXT', 'MD', 'HTML', 'DOCX_XML'];
    }

    /**
     * Compute SHA-256 hash of document content
     */
    computeDigest(content) {
        if (typeof content !== 'string') {
            content = JSON.stringify(content);
        }
        return crypto.createHash('sha256').update(content, 'utf8').digest('hex');
    }

    /**
     * Dynamically assemble contract from template and variables
     */
    assemble(templateText, variables = {}, clauses = []) {
        if (!templateText || typeof templateText !== 'string') {
            throw new Error('Template text must be a non-empty string');
        }

        let assembled = templateText;

        // 1. Substitute variables (e.g. {{COUNTERPARTY_NAME}}, {{EFFECTIVE_DATE}}, {{TOTAL_VALUE}})
        for (const [key, val] of Object.entries(variables)) {
            const regex = new RegExp(`\\{\\{\\s*${key}\\s*\\}\\}`, 'g');
            assembled = assembled.replace(regex, String(val));
        }

        // 2. Append selected clauses if provided
        if (Array.isArray(clauses) && clauses.length > 0) {
            assembled += '\n\n---\n### INCORPORATED STANDARD PLAYBOOK CLAUSES\n';
            clauses.forEach((cl, idx) => {
                const title = cl.title || cl.name || `Clause ${idx + 1}`;
                const body = cl.body || cl.text || cl.content || '';
                assembled += `\n#### Section ${idx + 1}. ${title}\n${body}\n`;
            });
        }

        const digest = this.computeDigest(assembled);
        return {
            content: assembled,
            digest: digest,
            timestamp: new Date().toISOString(),
            variable_count: Object.keys(variables).length,
            clause_count: clauses.length
        };
    }

    /**
     * Word-level Redline Diffing Engine
     * Generates atomic diff operations: 'equal', 'insert', 'delete'
     */
    computeRedlineDiff(originalText, revisedText) {
        const origWords = (originalText || '').split(/(\s+)/);
        const revWords = (revisedText || '').split(/(\s+)/);

        const diffs = [];
        let i = 0;
        let j = 0;

        while (i < origWords.length || j < revWords.length) {
            if (i < origWords.length && j < revWords.length && origWords[i] === revWords[j]) {
                diffs.push({ op: 'equal', text: origWords[i] });
                i++;
                j++;
            } else if (j < revWords.length && (i >= origWords.length || !origWords.slice(i, i + 5).includes(revWords[j]))) {
                diffs.push({ op: 'insert', text: revWords[j] });
                j++;
            } else if (i < origWords.length) {
                diffs.push({ op: 'delete', text: origWords[i] });
                i++;
            }
        }

        // Calculate statistics
        const additions = diffs.filter(d => d.op === 'insert' && d.text.trim().length > 0).length;
        const deletions = diffs.filter(d => d.op === 'delete' && d.text.trim().length > 0).length;
        const totalModifications = additions + deletions;

        return {
            diffs: diffs,
            stats: {
                additions: additions,
                deletions: deletions,
                total_modifications: totalModifications,
                variance_percentage: origWords.length > 0 ? ((totalModifications / origWords.length) * 100).toFixed(2) : 0
            },
            timestamp: new Date().toISOString()
        };
    }

    /**
     * Compile sealed Evidence Bundle containing document digest, signatures, and audit trail
     */
    compileEvidenceBundle(contract, signatures = [], auditEvents = []) {
        const bundle = {
            contract_id: contract.id,
            contract_number: contract.contract_number,
            title: contract.title,
            document_digest: contract.digest || this.computeDigest(contract.content_body || contract.title),
            effective_date: contract.effective_date,
            signatories: signatures.map(sig => ({
                signer_name: sig.signer_name,
                signer_email: sig.signer_email,
                signed_at: sig.signed_at,
                signature_digest: sig.signature_digest,
                auth_method: sig.auth_method || 'MFA_EMAIL_OTP'
            })),
            audit_records: auditEvents.slice(-10),
            sealed_at: new Date().toISOString()
        };

        bundle.bundle_hash = this.computeDigest(JSON.stringify(bundle));
        return bundle;
    }
}

module.exports = new DocumentEngine();

