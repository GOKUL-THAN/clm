const assert = require('assert');
const feature = require('./index.js');

try {
    assert.strictEqual(feature.id, 221, "Feature ID must match 221");
    assert.strictEqual(feature.name, "Obligation Creation from Contract", "Feature name must match");
    
    // Test validation
    const valResult = feature.validate({ tenant_id: "test-tenant-123" });
    assert.strictEqual(valResult.valid, true, "Validation should pass for valid payload");

    // Test execution
    const execResult = feature.execute({ tenant_id: "test-tenant-123", data: { sample: true } });
    assert.strictEqual(execResult.success, true, "Execution must succeed");
    assert.strictEqual(execResult.tenant_id, "test-tenant-123", "Tenant ID must be preserved");
    assert.ok(execResult.result.audit.record_hash, "Cryptographic audit hash must be present");

    console.log(`✅ PASS: Feature 221 [${feature.name}]`);
} catch (err) {
    console.error(`❌ FAIL: Feature 221 [${feature.name}]:`, err.message);
    process.exit(1);
}
