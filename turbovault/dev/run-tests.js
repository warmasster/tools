// Pruebas de extremo a extremo de TurboVAULT con un Chrome de verdad (sin dependencias).
//
// Compila TurboVAULT con la etiqueta e2e, registra el puente para Chrome solo
// durante la prueba y abre Chrome con un perfil temporal y la extensión. Es
// Chrome quien arranca TurboVAULT (como al encender el PC con Chrome abierto),
// con una carpeta y una tubería de prueba: no toca tu bóveda. Comprueba abrir la
// ventana, vincular, los iconos en los campos, rellenar, guardar y que una web
// falsa no reciba nada.
//
//   node dev/run-tests.js [--headed] [--keep] [--nobuild] [--shots carpeta]

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
// Todo lo que arranca después (Chrome, el puente, el agente y la ventana) hereda
// estas variables: carpeta y tubería de prueba, sin mirar los navegadores del PC.
const tmp = mkdtempSync(join(tmpdir(), 'tv-e2e-'));
const PORT = 47000 + Math.floor(Math.random() * 1000);
const CTRL_PORT = 9555 + Math.floor(Math.random() * 300);
const E2E_TOKEN = [...crypto.getRandomValues(new Uint8Array(16))].map((b) => b.toString(16).padStart(2, '0')).join('');
Object.assign(process.env, {
  TURBOVAULT_DIR: join(tmp, 'datos'),
  TURBOVAULT_PIPE: 'e2e' + process.pid,
  TURBOVAULT_BROWSERS: '',
  TURBOVAULT_E2E_PORT: String(CTRL_PORT),
  TURBOVAULT_E2E_TOKEN: E2E_TOKEN,
});

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
const loginForm = (p = '') => `<form action="/ok" method="post">
      <input id="${p}user" name="username" autocomplete="username" style="width:300px;height:36px"><br><br>
      <input id="${p}pw" name="password" type="password" autocomplete="current-password" style="width:300px;height:36px"><br><br>
      <button id="${p}go" type="submit">Iniciar sesión</button></form>`;
const server = createServer((req, res) => {
  const host = (req.headers.host || '').split(':')[0];
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  if (req.url.startsWith('/login')) return res.end(page(`<h1>${host}</h1>${loginForm()}`));
  if (req.url.startsWith('/dialog')) {
    // Inicio de sesión dentro de un diálogo modal: va por encima de todo y deja inerte lo de fuera.
    return res.end(page(`<h1>${host}</h1><p>Fondo</p><dialog id="dlg" style="padding:30px">${loginForm('d')}</dialog>
      <script>document.getElementById('dlg').showModal()</script>`));
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
// Espera (hasta ms) a que el texto de la ventana cumpla re; devuelve el último leído.
// Recién abierta, o con la pantalla apagada, la ventana tarda más en pintarse.
async function uiUntil(js, re, ms = 6000) {
  const t0 = Date.now();
  let v = '';
  do {
    v = String(await uiJS(js));
    if (re.test(v)) break;
    await sleep(250);
  } while (Date.now() - t0 < ms);
  return v;
}
let ui = false;
async function uiConnect() {
  for (let i = 0; i < 100; i++) {
    try { await uiCall('state'); return true; } catch { await sleep(200); }
  }
  return false;
}

let chrome = null;
function killAll() {
  try { execFileSync('powershell', ['-NoProfile', '-Command', 'Get-Process TurboVAULT-e2e -ErrorAction SilentlyContinue | Stop-Process -Force'], { stdio: 'ignore' }); } catch { /* nada */ }
}
function countProcs() {
  try { return Number(execFileSync('powershell', ['-NoProfile', '-Command', '@(Get-Process TurboVAULT-e2e -ErrorAction SilentlyContinue).Count'], { encoding: 'utf8' }).trim()) || 0; } catch { return 0; }
}
killAll();

async function cleanup() {
  try { if (chrome) await chrome.close(); } catch { /* nada */ }
  try { if (ui) await uiCall('quit'); } catch { /* nada */ }
  server.close();
  await sleep(300);
  killAll();
  restoreReg();
  await sleep(500);
  if (!flag('--keep')) { try { rmSync(tmp, { recursive: true, force: true, maxRetries: 40, retryDelay: 500 }); } catch (e) { log('no se pudo borrar', tmp, e.code); } }
}

try {
  log('Chrome con la extensión (arranca TurboVAULT él solo)');
  chrome = new Browser(join(tmp, 'perfil'), { headed: flag('--headed'), args: ['--host-resolver-rules=MAP *.test 127.0.0.1'] });
  await chrome.launch();
  const id = await chrome.loadExtension(EXT);
  check(id === EXT_ID, `ID de la extensión fijo (${id})`);

  const sw = (e) => chrome.evalSW(e);
  let s = null;
  for (let i = 0; i < 50; i++) { s = await sw('(async () => { await connect(true); return publicState(); })()'); if (s.agent) break; await sleep(300); }
  check(s && s.agent && !s.exists, 'el puente arranca TurboVAULT y la extensión habla con él (aún sin bóveda)');

  log('Ventana (pedida a un TurboVAULT que arrancó Chrome)');
  spawn(exe, [], { stdio: 'ignore', detached: true }).unref();
  ui = await uiConnect();
  check(ui, 'la ventana se abre (antes, un TurboVAULT arrancado por Chrome no podía abrirla)');
  if (!ui) throw new Error('No aparece la ventana de TurboVAULT');
  const welcome = await uiUntil(`document.querySelector('#lock').innerText`, /Crear mi bóveda/);
  check(/Crear mi bóveda/.test(welcome), 'la ventana de verdad enseña la bienvenida (la interfaz funciona con su CSP)');

  log('Bóveda nueva');
  await uiCall('create', { master: MASTER });
  const st = await uiCall('state');
  check(st.unlocked && st.exists, 'creada y abierta');
  const saved = await uiCall('save', { item: { title: 'Web de prueba', urls: [url('login.test', '/')], username: 'fernando' }, secret: { password: 'Clave-De-Prueba-91!' } });
  check(!!saved.id, 'entrada guardada');
  const listText = await uiUntil(`(document.querySelector('#lock').hidden ? 'abierta ' : 'cerrada ') + document.querySelector('#list').innerText`, /^abierta[\s\S]*Web de prueba/);
  check(/^abierta/.test(listText) && /Web de prueba/.test(listText), 'la ventana enseña la bóveda abierta con la entrada');

  for (let i = 0; i < 25; i++) { s = await sw('publicState()'); if (s.unlocked) break; await sleep(200); }
  check(s && s.agent && s.unlocked, 'la extensión se entera de que se abrió');
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

  log('Iconos en los campos');
  const tgt = await chrome.send('Target.createTarget', { url: url('login.test', '/login') });
  const { sessionId } = await chrome.send('Target.attachToTarget', { targetId: tgt.targetId, flatten: true });
  const ev = async (expr) => (await chrome.send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true }, sessionId)).result.value;
  for (let i = 0; i < 50; i++) { if (await ev(`document.readyState === 'complete' && !!document.querySelector('#pw')`)) break; await sleep(100); }
  await chrome.send('DOM.enable', {}, sessionId);

  function findNode(n, f) {
    if (f(n)) return n;
    for (const c of [...(n.children || []), ...(n.shadowRoots || [])]) { const r = findNode(c, f); if (r) return r; }
    return null;
  }
  // Elementos visibles de la interfaz de TurboVAULT (dentro de su shadow DOM cerrado).
  async function shadowAll(selector) {
    const { root } = await chrome.send('DOM.getDocument', { depth: -1, pierce: true }, sessionId);
    const host = findNode(root, (n) => n.nodeName === 'TURBOVAULT-UI');
    if (!host || !host.shadowRoots || !host.shadowRoots[0]) return [];
    const { nodeIds } = await chrome.send('DOM.querySelectorAll', { nodeId: host.shadowRoots[0].nodeId, selector }, sessionId);
    const out = [];
    for (const nodeId of nodeIds) {
      try {
        const { model } = await chrome.send('DOM.getBoxModel', { nodeId }, sessionId);
        if (model && model.width > 0) out.push(model);
      } catch { /* no se ve */ }
    }
    return out;
  }
  async function shadowQuery(selector, tries = 40) {
    for (let i = 0; i < tries; i++) {
      const [m] = await shadowAll(selector);
      if (m) return m;
      await sleep(150);
    }
    return null;
  }
  async function waitIcons(selector, n, tries = 30) {
    let list = [];
    for (let i = 0; i < tries; i++) { list = await shadowAll(selector); if (list.length === n) return list; await sleep(150); }
    return list;
  }
  async function clickModel(m) {
    const x = (m.content[0] + m.content[2]) / 2, y = (m.content[1] + m.content[5]) / 2;
    for (const type of ['mousePressed', 'mouseReleased']) await chrome.send('Input.dispatchMouseEvent', { type, x, y, button: 'left', clickCount: 1 }, sessionId);
  }
  async function boxOf(sel) {
    const { root } = await chrome.send('DOM.getDocument', { depth: -1, pierce: true }, sessionId);
    const { nodeId } = await chrome.send('DOM.querySelector', { nodeId: root.nodeId, selector: sel }, sessionId);
    return (await chrome.send('DOM.getBoxModel', { nodeId }, sessionId)).model;
  }
  async function clickSelector(sel) { await clickModel(await boxOf(sel)); }
  // Capturas para ver cómo queda (con --shots carpeta).
  const shotsDir = args.includes('--shots') ? args[args.indexOf('--shots') + 1] : '';
  async function shot(name) {
    if (!shotsDir) return;
    const { data } = await chrome.send('Page.captureScreenshot', { format: 'png', clip: { x: 0, y: 0, width: 520, height: 420, scale: 1 } }, sessionId);
    writeFileSync(join(shotsDir, name + '.png'), Buffer.from(data, 'base64'));
  }
  // El icono que cae dentro de un campo.
  async function iconIn(sel) {
    const f = await boxOf(sel);
    const icons = await shadowAll('.ic');
    return icons.find((m) => m.content[1] >= f.content[1] && m.content[5] <= f.content[5]) || null;
  }

  const icons = await waitIcons('.ic:not(.off)', 2);
  check(icons.length === 2, `sin pulsar nada, cada campo lleva su icono de color (${icons.length})`);
  await shot('1-iconos');
  check(!(await shadowQuery('.menu .it', 2)), 'el menú no se abre solo al cargar');

  // Foco puesto por la página (sin clic tuyo): el menú no debe abrirse solo.
  await ev(`document.querySelector('#user').focus()`);
  await sleep(700);
  check(!(await shadowQuery('.menu .it', 4)), 'si la página pone el foco ella sola, el menú no se abre');
  check(await ev(`document.querySelector('#user').getAttribute('autocomplete')`) === 'off', 'con el foco, Chrome no saca sus sugerencias (autocomplete=off)');
  await ev(`document.querySelector('#user').blur()`);
  check(await ev(`document.querySelector('#user').getAttribute('autocomplete')`) === 'username', 'al salir, el campo vuelve a estar como lo dejó la página');

  log('Un clic en el icono');
  await sleep(500);
  const userIcon = await iconIn('#user');
  check(!!userIcon, 'hay icono dentro del campo de usuario');
  if (userIcon) {
    await clickModel(userIcon);
    const it1 = await shadowQuery('.menu .it', 10);
    check(!!it1, 'con un solo clic en el icono se abre el menú');
    await shot('2-menu');
    check(await ev(`document.activeElement === document.querySelector('#user')`), 'y el cursor queda en el campo');
    await ev(`document.querySelector('#user').blur()`);
    await sleep(400);
  }

  log('Rellenar');
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

  log('Campos tapados');
  await ev(`(() => { const d = document.createElement('div'); d.id = 'tapa'; d.style.cssText = 'position:fixed;inset:0;background:#fff;z-index:99'; document.body.appendChild(d); })()`);
  const hidden = await waitIcons('.ic', 0);
  check(hidden.length === 0, 'si algo de la página tapa los campos, sus iconos no se ven');
  await ev(`document.getElementById('tapa').remove()`);
  check((await waitIcons('.ic', 2)).length === 2, 'al destaparlos vuelven');

  log('Copia nueva de la parte de la página (al recargar la extensión)');
  await sw(`chrome.tabs.query({}).then((ts) => Promise.all(ts.filter((t) => (t.url || '').includes('login.test')).map((t) => chrome.scripting.executeScript({ target: { tabId: t.id, allFrames: true }, files: ['content.js'] }))))`);
  await sleep(800);
  check(await ev(`document.querySelectorAll('turbovault-ui').length`) === 1, 'la copia anterior se retira: una sola interfaz en la página');
  check((await waitIcons('.ic', 2)).length === 2, 'y los iconos siguen ahí');

  log('Diálogo modal');
  await chrome.send('Page.enable', {}, sessionId);
  await chrome.send('Page.navigate', { url: url('login.test', '/dialog') }, sessionId);
  for (let i = 0; i < 50; i++) { if (await ev(`location.pathname === '/dialog' && document.readyState === 'complete' && !!document.querySelector('#dpw')`).catch(() => false)) break; await sleep(100); }
  const dIcons = await waitIcons('.ic', 2);
  check(dIcons.length === 2, `los campos del diálogo llevan icono (${dIcons.length})`);
  check(await ev(`!!document.querySelector('#dlg turbovault-ui')`), 'la interfaz va dentro del diálogo, por encima de todo');
  const dIcon = await iconIn('#dpw');
  if (dIcon) {
    await sleep(400);
    await clickModel(dIcon);
    const dItem = await shadowQuery('.menu .it', 10);
    check(!!dItem, 'en el diálogo, un clic en el icono abre el menú');
    if (dItem) {
      await sleep(500);
      await clickModel(dItem);
      await sleep(500);
      const dv = await ev(`[document.querySelector('#duser').value, document.querySelector('#dpw').value]`);
      check(dv[0] === 'fernando' && dv[1] === 'Clave-De-Prueba-91!', 'y rellena dentro del diálogo');
    }
  } else check(false, 'hay icono en el campo del diálogo');

  log('Web falsa');
  const evil = await sw(`call('matches', { url: '${url('login.test.evil.test', '/login')}' })`);
  check(Array.isArray(evil) && evil.length === 0, 'una web parecida no ve nada');
  const evilFill = await sw(`call('fill', { id: '${saved.id}', url: '${url('evil.test', '/login')}' }).then(() => 'relleno', (e) => e.code)`);
  check(evilFill === 'denied', 'ni pidiéndola por su ID');

  log('Guardar al iniciar sesión');
  await chrome.send('Page.navigate', { url: url('nueva.test', '/login') }, sessionId);
  for (let i = 0; i < 50; i++) { if (await ev(`location.host.startsWith('nueva.test') && document.readyState === 'complete' && !!document.querySelector('#pw')`).catch(() => false)) break; await sleep(100); }
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

  log('Menú de la extensión: el tiempo que queda baja');
  const pop = await chrome.send('Target.createTarget', { url: `chrome-extension://${EXT_ID}/popup.html` });
  const { sessionId: ps } = await chrome.send('Target.attachToTarget', { targetId: pop.targetId, flatten: true });
  const pev = async (expr) => (await chrome.send('Runtime.evaluate', { expression: expr, returnByValue: true }, ps)).result.value;
  let p1 = '';
  for (let i = 0; i < 30; i++) { p1 = await pev(`document.querySelector('#pill').textContent`); if (/\d:\d\d/.test(p1)) break; await sleep(200); }
  await sleep(2300);
  const p2 = await pev(`document.querySelector('#pill').textContent`);
  check(/\d:\d\d/.test(p1) && p1 !== p2, `la cuenta atrás baja (${p1} → ${p2})`);
  await chrome.send('Target.closeTarget', { targetId: pop.targetId });

  log('Cerrar la bóveda');
  await chrome.send('Page.navigate', { url: url('login.test', '/login') }, sessionId);
  for (let i = 0; i < 50; i++) { if (await ev(`location.pathname === '/login' && document.readyState === 'complete'`).catch(() => false)) break; await sleep(100); }
  await waitIcons('.ic:not(.off)', 2);
  await uiCall('lock');
  for (let i = 0; i < 25; i++) { s = await sw('publicState()'); if (!s.unlocked) break; await sleep(200); }
  check(!s.unlocked, 'la extensión se entera de que se cerró');
  check((await waitIcons('.ic.off', 2)).length === 2, 'los iconos de los campos se ponen grises');
  await shot('3-cerrada');
  await clickSelector('#user');
  await shadowQuery('.menu [data-a="unlock"]', 10);
  await shot('4-menu-cerrada');
  const locked = await sw(`call('fill', { id: '${saved.id}', url: '${url('login.test', '/login')}' }).then(() => 'relleno', (e) => e.code)`);
  check(locked === 'locked', 'cerrada no rellena');
  await sleep(500);
  const after = await uiJS(`[!document.querySelector('#lock').hidden, document.querySelector('#list').innerText.length, document.querySelector('#detail').innerText.length]`);
  check(after[0] && after[1] === 0 && after[2] === 0, 'la ventana vuelve a pedir la maestra y borra lo que enseñaba');

  log('Sin TurboVAULT: la extensión no se pone a lanzar procesos');
  await uiCall('quit');
  ui = false;
  for (let i = 0; i < 6 && countProcs() > 0; i++) await sleep(300);
  // Que no se vuelva a arrancar mientras se mide: el puente apunta a un programa que no existe.
  writeFileSync(manifest, JSON.stringify({ name: 'com.turbovault.native', description: 'prueba', path: join(tmp, 'no-existe.exe'), type: 'stdio', allowed_origins: [`chrome-extension://${EXT_ID}/`] }));
  await sleep(500);
  // Cada intento de conexión apunta su hora en lastTry: se cuentan las distintas.
  const tries = await sw(`(async () => {
    const seen = new Set();
    const t0 = lastTry;
    for (let i = 0; i < 40; i++) {
      await handle({ type: 'query' }, { tab: { id: 1, url: 'http://login.test/' }, url: 'http://login.test/', frameId: 0 }).catch(() => {});
      if (lastTry !== t0) seen.add(lastTry);
      await new Promise((r) => setTimeout(r, 50));
    }
    return seen.size;
  })()`);
  check(tries <= 2, `40 preguntas seguidas sin TurboVAULT: ${tries} intentos de conexión (antes, uno por pregunta)`);
} catch (e) {
  failures++;
  log('ERROR', e.stack || e.message);
} finally {
  await cleanup();
}
log(failures ? `${failures} fallos` : 'Todo bien');
process.exit(failures ? 1 : 0);
