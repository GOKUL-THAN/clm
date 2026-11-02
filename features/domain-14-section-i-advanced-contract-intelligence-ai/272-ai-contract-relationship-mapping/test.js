const assert = require('assert');
const feature = require('./index.js');

try {
    assert.strictEqual(feature.id, 272, "Feature ID must match 272");
    assert.strictEqual(feature.name, "AI Contract Relationship Mapping", "Feature name must match");
    
    // Test validation
    const valResult = feature.validate({ tenant_id: "test-tenant-123" });
    assert.strictEqual(valResult.valid, true, "Validation should pass for valid payload");

    // Test execution
    const execResult = feature.execute({ tenant_id: "test-tenant-123", data: { sample: true } });
    assert.strictEqual(execResult.success, true, "Execution must succeed");
    assert.strictEqual(execResult.tenant_id, "test-tenant-123", "Tenant ID must be preserved");
    assert.ok(execResult.result.audit.record_hash, "Cryptographic audit hash must be present");

    console.log(`✅ PASS: Feature 272 [${feature.name}]`);
} catch (err) {
    console.error(`❌ FAIL: Feature 272 [${feature.name}]:`, err.message);
    process.exit(1);
}
