// Pruebas de extremo a extremo de TurboVAULT con un Chrome de verdad (sin dependencias).
//
// Compila TurboVAULT con la etiqueta e2e, lo arranca con una carpeta y una
// tubería de prueba (no toca tu bóveda), registra el puente para Chrome solo
// durante la prueba, abre Chrome con un perfil temporal y la extensión, y
// comprueba vincular, rellenar, guardar y que una web falsa no reciba nada.
//
//   node dev/run-tests.js [--headed] [--keep] [--nobuild]

import { execFileSync, spawn } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync, existsSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Browser, sleep } from './cdp.js';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const EXT = join(ROOT, 'extension');
const EXT_ID = 'pchkjmmbookdihnaekobdmkmimknihbo';
const args = process.argv.slice(2);
const flag = (n) => args.includes(n);
const log = (...a) => console.log(`[${new Date().toISOString().slice(11, 19)}]`, ...a);
const MASTER = 'prueba-de-extremo-a-extremo-77';
const REG = 'HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\com.turbovault.native';

let failures = 0;
function check(cond, what) {
  if (cond) log('  ok', what);
  else { failures++; log('  FALLO', what); }
}

// ---------- compilar ----------
const exe = join(ROOT, 'build', 'bin', 'TurboVAULT-e2e.exe');
if (!flag('--nobuild') || !existsSync(exe)) {
  log('Compilando TurboVAULT (e2e)…');
  execFileSync('powershell', ['-NoProfile', '-Command',
    `$env:Path = "$env:LOCALAPPDATA\\Programs\\go\\bin;$env:USERPROFILE\\go\\bin;$env:Path"; Set-Location '${ROOT}'; wails build -tags e2e -skipbindings -o TurboVAULT-e2e.exe | Out-Null; if ($LASTEXITCODE) { exit 1 }`], { stdio: 'inherit' });
}

// ---------- entorno aislado ----------
const tmp = mkdtempSync(join(tmpdir(), 'tv-e2e-'));
const PORT = 47000 + Math.floor(Math.random() * 1000);
const CTRL_PORT = 9555 + Math.floor(Math.random() * 300);
process.env.TURBOVAULT_DIR = join(tmp, 'datos');
process.env.TURBOVAULT_PIPE = 'e2e' + process.pid;
const E2E_TOKEN = [...crypto.getRandomValues(new Uint8Array(16))].map((b) => b.toString(16).padStart(2, '0')).join('');
const uiEnv = { ...process.env, TURBOVAULT_E2E_PORT: String(CTRL_PORT), TURBOVAULT_E2E_TOKEN: E2E_TOKEN };

let prevReg = null;
try { prevReg = execFileSync('reg', ['query', REG, '/ve'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).match(/REG_SZ\s+(.+)/)?.[1]?.trim() || null; } catch { /* no había */ }
const manifest = join(tmp, 'host.json');
writeFileSync(manifest, JSON.stringify({ name: 'com.turbovault.native', description: 'prueba', path: exe, type: 'stdio', allowed_origins: [`chrome-extension://${EXT_ID}/`] }));
execFileSync('reg', ['add', REG, '/ve', '/t', 'REG_SZ', '/d', manifest, '/f'], { stdio: 'ignore' });

function restoreReg() {
  try {
    if (prevReg) execFileSync('reg', ['add', REG, '/ve', '/t', 'REG_SZ', '/d', prevReg, '/f'], { stdio: 'ignore' });
    else execFileSync('reg', ['delete', REG, '/f'], { stdio: 'ignore' });
  } catch { /* nada */ }
}

// ---------- servidor de páginas ----------
const page = (body) => `<!doctype html><meta charset="utf-8"><title>prueba</title><body style="font:16px sans-serif;padding:40px">${body}</body>`;
const server = createServer((req, res) => {
  const host = (req.headers.host || '').split(':')[0];
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  if (req.url.startsWith('/login')) {
    return res.end(page(`<h1>${host}</h1><form action="/ok" method="post">
      <input id="user" name="username" autocomplete="username" style="width:300px;height:36px"><br><br>
      <input id="pw" name="password" type="password" autocomplete="current-password" style="width:300px;height:36px"><br><br>
      <button id="go" type="submit">Iniciar sesión</button></form>`));
  }
  if (req.url.startsWith('/ok')) return res.end(page('<h1>Dentro</h1>'));
  res.end(page('hola'));
});
await new Promise((r) => server.listen(PORT, '127.0.0.1', r));
const url = (host, path) => `http://${host}:${PORT}${path}`;

// ---------- manejar la ventana de TurboVAULT (servidor de pruebas de la compilación e2e) ----------
const uiCall = async (cmd, a) => {
  const res = await fetch(`http://127.0.0.1:${CTRL_PORT}/`, { method: 'POST', headers: { 'X-Token': E2E_TOKEN }, body: JSON.stringify({ cmd, args: a ?? null }) });
  const r = await res.json();
  if (!r.ok) throw Object.assign(new Error(r.error), { code: r.code });
  return r.data ?? null;
};
const uiJS = async (js) => {
  // La interfaz puede tardar un poco en cargar: se reintenta unos segundos.
  for (let i = 0; ; i++) {
    const res = await fetch(`http://127.0.0.1:${CTRL_PORT}/ui?js=${encodeURIComponent(js)}`, { headers: { 'X-Token': E2E_TOKEN } });
    const text = await res.text();
    let r = null;
    try { r = JSON.parse(text); } catch { /* aún no responde */ }
    if (r && r.ok) return r.v;
    if (i >= 20) throw new Error(r ? r.e : text);
    await sleep(500);
  }
};
let ui = true;
async function uiConnect() {
  for (let i = 0; i < 100; i++) {
    try { await uiCall('state'); return; } catch { await sleep(200); }
  }
  throw new Error('No aparece la ventana de TurboVAULT');
}

let chrome = null;
let uiProc = null;
function killAll() {
  try { execFileSync('powershell', ['-NoProfile', '-Command', 'Get-Process TurboVAULT-e2e -ErrorAction SilentlyContinue | Stop-Process -Force'], { stdio: 'ignore' }); } catch { /* nada */ }
}
killAll();

async function cleanup() {
  try { if (chrome) await chrome.close(); } catch { /* nada */ }
  try { if (ui) await uiCall('quit'); } catch { /* nada */ }
  try { if (uiProc) uiProc.kill(); } catch { /* nada */ }
  server.close();
  await sleep(300);
  killAll();
  restoreReg();
  await sleep(500);
  if (!flag('--keep')) { try { rmSync(tmp, { recursive: true, force: true, maxRetries: 40, retryDelay: 500 }); } catch (e) { log('no se pudo borrar', tmp, e.code); } }
}

try {
  log('Abriendo TurboVAULT…');
  uiProc = spawn(exe, [], { env: uiEnv, stdio: 'ignore' });
  await uiConnect();

  log('Ventana');
  await sleep(800);
  const welcome = await uiJS(`document.querySelector('#lock').innerText`);
  check(/Crear mi bóveda/.test(welcome), 'la ventana de verdad enseña la bienvenida (la interfaz funciona con su CSP)');

  log('Bóveda nueva');
  await uiCall('create', { master: MASTER });
  const st = await uiCall('state');
  check(st.unlocked && st.exists, 'creada y abierta');
  const saved = await uiCall('save', { item: { title: 'Web de prueba', urls: [url('login.test', '/')], username: 'fernando' }, secret: { password: 'Clave-De-Prueba-91!' } });
  check(!!saved.id, 'entrada guardada');
  await sleep(600);
  const listText = await uiJS(`(document.querySelector('#lock').hidden ? 'abierta ' : 'cerrada ') + document.querySelector('#list').innerText`);
  check(/^abierta/.test(listText) && /Web de prueba/.test(listText), 'la ventana enseña la bóveda abierta con la entrada');

  log('Chrome con la extensión');
  chrome = new Browser(join(tmp, 'perfil'), { headed: flag('--headed'), args: ['--host-resolver-rules=MAP *.test 127.0.0.1'] });
  await chrome.launch();
  const id = await chrome.loadExtension(EXT);
  check(id === EXT_ID, `ID de la extensión fijo (${id})`);

  const sw = (e) => chrome.evalSW(e);
  let s = null;
  for (let i = 0; i < 50; i++) { s = await sw('(async () => { await connect(); return publicState(); })()'); if (s.agent) break; await sleep(200); }
  check(s && s.agent && s.unlocked, 'la extensión habla con TurboVAULT (bóveda abierta)');
  check(s && !s.paired, 'sin vincular al principio');

  const deny = await sw(`call('matches', { url: '${url('login.test', '/login')}' }).then(() => 'dio datos', (e) => e.code)`);
  check(deny === 'unpaired', 'sin vincular no da nada');
  const denied2 = await sw(`call('items', {}).then(() => 'dio datos', (e) => e.code)`);
  check(denied2 === 'denied', 'el navegador no puede pedir la lista completa');

  log('Vincular');
  const code = await sw('pair()');
  const st2 = await uiCall('state');
  check(st2.pair && st2.pair.code === code, `el código coincide (${code})`);
  await uiCall('approvePair', { code });
  for (let i = 0; i < 25; i++) { s = await sw('publicState()'); if (s.paired) break; await sleep(200); }
  check(s.paired, 'vinculado');

  log('Rellenar');
  const tgt = await chrome.send('Target.createTarget', { url: url('login.test', '/login') });
  const { sessionId } = await chrome.send('Target.attachToTarget', { targetId: tgt.targetId, flatten: true });
  const ev = async (expr) => (await chrome.send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true }, sessionId)).result.value;
  for (let i = 0; i < 50; i++) { if (await ev(`document.readyState === 'complete' && !!document.querySelector('#pw')`)) break; await sleep(100); }
  await sleep(300);
  await chrome.send('DOM.enable', {}, sessionId);
  async function shadowQuery(selector, tries = 40) {
    for (let i = 0; i < tries; i++) {
      const { root } = await chrome.send('DOM.getDocument', { depth: -1, pierce: true }, sessionId);
      const host = findNode(root, (n) => n.nodeName === 'TURBOVAULT-UI');
      if (host && host.shadowRoots && host.shadowRoots[0]) {
        const { nodeId } = await chrome.send('DOM.querySelector', { nodeId: host.shadowRoots[0].nodeId, selector }, sessionId);
        if (nodeId) {
          try {
            const { model } = await chrome.send('DOM.getBoxModel', { nodeId }, sessionId);
            if (model && model.width > 0) return model;
          } catch { /* aún no se ve */ }
        }
      }
      await sleep(150);
    }
    return null;
  }
  function findNode(n, f) {
    if (f(n)) return n;
    for (const c of [...(n.children || []), ...(n.shadowRoots || [])]) { const r = findNode(c, f); if (r) return r; }
    return null;
  }
  async function clickModel(m) {
    const x = (m.content[0] + m.content[2]) / 2, y = (m.content[1] + m.content[5]) / 2;
    for (const type of ['mousePressed', 'mouseReleased']) await chrome.send('Input.dispatchMouseEvent', { type, x, y, button: 'left', clickCount: 1 }, sessionId);
  }
  async function clickSelector(sel) {
    const { root } = await chrome.send('DOM.getDocument', { depth: -1, pierce: true }, sessionId);
    const { nodeId } = await chrome.send('DOM.querySelector', { nodeId: root.nodeId, selector: sel }, sessionId);
    const { model } = await chrome.send('DOM.getBoxModel', { nodeId }, sessionId);
    await clickModel(model);
  }
  // Foco puesto por la página (sin clic tuyo): el menú no debe abrirse solo.
  await ev(`document.querySelector('#user').focus()`);
  await sleep(700);
  check(!(await shadowQuery('.menu .it', 4)), 'si la página pone el foco ella sola, el menú no se abre');
  // Clic de verdad en el campo de la contraseña.
  await clickSelector('#pw');
  const item = await shadowQuery('.menu .it');
  check(!!item, 'el menú de TurboVAULT aparece al pulsar en el campo');
  if (item) {
    // Clickjacking: la página vuelve casi invisible el menú e intenta que pulses.
    await ev(`document.querySelector('turbovault-ui').style.setProperty('opacity', '0.02', 'important')`);
    await sleep(700);
    await clickModel(item);
    await sleep(400);
    let vals = await ev(`[document.querySelector('#user').value, document.querySelector('#pw').value]`);
    check(!vals[0] && !vals[1], 'con el menú escondido por la página, el clic no rellena');
    await ev(`document.querySelector('turbovault-ui').style.removeProperty('opacity')`);
    await sleep(800);
    const again = await shadowQuery('.menu .it');
    if (again) await clickModel(again);
    await sleep(500);
    vals = await ev(`[document.querySelector('#user').value, document.querySelector('#pw').value]`);
    check(vals[0] === 'fernando' && vals[1] === 'Clave-De-Prueba-91!', `con el menú a la vista, rellena usuario y contraseña (${vals[0]})`);
  }
  const acts = await uiCall('events');
  check(acts.some((e) => e.kind === 'fill' && e.src === 'Chrome'), 'la actividad apunta el relleno desde Chrome');

  log('Web falsa');
  const evil = await sw(`call('matches', { url: '${url('login.test.evil.test', '/login')}' })`);
  check(Array.isArray(evil) && evil.length === 0, 'una web parecida no ve nada');
  const evilFill = await sw(`call('fill', { id: '${saved.id}', url: '${url('evil.test', '/login')}' }).then(() => 'relleno', (e) => e.code)`);
  check(evilFill === 'denied', 'ni pidiéndola por su ID');

  log('Guardar al iniciar sesión');
  await chrome.send('Page.enable', {}, sessionId);
  await chrome.send('Page.navigate', { url: url('nueva.test', '/login') }, sessionId);
  for (let i = 0; i < 50; i++) { if (await ev(`location.host.startsWith('nueva.test') && document.readyState === 'complete' && !!document.querySelector('#pw')`)) break; await sleep(100); }
  await sleep(300);
  await ev(`(() => { const set = (el, v) => { el.value = v; el.dispatchEvent(new Event('input', { bubbles: true })); };
    set(document.querySelector('#user'), 'ana@correo.es'); set(document.querySelector('#pw'), 'Otra-Clave-Nueva-55?'); })()`);
  await clickSelector('#go');
  for (let i = 0; i < 50; i++) { if (await ev(`location.pathname === '/ok' && document.readyState === 'complete'`).catch(() => false)) break; await sleep(100); }
  const saveBtn = await shadowQuery('.bar [data-b="save"]');
  check(!!saveBtn, 'tras enviar, ofrece guardar en la página siguiente');
  if (saveBtn) {
    await sleep(700);
    await clickModel(saveBtn);
    await sleep(800);
    const items = await uiCall('items', { trash: false });
    const nueva = items.find((x) => x.username === 'ana@correo.es');
    check(!!nueva && nueva.urls[0].includes('nueva.test'), 'guardada en la bóveda');
  }

  log('Cerrar la bóveda');
  await uiCall('lock');
  for (let i = 0; i < 25; i++) { s = await sw('publicState()'); if (!s.unlocked) break; await sleep(200); }
  check(!s.unlocked, 'la extensión se entera de que se cerró');
  const locked = await sw(`call('fill', { id: '${saved.id}', url: '${url('login.test', '/login')}' }).then(() => 'relleno', (e) => e.code)`);
  check(locked === 'locked', 'cerrada no rellena');
  await sleep(500);
  const after = await uiJS(`[!document.querySelector('#lock').hidden, document.querySelector('#list').innerText.length, document.querySelector('#detail').innerText.length]`);
  check(after[0] && after[1] === 0 && after[2] === 0, 'la ventana vuelve a pedir la maestra y borra lo que enseñaba');
} catch (e) {
  failures++;
  log('ERROR', e.stack || e.message);
} finally {
  await cleanup();
}
log(failures ? `${failures} fallos` : 'Todo bien');
process.exit(failures ? 1 : 0);
