const express = require('express');
const { createProxyMiddleware } = require('http-proxy-middleware');
const path = require('path');

const app = express();
const PORT = process.env.INSPECTOR_PORT || 3457;
const TARGET = process.env.VITE_URL || 'http://localhost:5173';

// Serve inspector static files
app.get('/__inspector__/inspector.js', (req, res) => {
    res.type('application/javascript');
    res.sendFile(path.join(__dirname, 'inspector.js'));
});

app.get('/__inspector__/inspector.css', (req, res) => {
    res.type('text/css');
    res.sendFile(path.join(__dirname, 'inspector.css'));
});

// Proxy with HTML injection
app.use('/', createProxyMiddleware({
    target: TARGET,
    changeOrigin: true,
    ws: true, // Support WebSocket for Vite HMR
    selfHandleResponse: true,
    // Request uncompressed content
    onProxyReq: (proxyReq, req, res) => {
        proxyReq.setHeader('Accept-Encoding', 'identity');
    },
    onProxyRes: (proxyRes, req, res) => {
        const contentType = proxyRes.headers['content-type'] || '';

        // Copy headers (except encoding-related)
        Object.keys(proxyRes.headers).forEach(key => {
            if (key !== 'content-length' && key !== 'content-encoding' && key !== 'transfer-encoding') {
                res.setHeader(key, proxyRes.headers[key]);
            }
        });

        // Only inject into HTML
        if (contentType.includes('text/html')) {
            let chunks = [];
            proxyRes.on('data', chunk => chunks.push(chunk));
            proxyRes.on('end', () => {
                let body = Buffer.concat(chunks).toString('utf8');

                // Inject inspector scripts before </body>
                const injection = `
<link rel="stylesheet" href="/__inspector__/inspector.css">
<script src="/__inspector__/inspector.js"></script>
`;
                const modified = body.replace('</body>', injection + '</body>');
                res.end(modified);
            });
        } else {
            // Pass through non-HTML content
            proxyRes.pipe(res);
        }
    }
}));

app.listen(PORT, () => {
    console.log(`Dev Inspector running at http://localhost:${PORT}`);
    console.log(`Proxying ${TARGET}`);
    console.log('Press Ctrl+Shift+K to activate inspector');
});
