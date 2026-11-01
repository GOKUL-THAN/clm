const fs = require('fs');
const path = require('path');

const filePath = path.join(__dirname, '..', 'components/saas-control-plane/internal/services/monitoring/monitor.go');
let content = fs.readFileSync(filePath, 'utf8');

const isCRLF = content.includes('\r\n');
const eol = isCRLF ? '\r\n' : '\n';

// Add errors import if not present
if (!content.includes('"errors"')) {
    content = content.replace('"bytes"' + eol, '"bytes"' + eol + '\t"errors"' + eol);
}

// Add httpClient field to Monitor struct
content = content.replace(
    '\tlogger   *zap.Logger' + eol + '}',
    '\tlogger     *zap.Logger' + eol + '\thttpClient *http.Client' + eol + '}'
);

// Add httpClient init in NewMonitor
content = content.replace(
    '\t\tlogger:   logger,' + eol + '\t}',
    '\t\tlogger:     logger,' + eol + '\t\thttpClient: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return errors.New("redirects prohibited") }},' + eol + '\t}'
);

// Replace http.Post with m.httpClient.Post
content = content.replace(/http\.Post\(/g, 'm.httpClient.Post(');

fs.writeFileSync(filePath, content, 'utf8');
console.log('Successfully hardened monitor.go HTTP client');

