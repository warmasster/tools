// Servidor de pruebas de TurboDL (Node, sin dependencias).
//
// Sirve un archivo grande generado al vuelo (no ocupa disco) con soporte de Range,
// ETag/Last-Modified e If-Range, y límite de velocidad POR CONEXIÓN para que se note
// la mejora de descargar en paralelo.
//
//   node dev/test-server.js [opciones]
//
//   --port 8077            puerto
//   --size 2GB             tamaño del archivo /file.bin (admite KB, MB, GB)
//   --rate 2MB             límite por conexión en bytes/s (0 = sin límite)
//   --no-range             ignorar Range (responde siempre 200 con el archivo entero)
//   --fail-rate 0.01       probabilidad de cortar la conexión en cada bloque de 64 KB
//   --change-etag-after 30 a los N segundos el archivo cambia (nuevo ETag y contenido)
//   --verbose              una línea por petición
//
// Rutas: /  (página con enlaces de prueba) · /file.bin · /small.bin · /html-download
//        /login · /protected.bin (pide cookie y Referer) · /__hash · /__stats · /__bump

import http from 'node:http';
import crypto from 'node:crypto';
import { pathToFileURL } from 'node:url';

// ---------------------------------------------------------------- opciones

function parseBytes(v) {
  const m = /^([\d.]+)\s*(k|m|g|t)?i?b?$/i.exec(String(v).trim());
  if (!m) throw new Error(`Tamaño no válido: ${v}`);
  const mult = { k: 1024, m: 1024 ** 2, g: 1024 ** 3, t: 1024 ** 4 }[(m[2] || '').toLowerCase()] || 1;
  return Math.round(Number(m[1]) * mult);
}

export function parseArgs(argv) {
  const o = { port: 8077, size: parseBytes('2GB'), rate: parseBytes('2MB'), noRange: false, failRate: 0, changeEtagAfter: 0, verbose: false };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i], next = () => argv[++i];
    if (a === '--port') o.port = Number(next());
    else if (a === '--size') o.size = parseBytes(next());
    else if (a === '--rate') o.rate = parseBytes(next());
    else if (a === '--no-range') o.noRange = true;
    else if (a === '--fail-rate') o.failRate = Number(next());
    else if (a === '--change-etag-after') o.changeEtagAfter = Number(next());
    else if (a === '--verbose') o.verbose = true;
    else if (a === '--help' || a === '-h') { console.log('Ver la cabecera de dev/test-server.js'); process.exit(0); }
    else throw new Error(`Opción desconocida: ${a}`);
  }
  return o;
}

// ---------------------------------------------------------------- contenido

// byte(o) = base[o % P] ^ (floor(o / P) & 255). P es primo y no coincide con ningún
// tamaño de bloque, así que un trozo escrito en la posición equivocada cambia el hash.
const P = 1_048_573;
const bases = new Map();
function base(version) {
  if (!bases.has(version)) {
    const seed = crypto.createHash('sha256').update(`turbodl-v${version}`).digest();
    const buf = Buffer.alloc(P);
    let s = seed.readUInt32LE(0) || 1;
    for (let i = 0; i < P; i++) { s ^= s << 13; s ^= s >>> 17; s ^= s << 5; buf[i] = s & 255; }
    bases.set(version, buf);
  }
  return bases.get(version);
}

export function fillContent(version, buf, offset) {
  const b = base(version);
  let filled = 0;
  while (filled < buf.length) {
    const pos = offset + filled;
    const idx = pos % P;
    const cyc = Math.floor(pos / P) & 255;
    const n = Math.min(P - idx, buf.length - filled);
    b.copy(buf, filled, idx, idx + n);
    if (cyc) for (let i = filled; i < filled + n; i++) buf[i] ^= cyc;
    filled += n;
  }
  return buf;
}

const hashes = new Map();
function contentHash(version, size) {
  const key = `${version}:${size}`;
  if (!hashes.has(key)) {
    const h = crypto.createHash('sha256');
    const buf = Buffer.alloc(4 * 1024 * 1024);
    for (let o = 0; o < size; o += buf.length) {
      const n = Math.min(buf.length, size - o);
      h.update(fillContent(version, n === buf.length ? buf : Buffer.alloc(n), o));
    }
    hashes.set(key, h.digest('hex'));
  }
  return hashes.get(key);
}

// ---------------------------------------------------------------- servidor

export function createServer(opts) {
  const started = Date.now();
  let bumped = 0;
  const version = () => 1 + bumped + (opts.changeEtagAfter && Date.now() - started > opts.changeEtagAfter * 1000 ? 1 : 0);
  const etagOf = (v) => `"turbodl-${v}-${opts.size}"`;
  const lastModOf = (v) => new Date(Date.UTC(2026, 0, v)).toUTCString();

  // maxActiveRange cuenta solo peticiones con Range (las de TurboDL), sin la descarga
  // original de Chrome que se cancela al tomar el control.
  const stats = { requests: 0, rangeRequests: 0, active: 0, maxActive: 0, activeRange: 0, maxActiveRange: 0, bytes: 0, failures: 0, referers: [], cookies: [] };
  const remember = (list, v) => { list.push(v || ''); if (list.length > 20) list.shift(); };

  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, `http://${req.headers.host}`);
    if (opts.verbose) console.log(req.method, url.pathname, req.headers.range || '', req.headers['if-range'] || '');
    try {
      switch (url.pathname) {
        case '/': return page(res);
        case '/file.bin': return serveFile(req, res, { size: opts.size, name: `turbodl-test-${label(opts.size)}.bin` });
        case '/small.bin': return serveFile(req, res, { size: 1024 * 1024, name: 'pequeno-1MB.bin', small: true });
        case '/html-download': return htmlDownload(res);
        case '/login': return login(res);
        case '/protected.bin': {
          const okCookie = /(?:^|;\s*)session=ok(?:;|$)/.test(req.headers.cookie || '');
          const okRef = (req.headers.referer || '').startsWith(`http://${req.headers.host}/`);
          remember(stats.referers, req.headers.referer);
          remember(stats.cookies, req.headers.cookie);
          if (!okCookie || !okRef) {
            res.writeHead(403, { 'Content-Type': 'text/html; charset=utf-8' });
            return res.end(`<h1>403</h1><p>Falta ${!okCookie ? 'la cookie de sesión' : 'el Referer'}.</p>`);
          }
          return serveFile(req, res, { size: Math.min(opts.size, 64 * 1024 * 1024), name: 'protegido.bin' });
        }
        case '/__hash': {
          const size = url.searchParams.get('size') ? Number(url.searchParams.get('size')) : opts.size;
          const v = Number(url.searchParams.get('v')) || version();
          res.writeHead(200, { 'Content-Type': 'application/json' });
          return res.end(JSON.stringify({ version: v, size, sha256: contentHash(v, size) }));
        }
        case '/__stats':
          res.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
          return res.end(JSON.stringify({ ...stats, version: version(), etag: etagOf(version()) }));
        case '/__reset':
          Object.assign(stats, { requests: 0, rangeRequests: 0, maxActive: stats.active, maxActiveRange: stats.activeRange, bytes: 0, failures: 0, referers: [], cookies: [] });
          res.writeHead(204); return res.end();
        case '/__bump':
          bumped++;
          res.writeHead(200, { 'Content-Type': 'application/json' });
          return res.end(JSON.stringify({ version: version(), etag: etagOf(version()) }));
        default:
          res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
          return res.end('No encontrado');
      }
    } catch (e) {
      if (!res.headersSent) res.writeHead(500);
      res.end(String(e));
    }
  });

  async function serveFile(req, res, { size, name, small }) {
    const v = small ? 1 : version();
    const etag = etagOf(v), lastModified = lastModOf(v);
    const headers = {
      'Content-Type': 'application/octet-stream',
      'Content-Disposition': `attachment; filename="${name}"`,
      ETag: etag,
      'Last-Modified': lastModified,
      'Cache-Control': 'no-store',
    };
    let start = 0, end = size - 1, status = 200;
    const range = req.headers.range;
    const ifRange = req.headers['if-range'];
    const ifRangeOk = !ifRange || ifRange === etag || ifRange === lastModified;
    if (!opts.noRange) {
      headers['Accept-Ranges'] = 'bytes';
      if (range && ifRangeOk) {
        const m = /^bytes=(\d*)-(\d*)$/.exec(range);
        if (m && (m[1] || m[2])) {
          if (m[1]) { start = Number(m[1]); end = m[2] ? Math.min(Number(m[2]), size - 1) : size - 1; }
          else { start = Math.max(0, size - Number(m[2])); }
          if (start > end || start >= size) {
            res.writeHead(416, { 'Content-Range': `bytes */${size}` });
            return res.end();
          }
          status = 206;
          headers['Content-Range'] = `bytes ${start}-${end}/${size}`;
          stats.rangeRequests++;
        }
      }
    }
    stats.requests++;
    headers['Content-Length'] = String(end - start + 1);
    res.writeHead(status, headers);
    if (req.method === 'HEAD') return res.end();

    stats.active++;
    stats.maxActive = Math.max(stats.maxActive, stats.active);
    if (status === 206) stats.maxActiveRange = Math.max(stats.maxActiveRange, ++stats.activeRange);
    let closed = false;
    res.on('close', () => { closed = true; if (status === 206) stats.activeRange--; });
    try {
      const CHUNK = 64 * 1024;
      const t0 = Date.now();
      let sent = 0;
      for (let pos = start; pos <= end && !closed; pos += CHUNK) {
        if (opts.failRate && !small && Math.random() < opts.failRate) {
          stats.failures++;
          res.destroy(); // corte brusco, como una conexión que se cae
          return;
        }
        const n = Math.min(CHUNK, end - pos + 1);
        const buf = fillContent(v, Buffer.allocUnsafe(n), pos);
        if (!res.write(buf)) {
          await new Promise((r) => {
            const done = () => { res.off('drain', done); res.off('close', done); r(); };
            res.on('drain', done); res.on('close', done);
          });
        }
        sent += n;
        stats.bytes += n;
        if (opts.rate > 0) {
          const ahead = (sent / opts.rate) * 1000 - (Date.now() - t0);
          if (ahead > 0) await new Promise((r) => setTimeout(r, ahead));
        }
      }
      if (!closed) res.end();
    } finally {
      stats.active--;
    }
  }

  function htmlDownload(res) {
    // Una "descarga" que en realidad es HTML (como la página de login que devuelven
    // muchos sitios): TurboDL debe dejársela a Chrome.
    const body = `<!doctype html><title>Login</title><p>Inicia sesión</p>${'<!-- relleno -->'.repeat(800_000)}`;
    res.writeHead(200, {
      'Content-Type': 'text/html; charset=utf-8',
      'Content-Disposition': 'attachment; filename="pagina.html"',
      'Accept-Ranges': 'bytes',
      'Content-Length': Buffer.byteLength(body),
    });
    res.end(body);
  }

  function login(res) {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Set-Cookie': 'session=ok; Path=/; SameSite=Lax' });
    res.end(`<!doctype html><meta charset="utf-8"><title>Sesión iniciada</title>
<p>Cookie de sesión puesta. <a id="protected" href="/protected.bin">Descargar archivo protegido</a> (necesita cookie y Referer).</p>`);
  }

  function page(res) {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(`<!doctype html><meta charset="utf-8"><title>TurboDL · servidor de pruebas</title>
<style>body{font:15px system-ui;background:#0f1115;color:#e6e8ee;max-width:760px;margin:40px auto;padding:0 16px}
a{color:#7aa7ff}li{margin:8px 0}code{background:#1c2029;padding:2px 6px;border-radius:4px}</style>
<h1>Servidor de pruebas de TurboDL</h1>
<p>Archivo: <code>${label(opts.size)}</code> · velocidad por conexión: <code>${opts.rate ? label(opts.rate) + '/s' : 'sin límite'}</code>
 · Range: <code>${opts.noRange ? 'no' : 'sí'}</code> · fallos: <code>${opts.failRate}</code>
 · cambio de ETag: <code>${opts.changeEtagAfter ? opts.changeEtagAfter + ' s' : 'no'}</code></p>
<ul>
<li><a id="file" href="/file.bin">Archivo grande</a> (lo intercepta TurboDL)</li>
<li><a id="small" href="/small.bin">Archivo de 1 MB</a> (por debajo del mínimo: Chrome normal)</li>
<li><a id="html" href="/html-download">Descarga que es una página HTML</a> (Chrome normal)</li>
<li><a id="blob" href="#" onclick="blobDl();return false">Descarga blob: de 20 MB</a> (Chrome normal)</li>
<li><a id="data" href="data:application/octet-stream;base64,VHVyYm9ETA==" download="data.bin">Descarga data:</a> (Chrome normal)</li>
<li><a href="/login">Iniciar sesión</a> y luego descargar el archivo protegido (cookie + Referer)</li>
<li><a href="/__stats">Estadísticas</a> · <a href="/__hash">SHA-256 esperado</a> · <a href="/__bump">Cambiar versión (ETag)</a></li>
</ul>
<script>
function blobDl(){const b=new Blob([new Uint8Array(20*1024*1024)]);const a=document.createElement('a');
a.href=URL.createObjectURL(b);a.download='blob-20MB.bin';document.body.appendChild(a);a.click();}
</script>`);
  }

  return server;
}

function label(n) {
  for (const [u, m] of [['GB', 1024 ** 3], ['MB', 1024 ** 2], ['KB', 1024]]) if (n >= m) return `${+(n / m).toFixed(2)}${u}`;
  return `${n}B`;
}

// Ejecutado directamente (no importado desde run-tests.js).
if (process.argv[1] && import.meta.url.endsWith(process.argv[1].replace(/\\/g, '/').split('/').pop())) {
  const opts = parseArgs(process.argv.slice(2));
  createServer(opts).listen(opts.port, () => {
    console.log(`Servidor de pruebas en http://localhost:${opts.port}/`);
    console.log(`  /file.bin: ${label(opts.size)}, ${opts.rate ? label(opts.rate) + '/s por conexión' : 'sin límite'}${opts.noRange ? ', SIN Range' : ''}${opts.failRate ? `, fallos ${opts.failRate}` : ''}${opts.changeEtagAfter ? `, cambia a los ${opts.changeEtagAfter} s` : ''}`);
  });
}
