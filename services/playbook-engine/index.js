/**
 * Enterprise CLM Legal Clause Playbook & Fallback Hierarchy Engine
 * Patterned after Icertis, Sirion, and Ironclad Enterprise Playbooks.
 * Governs clause standardization, multi-tier fallback concessions, negotiation guidelines,
 * and automated risk variance calculations.
 */

const crypto = require('crypto');

class ClausePlaybookEngine {
    constructor() {
        this.playbook = {
            limitation_of_liability: {
                title: "Limitation of Liability",
                category: "Risk & Liability",
                standard: {
                    tier: "PREFERRED",
                    riskScore: 5,
                    approvalRequired: "L1_STANDARD",
                    text: "Except for breaches of confidentiality, gross negligence, or willful misconduct, in no event shall either party's aggregate liability arising out of or related to this Agreement exceed the total fees paid or payable by Customer in the twelve (12) months preceding the claim. Neither party shall be liable for indirect, incidental, consequential, special, or punitive damages."
                },
                fallback_1: {
                    tier: "FALLBACK_1",
                    riskScore: 20,
                    approvalRequired: "L2_LEGAL_COUNSEL",
                    text: "Except for breaches of confidentiality, gross negligence, or indemnification obligations, each party's aggregate liability under this Agreement shall be capped at two times (2x) the total fees paid by Customer in the twelve (12) months preceding the event giving rise to liability. Mutual waiver of consequential damages applies.",
                    guidance: "Acceptable when counterparty demands a higher cap for mission-critical services or higher transaction values."
                },
                fallback_2: {
                    tier: "FALLBACK_2",
                    riskScore: 40,
                    approvalRequired: "L3_GENERAL_COUNSEL_AND_CFO",
                    text: "Each party's aggregate liability arising out of or relating to this Agreement shall be capped at $1,000,000 USD or the total fees paid under this Agreement, whichever is greater. A super-cap of $5,000,000 applies to data breach and confidentiality claims.",
                    guidance: "Only acceptable with CFO written signoff and corporate insurance verification."
                },
                walkAwayPosition: "Reject any agreement with unlimited general liability or uncapped consequential damages."
            },

            indemnification: {
                title: "Mutual Indemnification",
                category: "Legal Protection",
                standard: {
                    tier: "PREFERRED",
                    riskScore: 10,
                    approvalRequired: "L1_STANDARD",
                    text: "Each party ('Indemnitor') shall defend, indemnify, and hold harmless the other party ('Indemnitee') from and against any third-party claims, damages, liabilities, and expenses arising out of: (a) infringement of any third-party intellectual property right; (b) gross negligence or willful misconduct; or (c) material breach of confidentiality."
                },
                fallback_1: {
                    tier: "FALLBACK_1",
                    riskScore: 25,
                    approvalRequired: "L2_LEGAL_COUNSEL",
                    text: "Provider shall indemnify Customer against third-party claims alleging that the Deliverables infringe a copyright or trade secret, provided Customer gives prompt written notice, sole control of defense, and reasonable cooperation.",
                    guidance: "Acceptable for procurement contracts where indemnity is one-way in favor of Customer."
                },
                fallback_2: {
                    tier: "FALLBACK_2",
                    riskScore: 35,
                    approvalRequired: "L3_GENERAL_COUNSEL",
                    text: "Each party agrees to indemnify the other party for third-party IP claims, with indemnity capped at the aggregate liability cap set forth in Section [Liability].",
                    guidance: "Tying indemnity into the liability cap increases corporate risk; require VP approval."
                },
                walkAwayPosition: "Never indemnify counterparty for their own negligence, misuse of products, or third-party content."
            },

            confidentiality: {
                title: "Confidentiality & Non-Disclosure",
                category: "Data & IP Governance",
                standard: {
                    tier: "PREFERRED",
                    riskScore: 5,
                    approvalRequired: "L1_STANDARD",
                    text: "The receiving party shall hold in confidence and protect all Confidential Information using the same degree of care it uses for its own confidential information, but not less than reasonable care. Confidentiality obligations shall survive termination for a period of five (5) years, except for trade secrets, which shall survive indefinitely."
                },
                fallback_1: {
                    tier: "FALLBACK_1",
                    riskScore: 15,
                    approvalRequired: "L2_LEGAL_COUNSEL",
                    text: "The receiving party shall protect Confidential Information with reasonable care for a term of three (3) years from the date of disclosure. Trade secrets remain protected as long as they qualify as trade secrets under applicable law.",
                    guidance: "Acceptable for standard non-strategic vendor engagements."
                },
                fallback_2: {
                    tier: "FALLBACK_2",
                    riskScore: 25,
                    approvalRequired: "L2_LEGAL_COUNSEL",
                    text: "Confidentiality obligations shall expire two (2) years following the termination or expiration of this Agreement. Trade secret protection is subject to statutory limitation periods.",
                    guidance: "Acceptable only if no source code, algorithms, or highly sensitive PII are shared."
                },
                walkAwayPosition: "Reject clauses that make confidentiality entirely unilateral against our company."
            },

            termination: {
                title: "Term & Termination for Convenience",
                category: "Commercial Operations",
                standard: {
                    tier: "PREFERRED",
                    riskScore: 5,
                    approvalRequired: "L1_STANDARD",
                    text: "Either party may terminate this Agreement for convenience upon sixty (60) days prior written notice. Either party may terminate immediately upon written notice if the other party materially breaches this Agreement and fails to cure such breach within thirty (30) days of receipt of written notice."
                },
                fallback_1: {
                    tier: "FALLBACK_1",
                    riskScore: 15,
                    approvalRequired: "L2_LEGAL_COUNSEL",
                    text: "Customer may terminate for convenience upon ninety (90) days prior written notice. Cure period for material breach is twenty (20) days following notice.",
                    guidance: "Common concession in multi-year service contracts."
                },
                fallback_2: {
                    tier: "FALLBACK_2",
                    riskScore: 30,
                    approvalRequired: "L3_GENERAL_COUNSEL_AND_CFO",
                    text: "Neither party may terminate for convenience prior to the end of the initial commitment term (minimum 12 months). Termination for cause requires 30 days written notice with right to cure.",
                    guidance: "Lock-in term must be approved by Finance to confirm revenue/expense schedule."
                },
                walkAwayPosition: "Reject termination for convenience notice periods less than 15 days or termination fees exceeding unperformed services."
            },

            governing_law: {
                title: "Governing Law & Dispute Resolution",
                category: "Legal Operations",
                standard: {
                    tier: "PREFERRED",
                    riskScore: 5,
                    approvalRequired: "L1_STANDARD",
                    text: "This Agreement shall be governed by and construed in accordance with the laws of the State of New York, USA, without regard to conflict of laws principles. Any legal action or proceeding shall be brought exclusively in state or federal courts located in New York County, New York."
                },
                fallback_1: {
                    tier: "FALLBACK_1",
                    riskScore: 15,
                    approvalRequired: "L2_LEGAL_COUNSEL",
                    text: "This Agreement shall be governed by the laws of the State of Delaware, USA. Any dispute shall be settled by binding arbitration administered by JAMS or AAA in Wilmington, Delaware.",
                    guidance: "Delaware law and arbitration are standard corporate alternatives."
                },
                fallback_2: {
                    tier: "FALLBACK_2",
                    riskScore: 25,
                    approvalRequired: "L3_GENERAL_COUNSEL",
                    text: "This Agreement shall be governed by the laws of England and Wales. Disputes shall be referred to the London Court of International Arbitration (LCIA).",
                    guidance: "Acceptable for European and international counterparties."
                },
                walkAwayPosition: "Reject jurisdiction in non-neutral foreign countries lacking clear legal precedent or arbitration treaty recognition."
            },

            payment_terms: {
                title: "Payment Terms & Invoice Settlement",
                category: "Finance & Billing",
                standard: {
                    tier: "PREFERRED",
                    riskScore: 5,
                    approvalRequired: "L1_STANDARD",
                    text: "Invoices are payable within Net 30 days of receipt via electronic funds transfer (ACH or wire) in USD. Undisputed amounts past due accrue interest at 1.0% per month or the maximum legal rate. Customer must notify disputed amounts within ten (10) business days."
                },
                fallback_1: {
                    tier: "FALLBACK_1",
                    riskScore: 15,
                    approvalRequired: "L2_FINANCE_DIRECTOR",
                    text: "Invoices are payable within Net 45 days of receipt. Late payment interest begins after a ten (10) day grace period at 0.75% per month.",
                    guidance: "Standard concession for enterprise procurement teams requiring 45-day cycles."
                },
                fallback_2: {
                    tier: "FALLBACK_2",
                    riskScore: 30,
                    approvalRequired: "L3_CFO",
                    text: "Invoices are payable within Net 60 days of invoice receipt. No interest shall accrue on late payments unless overdue by more than thirty (30) days.",
                    guidance: "Requires CFO approval due to working capital cash flow delay."
                },
                walkAwayPosition: "Reject Net 90+ days or clauses permitting withholding of undisputed payments."
            }
        };
    }

    /**
     * Retrieve complete clause playbook catalog
     */
    getPlaybook() {
        return Object.entries(this.playbook).map(([key, item]) => ({
            key: key,
            title: item.title,
            category: item.category,
            walk_away_position: item.walkAwayPosition,
            tiers: {
                standard: item.standard,
                fallback_1: item.fallback_1,
                fallback_2: item.fallback_2
            }
        }));
    }

    /**
     * Get specific clause by key
     */
    getClause(clauseKey) {
        return this.playbook[clauseKey] || null;
    }

    /**
     * Dynamically replace clause in contract text with selected fallback tier
     */
    applyFallback(contractText, clauseKey, tier = "fallback_1") {
        const clause = this.playbook[clauseKey];
        if (!clause) {
            throw new Error(`Clause '${clauseKey}' not found in corporate legal playbook.`);
        }

        const selectedTier = clause[tier];
        if (!selectedTier) {
            throw new Error(`Fallback tier '${tier}' is invalid for clause '${clauseKey}'.`);
        }

        // Compute risk delta
        const standardRisk = clause.standard.riskScore;
        const newRisk = selectedTier.riskScore;
        const riskDelta = newRisk - standardRisk;

        const updatedContract = `${contractText}\n\n[AMENDED CLAUSE: ${clause.title} - ${selectedTier.tier}]\n${selectedTier.text}\n`;
        const digest = crypto.createHash('sha256').update(updatedContract, 'utf8').digest('hex');

        return {
            success: true,
            clause_key: clauseKey,
            clause_title: clause.title,
            selected_tier: selectedTier.tier,
            approval_required: selectedTier.approvalRequired,
            risk_score_delta: riskDelta,
            new_clause_text: selectedTier.text,
            guidance: selectedTier.guidance || "Standard approved terms.",
            contract_digest: digest,
            applied_at: new Date().toISOString()
        };
    }
}

module.exports = new ClausePlaybookEngine();

