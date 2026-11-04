/**
 * Enterprise CLM Contract Comparison & Semantic Diff Engine
 * Patterned after Ironclad and Sirion side-by-side comparison.
 * Compares two contract versions (Baseline vs Revised or Internal vs Counterparty draft),
 * calculates similarity percentage, identifies clause-level deviations, and flags risk deltas.
 */

const crypto = require('crypto');
const aiEngine = require('../ai-intelligence');

class ContractComparisonEngine {
    constructor() {}

    /**
     * Tokenize text into words/tokens
     */
    tokenize(text) {
        return (text || '').toLowerCase().match(/\b[a-z0-9_-]+\b/g) || [];
    }

    /**
     * Compute Jaccard token similarity score between 0.0 and 1.0
     */
    computeSimilarity(tokensA, tokensB) {
        const setA = new Set(tokensA);
        const setB = new Set(tokensB);

        if (setA.size === 0 && setB.size === 0) return 1.0;
        if (setA.size === 0 || setB.size === 0) return 0.0;

        let intersection = 0;
        for (const token of setA) {
            if (setB.has(token)) intersection++;
        }

        const union = new Set([...setA, ...setB]).size;
        return parseFloat((intersection / union).toFixed(4));
    }

    /**
     * Side-by-side comprehensive contract comparison
     */
    compare(contractA, contractB, titleA = "Baseline Draft (v1.0)", titleB = "Revised Counterparty Draft (v2.0)") {
        const textA = typeof contractA === 'string' ? contractA : contractA?.content_body || '';
        const textB = typeof contractB === 'string' ? contractB : contractB?.content_body || '';

        const tokensA = this.tokenize(textA);
        const tokensB = this.tokenize(textB);
        const similarityScore = this.computeSimilarity(tokensA, tokensB);

        // Clause analysis on both versions
        const clausesA = aiEngine.extractClauses(textA);
        const clausesB = aiEngine.extractClauses(textB);

        const mapA = new Map(clausesA.map(c => [c.type, c]));
        const mapB = new Map(clausesB.map(c => [c.type, c]));

        const allClauseTypes = new Set([...mapA.keys(), ...mapB.keys()]);
        const clauseDeviations = [];

        for (const type of allClauseTypes) {
            const inA = mapA.has(type);
            const inB = mapB.has(type);

            if (inA && inB) {
                const confDiff = Math.abs(mapA.get(type).confidence - mapB.get(type).confidence);
                clauseDeviations.push({
                    clause: type,
                    status: confDiff > 0.05 ? "MODIFIED_SUBSTANTIVE" : "PRESERVED_ALIGNED",
                    in_baseline: true,
                    in_revised: true,
                    variance_degree: confDiff > 0.05 ? "HIGH_ATTENTION" : "LOW_DRIFT"
                });
            } else if (inA && !inB) {
                clauseDeviations.push({
                    clause: type,
                    status: "REMOVED_BY_COUNTERPARTY",
                    in_baseline: true,
                    in_revised: false,
                    variance_degree: "CRITICAL_MISSING"
                });
            } else if (!inA && inB) {
                clauseDeviations.push({
                    clause: type,
                    status: "NEWLY_INTRODUCED",
                    in_baseline: false,
                    in_revised: true,
                    variance_degree: "ATTENTION_REQUIRED"
                });
            }
        }

        // Risk evaluations
        const riskA = aiEngine.evaluateRisk({ title: titleA, content_body: textA, total_value: contractA?.total_value || 100000 });
        const riskB = aiEngine.evaluateRisk({ title: titleB, content_body: textB, total_value: contractB?.total_value || 100000 });
        const riskDelta = riskB.overall_score - riskA.overall_score;

        return {
            comparison_id: "CMP-" + crypto.randomBytes(6).toString('hex').toUpperCase(),
            metadata: {
                version_a_title: titleA,
                version_b_title: titleB,
                hash_a: crypto.createHash('sha256').update(textA).digest('hex'),
                hash_b: crypto.createHash('sha256').update(textB).digest('hex'),
                compared_at: new Date().toISOString()
            },
            similarity: {
                score: similarityScore,
                percentage: (similarityScore * 100).toFixed(1) + "%",
                alignment: similarityScore >= 0.85 ? "HIGH_ALIGNMENT" : similarityScore >= 0.60 ? "MODERATE_VARIANCE" : "SUBSTANTIAL_REWRITE"
            },
            risk_variance: {
                baseline_risk_score: riskA.overall_score,
                revised_risk_score: riskB.overall_score,
                risk_delta: riskDelta,
                risk_trajectory: riskDelta > 0 ? "INCREASED_EXPOSURE" : riskDelta < 0 ? "REDUCED_RISK" : "NEUTRAL"
            },
            clause_deviations: clauseDeviations,
            word_counts: {
                baseline_words: tokensA.length,
                revised_words: tokensB.length,
                net_word_change: tokensB.length - tokensA.length
            }
        };
    }
}

module.exports = new ContractComparisonEngine();

