// Vercel Serverless Function - CLM API Entrypoint
// Routes all /api/v1/*, /auth/*, /health requests to dev-server mock engine
const { handleMockAPI } = require('../dev-server.js');

module.exports = (req, res) => {
    // CORS
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, PATCH, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Agent-Secret, X-Test-Bypass-RateLimit, X-Requested-With');

    if (req.method === 'OPTIONS') {
        res.writeHead(204); res.end(); return;
    }

    // Restore original path from Vercel rewrite headers
    // x-matched-path contains the original request path before rewriting
    const matchedPath = req.headers['x-matched-path'];
    const nowRoute = req.headers['x-now-route-matches'];
    if (matchedPath && matchedPath !== '/api/index.js' && matchedPath !== '/api/') {
        req.url = matchedPath + (req.url.includes('?') ? req.url.substring(req.url.indexOf('?')) : '');
    } else if (req.url === '/api/index.js' || req.url === '/api/' || req.url === '/api') {
        // Try other headers
        const original = req.headers['x-invoke-path'] || req.headers['x-original-url'] || req.url;
        if (original && original !== '/api/index.js') req.url = original;
    }

    // Handle pre-parsed body (Vercel Node runtime may parse body before stream)
    if (req.body !== undefined) {
        const bodyData = typeof req.body === 'string'
            ? req.body
            : (Buffer.isBuffer(req.body) ? req.body.toString('utf8') : JSON.stringify(req.body));

        req.headers['content-length'] = Buffer.byteLength(bodyData).toString();
        const origOn = req.on ? req.on.bind(req) : null;
        req.on = function(event, handler) {
            if (event === 'data') {
                process.nextTick(() => { try { handler(bodyData); } catch(e) {} });
            } else if (event === 'end') {
                process.nextTick(() => { try { handler(); } catch(e) {} });
            } else if (origOn) {
                origOn(event, handler);
            }
            return req;
        };
    }

    // Delegate to dev-server mock engine
    try {
        const handled = handleMockAPI(req, res);
        if (!handled) {
            res.writeHead(404, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: 'Not Found', path: req.url, method: req.method }));
        }
    } catch(err) {
        console.error('[CLM API Error]', err.message, req.method, req.url);
        if (!res.headersSent) {
            res.writeHead(500, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: 'Internal Server Error', message: err.message }));
        }
    }
};