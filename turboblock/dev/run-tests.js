// Pruebas de extremo a extremo de TurboBLOCK en un Chrome de verdad (sin dependencias).
//
// Copia la extensión a una carpeta temporal, compila las listas reales (de la caché
// native/.cache) más dev/test-filters.txt, arranca Chrome con un perfil temporal
// resolviendo cualquier dominio a un servidor local y comprueba cada caso.
//
//   node dev/run-tests.js [--headed] [--only red,popups] [--keep]
//   CHROME=ruta\chrome.exe para usar otro Chrome.

import { cpSync, mkdtempSync, rmSync, existsSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { execFileSync } from 'node:child_process';
import { Browser, CHROME, EXT_DIR, sleep } from './cdp.js';

const args = process.argv.slice(2);
const flag = (n) => args.includes(n);
const opt = (n, d) => (args.includes(n) ? args[args.indexOf(n) + 1] : d);
const ONLY = opt('--only', '').split(',').filter(Boolean);
const log = (...a) => console.log(`[${new Date().toISOString().slice(11, 19)}]`, ...a);

// ---------- servidor: responde según el dominio ----------
const hits = [];
const handler = (req, res) => {
  const host = (req.headers.host || '').split(':')[0];
  hits.push(`${host}${req.url}`);
  const html = (body) => { res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' }); res.end(`<!doctype html><meta charset="utf-8">${body}`); };
  if (host === 'anuncios.test') { res.writeHead(200, { 'Content-Type': 'text/javascript' }); return res.end('window.adRan = 1;'); }
  if (host === 'site.test' && req.url === '/') {
    return html(`<title>site</title>
      <script src="http://anuncios.test:${PORT}/ad.js"></script>
      <div class="tb-especifico" id="esp">específico</div>
      <div class="caja" id="patro">Patrocinado</div><div class="caja" id="normal">Normal</div>
      <div class="estilo" id="estilo">estilo</div>
      <div class="tb-generico-prueba" id="gen">genérico</div>
      <a id="complejo" href="/x?tb-complejo">complejo</a>
      <div id="zona" style="width:300px;height:80px;background:#ccc">zona</div>
      <a id="enlace" href="http://otro.test:${PORT}/enlace" target="_blank">enlace</a>
      <button id="btn-lista">lista</button>
      <button id="btn-mala">mala</button>
      <div id="div-btn" style="display:inline-block;padding:6px 14px;cursor:pointer;background:#36f;color:#fff">Pagar</div>
      <script>
        document.getElementById('div-btn').addEventListener('click', () => window.open('http://otro.test:${PORT}/pago'));
        document.getElementById('zona').addEventListener('click', () => window.open('http://otro.test:${PORT}/popunder'));
        document.getElementById('btn-lista').addEventListener('click', () => window.open('http://popups.test:${PORT}/'));
        document.getElementById('btn-mala').addEventListener('click', () => { window.__w = window.open('http://otro.test:${PORT}/ventana-mala'); });
        setTimeout(() => { const d = document.createElement('div'); d.className = 'tb-generico-prueba'; d.id = 'tarde'; d.textContent = 'tarde'; document.body.appendChild(d);
          const p = document.createElement('div'); p.className = 'caja'; p.id = 'patro2'; p.textContent = 'Patrocinado tarde'; document.body.appendChild(p); }, 300);
      </script>`);
  }
  if (host === 'site.test' && req.url === '/enlaces') {
    return html(`<title>enlaces</title>
      <a id="hash-link" href="#">compartir</a>
      <a id="link-pop" href="http://site.test:${PORT}/destino">siguiente página</a>
      <a id="ctrl-link" href="http://otro.test:${PORT}/ctrl">otro sitio</a>
      <script>
        document.getElementById('hash-link').addEventListener('click', (e) => { e.preventDefault(); window.open('http://otro.test:${PORT}/compartir'); });
        // popunder pegada a un enlace normal: el enlace va a la misma pestaña y además se abre otra web
        document.getElementById('link-pop').addEventListener('click', (e) => { e.preventDefault(); window.open('http://otro.test:${PORT}/popunder2'); });
      </script>`);
  }
  if (host === 'video.test' && req.url === '/') {
    return html(`<title>video</title>
      <script src="${P80}/js/sdkloader/ima3.js"></script>
      <div id="movie_player" class="html5-video-player ad-showing"><video class="html5-main-video" src="/audio.wav" autoplay></video>
        <button class="ytp-ad-skip-button" onclick="window.skipped = true">Saltar</button></div>
      <script>
        window.res = {};
        const x = new XMLHttpRequest();
        x.onreadystatechange = () => { if (x.readyState === 4) window.res.xhr = x.responseText; };
        x.open('GET', '/player.json'); x.send();
        fetch('/lista.m3u8').then((r) => r.text()).then((t) => { window.res.m3u = t; });
        fetch('/vast.xml').then((r) => r.text()).then((t) => { window.res.xml = t; });
        const v = new XMLHttpRequest();
        v.onload = () => { window.res.vast = v.responseText; };
        v.onerror = () => { window.res.vast = 'error'; };
        v.open('GET', '${P80.replace('imasdk.googleapis.com', 'pubads.g.doubleclick.net')}/gampad/ads?iu=x&output=vast'); v.send();
      </script>`);
  }
  if (host === 'video.test' && req.url === '/player.json') { res.writeHead(200, { 'Content-Type': 'application/json' }); return res.end(JSON.stringify({ video: 'ok', adPlacements: [1, 2] })); }
  if (host === 'video.test' && req.url === '/lista.m3u8') { res.writeHead(200); return res.end('#EXTM3U\n#EXTINF:5,\nvideo1.ts\n#EXTINF:5,\nanuncio1.ts\n#EXTINF:5,\nvideo2.ts\n'); }
  if (host === 'video.test' && req.url === '/vast.xml') { res.writeHead(200, { 'Content-Type': 'text/xml' }); return res.end('<VAST><Ad id="1"><x/></Ad><Extra/></VAST>'); }
  if (host === 'video.test' && req.url === '/audio.wav') {
    const data = wav(6);
    const m = /bytes=(\d+)-(\d*)/.exec(req.headers.range || '');
    if (!m) { res.writeHead(200, { 'Content-Type': 'audio/wav', 'Accept-Ranges': 'bytes', 'Content-Length': data.length }); return res.end(data); }
    const start = +m[1], end = m[2] ? +m[2] : data.length - 1;
    res.writeHead(206, { 'Content-Type': 'audio/wav', 'Accept-Ranges': 'bytes', 'Content-Range': `bytes ${start}-${end}/${data.length}`, 'Content-Length': end - start + 1 });
    return res.end(data.subarray(start, end + 1));
  }
  if (host === 'imasdk.googleapis.com' || host === 'pubads.g.doubleclick.net') { res.writeHead(200); return res.end('/* de verdad */ window.realIma = true;'); }
  if (host === 'otro.test') return html('<title>otro</title><div class="tb-generico-prueba" id="gen">aquí no se oculta</div>');
  return html(`<title>${host}</title><p>${host}</p>`);
};
const server = createServer(handler);
const server80 = createServer(handler);
let PORT = 0;
let P80 = 'http://imasdk.googleapis.com';

// WAV mudo de n segundos (para tener un <video> con duración).
function wav(sec) {
  const rate = 8000, n = rate * sec;
  const b = Buffer.alloc(44 + n);
  b.write('RIFF', 0); b.writeUInt32LE(36 + n, 4); b.write('WAVEfmt ', 8); b.writeUInt32LE(16, 16); b.writeUInt16LE(1, 20); b.writeUInt16LE(1, 22);
  b.writeUInt32LE(rate, 24); b.writeUInt32LE(rate, 28); b.writeUInt16LE(1, 32); b.writeUInt16LE(8, 34); b.write('data', 36); b.writeUInt32LE(n, 40);
  b.fill(128, 44);
  return b;
}

// ---------- utilidades ----------
const results = [];
async function test(name, fn) {
  if (ONLY.length && !ONLY.includes(name)) return;
  log(`▶ ${name}`);
  const t0 = Date.now();
  try { await fn(); results.push([name, true]); log(`✔ ${name} (${((Date.now() - t0) / 1000).toFixed(1)} s)`); }
  catch (e) { results.push([name, false, e.message]); log(`✘ ${name}: ${e.message}`); }
}
function assert(c, msg) { if (!c) throw new Error(msg); }
async function waitFor(fn, what, timeout = 10_000) {
  const t0 = Date.now();
  for (;;) {
    const v = await fn().catch(() => null);
    if (v) return v;
    if (Date.now() - t0 > timeout) throw new Error(`Tiempo agotado: ${what}`);
    await sleep(150);
  }
}

class Page {
  constructor(b, targetId, sessionId) { this.b = b; this.targetId = targetId; this.s = sessionId; }
  static async open(b, url) {
    const { targetId } = await b.send('Target.createTarget', { url });
    const { sessionId } = await b.send('Target.attachToTarget', { targetId, flatten: true });
    const p = new Page(b, targetId, sessionId);
    await waitFor(() => p.eval(`document.readyState === 'complete' && location.href !== 'about:blank' && location.href`), `carga de ${url}`);
    return p;
  }
  async eval(expr) {
    const r = await this.b.send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }, this.s);
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text);
    return r.result.value;
  }
  async click(sel) {
    const rect = await this.eval(`(() => { const r = document.querySelector(${JSON.stringify(sel)}).getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; })()`);
    for (const type of ['mouseMoved', 'mousePressed', 'mouseReleased']) {
      await this.b.send('Input.dispatchMouseEvent', { type, x: rect.x, y: rect.y, button: 'left', clickCount: 1 }, this.s);
    }
  }
  hidden(sel) { return this.eval(`(() => { const e = document.querySelector(${JSON.stringify(sel)}); return !!e && getComputedStyle(e).display === 'none'; })()`); }
  style(sel, prop) { return this.eval(`getComputedStyle(document.querySelector(${JSON.stringify(sel)}))[${JSON.stringify(prop)}]`); }
  close() { return this.b.send('Target.closeTarget', { targetId: this.targetId }).catch(() => {}); }
}
const pages = async (b, part) => (await b.send('Target.getTargets')).targetInfos.filter((t) => t.type === 'page' && t.url.includes(part));

// ---------- main ----------
(async () => {
  if (!CHROME) throw new Error('No encuentro Chrome (variable CHROME)');
  await new Promise((ok) => server.listen(0, '127.0.0.1', ok));
  PORT = server.address().port;
  // Los filtros "||dominio/ruta" no admiten puerto: los de vídeo se prueban en el 80.
  await new Promise((ok) => server80.listen(80, '127.0.0.1', ok).on('error', () => { log('Puerto 80 ocupado: IMA y VAST se prueban con puerto'); P80 = `http://imasdk.googleapis.com:${PORT}`; ok(); }));

  // Extensión de prueba: copia + listas reales + filtros de prueba.
  const ext = mkdtempSync(join(tmpdir(), 'turboblock-ext-'));
  for (const f of ['manifest.json', 'background.js', 'popup.html', 'options.html', 'blocked.html', 'lib', 'content', 'ui', 'icons', 'resources', 'filters']) {
    cpSync(join(EXT_DIR, f), join(ext, f), { recursive: true });
  }
  const cache = resolve(EXT_DIR, 'native', '.cache');
  log('Compilando listas…');
  const out = execFileSync('go', ['run', '.', 'compile', '--ext', ext, '--cache', cache, '--offline',
    '--extra', `ads=${join(EXT_DIR, 'dev', 'test-filters.txt')}`, '--extra', `malware=${join(EXT_DIR, 'dev', 'test-malware.txt')}`],
  { cwd: join(EXT_DIR, 'native'), encoding: 'utf8' });
  process.stdout.write(out);

  const profile = mkdtempSync(join(tmpdir(), 'turboblock-prof-'));
  const b = new Browser(profile, { headed: flag('--headed'), args: ['--host-resolver-rules=MAP * 127.0.0.1, EXCLUDE localhost'] });
  await b.launch();
  try {
    const id = await b.loadExtension(ext);
    log('Extensión', id);
    const status = await waitFor(() => b.evalSW(`chrome.storage.local.get('status').then(r => r.status)`), 'primer apply', 30_000);
    log(`apply en ${status.ms} ms`, status.errors.length ? status.errors : '');

    await test('carga', async () => {
      assert(id === 'bdgliednephldclmchggpdnahalgdcif', `ID inesperado ${id}`);
      assert(!status.errors.length, status.errors.join('; '));
      const rs = await b.evalSW('chrome.declarativeNetRequest.getEnabledRulesets()');
      assert(['base', 'ads', 'privacy', 'malware'].every((r) => rs.includes(r)), `rulesets: ${rs}`);
      const sc = await b.evalSW('chrome.scripting.getRegisteredContentScripts().then(l => l.map(x => x.id))');
      assert(sc.includes('cosmetic') && sc.includes('popups') && sc.some((x) => x.startsWith('sl-ads-')), `scripts: ${sc.slice(0, 8)}`);
      const avail = await b.evalSW('chrome.declarativeNetRequest.getAvailableStaticRuleCount()');
      log(`  ${sc.length} scripts registrados; quedan ${avail} reglas estáticas libres`);
    });

    let site;
    await test('red', async () => {
      site = await Page.open(b, `http://site.test:${PORT}/`);
      assert(!(await site.eval('window.adRan')), 'el script de anuncios se ejecutó');
      assert(!hits.some((h) => h.startsWith('anuncios.test')), 'la petición llegó al servidor');
    });

    await test('cosmetica', async () => {
      site ||= await Page.open(b, `http://site.test:${PORT}/`);
      await waitFor(() => site.hidden('#esp'), 'ocultar .tb-especifico');
      await waitFor(() => site.hidden('#gen'), 'ocultar genérico por clase');
      await waitFor(() => site.hidden('#tarde'), 'ocultar genérico añadido después');
      await waitFor(() => site.hidden('#complejo'), 'ocultar genérico complejo (hoja fija)');
      await waitFor(() => site.hidden('#patro'), ':has-text');
      await waitFor(() => site.hidden('#patro2'), ':has-text en elemento añadido después');
      assert(!(await site.hidden('#normal')), '.caja sin el texto no debe ocultarse');
      assert((await site.style('#estilo', 'visibility')) === 'hidden', ':style() no aplicado');
      const otro = await Page.open(b, `http://otro.test:${PORT}/`);
      await sleep(500);
      assert(!(await otro.hidden('#gen')), 'la excepción #@# no funcionó');
      await otro.close();
    });

    await test('scriptlets', async () => {
      site ||= await Page.open(b, `http://site.test:${PORT}/`);
      assert((await site.eval('window.tbConstante')) === true, 'set-constant no aplicado');
      const ent = await Page.open(b, `http://www.entidad.es:${PORT}/`);
      await waitFor(() => ent.eval('window.tbEntidad === 42'), 'entidad (primera visita, inyectado al vuelo)');
      await ent.close();
      await sleep(500);
      const ent2 = await Page.open(b, `http://www.entidad.es:${PORT}/`);
      assert((await ent2.eval('window.tbEntidad')) === 42, 'entidad (segunda visita, registrado)');
      await ent2.close();
    });

    await test('popups', async () => {
      site ||= await Page.open(b, `http://site.test:${PORT}/`);
      // Clic en una zona cualquiera que abre otra web: se cierra.
      await site.click('#zona');
      await sleep(1500);
      assert((await pages(b, '/popunder')).length === 0, 'la popunder sigue abierta');
      // Enlace con target=_blank: se permite.
      await site.click('#enlace');
      await waitFor(async () => (await pages(b, '/enlace')).length === 1, 'abrir el enlace en pestaña nueva');
      await sleep(800);
      assert((await pages(b, '/enlace')).length === 1, 'se cerró una ventana legítima');
      // Botón hecho con un <div> (cursor de mano, tamaño de botón) que abre un pago: se permite.
      await site.click('#div-btn');
      await waitFor(async () => (await pages(b, '/pago')).length === 1, 'abrir desde un botón hecho con un div');
      await sleep(800);
      assert((await pages(b, '/pago')).length === 1, 'se cerró la ventana del botón hecho con un div');
      // Destino en las listas ($popup): se cierra aunque sea un botón.
      await site.click('#btn-lista');
      await sleep(1500);
      assert((await pages(b, 'popups.test')).length === 0, 'ventana de la lista abierta');
      // nowoif: window.open devuelve una ventana falsa.
      await site.click('#btn-mala');
      await sleep(800);
      assert((await pages(b, 'ventana-mala')).length === 0, 'nowoif no paró la ventana');
      // Aviso en la página.
      assert(await site.eval(`!!document.querySelector('turboblock-toast')`), 'no salió el aviso');
      const n = await b.evalSW(`chrome.storage.local.get('statsTotal').then(r => r.statsTotal.popups)`);
      assert(n >= 2, `contador de ventanas: ${n}`);
    });

    await test('enlaces', async () => {
      const p = await Page.open(b, `http://site.test:${PORT}/enlaces`);
      // Enlace "#" que abre una web con JavaScript (p. ej. «Compartir»): se permite.
      await p.click('#hash-link');
      await waitFor(async () => (await pages(b, '/compartir')).length === 1, 'abrir desde un enlace "#"');
      await sleep(700);
      assert((await pages(b, '/compartir')).length === 1, 'se cerró la ventana del enlace "#"');
      // Ctrl+clic en un enlace a otra web: se permite.
      const rect = await p.eval(`(() => { const r = document.querySelector('#ctrl-link').getBoundingClientRect(); return { x: r.x + 5, y: r.y + 5 }; })()`);
      for (const type of ['mousePressed', 'mouseReleased']) await b.send('Input.dispatchMouseEvent', { type, x: rect.x, y: rect.y, button: 'left', clickCount: 1, modifiers: 2 }, p.s);
      await waitFor(async () => (await pages(b, '/ctrl')).length === 1, 'Ctrl+clic');
      // Enlace normal con una popunder pegada: la popunder se cierra.
      await p.click('#link-pop');
      await sleep(1500);
      assert((await pages(b, '/popunder2')).length === 0, 'la popunder pegada al enlace sigue abierta');
      await p.close();
    });

    await test('video', async () => {
      const p = await Page.open(b, `http://video.test:${PORT}/`);
      assert(await p.eval('!!(window.google && google.ima && google.ima.__turboblock)'), 'no se cambió el SDK de IMA');
      assert(!(await p.eval('window.realIma')), 'se cargó el IMA de verdad');
      const r = await waitFor(() => p.eval('window.res.xhr && window.res.m3u && window.res.xml && window.res.vast && window.res'), 'respuestas', 8000);
      assert(!r.xhr.includes('adPlacements') && r.xhr.includes('"video":"ok"'), `json-prune XHR: ${r.xhr}`);
      assert(!r.m3u.includes('anuncio') && r.m3u.includes('video2.ts') && (r.m3u.match(/EXTINF/g) || []).length === 2, `m3u-prune: ${r.m3u}`);
      assert(!r.xml.includes('<Ad') && r.xml.includes('Extra'), `xml-prune: ${r.xml}`);
      assert(/<VAST version="4.0"><\/VAST>/.test(r.vast), `VAST vacío: ${r.vast}`);
      try {
        await waitFor(() => p.eval(`(() => { const v = document.querySelector('video'); return v.muted && v.duration > 0 && v.currentTime > v.duration - 1; })()`), 'saltar el anuncio de vídeo', 8000);
      } catch (e) {
        throw new Error(e.message + ' ' + await p.eval(`(() => { const v = document.querySelector('video'); return JSON.stringify({ muted: v.muted, d: v.duration, t: v.currentTime, rs: v.readyState, err: v.error && v.error.code, skipped: window.skipped }); })()`));
      }
      assert(await p.eval('window.skipped === true'), 'no pulsó «Saltar»');
      await p.eval(`document.getElementById('movie_player').classList.remove('ad-showing')`);
      await waitFor(() => p.eval(`!document.querySelector('video').muted`), 'devolver el sonido al acabar el anuncio');
      await p.close();
    });

    await test('estadisticas', async () => {
      const n = await b.evalSW(`turboblock.collectStats().then(() => chrome.storage.local.get('statsTotal')).then(r => r.statsTotal)`);
      assert(n.ads > 0, `sin anuncios contados: ${JSON.stringify(n)}`);
      const days = await b.evalSW(`chrome.storage.local.get('statsDays').then(r => r.statsDays)`);
      assert(Object.keys(days).length === 1, 'día de hoy');
    });

    await test('malware', async () => {
      const p = await Page.open(b, `http://malo.test:${PORT}/pagina`);
      const url = await p.eval('location.href');
      assert(url.startsWith(`chrome-extension://${id}/blocked.html#http://malo.test`), `no se redirigió: ${url}`);
      await p.eval(`document.getElementById('go').click()`);
      await waitFor(() => p.eval(`location.hostname === 'malo.test'`), 'entrar de todas formas');
      await p.close();
    });

    await test('web-permitida', async () => {
      await b.evalSW(`(async () => { const { settings = {} } = await chrome.storage.local.get('settings'); await chrome.storage.local.set({ settings: { ...settings, allow: ['site.test'] } }); })()`);
      await waitFor(() => b.evalSW(`chrome.storage.local.get('status').then(r => r.status.reason === 'ajustes')`), 'apply tras permitir la web');
      const p = await Page.open(b, `http://site.test:${PORT}/`);
      assert((await p.eval('window.adRan')) === 1, 'en una web permitida no debe bloquear');
      await sleep(400);
      assert(!(await p.hidden('#esp')), 'en una web permitida no debe ocultar');
      await p.close();
      await b.evalSW(`(async () => { const { settings } = await chrome.storage.local.get('settings'); await chrome.storage.local.set({ settings: { ...settings, allow: [] } }); })()`);
    });

    await test('mis-filtros', async () => {
      const before = await b.evalSW(`chrome.storage.local.get('status').then(r => r.status.applied)`);
      await b.evalSW(`(async () => { const { settings = {} } = await chrome.storage.local.get('settings'); await chrome.storage.local.set({ settings: { ...settings, allow: [], userFilters: '||otro.test^$image\\nsite.test###normal\\nmal filtro$opcioninventada' } }); })()`);
      const st = await waitFor(() => b.evalSW(`chrome.storage.local.get('status').then(r => r.status.applied > ${before} && r.status.userErrors.length > 0 && r.status)`), 'apply de mis filtros');
      assert(st.userErrors.length === 1 && st.userErrors[0].line === 3, `errores: ${JSON.stringify(st.userErrors)}`);
      const p = await Page.open(b, `http://site.test:${PORT}/`);
      await waitFor(() => p.hidden('#normal'), 'filtro propio ##');
      const rules = await b.evalSW('chrome.declarativeNetRequest.getDynamicRules()');
      assert(rules.some((r) => r.condition.urlFilter === '||otro.test^'), 'regla de red propia');
      await p.close();
    });

    await test('desactivar', async () => {
      await b.evalSW(`(async () => { const { settings } = await chrome.storage.local.get('settings'); await chrome.storage.local.set({ settings: { ...settings, enabled: false } }); })()`);
      await waitFor(async () => (await b.evalSW('chrome.declarativeNetRequest.getEnabledRulesets()')).length === 0, 'rulesets apagados');
      await waitFor(async () => (await b.evalSW('chrome.scripting.getRegisteredContentScripts()')).length === 0, 'scripts quitados');
      await b.evalSW(`(async () => { const { settings } = await chrome.storage.local.get('settings'); await chrome.storage.local.set({ settings: { ...settings, enabled: true } }); })()`);
      await waitFor(async () => (await b.evalSW('chrome.declarativeNetRequest.getEnabledRulesets()')).length >= 4, 'rulesets encendidos');
    });

    await test('interfaz', async () => {
      for (const pg of ['popup.html', 'options.html']) {
        const p = await Page.open(b, `chrome-extension://${id}/${pg}`);
        await sleep(600);
        const errs = await p.eval(`document.body.innerText.length`);
        assert(errs > 50, `${pg} vacío`);
        await p.close();
      }
    });
  } finally {
    await b.close();
    server.close();
    server80.close();
    if (!flag('--keep')) { rmSync(profile, { recursive: true, force: true }); rmSync(ext, { recursive: true, force: true }); }
  }
  const failed = results.filter((r) => !r[1]);
  console.log(`\n${results.length - failed.length}/${results.length} pruebas bien`);
  for (const f of failed) console.log(`  ✘ ${f[0]}: ${f[2]}`);
  process.exit(failed.length ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(1); });
