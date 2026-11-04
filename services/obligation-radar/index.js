/**
 * Enterprise CLM Post-Execution Obligation Radar & SLA Penalty Calculator
 * Patterned after Sirion and Icertis post-signature contract performance systems.
 * Tracks expiration milestones, auto-renewal opt-out cutoffs, SLA availability penalties,
 * and compliance fulfillment audit trails.
 */

const crypto = require('crypto');

class ObligationRadarEngine {
    constructor() {}

    /**
     * Scan contract portfolio and generate expiration & renewal radar
     */
    generateRadar(contracts = []) {
        const now = Date.now();
        const radar = {
            urgent_30_days: [],
            renewal_window_60_days: [],
            early_warning_90_days: [],
            stable_active: [],
            auto_renewal_cutoffs: []
        };

        contracts.forEach(cnt => {
            if (!cnt.expiration_date) return;
            const expTime = new Date(cnt.expiration_date).getTime();
            const daysLeft = Math.ceil((expTime - now) / (1000 * 60 * 60 * 24));
            const noticeDays = cnt.renewal_notice_days || 30;

            const item = {
                id: cnt.id,
                contract_number: cnt.contract_number,
                title: cnt.title,
                counterparty: cnt.counterparty_name,
                days_left: daysLeft,
                expiration_date: cnt.expiration_date,
                renewal_type: cnt.renewal_type || 'manual',
                total_value: cnt.total_value,
                currency: cnt.currency || 'USD'
            };

            // Check auto-renewal opt-out cutoff
            if (cnt.renewal_type === 'auto_renewal' && daysLeft <= noticeDays && daysLeft > 0) {
                radar.auto_renewal_cutoffs.push({
                    ...item,
                    notice_deadline_days: noticeDays,
                    alert: `ACTION REQUIRED: Auto-renewal opt-out notice deadline expires in ${daysLeft} days! If not cancelled, agreement will renew for an additional term.`
                });
            }

            if (daysLeft <= 30 && daysLeft >= 0) {
                radar.urgent_30_days.push(item);
            } else if (daysLeft <= 60 && daysLeft > 30) {
                radar.renewal_window_60_days.push(item);
            } else if (daysLeft <= 90 && daysLeft > 60) {
                radar.early_warning_90_days.push(item);
            } else if (daysLeft > 90) {
                radar.stable_active.push(item);
            }
        });

        return {
            radar_timestamp: new Date().toISOString(),
            total_contracts_scanned: contracts.length,
            counts: {
                urgent_30_days: radar.urgent_30_days.length,
                renewal_window_60_days: radar.renewal_window_60_days.length,
                early_warning_90_days: radar.early_warning_90_days.length,
                stable_active: radar.stable_active.length,
                auto_renewal_cutoffs: radar.auto_renewal_cutoffs.length
            },
            radar: radar
        };
    }

    /**
     * Compute automated contractual SLA uptime penalty and fee credit rebate
     */
    calculateSLAPenalty(contractValue, targetUptime = 99.9, actualUptime = 98.5, monthlyCycle = true) {
        const baseValue = parseFloat(contractValue) || 120000;
        const monthlyBase = monthlyCycle ? (baseValue / 12) : baseValue;
        const deficit = targetUptime - actualUptime;

        let creditPercentage = 0.0;
        let breachSeverity = "NONE";
        let rightToTerminate = false;

        if (deficit <= 0) {
            creditPercentage = 0.0;
            breachSeverity = "COMPLIANT";
        } else if (actualUptime >= 99.0 && actualUptime < targetUptime) {
            creditPercentage = 0.05; // 5% fee credit
            breachSeverity = "MINOR_SLA_BREACH";
        } else if (actualUptime >= 95.0 && actualUptime < 99.0) {
            creditPercentage = 0.15; // 15% fee credit
            breachSeverity = "MAJOR_SLA_BREACH";
        } else {
            creditPercentage = 0.30; // 30% fee credit
            breachSeverity = "CRITICAL_SYSTEMIC_BREACH";
            rightToTerminate = true; // Material breach clause triggers termination
        }

        const penaltyAmount = parseFloat((monthlyBase * creditPercentage).toFixed(2));

        return {
            target_uptime_percentage: targetUptime,
            actual_uptime_percentage: actualUptime,
            uptime_deficit: parseFloat(deficit.toFixed(2)),
            breach_severity: breachSeverity,
            monthly_billable_base: monthlyBase,
            credit_rebate_percentage: (creditPercentage * 100) + "%",
            calculated_penalty_rebate: penaltyAmount,
            right_to_terminate_for_cause: rightToTerminate,
            erp_adjustment_code: "SLA_CREDIT_MEMO",
            calculated_at: new Date().toISOString()
        };
    }

    /**
     * Record and cryptographically seal proof of milestone fulfillment
     */
    fulfillObligation(obligationId, verifiedByUser, proofNotes, documentDigest = null) {
        const auditDigest = crypto.createHash('sha256')
            .update(`${obligationId}-${verifiedByUser}-${Date.now()}-${proofNotes}`)
            .digest('hex');

        return {
            obligation_id: obligationId,
            status: "FULFILLED",
            verified_by: verifiedByUser,
            proof_notes: proofNotes,
            proof_document_digest: documentDigest || crypto.randomBytes(32).toString('hex'),
            audit_record_hash: auditDigest,
            fulfilled_at: new Date().toISOString()
        };
    }
}

module.exports = new ObligationRadarEngine();

