// Pruebas de extremo a extremo de TurboDL en un Chrome de verdad (sin dependencias).
//
// Arranca Chrome con un perfil temporal, carga la extensión por el protocolo de
// DevTools (pipe + Extensions.loadUnpacked), levanta servidores de prueba en este
// mismo proceso y comprueba los casos del README con el SHA-256 del archivo final.
//
//   node dev/run-tests.js [--headed] [--size 120MB] [--rate 2MB] [--only speed,pause]
//                         [--big]   (añade la descarga de 2 GB con 6 conexiones)
//   CHROME=ruta\chrome.exe para usar otro Chrome.

import { createReadStream, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import crypto from 'node:crypto';
import { createServer } from './test-server.js';
import { Browser, CHROME, sleep } from './cdp.js';

const args = process.argv.slice(2);
const flag = (n) => args.includes(n);
const opt = (n, d) => (args.includes(n) ? args[args.indexOf(n) + 1] : d);
const MB = 1024 * 1024;
const SIZE = parseSize(opt('--size', '120MB'));
const RATE = parseSize(opt('--rate', '2MB'));
const ONLY = opt('--only', '')?.split(',').filter(Boolean);

function parseSize(v) {
  const m = /^([\d.]+)(KB|MB|GB)?$/i.exec(v);
  return Math.round(Number(m[1]) * ({ KB: 1024, MB: MB, GB: 1024 * MB }[(m[2] || '').toUpperCase()] || 1));
}

const log = (...a) => console.log(`[${new Date().toISOString().slice(11, 19)}]`, ...a);

// ================================================================= utilidades

function startServer(opts) {
  return new Promise((ok) => {
    const s = createServer({ size: SIZE, rate: RATE, noRange: false, failRate: 0, changeEtagAfter: 0, verbose: false, ...opts });
    s.listen(0, '127.0.0.1', () => ok({ server: s, base: `http://127.0.0.1:${s.address().port}` }));
  });
}

async function getJSON(url) { return (await fetch(url)).json(); }

function sha256File(path) {
  return new Promise((ok, ko) => {
    const h = crypto.createHash('sha256');
    createReadStream(path).on('data', (d) => h.update(d)).on('end', () => ok(h.digest('hex'))).on('error', ko);
  });
}

async function waitFor(fn, { timeout = 120_000, every = 250, what = 'condición' } = {}) {
  const t0 = Date.now();
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() - t0 > timeout) throw new Error(`Tiempo agotado esperando: ${what}`);
    await sleep(every);
  }
}

const state = (b) => b.evalSW('turbodl.list()');

async function knownIds(b) {
  const s = await state(b);
  return new Set([...s.downloads, ...s.history].map((x) => x.id));
}

/** Espera a que TurboDL registre una descarga nueva y devuelve su id. */
async function waitNewDownload(b, before, timeout = 15_000) {
  return waitFor(async () => {
    const s = await state(b);
    const n = [...s.downloads, ...s.history].find((x) => !before.has(x.id));
    return n?.id;
  }, { timeout, what: 'nueva descarga de TurboDL' });
}

async function bytesDone(b, id) {
  const s = await state(b);
  return s.segs.find((x) => x.id === id)?.bytesDone ?? 0;
}

async function waitStatus(b, id, statuses, timeout) {
  return waitFor(async () => {
    const s = await state(b);
    const d = s.downloads.find((x) => x.id === id);
    if (d?.status === 'error') throw new Error(`La descarga falló: ${d.error}`);
    return d && statuses.includes(d.status) ? d : null;
  }, { timeout, what: `estado ${statuses}` });
}

async function waitHistory(b, id, timeout = 600_000) {
  return waitFor(async () => {
    const s = await state(b);
    const d = s.downloads.find((x) => x.id === id);
    if (d?.status === 'error') throw new Error(`La descarga falló: ${d.error}`);
    return s.history.find((x) => x.id === id);
  }, { timeout, every: 500, what: 'descarga terminada' });
}

async function checkHash(base, path, { size, v } = {}) {
  const q = new URLSearchParams();
  if (size) q.set('size', size);
  if (v) q.set('v', v);
  const expected = (await getJSON(`${base}/__hash?${q}`)).sha256;
  const got = await sha256File(path);
  if (got !== expected) throw new Error(`Hash distinto:\n  esperado ${expected}\n  obtenido ${got}`);
  return got;
}

/** Descargas que ha gestionado Chrome directamente (no creadas por una extensión). */
const chromeDownloads = (b, urlPart) => b.evalSW(`chrome.downloads.search({}).then(l => l
  .filter(d => d.url.includes(${JSON.stringify(urlPart)}) && !d.byExtensionId)
  .map(d => ({ id: d.id, state: d.state, url: d.url, filename: d.filename, error: d.error })))`);

// ================================================================= pruebas

const results = [];
async function test(name, fn) {
  if (ONLY.length && !ONLY.includes(name)) return;
  log(`▶ ${name}`);
  const t0 = Date.now();
  try {
    const detail = await fn();
    results.push({ name, ok: true, detail, secs: (Date.now() - t0) / 1000 });
    log(`✔ ${name}${detail ? ` — ${detail}` : ''}`);
  } catch (e) {
    results.push({ name, ok: false, detail: e.message, secs: (Date.now() - t0) / 1000 });
    log(`✘ ${name} — ${e.stack || e.message}`);
  }
}

async function timedDownload(b, base, connections) {
  await b.evalSW(`turbodl.setSettings({ connections: ${connections} })`);
  const before = await knownIds(b);
  await b.openAndClick(`${base}/file.bin`);
  const id = await waitNewDownload(b, before);
  const t0 = Date.now();
  const rec0 = (await state(b)).downloads.find((x) => x.id === id);
  if (rec0 && rec0.backend !== 'browser') throw new Error(`Se esperaba el motor del navegador y se usó ${rec0.backend}`);
  // Fin del trabajo de red: el motor marca "ready" y pasa a la entrega.
  await waitFor(async () => {
    const s = await state(b);
    const d = s.downloads.find((x) => x.id === id);
    if (d?.status === 'error') throw new Error(d.error);
    return !d || ['ready', 'delivering'].includes(d.status);
  }, { timeout: 30 * 60_000, every: 200, what: 'fin de la descarga' });
  const secs = (Date.now() - t0) / 1000;
  const h = await waitHistory(b, id);
  await checkHash(base, h.path);
  if (!/^turbodl-test-[\w.]+?( \(\d+\))?\.bin$/.test(h.filename)) throw new Error(`Nombre final inesperado: ${h.filename}`);
  return { id, secs, path: h.path };
}

function attachConsole(b) {
  // Lo que el service worker escribe en consola (p. ej. por qué deja una descarga a Chrome).
  b.onConsole = (type, text) => { if (type !== 'log' && type !== 'debug') log(`  [sw:${type}] ${text}`); };
  return b;
}

async function main() {
  if (!CHROME) throw new Error('No encuentro Chrome; usa la variable CHROME');
  const profile = mkdtempSync(join(tmpdir(), 'turbodl-test-'));
  const downloadsDir = join(profile, 'Descargas');
  mkdirSync(join(profile, 'Default'), { recursive: true });
  mkdirSync(downloadsDir);
  writeFileSync(join(profile, 'Default', 'Preferences'), JSON.stringify({
    download: { default_directory: downloadsDir, prompt_for_download: false, directory_upgrade: true },
    savefile: { default_directory: downloadsDir },
  }));
  log(`Chrome: ${CHROME}`);
  log(`Perfil: ${profile} · archivo ${SIZE / MB} MB · ${RATE / MB} MB/s por conexión`);

  const A = await startServer({}); // normal
  let b = attachConsole(new Browser(profile, { headed: flag('--headed') }));
  await b.launch();
  const extId = await b.loadExtension();
  log(`Extensión cargada: ${extId}`);
  // Las pruebas de siempre son del motor del navegador; las "native-*" activan el nativo.
  // onInstalled guarda los ajustes por defecto al cargar: esperar a que lo haga para
  // que no pise el nuestro, y comprobarlo.
  // nativeDir también aquí: si algo se colara por el motor nativo, que no acabe en
  // la carpeta Descargas real del usuario.
  const nativeDir = join(profile, 'Nativo');
  mkdirSync(nativeDir);
  await sleep(1000);
  await b.evalSW(`turbodl.setSettings({ engine: 'browser', nativeDir: ${JSON.stringify(nativeDir)} })`);
  if ((await b.evalSW('turbodl.getSettings()')).engine !== 'browser') throw new Error('No se pudo fijar el motor del navegador');
  const nativeInfo = await b.evalSW('turbodl.native()');
  log(nativeInfo ? `Motor nativo ${nativeInfo.version} detectado` : 'Motor nativo no instalado: se saltan las pruebas native-*');
  let browserSixSecs = null;

  await test('speed', async () => {
    const one = await timedDownload(b, A.base, 1);
    const six = await timedDownload(b, A.base, 6);
    const ratio = one.secs / six.secs;
    browserSixSecs = six.secs;
    const msg = `1 conexión ${one.secs.toFixed(1)} s · 6 conexiones ${six.secs.toFixed(1)} s · ${ratio.toFixed(1)}x · hash OK`;
    if (ratio < 4) throw new Error(`Mejora insuficiente: ${msg}`);
    return msg;
  });

  await test('pause', async () => {
    await b.evalSW('turbodl.setSettings({ connections: 6 })');
    const before = await knownIds(b);
    await b.openAndClick(`${A.base}/file.bin`);
    const id = await waitNewDownload(b, before);
    await waitFor(async () => (await bytesDone(b, id)) > SIZE * 0.3, { what: '30 %' });
    await b.evalSW(`turbodl.pause(${id})`);
    await waitStatus(b, id, ['paused'], 10_000);
    const p1 = await bytesDone(b, id);
    await sleep(2500);
    const p2 = await bytesDone(b, id);
    if (p1 !== p2) throw new Error(`Siguió descargando en pausa (${p1} → ${p2})`);
    await b.evalSW(`turbodl.resume(${id})`);
    const h = await waitHistory(b, id);
    await checkHash(A.base, h.path);
    // La métrica de tiempo no cuenta la pausa (2,5 s).
    const wall = h.completedAt - h.createdAt;
    if (!(h.activeMs > 0) || h.activeMs > wall - 2000) throw new Error(`Tiempo activo ${h.activeMs} ms con ${wall} ms en total`);
    return `pausado en ${(p1 / MB).toFixed(1)} MB, reanudado, hash OK, ${(h.activeMs / 1000).toFixed(1)} s activos de ${(wall / 1000).toFixed(1)} s`;
  });

  await test('restart', async () => {
    const before = await knownIds(b);
    await b.openAndClick(`${A.base}/file.bin`);
    const id = await waitNewDownload(b, before);
    await waitFor(async () => (await bytesDone(b, id)) > SIZE * 0.3, { what: '30 %' });
    const at = await bytesDone(b, id);
    log(`  cerrando Chrome con ${(at / MB).toFixed(1)} MB`);
    await b.close();
    b = attachConsole(new Browser(profile, { headed: flag('--headed') }));
    await b.launch();
    // Las extensiones cargadas por CDP no siempre persisten: si no está, volver a cargarla.
    b.extId = extId;
    try { await b.sw(); } catch { await b.loadExtension(); }
    await sleep(1000);
    const s = await state(b);
    const d = s.downloads.find((x) => x.id === id);
    const resumedFrom = s.segs.find((x) => x.id === id)?.bytesDone ?? 0;
    if (!d) throw new Error('La descarga desapareció tras reiniciar');
    const h = await waitHistory(b, id);
    await checkHash(A.base, h.path);
    return `cerrado en ${(at / MB).toFixed(1)} MB, retomado desde ${(resumedFrom / MB).toFixed(1)} MB, hash OK`;
  });

  await test('crash', async () => {
    // Como 'restart', pero matando el proceso (corte de luz, cuelgue): sin cierre ordenado.
    const before = await knownIds(b);
    await b.openAndClick(`${A.base}/file.bin`);
    const id = await waitNewDownload(b, before);
    await waitFor(async () => (await bytesDone(b, id)) > SIZE * 0.4, { what: '40 %' });
    b.proc.kill('SIGKILL');
    await b.exited;
    b = attachConsole(new Browser(profile, { headed: flag('--headed') }));
    await b.launch();
    b.extId = extId;
    try { await b.sw(); } catch { await b.loadExtension(); }
    await sleep(1000);
    const resumedFrom = (await state(b)).segs.find((x) => x.id === id)?.bytesDone ?? 0;
    const h = await waitHistory(b, id);
    await checkHash(A.base, h.path);
    return `proceso matado, retomado desde ${(resumedFrom / MB).toFixed(1)} MB, hash OK`;
  });

  await test('concurrent', async () => {
    // Dos descargas del mismo servidor: entre las dos no pueden pasar de 6 conexiones.
    await fetch(`${A.base}/__reset`);
    const before = await knownIds(b);
    await b.openAndClick(`${A.base}/file.bin`);
    const id1 = await waitNewDownload(b, before);
    await b.openAndClick(`${A.base}/file.bin`);
    const id2 = await waitNewDownload(b, new Set([...before, id1]));
    // Conexiones que el motor tiene asignadas a ese servidor, muestreadas mientras dura.
    let maxAlloc = 0, sampling = true;
    const sampler = (async () => {
      while (sampling) {
        const jobs = await b.evalSW('turbodl.jobs()').catch(() => []);
        maxAlloc = Math.max(maxAlloc, jobs.reduce((n, j) => n + j.alloc, 0));
        await sleep(150);
      }
    })();
    const [h1, h2] = await Promise.all([waitHistory(b, id1), waitHistory(b, id2)]).finally(() => { sampling = false; });
    await sampler;
    await checkHash(A.base, h1.path);
    await checkHash(A.base, h2.path);
    const st = await getJSON(`${A.base}/__stats`);
    if (maxAlloc > 6) throw new Error(`El motor asignó ${maxAlloc} conexiones al mismo servidor`);
    if (h1.path === h2.path) throw new Error('Las dos entregas tienen el mismo nombre (falta uniquify)');
    // El servidor puede ver alguna más un instante: Chrome tarda en cerrar el socket de
    // una petición abortada mientras ya abre la siguiente.
    return `máximo ${maxAlloc} conexiones asignadas (servidor vio ${st.maxActiveRange}), nombres distintos, hashes OK`;
  });

  await test('norange', async () => {
    const N = await startServer({ noRange: true, rate: 0, size: 24 * MB });
    try {
      // a) Interceptación: sin Range se la queda Chrome.
      const before = await knownIds(b);
      await b.openAndClick(`${N.base}/file.bin`);
      const dl = await waitFor(async () => (await chromeDownloads(b, N.base)).find((d) => d.state === 'complete'), { what: 'descarga normal de Chrome' });
      if ((await knownIds(b)).size !== before.size) throw new Error('TurboDL interceptó un servidor sin Range');
      await checkHash(N.base, dl.filename);
      // b) Forzada (menú contextual): una conexión en streaming.
      const id = await b.evalSW(`turbodl.addUrl(${JSON.stringify(`${N.base}/file.bin`)})`);
      const rec = (await state(b)).downloads.find((x) => x.id === id);
      if (rec && rec.mode !== 'single') throw new Error(`Modo ${rec.mode}, se esperaba single`);
      const h = await waitHistory(b, id);
      await checkHash(N.base, h.path);
      return 'interceptación → Chrome normal; forzada → 1 conexión; hashes OK';
    } finally { N.server.close(); }
  });

  await test('failures', async () => {
    const F = await startServer({ failRate: 0.01, rate: 4 * MB });
    try {
      await b.evalSW('turbodl.setSettings({ connections: 6 })');
      const before = await knownIds(b);
      await b.openAndClick(`${F.base}/file.bin`);
      const id = await waitNewDownload(b, before);
      const h = await waitHistory(b, id);
      await checkHash(F.base, h.path);
      const st = await getJSON(`${F.base}/__stats`);
      if (!st.failures) throw new Error('El servidor no llegó a cortar ninguna conexión');
      return `${st.failures} cortes simulados, terminado con hash OK`;
    } finally { F.server.close(); }
  });

  await test('etag', async () => {
    const E = await startServer({ size: 40 * MB });
    try {
      const before = await knownIds(b);
      await b.openAndClick(`${E.base}/file.bin`);
      const id = await waitNewDownload(b, before);
      await waitFor(async () => (await bytesDone(b, id)) > 40 * MB * 0.3, { what: '30 %' });
      await b.evalSW(`turbodl.pause(${id})`);
      await getJSON(`${E.base}/__bump`); // el archivo cambia en el servidor
      await b.evalSW(`turbodl.resume(${id})`);
      await waitFor(async () => (await state(b)).downloads.find((x) => x.id === id)?.notice?.includes('cambiado')
        || (await state(b)).history.some((x) => x.id === id), { what: 'aviso de archivo cambiado' });
      const h = await waitHistory(b, id);
      await checkHash(E.base, h.path, { v: 2 });
      return 'If-Range detectó el cambio, reinició y el hash es el de la versión nueva';
    } finally { E.server.close(); }
  });

  await test('cookies', async () => {
    const before = await knownIds(b);
    await fetch(`${A.base}/__reset`);
    await b.openAndClick(`${A.base}/login`, '#protected');
    const id = await waitNewDownload(b, before);
    const rec = (await state(b)).downloads.find((x) => x.id === id);
    const h = await waitHistory(b, id);
    await checkHash(A.base, h.path, { size: Math.min(SIZE, 64 * MB) });
    const st = await getJSON(`${A.base}/__stats`);
    const how = rec?.useCookies ? 'cookie puesta por regla DNR' : 'cookie con credentials:include';
    return `Referer por regla DNR (${st.referers.at(-1)}), ${how}, hash OK`;
  });

  if (nativeInfo) {
    const useNative = (conns = 16) => b.evalSW(`turbodl.setSettings({ engine: 'auto', nativeConnections: ${conns}, nativeDir: ${JSON.stringify(nativeDir)} })`);

    await test('native-speed', async () => {
      await useNative(16);
      const before = await knownIds(b);
      await b.openAndClick(`${A.base}/file.bin`);
      const id = await waitNewDownload(b, before);
      const t0 = Date.now();
      const rec = (await state(b)).downloads.find((x) => x.id === id);
      if (rec?.backend !== 'native') throw new Error(`Se usó el motor ${rec?.backend}`);
      const h = await waitHistory(b, id);
      const secs = (Date.now() - t0) / 1000;
      await checkHash(A.base, h.path);
      if (!h.path.startsWith(nativeDir)) throw new Error(`Guardado fuera de la carpeta pedida: ${h.path}`);
      const vs = browserSixSecs ? ` · navegador (6) ${browserSixSecs.toFixed(1)} s → ${(browserSixSecs / secs).toFixed(1)}x` : '';
      return `16 conexiones nativas ${secs.toFixed(1)} s${vs} · hash OK`;
    });

    await test('native-pause', async () => {
      await useNative(2); // lento a propósito: tiene que dar tiempo a pausar a mitad
      const before = await knownIds(b);
      await b.openAndClick(`${A.base}/file.bin`);
      const id = await waitNewDownload(b, before);
      await waitFor(async () => (await bytesDone(b, id)) > SIZE * 0.3, { what: '30 %' });
      await b.evalSW(`turbodl.pause(${id})`);
      await waitStatus(b, id, ['paused'], 15_000);
      await sleep(1500);
      await b.evalSW(`turbodl.resume(${id})`);
      const h = await waitHistory(b, id);
      await checkHash(A.base, h.path);
      return 'pausado y reanudado con el motor nativo, hash OK';
    });

    await test('native-restart', async () => {
      await useNative(2); // lento a propósito: tiene que dar tiempo a cerrar Chrome a mitad
      const before = await knownIds(b);
      await b.openAndClick(`${A.base}/file.bin`);
      const id = await waitNewDownload(b, before);
      await waitFor(async () => (await bytesDone(b, id)) > SIZE * 0.3, { what: '30 %' });
      const at = await bytesDone(b, id);
      await b.close(); // el host recibe EOF, pausa y guarda el estado
      b = attachConsole(new Browser(profile, { headed: flag('--headed') }));
      await b.launch();
      b.extId = extId;
      try { await b.sw(); } catch { await b.loadExtension(); }
      const h = await waitHistory(b, id);
      await checkHash(A.base, h.path);
      return `Chrome cerrado con ${(at / MB).toFixed(1)} MB, el motor nativo siguió desde ahí, hash OK`;
    });

    await test('native-cookies', async () => {
      await useNative(16);
      await fetch(`${A.base}/__reset`);
      const before = await knownIds(b);
      await b.openAndClick(`${A.base}/login`, '#protected');
      const id = await waitNewDownload(b, before);
      const h = await waitHistory(b, id);
      await checkHash(A.base, h.path, { size: Math.min(SIZE, 64 * MB) });
      return 'Cookie y Referer pasados al motor nativo, hash OK';
    });

    await b.evalSW("turbodl.setSettings({ engine: 'browser' })");
  }

  await test('skip', async () => {
    const before = await knownIds(b);
    const out = [];
    // Pequeño (< mínimo), HTML, blob: y data:
    await b.openAndClick(`${A.base}/small.bin`);
    await waitFor(async () => (await chromeDownloads(b, '/small.bin')).find((d) => d.state === 'complete'), { what: 'small.bin por Chrome' });
    out.push('pequeño');
    await b.openAndClick(`${A.base}/`, '#html');
    await waitFor(async () => (await chromeDownloads(b, '/html-download')).find((d) => d.state === 'complete'), { what: 'HTML por Chrome' });
    out.push('HTML');
    await b.openAndClick(`${A.base}/`, '#blob');
    await waitFor(async () => (await chromeDownloads(b, 'blob:')).find((d) => d.state === 'complete'), { what: 'blob por Chrome' });
    out.push('blob:');
    await b.openAndClick(`${A.base}/`, '#data');
    await waitFor(async () => (await chromeDownloads(b, 'data:')).find((d) => d.state === 'complete'), { what: 'data: por Chrome' });
    out.push('data:');
    await sleep(500);
    if ((await knownIds(b)).size !== before.size) throw new Error('TurboDL interceptó algo que debía ser de Chrome');
    return `${out.join(', ')} → Chrome normal`;
  });

  await test('noloop', async () => {
    // La entrega final la crea la extensión: no debe aparecer como descarga interceptada
    // ni generar registros nuevos de TurboDL.
    const s = await state(b);
    const all = await b.evalSW('chrome.downloads.search({}).then(l => l.map(d => ({ url: d.url, by: d.byExtensionId || null, state: d.state })))');
    const ours = all.filter((d) => d.by === extId);
    if (!ours.length) throw new Error('No hay entregas de la extensión que comprobar');
    if ([...s.downloads, ...s.history].some((x) => /^blob:/.test(x.url))) throw new Error('Se interceptó un blob de la entrega');
    if (s.downloads.length) throw new Error(`Quedan descargas activas: ${JSON.stringify(s.downloads.map((d) => d.status))}`);
    return `${ours.length} entregas por blob, ninguna reinterceptada`;
  });

  if (flag('--big')) {
    await test('big', async () => {
      const G = await startServer({ size: 2 * 1024 * MB, rate: RATE });
      try {
        const r = await timedDownload(b, G.base, 6);
        const mbps = (2 * 1024) / r.secs;
        return `2 GB en ${r.secs.toFixed(0)} s (${mbps.toFixed(1)} MB/s, límite ${((6 * RATE) / MB).toFixed(0)} MB/s), hash OK`;
      } finally { G.server.close(); }
    });
  }

  const rules = await b.evalSW('turbodl.rules()');
  await test('cleanup', async () => {
    if (rules.length) throw new Error(`Quedan ${rules.length} reglas de red`);
    const files = await b.evalSW(`(async () => { const r = await navigator.storage.getDirectory(); const n = []; for await (const k of r.keys()) n.push(k); return n; })()`);
    if (files.length) throw new Error(`Quedan archivos en OPFS: ${files}`);
    return 'sin reglas de red ni archivos temporales';
  });

  await b.close();
  A.server.close();
  console.log('\n──────── Resultado');
  for (const r of results) console.log(`${r.ok ? '✔' : '✘'} ${r.name.padEnd(9)} ${r.secs.toFixed(0).padStart(4)} s  ${r.detail || ''}`);
  const failed = results.filter((r) => !r.ok).length;
  if (!flag('--keep')) rmSync(profile, { recursive: true, force: true });
  process.exit(failed ? 1 : 0);
}

main().catch((e) => { console.error(e); process.exit(1); });
