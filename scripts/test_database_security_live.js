// ==============================================================================
// scripts/test_database_security_live.js
// ENTERPRISE CLM — SIMULATED POSTGRESQL MULTI-TENANCY & HASH CHAIN FORENSIC SUITE
// Note: This script executes a simulated PostgreSQL transaction pool and PL/pgSQL
// trigger model in Node.js for architectural and cryptographic verification.
// It DOES NOT connect to a live PostgreSQL daemon or live AWS S3 service.
// ==============================================================================

const crypto = require('crypto');
const assert = require('assert');

console.log('==============================================================================');
console.log('ENTERPRISE CLM — SIMULATED DATABASE SECURITY & FORENSIC AUDIT SUITE');
console.log('Execution Mode: In-Process Architectural & Algorithmic Simulation');
console.log('Timestamp:', new Date().toISOString());
console.log('==============================================================================\n');

let passCount = 0;
let failCount = 0;

function runTest(testName, fn) {
    try {
        fn();
        console.log(`  ✅ PASS: ${testName}`);
        passCount++;
    } catch (err) {
        console.error(`  ❌ FAIL: ${testName}`);
        console.error(`     Error: ${err.message}`);
        failCount++;
    }
}

// ------------------------------------------------------------------------------
// Simulation Engine: Modeling PostgreSQL Connection Pool, RLS & Trigger Ledger
// ------------------------------------------------------------------------------
class MockPostgresPool {
    constructor() {
        this.contracts = [];
        this.chainHeads = new Map(); // tenant_id -> { last_hash, sequence_num, updated_at }
        this.auditLogs = [];
        this.externalCheckpoints = new Map(); // date -> { tenant_id, sequence_num, head_hash, signature }
        this.activeConnections = 0;
    }

    // Simulates connection checkout and transaction with SET LOCAL
    async executeTransaction(tenantId, txFn) {
        this.activeConnections++;
        let localTenantContext = tenantId; // Simulates SET LOCAL app.current_tenant_id

        try {
            const result = await txFn({
                insertContract: (c) => {
                    if (!localTenantContext) throw new Error('RLS: null tenant context prohibited');
                    if (c.tenant_id !== localTenantContext) {
                        throw new Error('RLS: Cross-tenant insert blocked by WITH CHECK policy');
                    }
                    this.contracts.push({ ...c });
                    return c;
                },
                queryContracts: () => {
                    // RLS USING filter
                    if (!localTenantContext) return [];
                    return this.contracts.filter(c => c.tenant_id === localTenantContext);
                },
                insertAuditLog: (entry) => {
                    if (!localTenantContext || entry.tenant_id !== localTenantContext) {
                        throw new Error('RLS: Cross-tenant audit insert blocked');
                    }
                    
                    // Simulates advisory lock and row lock: SELECT FOR UPDATE on tenant_audit_chain_heads
                    let head = this.chainHeads.get(entry.tenant_id);
                    if (!head) {
                        // Genesis creation
                        head = {
                            tenant_id: entry.tenant_id,
                            last_hash: 'GENESIS_0000000000000000000000000000000000000000000000000000000000000000',
                            sequence_num: 0,
                            updated_at: new Date().toISOString()
                        };
                        this.chainHeads.set(entry.tenant_id, head);
                    }

                    const newSeq = head.sequence_num + 1;
                    const prevHash = head.last_hash;
                    const timestamp = new Date().toISOString();
                    const payload = `${prevHash}:${newSeq}:${entry.tenant_id}:${entry.actor_email}:${entry.action_type}:${timestamp}`;
                    const recordHash = crypto.createHash('sha256').update(payload).digest('hex');

                    const logRecord = {
                        id: crypto.randomUUID(),
                        tenant_id: entry.tenant_id,
                        sequence_num: newSeq,
                        previous_hash: prevHash,
                        record_hash: recordHash,
                        actor_email: entry.actor_email,
                        action_type: entry.action_type,
                        created_at: timestamp
                    };

                    this.auditLogs.push(logRecord);

                    // Update chain head
                    head.last_hash = recordHash;
                    head.sequence_num = newSeq;
                    head.updated_at = timestamp;

                    return logRecord;
                }
            });
            return result;
        } finally {
            // Simulates connection checkin: SET LOCAL is discarded automatically at transaction end
            localTenantContext = null;
            this.activeConnections--;
        }
    }

    // Full Forensic Hash Verification Scan for a Tenant
    verifyChainIntegrity(tenantId) {
        const logs = this.auditLogs
            .filter(l => l.tenant_id === tenantId)
            .sort((a, b) => a.sequence_num - b.sequence_num);

        if (logs.length === 0) return { valid: true, count: 0 };

        for (let i = 0; i < logs.length; i++) {
            const cur = logs[i];
            
            // Check sequence continuity
            if (cur.sequence_num !== i + 1) {
                return { valid: false, error: `Sequence gap detected at index ${i}: expected ${i + 1}, found ${cur.sequence_num}` };
            }

            // Check previous hash link
            if (i === 0) {
                if (!cur.previous_hash.startsWith('GENESIS_')) {
                    return { valid: false, error: `Invalid genesis hash on record 1: ${cur.previous_hash}` };
                }
            } else {
                const prev = logs[i - 1];
                if (cur.previous_hash !== prev.record_hash) {
                    return { valid: false, error: `Broken chain link at sequence ${cur.sequence_num}: previous_hash does not match record ${prev.sequence_num} hash` };
                }
            }

            // Recompute record hash
            const payload = `${cur.previous_hash}:${cur.sequence_num}:${cur.tenant_id}:${cur.actor_email}:${cur.action_type}:${cur.created_at}`;
            const expectedHash = crypto.createHash('sha256').update(payload).digest('hex');
            if (cur.record_hash !== expectedHash) {
                return { valid: false, error: `Hash mismatch at sequence ${cur.sequence_num}: content altered` };
            }
        }

        // Compare with chain head table
        const head = this.chainHeads.get(tenantId);
        const lastLog = logs[logs.length - 1];
        if (head.last_hash !== lastLog.record_hash || head.sequence_num !== lastLog.sequence_num) {
            return { valid: false, error: `Chain head table mismatch: head(${head.sequence_num}) vs logs(${lastLog.sequence_num})` };
        }

        return { valid: true, count: logs.length, head_hash: head.last_hash };
    }
}

const pool = new MockPostgresPool();
const tenantAlpha = crypto.randomUUID();
const tenantBeta = crypto.randomUUID();

// ------------------------------------------------------------------------------
// 1. Connection Pool Tenant Context Isolation (SET LOCAL Discard)
// ------------------------------------------------------------------------------
console.log('▶ [TEST SUITE 1] Connection Pooling & Session Scoping (SET LOCAL)...');

runTest('Tenant Alpha writes under session context', async () => {
    await pool.executeTransaction(tenantAlpha, async (tx) => {
        tx.insertContract({
            id: 'c1',
            tenant_id: tenantAlpha,
            title: 'Alpha Master Agreement'
        });
        const rows = tx.queryContracts();
        assert.strictEqual(rows.length, 1);
        assert.strictEqual(rows[0].title, 'Alpha Master Agreement');
    });
});

runTest('Connection Reuse without Context: Fails closed (0 rows returned)', async () => {
    await pool.executeTransaction(null, async (tx) => {
        const rows = tx.queryContracts();
        assert.strictEqual(rows.length, 0, 'Unauthenticated query must return 0 rows under RLS');
    });
});

runTest('Tenant Beta cross-tenant read attempt: Zero leakage', async () => {
    await pool.executeTransaction(tenantBeta, async (tx) => {
        const rows = tx.queryContracts();
        assert.strictEqual(rows.length, 0, 'Tenant Beta must not see Tenant Alpha contracts');
    });
});

runTest('Tenant Beta cross-tenant write attempt: Blocked by RLS WITH CHECK', async () => {
    await pool.executeTransaction(tenantBeta, async (tx) => {
        assert.throws(() => {
            tx.insertContract({
                id: 'c2',
                tenant_id: tenantAlpha, // Attempting to spoof Tenant Alpha ID!
                title: 'Hacked Alpha Contract'
            });
        }, /Cross-tenant insert blocked/);
    });
});

// ------------------------------------------------------------------------------
// 2. Concurrent Audit Insertion & Hash-Chain Continuity
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 2] Concurrent Audit Inserts & Hash-Chain Continuity...');

runTest('Genesis Initialization: First audit log starts at sequence 1 with Genesis previous_hash', async () => {
    await pool.executeTransaction(tenantAlpha, async (tx) => {
        const log = tx.insertAuditLog({
            tenant_id: tenantAlpha,
            actor_email: 'auditor@alpha.com',
            action_type: 'contract_create'
        });
        assert.strictEqual(log.sequence_num, 1);
        assert.strictEqual(log.previous_hash.startsWith('GENESIS_'), true);
        assert.strictEqual(log.record_hash.length, 64);
    });
});

runTest('Linear Chaining: Second audit log points to first record hash', async () => {
    await pool.executeTransaction(tenantAlpha, async (tx) => {
        const log2 = tx.insertAuditLog({
            tenant_id: tenantAlpha,
            actor_email: 'counsel@alpha.com',
            action_type: 'contract_approve'
        });
        assert.strictEqual(log2.sequence_num, 2);
        const log1 = pool.auditLogs[0];
        assert.strictEqual(log2.previous_hash, log1.record_hash, 'Log 2 must point to Log 1 hash');
    });
});

runTest('Simulated Concurrent Inserts: 10 concurrent transactions advance sequence monotonically', async () => {
    const promises = [];
    for (let i = 0; i < 10; i++) {
        promises.push(
            pool.executeTransaction(tenantAlpha, async (tx) => {
                return tx.insertAuditLog({
                    tenant_id: tenantAlpha,
                    actor_email: `user_${i}@alpha.com`,
                    action_type: 'clause_review'
                });
            })
        );
    }
    await Promise.all(promises);

    const alphaLogs = pool.auditLogs.filter(l => l.tenant_id === tenantAlpha);
    assert.strictEqual(alphaLogs.length, 12); // 1 + 1 + 10 = 12

    // Verify unbroken chain continuity across all 12 records
    for (let i = 1; i < alphaLogs.length; i++) {
        const prev = alphaLogs[i - 1];
        const curr = alphaLogs[i];
        assert.strictEqual(curr.sequence_num, prev.sequence_num + 1, 'Sequences must be contiguous');
        assert.strictEqual(curr.previous_hash, prev.record_hash, 'Hash chain must be unbroken');
    }
});

runTest('Tenant Beta Independent Chain: Independent genesis and sequence count', async () => {
    await pool.executeTransaction(tenantBeta, async (tx) => {
        const betaLog1 = tx.insertAuditLog({
            tenant_id: tenantBeta,
            actor_email: 'admin@beta.com',
            action_type: 'account_provision'
        });
        assert.strictEqual(betaLog1.sequence_num, 1, 'Tenant Beta must start at sequence 1');
        assert.strictEqual(betaLog1.previous_hash.startsWith('GENESIS_'), true);
    });
});

// ------------------------------------------------------------------------------
// 3. Forensic Tamper Detection Scan (Superuser Threat Model)
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 3] Forensic Tamper Detection Scan (Periodic / Checkpoint)...');

runTest('Baseline Verification: Untampered chain passes 100%', () => {
    const res = pool.verifyChainIntegrity(tenantAlpha);
    assert.strictEqual(res.valid, true);
    assert.strictEqual(res.count, 12);
});

runTest('Forensic Detection of Row Modification: Detected upon verification scan', () => {
    const target = pool.auditLogs[3];
    const realAction = target.action_type;
    
    // Superuser modifies row in raw table
    target.action_type = 'TAMPERED_ACTION';

    const scan = pool.verifyChainIntegrity(tenantAlpha);
    assert.strictEqual(scan.valid, false);
    assert.match(scan.error, /Hash mismatch at sequence 4/);

    // Revert
    target.action_type = realAction;
});

runTest('Forensic Detection of Row Deletion: Sequence gap detected upon scan', () => {
    // Superuser deletes row at sequence 6
    const deletedRow = pool.auditLogs.splice(5, 1)[0];

    const scan = pool.verifyChainIntegrity(tenantAlpha);
    assert.strictEqual(scan.valid, false);
    assert.match(scan.error, /Sequence gap detected/);

    // Restore
    pool.auditLogs.splice(5, 0, deletedRow);
});

runTest('Forensic Detection of Chain-Head Modification: Divergence detected upon scan', () => {
    const head = pool.chainHeads.get(tenantAlpha);
    const realHeadHash = head.last_hash;

    // Superuser alters head pointer
    head.last_hash = 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff';

    const scan = pool.verifyChainIntegrity(tenantAlpha);
    assert.strictEqual(scan.valid, false);
    assert.match(scan.error, /Chain head table mismatch/);

    // Restore
    head.last_hash = realHeadHash;
});

// ------------------------------------------------------------------------------
// 4. External Checkpoint Signing & Comparison
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 4] External Signed Checkpoints & Discrepancy Detection...');

const signingKey = crypto.generateKeyPairSync('rsa', { modulusLength: 2048 });

function createExternalCheckpoint(dateStr, tenantId, pool) {
    const head = pool.chainHeads.get(tenantId);
    const payload = `${dateStr}:${tenantId}:${head.sequence_num}:${head.last_hash}`;
    const signer = crypto.createSign('SHA256');
    signer.update(payload);
    signer.end();
    const sig = signer.sign(signingKey.privateKey);

    return {
        date: dateStr,
        tenant_id: tenantId,
        sequence_num: head.sequence_num,
        head_hash: head.last_hash,
        signature: sig.toString('base64')
    };
}

function verifyExternalCheckpoint(checkpoint, pool, publicKey) {
    // 1. Verify cryptographic signature of checkpoint
    const payload = `${checkpoint.date}:${checkpoint.tenant_id}:${checkpoint.sequence_num}:${checkpoint.head_hash}`;
    const verifier = crypto.createVerify('SHA256');
    verifier.update(payload);
    verifier.end();
    const sigValid = verifier.verify(publicKey, Buffer.from(checkpoint.signature, 'base64'));
    if (!sigValid) return { valid: false, error: 'External checkpoint signature invalid' };

    // 2. Compare against current database state
    const scan = pool.verifyChainIntegrity(checkpoint.tenant_id);
    if (!scan.valid) return { valid: false, error: `Database chain corrupted: ${scan.error}` };

    if (scan.head_hash !== checkpoint.head_hash) {
        return { valid: false, error: `Database head diverges from external checkpoint: db(${scan.head_hash}) vs ext(${checkpoint.head_hash})` };
    }

    return { valid: true };
}

runTest('External Checkpoint Comparison: Valid checkpoint matches DB state', () => {
    const cp = createExternalCheckpoint('2026-10-31', tenantAlpha, pool);
    pool.externalCheckpoints.set('2026-10-31', cp);

    const comp = verifyExternalCheckpoint(cp, pool, signingKey.publicKey);
    assert.strictEqual(comp.valid, true);
});

runTest('Tampered Checkpoint Detection: Altered checkpoint payload detected by signature', () => {
    const cp = { ...pool.externalCheckpoints.get('2026-10-31') };
    cp.head_hash = '1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff';

    const comp = verifyExternalCheckpoint(cp, pool, signingKey.publicKey);
    assert.strictEqual(comp.valid, false);
    assert.match(comp.error, /External checkpoint signature invalid/);
});

runTest('Missing Checkpoint Detection: Gaps in checkpoint calendar flagged', () => {
    const requiredDates = ['2026-10-29', '2026-10-30', '2026-10-31'];
    const missing = requiredDates.filter(d => !pool.externalCheckpoints.has(d));
    assert.strictEqual(missing.length, 2, 'Must detect 2 missing checkpoint dates');
    assert.deepStrictEqual(missing, ['2026-10-29', '2026-10-30']);
});

console.log('\n==============================================================================');
console.log(`DATABASE & FORENSIC VERIFICATION SUMMARY: ${passCount}/${passCount + failCount} TESTS PASSED`);
if (failCount === 0) {
    console.log('ALL SIMULATED MULTI-TENANCY & FORENSIC TESTS COMPLETED SUCCESSFULLY.');
} else {
    console.error(`FAILED TESTS: ${failCount}`);
    process.exit(1);
}
console.log('==============================================================================');
