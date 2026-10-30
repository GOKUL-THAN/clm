const API = window.location.origin;
let token = localStorage.getItem('clm_token') || '';
let currentPage = 'dashboard';
let certData = [];
let satData = [];

var defaultProviders = [
    { id:'1', name:'Lets Encrypt', provider:'letsencrypt', environment:'production', active:true, apiKey:'', budget:0, spent:0 },
    { id:'2', name:'DigiCert Enterprise', provider:'digicert', environment:'production', active:true, apiKey:'dk_****...masked', budget:20000, spent:3200 },
    { id:'3', name:'GoDaddy', provider:'godaddy', environment:'production', active:false, apiKey:'', budget:5000, spent:0 },
    { id:'4', name:'Sectigo', provider:'sectigo', environment:'production', active:true, apiKey:'sc_****...masked', budget:10000, spent:1500 },
    { id:'5', name:'ZeroSSL', provider:'zerossl', environment:'staging', active:true, apiKey:'zs_****...masked', budget:0, spent:0 },
    { id:'6', name:'Internal CA (Vault PKI)', provider:'vault_pki', environment:'production', active:true, apiKey:'vault-token-configured', budget:0, spent:0 },
    { id:'7', name:'Namecheap', provider:'namecheap', environment:'production', active:true, apiKey:'nc_****...masked', budget:2000, spent:450 }
];

// Version check to force refresh of cached data
var PROVIDER_VERSION = '2';
var storedVersion = localStorage.getItem('clm_ca_version');
let caProviders;
if (storedVersion === PROVIDER_VERSION) {
    try { caProviders = JSON.parse(localStorage.getItem('clm_ca_providers')); } catch(e) { caProviders = null; }
}
if (!caProviders || !Array.isArray(caProviders)) { caProviders = defaultProviders; }

let policies = (function(){
    try { var p = JSON.parse(localStorage.getItem('clm_policies')); if(p && p.min_rsa_bits) return p; } catch(e){}
    return {
        min_rsa_bits: 2048, min_ecdsa_curve: 'P-256', allowed_sig_algos: 'SHA-256, SHA-384, SHA-512',
        allowed_cas_prod: 'DigiCert, Lets Encrypt, Sectigo, Internal CA, Namecheap',
        allowed_cas_nonprod: 'All + Self-Signed, ZeroSSL',
        self_signed_prod: false, max_validity_days: 365, auto_renew_days: 30,
        wildcard_approval: 'L2', verify_after_deploy: true, rollback_on_failure: true, backup_existing: true, backup_count: 5
    };
})();

function savePolicies() { localStorage.setItem('clm_policies', JSON.stringify(policies)); }
function saveProviders() { localStorage.setItem('clm_ca_providers', JSON.stringify(caProviders)); localStorage.setItem('clm_ca_version', PROVIDER_VERSION); }

var DISCOVERY_VERSION = '1';
var discoveryConfig = (function(){
    try {
        if (localStorage.getItem('clm_disc_version') === DISCOVERY_VERSION) {
            var d = JSON.parse(localStorage.getItem('clm_discovery'));
            if (d && d.domains) return d;
        }
    } catch(e){}
    return {
        domains: [
            { value: '*.m2pfintech.com', label: 'M2P main domain', enabled: true },
            { value: '*.internal.m2p.local', label: 'Internal services', enabled: true },
            { value: '*.api.m2pfintech.com', label: 'API subdomains', enabled: true }
        ],
        cidrs: [
            { value: '10.0.0.0/16', label: 'Production VPC', enabled: true },
            { value: '172.16.0.0/12', label: 'Staging VPC', enabled: true },
            { value: '192.168.1.0/24', label: 'Office network', enabled: false }
        ],
        schedule: '0 2 * * *',
        ports: [443, 8443, 636, 6443, 9093, 5671],
        concurrency: 100,
        scanOnDemand: true,
        lastScan: null,
        autoDiscovery: true
    };
})();
function saveDiscovery() { localStorage.setItem('clm_discovery', JSON.stringify(discoveryConfig)); localStorage.setItem('clm_disc_version', DISCOVERY_VERSION); }

var ENDPOINTS_VERSION = '2';
var managedEndpoints = (function(){
    try {
        if (localStorage.getItem('clm_ep_version') === ENDPOINTS_VERSION) {
            var d = JSON.parse(localStorage.getItem('clm_endpoints'));
            if (d && Array.isArray(d)) return d;
        }
    } catch(e){}
    return [
        { id:'ep1', name:'API Gateway (Prod)', host:'10.0.5.42', port:443, category:'app', type:'nginx', environment:'prod', certPath:'/etc/nginx/ssl/api.crt', keyPath:'/etc/nginx/ssl/api.key', service:'nginx', reloadCmd:'systemctl reload nginx', certId:'', status:'active', lastDeployed:'', tags:['api','prod'] },
        { id:'ep2', name:'gRPC Service (Prod)', host:'10.0.5.43', port:443, category:'app', type:'nginx', environment:'prod', certPath:'/etc/nginx/ssl/grpc.crt', keyPath:'/etc/nginx/ssl/grpc.key', service:'nginx', reloadCmd:'systemctl reload nginx', certId:'', status:'active', lastDeployed:'', tags:['grpc','prod'] },
        { id:'ep3', name:'PostgreSQL Primary', host:'10.0.3.10', port:5432, category:'data', type:'postgresql', environment:'prod', certPath:'/var/lib/postgresql/certs/server.crt', keyPath:'/var/lib/postgresql/certs/server.key', service:'postgresql', reloadCmd:'pg_reload_conf', certId:'', status:'active', lastDeployed:'', tags:['database','prod'] },
        { id:'ep4', name:'Redis Cluster Node 1', host:'10.0.4.20', port:6380, category:'data', type:'redis', environment:'prod', certPath:'/etc/redis/tls/redis.crt', keyPath:'/etc/redis/tls/redis.key', service:'redis-server', reloadCmd:'redis-cli CONFIG SET tls-cert-file /etc/redis/tls/redis.crt', certId:'', status:'active', lastDeployed:'', tags:['cache','prod'] },
        { id:'ep5', name:'Staging Web Server', host:'10.0.6.10', port:443, category:'app', type:'nginx', environment:'staging', certPath:'/etc/nginx/ssl/staging.crt', keyPath:'/etc/nginx/ssl/staging.key', service:'nginx', reloadCmd:'systemctl reload nginx', certId:'', status:'active', lastDeployed:'', tags:['web','staging'] },
        { id:'ep6', name:'AWS ALB (Prod)', host:'alb-prod-123.ap-south-1.elb.amazonaws.com', port:443, category:'cloud', type:'aws-alb', environment:'prod', certPath:'', keyPath:'', service:'', reloadCmd:'', certId:'', status:'active', lastDeployed:'', tags:['loadbalancer','aws','prod'], resourceId:'arn:aws:elasticloadbalancing:ap-south-1:123456789:loadbalancer/app/api-alb/abc123' }
    ];
})();
function saveEndpoints() { localStorage.setItem('clm_endpoints', JSON.stringify(managedEndpoints)); localStorage.setItem('clm_ep_version', ENDPOINTS_VERSION); }

function headers() { return { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' }; }

async function doLogin() {
    var email = document.getElementById('login-email').value;
    var password = document.getElementById('login-password').value;
    var errEl = document.getElementById('login-error');
    errEl.style.display = 'none';
    try {
        var res = await fetch(API + '/api/v1/auth/login', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({email, password}) });
        var data = await res.json();
        if (data.access_token) {
            token = data.access_token;
            localStorage.setItem('clm_token', token);
            document.getElementById('login-overlay').style.display = 'none';
            document.getElementById('user-email').textContent = email;
            loadAll();
        } else { errEl.textContent = data.error || 'Login failed'; errEl.style.display = 'block'; }
    } catch(e) { errEl.textContent = 'Connection error'; errEl.style.display = 'block'; }
}
function logout() { token = ''; localStorage.removeItem('clm_token'); location.reload(); }

async function fetchAPI(path) {
    try {
        var res = await fetch(API + path, { headers: headers() });
        if (res.status === 401) { logout(); return null; }
        return await res.json();
    } catch(e) { return null; }
}

async function loadAll() {
    var [certRes, satRes] = await Promise.all([fetchAPI('/api/v1/certificates'), fetchAPI('/api/v1/satellites')]);
    certData = (certRes && certRes.certificates) ? certRes.certificates : [];
    satData = (satRes && satRes.satellites) ? satRes.satellites : [];
    var ec = document.getElementById('expiry-count');
    if (ec) ec.textContent = getExpiringCerts().length || '';
    renderPage();
}

function getExpiringCerts() {
    var now = new Date(), d30 = new Date(now.getTime() + 30*24*60*60*1000);
    return certData.filter(function(c){return c.status==='active'&&c.not_after&&new Date(c.not_after)<=d30&&new Date(c.not_after)>now});
}
function daysUntil(d){if(!d)return 999;return Math.ceil((new Date(d)-new Date())/(1000*60*60*24));}
function statusBadge(s){var cls={active:'badge-active',pending:'badge-pending',expired:'badge-expired',revoked:'badge-revoked',online:'badge-online',offline:'badge-offline',registering:'badge-registering',renewing:'badge-pending',failed:'badge-expired'}[s]||'badge-revoked';return '<span class="badge '+cls+'">'+s+'</span>';}

function navigate(page) {
    currentPage = page;
    document.querySelectorAll('.nav-item').forEach(function(el){el.classList.remove('active')});
    var el = document.querySelector('[data-page="'+page+'"]');
    if(el) el.classList.add('active');
    renderPage();
}

function renderPage() {
    var titles = {dashboard:'Dashboard',certificates:'Certificates',endpoints:'Endpoints',satellites:'Satellites',providers:'CA Providers',policies:'Policy Configuration',discovery:'Discovery & Scanning',expiring:'Expiring Certificates (Next 30 Days)',approvals:'Approvals'};
    document.getElementById('page-title').textContent = titles[currentPage]||'Dashboard';
    var area = document.getElementById('content-area');
    try {
    switch(currentPage) {
        case 'dashboard': area.innerHTML = renderDashboard(); break;
        case 'certificates': area.innerHTML = renderCertificates(); break;
        case 'endpoints': area.innerHTML = renderEndpoints(); break;
        case 'satellites': area.innerHTML = renderSatellites(); break;
        case 'providers': area.innerHTML = renderProviders(); break;
        case 'policies': area.innerHTML = renderPolicies(); break;
        case 'discovery': area.innerHTML = renderDiscovery(); break;
        case 'expiring': area.innerHTML = renderExpiring(); break;
        case 'approvals': area.innerHTML = renderApprovals(); break;
    }
    } catch(e) { area.innerHTML = '<div style="padding:2rem;color:red"><h3>Page Error: '+currentPage+'</h3><pre>'+e.message+'\n'+e.stack+'</pre></div>'; }
}

// Dashboard
function renderDashboard() {
    var active=certData.filter(function(c){return c.status==='active'}).length, pending=certData.filter(function(c){return c.status==='pending'}).length, expiring=getExpiringCerts().length, satsOnline=satData.filter(function(s){return s.status==='online'}).length, satsTotal=satData.length;
    return '<div class="stats-grid"><div class="stat-card blue"><div class="label">Total Certificates</div><div class="value">'+certData.length+'</div><div class="sub">'+active+' active, '+pending+' pending</div></div><div class="stat-card green"><div class="label">Active</div><div class="value">'+active+'</div><div class="sub">Deployed and valid</div></div><div class="stat-card yellow"><div class="label">Expiring (30d)</div><div class="value">'+expiring+'</div><div class="sub">'+(expiring>0?'Needs attention':'All clear')+'</div></div><div class="stat-card purple"><div class="label">Satellites</div><div class="value">'+satsOnline+'/'+satsTotal+'</div><div class="sub">'+satsOnline+' online</div></div></div>'+
    '<div style="display:grid;grid-template-columns:1fr 1fr;gap:1.5rem"><div class="panel"><div class="panel-header"><h3>Certificate Status</h3></div><div class="panel-body">'+renderStatusChart()+'</div></div><div class="panel"><div class="panel-header"><h3>Satellite Status</h3></div><div class="panel-body">'+(satData.length?satData.map(renderSatCard).join(''):'<div class="empty-state"><div class="icon">📡</div><p>No satellites registered</p></div>')+'</div></div></div>'+
    '<div class="panel"><div class="panel-header"><h3>Expiring Soon</h3></div><div class="panel-body">'+renderExpiryTable(getExpiringCerts())+'</div></div>';
}
function renderStatusChart(){var statuses=['active','pending','expired','revoked','renewing','failed'],colors={active:'#10b981',pending:'#f59e0b',expired:'#ef4444',revoked:'#94a3b8',renewing:'#2563eb',failed:'#dc2626'};if(!certData.length)return '<div class="empty-state"><div class="icon">📜</div><p>No certificates yet</p></div>';var total=certData.length;return '<div style="display:flex;flex-direction:column;gap:0.5rem">'+statuses.map(function(s){var count=certData.filter(function(c){return c.status===s}).length;if(!count)return '';var pct=Math.round(count/total*100);return '<div style="display:flex;align-items:center;gap:0.75rem"><span style="min-width:70px;font-size:0.8rem;color:var(--text-muted);text-transform:capitalize">'+s+'</span><div style="flex:1;height:20px;background:#f1f5f9;border-radius:4px;overflow:hidden"><div style="width:'+pct+'%;height:100%;background:'+colors[s]+';border-radius:4px"></div></div><span style="min-width:40px;font-size:0.8rem;font-weight:600;text-align:right">'+count+'</span></div>';}).join('')+'</div>';}
function renderSatCard(s){var isOn=s.status==='online',bg=isOn?'var(--success-light)':'var(--danger-light)';return '<div class="satellite-card"><div class="sat-icon" style="background:'+bg+'">📡</div><div class="sat-info"><h4>'+s.name+'</h4><p>'+(s.connector_mode||'direct')+' | '+(s.cloud||'N/A')+' | '+(s.region||'N/A')+'</p></div><div class="sat-meta">'+statusBadge(s.status)+'<br><span style="font-size:0.7rem">'+(s.last_heartbeat?new Date(s.last_heartbeat).toLocaleString():'Never')+'</span></div></div>';}
function renderExpiryTable(certs){if(!certs.length)return '<div class="empty-state"><div class="icon">✅</div><p>No certificates expiring in the next 30 days</p></div>';return '<div class="table-wrap"><table><thead><tr><th>Domain</th><th>Type</th><th>Issuer</th><th>Expires</th><th>Days Left</th><th>Risk</th></tr></thead><tbody>'+certs.map(function(c){var days=daysUntil(c.not_after),color=days<=7?'var(--danger)':days<=14?'var(--warning)':'var(--text)',barColor=days<=7?'var(--danger)':days<=14?'var(--warning)':'var(--success)',pct=Math.max(0,Math.round((30-days)/30*100));return '<tr><td><strong>'+c.domain+'</strong></td><td>'+c.type+'</td><td>'+(c.issuer||'Unknown')+'</td><td>'+new Date(c.not_after).toLocaleDateString()+'</td><td style="color:'+color+';font-weight:600">'+days+'d</td><td><div class="expiry-bar"><div class="expiry-bar-fill" style="width:'+pct+'%;background:'+barColor+'"></div></div></td></tr>';}).join('')+'</tbody></table></div>';}

// Certificates page - with CA provider tab
function renderCertificates() {
    var activeProviders = caProviders.filter(function(p){return p.active});
    var caOptions = activeProviders.map(function(p){return '<option value="'+p.provider+'">'+p.name+'</option>'}).join('');
    return '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem"><div style="display:flex;gap:0.5rem"><button class="btn btn-outline btn-sm" onclick="filterCerts(\'all\')">All ('+certData.length+')</button><button class="btn btn-outline btn-sm" onclick="filterCerts(\'active\')">Active</button><button class="btn btn-outline btn-sm" onclick="filterCerts(\'pending\')">Pending</button><button class="btn btn-outline btn-sm" onclick="filterCerts(\'expired\')">Expired</button></div><button class="btn btn-primary btn-sm" onclick="showCreateCert()">+ Request Certificate</button></div>'+
    '<div class="panel"><div class="panel-body"><div class="table-wrap" id="cert-table-wrap">'+renderCertTable(certData)+'</div></div></div>'+
    '<div class="panel hidden" id="create-cert-panel"><div class="panel-header"><h3>Request New Certificate</h3></div><div class="panel-body">'+
    '<div style="display:flex;gap:0.5rem;margin-bottom:1rem"><button class="btn btn-sm btn-primary" id="tab-details" onclick="showCertTab(\'details\')">Certificate Details</button><button class="btn btn-sm btn-outline" id="tab-ca" onclick="showCertTab(\'ca\')">CA Provider & Purchase</button></div>'+
    '<div id="cert-tab-details"><div class="form-row"><div class="form-group"><label>Domain</label><input id="cert-domain" placeholder="api.example.com"></div><div class="form-group"><label>Type</label><select id="cert-type"><option value="DV">DV (Domain Validated)</option><option value="OV">OV (Organization Validated)</option><option value="EV">EV (Extended Validation)</option><option value="wildcard">Wildcard</option><option value="SAN">SAN (Multi-Domain)</option><option value="mTLS">mTLS</option><option value="self_signed">Self-Signed</option></select></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Environment</label><select id="cert-env"><option value="prod">Production</option><option value="staging">Staging</option><option value="dev">Development</option></select></div><div class="form-group"><label>Key Algorithm</label><select id="cert-algo"><option value="ECDSA-P256">ECDSA P-256</option><option value="ECDSA-P384">ECDSA P-384</option><option value="RSA-4096">RSA-4096</option><option value="RSA-2048">RSA-2048</option></select></div></div>'+
    '<div class="form-group"><label>SANs (comma-separated)</label><input id="cert-sans" placeholder="grpc.example.com, ws.example.com"></div>'+
    '<div class="form-group"><label>Validity</label><select id="cert-validity"><option value="90d">90 Days</option><option value="1y">1 Year</option><option value="2y">2 Years</option></select></div></div>'+
    '<div id="cert-tab-ca" style="display:none"><div class="form-group"><label>Certificate Authority</label><select id="cert-ca"><option value="">Auto-select (best price)</option>'+caOptions+'</select></div>'+
    '<div style="background:#f8fafc;border:1px solid var(--border);border-radius:0.5rem;padding:1rem;margin-bottom:1rem"><h4 style="font-size:0.8rem;margin-bottom:0.5rem">Provider Info</h4><div id="ca-info-box"><p style="font-size:0.8rem;color:var(--text-muted)">Select a CA to see pricing and details</p></div></div>'+
    '<div class="form-group"><label>Justification / Notes</label><input id="cert-justification" placeholder="New API service going live"></div>'+
    '<div class="form-group"><label>Payment Method</label><select id="cert-payment"><option value="account_credit">Account Credit</option><option value="invoice">Invoice</option><option value="free">Free (Let\'s Encrypt / Internal)</option></select></div></div>'+
    '<div style="margin-top:1rem"><button class="btn btn-primary" onclick="createCert()">Submit Request</button> <button class="btn btn-outline" onclick="hideCreateCert()">Cancel</button></div></div></div>';
}

function showCertTab(tab) {
    document.getElementById('cert-tab-details').style.display = tab==='details'?'block':'none';
    document.getElementById('cert-tab-ca').style.display = tab==='ca'?'block':'none';
    document.getElementById('tab-details').className = 'btn btn-sm '+(tab==='details'?'btn-primary':'btn-outline');
    document.getElementById('tab-ca').className = 'btn btn-sm '+(tab==='ca'?'btn-primary':'btn-outline');
}

function renderCertTable(certs){if(!certs.length)return '<div class="empty-state"><div class="icon">📜</div><p>No certificates found</p></div>';return '<table><thead><tr><th>Domain</th><th>Type</th><th>Status</th><th>Algorithm</th><th>Env</th><th>Issuer</th><th>Expires</th><th>Actions</th></tr></thead><tbody>'+certs.map(function(c){return '<tr><td><strong>'+c.domain+'</strong>'+(c.sans&&c.sans.length?'<br><span style="font-size:0.7rem;color:var(--text-muted)">+'+c.sans.length+' SANs</span>':'')+'</td><td>'+c.type+'</td><td>'+statusBadge(c.status)+'</td><td style="font-size:0.75rem">'+c.key_algorithm+'</td><td>'+c.environment+'</td><td>'+(c.issuer||'-')+'</td><td>'+(c.not_after?new Date(c.not_after).toLocaleDateString():'-')+'</td><td>'+(c.status==='active'?'<button class="btn btn-outline btn-sm" onclick="revokeCert(\''+c.id+'\')">Revoke</button>':'')+'</td></tr>';}).join('')+'</tbody></table>';}
function filterCerts(s){var f=s==='all'?certData:certData.filter(function(c){return c.status===s});document.getElementById('cert-table-wrap').innerHTML=renderCertTable(f);}
function showCreateCert(){document.getElementById('create-cert-panel').classList.remove('hidden');}
function hideCreateCert(){document.getElementById('create-cert-panel').classList.add('hidden');}

async function createCert(){
    var sat=satData[0];if(!sat){alert('Register a satellite first');return;}
    var body={domain:document.getElementById('cert-domain').value,type:document.getElementById('cert-type').value,environment:document.getElementById('cert-env').value,key_algorithm:document.getElementById('cert-algo').value,sans:document.getElementById('cert-sans').value.split(',').map(function(s){return s.trim()}).filter(Boolean),satellite_id:sat.id,ca_provider:(document.getElementById('cert-ca')?document.getElementById('cert-ca').value:'')};
    var res=await fetch(API+'/api/v1/certificates',{method:'POST',headers:headers(),body:JSON.stringify(body)});
    if(res.ok){hideCreateCert();await loadAll();}else{alert('Failed to create certificate');}
}
async function revokeCert(id){if(!confirm('Revoke this certificate?'))return;await fetch(API+'/api/v1/certificates/'+id+'/revoke',{method:'POST',headers:headers()});await loadAll();}

// CA Providers page
function renderProviders() {
    try {
    return '<div style="display:flex;justify-content:flex-end;margin-bottom:1rem"><button class="btn btn-primary btn-sm" onclick="showAddProvider()">+ Add CA Provider</button></div>'+
    '<div class="stats-grid" style="grid-template-columns:repeat(3,1fr)"><div class="stat-card blue"><div class="label">Total Providers</div><div class="value">'+caProviders.length+'</div></div><div class="stat-card green"><div class="label">Active</div><div class="value">'+caProviders.filter(function(p){return p.active}).length+'</div></div><div class="stat-card yellow"><div class="label">Total Budget</div><div class="value">$'+caProviders.reduce(function(a,p){return a+(p.budget||0)},0).toLocaleString()+'</div></div></div>'+
    caProviders.map(function(p,i){
        var pIcon = {letsencrypt:'🔒',digicert:'🛡️',godaddy:'🌐',sectigo:'🔑',zerossl:'⚡',vault_pki:'🏛️',namecheap:'🏷️'}[p.provider]||'📜';
        var keyDisplay = p.apiKey ? '<span style="font-family:monospace">'+p.apiKey+'</span>' : '<span style="color:var(--danger)">Not configured</span>';
        return '<div class="panel" style="margin-bottom:1rem"><div class="panel-header"><h3>'+pIcon+' '+p.name+' '+(p.active?'<span class="badge badge-active">active</span>':'<span class="badge badge-offline">disabled</span>')+'</h3><div style="display:flex;gap:0.5rem"><button class="btn btn-outline btn-sm" onclick="editProvider('+i+')">Edit</button><button class="btn btn-outline btn-sm" onclick="toggleProvider('+i+')">'+(p.active?'Disable':'Enable')+'</button></div></div><div class="panel-body"><div style="display:grid;grid-template-columns:repeat(4,1fr);gap:1rem;font-size:0.8rem">'+
        '<div><span style="color:var(--text-muted)">Provider</span><br><strong>'+p.provider+'</strong></div>'+
        '<div><span style="color:var(--text-muted)">Environment</span><br><strong>'+p.environment+'</strong></div>'+
        '<div><span style="color:var(--text-muted)">API Key</span><br>'+keyDisplay+'</div>'+
        '<div><span style="color:var(--text-muted)">Budget / Spent</span><br><strong>'+(p.budget?'$'+(p.budget||0).toLocaleString()+' / $'+(p.spent||0).toLocaleString():'Free')+'</strong></div>'+
        '</div></div></div>';
    }).join('')+
    '<div class="panel hidden" id="provider-form-panel"><div class="panel-header"><h3 id="provider-form-title">Add CA Provider</h3></div><div class="panel-body">'+
    '<div class="form-row"><div class="form-group"><label>Display Name</label><input id="prov-name" placeholder="DigiCert Enterprise"></div><div class="form-group"><label>Provider Type</label><select id="prov-type"><option value="letsencrypt">Let\'s Encrypt (ACME)</option><option value="digicert">DigiCert</option><option value="godaddy">GoDaddy</option><option value="namecheap">Namecheap</option><option value="sectigo">Sectigo / Comodo</option><option value="globalSign">GlobalSign</option><option value="entrust">Entrust</option><option value="zerossl">ZeroSSL</option><option value="vault_pki">HashiCorp Vault PKI</option><option value="custom">Custom / Internal CA</option></select></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Environment</label><select id="prov-env"><option value="production">Production</option><option value="staging">Staging</option><option value="development">Development</option></select></div><div class="form-group"><label>Monthly Budget (USD)</label><input id="prov-budget" type="number" placeholder="5000" value="0"></div></div>'+
    '<div class="form-group"><label>API Key / Secret</label><input id="prov-apikey" type="password" placeholder="Enter API key or token"></div>'+
    '<div class="form-group"><label>API Endpoint (optional)</label><input id="prov-endpoint" placeholder="https://api.digicert.com/services/v2"></div>'+
    '<input type="hidden" id="prov-edit-idx" value="-1">'+
    '<button class="btn btn-primary" onclick="saveProvider()">Save Provider</button> <button class="btn btn-outline" onclick="hideProviderForm()">Cancel</button></div></div>';
    } catch(e) { return '<div class="empty-state"><div class="icon">🏢</div><p>Error loading providers: '+e.message+'</p></div>'; }
}

function showAddProvider(){
    var panel = document.getElementById('provider-form-panel');
    if (!panel) { alert('Form not found - try refreshing'); return; }
    panel.classList.remove('hidden');
    document.getElementById('provider-form-title').textContent='Add CA Provider';
    document.getElementById('prov-edit-idx').value='-1';
    var nameEl = document.getElementById('prov-name'); if(nameEl) nameEl.value='';
    var keyEl = document.getElementById('prov-apikey'); if(keyEl) { keyEl.value=''; keyEl.placeholder='Enter API key or token'; }
    var budgetEl = document.getElementById('prov-budget'); if(budgetEl) budgetEl.value='0';
    var endpointEl = document.getElementById('prov-endpoint'); if(endpointEl) endpointEl.value='';
}
function hideProviderForm(){var p=document.getElementById('provider-form-panel');if(p)p.classList.add('hidden');}

function editProvider(idx){
    var p=caProviders[idx];
    if(!p) return;
    var panel = document.getElementById('provider-form-panel');
    if (!panel) return;
    panel.classList.remove('hidden');
    document.getElementById('provider-form-title').textContent='Edit: '+p.name;
    document.getElementById('prov-edit-idx').value=idx;
    var nameEl=document.getElementById('prov-name'); if(nameEl) nameEl.value=p.name;
    var typeEl=document.getElementById('prov-type'); if(typeEl) typeEl.value=p.provider;
    var envEl=document.getElementById('prov-env'); if(envEl) envEl.value=p.environment;
    var budgetEl=document.getElementById('prov-budget'); if(budgetEl) budgetEl.value=p.budget;
    var keyEl=document.getElementById('prov-apikey'); if(keyEl) { keyEl.value=''; keyEl.placeholder='Enter new key (leave blank to keep current)'; }
}

function saveProvider(){
    var idxEl = document.getElementById('prov-edit-idx');
    var nameEl = document.getElementById('prov-name');
    var typeEl = document.getElementById('prov-type');
    var envEl = document.getElementById('prov-env');
    var budgetEl = document.getElementById('prov-budget');
    var keyEl = document.getElementById('prov-apikey');
    if (!nameEl || !nameEl.value) { alert('Please enter a provider name'); return; }
    var idx = parseInt(idxEl.value);
    var newKey = keyEl ? keyEl.value : '';
    if(idx>=0){
        if(newKey) caProviders[idx].apiKey = newKey.substring(0,6)+'...masked';
        caProviders[idx].name = nameEl.value;
        caProviders[idx].provider = typeEl ? typeEl.value : caProviders[idx].provider;
        caProviders[idx].environment = envEl ? envEl.value : caProviders[idx].environment;
        caProviders[idx].budget = budgetEl ? parseInt(budgetEl.value)||0 : caProviders[idx].budget;
    } else {
        var obj = {
            id: String(Date.now()),
            name: nameEl.value,
            provider: typeEl ? typeEl.value : 'custom',
            environment: envEl ? envEl.value : 'production',
            budget: budgetEl ? parseInt(budgetEl.value)||0 : 0,
            apiKey: newKey ? newKey.substring(0,6)+'...masked' : '',
            active: true,
            spent: 0
        };
        caProviders.push(obj);
    }
    saveProviders(); hideProviderForm(); renderPage();
}

function toggleProvider(idx){caProviders[idx].active=!caProviders[idx].active;saveProviders();renderPage();}

// Policies page - editable
function renderPolicies() {
    var p = policies;
    return '<div style="display:flex;justify-content:flex-end;margin-bottom:1rem"><button class="btn btn-primary btn-sm" id="policy-edit-btn" onclick="togglePolicyEdit()">Edit Policies</button></div>'+
    '<div class="stats-grid" style="grid-template-columns:repeat(3,1fr)"><div class="stat-card green"><div class="label">Min Key Size</div><div class="value">'+p.min_ecdsa_curve+'</div><div class="sub">ECDSA minimum</div></div><div class="stat-card blue"><div class="label">Max Validity</div><div class="value">'+p.max_validity_days+'d</div><div class="sub">'+(p.max_validity_days/365).toFixed(0)+' year(s)</div></div><div class="stat-card purple"><div class="label">Auto-Renewal</div><div class="value">'+p.auto_renew_days+'d</div><div class="sub">Before expiry</div></div></div>'+
    '<div class="panel"><div class="panel-header"><h3>Key Strength Policy</h3></div><div class="panel-body" id="policy-key-section">'+
    '<div class="policy-card"><h4>Minimum RSA Key Size</h4><p>All RSA keys must be at least this size</p><span class="policy-value" id="pv-rsa">'+p.min_rsa_bits+' bits</span><input class="policy-input hidden" id="pi-rsa" type="number" value="'+p.min_rsa_bits+'"></div>'+
    '<div class="policy-card"><h4>Minimum ECDSA Curve</h4><p>Minimum elliptic curve</p><span class="policy-value" id="pv-ecdsa">'+p.min_ecdsa_curve+'</span><select class="policy-input hidden" id="pi-ecdsa"><option value="P-256"'+(p.min_ecdsa_curve==='P-256'?' selected':'')+'>P-256</option><option value="P-384"'+(p.min_ecdsa_curve==='P-384'?' selected':'')+'>P-384</option><option value="P-521"'+(p.min_ecdsa_curve==='P-521'?' selected':'')+'>P-521</option></select></div>'+
    '<div class="policy-card"><h4>Allowed Signature Algorithms</h4><p>Permitted algorithms</p><span class="policy-value" id="pv-sig">'+p.allowed_sig_algos+'</span><input class="policy-input hidden" id="pi-sig" value="'+p.allowed_sig_algos+'"></div></div></div>'+
    '<div class="panel"><div class="panel-header"><h3>Certificate Authority Policy</h3></div><div class="panel-body">'+
    '<div class="policy-card"><h4>Allowed CAs (Production)</h4><p>Trusted CAs for prod</p><span class="policy-value" id="pv-caprod">'+p.allowed_cas_prod+'</span><input class="policy-input hidden" id="pi-caprod" value="'+p.allowed_cas_prod+'"></div>'+
    '<div class="policy-card"><h4>Allowed CAs (Non-Prod)</h4><p>Extended list</p><span class="policy-value" id="pv-cadev">'+p.allowed_cas_nonprod+'</span><input class="policy-input hidden" id="pi-cadev" value="'+p.allowed_cas_nonprod+'"></div>'+
    '<div class="policy-card"><h4>Self-Signed in Production</h4><p>Allow self-signed certs in prod?</p><span class="policy-value" id="pv-selfsign">'+(p.self_signed_prod?'Allowed':'Blocked')+'</span><select class="policy-input hidden" id="pi-selfsign"><option value="false"'+(!p.self_signed_prod?' selected':'')+'>Blocked</option><option value="true"'+(p.self_signed_prod?' selected':'')+'>Allowed</option></select></div></div></div>'+
    '<div class="panel"><div class="panel-header"><h3>Validity & Renewal Policy</h3></div><div class="panel-body">'+
    '<div class="policy-card"><h4>Maximum Validity</h4><p>Max certificate duration</p><span class="policy-value" id="pv-maxval">'+p.max_validity_days+' days</span><input class="policy-input hidden" id="pi-maxval" type="number" value="'+p.max_validity_days+'"></div>'+
    '<div class="policy-card"><h4>Auto-Renewal Threshold</h4><p>Days before expiry to renew</p><span class="policy-value" id="pv-renew">'+p.auto_renew_days+' days</span><input class="policy-input hidden" id="pi-renew" type="number" value="'+p.auto_renew_days+'"></div>'+
    '<div class="policy-card"><h4>Wildcard Approval Level</h4><p>Required approval for wildcards</p><span class="policy-value" id="pv-wild">'+p.wildcard_approval+'</span><select class="policy-input hidden" id="pi-wild"><option value="L1"'+(p.wildcard_approval==='L1'?' selected':'')+'>L1 (Team Lead)</option><option value="L2"'+(p.wildcard_approval==='L2'?' selected':'')+'>L2 (Security)</option><option value="L3"'+(p.wildcard_approval==='L3'?' selected':'')+'>L3 (Finance + Security)</option></select></div></div></div>'+
    '<div class="panel"><div class="panel-header"><h3>Deployment Policy</h3></div><div class="panel-body">'+
    '<div class="policy-card"><h4>Verify After Deploy</h4><p>TLS handshake check</p><span class="policy-value" id="pv-verify">'+(p.verify_after_deploy?'Enabled':'Disabled')+'</span><select class="policy-input hidden" id="pi-verify"><option value="true"'+(p.verify_after_deploy?' selected':'')+'>Enabled</option><option value="false"'+(!p.verify_after_deploy?' selected':'')+'>Disabled</option></select></div>'+
    '<div class="policy-card"><h4>Rollback on Failure</h4><p>Auto-revert on deploy failure</p><span class="policy-value" id="pv-rollback">'+(p.rollback_on_failure?'Enabled':'Disabled')+'</span><select class="policy-input hidden" id="pi-rollback"><option value="true"'+(p.rollback_on_failure?' selected':'')+'>Enabled</option><option value="false"'+(!p.rollback_on_failure?' selected':'')+'>Disabled</option></select></div>'+
    '<div class="policy-card"><h4>Backup Count</h4><p>Keep last N backups</p><span class="policy-value" id="pv-backup">'+p.backup_count+'</span><input class="policy-input hidden" id="pi-backup" type="number" value="'+p.backup_count+'"></div></div></div>'+
    '<div class="hidden" id="policy-save-bar" style="padding:1rem;background:var(--card);border:1px solid var(--primary);border-radius:0.5rem;text-align:center"><button class="btn btn-primary" onclick="savePoliciesForm()">Save All Policy Changes</button> <button class="btn btn-outline" onclick="cancelPolicyEdit()">Cancel</button></div>';
}

var policyEditing = false;
function togglePolicyEdit(){
    policyEditing = !policyEditing;
    document.querySelectorAll('.policy-value').forEach(function(el){el.style.display=policyEditing?'none':'inline'});
    document.querySelectorAll('.policy-input').forEach(function(el){
        if(policyEditing){el.classList.remove('hidden');el.style.display='inline-block';}
        else{el.classList.add('hidden');el.style.display='none';}
    });
    document.getElementById('policy-save-bar').classList.toggle('hidden',!policyEditing);
    document.getElementById('policy-edit-btn').textContent=policyEditing?'Cancel Edit':'Edit Policies';
}
function cancelPolicyEdit(){policyEditing=true;togglePolicyEdit();renderPage();}

function savePoliciesForm(){
    policies.min_rsa_bits=parseInt(document.getElementById('pi-rsa').value);
    policies.min_ecdsa_curve=document.getElementById('pi-ecdsa').value;
    policies.allowed_sig_algos=document.getElementById('pi-sig').value;
    policies.allowed_cas_prod=document.getElementById('pi-caprod').value;
    policies.allowed_cas_nonprod=document.getElementById('pi-cadev').value;
    policies.self_signed_prod=document.getElementById('pi-selfsign').value==='true';
    policies.max_validity_days=parseInt(document.getElementById('pi-maxval').value);
    policies.auto_renew_days=parseInt(document.getElementById('pi-renew').value);
    policies.wildcard_approval=document.getElementById('pi-wild').value;
    policies.verify_after_deploy=document.getElementById('pi-verify').value==='true';
    policies.rollback_on_failure=document.getElementById('pi-rollback').value==='true';
    policies.backup_count=parseInt(document.getElementById('pi-backup').value);
    savePolicies();policyEditing=false;renderPage();
}

// Endpoints Management page
function renderEndpoints() {
    try {
    var byCategory = {app:0, cloud:0, data:0, os:0};
    managedEndpoints.forEach(function(ep){ byCategory[ep.category] = (byCategory[ep.category]||0)+1; });

    var filterBtns = '<div style="display:flex;gap:0.5rem"><button class="btn btn-outline btn-sm" onclick="filterEndpoints(\'all\')">All ('+managedEndpoints.length+')</button><button class="btn btn-outline btn-sm" onclick="filterEndpoints(\'app\')">App ('+byCategory.app+')</button><button class="btn btn-outline btn-sm" onclick="filterEndpoints(\'cloud\')">Cloud ('+byCategory.cloud+')</button><button class="btn btn-outline btn-sm" onclick="filterEndpoints(\'data\')">Data ('+byCategory.data+')</button></div>';

    return '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem">'+filterBtns+'<button class="btn btn-primary btn-sm" onclick="showAddEndpoint()">+ Add Endpoint</button></div>'+
    '<div class="stats-grid" style="grid-template-columns:repeat(4,1fr)">'+
        '<div class="stat-card blue"><div class="label">Total Endpoints</div><div class="value">'+managedEndpoints.length+'</div></div>'+
        '<div class="stat-card green"><div class="label">App/Web</div><div class="value">'+byCategory.app+'</div><div class="sub">Nginx, Apache, HAProxy</div></div>'+
        '<div class="stat-card purple"><div class="label">Cloud</div><div class="value">'+byCategory.cloud+'</div><div class="sub">ALB, CloudFront, API GW</div></div>'+
        '<div class="stat-card yellow"><div class="label">Data</div><div class="value">'+byCategory.data+'</div><div class="sub">PostgreSQL, Redis, Kafka</div></div>'+
    '</div>'+
    '<div class="panel"><div class="panel-body"><div class="table-wrap" id="ep-table-wrap">'+renderEndpointTable(managedEndpoints)+'</div></div></div>'+
    renderEndpointForm();
    } catch(e) { return '<div class="empty-state"><div class="icon">🖥️</div><p>Error: '+e.message+'</p><pre>'+e.stack+'</pre></div>'; }
}

function renderEndpointTable(eps) {
    if(!eps.length) return '<div class="empty-state"><div class="icon">🖥️</div><p>No endpoints configured</p></div>';
    var categoryIcons = {app:'🌐', cloud:'☁️', data:'🗄️', os:'💻'};
    return '<table><thead><tr><th>Endpoint</th><th>Host</th><th>Type</th><th>Environment</th><th>Cert Path</th><th>Service</th><th>Tags</th><th>Actions</th></tr></thead><tbody>'+eps.map(function(ep, i){
        var idx = managedEndpoints.indexOf(ep);
        return '<tr><td><strong>'+(categoryIcons[ep.category]||'📌')+' '+ep.name+'</strong></td><td style="font-family:monospace;font-size:0.75rem">'+ep.host+':'+ep.port+'</td><td><span class="badge badge-active">'+ep.category+'/'+ep.type+'</span></td><td>'+ep.environment+'</td><td style="font-family:monospace;font-size:0.7rem">'+(ep.certPath||'N/A (cloud-managed)')+'</td><td>'+(ep.service||'-')+'</td><td>'+(ep.tags||[]).map(function(t){return '<span style="background:#f1f5f9;padding:0.1rem 0.3rem;border-radius:0.2rem;font-size:0.65rem;margin-right:0.2rem">'+t+'</span>'}).join('')+'</td><td><button class="btn btn-outline btn-sm" onclick="editEndpoint('+idx+')">Edit</button> <button class="btn btn-outline btn-sm" onclick="removeEndpoint('+idx+')">Remove</button></td></tr>';
    }).join('')+'</tbody></table>';
}

function filterEndpoints(cat) {
    var filtered = cat === 'all' ? managedEndpoints : managedEndpoints.filter(function(ep){return ep.category===cat});
    document.getElementById('ep-table-wrap').innerHTML = renderEndpointTable(filtered);
}

function renderEndpointForm() {
    return '<div class="panel hidden" id="ep-form-panel"><div class="panel-header"><h3 id="ep-form-title">Add Endpoint</h3></div><div class="panel-body">'+
    '<div class="form-row"><div class="form-group"><label>Endpoint Name</label><input id="ep-name" placeholder="API Gateway (Prod)"></div><div class="form-group"><label>Host / IP</label><input id="ep-host" placeholder="10.0.5.42 or alb-prod.elb.amazonaws.com"></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Port</label><input id="ep-port" type="number" value="443" placeholder="443"></div><div class="form-group"><label>Category</label><select id="ep-category" onchange="updateEndpointTypeOptions()"><option value="app">App (Nginx, Apache, HAProxy)</option><option value="cloud">Cloud (ALB, CloudFront, GKE)</option><option value="data">Data (PostgreSQL, Redis, Kafka)</option><option value="os">OS (Linux trust store, Windows)</option></select></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Type</label><select id="ep-type"><option value="nginx">Nginx</option><option value="apache">Apache</option><option value="haproxy">HAProxy</option></select></div><div class="form-group"><label>Environment</label><select id="ep-env"><option value="prod">Production</option><option value="staging">Staging</option><option value="dev">Development</option></select></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Certificate Path</label><input id="ep-certpath" placeholder="/etc/nginx/ssl/cert.pem"></div><div class="form-group"><label>Key Path</label><input id="ep-keypath" placeholder="/etc/nginx/ssl/key.pem"></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Service Name</label><input id="ep-service" placeholder="nginx"></div><div class="form-group"><label>Reload Command</label><input id="ep-reload" placeholder="systemctl reload nginx"></div></div>'+
    '<div class="form-row"><div class="form-group"><label>Tags (comma-separated)</label><input id="ep-tags" placeholder="api, prod, web-tier"></div><div class="form-group"><label>Cloud Resource ID (if cloud)</label><input id="ep-resource" placeholder="arn:aws:..."></div></div>'+
    '<input type="hidden" id="ep-edit-idx" value="-1">'+
    '<button class="btn btn-primary" onclick="saveEndpoint()">Save Endpoint</button> <button class="btn btn-outline" onclick="hideEndpointForm()">Cancel</button></div></div>';
}

function updateEndpointTypeOptions() {
    var cat = document.getElementById('ep-category').value;
    var typeEl = document.getElementById('ep-type');
    var options = {
        app: [['nginx','Nginx'],['apache','Apache'],['haproxy','HAProxy'],['tomcat','Tomcat'],['iis','IIS']],
        cloud: [['aws-alb','AWS ALB'],['aws-cloudfront','AWS CloudFront'],['aws-apigateway','AWS API Gateway'],['gcp-lb','GCP Load Balancer'],['azure-appgw','Azure App Gateway']],
        data: [['postgresql','PostgreSQL'],['mysql','MySQL'],['mongodb','MongoDB'],['redis','Redis'],['kafka','Kafka'],['rabbitmq','RabbitMQ'],['elasticsearch','Elasticsearch']],
        os: [['linux','Linux Trust Store'],['windows','Windows Cert Store'],['kubernetes','Kubernetes Secret']]
    };
    var opts = options[cat] || options.app;
    typeEl.innerHTML = opts.map(function(o){return '<option value="'+o[0]+'">'+o[1]+'</option>'}).join('');
}

function showAddEndpoint() {
    var panel = document.getElementById('ep-form-panel');
    if(panel) panel.classList.remove('hidden');
    document.getElementById('ep-form-title').textContent = 'Add Endpoint';
    document.getElementById('ep-edit-idx').value = '-1';
    ['ep-name','ep-host','ep-certpath','ep-keypath','ep-service','ep-reload','ep-tags','ep-resource'].forEach(function(id){var el=document.getElementById(id);if(el)el.value='';});
    var portEl = document.getElementById('ep-port'); if(portEl) portEl.value = '443';
}
function hideEndpointForm() { var p=document.getElementById('ep-form-panel'); if(p) p.classList.add('hidden'); }

function editEndpoint(idx) {
    var ep = managedEndpoints[idx];
    if(!ep) return;
    var panel = document.getElementById('ep-form-panel');
    if(panel) panel.classList.remove('hidden');
    document.getElementById('ep-form-title').textContent = 'Edit: ' + ep.name;
    document.getElementById('ep-edit-idx').value = idx;
    var setVal = function(id,v){var el=document.getElementById(id);if(el)el.value=v||'';};
    setVal('ep-name', ep.name);
    setVal('ep-host', ep.host);
    setVal('ep-port', ep.port);
    setVal('ep-category', ep.category);
    updateEndpointTypeOptions();
    setVal('ep-type', ep.type);
    setVal('ep-env', ep.environment);
    setVal('ep-certpath', ep.certPath);
    setVal('ep-keypath', ep.keyPath);
    setVal('ep-service', ep.service);
    setVal('ep-reload', ep.reloadCmd);
    setVal('ep-tags', (ep.tags||[]).join(', '));
    setVal('ep-resource', ep.resourceId || '');
}

function saveEndpoint() {
    var nameEl = document.getElementById('ep-name');
    var hostEl = document.getElementById('ep-host');
    if(!nameEl||!nameEl.value||!hostEl||!hostEl.value){alert('Name and Host are required');return;}
    var idx = parseInt(document.getElementById('ep-edit-idx').value);
    var obj = {
        name: nameEl.value,
        host: hostEl.value,
        port: parseInt(document.getElementById('ep-port').value)||443,
        category: document.getElementById('ep-category').value,
        type: document.getElementById('ep-type').value,
        environment: document.getElementById('ep-env').value,
        certPath: document.getElementById('ep-certpath').value,
        keyPath: document.getElementById('ep-keypath').value,
        service: document.getElementById('ep-service').value,
        reloadCmd: document.getElementById('ep-reload').value,
        tags: document.getElementById('ep-tags').value.split(',').map(function(s){return s.trim()}).filter(Boolean),
        resourceId: document.getElementById('ep-resource').value,
        status: 'active',
        certId: '',
        lastDeployed: ''
    };
    if(idx>=0){
        obj.id = managedEndpoints[idx].id;
        obj.certId = managedEndpoints[idx].certId;
        obj.lastDeployed = managedEndpoints[idx].lastDeployed;
        managedEndpoints[idx] = obj;
    } else {
        obj.id = 'ep-' + Date.now();
        managedEndpoints.push(obj);
    }
    saveEndpoints(); hideEndpointForm(); renderPage();
}

function removeEndpoint(idx) {
    if(!confirm('Remove endpoint "'+managedEndpoints[idx].name+'"?')) return;
    managedEndpoints.splice(idx, 1);
    saveEndpoints(); renderPage();
}

// Discovery & Scanning page
function renderDiscovery() {
    var d = discoveryConfig;
    var domainRows = d.domains.map(function(item, i){
        return '<tr><td><strong>'+item.value+'</strong></td><td>'+item.label+'</td><td>'+(item.enabled?'<span class="badge badge-active">active</span>':'<span class="badge badge-revoked">disabled</span>')+'</td><td><button class="btn btn-outline btn-sm" onclick="toggleDiscItem(\'domains\','+i+')">'+(item.enabled?'Disable':'Enable')+'</button> <button class="btn btn-outline btn-sm" onclick="removeDiscItem(\'domains\','+i+')">Remove</button></td></tr>';
    }).join('');
    var cidrRows = d.cidrs.map(function(item, i){
        return '<tr><td><strong>'+item.value+'</strong></td><td>'+item.label+'</td><td>'+(item.enabled?'<span class="badge badge-active">active</span>':'<span class="badge badge-revoked">disabled</span>')+'</td><td><button class="btn btn-outline btn-sm" onclick="toggleDiscItem(\'cidrs\','+i+')">'+(item.enabled?'Disable':'Enable')+'</button> <button class="btn btn-outline btn-sm" onclick="removeDiscItem(\'cidrs\','+i+')">Remove</button></td></tr>';
    }).join('');

    return '<div class="stats-grid" style="grid-template-columns:repeat(4,1fr)">'+
        '<div class="stat-card blue"><div class="label">Domains</div><div class="value">'+d.domains.filter(function(x){return x.enabled}).length+'</div><div class="sub">of '+d.domains.length+' configured</div></div>'+
        '<div class="stat-card green"><div class="label">CIDR Ranges</div><div class="value">'+d.cidrs.filter(function(x){return x.enabled}).length+'</div><div class="sub">of '+d.cidrs.length+' configured</div></div>'+
        '<div class="stat-card purple"><div class="label">Schedule</div><div class="value">Daily</div><div class="sub">'+d.schedule+'</div></div>'+
        '<div class="stat-card yellow"><div class="label">Last Scan</div><div class="value">'+(d.lastScan?'Done':'Never')+'</div><div class="sub">'+(d.lastScan||'No scans yet')+'</div></div>'+
    '</div>'+
    '<div class="panel"><div class="panel-header"><h3>Domains for Certificate Discovery</h3><button class="btn btn-primary btn-sm" onclick="showAddDisc(\'domain\')">+ Add Domain</button></div><div class="panel-body">'+
        (domainRows?'<div class="table-wrap"><table><thead><tr><th>Domain Pattern</th><th>Label</th><th>Status</th><th>Actions</th></tr></thead><tbody>'+domainRows+'</tbody></table></div>':'<div class="empty-state"><p>No domains configured</p></div>')+
    '</div></div>'+
    '<div class="panel"><div class="panel-header"><h3>CIDR Ranges for Network Scanning</h3><button class="btn btn-primary btn-sm" onclick="showAddDisc(\'cidr\')">+ Add CIDR</button></div><div class="panel-body">'+
        (cidrRows?'<div class="table-wrap"><table><thead><tr><th>CIDR Range</th><th>Label</th><th>Status</th><th>Actions</th></tr></thead><tbody>'+cidrRows+'</tbody></table></div>':'<div class="empty-state"><p>No CIDR ranges configured</p></div>')+
    '</div></div>'+
    '<div class="panel"><div class="panel-header"><h3>Scan Configuration</h3></div><div class="panel-body">'+
        '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:1rem">'+
        '<div class="form-group"><label>Schedule (Cron)</label><input id="disc-schedule" value="'+d.schedule+'" onchange="updateDiscConfig()"></div>'+
        '<div class="form-group"><label>Ports to Scan</label><input id="disc-ports" value="'+d.ports.join(', ')+'" onchange="updateDiscConfig()"></div>'+
        '<div class="form-group"><label>Concurrency</label><input id="disc-concurrency" type="number" value="'+d.concurrency+'" onchange="updateDiscConfig()"></div>'+
        '</div>'+
        '<div style="display:flex;gap:2rem;margin-top:1rem;align-items:center">'+
        '<label style="display:flex;align-items:center;gap:0.5rem;font-size:0.85rem"><input type="checkbox" id="disc-auto" '+(d.autoDiscovery?'checked':'')+' onchange="updateDiscConfig()"> Auto-Discovery Enabled</label>'+
        '<label style="display:flex;align-items:center;gap:0.5rem;font-size:0.85rem"><input type="checkbox" id="disc-ondemand" '+(d.scanOnDemand?'checked':'')+' onchange="updateDiscConfig()"> Allow On-Demand Scans</label>'+
        '<button class="btn btn-primary btn-sm" onclick="triggerScan()">Trigger Scan Now</button>'+
        '</div>'+
    '</div></div>'+
    '<div class="panel hidden" id="disc-add-panel"><div class="panel-header"><h3 id="disc-add-title">Add Domain</h3></div><div class="panel-body">'+
        '<div class="form-row"><div class="form-group"><label id="disc-add-value-label">Domain Pattern</label><input id="disc-add-value" placeholder="*.example.com or 10.0.0.0/16"></div><div class="form-group"><label>Label / Description</label><input id="disc-add-label" placeholder="Production services"></div></div>'+
        '<button class="btn btn-primary" onclick="addDiscItem()">Add</button> <button class="btn btn-outline" onclick="hideAddDisc()">Cancel</button>'+
        '<input type="hidden" id="disc-add-type" value="domain">'+
    '</div></div>';
}

var discAddType = 'domain';
function showAddDisc(type) {
    discAddType = type;
    var panel = document.getElementById('disc-add-panel');
    if(panel) panel.classList.remove('hidden');
    var title = document.getElementById('disc-add-title');
    if(title) title.textContent = type==='domain' ? 'Add Domain' : 'Add CIDR Range';
    var lbl = document.getElementById('disc-add-value-label');
    if(lbl) lbl.textContent = type==='domain' ? 'Domain Pattern' : 'CIDR Range';
    var valEl = document.getElementById('disc-add-value');
    if(valEl) { valEl.value=''; valEl.placeholder = type==='domain'?'*.example.com':'10.0.0.0/16'; }
    var lblEl = document.getElementById('disc-add-label');
    if(lblEl) lblEl.value='';
    var typeEl = document.getElementById('disc-add-type');
    if(typeEl) typeEl.value = type;
}
function hideAddDisc() { var p=document.getElementById('disc-add-panel'); if(p) p.classList.add('hidden'); }

function addDiscItem() {
    var valEl = document.getElementById('disc-add-value');
    var lblEl = document.getElementById('disc-add-label');
    var typeEl = document.getElementById('disc-add-type');
    if(!valEl || !valEl.value) { alert('Please enter a value'); return; }
    var type = typeEl ? typeEl.value : discAddType;
    var item = { value: valEl.value, label: lblEl?lblEl.value:'', enabled: true };
    if (type === 'domain') { discoveryConfig.domains.push(item); }
    else { discoveryConfig.cidrs.push(item); }
    saveDiscovery(); hideAddDisc(); renderPage();
}

function toggleDiscItem(list, idx) {
    discoveryConfig[list][idx].enabled = !discoveryConfig[list][idx].enabled;
    saveDiscovery(); renderPage();
}

function removeDiscItem(list, idx) {
    if(!confirm('Remove this entry?')) return;
    discoveryConfig[list].splice(idx, 1);
    saveDiscovery(); renderPage();
}

function updateDiscConfig() {
    var schedEl = document.getElementById('disc-schedule');
    var portsEl = document.getElementById('disc-ports');
    var concEl = document.getElementById('disc-concurrency');
    var autoEl = document.getElementById('disc-auto');
    var ondemandEl = document.getElementById('disc-ondemand');
    if(schedEl) discoveryConfig.schedule = schedEl.value;
    if(portsEl) discoveryConfig.ports = portsEl.value.split(',').map(function(s){return parseInt(s.trim())}).filter(function(n){return !isNaN(n)});
    if(concEl) discoveryConfig.concurrency = parseInt(concEl.value)||100;
    if(autoEl) discoveryConfig.autoDiscovery = autoEl.checked;
    if(ondemandEl) discoveryConfig.scanOnDemand = ondemandEl.checked;
    saveDiscovery();
}

function triggerScan() {
    discoveryConfig.lastScan = new Date().toLocaleString();
    saveDiscovery();
    alert('Scan triggered! Discovery will run against '+discoveryConfig.domains.filter(function(x){return x.enabled}).length+' domains and '+discoveryConfig.cidrs.filter(function(x){return x.enabled}).length+' CIDR ranges.');
    renderPage();
}

// Satellites page
var vsatAgents = [];
async function loadVSatAgents() {
    try {
        var res = await fetch(API + '/vsatellite/agents');
        if (res.ok) {
            var data = await res.json();
            vsatAgents = data.agents || [];
        }
    } catch(e) { vsatAgents = []; }
}

function renderSatellites(){
    loadVSatAgents().then(function(){ renderVSatSection(); });
    var cards=satData.length?satData.map(function(s){var isOn=s.status==='online';return '<div class="satellite-card" style="border-left:3px solid '+(isOn?'var(--success)':'var(--danger)')+'"><div class="sat-icon" style="background:'+(isOn?'var(--success-light)':'var(--danger-light)')+'">📡</div><div class="sat-info"><h4>'+s.name+' '+statusBadge(s.status)+'</h4><p>Mode: '+(s.connector_mode||'direct')+' | Cloud: '+(s.cloud||'N/A')+' | Region: '+(s.region||'N/A')+' | Secrets: '+s.secrets_backend+'</p><p>Agents: '+(s.agent_count||0)+' | Certs: '+(s.cert_count||0)+' | Version: '+(s.version||'N/A')+'</p></div><div class="sat-meta"><p>Last Heartbeat</p><p style="font-weight:600">'+(s.last_heartbeat?new Date(s.last_heartbeat).toLocaleString():'Never')+'</p></div></div>';}).join(''):'<div class="empty-state"><div class="icon">📡</div><p>No satellites registered</p><button class="btn btn-primary" style="margin-top:1rem" onclick="showRegSat()">Register First Satellite</button></div>';
    return '<div style="display:flex;justify-content:flex-end;margin-bottom:1rem"><button class="btn btn-primary btn-sm" onclick="showRegSat()">+ Register Satellite</button></div>'+
    '<div class="panel"><div class="panel-header"><h3>Gateway Satellites</h3></div><div class="panel-body">'+cards+'</div></div>'+
    '<div class="panel"><div class="panel-header"><h3>vSatellite Agents (Connected to Gateway)</h3></div><div class="panel-body" id="vsat-agents-section"><div class="empty-state"><p>Loading vSatellite agents...</p></div></div></div>'+
    '<div class="panel hidden" id="reg-sat-panel"><div class="panel-header"><h3>Register Satellite</h3></div><div class="panel-body"><div class="form-row"><div class="form-group"><label>Name</label><input id="sat-name" placeholder="dc-mumbai-01"></div><div class="form-group"><label>Type</label><select id="sat-type"><option value="local">Local</option><option value="virtual">vSatellite</option></select></div></div><div class="form-row"><div class="form-group"><label>Connector Mode</label><select id="sat-mode"><option value="direct">Direct HTTPS</option><option value="proxy">Via Proxy</option><option value="vpn">VPN</option><option value="offline">Offline</option></select></div><div class="form-group"><label>Secrets Backend</label><select id="sat-secrets"><option value="aws">AWS Secrets Manager</option><option value="gcp">GCP Secret Manager</option><option value="vault">HashiCorp Vault</option></select></div></div><div class="form-row"><div class="form-group"><label>Cloud</label><select id="sat-cloud"><option value="aws">AWS</option><option value="gcp">GCP</option><option value="">On-Prem</option></select></div><div class="form-group"><label>Region</label><input id="sat-region" placeholder="ap-south-1"></div></div><button class="btn btn-primary" onclick="registerSat()">Register</button> <button class="btn btn-outline" onclick="hideRegSat()">Cancel</button></div></div>';
}

function renderVSatSection() {
    var el = document.getElementById('vsat-agents-section');
    if (!el) return;
    if (!vsatAgents.length) {
        el.innerHTML = '<div class="empty-state"><div class="icon">🛰️</div><p>No vSatellite agents connected</p></div>';
        return;
    }
    el.innerHTML = vsatAgents.map(function(a) {
        var isOn = a.status === 'online';
        var lastHB = a.last_heartbeat ? new Date(a.last_heartbeat).toLocaleString() : 'Never';
        return '<div class="satellite-card" style="border-left:3px solid '+(isOn?'var(--success)':'var(--danger)')+'">'+
            '<div class="sat-icon" style="background:'+(isOn?'var(--success-light)':'var(--danger-light)')+'">🛰️</div>'+
            '<div class="sat-info"><h4>'+a.name+' '+statusBadge(a.status)+'</h4>'+
            '<p>VPC: '+(a.vpc||'N/A')+' | Subnet: '+(a.subnet||'N/A')+' | CIDRs: '+((a.cidrs && a.cidrs.length)?a.cidrs.join(', '):'N/A')+'</p>'+
            '<p>Certs Managed: '+a.cert_count+' | Version: '+(a.version||'N/A')+' | Capabilities: '+((a.capabilities && a.capabilities.length)?a.capabilities.join(', '):'N/A')+'</p>'+
            '</div>'+
            '<div class="sat-meta"><p>Last Heartbeat</p><p style="font-weight:600">'+lastHB+'</p></div></div>';
    }).join('');
}
function showRegSat(){document.getElementById('reg-sat-panel').classList.remove('hidden');}
function hideRegSat(){document.getElementById('reg-sat-panel').classList.add('hidden');}
async function registerSat(){
    var body={name:document.getElementById('sat-name').value,type:document.getElementById('sat-type').value,connector_mode:document.getElementById('sat-mode').value,secrets_backend:document.getElementById('sat-secrets').value,cloud:document.getElementById('sat-cloud').value,region:document.getElementById('sat-region').value};
    var res=await fetch(API+'/api/v1/satellites',{method:'POST',headers:headers(),body:JSON.stringify(body)});
    if(res.ok){hideRegSat();await loadAll();}else{alert('Failed to register');}
}

// Expiring & Approvals
function renderExpiring(){var exp=getExpiringCerts();return '<div class="stats-grid" style="grid-template-columns:repeat(3,1fr)"><div class="stat-card red"><div class="label">Critical (&lt; 7d)</div><div class="value">'+exp.filter(function(c){return daysUntil(c.not_after)<=7}).length+'</div></div><div class="stat-card yellow"><div class="label">Warning (7-14d)</div><div class="value">'+exp.filter(function(c){var d=daysUntil(c.not_after);return d>7&&d<=14}).length+'</div></div><div class="stat-card blue"><div class="label">Upcoming (15-30d)</div><div class="value">'+exp.filter(function(c){return daysUntil(c.not_after)>14}).length+'</div></div></div><div class="panel"><div class="panel-header"><h3>Certificates Expiring in Next 30 Days</h3></div><div class="panel-body">'+renderExpiryTable(exp)+'</div></div>';}
function renderApprovals(){return '<div class="empty-state"><div class="icon">✅</div><p>No pending approvals</p><p style="font-size:0.8rem;margin-top:0.5rem">Certificate requests requiring approval will appear here</p></div>';}

// Health & Init
async function checkHealth(){try{var r=await fetch(API+'/health');var d=await r.json();document.getElementById('server-dot').style.background=d.status==='ok'?'var(--success)':'var(--danger)';}catch(e){document.getElementById('server-dot').style.background='var(--danger)';}}
async function init(){await checkHealth();if(token){var t=await fetchAPI('/api/v1/certificates');if(t!==null){document.getElementById('login-overlay').style.display='none';await loadAll();}else{token='';localStorage.removeItem('clm_token');}}setInterval(checkHealth,30000);setInterval(function(){if(token)loadAll();},60000);}
init();
