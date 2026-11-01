// ==============================================================================
// Vercel Serverless Function API Entrypoint
// Routes all /api/*, /auth/*, and platform endpoints to dev-server.js mock engine
// ==============================================================================

const { handleMockAPI } = require('../dev-server.js');

module.exports = (req, res) => {
    // 1. Enable Global CORS Headers
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, PATCH, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Agent-Secret, X-Test-Bypass-RateLimit, X-Requested-With');

    // 2. Handle HTTP OPTIONS Preflight
    if (req.method === 'OPTIONS') {
        res.writeHead(204);
        res.end();
        return;
    }

    // 3. Normalize Request URL for Vercel Rewrites
    // Vercel routes can rewrite /api/v1/xyz to /api/index.js. Restore original path if needed.
    const originalUrl = req.headers['x-matched-path'] || req.url;
    if (originalUrl && !req.url.startsWith('/api/') && !req.url.startsWith('/auth/')) {
        req.url = originalUrl;
    }

    // 4. Handle pre-parsed body in Vercel Node runtime
    // dev-server.js listens on req.on('data') and req.on('end').
    // If Vercel has already consumed the stream, re-emit data and end events.
    if (req.body && (typeof req.body === 'object' || typeof req.body === 'string')) {
        const bodyData = typeof req.body === 'string' ? req.body : JSON.stringify(req.body);
        req.headers['content-length'] = Buffer.byteLength(bodyData).toString();

        const origOn = req.on ? req.on.bind(req) : null;
        req.on = function (event, handler) {
            if (event === 'data') {
                process.nextTick(() => {
                    try { handler(bodyData); } catch (e) { console.error('Data handler error:', e); }
                });
            } else if (event === 'end') {
                process.nextTick(() => {
                    try { handler(); } catch (e) { console.error('End handler error:', e); }
                });
            } else if (origOn) {
                origOn(event, handler);
            }
            return req;
        };
    }

    // 5. Delegate request processing to dev-server mock API engine
    try {
        const handled = handleMockAPI(req, res);
        if (!handled) {
            res.writeHead(404, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({
                error: 'Endpoint not found',
                path: req.url,
                method: req.method
            }));
        }
    } catch (err) {
        console.error('API Error in Vercel Serverless Function:', err);
        if (!res.headersSent) {
            res.writeHead(500, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({
                error: 'Internal Server Error',
                message: err.message
            }));
        }
    }
};
