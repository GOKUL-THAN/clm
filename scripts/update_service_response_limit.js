const fs = require('fs');
const path = require('path');

const filePath = path.join(__dirname, '..', 'components/saas-control-plane/internal/saas/integrations/service.go');
let content = fs.readFileSync(filePath, 'utf8');

const isCRLF = content.includes('\r\n');
const eol = isCRLF ? '\r\n' : '\n';

// Add io import if not present
if (!content.includes('"io"')) {
    content = content.replace('"fmt"' + eol, '"fmt"' + eol + '\t"io"' + eol);
}

// Add io.LimitReader before resp.Body.Close()
const oldClose = [
    '\t\trecord.StatusCode = resp.StatusCode',
    '\t\trecord.Success = (resp.StatusCode >= 200 && resp.StatusCode < 300)',
    '\t\t_ = resp.Body.Close()'
].join(eol);

const newClose = [
    '\t\trecord.StatusCode = resp.StatusCode',
    '\t\trecord.Success = (resp.StatusCode >= 200 && resp.StatusCode < 300)',
    '\t\t// Drain up to 1MB of response body to permit HTTP connection reuse while bounding memory',
    '\t\t_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))',
    '\t\t_ = resp.Body.Close()'
].join(eol);

content = content.replace(oldClose, newClose);

fs.writeFileSync(filePath, content, 'utf8');
console.log('Successfully added response size limit to service.go');

