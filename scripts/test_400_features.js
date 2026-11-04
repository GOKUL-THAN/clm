/**
 * Registry smoke checks for the 400 CLM feature catalog.
 * These checks do not verify feature business capabilities or production readiness.
 */
const registry = require('../features/registry.js');
const EventEmitter = require('events');
const { handleMockAPI } = require('../dev-server.js');

function postFeatureExecution(payload) {
    const body = JSON.stringify(payload);
    const req = new EventEmitter();
    req.method = 'POST';
    req.url = '/api/v1/features/1/execute';
    req.headers = { 'content-type': 'application/json' };
    req.socket = { remoteAddress: '127.0.0.1' };

    let response;
    const res = {
        statusCode: 200,
        headers: {},
        setHeader(name, value) { this.headers[name.toLowerCase()] = value; },
        writeHead(statusCode) { this.statusCode = statusCode; },
        end(text) {
            response = { statusCode: this.statusCode, body: JSON.parse(text) };
        }
    };

    if (!handleMockAPI(req, res)) {
        throw new Error('Feature execution API route was not handled.');
    }
    req.emit('data', Buffer.from(body));
    req.emit('end');
    return response;
}

console.log("==============================================================================");
console.log("ENTERPRISE CLM 400-FEATURE REGISTRY SMOKE TEST");
console.log("==============================================================================");

const all = registry.getAll();
let passed = 0;
let failed = 0;
const start = Date.now();
let missingTenantRejected = false;
try {
    registry.execute(1, { data: { ping: true } });
} catch (err) {
    missingTenantRejected = err.message.includes("A tenant_id is required");
}
if (!missingTenantRejected) {
    console.error("Registry execution did not reject a missing tenant ID.");
    failed++;
}
const apiMissingTenant = postFeatureExecution({ data: { ping: true } });
const apiWithTenant = postFeatureExecution({
    tenant_id: "tenant-registry-smoke",
    data: { ping: true }
});
if (apiMissingTenant.statusCode !== 400 ||
    !apiMissingTenant.body.error.includes("A tenant_id is required") ||
    apiWithTenant.statusCode !== 200 ||
    apiWithTenant.body.verification?.businessCapabilityVerified !== false) {
    console.error("Feature execution API did not preserve registry-only verification semantics.");
    failed++;
}

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
            meta.implementationStatus === "NOT_ASSESSED" &&
            meta.verificationProfileStatus === "NOT_CONFIGURED" &&
            meta.verificationStatus === "NOT_VERIFIED" &&
            meta.evidenceStatus === "NONE" &&
            meta.lastExecutionAt === null &&
            domainEntry &&
            domainEntry.status === "registered" &&
            domainEntry.implementationStatus === "NOT_ASSESSED" &&
            mod.id === meta.id &&
            mod.name === meta.name &&
            validation.valid === true &&
            invalidValidation.valid === false &&
            res.success === true &&
            res.tenant_id === "tenant-registry-smoke" &&
            /^[a-f0-9]{64}$/.test(auditHash) &&
            res.result.audit.execution_status === "REGISTRY_SMOKE" &&
            res.result.audit.persisted === false &&
            res.result.audit.hash_chained === false &&
            res.verification.level === "registry-smoke" &&
            res.verification.registryStatus === "PASS" &&
            res.verification.functionalStatus === "NOT_VERIFIED" &&
            res.verification.integrationStatus === "NOT_VERIFIED" &&
            res.verification.securityStatus === "NOT_VERIFIED" &&
            res.verification.performanceStatus === "NOT_VERIFIED" &&
            res.verification.acceptanceStatus === "NOT_VERIFIED" &&
            res.verification.businessCapabilityVerified === false &&
            res.verification.productionReady === false &&
            res.verification.evidencePersisted === false &&
            res.verification.executionMode === "domain-simulation") {
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
const uniqueIds = new Set(all.map(feature => feature.id)).size;
console.log("==============================================================================");
console.log(`SUMMARY: ${passed}/${all.length} REGISTRY SMOKE CHECKS PASSED (${failed} failed) in ${elapsed}ms`);
if (failed === 0 && all.length === 400 && uniqueIds === 400) {
    console.log("All 400 registry entries passed wrapper checks; business capabilities remain unverified.");
} else {
    console.error(`Registry smoke checks are incomplete (entries=${all.length}, unique IDs=${uniqueIds}).`);
    process.exitCode = 1;
}
console.log("==============================================================================");
