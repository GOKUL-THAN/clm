/**
 * Enterprise CLM Sourcing-to-Contract (S2C) & RFP-to-Agreement Converter
 * Patterned after SAP Ariba Contracts and Coupa Sourcing-to-Contract.
 * Automates the transition from strategic sourcing awards / RFP bid packages
 * directly into binding Master Services Agreements (MSAs) and Statements of Work (SOWs),
 * mapping line-item pricing tables, milestone schedules, SLAs, and cryptographic provenance.
 */

const crypto = require('crypto');

class SourcingConverterEngine {
    constructor() {
        this.convertedContracts = new Map();
    }

    /**
     * Converts a sourcing bid award into a fully structured, binding contract.
     * @param {Object} sourcingPackage Sourcing award details, supplier, line items, and terms
     * @returns {Object} Structured agreement with line items, clauses, and cryptographic provenance
     */
    convertSourcingAwardToContract(sourcingPackage) {
        if (!sourcingPackage) {
            throw new Error("Sourcing package is required");
        }

        const rfpId = sourcingPackage.rfp_id || "RFP-" + Math.floor(10000 + Math.random() * 90000);
        const supplierName = sourcingPackage.supplier_name || "Enterprise Supplier Ltd";
        const supplierTaxId = sourcingPackage.supplier_tax_id || "US-EIN-987654321";
        const agreementType = sourcingPackage.agreement_type || "STATEMENT_OF_WORK"; // or MASTER_SERVICES_AGREEMENT
        const currency = sourcingPackage.currency || "USD";
        const paymentTerms = sourcingPackage.payment_terms || "Net 30 Days";
        const effectiveDate = sourcingPackage.effective_date || new Date().toISOString().split('T')[0];
        
        // Line Items
        const lineItems = sourcingPackage.line_items || [
            { item_id: "ITEM-001", description: "Core Cloud Platform Implementation", quantity: 1, unit_price: 150000, unit: "lump_sum" },
            { item_id: "ITEM-002", description: "API Integration & Migration Services", quantity: 200, unit_price: 250, unit: "hour" },
            { item_id: "ITEM-003", description: "24x7 Tier-3 Mission Critical Support (Annual)", quantity: 12, unit_price: 5000, unit: "month" }
        ];

        let totalContractValue = 0;
        const normalizedItems = lineItems.map((item, idx) => {
            const lineTotal = (item.quantity || 1) * (item.unit_price || 0);
            totalContractValue += lineTotal;
            return {
                line_number: idx + 1,
                item_id: item.item_id || `LINE-${idx + 1}`,
                description: item.description,
                quantity: item.quantity,
                unit: item.unit || "unit",
                unit_price: item.unit_price,
                line_total: lineTotal
            };
        });

        // Milestones
        const milestones = sourcingPackage.milestones || [
            { milestone_id: "M-01", title: "Architecture & Security Sign-off", target_date: "2026-12-15", percentage: 25, amount: totalContractValue * 0.25 },
            { milestone_id: "M-02", title: "Phase 1 Deployment & UAT", target_date: "2027-02-28", percentage: 50, amount: totalContractValue * 0.50 },
            { milestone_id: "M-03", title: "Final Production Handover", target_date: "2027-04-30", percentage: 25, amount: totalContractValue * 0.25 }
        ];

        // Format Markdown Pricing Table
        let pricingTableMd = `| Line | Item Description | Qty | Unit | Unit Price (${currency}) | Total Price (${currency}) |\n`;
        pricingTableMd += `| :---: | :--- | :---: | :---: | :---: | :---: |\n`;
        for (const itm of normalizedItems) {
            pricingTableMd += `| ${itm.line_number} | ${itm.description} | ${itm.quantity} | ${itm.unit} | $${itm.unit_price.toLocaleString()} | $${itm.line_total.toLocaleString()} |\n`;
        }
        pricingTableMd += `| **TOTAL** | **Aggregate Sourcing Commitment** | | | | **$${totalContractValue.toLocaleString()}** |\n`;

        // Format Milestone Table
        let milestoneTableMd = `| Milestone ID | Milestone Objective | Target Date | Payout % | Payout Amount (${currency}) |\n`;
        milestoneTableMd += `| :---: | :--- | :---: | :---: | :---: |\n`;
        for (const ms of milestones) {
            milestoneTableMd += `| ${ms.milestone_id} | ${ms.title} | ${ms.target_date} | ${ms.percentage}% | $${ms.amount.toLocaleString()} |\n`;
        }

        // Generate Binding Agreement Text
        const contractNumber = `S2C-${new Date().getFullYear()}-${Math.floor(1000 + Math.random() * 9000)}`;
        const contractTitle = `${agreementType === "STATEMENT_OF_WORK" ? "Statement of Work" : "Master Services Agreement"} - ${supplierName} (Ref: ${rfpId})`;
        
        const fullDraftText = `# ${contractTitle}
**Contract Number:** ${contractNumber}
**Sourcing RFP Origin:** ${rfpId}
**Effective Date:** ${effectiveDate}

### PARTIES
This Agreement is entered into between **Enterprise Corporation** ("Buyer") and **${supplierName}** ("Supplier"), Tax ID: ${supplierTaxId}.

### 1. SCOPE OF DELIVERABLES & RATE CARD
Supplier shall provide the goods, software, and services strictly in accordance with the awarded RFP specifications:

${pricingTableMd}

### 2. MILESTONE ACCEPTANCE SCHEDULE
Payment disbursements are strictly contingent upon written Buyer acceptance of each deliverable milestone:

${milestoneTableMd}

### 3. PAYMENT & INVOICING TERMS
All undisputed invoices shall be payable in ${currency} under **${paymentTerms}** following 3-way reconciliation (PO + Goods/Service Receipt + Invoice).

### 4. SERVICE LEVEL AGREEMENT (SLA) & PERFORMANCE COMMITMENTS
Supplier commits to minimum service uptime of 99.9% and response times within 1 hour for Severity-1 incidents as stipulated in RFP ${rfpId}.

### 5. PROVENANCE & GOVERNING LAW
This contract is automatically compiled from certified sourcing award records and governed by the laws of New York State.
`;

        // Cryptographic Sourcing Provenance Digest
        const rfpSourceDigest = crypto.createHash('sha256')
            .update(JSON.stringify({ rfp_id: rfpId, supplier: supplierName, total: totalContractValue, items: normalizedItems }))
            .digest('hex');

        const contractDigest = crypto.createHash('sha256')
            .update(fullDraftText)
            .digest('hex');

        const compiledContract = {
            contract_number: contractNumber,
            title: contractTitle,
            agreement_type: agreementType,
            rfp_id: rfpId,
            rfp_source_digest: rfpSourceDigest,
            contract_digest: contractDigest,
            supplier: {
                name: supplierName,
                tax_id: supplierTaxId
            },
            financials: {
                total_value: totalContractValue,
                currency: currency,
                payment_terms: paymentTerms
            },
            line_items: normalizedItems,
            milestones: milestones,
            generated_content: fullDraftText,
            pricing_table_md: pricingTableMd,
            milestone_table_md: milestoneTableMd,
            provenance_chain: {
                rfp_source_hash: rfpSourceDigest,
                contract_hash: contractDigest,
                transition_engine: "CLM-S2C-CONVERTER-V4",
                timestamp: new Date().toISOString()
            },
            status: "DRAFT_READY_FOR_LEGAL_REVIEW"
        };

        this.convertedContracts.set(contractNumber, compiledContract);
        return compiledContract;
    }

    /**
     * Retrieves a converted contract by number.
     */
    getConvertedContract(contractNumber) {
        return this.convertedContracts.get(contractNumber) || null;
    }
}

module.exports = new SourcingConverterEngine();

