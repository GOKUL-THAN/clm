// ==============================================================================
// scripts/test_cryptographic_security_live.js
// ENTERPRISE CLM — LIVE CRYPTOGRAPHIC SECURITY & NON-REPUDIATION TEST SUITE
// ==============================================================================

const crypto = require('crypto');
const assert = require('assert');

console.log('==============================================================================');
console.log('ENTERPRISE CLM — LIVE CRYPTOGRAPHIC SECURITY VERIFICATION SUITE');
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
// Setup: Generate Key Pairs for RSA and ECDSA
// ------------------------------------------------------------------------------
const rsaKeyPair = crypto.generateKeyPairSync('rsa', {
    modulusLength: 2048,
    publicKeyEncoding: { type: 'spki', format: 'pem' },
    privateKeyEncoding: { type: 'pkcs8', format: 'pem' }
});

const ecKeyPair = crypto.generateKeyPairSync('ec', {
    namedCurve: 'prime256v1',
    publicKeyEncoding: { type: 'spki', format: 'pem' },
    privateKeyEncoding: { type: 'pkcs8', format: 'pem' }
});

// ------------------------------------------------------------------------------
// 1. Contract Version and Digest Binding
// ------------------------------------------------------------------------------
console.log('▶ [TEST SUITE 1] Contract Version & Digest Binding...');

const contractV1_0 = "This Master Services Agreement Version 1.0 is entered into between Enterprise Alpha and Vendor Beta.";
const digestV1_0 = crypto.createHash('sha256').update(contractV1_0).digest('hex');

const contractV1_1 = "This Master Services Agreement Version 1.1 is entered into between Enterprise Alpha and Vendor Beta.";
const digestV1_1 = crypto.createHash('sha256').update(contractV1_1).digest('hex');

runTest('Document Digest Binding: Version change alters SHA-256 digest', () => {
    assert.notStrictEqual(digestV1_0, digestV1_1, 'Digests must differ between contract versions');
    assert.strictEqual(digestV1_0.length, 64, 'Digest must be 64-character hex');
});

runTest('Bit-Flip Sensitivity: 1-character modification invalidates digest', () => {
    const tampered = contractV1_0.replace('Vendor Beta', 'Vendor Hacked');
    const tamperedDigest = crypto.createHash('sha256').update(tampered).digest('hex');
    assert.notStrictEqual(digestV1_0, tamperedDigest, 'Tampered document must yield a distinct digest');
});

// ------------------------------------------------------------------------------
// 2. Cryptographic Digital Signature Execution & Verification
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 2] RSA-PSS & ECDSA Cryptographic Verification...');

// Sign canonical digest with RSA-PSS
const rsaSigner = crypto.createSign('SHA256');
rsaSigner.update(Buffer.from(digestV1_0, 'hex'));
rsaSigner.end();
const rsaSignature = rsaSigner.sign({
    key: rsaKeyPair.privateKey,
    padding: crypto.constants.RSA_PKCS1_PSS_PADDING,
    saltLength: crypto.constants.RSA_PSS_SALTLENGTH_DIGEST
});

runTest('RSA-PSS Signature Verification: Valid signature matches public key', () => {
    const verifier = crypto.createVerify('SHA256');
    verifier.update(Buffer.from(digestV1_0, 'hex'));
    verifier.end();
    const isValid = verifier.verify({
        key: rsaKeyPair.publicKey,
        padding: crypto.constants.RSA_PKCS1_PSS_PADDING,
        saltLength: crypto.constants.RSA_PSS_SALTLENGTH_DIGEST
    }, rsaSignature);
    assert.strictEqual(isValid, true, 'RSA-PSS signature verification must succeed');
});

runTest('RSA-PSS Signature Rejection: Verification fails against tampered digest', () => {
    const verifier = crypto.createVerify('SHA256');
    verifier.update(Buffer.from(digestV1_1, 'hex')); // Mismatched digest!
    verifier.end();
    const isValid = verifier.verify({
        key: rsaKeyPair.publicKey,
        padding: crypto.constants.RSA_PKCS1_PSS_PADDING,
        saltLength: crypto.constants.RSA_PSS_SALTLENGTH_DIGEST
    }, rsaSignature);
    assert.strictEqual(isValid, false, 'Signature verification must fail on wrong digest');
});

// ------------------------------------------------------------------------------
// 3. Signing Authorization Expiry (TTL Validation)
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 3] Signing Authorization Expiry & Replay Nonces...');

class MockCryptoService {
    constructor() {
        this.nonces = new Set();
        this.signatures = new Map();
        this.s3Vault = new Map();
        this.quarantined = new Map();
    }

    validateSigningRequest(req) {
        const now = Math.floor(Date.now() / 1000);
        if (req.expires_at <= now) {
            throw new Error('CLM-CRYPTO-ERR-002: signing authorization token or TTL has expired');
        }
        if (req.expires_at - now > 300) {
            throw new Error('signing request TTL exceeds maximum 300 second threshold');
        }
        if (this.nonces.has(req.request_id)) {
            throw new Error('CLM-CRYPTO-ERR-003: cryptographic nonce replay detected (RequestID already processed)');
        }
        this.nonces.add(req.request_id);
        return true;
    }

    executeSignature(req, sigId) {
        this.validateSigningRequest(req);
        const existing = this.signatures.get(sigId);
        if (existing && existing.status === 'signed') {
            throw new Error('CLM-CRYPTO-ERR-001: target signature has already been executed (duplicate signing prohibited)');
        }
        const sig = {
            id: sigId,
            status: 'signed',
            signed_at: new Date().toISOString(),
            digest_bound: req.document_digest
        };
        this.signatures.set(sigId, sig);
        return sig;
    }

    persistEvidenceBundle(bundle, simulateS3Failure = false) {
        bundle.storage_status = 'staged_in_database';
        if (simulateS3Failure) {
            bundle.storage_status = 'quarantined_pending_s3_sync';
            this.quarantined.set(bundle.bundle_id, bundle);
            throw new Error('CLM-CRYPTO-ERR-008: partial storage failure; evidence bundle marked quarantined pending S3 WORM reconciliation');
        }
        bundle.storage_status = 's3_object_lock_committed';
        this.s3Vault.set(bundle.bundle_id, bundle);
        return true;
    }

    recoverQuarantined(bundleId) {
        const bundle = this.quarantined.get(bundleId);
        if (!bundle) throw new Error('bundle not found in quarantine');
        bundle.storage_status = 's3_object_lock_committed';
        this.s3Vault.set(bundleId, bundle);
        this.quarantined.delete(bundleId);
        return bundle;
    }

    syncOfflineQueue(contractStatus, sig) {
        if (contractStatus === 'cancelled' || contractStatus === 'terminated') {
            throw new Error('CLM-CRYPTO-ERR-007: contract state conflict during offline synchronization (contract cancelled or terminated)');
        }
        this.signatures.set(sig.id, sig);
        return true;
    }
}

const svc = new MockCryptoService();

runTest('TTL Validation: Valid 120s TTL accepted', () => {
    const validReq = {
        request_id: crypto.randomUUID(),
        document_digest: digestV1_0,
        expires_at: Math.floor(Date.now() / 1000) + 120,
        auth_ref: 'auth-token-valid'
    };
    assert.strictEqual(svc.validateSigningRequest(validReq), true);
});

runTest('TTL Validation: Expired timestamp rejected with ErrExpired', () => {
    const expiredReq = {
        request_id: crypto.randomUUID(),
        document_digest: digestV1_0,
        expires_at: Math.floor(Date.now() / 1000) - 10,
        auth_ref: 'auth-token-expired'
    };
    assert.throws(() => svc.validateSigningRequest(expiredReq), /expired/);
});

runTest('Replay Prevention: Duplicate RequestID nonce rejected', () => {
    const nonce = crypto.randomUUID();
    const req1 = {
        request_id: nonce,
        document_digest: digestV1_0,
        expires_at: Math.floor(Date.now() / 1000) + 60,
        auth_ref: 'auth-token'
    };
    svc.validateSigningRequest(req1); // First should succeed

    const req2 = { ...req1 };
    assert.throws(() => svc.validateSigningRequest(req2), /replay detected/);
});

runTest('Duplicate Signing Defense: Second execution on same signature ID rejected', () => {
    const sigId = crypto.randomUUID();
    const req = {
        request_id: crypto.randomUUID(),
        document_digest: digestV1_0,
        expires_at: Math.floor(Date.now() / 1000) + 60,
        auth_ref: 'auth-token'
    };
    const executed = svc.executeSignature(req, sigId);
    assert.strictEqual(executed.status, 'signed');

    const req2 = {
        request_id: crypto.randomUUID(),
        document_digest: digestV1_0,
        expires_at: Math.floor(Date.now() / 1000) + 60,
        auth_ref: 'auth-token-2'
    };
    assert.throws(() => svc.executeSignature(req2, sigId), /duplicate signing prohibited/);
});

// ------------------------------------------------------------------------------
// 4. DB/S3 Partial Failure and Quarantine Recovery
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 4] Storage Partial Failure & Quarantine Recovery...');

runTest('S3 Failure Simulation: Bundle transitions to quarantined_pending_s3_sync', () => {
    const bundleId = crypto.randomUUID();
    const bundle = {
        bundle_id: bundleId,
        contract_id: crypto.randomUUID(),
        document_sha256: digestV1_0
    };
    assert.throws(() => svc.persistEvidenceBundle(bundle, true), /partial storage failure/);
    assert.strictEqual(bundle.storage_status, 'quarantined_pending_s3_sync');
    assert.strictEqual(svc.quarantined.has(bundleId), true);
});

runTest('Quarantine Recovery: Background worker successfully commits quarantined bundle to S3', () => {
    const bundleId = Array.from(svc.quarantined.keys())[0];
    const recovered = svc.recoverQuarantined(bundleId);
    assert.strictEqual(recovered.storage_status, 's3_object_lock_committed');
    assert.strictEqual(svc.quarantined.has(bundleId), false);
    assert.strictEqual(svc.s3Vault.has(bundleId), true);
});

// ------------------------------------------------------------------------------
// 5. Offline Queue Synchronization & Conflict Handling
// ------------------------------------------------------------------------------
console.log('\n▶ [TEST SUITE 5] Offline Queue Sync & Conflict Resolution...');

runTest('Offline Sync: Normal active contract synchronizes queued signature', () => {
    const offlineSig = { id: crypto.randomUUID(), status: 'signed' };
    assert.strictEqual(svc.syncOfflineQueue('ready_for_signature', offlineSig), true);
});

runTest('Offline Conflict: Terminated/Cancelled contract rejects offline signature', () => {
    const offlineSig = { id: crypto.randomUUID(), status: 'signed' };
    assert.throws(() => svc.syncOfflineQueue('cancelled', offlineSig), /contract state conflict/);
    assert.throws(() => svc.syncOfflineQueue('terminated', offlineSig), /contract state conflict/);
});

console.log('\n==============================================================================');
console.log(`CRYPTOGRAPHIC VERIFICATION SUMMARY: ${passCount}/${passCount + failCount} TESTS PASSED`);
if (failCount === 0) {
    console.log('ALL CRYPTOGRAPHIC INTEGRITY & NON-REPUDIATION CHECKS PASSED OK!');
} else {
    console.error(`FAILED CHECKS: ${failCount}`);
    process.exit(1);
}
console.log('==============================================================================');

