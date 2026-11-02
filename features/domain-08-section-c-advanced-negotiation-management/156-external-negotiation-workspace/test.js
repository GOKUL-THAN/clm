const assert = require('assert');
const feature = require('./index.js');

try {
    assert.strictEqual(feature.id, 156, "Feature ID must match 156");
    assert.strictEqual(feature.name, "External Negotiation Workspace", "Feature name must match");
    
    // Test validation
    const valResult = feature.validate({ tenant_id: "test-tenant-123" });
    assert.strictEqual(valResult.valid, true, "Validation should pass for valid payload");

    // Test execution
    const execResult = feature.execute({ tenant_id: "test-tenant-123", data: { sample: true } });
    assert.strictEqual(execResult.success, true, "Execution must succeed");
    assert.strictEqual(execResult.tenant_id, "test-tenant-123", "Tenant ID must be preserved");
    assert.ok(execResult.result.audit.record_hash, "Cryptographic audit hash must be present");

    console.log(`✅ PASS: Feature 156 [${feature.name}]`);
} catch (err) {
    console.error(`❌ FAIL: Feature 156 [${feature.name}]:`, err.message);
    process.exit(1);
}
