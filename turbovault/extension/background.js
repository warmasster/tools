// TurboVAULT: puente entre las páginas y la app de Windows.
//
// La extensión no guarda ninguna contraseña. Las pide a TurboVAULT (por Native
// Messaging) solo cuando tú lo pides, solo para la web en la que estás, y se
// las da únicamente al marco de la página que las va a usar. Lo único que
// guarda es un código aleatorio para que TurboVAULT reconozca este navegador.

importScripts('payhosts.js');

const HOST = 'com.turbovault.native';
const PENDING_TTL = 3 * 60 * 1000;

let port = null;
let seq = 0;
const waiting = new Map();
let state = { agent: false, unlocked: false, paired: false, exists: true };
let helloDone = null;
const pendingSave = new Map(); // tabId → { origin, url, username, password, status, id, title, time }
const lastUser = new Map(); // tabId → { origin, username, time }

// ---------- conexión con TurboVAULT ----------

async function token() {
  const { pairToken } = await chrome.storage.local.get('pairToken');
  if (pairToken && pairToken.length >= 32) return pairToken;
  const b = new Uint8Array(32);
  crypto.getRandomValues(b);
  const t = [...b].map((x) => x.toString(16).padStart(2, '0')).join('');
  await chrome.storage.local.set({ pairToken: t });
  return t;
}

// Si TurboVAULT no contesta, no se reintenta sin parar: cada intento lanza un
// proceso, y muchas pestañas preguntando a la vez llegaban a tumbar Chrome.
// Se espera cada vez más (2 s, 4 s… hasta 1 min). Lo que pides tú (pulsar el
// icono, abrir el menú de la extensión) puede reintentar antes.
let nextTry = 0;
let backoff = 0;
let lastTry = 0;
let rejected = ''; // por qué TurboVAULT rechazó la última conexión

function failed(ms) {
  backoff = Math.max(ms || 0, Math.min(backoff ? backoff * 2 : 2000, 60000));
  nextTry = Date.now() + backoff;
}

function connect(user = false) {
  if (port) return helloDone;
  const now = Date.now();
  if (now < nextTry && !(user && now - lastTry > 1500)) return Promise.resolve();
  lastTry = now;
  let p;
  try {
    p = chrome.runtime.connectNative(HOST);
  } catch (e) {
    failed();
    setState({ agent: false, error: String(e.message || e) });
    return Promise.resolve();
  }
  port = p;
  p.onMessage.addListener(onNative);
  p.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError ? chrome.runtime.lastError.message : 'Se cerró la conexión con TurboVAULT';
    if (port !== p) return;
    dropPort(friendlyError(err));
  });
  helloDone = (async () => {
    try {
      const st = await send('hello', { token: await token() });
      backoff = 0;
      nextTry = 0;
      setState(Object.assign({ agent: true, error: '' }, st));
      checkVersion(st && st.extVersion);
    } catch (e) {
      // El puente contesta pero sin TurboVAULT detrás: se cierra y se espera.
      if (port === p) { try { p.disconnect(); } catch { /* ya cerrado */ } dropPort(e.message); }
    }
  })();
  return helloDone;
}

function dropPort(error) {
  if (rejected) { error = rejected; rejected = ''; }
  port = null;
  helloDone = null;
  for (const [, w] of waiting) w.reject(Object.assign(new Error(error), { code: 'noagent' }));
  waiting.clear();
  failed();
  setState({ agent: false, unlocked: false, paired: false, error });
}

// TurboVAULT trae una versión nueva de la extensión (se actualizó): Chrome no
// recarga sola una extensión «descomprimida», así que se recarga ella misma.
// Una vez por versión, para no entrar en bucle si los archivos no cambiaron.
async function checkVersion(v) {
  if (!v || v === chrome.runtime.getManifest().version) return;
  const { reloadedFor } = await chrome.storage.local.get('reloadedFor');
  if (reloadedFor === v) return;
  await chrome.storage.local.set({ reloadedFor: v });
  chrome.runtime.reload();
}

// Tras instalar o recargar la extensión, las pestañas que ya estaban abiertas
// no tienen su parte en la página: se les pone sin tener que recargarlas.
async function injectOpenTabs() {
  const tabs = await chrome.tabs.query({ url: ['http://*/*', 'https://*/*'] }).catch(() => []);
  for (const t of tabs) {
    if (t.discarded) continue;
    chrome.scripting.executeScript({ target: { tabId: t.id, allFrames: true }, files: ['payhosts.js', 'content.js'] }).catch(() => {});
  }
}

function friendlyError(msg) {
  if (/not found|no se encuentra|Specified native messaging host/i.test(msg)) return 'TurboVAULT no está instalado en este PC (o no se ha preparado la extensión desde sus ajustes).';
  if (/Access to the specified native messaging host is forbidden/i.test(msg)) return 'TurboVAULT no reconoce esta extensión.';
  return msg;
}

function send(cmd, args, timeout = 20000) {
  return new Promise((resolve, reject) => {
    if (!port) { reject(Object.assign(new Error(state.error || 'TurboVAULT no responde'), { code: 'noagent' })); return; }
    const id = ++seq;
    const t = setTimeout(() => { waiting.delete(id); reject(Object.assign(new Error('TurboVAULT no responde'), { code: 'timeout' })); }, timeout);
    waiting.set(id, {
      resolve: (v) => { clearTimeout(t); resolve(v); },
      reject: (e) => { clearTimeout(t); reject(e); },
    });
    try {
      port.postMessage({ id, cmd, args });
    } catch (e) {
      waiting.delete(id);
      clearTimeout(t);
      reject(e);
    }
  });
}

// call se conecta si hace falta y manda una orden.
async function call(cmd, args, timeout) {
  if (!port) await connect(true);
  else if (helloDone) await helloDone;
  return send(cmd, args, timeout);
}

function onNative(m) {
  if (m.event) {
    if (m.event === 'state' && m.data) setState(Object.assign({ agent: true }, m.data));
    // TurboVAULT no acepta este navegador: reintentar enseguida no lo arregla.
    if (m.event === 'rejected') { failed(60000); rejected = 'TurboVAULT no acepta esta conexión: ' + (m.error || ''); }
    return;
  }
  const w = waiting.get(m.id);
  if (!w) return;
  waiting.delete(m.id);
  if (m.ok) w.resolve(m.data === undefined ? null : m.data);
  else w.reject(Object.assign(new Error(m.error || 'Error'), { code: m.code }));
}

let lastPublic = '';
function setState(patch) {
  const wasOpen = state.unlocked && state.paired;
  state = Object.assign({}, state, patch);
  // Solo se avisa a las pestañas si algo ha cambiado de verdad.
  const pub = publicState();
  const key = JSON.stringify(Object.assign({}, pub, { now: 0 }));
  if (key === lastPublic) return;
  lastPublic = key;
  const open = state.agent && state.unlocked;
  chrome.action.setIcon({ path: open ? { 16: 'icons/open-16.png', 24: 'icons/open-24.png', 32: 'icons/open-32.png' } : { 16: 'icons/locked-16.png', 24: 'icons/locked-24.png', 32: 'icons/locked-32.png' } }).catch(() => {});
  chrome.action.setBadgeText({ text: state.agent && state.unlocked && !state.paired ? '!' : '' }).catch(() => {});
  chrome.action.setBadgeBackgroundColor({ color: '#f59e0b' }).catch(() => {});
  chrome.action.setTitle({ title: !state.agent ? 'TurboVAULT: sin conexión' : state.unlocked ? (state.paired ? 'TurboVAULT: abierta' : 'TurboVAULT: vincula este navegador') : 'TurboVAULT: cerrada' }).catch(() => {});
  broadcast({ type: 'state', state: pub });
  if (!wasOpen && state.unlocked && state.paired) retryPendingSaves();
}

function publicState() {
  return {
    agent: state.agent, unlocked: !!state.unlocked, paired: !!state.paired, exists: state.exists !== false,
    expires: state.expires || 0, now: state.now || 0, error: state.error || '', sessionMin: state.sessionMin || 10,
    cards: state.cards === true, cardAsk: state.cardAsk !== false, cardMask: !!state.cardMask,
  };
}

async function broadcast(msg) {
  const tabs = await chrome.tabs.query({}).catch(() => []);
  for (const t of tabs) chrome.tabs.sendMessage(t.id, msg).catch(() => {});
  chrome.runtime.sendMessage(msg).catch(() => {});
}

// ---------- utilidades ----------

function originOf(u) {
  try {
    const x = new URL(u);
    if (x.protocol !== 'http:' && x.protocol !== 'https:') return '';
    return x.origin;
  } catch { return ''; }
}

function hostOf(u) {
  try { return new URL(u).hostname.replace(/^www\./, ''); } catch { return ''; }
}

// Solo se rellena en la página principal o en marcos de su mismo origen: un
// iframe de otra web (un anuncio, por ejemplo) no recibe nada.
function frameAllowed(sender) {
  if (!sender.tab || !sender.url) return false;
  const fo = originOf(sender.url);
  if (!fo) return false;
  if (sender.frameId === 0) return true;
  return fo === originOf(sender.tab.url);
}

// Tarjetas: además de la página y sus marcos del mismo origen, los recuadros de
// pago de las pasarelas conocidas (Stripe, Adyen, Redsys…), solo por https.
function cardFrameAllowed(sender) {
  if (frameAllowed(sender)) return true;
  if (!sender.tab || !sender.url || !originOf(sender.tab.url)) return false;
  try {
    const u = new URL(sender.url);
    return u.protocol === 'https:' && self.TV_PAY.isPayHost(u.hostname);
  } catch { return false; }
}

async function pairCode() {
  const t = await token();
  const h = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode('TurboVAULT code|' + t)));
  const n = ((h[0] << 16) | (h[1] << 8) | h[2]) % 1000000;
  return `${String(Math.floor(n / 1000)).padStart(3, '0')} ${String(n % 1000).padStart(3, '0')}`;
}

async function pair() {
  const t = await token();
  await call('pairRequest', { token: t });
  return pairCode();
}

// ---------- mensajes de las páginas y del menú ----------

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  handle(msg, sender).then((r) => reply({ ok: true, data: r }), (e) => reply({ ok: false, error: e.message, code: e.code }));
  return true;
});

async function handle(msg, sender) {
  // Solo las páginas de la propia extensión tienen esta dirección (las webs no pueden cargarlas).
  const fromPopup = sender.id === chrome.runtime.id && !!sender.url && sender.url.startsWith(`chrome-extension://${chrome.runtime.id}/`);
  switch (msg.type) {
    case 'state':
      if (!port) await connect(!!msg.user);
      else if (helloDone) await helloDone;
      return publicState();

    case 'query': {
      if (!port) await connect(!!msg.user);
      else if (helloDone) await helloDone;
      const loginOK = frameAllowed(sender);
      if (!loginOK && !cardFrameAllowed(sender)) return { state: publicState(), matches: [], blocked: true };
      if (!state.agent || !state.unlocked || !state.paired) return { state: publicState(), matches: [], cards: [] };
      if (msg.cards) {
        // La lista de tarjetas: marca, cuatro últimas cifras y caducidad (nunca el número).
        const cards = state.cards ? await call('cards', {}) : [];
        return { state: publicState(), matches: [], cards, host: hostOf(sender.tab.url) };
      }
      if (!loginOK) return { state: publicState(), matches: [] };
      const matches = await call('matches', { url: sender.url });
      return { state: publicState(), matches, host: hostOf(sender.url) };
    }

    case 'cardPanel': {
      // Un recuadro de pago es muy pequeño para el menú: lo abre la página principal.
      if (!cardFrameAllowed(sender)) throw new Error('No permitido');
      await chrome.tabs.sendMessage(sender.tab.id, { type: 'cardPanel' }, { frameId: 0 }).catch(() => {});
      return true;
    }

    case 'fillCard': {
      if (!cardFrameAllowed(sender)) throw new Error('Este marco no puede recibir tarjetas.');
      // Puede tardar: TurboVAULT espera a que escribas la contraseña maestra en su ventana.
      const top = originOf(sender.tab.url);
      const data = await call('fillCard', { id: msg.id, url: sender.tab.url, frame: sender.url }, 150000);
      // Mientras confirmabas, la pestaña ha podido irse a otra web: entonces nada.
      const now = await chrome.tabs.get(sender.tab.id).catch(() => null);
      if (!now || originOf(now.url) !== top) throw new Error('La página cambió mientras confirmabas: la tarjeta no se ha rellenado.');
      // Los campos de una misma tarjeta pueden ir en marcos distintos (el número en
      // uno de Stripe, el titular en la tienda): se manda a todos los de la pestaña,
      // y cada uno rellena solo sus campos de tarjeta visibles.
      chrome.tabs.sendMessage(sender.tab.id, { type: 'cardFill', data, rid: msg.rid, top, fromTop: sender.frameId === 0 }).catch(() => {});
      return data;
    }

    case 'fill': {
      if (!frameAllowed(sender)) throw new Error('Este marco no es de la misma web: no se rellena.');
      return call('fill', { id: msg.id, url: sender.url });
    }

    case 'totp': {
      if (!frameAllowed(sender)) throw new Error('Este marco no es de la misma web.');
      return call('totp', { id: msg.id, url: sender.url });
    }

    case 'generate':
      return call('generate', msg.opts ? { gen: msg.opts } : {});

    case 'unlock':
      return call('unlock', {});

    case 'openApp':
      return call('openApp', {});

    case 'pair':
      return { code: await pair() };

    case 'pairCode':
      return { code: await pairCode() };

    case 'lock':
      return call('lock', {});

    case 'username': {
      // Primer paso de los inicios de sesión en dos pantallas (correo y luego contraseña).
      if (!frameAllowed(sender) || !msg.username) return null;
      lastUser.set(sender.tab.id, { origin: originOf(sender.url), username: String(msg.username).slice(0, 300), time: Date.now() });
      return null;
    }

    case 'captured':
      return captured(msg, sender);

    case 'pendingSave': {
      if (!sender.tab) return null;
      const p = pendingSave.get(sender.tab.id);
      if (!p || Date.now() - p.time > PENDING_TTL || p.origin !== originOf(sender.url) || sender.frameId !== 0) return null;
      return { status: p.status, host: hostOf(p.url), username: p.username, title: p.title };
    }

    case 'saveAction':
      return saveAction(msg, sender);

    // --- desde el menú de la extensión ---
    case 'popup:info': {
      if (!fromPopup) throw new Error('No permitido');
      if (!port) await connect(true);
      else if (helloDone) await helloDone;
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
      const url = tab && tab.url ? tab.url : '';
      let matches = [];
      if (state.agent && state.unlocked && state.paired && originOf(url)) {
        matches = await call('matches', { url }).catch(() => []);
      }
      return { state: publicState(), url, host: hostOf(url), matches, code: state.paired ? '' : await pairCode() };
    }
    case 'popup:search':
      if (!fromPopup) throw new Error('No permitido');
      return call('search', { q: msg.q });
    case 'popup:copy': {
      if (!fromPopup) throw new Error('No permitido');
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
      return call('copy', { id: msg.id, field: msg.field, url: tab && tab.url ? tab.url : '' });
    }
    case 'popup:fill': {
      if (!fromPopup) throw new Error('No permitido');
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
      if (!tab || !originOf(tab.url)) throw new Error('Esta página no admite rellenar.');
      const data = await call('fill', { id: msg.id, url: tab.url });
      await chrome.tabs.sendMessage(tab.id, { type: 'fillNow', data, origin: originOf(tab.url) }, { frameId: 0 });
      return true;
    }
    case 'popup:generate':
      if (!fromPopup) throw new Error('No permitido');
      return call('generate', msg.opts ? { gen: msg.opts } : {});
    case 'popup:copyText':
      if (!fromPopup) throw new Error('No permitido');
      // Copiar lo generado: lo hace TurboVAULT para que no quede en el historial
      // de Windows y se borre solo.
      return call('copyText', { password: msg.text });
  }
  throw new Error('Mensaje desconocido');
}

// ---------- guardar contraseñas nuevas ----------

const captureTimes = new Map(); // tabId → tiempos de las últimas capturas

async function captured(msg, sender) {
  if (!frameAllowed(sender) || !msg.password) return null;
  const tabId = sender.tab.id;
  // Una pestaña no puede preguntar «¿guardo esto?» sin parar.
  const now = Date.now();
  const times = (captureTimes.get(tabId) || []).filter((t) => now - t < 60000);
  if (times.length >= 5) return null;
  times.push(now);
  captureTimes.set(tabId, times);
  const origin = originOf(sender.url);
  let username = String(msg.username || '').slice(0, 300);
  if (!username) {
    const lu = lastUser.get(tabId);
    if (lu && lu.origin === origin && Date.now() - lu.time < 10 * 60 * 1000) username = lu.username;
  }
  const password = String(msg.password).slice(0, 1000);
  const prev = pendingSave.get(tabId);
  if (prev && prev.origin === origin && prev.password === password && prev.username === username && Date.now() - prev.time < 10000) {
    return { status: prev.status, host: hostOf(prev.url), username, title: prev.title };
  }
  const p = { origin, url: sender.url, username, password, status: 'locked', id: '', title: hostOf(sender.url), time: Date.now() };
  pendingSave.set(tabId, p);
  setTimeout(() => { const x = pendingSave.get(tabId); if (x === p) pendingSave.delete(tabId); }, PENDING_TTL);
  if (!state.agent) await connect();
  if (state.unlocked && state.paired) {
    try {
      const r = await call('check', { url: p.url, username, password });
      p.status = r.status;
      p.id = r.id || '';
      if (r.title) p.title = r.title;
    } catch (e) {
      p.status = e.code === 'locked' ? 'locked' : e.code === 'unpaired' ? 'unpaired' : 'error';
    }
  } else {
    p.status = !state.agent ? 'noagent' : !state.unlocked ? 'locked' : 'unpaired';
  }
  if (p.status === 'same' || p.status === 'never' || p.status === 'skip' || p.status === 'noagent' || p.status === 'error') {
    pendingSave.delete(tabId);
    return null;
  }
  return { status: p.status, host: hostOf(p.url), username, title: p.title };
}

async function retryPendingSaves() {
  for (const [tabId, p] of pendingSave) {
    if (p.status !== 'locked' && p.status !== 'unpaired') continue;
    try {
      const r = await call('check', { url: p.url, username: p.username, password: p.password });
      p.status = r.status;
      p.id = r.id || '';
      if (r.title) p.title = r.title;
      if (p.status === 'same' || p.status === 'never') { pendingSave.delete(tabId); continue; }
      if (p.wantSave) { await doSave(tabId, p); continue; }
      chrome.tabs.sendMessage(tabId, { type: 'saveUpdate', status: p.status, host: hostOf(p.url), username: p.username, title: p.title }, { frameId: 0 }).catch(() => {});
    } catch { /* sigue pendiente */ }
  }
}

async function doSave(tabId, p) {
  const r = await call('save', { url: p.url, username: p.username, password: p.password, id: p.status === 'update' ? p.id : '' });
  pendingSave.delete(tabId);
  chrome.tabs.sendMessage(tabId, { type: 'saveUpdate', status: 'saved', title: r && r.title }, { frameId: 0 }).catch(() => {});
  return r;
}

async function saveAction(msg, sender) {
  if (!sender.tab) return null;
  const tabId = sender.tab.id;
  const p = pendingSave.get(tabId);
  if (!p || p.origin !== originOf(sender.url)) throw new Error('Ya no hay nada que guardar.');
  if (msg.action === 'dismiss') { pendingSave.delete(tabId); return null; }
  if (msg.action === 'never') {
    pendingSave.delete(tabId);
    await call('never', { url: p.url });
    return { done: 'never' };
  }
  // guardar
  if (!state.unlocked || !state.paired) {
    p.wantSave = true;
    if (!state.unlocked) await call('unlock', {});
    else if (!state.paired) return { needPair: true, code: await pair() };
    return { waiting: true };
  }
  if (p.status === 'locked' || p.status === 'unpaired') {
    const r = await call('check', { url: p.url, username: p.username, password: p.password });
    p.status = r.status; p.id = r.id || '';
  }
  const r = await doSave(tabId, p);
  return { done: 'saved', title: r && r.title };
}

// ---------- atajo y menú contextual ----------

chrome.commands.onCommand.addListener(async (cmd) => {
  if (cmd !== 'fill') return;
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab) chrome.tabs.sendMessage(tab.id, { type: 'openMenu' }).catch(() => {});
});

function setupMenus() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'tv-fill', title: 'Rellenar con TurboVAULT', contexts: ['editable'] });
    chrome.contextMenus.create({ id: 'tv-gen', title: 'Generar una contraseña segura', contexts: ['editable'] });
  });
}
chrome.runtime.onInstalled.addListener(() => { setupMenus(); injectOpenTabs(); });
chrome.runtime.onStartup.addListener(() => { setupMenus(); connect(); });
chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (!tab) return;
  const type = info.menuItemId === 'tv-gen' ? 'generateHere' : 'openMenu';
  chrome.tabs.sendMessage(tab.id, { type }, { frameId: info.frameId || 0 }).catch(() => {});
});

chrome.tabs.onRemoved.addListener((id) => { pendingSave.delete(id); lastUser.delete(id); captureTimes.delete(id); });

connect();
