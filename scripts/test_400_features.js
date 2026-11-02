/**
 * Registry smoke checks for the 400 CLM feature catalog.
 * These checks validate registration and module wrappers, not business capability.
 */
const registry = require('../features/registry.js');

console.log("==============================================================================");
console.log("ENTERPRISE CLM 400-FEATURE REGISTRY SMOKE TEST");
console.log("==============================================================================");

const all = registry.getAll();
let passed = 0;
let failed = 0;
const start = Date.now();

for (const [index, meta] of all.entries()) {
    try {
        const mod = registry.getById(meta.id);
        const domainEntry = registry.getByDomain(meta.domainId).find(feature => feature.id === meta.id);
        const validation = mod.validate({ tenant_id: "tenant-registry-smoke" });
        const invalidValidation = mod.validate({ tenant_id: { invalid: true } });
        const res = registry.execute(meta.id, {
            tenant_id: "tenant-registry-smoke",
            data: { ping: true }
        });
        const auditHash = res.result && res.result.audit && res.result.audit.record_hash;

        if (meta.id === index + 1 &&
            meta.status === "registered" &&
            meta.implementationStatus === "NOT_STARTED" &&
            domainEntry &&
            domainEntry.status === "registered" &&
            domainEntry.implementationStatus === "NOT_STARTED" &&
            mod.id === meta.id &&
            mod.name === meta.name &&
            validation.valid === true &&
            invalidValidation.valid === false &&
            res.success === true &&
            res.tenant_id === "tenant-registry-smoke" &&
            /^[a-f0-9]{64}$/.test(auditHash) &&
            res.verification.level === "registry-smoke" &&
            res.verification.businessCapabilityVerified === false) {
            passed++;
        } else {
            console.error(`Failed registry smoke check for feature #${meta.id}: ${meta.name}`);
            failed++;
        }
    } catch (err) {
        console.error(`Exception in registry smoke check for feature #${meta.id} [${meta.name}]:`, err.message);
        failed++;
    }
}

const elapsed = Date.now() - start;
console.log("==============================================================================");
console.log(`SUMMARY: ${passed}/${all.length} REGISTRY SMOKE CHECKS PASSED (${failed} failed) in ${elapsed}ms`);
if (failed === 0 && all.length === 400 && passed === 400 && new Set(all.map(feature => feature.id)).size === 400) {
    console.log("All 400 catalog entries load and pass wrapper smoke checks; feature functionality is NOT verified.");
} else {
    console.error("Registry smoke checks are incomplete or failed.");
    process.exit(1);
}
console.log("==============================================================================");
