/**
 * Enterprise CLM Visual Contract Risk Heatmap & Governance Cockpit Engine
 * Patterned after Evisort and Icertis Executive Risk Intelligence.
 * Evaluates multidimensional portfolio risk vectors across 5 core risk categories,
 * computes departmental exposure distributions, identifies high-risk outliers,
 * and calculates enterprise governance compliance indices.
 */

const crypto = require('crypto');

class RiskHeatmapEngine {
    constructor() {
        this.riskCategories = [
            { key: "liability_exposure", name: "Liability & Damages", weight: 0.25 },
            { key: "ip_indemnification", name: "IP & Indemnification", weight: 0.25 },
            { key: "data_privacy_compliance", name: "Privacy & Compliance", weight: 0.20 },
            { key: "financial_commitments", name: "Financial & Spend", weight: 0.15 },
            { key: "operational_termination", name: "Operational & Exit", weight: 0.15 }
        ];
    }

    /**
     * Evaluates the risk profile of an individual contract across 5 risk vectors.
     * @param {Object} contract Contract entity or draft text
     * @returns {Object} Multidimensional risk vector scoring and tier
     */
    evaluateContractRisk(contract) {
        const text = (contract.content || contract.contract_text || "").toLowerCase();
        const scores = {};

        // 1. Liability Exposure
        let liabilityScore = 15; // baseline low risk
        if (text.includes("unlimited liability") || text.includes("without limitation")) liabilityScore += 65;
        if (text.includes("consequential damages") && !text.includes("waiver of consequential damages")) liabilityScore += 25;
        if (text.includes("gross negligence")) liabilityScore += 15;
        scores.liability_exposure = Math.min(100, liabilityScore);

        // 2. IP & Indemnification
        let ipScore = 15;
        if (text.includes("sole and exclusive property of counterparty") || text.includes("assignment of all ip")) ipScore += 55;
        if (text.includes("indemnify") && text.includes("all claims")) ipScore += 35;
        if (!text.includes("infringement") && !text.includes("intellectual property")) ipScore += 20;
        scores.ip_indemnification = Math.min(100, ipScore);

        // 3. Privacy & Compliance
        let privacyScore = 15;
        if (text.includes("gdpr") || text.includes("ccpa") || text.includes("hipaa")) {
            if (text.includes("24 hours") || text.includes("immediate notice")) privacyScore += 35;
            if (text.includes("cross-border") || text.includes("international transfer")) privacyScore += 25;
        } else {
            privacyScore += 25; // unaddressed compliance
        }
        scores.data_privacy_compliance = Math.min(100, privacyScore);

        // 4. Financial & Spend
        let finScore = 15;
        if (contract.contract_value > 500000) finScore += 35;
        else if (contract.contract_value > 100000) finScore += 20;
        if (text.includes("non-refundable") || text.includes("prepayment")) finScore += 30;
        if (text.includes("price increase") || text.includes("automatic escalation")) finScore += 25;
        scores.financial_commitments = Math.min(100, finScore);

        // 5. Operational & Termination
        let opScore = 15;
        if (text.includes("terminate for convenience") && !text.includes("mutual")) opScore += 35;
        if (text.includes("liquidated damages") || text.includes("sla penalty")) opScore += 35;
        if (text.includes("exclusivity") || text.includes("non-compete")) opScore += 30;
        scores.operational_termination = Math.min(100, opScore);

        // Weighted Overall Score
        let compositeScore = 0;
        for (const cat of this.riskCategories) {
            compositeScore += scores[cat.key] * cat.weight;
        }
        compositeScore = Math.round(compositeScore);

        let riskTier = "LOW";
        if (compositeScore >= 75) riskTier = "CRITICAL";
        else if (compositeScore >= 50) riskTier = "HIGH";
        else if (compositeScore >= 25) riskTier = "MEDIUM";

        return {
            contract_id: contract.id || contract.contract_number || "CNT-UNKNOWN",
            contract_number: contract.contract_number || "CNT-" + Math.floor(1000 + Math.random() * 9000),
            title: contract.title || "Enterprise Agreement",
            department: contract.department || "General Legal",
            contract_value: contract.contract_value || 0,
            composite_risk_score: compositeScore,
            risk_tier: riskTier,
            vectors: scores,
            assessed_at: new Date().toISOString()
        };
    }

    /**
     * Generates an enterprise-wide Portfolio Risk Heatmap Matrix.
     * Aggregates risk vectors across departments and highlights top exposure outliers.
     * @param {Array} contracts Array of contract objects
     * @returns {Object} Comprehensive Heatmap Matrix and Executive Summary
     */
    generateHeatmapMatrix(contracts = []) {
        if (!contracts || contracts.length === 0) {
            // Provide default enterprise contracts portfolio if none provided
            contracts = [
                { id: "cnt-001", contract_number: "CNT-2026-001", title: "Global Cloud SaaS MSA", department: "Engineering", contract_value: 450000, content: "Mutual indemnification. Capped liability at 12 months. GDPR compliant." },
                { id: "cnt-002", contract_number: "CNT-2026-002", title: "Enterprise Hardware Procurement", department: "Procurement", contract_value: 850000, content: "Vendor demands unlimited liability. Non-refundable prepayments. Strict SLAs with liquidated damages." },
                { id: "cnt-003", contract_number: "CNT-2026-003", title: "Customer Enterprise License", department: "Sales", contract_value: 1200000, content: "Standard liability cap. Standard payment terms within 30 days." },
                { id: "cnt-004", contract_number: "CNT-2026-004", title: "External Legal Consultancy SOW", department: "Legal", contract_value: 75000, content: "Confidentiality assured. Hourly billing." },
                { id: "cnt-005", contract_number: "CNT-2026-005", title: "Executive Search & HR Services", department: "HR", contract_value: 120000, content: "Assignment of all IP. Cross-border employee data processing under GDPR." }
            ];
        }

        const evaluatedContracts = contracts.map(c => this.evaluateContractRisk(c));

        // Group by department
        const departments = {};
        for (const evalResult of evaluatedContracts) {
            const dept = evalResult.department;
            if (!departments[dept]) {
                departments[dept] = {
                    department: dept,
                    contract_count: 0,
                    total_value: 0,
                    avg_risk_score: 0,
                    vector_totals: {
                        liability_exposure: 0,
                        ip_indemnification: 0,
                        data_privacy_compliance: 0,
                        financial_commitments: 0,
                        operational_termination: 0
                    },
                    contracts: []
                };
            }
            departments[dept].contract_count++;
            departments[dept].total_value += evalResult.contract_value;
            departments[dept].contracts.push(evalResult);

            for (const cat of this.riskCategories) {
                departments[dept].vector_totals[cat.key] += evalResult.vectors[cat.key];
            }
        }

        // Calculate averages per department
        const departmentMatrix = Object.values(departments).map(dept => {
            const count = dept.contract_count;
            const avgVectors = {};
            let weightedSum = 0;
            for (const cat of this.riskCategories) {
                avgVectors[cat.key] = Math.round(dept.vector_totals[cat.key] / count);
                weightedSum += avgVectors[cat.key] * cat.weight;
            }
            const avgScore = Math.round(weightedSum);
            return {
                department: dept.department,
                contract_count: count,
                total_value: dept.total_value,
                average_risk_score: avgScore,
                risk_tier: avgScore >= 75 ? "CRITICAL" : avgScore >= 50 ? "HIGH" : avgScore >= 25 ? "MEDIUM" : "LOW",
                vector_averages: avgVectors
            };
        });

        // Identify Top Outliers
        const sortedOutliers = [...evaluatedContracts].sort((a, b) => b.composite_risk_score - a.composite_risk_score);
        const highRiskOutliers = sortedOutliers.filter(c => c.composite_risk_score >= 50);

        // Overall Portfolio Metrics
        const totalPortfolioValue = evaluatedContracts.reduce((sum, c) => sum + c.contract_value, 0);
        const overallAvgRisk = Math.round(evaluatedContracts.reduce((sum, c) => sum + c.composite_risk_score, 0) / evaluatedContracts.length);
        const governanceComplianceIndex = Math.max(0, 100 - overallAvgRisk);

        // Cryptographic Hash of the Risk Assessment
        const assessmentDigest = crypto.createHash('sha256')
            .update(JSON.stringify(evaluatedContracts))
            .digest('hex');

        return {
            matrix_id: "RISK-MATRIX-" + Date.now(),
            evaluated_at: new Date().toISOString(),
            portfolio_summary: {
                total_contracts: evaluatedContracts.length,
                total_portfolio_value: totalPortfolioValue,
                overall_portfolio_risk_score: overallAvgRisk,
                governance_compliance_index: governanceComplianceIndex,
                distribution: {
                    critical: evaluatedContracts.filter(c => c.risk_tier === "CRITICAL").length,
                    high: evaluatedContracts.filter(c => c.risk_tier === "HIGH").length,
                    medium: evaluatedContracts.filter(c => c.risk_tier === "MEDIUM").length,
                    low: evaluatedContracts.filter(c => c.risk_tier === "LOW").length
                }
            },
            department_matrix: departmentMatrix,
            top_risk_outliers: highRiskOutliers.slice(0, 5),
            all_evaluated_contracts: evaluatedContracts,
            assessment_digest: assessmentDigest
        };
    }
}

module.exports = new RiskHeatmapEngine();
