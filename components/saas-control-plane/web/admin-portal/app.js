// CLM Admin Portal — SaaS Control Plane UI
// Auth: Keycloak (Direct Grant) | Manages: Tenants, Users, Groups, Cloud Accounts, Gateways

const API = window.location.origin;
const KEYCLOAK_TOKEN_URL = window.location.origin + '/auth/realms/clm/protocol/openid-connect/token';
const KEYCLOAK_CLIENT_ID = 'clm-admin-portal';
let token = localStorage.getItem('clm_token') || '';
let currentPage = 'dashboard';

// Cloud accounts are tenant-scoped server records. Credential material is
// submitted only on create/rotate, encrypted by the control plane, and never
// returned to or persisted by this browser.
let cloudAccountsCache = [];
let activeTenantId = '';
localStorage.removeItem('clm_cloud_accounts'); // remove obsolete plaintext entries

function getCloudAccounts() {
    return cloudAccountsCache;
}

function scopedPath(path, tenantId) {
    const separator = path.includes('?') ? '&' : '?';
    return path + separator + 'tenant_id=' + encodeURIComponent(tenantId);
}

function resolveTenantScope() {
    const claims = decodeJwt(token) || {};
    if (claims.tenant_id) {
        activeTenantId = claims.tenant_id;
        return Promise.resolve(activeTenantId);
    }
    if (activeTenantId) return Promise.resolve(activeTenantId);
    return apiGet('/api/v1/tenants').then(data => {
        const tenants = data.tenants || [];
        if (tenants.length !== 1) {
            throw new Error(tenants.length === 0
                ? 'No tenant exists. Create a tenant before managing gateways.'
                : 'Select a tenant before managing gateways.');
        }
        activeTenantId = tenants[0].id;
        return activeTenantId;
    });
}

function defaultRegion(provider) {
    if (provider === 'gcp') return 'asia-south1';
    if (provider === 'azure') return 'centralindia';
    return 'ap-south-1';
}

function tenantApiGet(path) {
    return resolveTenantScope().then(tenantId => apiGet(scopedPath(path, tenantId)));
}

// --- Auth (Keycloak Direct Grant + fallback to CLM API) ---
function doLogin() {
    const email = document.getElementById('login-email').value.trim();
    const password = document.getElementById('login-password').value;
    const errEl = document.getElementById('login-error');
    errEl.textContent = '';

    if (!email || !password) { errEl.textContent = 'Email and password are required.'; return; }

    // Try Keycloak first (Resource Owner Password Grant)
    fetch(KEYCLOAK_TOKEN_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: `grant_type=password&client_id=${KEYCLOAK_CLIENT_ID}&username=${encodeURIComponent(email)}&password=${encodeURIComponent(password)}&scope=openid`
    })
    .then(r => r.json())
    .then(data => {
        if (data.access_token) {
            token = data.access_token;
            localStorage.setItem('clm_token', token);
            if (data.refresh_token) localStorage.setItem('clm_refresh_token', data.refresh_token);
            document.getElementById('login-overlay').classList.add('hidden');
            document.getElementById('user-email').textContent = email;
            setupSidebar();
            loadDashboard();
        } else {
            // Keycloak failed, try CLM API fallback
            fallbackLogin(email, password, errEl);
        }
    })
    .catch(() => {
        // Keycloak unreachable, try CLM API fallback
        fallbackLogin(email, password, errEl);
    });
}

function fallbackLogin(email, password, errEl) {
    fetch(API + '/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
    })
    .then(r => r.json())
    .then(data => {
        if (data.access_token) {
            token = data.access_token;
            localStorage.setItem('clm_token', token);
            document.getElementById('login-overlay').classList.add('hidden');
            document.getElementById('user-email').textContent = email;
            setupSidebar();
            loadDashboard();
        } else {
            errEl.textContent = data.error || 'Invalid email or password.';
        }
    })
    .catch(() => { errEl.textContent = 'Cannot connect to server.'; });
}

function logout() {
    token = '';
    localStorage.removeItem('clm_token');
    localStorage.removeItem('clm_refresh_token');
    window.location.reload();
}

// decodeJwt reads a JWT payload.
//
// JWTs are base64URL encoded and unpadded, but atob() expects standard base64.
// A token whose payload happens to contain '-' or '_' therefore makes atob()
// throw — which is intermittent, because whether those characters appear depends
// on the byte values in the claims. Adding claims to the realm changes the
// encoding and can flip a working token into a failing one.
//
// Getting this wrong was severe: the init path cleared the stored token on a
// decode failure (silent logout), and getUserRole() fell back to 'viewer',
// which hid tenant and identity management from a super_admin.
function decodeJwt(t) {
    if (!t) return null;
    try {
        const parts = t.split('.');
        if (parts.length < 2) return null;
        let b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
        while (b64.length % 4 !== 0) b64 += '=';
        return JSON.parse(decodeURIComponent(
            atob(b64).split('').map(c =>
                '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2)).join('')
        ));
    } catch (e) {
        // Fall back to a plain decode in case the payload is not UTF-8 escaped.
        try {
            let b64 = t.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
            while (b64.length % 4 !== 0) b64 += '=';
            return JSON.parse(atob(b64));
        } catch (e2) {
            console.error('JWT decode failed', e2);
            return null;
        }
    }
}

// --- Init ---
if (token) {
    // Only discard the token when it is genuinely expired. A decode failure is
    // not proof of expiry and must not silently log the user out.
    const payload = decodeJwt(token);
    if (payload && payload.exp && payload.exp * 1000 < Date.now()) {
        token = '';
        localStorage.removeItem('clm_token');
    }
}
if (token) {
    document.getElementById('login-overlay').classList.add('hidden');
    const p = decodeJwt(token) || {};
    document.getElementById('user-email').textContent = p.email || p.preferred_username || '';
    setupSidebar();
    loadDashboard();
}

// --- Role helpers ---
function getUserRole() {
    if (!token) return 'viewer';
    const payload = decodeJwt(token);
    if (!payload) {
        // Do not silently downgrade to viewer: that hides tenant and identity
        // management from an administrator with no visible explanation.
        console.error('Could not read role from token; treating as viewer');
        return 'viewer';
    }
    // Keycloak tokens in this realm carry roles at the top level; realm_access
    // is the standard location. Internal HS256 tokens use a single `role`.
    const roles = payload.roles || (payload.realm_access && payload.realm_access.roles) || [];
    if (Array.isArray(roles)) {
        const priority = ['super_admin','admin','tenant_admin','cert_manager','approver','operator','auditor','viewer'];
        for (const p of priority) { if (roles.indexOf(p) >= 0) return p; }
    }
    if (payload.role) return payload.role;
    return 'viewer';
}

// isPlatformAdmin reports whether the caller may manage tenants and identity.
function isPlatformAdmin() {
    const role = getUserRole();
    return role === 'super_admin' || role === 'admin';
}

function setupSidebar() {
    // Admin-only entries. Hidden rather than disabled, since the API rejects
    // them anyway and a dead menu item is worse than none.
    const adminOnly = ['nav-tenants', 'nav-subscription', 'nav-billing',
                       'nav-keycloak', 'nav-id-users', 'nav-id-groups', 'nav-id-providers'];
    const show = isPlatformAdmin();
    adminOnly.forEach(id => {
        const el = document.getElementById(id);
        if (el) el.classList.toggle('hidden', !show);
    });
}

// --- Navigation ---
function navigate(page) {
    currentPage = page;
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    // Find and activate the clicked nav item (safe for programmatic calls)
    document.querySelectorAll('.nav-item').forEach(n => {
        if (n.textContent.trim().toLowerCase().replace(' ','-') === page) n.classList.add('active');
    });
    document.getElementById('page-title').textContent = page.charAt(0).toUpperCase() + page.slice(1).replace('-', ' ');
    switch (page) {
        case 'dashboard': loadDashboard(); break;
        case 'org-tenants': loadOrgTenants(); break;
        case 'sso': loadSSO(); break;
        case 'cloud-accounts': loadCloudAccounts(); break;
        case 'gateways': loadGateways(); break;
        case 'jobs': loadDeployJobs(); break;
        case 'approvals': loadApprovals(); break;
        case 'onboarding': loadCustomerOnboarding(); break;
        case 'directory-mappings': loadDirectoryMappings(); break;
        case 'departments': loadDepartments(); break;
        case 'itsm': loadITSMWarRoom(); break;
        case 'risk': loadRiskRegister(); break;
        case 'integrations': loadIntegrations(); break;
        case 'activity': loadActivity(); break;
        case 'identity': loadIdentity(); break;
        // Sidebar entries and legacy links that open a specific hub tab.
        case 'organization': loadOrgTenants('organization'); break;
        case 'tenants': loadOrgTenants('tenants'); break;
        case 'subscription': loadOrgTenants('subscription'); break;
        case 'billing': loadOrgTenants('billing'); break;
        case 'users': loadIdentity('users'); break;
        case 'groups': loadIdentity('groups'); break;
        case 'providers': loadIdentity('providers'); break;
    }
}

// --- API helpers ---
function parseApiResponse(response) {
    if (response.status === 401) {
        logout();
        throw new Error('Session expired');
    }
    return response.json().catch(() => ({})).then(data => {
        if (!response.ok) throw new Error(data.error || data.message || ('Request failed with HTTP ' + response.status));
        return data;
    });
}
function apiGet(path) {
    return fetch(API + path, { headers: { 'Authorization': 'Bearer ' + token } }).then(parseApiResponse);
}
function apiPost(path, body, extraHeaders) {
    return fetch(API + path, {
        method: 'POST',
        headers: Object.assign({ 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' }, extraHeaders || {}),
        body: JSON.stringify(body)
    }).then(parseApiResponse);
}
function apiPut(path, body) {
    return fetch(API + path, { method: 'PUT', headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' }, body: JSON.stringify(body) }).then(parseApiResponse);
}
function apiDelete(path) {
    return fetch(API + path, { method: 'DELETE', headers: { 'Authorization': 'Bearer ' + token } }).then(parseApiResponse);
}

function handleSessionExpired() {
    logout();
}

// --- Pages ---
function loadDashboard() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="stats">
            <div class="card stat"><div class="label">Tenants</div><div class="value" id="s-tenants">0</div></div>
            <div class="card stat"><div class="label">Users</div><div class="value" id="s-users">0</div></div>
            <div class="card stat"><div class="label">Gateways</div><div class="value" id="s-sats">0</div></div>
            <div class="card stat"><div class="label">Deploy Jobs</div><div class="value" id="s-jobs">0</div></div>
        </div>
        <div class="card"><h3 style="margin-bottom:0.5rem">CLM Admin Portal</h3><p style="color:var(--muted);font-size:0.85rem">Manage tenants, users, SSO configuration, and deploy satellite gateways to customer environments.</p></div>
    `;
    apiGet('/api/v1/tenants').then(d => { document.getElementById('s-tenants').textContent = (d.tenants||[]).length; }).catch(()=>{});
    tenantApiGet('/api/v1/users').then(d => { document.getElementById('s-users').textContent = (d.users||[]).length; }).catch(()=>{});
    tenantApiGet('/api/v1/satellites').then(d => { document.getElementById('s-sats').textContent = (d.satellites||[]).length; }).catch(()=>{});
    tenantApiGet('/api/v1/satellites/deploy').then(d => { document.getElementById('s-jobs').textContent = (d.jobs||[]).length; }).catch(()=>{});
}

function loadTenants() {
    const el = panelEl();
    el.innerHTML = '<div class="card"><p>Loading tenants...</p></div>';
    if (!document.getElementById('org-body')) document.getElementById('page-title').textContent = 'Tenant Management';

    apiGet('/api/v1/tenants').then(data => {
        const tenants = data.tenants || [];
        let rows = tenants.map(t => {
            const statusBadge = t.status === 'active'
                ? '<span class="badge badge-active">active</span>'
                : t.status === 'suspended'
                ? '<span class="badge badge-warning">suspended</span>'
                : '<span class="badge badge-disabled">' + t.status + '</span>';
            const planBadge = t.plan === 'enterprise'
                ? '<span class="badge" style="background:#7c3aed20;color:#7c3aed;padding:0.2rem 0.4rem">Enterprise</span>'
                : t.plan === 'pro'
                ? '<span class="badge" style="background:#2563eb20;color:#2563eb;padding:0.2rem 0.4rem">Pro</span>'
                : '<span class="badge" style="background:#64748b20;color:#64748b;padding:0.2rem 0.4rem">Free</span>';
            return `<tr>
                <td><strong>${t.name}</strong></td>
                <td><code style="font-size:0.75rem">${t.slug}</code></td>
                <td>${planBadge}</td>
                <td>${statusBadge}</td>
                <td>${t.max_users||5} users / ${t.max_satellites||2} gw / ${t.max_certs||100} certs</td>
                <td>${t.data_region||'ap-south-1'}</td>
                <td style="white-space:nowrap">
                    <select onchange="changeTenantPlan('${t.id}', this.value)" style="font-size:0.7rem;padding:0.15rem;border:1px solid var(--border);border-radius:0.25rem">
                        <option value="" disabled selected>Plan</option>
                        <option value="free">Free</option>
                        <option value="pro">Pro</option>
                        <option value="enterprise">Enterprise</option>
                    </select>
                    ${t.status === 'active'
                        ? `<button class="btn" style="font-size:0.65rem;padding:0.15rem 0.35rem;border:1px solid #f59e0b;color:#f59e0b;margin-left:0.2rem" onclick="toggleTenant('${t.id}','suspend','${t.name}')">Suspend</button>`
                        : `<button class="btn" style="font-size:0.65rem;padding:0.15rem 0.35rem;border:1px solid var(--success);color:var(--success);margin-left:0.2rem" onclick="toggleTenant('${t.id}','activate','${t.name}')">Activate</button>`}
                    <button class="btn btn-danger" style="font-size:0.65rem;padding:0.15rem 0.35rem;margin-left:0.2rem" onclick="deleteTenant('${t.id}','${t.name}')">Delete</button>
                </td>
            </tr>`;
        }).join('');

        el.innerHTML = `<div class="card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Tenant Management</h3>
                <button class="btn btn-primary" onclick="showAddTenantForm()">+ Add Tenant</button>
            </div>
            <p style="color:var(--muted);font-size:0.8rem;margin-bottom:1rem">Manage all customer organizations. Add, suspend, activate, upgrade plans, or delete tenants.</p>
            <table><thead><tr><th>Name</th><th>Slug</th><th>Plan</th><th>Status</th><th>Limits</th><th>Region</th><th>Actions</th></tr></thead>
            <tbody>${rows || '<tr><td colspan="7" style="text-align:center;color:var(--muted)">No tenants</td></tr>'}</tbody></table>
        </div>
        <div class="card hidden" id="add-tenant-form">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Add New Tenant</h3>
                <button class="btn" onclick="document.getElementById('add-tenant-form').classList.add('hidden')" style="border:1px solid var(--border)">Cancel</button>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Organization Name</label><input id="nt-name" placeholder="Acme Corp"></div>
                <div class="form-group"><label>Plan</label><select id="nt-plan" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="free">Free (5 users, 2 gateways, 100 certs)</option>
                    <option value="pro">Pro (25 users, 10 gateways, 1000 certs)</option>
                    <option value="enterprise">Enterprise (unlimited)</option>
                </select></div>
            </div>
            <div class="form-group"><label>Data Region</label><select id="nt-region" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                <option value="ap-south-1">Asia Pacific (Mumbai)</option>
                <option value="us-east-1">US East (Virginia)</option>
                <option value="eu-west-1">Europe (Ireland)</option>
                <option value="ap-southeast-1">Asia Pacific (Singapore)</option>
            </select></div>
            <button class="btn btn-primary" onclick="createTenant()">Create Tenant</button>
            <div id="nt-status" style="margin-top:0.5rem"></div>
        </div>`;
    }).catch(err => {
        el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load tenants: ${err.message}</p></div>`;
    });
}

function showAddTenantForm() {
    document.getElementById('add-tenant-form').classList.remove('hidden');
}

function createTenant() {
    const name = document.getElementById('nt-name').value.trim();
    const plan = document.getElementById('nt-plan').value;
    const region = document.getElementById('nt-region').value;
    const statusEl = document.getElementById('nt-status');

    if (!name) { statusEl.innerHTML = '<p style="color:var(--danger)">Organization name is required.</p>'; return; }

    statusEl.innerHTML = '<p style="color:var(--muted)">Creating tenant...</p>';
    apiPost('/api/v1/tenants', { name, plan, region }).then(data => {
        if (data.error) {
            statusEl.innerHTML = `<p style="color:var(--danger)">${data.error}</p>`;
        } else {
            statusEl.innerHTML = '<p style="color:var(--success)">Tenant created.</p>';
            setTimeout(loadTenants, 1000);
        }
    }).catch(err => { statusEl.innerHTML = `<p style="color:var(--danger)">Failed: ${err.message}</p>`; });
}

function changeTenantPlan(tenantId, newPlan) {
    if (!newPlan) return;
    if (!confirm('Upgrade/change plan to "' + newPlan + '"?')) return;

    fetch(API + '/api/v1/tenants/' + tenantId, {
        method: 'PATCH',
        headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
        body: JSON.stringify({ plan: newPlan })
    }).then(r => r.json()).then(data => {
        if (data.error) alert('Failed: ' + data.error);
        else loadTenants();
    }).catch(() => alert('Failed to update plan.'));
}

function toggleTenant(tenantId, action, name) {
    const newStatus = action === 'suspend' ? 'suspended' : 'active';
    if (!confirm(action.charAt(0).toUpperCase() + action.slice(1) + ' tenant "' + name + '"?')) return;

    fetch(API + '/api/v1/tenants/' + tenantId, {
        method: 'PATCH',
        headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: newStatus })
    }).then(r => r.json()).then(data => {
        if (data.error) alert('Failed: ' + data.error);
        else loadTenants();
    }).catch(() => alert('Failed to ' + action + ' tenant.'));
}

function deleteTenant(tenantId, name) {
    // Require the name to be typed: this removes an entire organization.
    const typed = prompt('This permanently deletes tenant "' + name + '".\n\n' +
        'Type the tenant name exactly to confirm:');
    if (typed === null) return;
    if (typed.trim() !== name) {
        alert('Name did not match. Nothing was deleted.');
        return;
    }
    sendTenantDelete(tenantId, name, false);
}

// sendTenantDelete issues the delete. The server refuses while the tenant still
// owns records and reports what is blocking, so the operator can decide whether
// to remove those too.
function sendTenantDelete(tenantId, name, force) {
    const url = API + '/api/v1/tenants/' + tenantId + (force ? '?force=true' : '');

    fetch(url, { method: 'DELETE', headers: { 'Authorization': 'Bearer ' + token } })
        .then(r => r.text().then(body => ({ status: r.status, body: body })))
        .then(res => {
            let data = {};
            try { data = JSON.parse(res.body); } catch (e) { data = { error: res.body }; }

            if (res.status === 200) {
                alert('Tenant "' + name + '" deleted.');
                loadTenants();
                return;
            }

            if (res.status === 409 && data.dependents) {
                const lines = Object.keys(data.dependents)
                    .map(k => '  \u2022 ' + data.dependents[k] + ' ' + k).join('\n');
                const go = confirm('Tenant "' + name + '" still owns:\n\n' + lines +
                    '\n\nDelete these records as well?\n\n' +
                    'OK = delete everything, Cancel = keep the tenant');
                if (go) sendTenantDelete(tenantId, name, true);
                return;
            }

            if (res.status === 401) { logout(); return; }
            alert('Could not delete tenant (HTTP ' + res.status + '):\n' +
                  (data.error || res.body || 'unknown error'));
        })
        .catch(err => alert('Could not reach the server: ' + err.message));
}

function loadUsers() {
    const el = document.getElementById('content-area');
    el.innerHTML = '<div class="card"><p>Loading users...</p></div>';
    document.getElementById('page-title').textContent = 'Users & Access';

    const currentRole = getUserRole();
    const canManage = (currentRole === 'super_admin' || currentRole === 'admin' || currentRole === 'tenant_admin');

    tenantApiGet('/api/v1/users').then(data => {
        const users = data.users || [];
        const groups = getGroups();

        // Build email→group lookup (group name + group role)
        const userGroupMap = {};
        groups.forEach(g => {
            (g.members || []).forEach(email => {
                userGroupMap[email] = { name: g.name, role: g.role || 'viewer' };
            });
        });

        let rows = users.map(u => {
            const group = userGroupMap[u.email];
            const effectiveRole = group ? group.role : u.role; // Group role takes precedence
            const roleBadge = getRoleBadge(effectiveRole);
            const roleSource = group
                ? `<span style="font-size:0.7rem;color:var(--muted)">(from group)</span>`
                : `<span style="font-size:0.7rem;color:var(--muted)">(individual)</span>`;
            const statusBadge = u.status === 'active'
                ? '<span class="badge badge-active">active</span>'
                : '<span class="badge badge-disabled">disabled</span>';
            const groupDisplay = group
                ? `<span class="badge" style="background:#2563eb15;color:#2563eb;padding:0.2rem 0.4rem">${group.name}</span>`
                : '<span style="color:var(--muted)">—</span>';
            const actions = canManage ? `
                <select onchange="changeUserRole('${u.id}', this.value)" style="font-size:0.75rem;padding:0.2rem;border:1px solid var(--border);border-radius:0.25rem" title="Override individual role">
                    <option value="" disabled selected>Override role</option>
                    <option value="super_admin">Super Admin</option>
                    <option value="admin">Admin</option>
                    <option value="tenant_admin">Tenant Admin</option>
                    <option value="cert_manager">Cert Manager</option>
                    <option value="approver">Approver</option>
                    <option value="operator">Operator</option>
                    <option value="viewer">Viewer</option>
                    <option value="auditor">Auditor</option>
                </select>
                ${u.status === 'active'
                    ? `<button class="btn btn-danger" style="font-size:0.7rem;padding:0.2rem 0.4rem;margin-left:0.25rem" onclick="disableUser('${u.id}','${u.email}')">Disable</button>`
                    : `<span style="font-size:0.75rem;color:var(--muted)">disabled</span>`}
            ` : '';
            return `<tr>
                <td>${u.email}</td>
                <td>${u.name}</td>
                <td>${groupDisplay}</td>
                <td>${roleBadge} ${roleSource}</td>
                <td>${statusBadge}</td>
                <td>${u.auth_provider||'local'}</td>
                <td style="white-space:nowrap">${actions}</td>
            </tr>`;
        }).join('');

        el.innerHTML = `<div class="card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Users & Access Management</h3>
                ${canManage ? '<button class="btn btn-primary" onclick="showAddUserForm()">+ Add User</button>' : ''}
            </div>
            <p style="color:var(--muted);font-size:0.8rem;margin-bottom:1rem">Users inherit their role from their group. Individual role overrides can be set per user.</p>
            <table><thead><tr><th>Email</th><th>Name</th><th>Group</th><th>Effective Role</th><th>Status</th><th>Auth</th>${canManage?'<th>Actions</th>':''}</tr></thead>
            <tbody>${rows || '<tr><td colspan="7" style="text-align:center;color:var(--muted)">No users</td></tr>'}</tbody></table>
        </div>
        <div class="card hidden" id="add-user-form">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Add New User</h3>
                <button class="btn" onclick="document.getElementById('add-user-form').classList.add('hidden')" style="border:1px solid var(--border)">Cancel</button>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Full Name</label><input id="nu-name" placeholder="John Doe"></div>
                <div class="form-group"><label>Email</label><input id="nu-email" type="email" placeholder="john@company.com"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Password</label><input id="nu-password" type="password" placeholder="Min 8 characters"></div>
                <div class="form-group"><label>Role</label><select id="nu-role" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="viewer">Viewer — read-only access</option>
                    <option value="operator">Operator — manage gateways</option>
                    <option value="cert_manager">Cert Manager — manage certificates</option>
                    <option value="approver">Approver — approve cert requests</option>
                    <option value="auditor">Auditor — read all, audit logs</option>
                    <option value="tenant_admin">Tenant Admin — full tenant access</option>
                    <option value="admin">Admin — full access (no tenant mgmt)</option>
                    <option value="super_admin">Super Admin — full platform access</option>
                </select></div>
            </div>
            <button class="btn btn-primary" onclick="createUser()">Create User</button>
            <div id="nu-status" style="margin-top:0.5rem"></div>
        </div>
        <div class="card" style="margin-top:1rem">
            <h3 style="margin-bottom:0.75rem">Role Permissions (RBAC)</h3>
            <p style="color:var(--muted);font-size:0.8rem;margin-bottom:1rem">Each role has specific permissions within the platform.</p>
            <table style="font-size:0.78rem">
                <thead><tr><th>Role</th><th>Tenants</th><th>Users</th><th>Groups</th><th>Gateways</th><th>Deploy</th><th>Certificates</th><th>Approvals</th><th>Audit</th></tr></thead>
                <tbody>
                    <tr><td><strong>Super Admin</strong></td><td>Full (add/remove)</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td></tr>
                    <tr><td><strong>Admin</strong></td><td>Read</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td></tr>
                    <tr><td><strong>Tenant Admin</strong></td><td>Own only</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Full</td><td>Read</td></tr>
                    <tr><td><strong>Cert Manager</strong></td><td>—</td><td>—</td><td>—</td><td>Read</td><td>—</td><td>Full</td><td>—</td><td>—</td></tr>
                    <tr><td><strong>Approver</strong></td><td>—</td><td>—</td><td>—</td><td>Read</td><td>—</td><td>Read</td><td>Approve/Reject</td><td>—</td></tr>
                    <tr><td><strong>Operator</strong></td><td>—</td><td>—</td><td>—</td><td>Full</td><td>Full</td><td>Read</td><td>—</td><td>—</td></tr>
                    <tr><td><strong>Viewer</strong></td><td>—</td><td>—</td><td>—</td><td>Read</td><td>—</td><td>Read</td><td>Read</td><td>—</td></tr>
                    <tr><td><strong>Auditor</strong></td><td>—</td><td>Read</td><td>Read</td><td>Read</td><td>Read</td><td>Read</td><td>Read</td><td>Full</td></tr>
                </tbody>
            </table>
        </div>`;
    }).catch(err => {
        el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load users: ${err.message}</p></div>`;
    });
}

function getRoleBadge(role) {
    const colors = {
        'super_admin': 'background:#7c3aed20;color:#7c3aed',
        'admin': 'background:#4f46e520;color:#4f46e5',
        'tenant_admin': 'background:#2563eb20;color:#2563eb',
        'cert_manager': 'background:#05966920;color:#059669',
        'approver': 'background:#d9770620;color:#d97706',
        'operator': 'background:#0891b220;color:#0891b2',
        'viewer': 'background:#64748b20;color:#64748b',
        'auditor': 'background:#be123c20;color:#be123c'
    };
    const style = colors[role] || 'background:#64748b20;color:#64748b';
    return `<span class="badge" style="${style};padding:0.2rem 0.5rem">${role.replace('_',' ')}</span>`;
}

function showAddUserForm() {
    document.getElementById('add-user-form').classList.remove('hidden');
    document.getElementById('nu-status').innerHTML = '';
}

function createUser() {
    const name = document.getElementById('nu-name').value.trim();
    const email = document.getElementById('nu-email').value.trim();
    const password = document.getElementById('nu-password').value;
    const role = document.getElementById('nu-role').value;
    const statusEl = document.getElementById('nu-status');

    if (!name || !email || !password) {
        statusEl.innerHTML = '<p style="color:var(--danger)">All fields are required.</p>';
        return;
    }
    if (password.length < 8) {
        statusEl.innerHTML = '<p style="color:var(--danger)">Password must be at least 8 characters.</p>';
        return;
    }

    statusEl.innerHTML = '<p style="color:var(--muted)">Creating user...</p>';

    apiPost('/api/v1/users', { name, email, password, role }).then(data => {
        if (data.error) {
            statusEl.innerHTML = `<p style="color:var(--danger)">${data.error}</p>`;
        } else {
            statusEl.innerHTML = '<p style="color:var(--success)">User created successfully.</p>';
            setTimeout(loadUsers, 1000);
        }
    }).catch(err => {
        statusEl.innerHTML = `<p style="color:var(--danger)">Failed: ${err.message}</p>`;
    });
}

function changeUserRole(userId, newRole) {
    if (!newRole) return;
    if (!confirm('Change this user\'s role to "' + newRole.replace('_',' ') + '"?')) return;

    fetch(API + '/api/v1/users/' + userId + '/role', {
        method: 'PATCH',
        headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
        body: JSON.stringify({ role: newRole })
    }).then(r => r.json()).then(data => {
        if (data.error) {
            alert('Failed: ' + data.error);
        } else {
            loadUsers(); // Refresh the list
        }
    }).catch(() => alert('Failed to update role.'));
}

function disableUser(userId, email) {
    if (!confirm('Disable user "' + email + '"? They will no longer be able to sign in.')) return;

    fetch(API + '/api/v1/users/' + userId, {
        method: 'DELETE',
        headers: { 'Authorization': 'Bearer ' + token }
    }).then(r => r.json()).then(data => {
        if (data.error) {
            alert('Failed: ' + data.error);
        } else {
            loadUsers();
        }
    }).catch(() => alert('Failed to disable user.'));
}

// --- Groups Management (localStorage-backed until backend API ready) ---
function getGroups() {
    try { return JSON.parse(localStorage.getItem('clm_groups') || '[]'); } catch(e) { return []; }
}
function saveGroups(groups) {
    localStorage.setItem('clm_groups', JSON.stringify(groups));
}

function loadGroups() {
    const el = document.getElementById('content-area');
    document.getElementById('page-title').textContent = 'Groups';

    const currentRole = getUserRole();
    const canManage = (currentRole === 'super_admin' || currentRole === 'admin' || currentRole === 'tenant_admin');
    const groups = getGroups();

    let rows = groups.map(g => {
        const memberCount = g.members ? g.members.length : 0;
        return `<tr>
            <td><strong>${g.name}</strong></td>
            <td>${g.description||'-'}</td>
            <td>${g.role||'viewer'}</td>
            <td>${memberCount}</td>
            <td>${g.created_at ? new Date(g.created_at).toLocaleDateString() : '-'}</td>
            <td style="white-space:nowrap">
                ${canManage ? `<button class="btn" style="font-size:0.7rem;padding:0.2rem 0.4rem;border:1px solid var(--border)" onclick="editGroup('${g.id}')">Manage</button>
                <button class="btn btn-danger" style="font-size:0.7rem;padding:0.2rem 0.4rem" onclick="deleteGroup('${g.id}','${g.name}')">Delete</button>` : ''}
            </td>
        </tr>`;
    }).join('');

    el.innerHTML = `<div class="card">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
            <h3>Groups</h3>
            ${canManage ? '<button class="btn btn-primary" onclick="showCreateGroupForm()">+ Create Group</button>' : ''}
        </div>
        <p style="color:var(--muted);font-size:0.8rem;margin-bottom:1rem">Organize users into groups. Assign a default role to each group — members inherit the group role unless individually overridden.</p>
        <table><thead><tr><th>Group Name</th><th>Description</th><th>Default Role</th><th>Members</th><th>Created</th><th>Actions</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="6" style="text-align:center;color:var(--muted)">No groups created yet.</td></tr>'}</tbody></table>
    </div>
    <div class="card hidden" id="create-group-form">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
            <h3 id="group-form-title">Create Group</h3>
            <button class="btn" onclick="document.getElementById('create-group-form').classList.add('hidden')" style="border:1px solid var(--border)">Cancel</button>
        </div>
        <div class="form-group"><label>Group Name</label><input id="grp-name" placeholder="e.g., DevOps Team, Security Team"></div>
        <div class="form-group"><label>Description</label><input id="grp-desc" placeholder="Team responsible for..."></div>
        <div class="form-group"><label>Default Role for Members</label><select id="grp-role" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
            <option value="viewer">Viewer</option>
            <option value="operator">Operator</option>
            <option value="cert_manager">Cert Manager</option>
            <option value="approver">Approver</option>
            <option value="auditor">Auditor</option>
            <option value="tenant_admin">Tenant Admin</option>
            <option value="admin">Admin</option>
            <option value="super_admin">Super Admin</option>
        </select></div>
        <button class="btn btn-primary" onclick="createGroup()">Create Group</button>
        <div id="grp-status" style="margin-top:0.5rem"></div>
    </div>
    <div class="card hidden" id="manage-group-card">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
            <h3 id="manage-group-title">Manage Group</h3>
            <button class="btn" onclick="document.getElementById('manage-group-card').classList.add('hidden')" style="border:1px solid var(--border)">Close</button>
        </div>
        <input type="hidden" id="manage-grp-id">
        <div id="manage-group-members"></div>
        <div class="form-group" style="margin-top:1rem"><label>Add Member (email)</label>
            <div style="display:flex;gap:0.5rem">
                <input id="add-member-email" placeholder="user@company.com" style="flex:1">
                <button class="btn btn-primary" onclick="addGroupMember()">Add</button>
            </div>
        </div>
        <div id="manage-grp-status" style="margin-top:0.5rem"></div>
    </div>`;
}

function showCreateGroupForm() {
    document.getElementById('create-group-form').classList.remove('hidden');
    document.getElementById('manage-group-card').classList.add('hidden');
    document.getElementById('grp-status').innerHTML = '';
}

function createGroup() {
    const name = document.getElementById('grp-name').value.trim();
    const description = document.getElementById('grp-desc').value.trim();
    const role = document.getElementById('grp-role').value;

    if (!name) {
        document.getElementById('grp-status').innerHTML = '<p style="color:var(--danger)">Group name is required.</p>';
        return;
    }

    const groups = getGroups();
    groups.push({
        id: Date.now().toString(36) + Math.random().toString(36).slice(2,6),
        name: name,
        description: description,
        role: role,
        members: [],
        created_at: new Date().toISOString()
    });
    saveGroups(groups);
    document.getElementById('grp-status').innerHTML = '<p style="color:var(--success)">Group created.</p>';
    setTimeout(loadGroups, 800);
}

function deleteGroup(id, name) {
    if (!confirm('Delete group "' + name + '"? Members will not be deleted but will lose their group assignment.')) return;
    const groups = getGroups().filter(g => g.id !== id);
    saveGroups(groups);
    loadGroups();
}

function editGroup(id) {
    const groups = getGroups();
    const group = groups.find(g => g.id === id);
    if (!group) return;

    document.getElementById('manage-group-card').classList.remove('hidden');
    document.getElementById('create-group-form').classList.add('hidden');
    document.getElementById('manage-grp-id').value = id;
    document.getElementById('manage-group-title').textContent = 'Manage: ' + group.name;
    document.getElementById('manage-grp-status').innerHTML = '';

    const members = group.members || [];
    if (members.length === 0) {
        document.getElementById('manage-group-members').innerHTML = '<p style="color:var(--muted);font-size:0.85rem">No members in this group yet. Add members below.</p>';
    } else {
        const memberRows = members.map(m => `<div style="display:flex;justify-content:space-between;align-items:center;padding:0.4rem 0;border-bottom:1px solid var(--border)">
            <span style="font-size:0.85rem">${m}</span>
            <button class="btn btn-danger" style="font-size:0.65rem;padding:0.15rem 0.4rem" onclick="removeGroupMember('${id}','${m}')">Remove</button>
        </div>`).join('');
        document.getElementById('manage-group-members').innerHTML = `<p style="font-size:0.8rem;color:var(--muted);margin-bottom:0.5rem">${members.length} member(s):</p>${memberRows}`;
    }
}

function addGroupMember() {
    const groupId = document.getElementById('manage-grp-id').value;
    const email = document.getElementById('add-member-email').value.trim();
    if (!email) return;

    const groups = getGroups();
    const group = groups.find(g => g.id === groupId);
    if (!group) return;

    if (!group.members) group.members = [];
    if (group.members.includes(email)) {
        document.getElementById('manage-grp-status').innerHTML = '<p style="color:var(--danger)">User already in group.</p>';
        return;
    }
    group.members.push(email);
    saveGroups(groups);
    document.getElementById('add-member-email').value = '';
    editGroup(groupId); // refresh member list
}

function removeGroupMember(groupId, email) {
    const groups = getGroups();
    const group = groups.find(g => g.id === groupId);
    if (!group) return;
    group.members = (group.members || []).filter(m => m !== email);
    saveGroups(groups);
    editGroup(groupId);
}

function loadSSO() {
    const el = document.getElementById('content-area');
    el.innerHTML = `<div class="card">
        <h3>SSO Configuration</h3>
        <p style="color:var(--muted);font-size:0.85rem;margin-bottom:1rem">Configure identity provider per tenant. Supports SAML 2.0, OIDC, Google, and Microsoft.</p>
        <div class="form-group"><label>Provider</label><select id="sso-provider" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
            <option value="google">Google (Gmail / Workspace)</option>
            <option value="microsoft">Microsoft (Outlook / Azure AD)</option>
            <option value="oidc">OpenID Connect (Generic)</option>
            <option value="saml">SAML 2.0</option>
        </select></div>
        <div class="form-group"><label>Client ID</label><input id="sso-client-id" placeholder="OAuth Client ID"></div>
        <div class="form-group"><label>Client Secret</label><input id="sso-client-secret" type="password" placeholder="OAuth Client Secret"></div>
        <div class="form-group"><label>Allowed Domains (comma-separated)</label><input id="sso-domains" placeholder="aiforgers.tech"></div>
        <div class="form-group"><label>Auto-Provision New Users</label><select id="sso-auto" style="padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"><option value="true">Yes</option><option value="false">No</option></select></div>
        <button class="btn btn-primary" onclick="saveSSO()">Save SSO Config</button>
    </div>`;
}

function saveSSO() { alert('SSO configuration saved (demo). In production, calls POST /api/v1/tenants/:id/sso'); }

// --- Organization Page (Client Admin) ---
function loadOrganization() {
    const el = panelEl();
    if (!document.getElementById('org-body')) document.getElementById('page-title').textContent = 'Organization';
    el.innerHTML = '<div class="card"><p>Loading organization details...</p></div>';

    // Prefer the tenant_id claim, but never depend on it. When the claim is
    // missing the server resolves the caller's tenant itself via /tenants/me, so
    // this page works regardless of how the realm's protocol mappers are set up.
    // Relying on the claim alone previously left this page permanently blank.
    const claims = decodeJwt(token) || {};
    const tenantId = claims.tenant_id || '';
    const userEmail = claims.email || claims.preferred_username || '';

    const target = tenantId ? '/api/v1/tenants/' + tenantId : '/api/v1/tenants/me';

    apiGet(target).then(data => {
        const t = data.tenant || data;
        el.innerHTML = `
        <div class="card">
            <h3 style="margin-bottom:1rem">Organization Settings</h3>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem">
                <div>
                    <div class="form-group"><label>Organization Name</label><input id="org-name" value="${t.name||''}" style="width:100%"></div>
                    <div class="form-group"><label>Slug</label><input value="${t.slug||''}" disabled style="width:100%;background:#f8fafc"></div>
                </div>
                <div>
                    <div class="form-group"><label>Plan</label>
                        <div style="display:flex;align-items:center;gap:0.5rem">
                            <span class="badge badge-active" style="font-size:0.85rem;padding:0.3rem 0.6rem">${(t.plan||'free').toUpperCase()}</span>
                            <button class="btn" style="font-size:0.75rem;border:1px solid var(--primary);color:var(--primary)" onclick="alert('Contact sales@aiforgers.tech to upgrade your plan.')">Upgrade Plan</button>
                        </div>
                    </div>
                    <div class="form-group"><label>Data Region</label><input value="${t.data_region||'ap-south-1'}" disabled style="width:100%;background:#f8fafc"></div>
                </div>
            </div>
            <button class="btn btn-primary" style="margin-top:0.5rem" onclick="updateOrg('${tenantId}')">Save Changes</button>
            <div id="org-status" style="margin-top:0.5rem"></div>
        </div>
        <div class="card">
            <h3 style="margin-bottom:0.75rem">Plan Limits</h3>
            <table style="font-size:0.85rem">
                <thead><tr><th>Resource</th><th>Used</th><th>Limit</th></tr></thead>
                <tbody>
                    <tr><td>Users</td><td id="org-users-used">—</td><td>${t.max_users||5}</td></tr>
                    <tr><td>Gateways</td><td id="org-gw-used">—</td><td>${t.max_satellites||2}</td></tr>
                    <tr><td>Certificates</td><td id="org-cert-used">—</td><td>${t.max_certs||100}</td></tr>
                </tbody>
            </table>
        </div>
        <div class="card">
            <h3 style="margin-bottom:0.75rem">Account Info</h3>
            <table style="font-size:0.85rem">
                <tr><td style="font-weight:600;width:160px">Tenant ID</td><td><code style="font-size:0.8rem">${tenantId}</code></td></tr>
                <tr><td style="font-weight:600">Admin Email</td><td>${userEmail}</td></tr>
                <tr><td style="font-weight:600">Created</td><td>${t.created_at ? new Date(t.created_at).toLocaleString() : '-'}</td></tr>
                <tr><td style="font-weight:600">Status</td><td><span class="badge badge-active">${t.status||'active'}</span></td></tr>
            </table>
        </div>`;

        // Load usage counts
        tenantApiGet('/api/v1/users').then(d => { document.getElementById('org-users-used').textContent = (d.users||[]).length; }).catch(()=>{});
        tenantApiGet('/api/v1/satellites').then(d => { document.getElementById('org-gw-used').textContent = (d.satellites||[]).length; }).catch(()=>{});
    }).catch(err => {
        el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load organization: ${err.message}</p></div>`;
    });
}

function updateOrg(tenantId) {
    const name = document.getElementById('org-name').value.trim();
    if (!name) { document.getElementById('org-status').innerHTML = '<p style="color:var(--danger)">Name is required.</p>'; return; }

    fetch(API + '/api/v1/tenants/' + tenantId, {
        method: 'PATCH',
        headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name })
    }).then(r => r.json()).then(data => {
        if (data.error) {
            document.getElementById('org-status').innerHTML = `<p style="color:var(--danger)">${data.error}</p>`;
        } else {
            document.getElementById('org-status').innerHTML = '<p style="color:var(--success)">Organization updated.</p>';
        }
    }).catch(() => {
        document.getElementById('org-status').innerHTML = '<p style="color:var(--danger)">Failed to update.</p>';
    });
}

// --- Activity Log Page (Client Admin) ---
function loadActivity() {
    const el = document.getElementById('content-area');
    document.getElementById('page-title').textContent = 'Activity Log';
    el.innerHTML = '<div class="card"><p>Loading activity...</p></div>';

    // Combine events from multiple sources including gateway activity
    Promise.all([
        tenantApiGet('/api/v1/satellites/deploy').catch(() => ({ jobs: [] })),
        tenantApiGet('/api/v1/users').catch(() => ({ users: [] })),
        tenantApiGet('/api/v1/satellites').catch(() => ({ satellites: [] }))
    ]).then(([deployData, userData, satData]) => {
        const jobs = deployData.jobs || [];
        const users = userData.users || [];
        const gateways = satData.satellites || [];

        let activities = [];

        // Deploy job events
        jobs.forEach(j => {
            activities.push({
                time: j.created_at || new Date().toISOString(),
                type: 'deploy',
                icon: '&#9881;',
                color: j.state === 'online' ? 'var(--success)' : j.state === 'failed' ? 'var(--danger)' : '#f59e0b',
                message: `Gateway deployment "${j.name}" — ${j.state}`,
                detail: j.region ? `${j.cloud} / ${j.region}` + (j.public_ip ? ' | IP: ' + j.public_ip : '') : ''
            });
        });

        // User events
        users.forEach(u => {
            activities.push({
                time: u.created_at || new Date().toISOString(),
                type: 'user',
                icon: '&#128100;',
                color: 'var(--primary)',
                message: `User "${u.name}" (${u.email}) added`,
                detail: `Role: ${u.role}`
            });
        });

        // Gateway events (registration + heartbeat status)
        gateways.forEach(g => {
            activities.push({
                time: g.created_at || new Date().toISOString(),
                type: 'gateway',
                icon: '&#9729;',
                color: 'var(--success)',
                message: `Gateway "${g.name}" registered`,
                detail: g.cloud ? `${g.cloud} / ${g.region}` : ''
            });
            // Heartbeat activity
            if (g.last_heartbeat) {
                const statusColor = g.status === 'online' ? 'var(--success)' : g.status === 'offline' ? 'var(--danger)' : '#f59e0b';
                activities.push({
                    time: g.last_heartbeat,
                    type: 'heartbeat',
                    icon: '&#128154;',
                    color: statusColor,
                    message: `Gateway "${g.name}" heartbeat — ${g.status}`,
                    detail: `Version: ${g.version || 'unknown'} | Certs: ${g.cert_count || 0} | Agents: ${g.agent_count || 0}`
                });
            }
        });

        // Fetch gateway command logs from each gateway
        const cmdPromises = gateways.map(g =>
            apiGet('/api/v1/satellites/' + g.id + '/commands').catch(() => ({ commands: [] }))
                .then(data => ({ gateway: g.name, commands: data.commands || [] }))
        );

        Promise.all(cmdPromises).then(cmdResults => {
            // Add gateway commands to activity
            cmdResults.forEach(({ gateway, commands }) => {
                commands.forEach(cmd => {
                    const cmdColor = cmd.status === 'completed' ? 'var(--success)' : cmd.status === 'failed' ? 'var(--danger)' : '#f59e0b';
                    activities.push({
                        time: cmd.created_at || cmd.sent_at || new Date().toISOString(),
                        type: 'command',
                        icon: '&#128268;',
                        color: cmdColor,
                        message: `[${gateway}] Command: ${cmd.type} — ${cmd.status}`,
                        detail: cmd.result ? cmd.result.substring(0, 100) : ''
                    });
                });
            });

            // Sort by time descending
            activities.sort((a, b) => new Date(b.time) - new Date(a.time));

            // Filter tabs
            const types = ['all', 'deploy', 'gateway', 'heartbeat', 'command', 'user'];
            const tabsHtml = types.map(t => `<button class="btn" style="font-size:0.75rem;padding:0.25rem 0.6rem;margin-right:0.3rem;border:1px solid ${t==='all'?'var(--primary)':'var(--border)'};color:${t==='all'?'var(--primary)':'var(--muted)'}" onclick="filterActivity('${t}',this)">${t === 'all' ? 'All' : t.charAt(0).toUpperCase()+t.slice(1)}</button>`).join('');

            let rows = activities.slice(0, 100).map(a => `
                <div class="activity-row" data-type="${a.type}" style="display:flex;align-items:flex-start;padding:0.6rem 0;border-bottom:1px solid var(--border)">
                    <span style="font-size:1.1rem;width:28px;text-align:center;color:${a.color}">${a.icon}</span>
                    <div style="flex:1;margin-left:0.5rem">
                        <p style="font-size:0.85rem;margin-bottom:0.15rem">${a.message}</p>
                        <p style="font-size:0.75rem;color:var(--muted)">${a.detail}</p>
                    </div>
                    <span style="font-size:0.75rem;color:var(--muted);white-space:nowrap">${new Date(a.time).toLocaleString()}</span>
                </div>`).join('');

            el.innerHTML = `<div class="card">
                <h3 style="margin-bottom:0.5rem">Activity Log</h3>
                <p style="color:var(--muted);font-size:0.8rem;margin-bottom:0.75rem">All activity from your organization and gateways.</p>
                <div style="margin-bottom:1rem">${tabsHtml}</div>
                <div id="activity-list">${rows || '<p style="color:var(--muted);font-size:0.85rem">No activity yet.</p>'}</div>
            </div>`;
        });
    });
}

function filterActivity(type, btn) {
    // Update tab button styles
    btn.parentElement.querySelectorAll('.btn').forEach(b => { b.style.borderColor = 'var(--border)'; b.style.color = 'var(--muted)'; });
    btn.style.borderColor = 'var(--primary)';
    btn.style.color = 'var(--primary)';

    // Filter rows
    document.querySelectorAll('.activity-row').forEach(row => {
        if (type === 'all' || row.dataset.type === type) {
            row.style.display = 'flex';
        } else {
            row.style.display = 'none';
        }
    });
}

// --- Cloud Accounts Page ---
function loadCloudAccounts() {
    const el = document.getElementById('content-area');
    document.getElementById('page-title').textContent = 'Cloud Accounts';
    el.innerHTML = '<div class="card"><p>Loading cloud accounts...</p></div>';

    resolveTenantScope().then(tenantId => apiGet(scopedPath('/api/v1/cloud-accounts', tenantId))).then(data => {
    const accounts = (data.cloud_accounts || []).map(a => Object.assign({}, a, {
        cloud: a.provider,
        region: defaultRegion(a.provider)
    }));
    cloudAccountsCache = accounts;
    let rows = accounts.map(a => `<tr>
        <td><strong>${a.name}</strong></td>
        <td><span class="badge badge-active">${a.cloud.toUpperCase()}</span></td>
        <td>${a.region||'-'}</td>
        <td><code style="font-size:0.75rem">Encrypted credential v${a.credential_version || 1}</code></td>
        <td><span class="badge badge-active">active</span></td>
        <td>${new Date(a.created_at).toLocaleDateString()}</td>
        <td><button class="btn btn-danger" style="font-size:0.7rem;padding:0.2rem 0.5rem" onclick="deleteCloudAccount('${a.id}')">Remove</button></td>
    </tr>`).join('');

    el.innerHTML = `<div class="card">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
            <h3>Cloud Accounts</h3>
            <button class="btn btn-primary" onclick="showAddCloudAccount()">+ Register Cloud</button>
        </div>
        <p style="color:var(--muted);font-size:0.85rem;margin-bottom:1rem">Register your cloud provider credentials. These are used when deploying satellite gateways to your infrastructure.</p>
        <table><thead><tr><th>Name</th><th>Provider</th><th>Region</th><th>Credential</th><th>Status</th><th>Added</th><th></th></tr></thead>
        <tbody>${rows || '<tr><td colspan="7" style="text-align:center;color:var(--muted)">No cloud accounts registered yet. Click "+ Register Cloud" to add one.</td></tr>'}</tbody></table>
    </div>
    <div class="card hidden" id="add-cloud-form">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
            <h3>Register Cloud Account</h3>
            <button class="btn" onclick="document.getElementById('add-cloud-form').classList.add('hidden')" style="border:1px solid var(--border)">Cancel</button>
        </div>
        <div class="form-group"><label>Account Name</label><input id="ca-name" placeholder="AWS Production - Mumbai"></div>
        <div class="form-group"><label>Infrastructure Target</label><select id="ca-cloud" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem" onchange="updateCloudFields()">
            <option value="aws">Amazon Web Services (AWS)</option>
            <option value="gcp">Google Cloud Platform (GCP)</option>
            <option value="azure">Microsoft Azure</option>
        </select></div>
        <div id="cloud-cred-fields">
            <div class="form-group"><label>AWS Access Key ID</label><input id="ca-key" placeholder="AKIAIOSFODNN7EXAMPLE"></div>
            <div class="form-group"><label>AWS Secret Access Key</label><input id="ca-secret" type="password" placeholder="wJalrXUtnFEMI/K7MDENG/..."></div>
            <div class="form-group"><label>Default Region</label><input id="ca-region" placeholder="ap-south-1" value="ap-south-1"></div>
        </div>
        <div class="form-group"><label>Role ARN (optional, for cross-account)</label><input id="ca-role" placeholder="arn:aws:iam::123456789:role/CLM-Deploy"></div>
        <button class="btn btn-primary" onclick="registerCloudAccount()">Encrypt & Register</button>
        <div id="ca-status" style="margin-top:0.75rem"></div>
    </div>`;
    }).catch(err => {
        el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load cloud accounts: ${esc(err.message)}</p></div>`;
    });
}

function deleteCloudAccount(id) {
    if (!confirm('Remove this cloud account?')) return;
    resolveTenantScope()
        .then(tenantId => apiDelete(scopedPath('/api/v1/cloud-accounts/' + id, tenantId)))
        .then(() => loadCloudAccounts())
        .catch(err => alert('Failed to remove cloud account: ' + err.message));
}

function showAddCloudAccount() {
    document.getElementById('add-cloud-form').classList.remove('hidden');
}

function updateCloudFields() {
    const cloud = document.getElementById('ca-cloud').value;
    const el = document.getElementById('cloud-cred-fields');
    if (cloud === 'aws') {
        el.innerHTML = `
            <div class="form-group"><label>AWS Access Key ID</label><input id="ca-key" placeholder="AKIAIOSFODNN7EXAMPLE"></div>
            <div class="form-group"><label>AWS Secret Access Key</label><input id="ca-secret" type="password" placeholder="wJalrXUtnFEMI/K7MDENG/..."></div>
            <div class="form-group"><label>Default Region</label><input id="ca-region" placeholder="ap-south-1" value="ap-south-1"></div>`;
    } else if (cloud === 'gcp') {
        el.innerHTML = `
            <div class="form-group"><label>GCP Project ID</label><input id="ca-key" placeholder="my-project-123"></div>
            <div class="form-group"><label>Service Account JSON Key</label><textarea id="ca-secret" rows="4" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem;font-size:0.8rem;font-family:monospace" placeholder='{"type":"service_account",...}'></textarea></div>
            <div class="form-group"><label>Default Region</label><input id="ca-region" placeholder="asia-south1" value="asia-south1"></div>`;
    } else if (cloud === 'azure') {
        el.innerHTML = `
            <div class="form-group"><label>Azure Tenant ID</label><input id="ca-key" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"></div>
            <div class="form-group"><label>Client ID (App ID)</label><input id="ca-secret" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"></div>
            <div class="form-group"><label>Client Secret</label><input id="ca-region" type="password" placeholder="Client secret value"></div>`;
    } else if (cloud === 'onprem_linux') {
        // No cloud API involved: the gateway is installed over SSH onto a host
        // the customer already runs, so the "credential" is SSH access.
        el.innerHTML = `
            <div class="form-group"><label>Host (IP or DNS name)</label><input id="ca-key" placeholder="10.0.5.42 or gw01.dc.internal"></div>
            <div class="form-group"><label>SSH User</label><input id="ca-secret" placeholder="ubuntu / ec2-user / root"></div>
            <div class="form-group"><label>SSH Port</label><input id="ca-region" placeholder="22" value="22"></div>
            <div class="form-group"><label>SSH Private Key or Password</label>
                <textarea id="ca-sshkey" rows="4" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem;font-size:0.8rem;font-family:monospace" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"></textarea></div>
            <div class="form-group"><label>Linux Flavour</label>
                <select id="ca-flavour" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="auto">Auto-detect (recommended)</option>
                    <option value="ubuntu">Ubuntu</option>
                    <option value="debian">Debian</option>
                    <option value="rhel">RHEL</option>
                    <option value="centos">CentOS</option>
                    <option value="rocky">Rocky Linux</option>
                    <option value="almalinux">AlmaLinux</option>
                    <option value="amzn">Amazon Linux</option>
                </select></div>
            <div class="card" style="background:#fffbeb;border-color:#fde68a;padding:0.7rem;margin-bottom:0.75rem">
                <p style="font-size:0.75rem;color:#92400e;margin:0">
                    <strong>Not stored securely yet.</strong> Cloud Accounts has no backend API — entries are
                    held in this browser's local storage, unencrypted and invisible to other administrators.
                    Do not enter a production SSH key until a server-side encrypted store exists.
                </p>
            </div>`;
    }
}

function registerCloudAccount() {
    const name = document.getElementById('ca-name').value.trim();
    const cloud = document.getElementById('ca-cloud').value;
    const statusEl = document.getElementById('ca-status');

    if (!name) { statusEl.innerHTML = '<p style="color:var(--danger)">Account name is required.</p>'; return; }

    statusEl.innerHTML = '<p style="color:var(--muted)">Encrypting and registering credentials...</p>';

    // Build credentials object
    const creds = {};
    let region = '';
    if (cloud === 'aws') {
        creds.aws_access_key_id = document.getElementById('ca-key').value;
        creds.aws_secret_access_key = document.getElementById('ca-secret').value;
        region = document.getElementById('ca-region').value || 'ap-south-1';
        if (!creds.aws_access_key_id || !creds.aws_secret_access_key) {
            statusEl.innerHTML = '<p style="color:var(--danger)">Access Key and Secret are required.</p>'; return;
        }
    } else if (cloud === 'gcp') {
        creds.gcp_project_id = document.getElementById('ca-key').value;
        creds.gcp_service_account_json = document.getElementById('ca-secret').value;
        region = document.getElementById('ca-region').value || 'asia-south1';
    } else if (cloud === 'azure') {
        creds.azure_tenant_id = document.getElementById('ca-key').value;
        creds.azure_client_id = document.getElementById('ca-secret').value;
        creds.azure_client_secret = document.getElementById('ca-region').value;
        region = 'centralindia';
    } else if (cloud === 'onprem_linux') {
        statusEl.innerHTML = '<p style="color:var(--danger)">On-premise SSH accounts are not supported by the encrypted cloud-account API yet. No credential was stored.</p>';
        return;
    }

    const roleEl = document.getElementById('ca-role');
    const role = roleEl ? roleEl.value.trim() : '';
    if (role) creds.aws_role_arn = role;

    const idempotencyKey = (window.crypto && crypto.randomUUID)
        ? crypto.randomUUID()
        : 'portal-' + Date.now() + '-' + Math.random().toString(36).slice(2);

    resolveTenantScope().then(tenantId =>
        apiPost(scopedPath('/api/v1/cloud-accounts', tenantId), {
            name: name,
            provider: cloud,
            credentials: creds
        }, { 'Idempotency-Key': idempotencyKey }).then(account =>
            apiPost(scopedPath('/api/v1/cloud-accounts/' + account.id + '/validate', tenantId), {})
        )
    ).then(() => {
        statusEl.innerHTML = '<p style="color:var(--success)">Cloud account encrypted and registered. Offline decryptability validation passed; provider connectivity is checked only when provisioning is enabled.</p>';
        setTimeout(() => loadCloudAccounts(), 1000);
    }).catch(err => {
        statusEl.innerHTML = '<p style="color:var(--danger)">Error: ' + esc(err.message || 'registration failed') + '</p>';
    });
}

function loadGateways() {
    const el = document.getElementById('content-area');
    el.innerHTML = '<div class="card"><p>Loading gateways...</p></div>';
    document.getElementById('page-title').textContent = 'Gateways';

    resolveTenantScope().then(tenantId => Promise.all([
        apiGet(scopedPath('/api/v1/satellites', tenantId)),
        apiGet(scopedPath('/api/v1/cloud-accounts', tenantId))
    ])).then(([data, accountData]) => {
        cloudAccountsCache = (accountData.cloud_accounts || []).map(a => Object.assign({}, a, {
            cloud: a.provider,
            region: defaultRegion(a.provider)
        }));
        const gateways = data.satellites || [];
        let rows = gateways.map(g => {
            const statusBadge = g.status === 'online'
                ? '<span class="badge badge-active">online</span>'
                : g.status === 'registering'
                ? '<span class="badge badge-warning">registering</span>'
                : g.status === 'offline'
                ? '<span class="badge badge-disabled">offline</span>'
                : '<span class="badge badge-warning">' + g.status + '</span>';
            return `<tr>
                <td><strong>${g.name}</strong></td>
                <td>${statusBadge}</td>
                <td>${g.cloud||'-'}</td>
                <td>${g.region||'-'}</td>
                <td>${g.ip_address||'-'}</td>
                <td>${g.last_heartbeat ? new Date(g.last_heartbeat).toLocaleString() : '<span style="color:var(--muted)">Never</span>'}</td>
                <td style="white-space:nowrap">
                    <button class="btn" style="font-size:0.7rem;padding:0.2rem 0.5rem;border:1px solid var(--primary);color:var(--primary)" onclick="openGatewayPortal('${g.id}','${g.ip_address||''}','${g.name}')">Open Portal</button>
                    <button class="btn" style="font-size:0.7rem;padding:0.2rem 0.5rem;border:1px solid var(--success);color:var(--success)" onclick="showGatewayActions('${g.id}','${g.ip_address||''}','${g.name}')">Manage</button>
                    <button class="btn" style="font-size:0.7rem;padding:0.2rem 0.5rem;border:1px solid var(--border)" onclick="editGateway('${g.id}','${g.name}','${g.cloud||''}','${g.region||''}')">Edit</button>
                    <button class="btn btn-danger" style="font-size:0.7rem;padding:0.2rem 0.5rem" onclick="deleteGateway('${g.id}','${g.name}')">Delete</button>
                </td>
            </tr>`;
        }).join('');
        el.innerHTML = `<div class="card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Gateways</h3>
                <button class="btn btn-primary" onclick="showDeployForm()">+ Deploy New Gateway</button>
            </div>
            <p style="color:var(--muted);font-size:0.8rem;margin-bottom:1rem">Deploy, manage, and access your gateway fleet. All gateway operations are performed from here.</p>
            <table><thead><tr><th>Name</th><th>Status</th><th>Cloud</th><th>Region</th><th>IP</th><th>Last Heartbeat</th><th>Actions</th></tr></thead>
            <tbody>${rows || '<tr><td colspan="7" style="text-align:center;color:var(--muted)">No gateways yet. Click "+ Deploy New Gateway" to create your first one.</td></tr>'}</tbody></table>
        </div>
        <div class="card hidden" id="deploy-form-card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3 id="deploy-form-title">Deploy New Gateway</h3>
                <button class="btn" onclick="hideDeployForm()" style="border:1px solid var(--border)">Cancel</button>
            </div>
            <p style="color:var(--warning);font-size:0.8rem;margin-bottom:1rem"><strong>Gateway provisioning is temporarily disabled.</strong> The legacy bootstrap cannot inject the one-time satellite credential and tenant-bound mTLS identity required by the hardened transport. Cloud accounts may be registered securely, but no infrastructure will be created until that bootstrap is implemented.</p>
            <div class="form-group"><label>Gateway Name</label><input id="d-name" placeholder="dc-mumbai-prod-01"></div>
            <div class="form-group"><label>Cloud Account</label><select id="d-cloud-account" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem" onchange="onCloudAccountSelect()">
                <option value="">-- Select a registered cloud account --</option>
            </select>
            <p style="font-size:0.75rem;color:var(--muted);margin-top:0.25rem">No accounts? <a href="#" onclick="navigate('cloud-accounts');return false" style="color:var(--primary)">Register one first</a></p></div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Deploy Mode</label><select id="d-mode" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem" onchange="onDeployModeChange()">
                    <option value="lightsail">Lightsail (VM — simplest)</option>
                    <option value="ec2">EC2 + ALB (production VM)</option>
                    <option value="eks">EKS (Kubernetes)</option>
                </select></div>
                <div class="form-group"><label>Region</label><input id="d-region" placeholder="ap-south-1"></div>
            </div>
            <div id="deploy-mode-config">
                <!-- Dynamic config based on mode -->
            </div>
            <button class="btn" style="margin-top:0.75rem" disabled title="Secure satellite credential and mTLS bootstrap is not configured">Deployment Unavailable</button>
            <div id="deploy-status" style="margin-top:1rem"></div>
        </div>
        <div class="card hidden" id="edit-gateway-card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Edit Gateway</h3>
                <button class="btn" onclick="document.getElementById('edit-gateway-card').classList.add('hidden')" style="border:1px solid var(--border)">Cancel</button>
            </div>
            <input type="hidden" id="edit-gw-id">
            <div class="form-group"><label>Gateway Name</label><input id="edit-gw-name" placeholder="Gateway name"></div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Cloud</label><input id="edit-gw-cloud" placeholder="aws"></div>
                <div class="form-group"><label>Region</label><input id="edit-gw-region" placeholder="ap-south-1"></div>
            </div>
            <button class="btn btn-primary" onclick="saveGatewayEdit()">Save Changes</button>
            <div id="edit-gw-status" style="margin-top:0.5rem"></div>
        </div>`;
    }).catch(err => {
        el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load gateways: ${err.message}</p></div>`;
    });
}

// --- Gateway CRUD ---
function editGateway(id, name, cloud, region) {
    document.getElementById('edit-gateway-card').classList.remove('hidden');
    document.getElementById('deploy-form-card').classList.add('hidden');
    document.getElementById('edit-gw-id').value = id;
    document.getElementById('edit-gw-name').value = name;
    document.getElementById('edit-gw-cloud').value = cloud;
    document.getElementById('edit-gw-region').value = region;
    document.getElementById('edit-gw-status').innerHTML = '';
}

function saveGatewayEdit() {
    const id = document.getElementById('edit-gw-id').value;
    const body = {
        name: document.getElementById('edit-gw-name').value,
        cloud: document.getElementById('edit-gw-cloud').value,
        region: document.getElementById('edit-gw-region').value
    };
    fetch(API + '/api/v1/satellites/' + id, {
        method: 'PATCH',
        headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    }).then(r => r.json()).then(data => {
        if (data.error) {
            document.getElementById('edit-gw-status').innerHTML = `<p style="color:var(--danger)">${data.error}</p>`;
        } else {
            document.getElementById('edit-gw-status').innerHTML = `<p style="color:var(--success)">Gateway updated.</p>`;
            setTimeout(loadGateways, 1000);
        }
    }).catch(() => {
        document.getElementById('edit-gw-status').innerHTML = `<p style="color:var(--danger)">Failed to update.</p>`;
    });
}

function deleteGateway(id, name) {
    if (!confirm('Delete gateway "' + name + '"? This will remove the gateway from management.')) return;
    fetch(API + '/api/v1/satellites/' + id, {
        method: 'DELETE',
        headers: { 'Authorization': 'Bearer ' + token }
    }).then(r => r.json()).then(data => {
        if (data.error) {
            alert('Delete failed: ' + data.error);
        } else {
            alert('Gateway "' + name + '" deleted.');
            loadGateways();
        }
    }).catch(() => alert('Failed to delete gateway.'));
}

function redeployGateway(id, name) {
    if (!confirm('Redeploy gateway "' + name + '"? This will upgrade the gateway to the latest version.')) return;
    apiPost('/api/v1/satellites/' + id + '/upgrade', {}).then(data => {
        if (data.error) {
            alert('Redeploy failed: ' + data.error);
        } else {
            alert('Redeploy initiated for "' + name + '". The gateway will pull the latest version on next heartbeat.');
        }
    }).catch(() => alert('Failed to redeploy gateway.'));
}

function openGatewayPortal(id, ip, name) {
    if (ip) { window.open('http://' + ip, '_blank'); return; }
    const storedUrls = JSON.parse(localStorage.getItem('clm_gateway_urls') || '{}');
    if (storedUrls[id]) { window.open(storedUrls[id], '_blank'); return; }
    tenantApiGet('/api/v1/satellites/deploy').then(data => {
        const jobs = data.jobs || [];
        const job = jobs.find(j => j.public_ip && (j.name === name || j.satellite_id === id));
        if (job && job.public_ip) {
            storedUrls[id] = 'http://' + job.public_ip;
            localStorage.setItem('clm_gateway_urls', JSON.stringify(storedUrls));
            window.open(storedUrls[id], '_blank');
        } else {
            const url = prompt('Enter Gateway Portal URL for "' + name + '":\nExample: http://13.207.101.74');
            if (url) { storedUrls[id] = url.startsWith('http') ? url : 'http://' + url; localStorage.setItem('clm_gateway_urls', JSON.stringify(storedUrls)); window.open(storedUrls[id], '_blank'); }
        }
    }).catch(() => {
        const url = prompt('Enter Gateway Portal URL for "' + name + '":');
        if (url) { const storedUrls = JSON.parse(localStorage.getItem('clm_gateway_urls') || '{}'); storedUrls[id] = url.startsWith('http') ? url : 'http://' + url; localStorage.setItem('clm_gateway_urls', JSON.stringify(storedUrls)); window.open(storedUrls[id], '_blank'); }
    });
}

// --- Gateway Server Management ---
function showGatewayActions(id, ip, name) {
    const el = document.getElementById('content-area');
    document.getElementById('page-title').textContent = 'Manage: ' + name;

    const gatewayUrl = ip ? 'http://' + ip : (JSON.parse(localStorage.getItem('clm_gateway_urls') || '{}')[id] || '');

    el.innerHTML = `
        <div style="margin-bottom:1rem"><button class="btn" onclick="loadGateways()" style="border:1px solid var(--border)">&larr; Back to Gateways</button></div>
        <div class="card">
            <h3 style="margin-bottom:0.5rem">${name}</h3>
            <p style="color:var(--muted);font-size:0.8rem">Gateway ID: ${id} ${ip ? '| IP: ' + ip : ''}</p>
        </div>
        <div class="stats" style="grid-template-columns:repeat(auto-fit,minmax(200px,1fr))">
            <div class="card" style="cursor:pointer;border:2px solid var(--primary)" onclick="openGatewayPortal('${id}','${ip}','${name}')">
                <div style="text-align:center">
                    <div style="font-size:1.5rem">&#127760;</div>
                    <div style="font-weight:600;margin-top:0.25rem">Open Portal</div>
                    <div style="font-size:0.75rem;color:var(--muted)">Access Gateway UI</div>
                </div>
            </div>
            <div class="card" style="cursor:pointer;border:2px solid var(--success)" onclick="deployGatewayServices('${id}','${name}')">
                <div style="text-align:center">
                    <div style="font-size:1.5rem">&#128640;</div>
                    <div style="font-weight:600;margin-top:0.25rem">Deploy Services</div>
                    <div style="font-size:0.75rem;color:var(--muted)">Push latest gateway version</div>
                </div>
            </div>
            <div class="card" style="cursor:pointer;border:2px solid #f59e0b" onclick="restartGatewayServices('${id}','${name}')">
                <div style="text-align:center">
                    <div style="font-size:1.5rem">&#128260;</div>
                    <div style="font-weight:600;margin-top:0.25rem">Restart Services</div>
                    <div style="font-size:0.75rem;color:var(--muted)">Restart gateway process</div>
                </div>
            </div>
            <div class="card" style="cursor:pointer;border:2px solid var(--border)" onclick="accessGatewayServer('${id}','${ip}','${name}')">
                <div style="text-align:center">
                    <div style="font-size:1.5rem">&#128421;</div>
                    <div style="font-weight:600;margin-top:0.25rem">Server Access</div>
                    <div style="font-size:0.75rem;color:var(--muted)">SSH / Console access</div>
                </div>
            </div>
        </div>
        <div id="gw-action-status"></div>
        <div class="card">
            <h3 style="margin-bottom:0.75rem">Service Status</h3>
            <div id="gw-service-status"><p style="color:var(--muted);font-size:0.85rem">Loading...</p></div>
        </div>
        <div class="card">
            <h3 style="margin-bottom:0.75rem">Command History</h3>
            <div id="gw-command-history"><p style="color:var(--muted);font-size:0.85rem">Loading...</p></div>
        </div>`;

    // Load gateway details for service status
    apiGet('/api/v1/satellites/' + id).then(sat => {
        const g = sat;
        const statusColor = g.status === 'online' ? 'var(--success)' : g.status === 'offline' ? 'var(--danger)' : '#f59e0b';
        const lastHB = g.last_heartbeat ? new Date(g.last_heartbeat).toLocaleString() : 'Never';
        const timeSinceHB = g.last_heartbeat ? Math.round((Date.now() - new Date(g.last_heartbeat).getTime()) / 1000) : null;
        const hbAge = timeSinceHB !== null ? (timeSinceHB < 60 ? timeSinceHB + 's ago' : timeSinceHB < 3600 ? Math.round(timeSinceHB/60) + 'm ago' : Math.round(timeSinceHB/3600) + 'h ago') : 'N/A';

        document.getElementById('gw-service-status').innerHTML = `
            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:0.75rem;margin-bottom:1rem">
                <div style="text-align:center;padding:0.75rem;border-radius:0.5rem;background:${statusColor}10;border:1px solid ${statusColor}30">
                    <div style="font-size:0.7rem;text-transform:uppercase;color:var(--muted)">Status</div>
                    <div style="font-size:1.1rem;font-weight:700;color:${statusColor}">${g.status || 'unknown'}</div>
                </div>
                <div style="text-align:center;padding:0.75rem;border-radius:0.5rem;background:#f8fafc;border:1px solid var(--border)">
                    <div style="font-size:0.7rem;text-transform:uppercase;color:var(--muted)">Version</div>
                    <div style="font-size:1.1rem;font-weight:700">${g.version || 'unknown'}</div>
                </div>
                <div style="text-align:center;padding:0.75rem;border-radius:0.5rem;background:#f8fafc;border:1px solid var(--border)">
                    <div style="font-size:0.7rem;text-transform:uppercase;color:var(--muted)">Last Heartbeat</div>
                    <div style="font-size:0.9rem;font-weight:600">${hbAge}</div>
                </div>
                <div style="text-align:center;padding:0.75rem;border-radius:0.5rem;background:#f8fafc;border:1px solid var(--border)">
                    <div style="font-size:0.7rem;text-transform:uppercase;color:var(--muted)">Certificates</div>
                    <div style="font-size:1.1rem;font-weight:700">${g.cert_count || 0}</div>
                </div>
                <div style="text-align:center;padding:0.75rem;border-radius:0.5rem;background:#f8fafc;border:1px solid var(--border)">
                    <div style="font-size:0.7rem;text-transform:uppercase;color:var(--muted)">Agents</div>
                    <div style="font-size:1.1rem;font-weight:700">${g.agent_count || 0}</div>
                </div>
                <div style="text-align:center;padding:0.75rem;border-radius:0.5rem;background:#f8fafc;border:1px solid var(--border)">
                    <div style="font-size:0.7rem;text-transform:uppercase;color:var(--muted)">Connector</div>
                    <div style="font-size:0.9rem;font-weight:600">${g.connector_mode || 'direct'}</div>
                </div>
            </div>
            <table style="font-size:0.82rem">
                <thead><tr><th>Service</th><th>Status</th><th>Details</th></tr></thead>
                <tbody>
                    <tr><td>CLM Gateway</td><td><span class="badge ${g.status==='online'?'badge-active':'badge-disabled'}">${g.status==='online'?'running':'stopped'}</span></td><td>Main gateway process (port 8080)</td></tr>
                    <tr><td>vSatellite API</td><td><span class="badge ${g.status==='online'?'badge-active':'badge-disabled'}">${g.status==='online'?'running':'stopped'}</span></td><td>Agent coordination (port 9090)</td></tr>
                    <tr><td>SaaS Connector</td><td><span class="badge ${g.status==='online'?'badge-active':'badge-warning'}">${g.status==='online'?'connected':'disconnected'}</span></td><td>Outbound to SaaS control plane</td></tr>
                    <tr><td>Crypto Engine</td><td><span class="badge badge-active">ready</span></td><td>Key generation, CSR, chain assembly</td></tr>
                    <tr><td>Secrets Store</td><td><span class="badge badge-active">configured</span></td><td>${g.secrets_backend || 'aws'} secrets manager</td></tr>
                    <tr><td>Discovery Scanner</td><td><span class="badge badge-active">scheduled</span></td><td>Network TLS certificate scanning</td></tr>
                </tbody>
            </table>
            <p style="font-size:0.75rem;color:var(--muted);margin-top:0.5rem">Last updated: ${lastHB}</p>`;
    }).catch(() => {
        document.getElementById('gw-service-status').innerHTML = '<p style="color:var(--muted);font-size:0.85rem">Could not load service status.</p>';
    });

    // Load command history
    apiGet('/api/v1/satellites/' + id + '/commands').then(data => {
        const cmds = data.commands || [];
        if (cmds.length === 0) {
            document.getElementById('gw-command-history').innerHTML = '<p style="color:var(--muted);font-size:0.85rem">No commands sent yet.</p>';
        } else {
            const rows = cmds.map(c => `<div style="display:flex;align-items:center;padding:0.4rem 0;border-bottom:1px solid var(--border)">
                <span class="badge ${c.status==='completed'?'badge-active':c.status==='sent'?'badge-warning':'badge-disabled'}" style="margin-right:0.5rem">${c.status}</span>
                <span style="flex:1;font-size:0.82rem">${c.type}</span>
                <span style="font-size:0.75rem;color:var(--muted)">${c.created_at ? new Date(c.created_at).toLocaleString() : ''}</span>
            </div>`).join('');
            document.getElementById('gw-command-history').innerHTML = rows;
        }
    }).catch(() => {
        document.getElementById('gw-command-history').innerHTML = '<p style="color:var(--muted);font-size:0.85rem">Could not load command history.</p>';
    });
}

function deployGatewayServices(id, name) {
    if (!confirm('Deploy latest gateway services to "' + name + '"? This will pull and install the latest version.')) return;
    document.getElementById('gw-action-status').innerHTML = '<div class="card" style="border-left:3px solid var(--success)"><p style="font-size:0.85rem">Deploying services...</p></div>';

    apiPost('/api/v1/satellites/' + id + '/upgrade', { command: 'deploy_services' }).then(data => {
        document.getElementById('gw-action-status').innerHTML = `<div class="card" style="border-left:3px solid var(--success)"><p style="font-size:0.85rem;color:#065f46">Deploy command sent. The gateway will pull the latest version on next heartbeat.</p></div>`;
    }).catch(() => {
        document.getElementById('gw-action-status').innerHTML = '<div class="card" style="border-left:3px solid var(--danger)"><p style="font-size:0.85rem;color:var(--danger)">Failed to send deploy command.</p></div>';
    });
}

function restartGatewayServices(id, name) {
    if (!confirm('Restart gateway services on "' + name + '"? This will briefly interrupt the gateway.')) return;
    document.getElementById('gw-action-status').innerHTML = '<div class="card" style="border-left:3px solid #f59e0b"><p style="font-size:0.85rem">Restarting services...</p></div>';

    apiPost('/api/v1/satellites/' + id + '/upgrade', { command: 'restart_services' }).then(data => {
        document.getElementById('gw-action-status').innerHTML = `<div class="card" style="border-left:3px solid var(--success)"><p style="font-size:0.85rem;color:#065f46">Restart command sent. The gateway will restart on next heartbeat.</p></div>`;
    }).catch(() => {
        document.getElementById('gw-action-status').innerHTML = '<div class="card" style="border-left:3px solid var(--danger)"><p style="font-size:0.85rem;color:var(--danger)">Failed to send restart command.</p></div>';
    });
}

function accessGatewayServer(id, ip, name) {
    const gatewayIp = ip || (JSON.parse(localStorage.getItem('clm_gateway_urls') || '{}')[id] || '').replace('http://','').replace('https://','');

    const statusEl = document.getElementById('gw-action-status');
    if (gatewayIp) {
        statusEl.innerHTML = `<div class="card">
            <h3 style="margin-bottom:0.75rem">Server Access — ${name}</h3>
            <table style="font-size:0.85rem">
                <tr><td style="font-weight:600;width:140px">SSH Command</td><td><code style="background:#f1f5f9;padding:0.3rem 0.5rem;border-radius:0.25rem;font-size:0.8rem">ssh -i ~/.ssh/clm-saas-deploy.pem ubuntu@${gatewayIp}</code></td></tr>
                <tr><td style="font-weight:600">IP Address</td><td>${gatewayIp}</td></tr>
                <tr><td style="font-weight:600">User</td><td>ubuntu</td></tr>
                <tr><td style="font-weight:600">Key</td><td>~/.ssh/clm-saas-deploy.pem</td></tr>
                <tr><td style="font-weight:600">Gateway Logs</td><td><code style="font-size:0.8rem">sudo docker logs clm-gateway-clm-gateway-1 -f</code></td></tr>
                <tr><td style="font-weight:600">Restart</td><td><code style="font-size:0.8rem">sudo docker compose restart clm-gateway</code></td></tr>
                <tr><td style="font-weight:600">Status</td><td><code style="font-size:0.8rem">sudo docker compose ps</code></td></tr>
            </table>
        </div>`;
    } else {
        statusEl.innerHTML = `<div class="card" style="border-left:3px solid #f59e0b"><p style="font-size:0.85rem;color:#92400e">No IP address available. Deploy the gateway first or set the IP via Edit.</p></div>`;
    }
}

function showDeployForm() {
    document.getElementById('deploy-form-card').classList.remove('hidden');
    // Populate cloud account dropdown
    const select = document.getElementById('d-cloud-account');
    const accounts = getCloudAccounts();
    select.innerHTML = '<option value="">-- Select a registered cloud account --</option>';
    accounts.forEach(a => {
        select.innerHTML += `<option value="${a.id}">${a.name} (${a.cloud.toUpperCase()} - ${a.region})</option>`;
    });
    if (accounts.length === 0) {
        select.innerHTML = '<option value="">-- No cloud accounts registered --</option>';
    }
    // Auto-select if only one account
    if (accounts.length === 1) {
        select.value = accounts[0].id;
        onCloudAccountSelect();
    }
    onDeployModeChange(); // Render mode-specific config
}
function hideDeployForm() {
    document.getElementById('deploy-form-card').classList.add('hidden');
}
function onDeployModeChange() {
    const mode = document.getElementById('d-mode').value;
    const el = document.getElementById('deploy-mode-config');

    if (mode === 'lightsail') {
        el.innerHTML = `
            <div style="background:#f0f9ff;border:1px solid #bae6fd;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#0369a1;font-weight:600;margin-bottom:0.25rem">Lightsail VM</p>
                <p style="font-size:0.75rem;color:#0c4a6e">Single VM with static IP. Best for dev/test or small deployments.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>VM Size</label><select id="d-size" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="nano_3_1">Nano (1 vCPU, 512MB) — $3.50/mo</option>
                    <option value="micro_3_1">Micro (1 vCPU, 1GB) — $5/mo</option>
                    <option value="small_3_1">Small (1 vCPU, 2GB) — $10/mo</option>
                    <option value="medium_3_1" selected>Medium (2 vCPU, 4GB) — $20/mo</option>
                    <option value="large_3_1">Large (2 vCPU, 8GB) — $40/mo</option>
                    <option value="xlarge_3_1">XLarge (4 vCPU, 16GB) — $80/mo</option>
                </select></div>
                <div class="form-group"><label>Availability Zone</label><select id="d-az" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="">Auto (default AZ)</option>
                    <option value="a">Zone A</option>
                    <option value="b">Zone B</option>
                    <option value="c">Zone C</option>
                </select></div>
            </div>`;
    } else if (mode === 'ec2') {
        el.innerHTML = `
            <div style="background:#f0fdf4;border:1px solid #bbf7d0;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#166534;font-weight:600;margin-bottom:0.25rem">EC2 + Application Load Balancer</p>
                <p style="font-size:0.75rem;color:#14532d">Auto-scaling EC2 instances behind ALB. Best for production workloads.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Instance Type</label><select id="d-size" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="t3.small">t3.small (2 vCPU, 2GB)</option>
                    <option value="t3.medium" selected>t3.medium (2 vCPU, 4GB)</option>
                    <option value="t3.large">t3.large (2 vCPU, 8GB)</option>
                    <option value="t3.xlarge">t3.xlarge (4 vCPU, 16GB)</option>
                    <option value="m5.large">m5.large (2 vCPU, 8GB)</option>
                    <option value="m5.xlarge">m5.xlarge (4 vCPU, 16GB)</option>
                    <option value="c5.large">c5.large (2 vCPU, 4GB, compute)</option>
                    <option value="c5.xlarge">c5.xlarge (4 vCPU, 8GB, compute)</option>
                </select></div>
                <div class="form-group"><label>VPC ID (optional)</label><input id="d-vpc" placeholder="vpc-0abc123 (default VPC if empty)" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Subnet ID (optional)</label><input id="d-subnet" placeholder="subnet-0abc123" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Min Instances</label><input id="d-min" type="number" value="1" min="1" max="10" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Max Instances (auto-scale)</label><input id="d-max" type="number" value="3" min="1" max="20" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div class="form-group"><label>Target Group ARN (optional — attach to existing ALB)</label><input id="d-tg" placeholder="arn:aws:elasticloadbalancing:..." style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>`;
    } else if (mode === 'eks') {
        el.innerHTML = `
            <div style="background:#faf5ff;border:1px solid #e9d5ff;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#6b21a8;font-weight:600;margin-bottom:0.25rem">EKS (Kubernetes)</p>
                <p style="font-size:0.75rem;color:#581c87">Deploy as pods in an existing EKS cluster. Best for cloud-native environments.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>EKS Cluster Name</label><input id="d-cluster" placeholder="my-production-cluster" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Namespace</label><input id="d-namespace" placeholder="clm-gateway" value="clm-gateway" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>CPU Request</label><select id="d-cpu" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="250m">250m (0.25 CPU)</option>
                    <option value="500m" selected>500m (0.5 CPU)</option>
                    <option value="1000m">1000m (1 CPU)</option>
                    <option value="2000m">2000m (2 CPU)</option>
                </select></div>
                <div class="form-group"><label>Memory Request</label><select id="d-memory" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="256Mi">256Mi</option>
                    <option value="512Mi" selected>512Mi</option>
                    <option value="1Gi">1Gi</option>
                    <option value="2Gi">2Gi</option>
                    <option value="4Gi">4Gi</option>
                </select></div>
                <div class="form-group"><label>Replicas</label><input id="d-replicas" type="number" value="2" min="1" max="10" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Min Pods (HPA)</label><input id="d-min" type="number" value="2" min="1" max="10" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Max Pods (HPA auto-scale)</label><input id="d-max" type="number" value="5" min="1" max="50" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div class="form-group"><label>Ingress Class (optional)</label><input id="d-ingress" placeholder="nginx or alb" value="alb" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>`;
    } else if (mode === 'gce') {
        el.innerHTML = `
            <div style="background:#fff7ed;border:1px solid #fed7aa;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#c2410c;font-weight:600">GCP Compute Engine (VM)</p>
                <p style="font-size:0.75rem;color:#7c2d12">Single VM with external IP on Google Cloud.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Machine Type</label><select id="d-size" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="e2-small">e2-small (2 vCPU, 2GB)</option>
                    <option value="e2-medium" selected>e2-medium (2 vCPU, 4GB)</option>
                    <option value="e2-standard-2">e2-standard-2 (2 vCPU, 8GB)</option>
                    <option value="e2-standard-4">e2-standard-4 (4 vCPU, 16GB)</option>
                    <option value="n2-standard-2">n2-standard-2 (2 vCPU, 8GB)</option>
                </select></div>
                <div class="form-group"><label>Zone</label><input id="d-az" placeholder="asia-south1-a" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Network</label><input id="d-vpc" placeholder="default" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Subnet</label><input id="d-subnet" placeholder="default" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>`;
    } else if (mode === 'gke') {
        el.innerHTML = `
            <div style="background:#fff7ed;border:1px solid #fed7aa;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#c2410c;font-weight:600">GCP GKE (Kubernetes)</p>
                <p style="font-size:0.75rem;color:#7c2d12">Deploy as pods in an existing GKE cluster.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>GKE Cluster</label><input id="d-cluster" placeholder="my-gke-cluster" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Namespace</label><input id="d-namespace" value="clm-gateway" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>CPU</label><select id="d-cpu" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"><option value="250m">250m</option><option value="500m" selected>500m</option><option value="1000m">1000m</option></select></div>
                <div class="form-group"><label>Memory</label><select id="d-memory" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"><option value="512Mi" selected>512Mi</option><option value="1Gi">1Gi</option><option value="2Gi">2Gi</option></select></div>
                <div class="form-group"><label>Replicas</label><input id="d-replicas" type="number" value="2" min="1" max="10" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Min Pods (HPA)</label><input id="d-min" type="number" value="2" min="1" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Max Pods (HPA)</label><input id="d-max" type="number" value="5" min="1" max="50" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>`;
    } else if (mode === 'azure_vm') {
        el.innerHTML = `
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#1d4ed8;font-weight:600">Azure Virtual Machine</p>
                <p style="font-size:0.75rem;color:#1e3a5f">Single VM with public IP on Azure.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>VM Size</label><select id="d-size" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                    <option value="Standard_B1ms">B1ms (1 vCPU, 2GB)</option>
                    <option value="Standard_B2s" selected>B2s (2 vCPU, 4GB)</option>
                    <option value="Standard_B2ms">B2ms (2 vCPU, 8GB)</option>
                    <option value="Standard_D2s_v3">D2s_v3 (2 vCPU, 8GB)</option>
                    <option value="Standard_D4s_v3">D4s_v3 (4 vCPU, 16GB)</option>
                </select></div>
                <div class="form-group"><label>Resource Group</label><input id="d-rg" placeholder="clm-gateway-rg" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>VNet</label><input id="d-vpc" placeholder="my-vnet" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Subnet</label><input id="d-subnet" placeholder="default" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>`;
    } else if (mode === 'aks') {
        el.innerHTML = `
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:0.5rem;padding:0.75rem;margin-bottom:0.75rem">
                <p style="font-size:0.8rem;color:#1d4ed8;font-weight:600">Azure AKS (Kubernetes)</p>
                <p style="font-size:0.75rem;color:#1e3a5f">Deploy as pods in an existing AKS cluster.</p>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>AKS Cluster</label><input id="d-cluster" placeholder="my-aks-cluster" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Resource Group</label><input id="d-rg" placeholder="clm-rg" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>Namespace</label><input id="d-namespace" value="clm-gateway" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label>Replicas</label><input id="d-replicas" type="number" value="2" min="1" max="10" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:0.75rem">
                <div class="form-group"><label>CPU</label><select id="d-cpu" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"><option value="250m">250m</option><option value="500m" selected>500m</option><option value="1000m">1000m</option></select></div>
                <div class="form-group"><label>Memory</label><select id="d-memory" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"><option value="512Mi" selected>512Mi</option><option value="1Gi">1Gi</option><option value="2Gi">2Gi</option></select></div>
                <div class="form-group"><label>Max Pods (HPA)</label><input id="d-max" type="number" value="5" min="1" max="50" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>`;
    }
}
function onCloudAccountSelect() {
    const id = document.getElementById('d-cloud-account').value;
    const accounts = getCloudAccounts();
    const account = accounts.find(a => a.id === id);
    if (account) {
        document.getElementById('d-region').value = account.region || '';
        // Update deploy mode options based on cloud provider
        const modeSelect = document.getElementById('d-mode');
        if (account.cloud === 'aws') {
            modeSelect.innerHTML = `
                <option value="lightsail">Lightsail (VM — simplest)</option>
                <option value="ec2">EC2 + ALB (production VM)</option>
                <option value="eks">EKS (Kubernetes)</option>`;
        } else if (account.cloud === 'gcp') {
            modeSelect.innerHTML = `
                <option value="gce">Compute Engine (VM)</option>
                <option value="gke">GKE (Kubernetes)</option>`;
        } else if (account.cloud === 'azure') {
            modeSelect.innerHTML = `
                <option value="azure_vm">Azure VM</option>
                <option value="aks">AKS (Kubernetes)</option>`;
        }
        onDeployModeChange(); // Refresh config fields
    }
}

function startDeploy() {
    const accountId = document.getElementById('d-cloud-account').value;
    const accounts = getCloudAccounts();
    const account = accounts.find(a => a.id === accountId);

    if (!account) {
        document.getElementById('deploy-status').innerHTML = '<p style="color:var(--danger)">Please select a cloud account.</p>';
        return;
    }

    const name = document.getElementById('d-name').value.trim();
    if (!name) {
        document.getElementById('deploy-status').innerHTML = '<p style="color:var(--danger)">Gateway name is required.</p>';
        return;
    }

    const mode = document.getElementById('d-mode').value;
    const region = document.getElementById('d-region').value || account.region;

    // Build infra config based on mode
    const infra = {};
    if (mode === 'lightsail') {
        infra.lightsail_bundle = document.getElementById('d-size').value;
        const az = document.getElementById('d-az').value;
        if (az) infra.lightsail_az = region + az;
    } else if (mode === 'ec2') {
        infra.instance_type = document.getElementById('d-size').value;
        const vpc = document.getElementById('d-vpc').value.trim();
        const subnet = document.getElementById('d-subnet').value.trim();
        const tg = document.getElementById('d-tg').value.trim();
        if (vpc) infra.vpc_id = vpc;
        if (subnet) infra.subnet_id = subnet;
        if (tg) infra.target_group_arn = tg;
        infra.min_instances = parseInt(document.getElementById('d-min').value) || 1;
        infra.max_instances = parseInt(document.getElementById('d-max').value) || 3;
    } else if (mode === 'eks') {
        const cluster = document.getElementById('d-cluster').value.trim();
        if (!cluster) {
            document.getElementById('deploy-status').innerHTML = '<p style="color:var(--danger)">EKS cluster name is required.</p>';
            return;
        }
        infra.cluster_name = cluster;
        infra.namespace = document.getElementById('d-namespace').value.trim() || 'clm-gateway';
        infra.cpu_request = document.getElementById('d-cpu').value;
        infra.memory_request = document.getElementById('d-memory').value;
        infra.replicas = parseInt(document.getElementById('d-replicas').value) || 2;
        infra.min_instances = parseInt(document.getElementById('d-min').value) || 2;
        infra.max_instances = parseInt(document.getElementById('d-max').value) || 5;
        infra.ingress_class = document.getElementById('d-ingress').value.trim() || 'alb';
    }

    const body = {
        name: name,
        tenant_id: activeTenantId,
        cloud_account_id: account.id,
        cloud: account.cloud,
        mode: mode,
        region: region,
        infra: infra
    };

    document.getElementById('deploy-status').innerHTML = '<p style="color:var(--muted)">Validating credentials and deploying gateway...</p>';

    apiPost(scopedPath('/api/v1/satellites/deploy', activeTenantId), body).then(data => {
        if (data.error) {
            document.getElementById('deploy-status').innerHTML = `<p style="color:var(--danger)">Error: ${data.error}</p>`;
            return;
        }
        document.getElementById('deploy-status').innerHTML = `
            <div style="background:#d1fae5;border:1px solid #6ee7b7;border-radius:0.5rem;padding:1rem;margin-top:0.5rem">
                <p style="color:#065f46;font-weight:600;margin-bottom:0.5rem">Deployment Initiated</p>
                <p style="color:#065f46;font-size:0.85rem">Job ID: <code>${data.job_id}</code></p>
                <p style="color:#065f46;font-size:0.85rem">State: ${data.state} | Mode: ${mode}</p>
                <button class="btn btn-primary" style="margin-top:0.75rem" onclick="viewDeployJob('${data.job_id}')">View Progress</button>
            </div>`;
    }).catch(err => {
        document.getElementById('deploy-status').innerHTML = `<p style="color:var(--danger)">Error: ${err.message||'deployment failed'}</p>`;
    });
}

// --- Deploy Jobs Page ---
function loadDeployJobs() {
    const el = document.getElementById('content-area');
    el.innerHTML = '<div class="card"><p>Loading deployment jobs...</p></div>';
    document.getElementById('page-title').textContent = 'Deploy Jobs';

    resolveTenantScope().then(tenantId => apiGet(scopedPath('/api/v1/satellites/deploy', tenantId))).then(data => {
        const jobs = data.jobs || [];
        if (jobs.length === 0) {
            el.innerHTML = `<div class="card">
                <h3>Deployment Jobs</h3>
                <p style="color:var(--muted);margin-top:0.5rem">No deployment jobs found. Go to <a href="#" onclick="navigate('gateways');return false" style="color:var(--primary)">Gateways</a> and click "+ Deploy New Gateway" to start one.</p>
            </div>`;
            return;
        }
        let rows = jobs.map(j => {
            const stateColor = j.state === 'online' ? 'var(--success)' : j.state === 'failed' ? 'var(--danger)' : j.state === 'deploying' ? '#f59e0b' : 'var(--muted)';
            const stateIcon = j.state === 'online' ? '&#10003;' : j.state === 'failed' ? '&#10007;' : j.state === 'deploying' ? '&#9881;' : '&#8226;';
            return `<tr style="cursor:pointer" onclick="viewDeployJob('${j.id}')">
                <td><strong>${j.name}</strong></td>
                <td>${j.cloud||'aws'}</td>
                <td>${j.mode||'lightsail'}</td>
                <td>${j.region||'-'}</td>
                <td><span style="color:${stateColor};font-weight:600">${stateIcon} ${j.state}</span></td>
                <td>${j.public_ip||'-'}</td>
                <td>${j.current_step||0}/${(j.steps||[]).length||'?'}</td>
                <td>${j.created_at ? new Date(j.created_at).toLocaleString() : '-'}</td>
            </tr>`;
        }).join('');
        el.innerHTML = `<div class="card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                <h3>Deployment Jobs</h3>
                <button class="btn btn-primary" onclick="navigate('gateways')">+ New Deploy</button>
            </div>
            <table><thead><tr><th>Name</th><th>Cloud</th><th>Mode</th><th>Region</th><th>State</th><th>IP</th><th>Progress</th><th>Created</th></tr></thead>
            <tbody>${rows}</tbody></table>
        </div>`;
    }).catch(err => {
        el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load jobs: ${err.message}</p></div>`;
    });
}

// --- Deploy Job Detail View ---
let jobPollTimer = null;

function viewDeployJob(jobId) {
    if (jobPollTimer) { clearInterval(jobPollTimer); jobPollTimer = null; }
    const el = document.getElementById('content-area');
    document.getElementById('page-title').textContent = 'Deploy Job';
    el.innerHTML = '<div class="card"><p>Loading job details...</p></div>';

    function renderJob() {
        apiGet(scopedPath('/api/v1/satellites/deploy/' + jobId, activeTenantId)).then(job => {
            const j = job;
            const stateColor = j.state === 'online' ? 'var(--success)' : j.state === 'failed' ? 'var(--danger)' : j.state === 'deploying' ? '#f59e0b' : 'var(--muted)';

            let stepsHtml = '';
            if (j.steps && j.steps.length > 0) {
                stepsHtml = j.steps.map((step, idx) => {
                    let icon = '&#9675;';  // empty circle
                    let color = 'var(--muted)';
                    if (step.status === 'done') { icon = '&#10003;'; color = 'var(--success)'; }
                    else if (step.status === 'running') { icon = '&#9881;'; color = '#f59e0b'; }
                    else if (step.status === 'failed') { icon = '&#10007;'; color = 'var(--danger)'; }
                    const duration = step.started_at && step.done_at ? ((new Date(step.done_at) - new Date(step.started_at))/1000).toFixed(1) + 's' : '';
                    return `<div style="display:flex;align-items:center;padding:0.5rem 0;border-bottom:1px solid var(--border)">
                        <span style="color:${color};font-size:1.1rem;width:24px;text-align:center">${icon}</span>
                        <span style="flex:1;margin-left:0.5rem;font-size:0.85rem">${step.name}</span>
                        <span style="font-size:0.75rem;color:var(--muted)">${duration}</span>
                        ${step.error ? '<span style="font-size:0.75rem;color:var(--danger);margin-left:0.5rem" title="'+step.error+'">error</span>' : ''}
                    </div>`;
                }).join('');
            }

            el.innerHTML = `
                <div style="margin-bottom:1rem"><button class="btn" onclick="loadDeployJobs()" style="border:1px solid var(--border)">&larr; Back to Jobs</button></div>
                <div class="card">
                    <div style="display:flex;justify-content:space-between;align-items:flex-start">
                        <div>
                            <h3 style="margin-bottom:0.25rem">${j.name||'Deploy Job'}</h3>
                            <p style="color:var(--muted);font-size:0.8rem">Job ID: ${j.id}</p>
                        </div>
                        <span class="badge" style="background:${stateColor}20;color:${stateColor};font-size:0.85rem;padding:0.3rem 0.8rem;font-weight:600">${j.state}</span>
                    </div>
                </div>
                <div class="stats" style="grid-template-columns:repeat(auto-fit,minmax(140px,1fr))">
                    <div class="card stat"><div class="label">Cloud</div><div class="value" style="font-size:1.1rem">${j.cloud||'aws'}</div></div>
                    <div class="card stat"><div class="label">Mode</div><div class="value" style="font-size:1.1rem">${j.mode||'lightsail'}</div></div>
                    <div class="card stat"><div class="label">Region</div><div class="value" style="font-size:1.1rem">${j.region||'-'}</div></div>
                    <div class="card stat"><div class="label">Public IP</div><div class="value" style="font-size:1.1rem">${j.public_ip||'Pending...'}</div></div>
                    <div class="card stat"><div class="label">Resource</div><div class="value" style="font-size:0.9rem">${j.resource_id||'Pending...'}</div></div>
                </div>
                <div class="card">
                    <h3 style="margin-bottom:0.75rem">Deployment Steps</h3>
                    ${stepsHtml || '<p style="color:var(--muted);font-size:0.85rem">No step data available yet.</p>'}
                </div>
                ${j.state === 'deploying' || j.state === 'pending' || j.state === 'validating' ? '<div class="card" style="border-left:3px solid #f59e0b"><p style="font-size:0.85rem;color:#92400e">Deployment in progress. This page auto-refreshes every 5 seconds.</p></div>' : ''}
                ${j.state === 'online' && j.public_ip ? '<div class="card" style="border-left:3px solid var(--success)"><p style="font-size:0.85rem;color:#065f46">Gateway is online at <strong>'+j.public_ip+'</strong>. Access: <a href="http://'+j.public_ip+':8080" target="_blank" style="color:var(--primary)">http://'+j.public_ip+':8080</a></p></div>' : ''}
                ${j.state === 'failed' ? '<div class="card" style="border-left:3px solid var(--danger)"><p style="font-size:0.85rem;color:#991b1b">Deployment failed. Check the step errors above for details.</p></div>' : ''}
            `;

            // Auto-refresh if still deploying
            if (j.state === 'deploying' || j.state === 'pending' || j.state === 'validating') {
                if (!jobPollTimer) {
                    jobPollTimer = setInterval(renderJob, 5000);
                }
            } else {
                if (jobPollTimer) { clearInterval(jobPollTimer); jobPollTimer = null; }
            }
        }).catch(err => {
            el.innerHTML = `<div class="card"><p style="color:var(--danger)">Failed to load job: ${err.message}</p></div>`;
            if (jobPollTimer) { clearInterval(jobPollTimer); jobPollTimer = null; }
        });
    }

    renderJob();
}


// ============================================================
// Identity Management — native Keycloak administration
// Talks to /api/v1/identity/* which proxies the Keycloak Admin
// REST API, so the Keycloak Admin Console is never exposed.
// ============================================================

let identityTab = 'users';
const IDENTITY_TABS = [
    ['users', 'Users'],
    ['groups', 'Groups'],
    ['roles', 'Roles'],
    ['providers', 'Identity Providers'],
    ['sessions', 'Active Sessions'],
    ['clients', 'Clients']
];

function loadIdentity(initialTab) {
    if (initialTab && IDENTITY_TABS.some(t => t[0] === initialTab)) {
        identityTab = initialTab;
    }
    document.getElementById('page-title').textContent = 'Identity Management';
    const el = document.getElementById('content-area');

    const tabs = IDENTITY_TABS.map(t =>
        '<button class="btn" id="idtab-' + t[0] + '" onclick="switchIdentityTab(\'' + t[0] + '\')" ' +
        'style="border-radius:0;background:none;color:var(--muted);border-bottom:2px solid transparent;padding:0.7rem 1.1rem">' +
        t[1] + '</button>'
    ).join('');

    el.innerHTML = `
        <div class="stats" id="id-stats">
            <div class="card stat"><div class="label">Realm Users</div><div class="value" id="id-s-users">-</div></div>
            <div class="card stat"><div class="label">Enabled</div><div class="value" id="id-s-enabled">-</div></div>
            <div class="card stat"><div class="label">Roles</div><div class="value" id="id-s-roles">-</div></div>
            <div class="card stat"><div class="label">Active Sessions</div><div class="value" id="id-s-sessions">-</div></div>
        </div>
        <div id="id-banner"></div>
        <div class="card" style="padding:0;margin-bottom:1rem">
            <div style="display:flex;flex-wrap:wrap;border-bottom:1px solid var(--border)">${tabs}</div>
        </div>
        <div id="id-body"><div class="card"><p style="color:var(--muted)">Loading...</p></div></div>
    `;

    loadIdentityStats();
    switchIdentityTab(identityTab);
}

function loadIdentityStats() {
    apiGet('/api/v1/identity/status').then(s => {
        const banner = document.getElementById('id-banner');
        if (!s || s.reachable === false) {
            if (banner) banner.innerHTML = '<div class="card" style="border-left:3px solid var(--danger)">' +
                '<p style="font-size:0.85rem;color:#991b1b">Keycloak Admin API is not reachable. ' +
                'Check that the keycloak container is running and KEYCLOAK_ADMIN_PASS is set on clm-server.</p></div>';
            return;
        }
        if (banner) banner.innerHTML = '';
        setText('id-s-users', s.users);
        setText('id-s-enabled', s.enabled_users);
        setText('id-s-roles', s.roles);
        setText('id-s-sessions', s.active_sessions);
    }).catch(err => {
        const banner = document.getElementById('id-banner');
        if (banner) banner.innerHTML = '<div class="card" style="border-left:3px solid var(--danger)">' +
            '<p style="font-size:0.85rem;color:#991b1b">Identity API error: ' + esc(err.message) + '</p></div>';
    });
}

function setText(id, val) {
    const n = document.getElementById(id);
    if (n) n.textContent = (val === undefined || val === null) ? '-' : val;
}

// esc guards against HTML/attribute injection when rendering values that come
// back from Keycloak. Ampersand must be replaced first.
function esc(s) {
    if (s === undefined || s === null) return '';
    return String(s)
        .split('&').join('&amp;')
        .split('<').join('&lt;')
        .split('>').join('&gt;')
        .split('"').join('&quot;')
        .split("'").join('&#39;');
}

// jsq escapes a value that is embedded inside a single-quoted JS string within
// an HTML attribute, e.g. onclick="fn('VALUE')". The browser HTML-decodes the
// attribute before parsing the JS, so the JS escaping must be applied first.
function jsq(s) {
    if (s === undefined || s === null) return '';
    return esc(String(s).split('\\').join('\\\\').split("'").join("\\'"));
}

function switchIdentityTab(tab) {
    identityTab = tab;
    IDENTITY_TABS.forEach(t => {
        const b = document.getElementById('idtab-' + t[0]);
        if (!b) return;
        const on = t[0] === tab;
        b.style.color = on ? 'var(--primary)' : 'var(--muted)';
        b.style.borderBottom = on ? '2px solid var(--primary)' : '2px solid transparent';
    });
    IDENTITY_TABS.forEach(t => {
        const b = document.getElementById('idtab-' + t[0]);
        if (b) {
            const on = t[0] === tab;
            b.style.background = on ? '#eff6ff' : 'none';
            b.style.fontWeight = on ? '700' : '600';
        }
    });

    if (tab === 'users') renderIdentityUsers();
    else if (tab === 'groups') renderIdentityGroups();
    else if (tab === 'roles') renderIdentityRoles();
    else if (tab === 'providers') renderIdentityProviders();
    else if (tab === 'sessions') renderIdentitySessions();
    else if (tab === 'clients') renderIdentityClients();
}

// --- Users tab ---
let identityRoleCache = [];

function renderIdentityUsers(search) {
    const body = document.getElementById('id-body');
    if (!body) return;
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading users...</p></div>';

    const q = search ? '?search=' + encodeURIComponent(search) : '';
    Promise.all([
        apiGet('/api/v1/identity/users' + q),
        apiGet('/api/v1/identity/roles')
    ]).then(([ud, rd]) => {
        identityRoleCache = (rd.roles || []).map(r => r.name).sort();
        const users = ud.users || [];

        const rows = users.length === 0
            ? '<tr><td colspan="6" style="color:var(--muted)">No users found</td></tr>'
            : users.map(u => identityUserRow(u)).join('');

        body.innerHTML = `
            <div class="card" style="margin-bottom:1rem;display:flex;gap:0.5rem;align-items:center;flex-wrap:wrap">
                <input id="id-search" placeholder="Search username, email or name"
                       value="${esc(search || '')}"
                       style="flex:1;min-width:220px;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem;font-size:0.82rem">
                <button class="btn btn-primary" onclick="identitySearch()">Search</button>
                <button class="btn" style="background:#e2e8f0" onclick="renderIdentityUsers()">Reset</button>
                <button class="btn btn-primary" onclick="showCreateIdentityUser()">+ New User</button>
            </div>
            <div class="card">
                <table>
                    <thead><tr><th>Username</th><th>Email</th><th>Name</th><th>Role</th><th>Status</th><th style="width:1%">Actions</th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
            <div id="id-user-panel"></div>
        `;
        const input = document.getElementById('id-search');
        if (input) input.onkeydown = e => { if (e.key === 'Enter') identitySearch(); };
    }).catch(err => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load users: ' + esc(err.message) + '</p></div>';
    });
}

function identitySearch() {
    const input = document.getElementById('id-search');
    renderIdentityUsers(input ? input.value.trim() : '');
}

function identityUserRow(u) {
    const role = (u.roles && u.roles.length) ? u.roles.join(', ') : '<span style="color:var(--muted)">none</span>';
    let status = u.enabled
        ? '<span class="badge badge-active">Enabled</span>'
        : '<span class="badge badge-disabled">Disabled</span>';

    // Keycloak refuses a Direct Grant login — the flow both portals use — in two
    // cases that leave the account looking perfectly healthy while every login
    // fails with "Account is not fully set up":
    //   1. a stored required action (UPDATE_PASSWORD, VERIFY_EMAIL, ...)
    //   2. a missing required profile attribute; the realm requires firstName and
    //      lastName, and that action is derived at login, never stored.
    // Flag both so the cause is visible.
    const pending = u.requiredActions || [];
    const noName = !u.firstName || !u.lastName;
    const blocked = pending.length > 0 || noName;
    if (blocked) {
        const why = pending.length > 0
            ? 'Pending actions: ' + pending.join(', ')
            : 'Profile incomplete: first and last name are required by the realm';
        status += ' <span class="badge badge-disabled" style="background:#fef3c7;color:#92400e" ' +
                  'title="' + esc(why) + '. Login is refused with &quot;Account is not fully ' +
                  'set up&quot; until this is resolved.">Login blocked</span>';
    }

    const name = ((u.firstName || '') + ' ' + (u.lastName || '')).trim();
    const id = jsq(u.id);
    const uname = jsq(u.username);
    const fixBtn = blocked
        ? '<button class="btn btn-primary" onclick="unlockIdentityUser(\'' + id + '\',\'' + uname +
          '\',\'' + jsq(u.firstName || '') + '\',\'' + jsq(u.lastName || '') + '\')">Fix Login</button> '
        : '';
    return '<tr>' +
        '<td><strong>' + esc(u.username) + '</strong></td>' +
        '<td>' + esc(u.email) + '</td>' +
        '<td>' + esc(name) + '</td>' +
        '<td>' + role + '</td>' +
        '<td>' + status + '</td>' +
        '<td style="white-space:nowrap">' +
            fixBtn +
            '<button class="btn" style="background:#e2e8f0" onclick="showIdentityUser(\'' + id + '\')">Manage</button> ' +
            '<button class="btn" style="background:#e2e8f0" onclick="promptResetPassword(\'' + id + '\',\'' + uname + '\')">Password</button> ' +
            '<button class="btn" style="background:#e2e8f0" onclick="toggleIdentityUser(\'' + id + '\',' + (!u.enabled) + ')">' +
                (u.enabled ? 'Disable' : 'Enable') + '</button> ' +
            '<button class="btn btn-danger" onclick="deleteIdentityUser(\'' + id + '\',\'' + uname + '\')">Delete</button>' +
        '</td>' +
    '</tr>';
}

// unlockIdentityUser repairs an account that Keycloak refuses to authenticate:
// it clears pending required actions and completes the mandatory profile name.
// When the name is what's missing, the admin is asked for it so the account is
// not left with a value guessed from the email address.
function unlockIdentityUser(id, username, firstName, lastName) {
    const body = {};
    if (!firstName || !lastName) {
        const entered = prompt(
            'First and last name are required by the identity realm, and ' +
            username + ' is missing one.\n\n' +
            'Enter "First Last" to complete the profile (leave blank to derive ' +
            'it from the email address):',
            ((firstName || '') + ' ' + (lastName || '')).trim());
        if (entered === null) return;
        const parts = entered.trim().split(/\s+/).filter(Boolean);
        if (parts.length >= 2) {
            body.first_name = parts[0];
            body.last_name = parts.slice(1).join(' ');
        } else if (parts.length === 1) {
            body.first_name = parts[0];
        }
    } else if (!confirm('Clear all pending required actions for ' + username + '?\n\n' +
                        'This lets the account log in again. Any outstanding forced ' +
                        'password change or email verification will be dropped.')) {
        return;
    }
    apiPost('/api/v1/identity/users/' + id + '/unlock', body).then(d => {
        if (d.error) { alert(d.error); return; }
        alert('Login restored for ' + username + ':\n\n- ' +
              (d.changes || ['done']).join('\n- '));
        renderIdentityUsers();
    }).catch(e => alert(e.message));
}

function showCreateIdentityUser() {
    const panel = document.getElementById('id-user-panel');
    if (!panel) return;
    const roleOpts = identityRoleCache.map(r => '<option value="' + esc(r) + '">' + esc(r) + '</option>').join('');
    panel.innerHTML = `
        <div class="card" style="margin-top:1rem;border-left:3px solid var(--primary)">
            <h3 style="margin-bottom:0.75rem;font-size:0.95rem">Create Realm User</h3>
            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:0.75rem">
                <div class="form-group"><label class="label">Email *</label>
                    <input id="nu-email" type="email" placeholder="user@aiforgers.tech" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Username</label>
                    <input id="nu-username" placeholder="defaults to email" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">First Name *</label>
                    <input id="nu-first" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Last Name *</label>
                    <input id="nu-last" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Password * (min 8)</label>
                    <input id="nu-pass" type="password" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Role</label>
                    <select id="nu-role" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"><option value="">(none)</option>${roleOpts}</select></div>
            </div>
            <div style="font-size:0.78rem;color:var(--text-muted);background:#f8fafc;border-left:3px solid var(--warning,#d97706);padding:0.5rem 0.65rem;margin:0.6rem 0;border-radius:0.25rem">
                The password is set as permanent. "Require password change at first login"
                is deliberately not offered: both portals sign in with the OAuth Direct Grant
                flow, which cannot carry out a forced password change, so such an account
                would be rejected with <em>"Account is not fully set up"</em> and could never
                log in. The user is created in this tenant and can change their own password
                after signing in.
            </div>
            <div id="nu-error" style="color:var(--danger);font-size:0.8rem;margin-bottom:0.5rem"></div>
            <button class="btn btn-primary" onclick="createIdentityUser()">Create User</button>
            <button class="btn" style="background:#e2e8f0" onclick="document.getElementById('id-user-panel').innerHTML=''">Cancel</button>
        </div>
    `;
}

function createIdentityUser() {
    const err = document.getElementById('nu-error');
    const val = id => { const n = document.getElementById(id); return n ? n.value.trim() : ''; };
    const payload = {
        email: val('nu-email'),
        username: val('nu-username'),
        first_name: val('nu-first'),
        last_name: val('nu-last'),
        password: document.getElementById('nu-pass').value,
        role: val('nu-role'),
        email_verified: true
    };
    if (!payload.email) { err.textContent = 'Email is required'; return; }
    // The realm requires firstName and lastName. Without them Keycloak refuses
    // every login with "Account is not fully set up", so block it here rather
    // than create an account that cannot sign in.
    if (!payload.first_name || !payload.last_name) {
        err.textContent = 'First and last name are required — the identity realm ' +
            'treats them as mandatory profile fields and a user missing either one ' +
            'cannot log in to the portals or the gateway.';
        return;
    }
    if (!payload.password || payload.password.length < 8) { err.textContent = 'Password must be at least 8 characters'; return; }

    err.textContent = 'Creating...';
    apiPost('/api/v1/identity/users', payload).then(d => {
        if (d.error) { err.textContent = d.error; return; }
        if (d.warning) { alert('User created with a warning: ' + d.warning); }
        document.getElementById('id-user-panel').innerHTML = '';
        loadIdentityStats();
        renderIdentityUsers();
    }).catch(e => { err.textContent = e.message; });
}

function toggleIdentityUser(id, enabled) {
    apiPost('/api/v1/identity/users/' + id + '/enabled', { enabled: enabled }).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentityUsers();
    }).catch(e => alert(e.message));
}

function deleteIdentityUser(id, username) {
    if (!confirm('Delete Keycloak user "' + username + '"?\n\nThis cannot be undone and the user will lose access immediately.')) return;
    apiDelete('/api/v1/identity/users/' + id).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentityUsers();
    }).catch(e => alert(e.message));
}

function promptResetPassword(id, username) {
    const pw = prompt('New password for "' + username + '" (min 8 characters):');
    if (!pw) return;
    if (pw.length < 8) { alert('Password must be at least 8 characters'); return; }
    const temp = confirm('Require the user to change this password at next login?\n\nOK = temporary, Cancel = permanent');
    apiPost('/api/v1/identity/users/' + id + '/password', { password: pw, temporary: temp }).then(d => {
        alert(d.error ? ('Error: ' + d.error) : 'Password updated for ' + username);
    }).catch(e => alert(e.message));
}

function showIdentityUser(id) {
    const panel = document.getElementById('id-user-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="card" style="margin-top:1rem"><p style="color:var(--muted)">Loading user...</p></div>';

    apiGet('/api/v1/identity/users/' + id).then(d => {
        if (d.error) { panel.innerHTML = '<div class="card" style="margin-top:1rem"><p style="color:var(--danger)">' + esc(d.error) + '</p></div>'; return; }
        const u = d.user || {};
        const sessions = d.sessions || [];
        const current = (u.roles && u.roles.length) ? u.roles[0] : '';
        const roleOpts = identityRoleCache.map(r =>
            '<option value="' + esc(r) + '"' + (r === current ? ' selected' : '') + '>' + esc(r) + '</option>').join('');
        const created = u.createdTimestamp ? new Date(u.createdTimestamp).toLocaleString() : 'unknown';

        const sessRows = sessions.length === 0
            ? '<p style="font-size:0.8rem;color:var(--muted)">No active sessions.</p>'
            : '<table><thead><tr><th>IP</th><th>Started</th><th>Last Access</th></tr></thead><tbody>' +
              sessions.map(s => '<tr><td>' + esc(s.ipAddress) + '</td><td>' +
                  new Date(s.start).toLocaleString() + '</td><td>' +
                  new Date(s.lastAccess).toLocaleString() + '</td></tr>').join('') +
              '</tbody></table>';

        panel.innerHTML = `
            <div class="card" style="margin-top:1rem;border-left:3px solid var(--primary)">
                <h3 style="margin-bottom:0.25rem;font-size:0.95rem">${esc(u.username)}</h3>
                <p style="font-size:0.75rem;color:var(--muted);margin-bottom:0.75rem">Keycloak ID ${esc(u.id)} &middot; created ${esc(created)}</p>
                <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:0.75rem">
                    <div class="form-group"><label class="label">Email</label>
                        <input id="eu-email" value="${esc(u.email)}" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                    <div class="form-group"><label class="label">First Name</label>
                        <input id="eu-first" value="${esc(u.firstName)}" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                    <div class="form-group"><label class="label">Last Name</label>
                        <input id="eu-last" value="${esc(u.lastName)}" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                    <div class="form-group"><label class="label">Realm Role</label>
                        <select id="eu-role" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"><option value="">(none)</option>${roleOpts}</select></div>
                </div>
                <div id="eu-error" style="color:var(--danger);font-size:0.8rem;margin:0.5rem 0"></div>
                <button class="btn btn-primary" onclick="saveIdentityUser('${jsq(u.id)}')">Save Changes</button>
                <button class="btn" style="background:#e2e8f0" onclick="logoutIdentityUser('${jsq(u.id)}')">Revoke Sessions</button>
                <button class="btn" style="background:#e2e8f0" onclick="document.getElementById('id-user-panel').innerHTML=''">Close</button>
                <h4 style="margin:1rem 0 0.5rem;font-size:0.85rem">Active Sessions (${sessions.length})</h4>
                ${sessRows}
            </div>
        `;
    }).catch(e => {
        panel.innerHTML = '<div class="card" style="margin-top:1rem"><p style="color:var(--danger)">' + esc(e.message) + '</p></div>';
    });
}

function saveIdentityUser(id) {
    const err = document.getElementById('eu-error');
    const val = i => { const n = document.getElementById(i); return n ? n.value.trim() : ''; };
    const role = val('eu-role');
    err.textContent = 'Saving...';

    apiPut('/api/v1/identity/users/' + id, {
        email: val('eu-email'),
        first_name: val('eu-first'),
        last_name: val('eu-last')
    }).then(d => {
        if (d.error) { err.textContent = d.error; return null; }
        if (!role) return null;
        return apiPost('/api/v1/identity/users/' + id + '/role', { role: role });
    }).then(d => {
        if (d && d.error) { err.textContent = d.error; return; }
        err.textContent = '';
        document.getElementById('id-user-panel').innerHTML = '';
        renderIdentityUsers();
    }).catch(e => { err.textContent = e.message; });
}

function logoutIdentityUser(id) {
    if (!confirm('Revoke all sessions for this user? They will have to sign in again.')) return;
    apiPost('/api/v1/identity/users/' + id + '/logout', {}).then(d => {
        alert(d.error ? ('Error: ' + d.error) : 'All sessions revoked');
        loadIdentityStats();
        showIdentityUser(id);
    }).catch(e => alert(e.message));
}

// --- Roles tab ---
function renderIdentityRoles() {
    const body = document.getElementById('id-body');
    if (!body) return;
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading roles...</p></div>';

    apiGet('/api/v1/identity/roles').then(d => {
        const roles = d.roles || [];
        const rows = roles.length === 0
            ? '<tr><td colspan="3" style="color:var(--muted)">No roles defined</td></tr>'
            : roles.map(r => '<tr><td><strong>' + esc(r.name) + '</strong></td><td>' +
                esc(r.description) + '</td><td style="white-space:nowrap">' +
                '<button class="btn btn-danger" onclick="deleteIdentityRole(\'' + jsq(r.name) + '\')">Delete</button></td></tr>').join('');

        body.innerHTML = `
            <div class="card" style="margin-bottom:1rem;display:flex;gap:0.5rem;align-items:flex-end;flex-wrap:wrap">
                <div class="form-group" style="flex:1;min-width:160px"><label class="label">Role name</label>
                    <input id="nr-name" placeholder="e.g. cert_manager" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group" style="flex:2;min-width:200px"><label class="label">Description</label>
                    <input id="nr-desc" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <button class="btn btn-primary" onclick="createIdentityRole()">Add Role</button>
            </div>
            <div class="card">
                <table><thead><tr><th>Role</th><th>Description</th><th style="width:1%">Actions</th></tr></thead><tbody>${rows}</tbody></table>
            </div>
        `;
    }).catch(e => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load roles: ' + esc(e.message) + '</p></div>';
    });
}

function createIdentityRole() {
    const name = document.getElementById('nr-name').value.trim();
    const desc = document.getElementById('nr-desc').value.trim();
    if (!name) { alert('Role name is required'); return; }
    apiPost('/api/v1/identity/roles', { name: name, description: desc }).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentityRoles();
    }).catch(e => alert(e.message));
}

function deleteIdentityRole(name) {
    if (!confirm('Delete role "' + name + '"?\n\nUsers holding this role will lose the permissions it grants.')) return;
    apiDelete('/api/v1/identity/roles/' + encodeURIComponent(name)).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentityRoles();
    }).catch(e => alert(e.message));
}

// --- Identity Providers tab ---
function renderIdentityProviders() {
    const body = document.getElementById('id-body');
    if (!body) return;
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading identity providers...</p></div>';

    apiGet('/api/v1/identity/providers').then(d => {
        const idps = d.providers || [];
        const available = d.available || {};
        const socialOpts = Object.keys(available).map(k =>
            '<option value="' + esc(k) + '">' + esc(available[k]) + '</option>').join('');

        const rows = idps.length === 0
            ? '<tr><td colspan="5" style="color:var(--muted)">No external identity providers configured</td></tr>'
            : idps.map(p => {
                const redirect = window.location.origin + '/auth/realms/clm/broker/' + p.alias + '/endpoint';
                return '<tr><td><strong>' + esc(p.displayName || p.alias) + '</strong></td>' +
                    '<td>' + esc(p.providerId) + '</td>' +
                    '<td>' + (p.enabled ? '<span class="badge badge-active">Enabled</span>' : '<span class="badge badge-disabled">Disabled</span>') + '</td>' +
                    '<td style="font-size:0.7rem;color:var(--muted);word-break:break-all">' + esc(redirect) + '</td>' +
                    '<td style="white-space:nowrap">' +
                        '<button class="btn" style="background:#e2e8f0" onclick="toggleIdentityProvider(\'' + jsq(p.alias) + '\',' + (!p.enabled) + ')">' +
                            (p.enabled ? 'Disable' : 'Enable') + '</button> ' +
                        '<button class="btn btn-danger" onclick="deleteIdentityProvider(\'' + jsq(p.alias) + '\')">Delete</button>' +
                    '</td></tr>';
            }).join('');

        body.innerHTML = renderProviderTabHTML(socialOpts, rows);
    }).catch(e => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load providers: ' + esc(e.message) + '</p></div>';
    });
}

function renderProviderTabHTML(socialOpts, rows) {
    return `
        <div class="card" style="margin-bottom:1rem">
            <h3 style="margin-bottom:0.5rem;font-size:0.95rem">Add External Identity Provider</h3>
            <p style="font-size:0.78rem;color:var(--muted);margin-bottom:0.75rem">
                Register the redirect URI shown in the table below with the provider before enabling it.
            </p>
            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:0.75rem">
                <div class="form-group"><label class="label">Provider</label>
                    <select id="np-type" onchange="toggleOIDCFields()" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem">
                        ${socialOpts}<option value="oidc">Generic OIDC</option></select></div>
                <div class="form-group"><label class="label">Client ID *</label>
                    <input id="np-cid" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Client Secret *</label>
                    <input id="np-secret" type="password" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div id="np-oidc" class="hidden" style="display:none;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:0.75rem;margin-top:0.75rem">
                <div class="form-group"><label class="label">Alias *</label>
                    <input id="np-alias" placeholder="corp-sso" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Display Name</label>
                    <input id="np-name" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Authorization URL *</label>
                    <input id="np-auth" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Token URL *</label>
                    <input id="np-token" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">UserInfo URL</label>
                    <input id="np-userinfo" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                <div class="form-group"><label class="label">Issuer</label>
                    <input id="np-issuer" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
            </div>
            <div id="np-error" style="color:var(--danger);font-size:0.8rem;margin:0.5rem 0"></div>
            <button class="btn btn-primary" onclick="createIdentityProvider()">Add Provider</button>
        </div>
        <div class="card">
            <table><thead><tr><th>Name</th><th>Type</th><th>Status</th><th>Redirect URI</th><th style="width:1%">Actions</th></tr></thead>
            <tbody>${rows}</tbody></table>
        </div>
    `;
}

function toggleOIDCFields() {
    const type = document.getElementById('np-type').value;
    const box = document.getElementById('np-oidc');
    if (!box) return;
    box.style.display = (type === 'oidc') ? 'grid' : 'none';
}

function createIdentityProvider() {
    const err = document.getElementById('np-error');
    const val = i => { const n = document.getElementById(i); return n ? n.value.trim() : ''; };
    const type = val('np-type');

    const payload = {
        type: type,
        client_id: val('np-cid'),
        client_secret: val('np-secret')
    };
    if (!payload.client_id || !payload.client_secret) {
        err.textContent = 'Client ID and Client Secret are required';
        return;
    }
    if (type === 'oidc') {
        payload.alias = val('np-alias');
        payload.display_name = val('np-name');
        payload.authorization_url = val('np-auth');
        payload.token_url = val('np-token');
        payload.user_info_url = val('np-userinfo');
        payload.issuer = val('np-issuer');
        if (!payload.alias || !payload.authorization_url || !payload.token_url) {
            err.textContent = 'Alias, Authorization URL and Token URL are required for generic OIDC';
            return;
        }
    }

    err.textContent = 'Creating...';
    apiPost('/api/v1/identity/providers', payload).then(d => {
        if (d.error) { err.textContent = d.error; return; }
        alert('Provider added.\n\nRegister this redirect URI with the provider:\n' +
              window.location.origin + d.redirect_uri);
        loadIdentityStats();
        renderIdentityProviders();
    }).catch(e => { err.textContent = e.message; });
}

function toggleIdentityProvider(alias, enabled) {
    apiPut('/api/v1/identity/providers/' + encodeURIComponent(alias), { enabled: enabled }).then(d => {
        if (d.error) { alert(d.error); return; }
        renderIdentityProviders();
    }).catch(e => alert(e.message));
}

function deleteIdentityProvider(alias) {
    if (!confirm('Delete identity provider "' + alias + '"?\n\nUsers who sign in through it will lose access.')) return;
    apiDelete('/api/v1/identity/providers/' + encodeURIComponent(alias)).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentityProviders();
    }).catch(e => alert(e.message));
}

// --- Sessions tab ---
function renderIdentitySessions() {
    const body = document.getElementById('id-body');
    if (!body) return;
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading sessions...</p></div>';

    apiGet('/api/v1/identity/sessions').then(d => {
        const sessions = d.sessions || [];
        const rows = sessions.length === 0
            ? '<tr><td colspan="5" style="color:var(--muted)">No active sessions</td></tr>'
            : sessions.map(s => {
                const clients = s.clients ? Object.keys(s.clients).map(k => s.clients[k]).join(', ') : '';
                return '<tr><td><strong>' + esc(s.username) + '</strong></td>' +
                    '<td>' + esc(s.ipAddress) + '</td>' +
                    '<td>' + esc(clients) + '</td>' +
                    '<td>' + new Date(s.lastAccess).toLocaleString() + '</td>' +
                    '<td><button class="btn btn-danger" onclick="revokeIdentitySession(\'' + jsq(s.id) + '\')">Revoke</button></td></tr>';
            }).join('');

        body.innerHTML = `
            <div class="card" style="margin-bottom:1rem;display:flex;justify-content:space-between;align-items:center">
                <span style="font-size:0.85rem;color:var(--muted)">${sessions.length} active session(s) across all clients</span>
                <button class="btn" style="background:#e2e8f0" onclick="renderIdentitySessions()">Refresh</button>
            </div>
            <div class="card">
                <table><thead><tr><th>User</th><th>IP Address</th><th>Clients</th><th>Last Access</th><th style="width:1%">Actions</th></tr></thead>
                <tbody>${rows}</tbody></table>
            </div>
        `;
    }).catch(e => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load sessions: ' + esc(e.message) + '</p></div>';
    });
}

function revokeIdentitySession(sid) {
    if (!confirm('Revoke this session?')) return;
    apiDelete('/api/v1/identity/sessions/' + encodeURIComponent(sid)).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentitySessions();
    }).catch(e => alert(e.message));
}

// --- Clients tab (read-only) ---
function renderIdentityClients() {
    const body = document.getElementById('id-body');
    if (!body) return;
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading clients...</p></div>';

    apiGet('/api/v1/identity/clients').then(d => {
        const clients = (d.clients || []).filter(c => c.clientId && c.clientId.indexOf('realm-management') !== 0);
        const rows = clients.map(c =>
            '<tr><td><strong>' + esc(c.clientId) + '</strong></td>' +
            '<td>' + esc(c.name) + '</td>' +
            '<td>' + (c.publicClient ? 'Public' : 'Confidential') + '</td>' +
            '<td>' + (c.enabled ? '<span class="badge badge-active">Enabled</span>' : '<span class="badge badge-disabled">Disabled</span>') + '</td></tr>'
        ).join('');
        body.innerHTML = `
            <div class="card">
                <p style="font-size:0.78rem;color:var(--muted);margin-bottom:0.75rem">OIDC clients registered in the <strong>clm</strong> realm. Read-only.</p>
                <table><thead><tr><th>Client ID</th><th>Name</th><th>Type</th><th>Status</th></tr></thead><tbody>${rows}</tbody></table>
            </div>
        `;
    }).catch(e => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load clients: ' + esc(e.message) + '</p></div>';
    });
}


// ============================================================
// Organization & Tenants — merged hub
// Consolidates what were separate Organization and Tenants pages, and adds
// Subscription and Billing. Tenants/Subscription/Billing are restricted to
// platform admins; Organization is visible to everyone.
// ============================================================

let orgTab = 'organization';

const ORG_TABS = [
    ['organization', 'Organization', false],
    ['tenants', 'Tenants', true],
    ['subscription', 'Subscription', true],
    ['billing', 'Billing', true]
];

function loadOrgTenants(initialTab) {
    document.getElementById('page-title').textContent = 'Organization & Tenants';
    const el = document.getElementById('content-area');

    const visible = ORG_TABS.filter(t => !t[2] || isPlatformAdmin());
    if (initialTab && visible.some(t => t[0] === initialTab)) {
        orgTab = initialTab;
    } else if (!visible.some(t => t[0] === orgTab)) {
        orgTab = visible[0][0];
    }

    const tabs = visible.map(t =>
        '<button class="btn" id="orgtab-' + t[0] + '" onclick="switchOrgTab(\'' + t[0] + '\')" ' +
        'style="border-radius:0;background:none;color:var(--muted);border-bottom:2px solid transparent;padding:0.7rem 1.1rem">' +
        t[1] + '</button>'
    ).join('');

    el.innerHTML = `
        <div class="card" style="padding:0;margin-bottom:1rem">
            <div style="display:flex;flex-wrap:wrap;border-bottom:1px solid var(--border)">${tabs}</div>
        </div>
        <div id="org-body"><div class="card"><p style="color:var(--muted)">Loading...</p></div></div>
    `;
    switchOrgTab(orgTab);
}

function switchOrgTab(tab) {
    orgTab = tab;
    ORG_TABS.forEach(t => {
        const b = document.getElementById('orgtab-' + t[0]);
        if (!b) return;
        const on = t[0] === tab;
        b.style.color = on ? 'var(--primary)' : 'var(--muted)';
        b.style.background = on ? '#eff6ff' : 'none';
        b.style.fontWeight = on ? '700' : '600';
        b.style.borderBottom = on ? '2px solid var(--primary)' : '2px solid transparent';
    });

    if (tab === 'organization') renderOrgProfile();
    else if (tab === 'tenants') renderTenantsTab();
    else if (tab === 'subscription') renderSubscriptionTab();
    else if (tab === 'billing') renderBillingTab();
}

// panelEl returns the sub-tab panel when a hub page is showing, otherwise the
// main content area. This lets the original page loaders be reused inside the
// merged hubs without duplicating them.
function panelEl() {
    return document.getElementById('org-body') || document.getElementById('content-area');
}

function renderOrgProfile() {
    loadOrganization();
}

function renderTenantsTab() {
    if (!isPlatformAdmin()) {
        panelEl().innerHTML = '<div class="card"><p style="color:var(--muted)">' +
            'Tenant management requires the admin or super_admin role.</p></div>';
        return;
    }
    loadTenants();
}

// PLAN_LIMITS mirrors planLimits() in internal/saas/tenant/service.go. Kept in
// sync deliberately: the server rewrites a tenant's limits from its plan on
// every update, so showing anything else here would mislead.
const PLAN_LIMITS = {
    free:       { certs: 100,    gateways: 2,   users: 5 },
    pro:        { certs: 10000,  gateways: 20,  users: 50 },
    enterprise: { certs: 100000, gateways: 100, users: 500 }
};

function renderSubscriptionTab() {
    const body = panelEl();
    if (!isPlatformAdmin()) {
        body.innerHTML = '<div class="card"><p style="color:var(--muted)">Subscription requires the admin or super_admin role.</p></div>';
        return;
    }
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading subscription...</p></div>';

    Promise.all([
        apiGet('/api/v1/tenants'),
        tenantApiGet('/api/v1/satellites'),
        apiGet('/api/v1/identity/status').catch(() => null)
    ]).then(([td, sd, idst]) => {
        const tenants = td.tenants || [];
        const gateways = (sd.satellites || []).length;
        const users = (idst && idst.users) || 0;

        const rows = tenants.map(t => {
            const lim = PLAN_LIMITS[t.plan] || { certs: t.max_certs, gateways: t.max_satellites, users: t.max_users };
            return '<tr>' +
                '<td><strong>' + esc(t.name) + '</strong></td>' +
                '<td><span class="badge ' + (t.plan === 'enterprise' ? 'badge-active' : 'badge-warning') + '">' + esc(t.plan) + '</span></td>' +
                '<td>' + esc(t.status) + '</td>' +
                '<td>' + usageCell(gateways, lim.gateways) + '</td>' +
                '<td>' + usageCell(users, lim.users) + '</td>' +
                '<td>0 / ' + lim.certs.toLocaleString() + '</td>' +
                '<td>' + planSelect(t) + '</td>' +
                '</tr>';
        }).join('');

        body.innerHTML = `
            <div class="card" style="border-left:3px solid var(--primary)">
                <h3 style="margin-bottom:0.4rem;font-size:0.95rem">Plan Entitlements</h3>
                <p style="font-size:0.8rem;color:var(--muted)">
                    Changing a plan rewrites the tenant's limits from the tier definition on the server.
                    Certificate counts are held by each gateway, so they read 0 here until gateway reporting is wired up.
                </p>
            </div>
            <div class="card">
                <table>
                    <thead><tr><th>Tenant</th><th>Plan</th><th>Status</th><th>Gateways</th><th>Users</th><th>Certificates</th><th>Change plan</th></tr></thead>
                    <tbody>${rows || '<tr><td colspan="7" style="color:var(--muted)">No tenants</td></tr>'}</tbody>
                </table>
            </div>
            <div class="card">
                <h4 style="font-size:0.85rem;margin-bottom:0.5rem">Tier definitions</h4>
                <table>
                    <thead><tr><th>Plan</th><th>Max certificates</th><th>Max gateways</th><th>Max users</th></tr></thead>
                    <tbody>
                        ${Object.keys(PLAN_LIMITS).map(p => '<tr><td>' + p + '</td><td>' +
                            PLAN_LIMITS[p].certs.toLocaleString() + '</td><td>' +
                            PLAN_LIMITS[p].gateways + '</td><td>' + PLAN_LIMITS[p].users + '</td></tr>').join('')}
                    </tbody>
                </table>
            </div>
        `;
    }).catch(err => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load subscription: ' + esc(err.message) + '</p></div>';
    });
}

// usageCell shows consumption against an entitlement, flagging when it is close.
function usageCell(used, limit) {
    if (!limit) return String(used);
    const pct = Math.round((used / limit) * 100);
    const colour = pct >= 90 ? 'var(--danger)' : pct >= 70 ? 'var(--warning)' : 'inherit';
    return '<span style="color:' + colour + '">' + used + ' / ' + limit + '</span>';
}

function planSelect(t) {
    const opts = Object.keys(PLAN_LIMITS).map(p =>
        '<option value="' + p + '"' + (p === t.plan ? ' selected' : '') + '>' + p + '</option>').join('');
    return '<select onchange="changeTenantPlan(\'' + jsq(t.id) + '\', this.value)" ' +
        'style="padding:0.3rem;border:1px solid var(--border);border-radius:0.25rem;font-size:0.78rem">' +
        opts + '</select>';
}

// MARKETPLACES documents the three billing integrations. None are implemented,
// so this tab states that plainly rather than rendering a fake invoice history.
const MARKETPLACES = [
    ['aws',   'AWS Marketplace',   'ResolveCustomer + BatchMeterUsage'],
    ['gcp',   'GCP Marketplace',   'Procurement API entitlements'],
    ['azure', 'Azure Marketplace', 'SaaS Fulfillment API v2 + usageEvent']
];

function renderBillingTab() {
    const body = panelEl();
    if (!isPlatformAdmin()) {
        body.innerHTML = '<div class="card"><p style="color:var(--muted)">Billing requires the admin or super_admin role.</p></div>';
        return;
    }

    const mkRows = MARKETPLACES.map(m =>
        '<tr>' +
            '<td><strong>' + m[1] + '</strong></td>' +
            '<td style="font-size:0.75rem;color:var(--muted)">' + m[2] + '</td>' +
            '<td><span class="badge badge-warning">Not configured</span></td>' +
            '<td><button class="btn" style="background:#e2e8f0" onclick="marketplaceInfo(\'' + m[0] + '\')">Details</button></td>' +
        '</tr>'
    ).join('');

    body.innerHTML = `
        <div class="card" style="border-left:3px solid var(--warning)">
            <h3 style="margin-bottom:0.4rem;font-size:0.95rem">Billing is not yet active</h3>
            <p style="font-size:0.8rem;color:var(--muted)">
                No billing account or marketplace subscription is connected, so no charges are being
                calculated or metered. Tenants currently run on plan entitlements only, enforced by the
                server's tier definitions. This page will show invoices and metered usage once a
                billing mode is configured.
            </p>
        </div>

        <div class="card">
            <h4 style="font-size:0.85rem;margin-bottom:0.5rem">Billing mode</h4>
            <table>
                <thead><tr><th>Mode</th><th>How it works</th><th>Status</th></tr></thead>
                <tbody>
                    <tr>
                        <td><strong>Direct</strong></td>
                        <td style="font-size:0.78rem;color:var(--muted)">Plan-based flat rate, entitlements capped by the tier definition</td>
                        <td><span class="badge badge-active">Active (entitlements only, no charging)</span></td>
                    </tr>
                    <tr>
                        <td><strong>Cloud marketplace</strong></td>
                        <td style="font-size:0.78rem;color:var(--muted)">Customer pays through their existing cloud commitment; the marketplace is the source of truth for plan tier</td>
                        <td><span class="badge badge-warning">Not configured</span></td>
                    </tr>
                </tbody>
            </table>
        </div>

        <div class="card">
            <h4 style="font-size:0.85rem;margin-bottom:0.5rem">Marketplace integrations</h4>
            <table>
                <thead><tr><th>Marketplace</th><th>Binding + metering</th><th>Status</th><th></th></tr></thead>
                <tbody>${mkRows}</tbody>
            </table>
        </div>
    `;
}

function marketplaceInfo(which) {
    const detail = {
        aws: 'AWS Marketplace\n\n' +
             '1. Customer subscribes to the CLM listing.\n' +
             '2. AWS redirects to the CLM landing page with a registration token.\n' +
             '3. ResolveCustomer exchanges it for CustomerIdentifier + ProductCode.\n' +
             '4. Those are stored against the tenant and the plan is set from the offer.\n' +
             '5. Usage is reported with BatchMeterUsage.\n\n' +
             'Requires: a published SaaS listing and an IAM role for the metering API.',
        gcp: 'GCP Marketplace\n\n' +
             '1. Customer subscribes via Google Cloud Marketplace.\n' +
             '2. An Entitlement is created in the Procurement API and must be approved.\n' +
             '3. The entitlement is bound to the tenant and the plan set from the offer.\n' +
             '4. Usage is reported through Cloud Billing / Service Control.\n\n' +
             'Requires: a Marketplace-integrated project and a Pub/Sub topic for entitlement events.',
        azure: 'Azure Marketplace\n\n' +
             '1. Customer subscribes to the SaaS offer.\n' +
             '2. Azure posts a token to the CLM landing page.\n' +
             '3. SaaS Fulfillment API v2 resolves it to a subscription id, then activates it.\n' +
             '4. Usage is reported with Metering Service usageEvent on custom dimensions.\n\n' +
             'Requires: a published SaaS offer and an Entra app registration for fulfillment.'
    };
    alert((detail[which] || 'Unknown marketplace') +
          '\n\nNote: plan changes for marketplace-billed tenants must originate in the marketplace, ' +
          'otherwise metering bills against the wrong tier.');
}


// --- Identity Management: Groups tab ---
// Local Keycloak realm groups. A group carries one realm role and its members
// inherit it, so access can be managed by team rather than per user.

function renderIdentityGroups() {
    const body = document.getElementById('id-body');
    if (!body) return;
    body.innerHTML = '<div class="card"><p style="color:var(--muted)">Loading groups...</p></div>';

    Promise.all([
        apiGet('/api/v1/identity/groups'),
        apiGet('/api/v1/identity/roles')
    ]).then(([gd, rd]) => {
        identityRoleCache = (rd.roles || []).map(r => r.name).sort();
        const groups = gd.groups || [];

        const rows = groups.length === 0
            ? '<tr><td colspan="4" style="color:var(--muted)">No groups defined</td></tr>'
            : groups.map(g => identityGroupRow(g)).join('');

        const roleOpts = identityRoleCache.map(r =>
            '<option value="' + esc(r) + '">' + esc(r) + '</option>').join('');

        body.innerHTML = `
            <div class="card" style="margin-bottom:1rem">
                <h3 style="margin-bottom:0.4rem;font-size:0.95rem">Create Group</h3>
                <p style="font-size:0.78rem;color:var(--muted);margin-bottom:0.75rem">
                    Members inherit the group's realm role. A user's directly-assigned role still applies
                    in addition to anything inherited here.
                </p>
                <div style="display:flex;gap:0.5rem;align-items:flex-end;flex-wrap:wrap">
                    <div class="form-group" style="flex:1;min-width:180px"><label class="label">Group name</label>
                        <input id="ng-name" placeholder="e.g. platform-engineering"
                               style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem"></div>
                    <div class="form-group" style="min-width:160px"><label class="label">Inherited role</label>
                        <select id="ng-role" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem">
                            <option value="">(none)</option>${roleOpts}</select></div>
                    <button class="btn btn-primary" onclick="createIdentityGroup()">Add Group</button>
                </div>
                <div id="ng-error" style="color:var(--danger);font-size:0.8rem;margin-top:0.5rem"></div>
            </div>
            <div class="card">
                <table>
                    <thead><tr><th>Group</th><th>Inherited role</th><th>Members</th><th style="width:1%">Actions</th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
            <div id="id-group-panel"></div>
        `;
    }).catch(err => {
        body.innerHTML = '<div class="card"><p style="color:var(--danger)">Failed to load groups: ' + esc(err.message) + '</p></div>';
    });
}

function identityGroupRow(g) {
    const gid = jsq(g.id);
    const roleOpts = ['<option value="">(none)</option>'].concat(
        identityRoleCache.map(r =>
            '<option value="' + esc(r) + '"' + ((g.roles || []).indexOf(r) >= 0 ? ' selected' : '') + '>' + esc(r) + '</option>')
    ).join('');

    return '<tr>' +
        '<td><strong>' + esc(g.name) + '</strong></td>' +
        '<td><select onchange="setGroupRole(\'' + gid + '\', this.value)" ' +
            'style="padding:0.3rem;border:1px solid var(--border);border-radius:0.25rem;font-size:0.78rem">' +
            roleOpts + '</select></td>' +
        '<td>' + (g.members || 0) + '</td>' +
        '<td style="white-space:nowrap">' +
            '<button class="btn" style="background:#e2e8f0" onclick="showGroupMembers(\'' + gid + '\',\'' + jsq(g.name) + '\')">Members</button> ' +
            '<button class="btn btn-danger" onclick="deleteIdentityGroup(\'' + gid + '\',\'' + jsq(g.name) + '\')">Delete</button>' +
        '</td>' +
    '</tr>';
}

function createIdentityGroup() {
    const err = document.getElementById('ng-error');
    const name = document.getElementById('ng-name').value.trim();
    const role = document.getElementById('ng-role').value;
    if (!name) { err.textContent = 'Group name is required'; return; }

    err.textContent = 'Creating...';
    apiPost('/api/v1/identity/groups', { name: name, role: role }).then(d => {
        if (d.error) { err.textContent = d.error; return; }
        loadIdentityStats();
        renderIdentityGroups();
    }).catch(e => { err.textContent = e.message; });
}

function setGroupRole(gid, role) {
    apiPost('/api/v1/identity/groups/' + encodeURIComponent(gid) + '/role', { role: role }).then(d => {
        if (d.error) { alert(d.error); return; }
        renderIdentityGroups();
    }).catch(e => alert(e.message));
}

function deleteIdentityGroup(gid, name) {
    if (!confirm('Delete group "' + name + '"?\n\nMembers are not deleted, but they lose any role this group granted them.')) return;
    apiDelete('/api/v1/identity/groups/' + encodeURIComponent(gid)).then(d => {
        if (d.error) { alert(d.error); return; }
        loadIdentityStats();
        renderIdentityGroups();
    }).catch(e => alert(e.message));
}

// showGroupMembers lists members and offers the realm's users for assignment.
function showGroupMembers(gid, name) {
    const panel = document.getElementById('id-group-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="card" style="margin-top:1rem"><p style="color:var(--muted)">Loading members...</p></div>';

    Promise.all([
        apiGet('/api/v1/identity/groups/' + encodeURIComponent(gid) + '/members'),
        apiGet('/api/v1/identity/users')
    ]).then(([md, ud]) => {
        const members = md.members || [];
        const all = ud.users || [];
        const memberIds = members.map(m => m.id);
        const candidates = all.filter(u => memberIds.indexOf(u.id) < 0);

        const memberRows = members.length === 0
            ? '<tr><td colspan="3" style="color:var(--muted)">No members</td></tr>'
            : members.map(m =>
                '<tr><td>' + esc(m.username) + '</td><td>' + esc(m.email) + '</td>' +
                '<td><button class="btn btn-danger" onclick="removeFromGroup(\'' + jsq(m.id) + '\',\'' +
                    jsq(gid) + '\',\'' + jsq(name) + '\')">Remove</button></td></tr>').join('');

        const candidateOpts = candidates.map(u =>
            '<option value="' + esc(u.id) + '">' + esc(u.username) + '</option>').join('');

        panel.innerHTML = `
            <div class="card" style="margin-top:1rem;border-left:3px solid var(--primary)">
                <h3 style="margin-bottom:0.75rem;font-size:0.95rem">Members of ${esc(name)}</h3>
                <div style="display:flex;gap:0.5rem;align-items:flex-end;margin-bottom:1rem;flex-wrap:wrap">
                    <div class="form-group" style="flex:1;min-width:200px"><label class="label">Add a user</label>
                        <select id="gm-user" style="width:100%;padding:0.45rem;border:1px solid var(--border);border-radius:0.375rem">
                            ${candidateOpts || '<option value="">(all users are already members)</option>'}</select></div>
                    <button class="btn btn-primary" onclick="addToGroup('${jsq(gid)}','${jsq(name)}')">Add to Group</button>
                    <button class="btn" style="background:#e2e8f0" onclick="document.getElementById('id-group-panel').innerHTML=''">Close</button>
                </div>
                <table>
                    <thead><tr><th>Username</th><th>Email</th><th style="width:1%">Action</th></tr></thead>
                    <tbody>${memberRows}</tbody>
                </table>
            </div>
        `;
    }).catch(e => {
        panel.innerHTML = '<div class="card" style="margin-top:1rem"><p style="color:var(--danger)">' + esc(e.message) + '</p></div>';
    });
}

function addToGroup(gid, name) {
    const sel = document.getElementById('gm-user');
    const uid = sel ? sel.value : '';
    if (!uid) { alert('No user selected.'); return; }
    apiPut('/api/v1/identity/users/' + encodeURIComponent(uid) + '/groups/' + encodeURIComponent(gid), {})
        .then(d => {
            if (d.error) { alert(d.error); return; }
            showGroupMembers(gid, name);
            renderIdentityGroups();
        }).catch(e => alert(e.message));
}

function removeFromGroup(uid, gid, name) {
    apiDelete('/api/v1/identity/users/' + encodeURIComponent(uid) + '/groups/' + encodeURIComponent(gid))
        .then(d => {
            if (d.error) { alert(d.error); return; }
            showGroupMembers(gid, name);
            renderIdentityGroups();
        }).catch(e => alert(e.message));
}

// ==============================================================================
// Enterprise Multi-Level Approvals & Governance (ARC-004)
// ==============================================================================
function loadApprovals() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--primary)">
            <h3 style="margin-bottom:0.25rem">Multi-Level Approval Workflow & Segregation of Duties (ARC-004)</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                All production wildcard orders, high-value commercial certificates, and elevated permissions require multi-tier authorization.
                Requesters are strictly prohibited from approving their own lifecycle actions.
            </p>
        </div>
        <div id="approvals-container"><p style="color:var(--muted)">Loading pending approvals...</p></div>
    `;

    apiGet('/api/v1/approvals/pending').then(data => {
        const container = document.getElementById('approvals-container');
        const list = data.approvals || [];
        if (!list.length) {
            container.innerHTML = '<div class="card"><p style="color:var(--muted)">No approval requests pending your review.</p></div>';
            return;
        }

        let rows = list.map(a => {
            let badgeClass = 'badge-active';
            if (a.level === 'L2') badgeClass = 'badge-warning';
            if (a.level === 'L3') badgeClass = 'badge-disabled';

            return `
                <tr>
                    <td><strong>${esc(a.id)}</strong></td>
                    <td><span class="badge ${badgeClass}" style="font-weight:700">${esc(a.level)}</span></td>
                    <td>${esc(a.request_type)}</td>
                    <td>${esc(a.requested_by_email || a.requested_by)}</td>
                    <td>${esc(a.justification)}</td>
                    <td>$${(a.estimated_cost || 0).toFixed(2)}</td>
                    <td style="font-size:0.75rem">${esc(a.comment || 'Awaiting review')}</td>
                    <td>
                        <button class="btn btn-primary" onclick="doApprove('${esc(a.id)}')">Approve</button>
                        <button class="btn btn-danger" onclick="doReject('${esc(a.id)}')">Reject</button>
                    </td>
                </tr>
            `;
        }).join('');

        container.innerHTML = `
            <div class="card">
                <table>
                    <thead>
                        <tr>
                            <th>Request ID</th>
                            <th>Tier Level</th>
                            <th>Type</th>
                            <th>Requester</th>
                            <th>Justification</th>
                            <th>Cost</th>
                            <th>Progress / Comment</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
        `;
    }).catch(e => {
        document.getElementById('approvals-container').innerHTML = `<div class="card"><p style="color:var(--danger)">${esc(e.message)}</p></div>`;
    });
}

function doApprove(id) {
    apiPost(`/api/v1/approvals/${encodeURIComponent(id)}/approve`, { comment: "Approved via Admin Portal" })
        .then(res => {
            if (res.error) {
                alert("Approval Error: " + res.error);
            } else {
                alert("Decision recorded: " + (res.message || "Approved"));
                loadApprovals();
            }
        }).catch(e => alert(e.message));
}

function doReject(id) {
    const reason = prompt("Enter rejection reason:");
    if (!reason) return;
    apiPost(`/api/v1/approvals/${encodeURIComponent(id)}/reject`, { comment: reason })
        .then(res => {
            alert("Request rejected.");
            loadApprovals();
        }).catch(e => alert(e.message));
}

// ==============================================================================
// Enterprise Customer / Tenant Onboarding (Hybrid Managed Cloud Architecture)
// ==============================================================================
function loadCustomerOnboarding() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--primary)">
            <h3 style="margin-bottom:0.25rem">Enterprise Customer Onboarding (Hybrid Managed VPC)</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                Onboard a new enterprise tenant into dedicated AWS VPC infrastructure. Preflights cross-account STS trust,
                enforces strict quotas, provisions initial tenant security leads, and generates primary gateway pairing tokens.
            </p>
        </div>

        <div style="display:grid;grid-template-columns:1fr 1fr;gap:1.5rem">
            <div class="card">
                <h4 style="margin-bottom:1rem;color:var(--text)">1. Customer Cloud Specification</h4>
                <div class="form-group">
                    <label>Organization Slug (lowercase)</label>
                    <input type="text" id="ob-slug" value="m2p-hyc-uat" placeholder="e.g. m2p-hyc-uat">
                </div>
                <div class="form-group">
                    <label>Organization Legal / Display Name</label>
                    <input type="text" id="ob-name" value="M2P Hybrid Cloud UAT" placeholder="e.g. M2P Financial Technologies">
                </div>
                <div class="form-group">
                    <label>Lead Security / Cloud Contact Email</label>
                    <input type="email" id="ob-email" value="cloud-sec@m2pfintech.com" placeholder="cloud@customer.io">
                </div>
                <div class="form-group">
                    <label>Customer AWS Account ID (12 digits)</label>
                    <input type="text" id="ob-account" value="279078306945" placeholder="e.g. 279078306945">
                </div>
                <div class="form-group">
                    <label>Primary Cloud Region</label>
                    <input type="text" id="ob-region" value="ap-south-1" placeholder="e.g. ap-south-1">
                </div>
                <div class="form-group">
                    <label>Customer VPC ID</label>
                    <input type="text" id="ob-vpc" value="vpc-00ec0a6fc7686398c" placeholder="e.g. vpc-00ec0a6fc7686398c">
                </div>
                <div class="form-group">
                    <label>Cross-Account AssumeRole ARN</label>
                    <input type="text" id="ob-role" value="arn:aws:iam::279078306945:role/CrewpeOrchestratorRole">
                </div>
                <div class="form-group">
                    <label>STS External ID</label>
                    <input type="text" id="ob-extid" value="db95ac738a746230ddda613271903536">
                </div>
                <div class="form-group">
                    <label>Plan Tier</label>
                    <select id="ob-plan" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                        <option value="enterprise" selected>Enterprise (10k certs, 50 gateways)</option>
                        <option value="fintech-sovereign">Fintech Sovereign (Unlimited, Dedicated HSM)</option>
                        <option value="growth">Growth (100 certs, 5 gateways)</option>
                        <option value="starter">Starter (5 certs, 1 gateway)</option>
                    </select>
                </div>
                <div style="margin-top:1.25rem;display:flex;gap:0.75rem">
                    <button class="btn" style="background:#475569;color:#fff" onclick="runPreflight()">Verify Preflight Contract</button>
                    <button class="btn btn-primary" onclick="runBootstrap()">Provision Customer</button>
                </div>
            </div>

            <div>
                <div class="card" id="preflight-results-card">
                    <h4 style="margin-bottom:0.75rem">2. Preflight Verification Status</h4>
                    <p style="color:var(--muted);font-size:0.85rem">Click "Verify Preflight Contract" to validate parameters against corporate security rules.</p>
                </div>
                <div class="card hidden" id="bootstrap-results-card" style="margin-top:1rem;border-color:var(--success)">
                    <h4 style="margin-bottom:0.75rem;color:var(--success)">3. Customer Provisioning Dossier</h4>
                    <div id="bootstrap-details"></div>
                </div>
            </div>
        </div>
    `;
}

function getOnboardingPayload() {
    return {
        slug: (document.getElementById('ob-slug').value || '').trim(),
        name: (document.getElementById('ob-name').value || '').trim(),
        contact_email: (document.getElementById('ob-email').value || '').trim(),
        aws_account_id: (document.getElementById('ob-account').value || '').trim(),
        region: (document.getElementById('ob-region').value || '').trim(),
        vpc_id: (document.getElementById('ob-vpc').value || '').trim(),
        cross_account_role_arn: (document.getElementById('ob-role').value || '').trim(),
        external_id: (document.getElementById('ob-extid').value || '').trim(),
        plan: document.getElementById('ob-plan').value
    };
}

function runPreflight() {
    const payload = getOnboardingPayload();
    const card = document.getElementById('preflight-results-card');
    card.innerHTML = '<p style="color:var(--muted)">Evaluating preflight rules...</p>';

    apiPost('/api/v1/onboarding/preflight', payload).then(res => {
        let checks = (res.checks || []).map(c => {
            const icon = c.passed ? '<span style="color:var(--success)">&#10004;</span>' : '<span style="color:var(--danger)">&#10008;</span>';
            return `<li style="padding:0.35rem 0;border-bottom:1px solid #f1f5f9;display:flex;justify-content:space-between">
                <span>${icon} <strong>${esc(c.name)}</strong></span>
                <span style="font-size:0.75rem;color:var(--muted)">${esc(c.message)}</span>
            </li>`;
        }).join('');

        const banner = res.valid 
            ? '<div style="background:#d1fae5;color:#065f46;padding:0.5rem;border-radius:0.375rem;margin-bottom:0.75rem;font-weight:600">All Enterprise Preflight Checks Passed! Ready to Bootstrap.</div>'
            : '<div style="background:#fee2e2;color:#991b1b;padding:0.5rem;border-radius:0.375rem;margin-bottom:0.75rem;font-weight:600">Preflight Validation Failed. Review errors below.</div>';

        card.innerHTML = `
            <h4 style="margin-bottom:0.75rem">2. Preflight Verification Status</h4>
            ${banner}
            <ul style="list-style:none;font-size:0.82rem">${checks}</ul>
        `;
    }).catch(e => {
        card.innerHTML = `<p style="color:var(--danger)">Preflight evaluation failed: ${esc(e.message)}</p>`;
    });
}

function runBootstrap() {
    const payload = getOnboardingPayload();
    const card = document.getElementById('bootstrap-results-card');
    const details = document.getElementById('bootstrap-details');
    card.classList.remove('hidden');
    details.innerHTML = '<p style="color:var(--muted)">Provisioning tenant, security leads, and gateway pairing tokens...</p>';

    apiPost('/api/v1/onboarding/bootstrap', payload).then(res => {
        if (res.error) {
            details.innerHTML = `<p style="color:var(--danger)">Bootstrap failed: ${esc(res.error)}</p>`;
            return;
        }

        details.innerHTML = `
            <div style="font-size:0.85rem">
                <p><strong>Tenant ID:</strong> <code>${esc(res.tenant_id)}</code></p>
                <p><strong>Slug:</strong> ${esc(res.slug)} | <strong>Plan:</strong> ${esc(res.plan)}</p>
                <p><strong>Initial Admin Email:</strong> ${esc(res.admin_email)}</p>
                <p><strong>Temporary Secret:</strong> <code style="background:#fef3c7;padding:0.2rem 0.4rem;border-radius:0.25rem">${esc(res.temporary_secret)}</code></p>
                <div style="margin-top:1rem">
                    <p style="font-weight:600">Primary Customer Gateway Bootstrap Command:</p>
                    <pre style="background:#0f172a;color:#38bdf8;padding:0.75rem;border-radius:0.5rem;font-size:0.75rem;overflow-x:auto;margin-top:0.35rem">${esc(res.primary_gateway.install_script)}</pre>
                </div>
            </div>
        `;
    }).catch(e => {
        details.innerHTML = `<p style="color:var(--danger)">Bootstrap error: ${esc(e.message)}</p>`;
    });
}


// ==============================================================================
// Enterprise Directory Group Mappings (AWS IAM Identity Center / SAML / OIDC)
// ==============================================================================
function loadDirectoryMappings() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--primary)">
            <h3 style="margin-bottom:0.25rem">Enterprise Directory Group-to-Role Mappings</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                Map incoming enterprise directory groups (AWS IAM Identity Center permission sets, Okta groups, Azure AD groups, Keycloak roles)
                directly to internal CLM tenant roles with priority-based conflict resolution and Just-In-Time role synchronization.
            </p>
        </div>

        <div style="display:grid;grid-template-columns:2fr 1fr;gap:1.5rem">
            <div class="card">
                <h4 style="margin-bottom:1rem">Active Directory Group Rules</h4>
                <div id="mappings-table-container"><p style="color:var(--muted)">Loading directory group rules...</p></div>
            </div>

            <div class="card">
                <h4 style="margin-bottom:1rem">Add Group Mapping Rule</h4>
                <div class="form-group">
                    <label>Enterprise Group Name / Claim</label>
                    <input type="text" id="map-group-name" placeholder="e.g. AWSReservedSSO_AdministratorAccess">
                </div>
                <div class="form-group">
                    <label>Target CLM Role</label>
                    <select id="map-target-role" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                        <option value="super_admin">super_admin (Full Platform Access)</option>
                        <option value="admin">admin (Tenant Administrator)</option>
                        <option value="operator">operator (SecOps / Certificates)</option>
                        <option value="auditor">auditor (Compliance & Audits)</option>
                        <option value="viewer">viewer (Read-Only)</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Priority (1-100, Higher wins)</label>
                    <input type="number" id="map-priority" value="80" min="1" max="100">
                </div>
                <div class="form-group">
                    <label>Description</label>
                    <input type="text" id="map-description" placeholder="SecOps or Cloud Admins">
                </div>
                <button class="btn btn-primary" style="width:100%;margin-top:0.5rem" onclick="saveDirectoryMapping()">Save Rule</button>
            </div>
        </div>
    `;

    renderDirectoryMappingsTable();
}

function renderDirectoryMappingsTable() {
    const tenantId = (currentUser && currentUser.tenant_id) || 'tenant-m2p-uat';
    apiGet(`/api/v1/tenants/${encodeURIComponent(tenantId)}/sso/mappings`).then(res => {
        const mappings = res.mappings || [];
        const container = document.getElementById('mappings-table-container');
        if (!container) return;

        if (mappings.length === 0) {
            container.innerHTML = '<p style="color:var(--muted)">No directory group rules configured yet. Add a rule using the form on the right.</p>';
            return;
        }

        let rows = mappings.map(m => `
            <tr>
                <td><code>${esc(m.group_name)}</code></td>
                <td><span class="badge badge-active">${esc(m.target_role)}</span></td>
                <td><strong>${esc(m.priority)}</strong></td>
                <td style="font-size:0.75rem;color:var(--muted)">${esc(m.description || '-')}</td>
                <td>
                    <button class="btn btn-danger" style="padding:0.25rem 0.5rem;font-size:0.75rem" onclick="deleteDirectoryMapping('${esc(m.id)}')">Delete</button>
                </td>
            </tr>
        `).join('');

        container.innerHTML = `
            <table>
                <thead>
                    <tr>
                        <th>Directory Group Name</th>
                        <th>Target Role</th>
                        <th>Priority</th>
                        <th>Description</th>
                        <th>Action</th>
                    </tr>
                </thead>
                <tbody>${rows}</tbody>
            </table>
        `;
    }).catch(e => {
        const container = document.getElementById('mappings-table-container');
        if (container) container.innerHTML = `<p style="color:var(--danger)">Failed to load rules: ${esc(e.message)}</p>`;
    });
}

function saveDirectoryMapping() {
    const tenantId = (currentUser && currentUser.tenant_id) || 'tenant-m2p-uat';
    const groupName = (document.getElementById('map-group-name').value || '').trim();
    const targetRole = document.getElementById('map-target-role').value;
    const priority = parseInt(document.getElementById('map-priority').value, 10) || 50;
    const description = (document.getElementById('map-description').value || '').trim();

    if (!groupName) {
        alert('Group Name is required.');
        return;
    }

    const payload = {
        group_name: groupName,
        target_role: targetRole,
        priority: priority,
        description: description
    };

    apiPost(`/api/v1/tenants/${encodeURIComponent(tenantId)}/sso/mappings`, payload).then(res => {
        alert('Directory group rule saved successfully!');
        document.getElementById('map-group-name').value = '';
        document.getElementById('map-description').value = '';
        renderDirectoryMappingsTable();
    }).catch(e => alert('Error saving rule: ' + e.message));
}

function deleteDirectoryMapping(mappingId) {
    if (!confirm('Are you sure you want to remove this directory mapping rule?')) return;
    const tenantId = (currentUser && currentUser.tenant_id) || 'tenant-m2p-uat';
    apiDelete(`/api/v1/tenants/${encodeURIComponent(tenantId)}/sso/mappings/${encodeURIComponent(mappingId)}`).then(() => {
        renderDirectoryMappingsTable();
    }).catch(e => alert('Error deleting rule: ' + e.message));
}


// ==============================================================================
// 1. Departments & Financial Cost Centers (SAP / Workday / FinOps)
// ==============================================================================
function loadDepartments() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--primary)">
            <h3 style="margin-bottom:0.25rem">Corporate Departments & Financial Cost Centers</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                Allocate commercial CA purchasing budgets, track month-to-date (MTD) expenditure, and enforce active certificate quotas across enterprise business units.
            </p>
        </div>

        <div style="display:grid;grid-template-columns:2fr 1fr;gap:1.5rem">
            <div class="card">
                <h4 style="margin-bottom:1rem">Active Business Units</h4>
                <div id="departments-table-container"><p style="color:var(--muted)">Loading departments...</p></div>
            </div>

            <div class="card">
                <h4 style="margin-bottom:1rem">Add Department / Cost Center</h4>
                <div class="form-group"><label>Department Name</label><input type="text" id="dept-name" placeholder="e.g. Core Banking & Settlements"></div>
                <div class="form-group"><label>Department Code</label><input type="text" id="dept-code" placeholder="DEPT-BANK-01"></div>
                <div class="form-group"><label>Financial Cost Center</label><input type="text" id="dept-cost-center" placeholder="CC-FIN-1049"></div>
                <div class="form-group"><label>Lead Email</label><input type="email" id="dept-lead" placeholder="banking-lead@m2pfintech.com"></div>
                <div class="form-group"><label>Monthly Budget Cap (USD)</label><input type="number" id="dept-budget" value="5000"></div>
                <div class="form-group"><label>Certificate Quota</label><input type="number" id="dept-quota" value="250"></div>
                <button class="btn btn-primary" style="width:100%;margin-top:0.5rem" onclick="saveDepartment()">Create Department</button>
            </div>
        </div>
    `;

    renderDepartments();
}

function renderDepartments() {
    apiGet('/api/v1/departments').then(res => {
        const list = res.departments || [];
        const container = document.getElementById('departments-table-container');
        if (!container) return;

        if (list.length === 0) {
            container.innerHTML = '<p style="color:var(--muted)">No departments configured. Add one on the right.</p>';
            return;
        }

        let rows = list.map(d => {
            const pct = Math.min(100, Math.round(((d.spent_usd || 0) / (d.budget_usd || 1)) * 100));
            const barColor = pct > 85 ? 'var(--danger)' : pct > 60 ? 'var(--warning)' : 'var(--success)';
            return `
                <tr>
                    <td><strong>${esc(d.name)}</strong><br><small style="color:var(--muted)">${esc(d.code)} | ${esc(d.cost_center)}</small></td>
                    <td>${esc(d.lead_email)}</td>
                    <td>$${(d.spent_usd || 0).toFixed(2)} / $${(d.budget_usd || 0).toFixed(2)}
                        <div style="background:#e2e8f0;height:4px;border-radius:2px;margin-top:4px"><div style="background:${barColor};height:4px;width:${pct}%"></div></div>
                    </td>
                    <td><span class="badge badge-active">${d.cert_count || 0} / ${d.cert_quota || 250} certs</span></td>
                </tr>
            `;
        }).join('');

        container.innerHTML = `
            <table>
                <thead>
                    <tr><th>Department / Code</th><th>Lead</th><th>Budget Utilization (MTD)</th><th>Quota</th></tr>
                </thead>
                <tbody>${rows}</tbody>
            </table>
        `;
    }).catch(e => {
        const container = document.getElementById('departments-table-container');
        if (container) container.innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    });
}

function saveDepartment() {
    const name = (document.getElementById('dept-name').value || '').trim();
    const code = (document.getElementById('dept-code').value || '').trim();
    const costCenter = (document.getElementById('dept-cost-center').value || '').trim();
    const lead = (document.getElementById('dept-lead').value || '').trim();
    const budget = parseFloat(document.getElementById('dept-budget').value) || 5000;
    const quota = parseInt(document.getElementById('dept-quota').value, 10) || 250;

    if (!name || !code) {
        alert('Department name and code are required.');
        return;
    }

    apiPost('/api/v1/departments', {
        name: name,
        code: code,
        cost_center: costCenter,
        lead_email: lead,
        budget_usd: budget,
        cert_quota: quota
    }).then(res => {
        alert('Department created successfully!');
        document.getElementById('dept-name').value = '';
        document.getElementById('dept-code').value = '';
        renderDepartments();
    }).catch(e => alert('Error: ' + e.message));
}

// ==============================================================================
// 2. ITSM Incidents & Service Requests (ServiceNow / ITIL)
// ==============================================================================
function loadITSMWarRoom() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--danger)">
            <h3 style="margin-bottom:0.25rem">ITSM Incidents & Service Request War Room</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                Real-time incident response and ITIL service requests for TLS certificate expirations, chain failures, and emergency rotation workflows.
            </p>
        </div>

        <div style="display:flex;gap:1rem;margin-bottom:1rem">
            <button class="btn btn-primary" onclick="showITSMTab('incidents')">Active Incidents</button>
            <button class="btn" style="background:#475569;color:#fff" onclick="showITSMTab('requests')">Service Catalog Requests</button>
        </div>

        <div id="itsm-content-container"></div>
    `;

    showITSMTab('incidents');
}

function showITSMTab(tab) {
    const container = document.getElementById('itsm-content-container');
    if (!container) return;

    if (tab === 'incidents') {
        container.innerHTML = `
            <div class="card">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                    <h4>Security & Availability Incidents (SEV-1 to SEV-4)</h4>
                    <button class="btn btn-danger" onclick="promptRaiseIncident()">+ Declare Emergency Incident</button>
                </div>
                <div id="itsm-incidents-table"><p style="color:var(--muted)">Loading incidents...</p></div>
            </div>
        `;
        renderIncidents();
    } else {
        container.innerHTML = `
            <div class="card">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">
                    <h4>Self-Service Certificate Catalog Requests</h4>
                    <button class="btn btn-primary" onclick="promptServiceRequest()">+ New Certificate Order Request</button>
                </div>
                <div id="itsm-requests-table"><p style="color:var(--muted)">Loading service requests...</p></div>
            </div>
        `;
        renderServiceRequests();
    }
}

function renderIncidents() {
    apiGet('/api/v1/itsm/incidents').then(res => {
        const list = res.incidents || [];
        const table = document.getElementById('itsm-incidents-table');
        if (!table) return;

        if (list.length === 0) {
            table.innerHTML = '<p style="color:var(--success)">No active incidents. Estate TLS health is normal.</p>';
            return;
        }

        let rows = list.map(inc => {
            const sevBadge = inc.severity === 'SEV1' ? 'badge-danger' : inc.severity === 'SEV2' ? 'badge-warning' : 'badge-active';
            const statusBadge = inc.status === 'resolved' ? 'badge-active' : 'badge-danger';
            return `
                <tr>
                    <td><code>${esc(inc.itsm_external_ref || inc.id)}</code></td>
                    <td><span class="badge ${sevBadge}">${esc(inc.severity)}</span></td>
                    <td><strong>${esc(inc.title)}</strong><br><small style="color:var(--muted)">Domain: ${esc(inc.affected_domain)}</small></td>
                    <td><span class="badge badge-warning">Blast Radius: ${inc.blast_radius_score}/100</span></td>
                    <td><span class="badge ${statusBadge}">${esc(inc.status)}</span></td>
                    <td>
                        ${inc.status !== 'resolved' ? `<button class="btn btn-primary" style="padding:0.25rem 0.5rem;font-size:0.75rem" onclick="resolveIncident('${esc(inc.id)}')">Resolve</button>` : '<small style="color:var(--muted)">Resolved</small>'}
                    </td>
                </tr>
            `;
        }).join('');

        table.innerHTML = `
            <table>
                <thead><tr><th>Ticket ID</th><th>Severity</th><th>Incident Summary</th><th>Blast Radius</th><th>Status</th><th>Action</th></tr></thead>
                <tbody>${rows}</tbody>
            </table>
        `;
    }).catch(e => {
        const table = document.getElementById('itsm-incidents-table');
        if (table) table.innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    });
}

function promptRaiseIncident() {
    const title = prompt('Enter Incident Summary:');
    if (!title) return;
    const domain = prompt('Enter Affected Domain Name:');
    if (!domain) return;
    const sev = prompt('Severity (SEV1, SEV2, SEV3, SEV4):', 'SEV1');

    apiPost('/api/v1/itsm/incidents', {
        title: title,
        affected_domain: domain,
        severity: sev.toUpperCase()
    }).then(() => {
        alert('Emergency incident created!');
        renderIncidents();
    }).catch(e => alert('Error: ' + e.message));
}

function resolveIncident(id) {
    const rootCause = prompt('Enter Root Cause Summary:', 'Expired token renewed');
    if (!rootCause) return;
    const remediation = prompt('Enter Remediation Actions Taken:', 'Atomic certificate rollout verified');

    apiPost(`/api/v1/itsm/incidents/${encodeURIComponent(id)}/resolve`, {
        root_cause: rootCause,
        remediation_steps: remediation
    }).then(() => {
        alert('Incident successfully resolved and closed.');
        renderIncidents();
    }).catch(e => alert('Error: ' + e.message));
}

function renderServiceRequests() {
    apiGet('/api/v1/itsm/requests').then(res => {
        const list = res.requests || [];
        const table = document.getElementById('itsm-requests-table');
        if (!table) return;

        if (list.length === 0) {
            table.innerHTML = '<p style="color:var(--muted)">No service catalog requests found.</p>';
            return;
        }

        let rows = list.map(r => `
            <tr>
                <td><code>${esc(r.itsm_ticket_ref || r.id)}</code></td>
                <td><strong>${esc(r.domain_name)}</strong></td>
                <td>${esc(r.department || 'General')}</td>
                <td><span class="badge badge-active">${esc(r.environment || 'prod')}</span></td>
                <td>${r.validity_months || 12} mos (${esc(r.cert_type || 'ov')})</td>
                <td><span class="badge badge-warning">${esc(r.status)}</span></td>
            </tr>
        `).join('');

        table.innerHTML = `
            <table>
                <thead><tr><th>Request Ref</th><th>Domain</th><th>Department</th><th>Env</th><th>Type</th><th>Status</th></tr></thead>
                <tbody>${rows}</tbody>
            </table>
        `;
    }).catch(e => {
        const table = document.getElementById('itsm-requests-table');
        if (table) table.innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    });
}

function promptServiceRequest() {
    const domain = prompt('Enter Requested Domain Name (e.g. pay.m2pfintech.com):');
    if (!domain) return;
    const dept = prompt('Department Name:', 'Payments & Gateway');

    apiPost('/api/v1/itsm/requests', {
        domain_name: domain,
        department: dept,
        environment: 'prod',
        cert_type: 'ov',
        validity_months: 12,
        requester_email: (currentUser && currentUser.email) || 'admin@m2pfintech.com'
    }).then(() => {
        alert('Service catalog order request submitted for approval!');
        renderServiceRequests();
    }).catch(e => alert('Error: ' + e.message));
}

// ==============================================================================
// 3. Cryptographic Risk Register (Deloitte / JPMorgan Chase / PQC)
// ==============================================================================
function loadRiskRegister() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--warning)">
            <h3 style="margin-bottom:0.25rem">Enterprise Cryptographic Risk Register</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                Continuous quantitative assessment of cryptographic vulnerability, Post-Quantum Cryptography (PQC) readiness, single-point-of-failure (SPOF) CA risks, and wildcard blast radiuses.
            </p>
        </div>

        <div class="stats">
            <div class="card stat"><div class="value" id="risk-score" style="color:var(--warning)">-</div><div class="label">Overall Risk Index (0-100)</div></div>
            <div class="card stat"><div class="value" id="risk-rating">-</div><div class="label">Exposure Level</div></div>
            <div class="card stat"><div class="value" id="risk-pqc">-</div><div class="label">PQC Transition State</div></div>
            <div class="card stat"><div class="value" id="risk-total" style="color:var(--danger)">-</div><div class="label">Identified Vulnerabilities</div></div>
        </div>

        <div class="card">
            <h4 style="margin-bottom:1rem">Prioritized Cryptographic Risk Findings</h4>
            <div id="risk-items-table"><p style="color:var(--muted)">Evaluating cryptographic estate...</p></div>
        </div>
    `;

    apiGet('/api/v1/risk/posture').then(res => {
        document.getElementById('risk-score').textContent = res.overall_risk_score !== undefined ? res.overall_risk_score : 28;
        document.getElementById('risk-rating').textContent = res.risk_rating || 'MODERATE';
        document.getElementById('risk-pqc').innerHTML = res.pqc_transition_ready
            ? '<span style="color:var(--success)">&#10004; Quantum-Safe</span>'
            : '<span style="color:var(--warning)">Vulnerable to Shor\'s Alg</span>';
        document.getElementById('risk-total').textContent = res.total_identified_risks || 2;

        const container = document.getElementById('risk-items-table');
        const items = res.items || [];
        if (items.length === 0) {
            container.innerHTML = '<p style="color:var(--success)">No critical cryptographic risks detected.</p>';
            return;
        }

        let rows = items.map(it => {
            const sevBadge = it.severity === 'CRITICAL' ? 'badge-danger' : 'badge-warning';
            return `
                <tr>
                    <td><code>${esc(it.id)}</code></td>
                    <td><span class="badge ${sevBadge}">${esc(it.severity)}</span></td>
                    <td><strong>${esc(it.affected)}</strong><br><small style="color:var(--muted)">${esc(it.description)}</small></td>
                    <td><span class="badge badge-warning">Score: ${it.score}</span></td>
                    <td style="font-size:0.75rem;color:var(--primary)">${esc(it.remediation)}</td>
                </tr>
            `;
        }).join('');

        container.innerHTML = `
            <table>
                <thead><tr><th>Risk ID</th><th>Severity</th><th>Vulnerability & Impact</th><th>Risk Score</th><th>Required Mitigation</th></tr></thead>
                <tbody>${rows}</tbody>
            </table>
        `;
    }).catch(e => {
        document.getElementById('risk-items-table').innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    });
}

// ==============================================================================
// 4. Enterprise Integrations & Outbound Webhooks (Stripe / Slack / ServiceNow)
// ==============================================================================
function loadIntegrations() {
    const el = document.getElementById('content-area');
    el.innerHTML = `
        <div class="card" style="margin-bottom:1.25rem;border-left:4px solid var(--primary)">
            <h3 style="margin-bottom:0.25rem">Enterprise Integrations & Webhook Subscriptions</h3>
            <p style="color:var(--muted);font-size:0.85rem">
                Connect CLM event streams to corporate webhooks, ServiceNow, Slack, Microsoft Teams, and SIEM forwarders with HMAC-SHA256 non-repudiation seals.
            </p>
        </div>

        <div style="display:grid;grid-template-columns:2fr 1fr;gap:1.5rem">
            <div class="card">
                <h4 style="margin-bottom:1rem">Configured Integrations & Event Hooks</h4>
                <div id="integrations-table-container"><p style="color:var(--muted)">Loading integrations...</p></div>
            </div>

            <div class="card">
                <h4 style="margin-bottom:1rem">Register Webhook / Connector</h4>
                <div class="form-group"><label>Integration Name</label><input type="text" id="integ-name" placeholder="e.g. Corporate Slack Ops Channel"></div>
                <div class="form-group">
                    <label>Integration Type</label>
                    <select id="integ-type" style="width:100%;padding:0.5rem;border:1px solid var(--border);border-radius:0.375rem">
                        <option value="slack">Slack Incoming Webhook</option>
                        <option value="teams">Microsoft Teams Webhook</option>
                        <option value="servicenow">ServiceNow ITSM Connector</option>
                        <option value="webhook">Generic HTTPS Webhook</option>
                        <option value="siem">SIEM / Splunk Event Stream</option>
                    </select>
                </div>
                <div class="form-group"><label>Endpoint URL</label><input type="url" id="integ-url" placeholder="https://hooks.slack.com/services/..."></div>
                <div class="form-group"><label>Subscribed Events</label>
                    <input type="text" id="integ-events" value="incident.raised, certificate.expired, approval.pending" placeholder="comma-separated events">
                </div>
                <button class="btn btn-primary" style="width:100%;margin-top:0.5rem" onclick="saveIntegration()">Register Integration</button>
            </div>
        </div>
    `;

    renderIntegrations();
}

function renderIntegrations() {
    apiGet('/api/v1/integrations').then(res => {
        const list = res.integrations || [];
        const container = document.getElementById('integrations-table-container');
        if (!container) return;

        if (list.length === 0) {
            container.innerHTML = '<p style="color:var(--muted)">No integrations configured. Add one on the right.</p>';
            return;
        }

        let rows = list.map(sub => `
            <tr>
                <td><strong>${esc(sub.name)}</strong></td>
                <td><span class="badge badge-active">${esc(sub.type)}</span></td>
                <td><code style="font-size:0.75rem">${esc(sub.endpoint_url)}</code></td>
                <td style="font-size:0.75rem">${(sub.events || []).join(', ')}</td>
                <td>
                    <button class="btn btn-danger" style="padding:0.25rem 0.5rem;font-size:0.75rem" onclick="deleteIntegration('${esc(sub.id)}')">Delete</button>
                </td>
            </tr>
        `).join('');

        container.innerHTML = `
            <table>
                <thead><tr><th>Name</th><th>Type</th><th>Endpoint</th><th>Events</th><th>Action</th></tr></thead>
                <tbody>${rows}</tbody>
            </table>
        `;
    }).catch(e => {
        const container = document.getElementById('integrations-table-container');
        if (container) container.innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    });
}

function saveIntegration() {
    const name = (document.getElementById('integ-name').value || '').trim();
    const type = document.getElementById('integ-type').value;
    const url = (document.getElementById('integ-url').value || '').trim();
    const eventsStr = (document.getElementById('integ-events').value || '').trim();

    if (!name || !url) {
        alert('Name and endpoint URL are required.');
        return;
    }

    const events = eventsStr.split(',').map(s => s.trim()).filter(Boolean);

    apiPost('/api/v1/integrations', {
        name: name,
        type: type,
        endpoint_url: url,
        events: events
    }).then(() => {
        alert('Integration registered successfully!');
        document.getElementById('integ-name').value = '';
        document.getElementById('integ-url').value = '';
        renderIntegrations();
    }).catch(e => alert('Error: ' + e.message));
}

function deleteIntegration(id) {
    if (!confirm('Remove this integration?')) return;
    apiDelete('/api/v1/integrations/' + encodeURIComponent(id)).then(() => {
        renderIntegrations();
    }).catch(e => alert('Error: ' + e.message));
}
