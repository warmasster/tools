// Service worker de TurboDL.
//
// Intercepta descargas, decide si se encarga TurboDL, orquesta el backend (que hace
// el trabajo pesado en el documento offscreen), entrega el archivo final a Descargas
// y mantiene el badge. No descarga nada él mismo: MV3 lo duerme a los 30 s.

import * as db from './lib/db.js';
import { getSettings, setSettings } from './lib/settings.js';
import { createBackend, ensureOffscreen } from './lib/backend.js';
import { probe } from './lib/probe.js';
import { installHeaderRule, removeHeaderRule, cookieHeaderFor } from './lib/net-rules.js';
import {
  MiB, hostOf, domainMatches, sanitizeFilename, basename, filenameFromUrl, formatBytes,
} from './lib/util.js';

const backendReady = createBackend();
const ui = new BroadcastChannel('turbodl');
const ACTIVE = ['queued', 'downloading', 'ready', 'delivering'];

// Estado en memoria (si el SW se reinicia, todo se puede reconstruir desde IndexedDB).
const deliveryNames = new Map(); // blob URL → nombre, para el onDeterminingFilename de la entrega
const deliveries = new Map(); // id de chrome.downloads → id de TurboDL
const delivering = new Set(); // ids con una entrega en marcha (evita entregar dos veces)

// ================================================================= arranque

chrome.runtime.onInstalled.addListener(async () => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'turbodl-link', title: 'Descargar con TurboDL', contexts: ['link'] });
  });
  await setSettings({}); // guarda los valores por defecto si no había ajustes
  await boot();
});

chrome.runtime.onStartup.addListener(boot);

/**
 * Recrea el offscreen, limpia restos y reanuda lo que estaba en marcha. Se llama al
 * abrir Chrome y al instalar/actualizar la extensión.
 */
async function boot() {
  const backend = await backendReady;
  await ensureOffscreen();
  const all = await db.getAll('downloads');
  const ids = new Set(all.map((r) => r.id));
  await backend.cleanup([...ids]).catch((e) => console.warn('Limpieza OPFS:', e));
  for (const s of await db.getAll('segments')) if (!ids.has(s.id)) await db.del('segments', s.id);

  for (const rec of all) {
    try {
      if (rec.status === 'queued' || rec.status === 'downloading') {
        await resume(rec.id);
      } else if (rec.status === 'ready') {
        await deliver(rec.id);
      } else if (rec.status === 'delivering') {
        const [item] = rec.deliveryId ? await chrome.downloads.search({ id: rec.deliveryId }) : [];
        if (item?.state === 'complete') await complete(rec.id, rec.deliveryId);
        else if (item?.state !== 'in_progress') await deliver(rec.id); // el blob URL murió con Chrome
      }
    } catch (e) {
      console.warn('No se pudo reanudar', rec.id, e);
    }
  }
  updateBadge();
}

// ================================================================= interceptación

chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
  if (item.byExtensionId) {
    // Descargas creadas por una extensión: la entrega final de TurboDL, el plan B
    // "Descargar con Chrome normal" o las de otras extensiones. Nunca se interceptan;
    // si lo hiciéramos con las nuestras habría un bucle infinito.
    const name = item.byExtensionId === chrome.runtime.id && deliveryNames.get(item.url);
    if (name) suggest({ filename: name, conflictAction: 'uniquify' });
    else suggest();
    return;
  }

  // suggest() hay que llamarlo siempre y una sola vez: mientras no se llama, Chrome
  // retiene esta descarga y las que vengan detrás.
  let called = false;
  const once = () => { if (!called) { called = true; try { suggest(); } catch { /* ya resuelta */ } } };
  intercept(item, once)
    .catch((e) => console.warn('Interceptación:', e))
    .finally(once);
  return true; // respuesta asíncrona
});

async function intercept(item, release) {
  const s = await getSettings();
  const url = item.finalUrl || item.url;
  if (!s.enabled) return;
  if (!/^https?:/i.test(url)) return; // blob:, data:, file:...
  const host = hostOf(url);
  if (s.excludedDomains.some((d) => domainMatches(host, d))) return;
  if (item.totalBytes > 0 && item.totalBytes < s.minSizeMB * MiB) return;
  if (/^text\/html/i.test(item.mime || '')) return; // casi seguro una página de login o un POST

  const referer = item.referrer || (await activeTabUrl());
  const plan = await evaluate(url, {
    referer,
    expectedSize: item.totalBytes > 0 ? item.totalBytes : null,
    minBytes: s.minSizeMB * MiB,
  });
  if (!plan.ok) {
    if (plan.reason) console.info(`TurboDL deja la descarga a Chrome: ${plan.reason}`, url);
    if (plan.notify) notify(`turbodl-skip-${item.id}`, 'TurboDL', plan.notify);
    return;
  }

  // Tomamos el control: cancelar la de Chrome y crear la nuestra.
  await chrome.downloads.cancel(item.id).catch(() => {});
  release();
  await chrome.downloads.erase({ id: item.id }).catch(() => {});
  await createDownload(plan, { filename: basename(item.filename), originalUrl: item.url, mime: item.mime });
}

/**
 * Sondea la URL y decide si TurboDL puede encargarse.
 * forced = true viene del menú contextual o del gestor: se ignora el tamaño mínimo
 * y, si el servidor no admite Range, se descarga con una sola conexión.
 */
async function evaluate(url, { referer, expectedSize = null, minBytes = 0, forced = false }) {
  const id = await nextId();
  // La regla del Referer tiene que estar puesta antes del sondeo para que cuente.
  await installHeaderRule(id, { url, referer });
  let info;
  let useCookies = false;
  const releaseSlot = await (await backendReady).reserveSlot(new URL(url).origin);
  try {
    info = await probe(url);
    // Con credentials:'include' las cookies normalmente llegan solas. Si la respuesta
    // parece un "no autorizado" y el navegador tiene cookies para esa URL, es que no
    // han llegado (cookies de terceros bloqueadas, SameSite...): las ponemos a mano.
    if (looksDenied(info) && (await cookieHeaderFor(url))) {
      await installHeaderRule(id, { url, referer, useCookies: true });
      const again = await probe(url);
      if (!looksDenied(again)) { info = again; useCookies = true; }
    }
  } catch (e) {
    await removeHeaderRule(id);
    return { ok: false, reason: `sondeo fallido (${e.message})`, notify: forced ? 'No se pudo conectar con el servidor.' : null };
  } finally {
    releaseSlot();
  }

  const fail = async (reason, notifyMsg) => {
    await removeHeaderRule(id);
    return { ok: false, reason, notify: notifyMsg };
  };
  const html = /^text\/html/.test(info.contentType);
  let mode = 'multi';

  if (!forced) {
    if (info.status !== 206 || !info.rangeOk) return fail(`sin soporte de Range (HTTP ${info.status})`);
    if (expectedSize != null && info.size !== expectedSize) return fail('el tamaño no coincide con el de Chrome');
    if (html) return fail('el servidor devuelve HTML');
    if (info.size < minBytes) return fail('por debajo del tamaño mínimo');
  } else if (!(info.status === 206 && info.rangeOk)) {
    if (info.status < 200 || info.status >= 300 || info.encoded && info.status === 206) {
      return fail(`HTTP ${info.status}`, `El servidor respondió ${info.status}; no se puede descargar.`);
    }
    mode = 'single';
  }

  // Hace falta ~2x el tamaño: el archivo en OPFS y su copia en Descargas.
  if (info.size > 0) {
    const { quota = 0, usage = 0 } = await navigator.storage.estimate();
    if (quota - usage < info.size * 2) {
      return fail('sin espacio', `No hay espacio suficiente (hacen falta ${formatBytes(info.size * 2)}). La descarga sigue con Chrome.`);
    }
  }

  return {
    ok: true, id, mode, useCookies, referer,
    url: info.finalUrl, size: info.size, etag: info.etag, lastModified: info.lastModified,
    filename: info.filename, contentType: info.contentType,
  };
}

function looksDenied(info) {
  return info.status === 401 || info.status === 403 || /^text\/html/.test(info.contentType);
}

async function createDownload(plan, { filename, originalUrl, mime }) {
  const s = await getSettings();
  const name = sanitizeFilename(filename || plan.filename || filenameFromUrl(plan.url) || 'descarga');
  const rec = {
    id: plan.id,
    url: plan.url,
    originalUrl: originalUrl || plan.url,
    filename: name,
    size: plan.size,
    mime: mime || plan.contentType || '',
    referer: plan.referer || '',
    etag: plan.etag,
    lastModified: plan.lastModified,
    mode: plan.mode,
    resumable: plan.mode === 'multi',
    useCookies: plan.useCookies,
    connections: s.connections,
    opfsName: `dl-${plan.id}.part`,
    status: 'queued',
    error: null,
    errorKind: null,
    notice: plan.mode === 'single' ? 'El servidor no permite descargar por partes: una sola conexión y sin reanudación.' : null,
    createdAt: Date.now(),
    updatedAt: Date.now(),
  };
  await db.put('downloads', rec);
  // Tras redirecciones el dominio final puede ser otro: rehacer la regla con la URL final.
  await installHeaderRule(rec.id, rec);
  ui.postMessage({ type: 'changed', id: rec.id });
  await (await backendReady).start(rec.id, s);
  notify(`turbodl-start-${rec.id}`, 'Descargando con TurboDL', `${name}${rec.size ? ` · ${formatBytes(rec.size)}` : ''}`);
  updateBadge();
  return rec.id;
}

/**
 * Menú contextual, gestor y pruebas: descargar una URL a la fuerza.
 * Devuelve el id de la descarga o lanza un error con un mensaje para el usuario.
 */
async function addUrl(url, referer = '') {
  if (!/^https?:\/\//i.test(url || '')) throw new Error('Solo se pueden descargar enlaces http o https.');
  const plan = await evaluate(url, { referer, forced: true });
  if (!plan.ok) {
    // Si no hay espacio, que lo intente Chrome directamente.
    if (plan.reason === 'sin espacio') chrome.downloads.download({ url });
    throw new Error(plan.notify || 'No se pudo descargar ese enlace.');
  }
  return createDownload(plan, { originalUrl: url });
}

// ================================================================= control

async function pause(id) {
  await (await backendReady).pause(id);
  await removeHeaderRule(id);
  updateBadge();
}

async function resume(id) {
  const rec = await db.get('downloads', id);
  if (!rec) return;
  if (rec.errorKind === 'delivery' || rec.status === 'ready') return deliver(id);
  await db.patch('downloads', id, { status: 'queued', error: null, errorKind: null });
  ui.postMessage({ type: 'changed', id });
  await installHeaderRule(id, rec);
  await (await backendReady).resume(id, await getSettings());
  updateBadge();
}

async function cancel(id) {
  await (await backendReady).cancel(id).catch((e) => console.warn(e));
  await removeHeaderRule(id);
  await db.del('segments', id);
  await db.del('downloads', id);
  ui.postMessage({ type: 'changed', id });
  updateBadge();
}

/** Plan B: que Chrome lo descargue de la forma normal. */
async function fallback(id) {
  const rec = await db.get('downloads', id);
  if (!rec) return;
  await cancel(id);
  // Al venir de nuestra extensión, onDeterminingFilename no la intercepta.
  await chrome.downloads.download({ url: rec.originalUrl || rec.url, filename: rec.filename, conflictAction: 'uniquify' });
}

// ================================================================= entrega final

async function deliver(id) {
  if (delivering.has(id)) return;
  delivering.add(id);
  try {
    const rec = await db.get('downloads', id);
    if (!rec) return;
    const url = await (await backendReady).fileUrl(id);
    deliveryNames.set(url, rec.filename);
    const deliveryId = await chrome.downloads.download({
      url, filename: rec.filename, conflictAction: 'uniquify', saveAs: false,
    });
    deliveries.set(deliveryId, id);
    await db.patch('downloads', id, { status: 'delivering', deliveryId, error: null, errorKind: null });
    ui.postMessage({ type: 'changed', id });
  } catch (e) {
    await deliveryFailed(id, e.message);
  } finally {
    delivering.delete(id);
  }
}

async function deliveryFailed(id, detail) {
  const rec = await db.patch('downloads', id, {
    status: 'error', errorKind: 'delivery',
    error: `El archivo se descargó, pero no se pudo guardar en Descargas${detail ? ` (${detail})` : ''}. Pulsa Reintentar.`,
  });
  ui.postMessage({ type: 'changed', id });
  if (rec) notify(`turbodl-err-${id}`, 'Error al guardar', rec.filename);
  updateBadge();
}

chrome.downloads.onChanged.addListener(async (delta) => {
  if (!delta.state) return;
  const id = deliveries.get(delta.id) ?? (await db.getByIndex('downloads', 'deliveryId', delta.id))?.id;
  if (id == null) return;
  if (delta.state.current === 'complete') await complete(id, delta.id);
  else if (delta.state.current === 'interrupted') await deliveryFailed(id, delta.error?.current);
});

async function complete(id, deliveryId) {
  const rec = await db.get('downloads', id);
  if (!rec) return;
  const [item] = await chrome.downloads.search({ id: deliveryId });
  for (const [url, name] of deliveryNames) if (name === rec.filename) deliveryNames.delete(url);
  deliveries.delete(deliveryId);
  // revokeObjectURL + borrar de OPFS, y fuera de las descargas activas.
  await (await backendReady).finalize(id).catch((e) => console.warn(e));
  await db.put('history', {
    id,
    filename: basename(item?.filename) || rec.filename,
    path: item?.filename || '',
    size: item?.fileSize ?? rec.size,
    url: rec.originalUrl || rec.url,
    deliveryId,
    createdAt: rec.createdAt,
    completedAt: Date.now(),
  });
  await db.del('segments', id);
  await db.del('downloads', id);
  await removeHeaderRule(id);
  ui.postMessage({ type: 'changed', id });
  chrome.notifications.clear(`turbodl-start-${id}`);
  notify(`turbodl-done-${id}`, 'Descarga terminada', basename(item?.filename) || rec.filename);
  updateBadge();
}

// ================================================================= mensajes

// Avisos de estado del motor (offscreen).
async function onEngineStatus({ id, status, error }) {
  if (status === 'ready') {
    await removeHeaderRule(id);
    await deliver(id);
  } else if (status === 'error') {
    await removeHeaderRule(id);
    const rec = await db.get('downloads', id);
    notify(`turbodl-err-${id}`, 'Error en la descarga', `${rec?.filename || ''}\n${error || ''}`);
  }
  updateBadge();
}

const commands = {
  add: (m) => addUrl(m.url, m.referer),
  pause: (m) => pause(m.id),
  resume: (m) => resume(m.id),
  retry: (m) => resume(m.id),
  cancel: (m) => cancel(m.id),
  fallback: (m) => fallback(m.id),
  removeHistory: async (m) => { await db.del('history', m.id); ui.postMessage({ type: 'changed' }); },
  clearHistory: async () => {
    for (const h of await db.getAll('history')) await db.del('history', h.id);
    ui.postMessage({ type: 'changed' });
  },
  openManager: () => openManager(),
};

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg?.target !== 'background') return;
  if (msg.type === 'status') { onEngineStatus(msg); return; }
  const fn = commands[msg.cmd];
  if (!fn) return;
  Promise.resolve(fn(msg)).then(
    (result) => sendResponse({ ok: true, result }),
    (e) => sendResponse({ ok: false, error: String(e?.message || e) }),
  );
  return true;
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (info.menuItemId !== 'turbodl-link') return;
  addUrl(info.linkUrl, info.pageUrl || tab?.url || '')
    .catch((e) => notify(`turbodl-add-${Date.now()}`, 'TurboDL', e.message));
});

chrome.commands.onCommand.addListener(async (command) => {
  if (command !== 'toggle-interception') return;
  const s = await getSettings();
  await setSettings({ enabled: !s.enabled });
});

chrome.storage.onChanged.addListener(async (changes, area) => {
  if (area !== 'local' || !changes.settings) return;
  updateBadge();
  // Solo si el motor ya existe: no merece la pena crearlo para un cambio de ajustes.
  const ctx = await chrome.runtime.getContexts({ contextTypes: ['OFFSCREEN_DOCUMENT'] });
  if (ctx.length) (await backendReady).updateSettings(await getSettings()).catch(() => {});
});

chrome.notifications.onClicked.addListener((nid) => {
  chrome.notifications.clear(nid);
  openManager();
});

// ================================================================= varios

async function openManager() {
  const url = chrome.runtime.getURL('manager.html');
  const [ctx] = await chrome.runtime.getContexts({ contextTypes: ['TAB'], documentUrls: [url] });
  if (ctx) {
    await chrome.tabs.update(ctx.tabId, { active: true });
    await chrome.windows.update(ctx.windowId, { focused: true });
  } else {
    await chrome.tabs.create({ url });
  }
}

async function updateBadge() {
  const [s, all] = await Promise.all([getSettings(), db.getAll('downloads')]);
  const n = all.filter((r) => ACTIVE.includes(r.status)).length;
  await chrome.action.setBadgeText({ text: n ? String(n) : s.enabled ? '' : 'OFF' });
  await chrome.action.setBadgeBackgroundColor({ color: s.enabled ? '#3b82f6' : '#6b7280' });
  await chrome.action.setTitle({
    title: `TurboDL · ${s.enabled ? 'interceptación activada' : 'interceptación desactivada'}${n ? ` · ${n} activa(s)` : ''}`,
  });
}

function notify(id, title, message) {
  chrome.notifications.create(id, {
    type: 'basic', iconUrl: 'icons/icon128.png', title, message: message || '', priority: 0,
  }, () => void chrome.runtime.lastError);
}

async function activeTabUrl() {
  try {
    const [tab] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
    return /^https?:/.test(tab?.url || '') ? tab.url : '';
  } catch { return ''; }
}

// Ids numéricos (sirven también como id de la regla de declarativeNetRequest).
let idChain = Promise.resolve();
function nextId() {
  idChain = idChain.then(async () => {
    const { nextId: n = 1 } = await chrome.storage.local.get('nextId');
    await chrome.storage.local.set({ nextId: n + 1 });
    return n;
  });
  return idChain;
}

// Acceso para depurar desde la consola del service worker y para dev/run-tests.js.
self.turbodl = {
  addUrl, pause, resume, cancel, fallback, boot, getSettings, setSettings,
  list: async () => ({
    downloads: await db.getAll('downloads'), history: await db.getAll('history'), segs: await db.getAll('segments'),
  }),
  jobs: async () => (await backendReady).send('jobs'),
  rules: () => chrome.declarativeNetRequest.getSessionRules(),
};
