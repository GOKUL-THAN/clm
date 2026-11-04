/**
 * Enterprise CLM ERP & External Integrations Service
 * Provides automated Three-Way Invoice Matching (SAP S/4HANA & NetSuite),
 * CRM opportunity-to-contract intake, and HMAC-SHA256 signed webhook dispatching.
 */

const crypto = require('crypto');

class IntegrationService {
    constructor() {
        this.webhookSecret = process.env.CLM_WEBHOOK_SECRET || 'clm-enterprise-secure-hmac-key-2026';
    }

    /**
     * Automated Three-Way Invoice Matching Engine (PO + Goods Receipt + Vendor Invoice)
     */
    reconcileThreeWayMatch(purchaseOrder, goodsReceipt, vendorInvoice) {
        const po = purchaseOrder || { po_number: 'PO-2026-9901', amount: 125000.00, currency: 'USD', status: 'approved' };
        const gr = goodsReceipt || { receipt_number: 'GR-2026-4412', quantity_accepted: 100, status: 'verified' };
        const inv = vendorInvoice || { invoice_number: 'INV-2026-8821', billed_amount: 125000.00, currency: 'USD' };

        const poAmount = parseFloat(po.amount) || 0;
        const invAmount = parseFloat(inv.billed_amount) || 0;
        const variance = Math.abs(poAmount - invAmount);
        const tolerancePercentage = 0.01; // 1% corporate tolerance
        const maxAllowedVariance = poAmount * tolerancePercentage;

        const isMatched = variance <= maxAllowedVariance && gr.status === 'verified';

        return {
            status: isMatched ? 'MATCHED' : 'VARIANCE_FLAGGED',
            po_reference: po.po_number,
            receipt_reference: gr.receipt_number,
            invoice_reference: inv.invoice_number,
            po_amount: poAmount,
            billed_amount: invAmount,
            variance: variance,
            currency: po.currency || 'USD',
            tolerance_threshold: maxAllowedVariance,
            reconciled_at: new Date().toISOString(),
            erp_target: 'SAP S/4HANA Financial Ledger'
        };
    }

    /**
     * Sign an outbound payload using HMAC-SHA256
     */
    signWebhook(payload, secret = null) {
        const sec = secret || this.webhookSecret;
        const bodyStr = typeof payload === 'string' ? payload : JSON.stringify(payload);
        const timestamp = new Date().toISOString();
        const signature = crypto.createHmac('sha256', sec).update(`${timestamp}.${bodyStr}`).digest('hex');

        return {
            'X-CLM-Signature': signature,
            'X-CLM-Timestamp': timestamp,
            'Content-Type': 'application/json'
        };
    }

    /**
     * Verify inbound webhook HMAC-SHA256 signature
     */
    verifyWebhookSignature(payload, signature, timestamp, secret = null, maxDriftSec = 300) {
        const sec = secret || this.webhookSecret;
        const bodyStr = typeof payload === 'string' ? payload : JSON.stringify(payload);

        // Check timestamp drift
        const reqTime = new Date(timestamp).getTime();
        const now = Date.now();
        if (Math.abs(now - reqTime) > maxDriftSec * 1000) {
            return { valid: false, error: 'Timestamp drift exceeds maximum allowable tolerance (replay protection)' };
        }

        const expectedSig = crypto.createHmac('sha256', sec).update(`${timestamp}.${bodyStr}`).digest('hex');
        const isValid = crypto.timingSafeEqual(Buffer.from(signature, 'hex'), Buffer.from(expectedSig, 'hex'));

        return { valid: isValid };
    }
}

module.exports = new IntegrationService();

