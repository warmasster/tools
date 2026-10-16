// Service worker de TurboDL.
//
// Intercepta descargas, decide si se encarga TurboDL, orquesta el motor que hace el
// trabajo pesado (el del navegador en el documento offscreen, o el nativo en Go si
// está instalado), entrega el archivo final y mantiene el badge. No descarga nada él
// mismo: MV3 lo duerme a los 30 s.

import * as db from './lib/db.js';
import { getSettings, setSettings } from './lib/settings.js';
import { createBackends, NativeBackend, ensureOffscreen } from './lib/backend.js';
import { probe } from './lib/probe.js';
import { installHeaderRule, removeHeaderRule, cookieHeaderFor } from './lib/net-rules.js';
import {
  MiB, hostOf, domainMatches, sanitizeFilename, basename, filenameFromUrl, formatBytes,
  formatDuration, formatSpeed, activeMsOf,
} from './lib/util.js';

const ui = new BroadcastChannel('turbodl');
const ACTIVE = ['queued', 'downloading', 'ready', 'delivering'];

// Estado en memoria (si el SW se reinicia, todo se puede reconstruir desde IndexedDB).
const deliveryNames = new Map(); // blob URL → nombre, para el onDeterminingFilename de la entrega
const deliveries = new Map(); // id de chrome.downloads → id de TurboDL
const delivering = new Set(); // ids con una entrega en marcha (evita entregar dos veces)

// ================================================================= motores

// El del navegador existe siempre; el nativo (turbodl-native.exe) solo si está instalado.
const backendsReady = createBackends().then((b) => { onNativeDetected(b.native); return b; });
let nativeCheck = null; // { at, backend }

function onNativeDetected(n) {
  nativeCheck = { at: Date.now(), backend: n };
  if (n) wireNative(n);
  chrome.storage.local.set({ engine: n ? { native: true, ...n.info } : { native: false } });
}

/** El motor nativo, volviendo a buscarlo si no estaba (por si se instaló después). */
async function getNative() {
  await backendsReady;
  if (!nativeCheck.backend && Date.now() - nativeCheck.at > 15_000) {
    nativeCheck.at = Date.now();
    onNativeDetected(await NativeBackend.detect());
  }
  return nativeCheck.backend;
}

async function getBrowser() { return (await backendsReady).browser; }

/** Motor para las descargas nuevas. */
async function preferredBackend(s) {
  return (s.engine !== 'browser' && (await getNative())) || getBrowser();
}

/** Motor de una descarga existente (sigue con el que empezó). */
async function backendFor(rec) {
  if (rec.backend !== 'native') return getBrowser();
  const n = await getNative();
  if (!n) throw new Error('El motor nativo no está instalado. Instálalo o usa «Descargar con Chrome normal».');
  return n;
}

// ================================================================= arranque

chrome.runtime.onInstalled.addListener(async () => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'turbodl-link', title: 'Descargar con TurboDL', contexts: ['link'] });
  });
  // Valores por defecto solo si no había ajustes (si no, una actualización los pisaría).
  if (!(await chrome.storage.local.get('settings')).settings) await setSettings({});
  await boot();
});

chrome.runtime.onStartup.addListener(boot);

/**
 * Recrea el offscreen, limpia restos y reanuda lo que estaba en marcha. Se llama al
 * abrir Chrome y al instalar/actualizar la extensión.
 */
async function boot() {
  const backend = await getBrowser();
  await ensureOffscreen();
  const all = await db.getAll('downloads');
  const ids = new Set(all.map((r) => r.id));
  await backend.cleanup([...ids]).catch((e) => console.warn('Limpieza OPFS:', e));
  const segs = await db.getAll('segments');
  for (const s of segs) if (!ids.has(s.id)) await db.del('segments', s.id);
  // Lo que se cerró descargando sigue marcado así: el tiempo activo real llega hasta
  // el último guardado de progreso, no hasta ahora (Chrome ha estado cerrado).
  for (const rec of all) {
    if (rec.status !== 'downloading' || !rec.runningSince) continue;
    const last = segs.find((s) => s.id === rec.id)?.updatedAt || rec.runningSince;
    await db.patch('downloads', rec.id, (cur) => ({
      activeMs: (cur.activeMs || 0) + Math.max(0, Math.min(last, Date.now()) - cur.runningSince),
      runningSince: null,
    }));
    rec.runningSince = null;
  }

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
  const backend = await preferredBackend(s);
  const plan = await evaluate(url, {
    referer,
    expectedSize: item.totalBytes > 0 ? item.totalBytes : null,
    minBytes: s.minSizeMB * MiB,
    native: backend.kind === 'native',
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
  await createDownload(plan, { filename: basename(item.filename), originalUrl: item.url, mime: item.mime, backend });
}

/**
 * Sondea la URL y decide si TurboDL puede encargarse.
 * forced = true viene del menú contextual o del gestor: se ignora el tamaño mínimo
 * y, si el servidor no admite Range, se descarga con una sola conexión.
 */
async function evaluate(url, { referer, expectedSize = null, minBytes = 0, forced = false, native = false }) {
  const id = await nextId();
  // La regla del Referer tiene que estar puesta antes del sondeo para que cuente.
  await installHeaderRule(id, { url, referer });
  let info;
  let useCookies = false;
  const releaseSlot = await (await getBrowser()).reserveSlot(new URL(url).origin);
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

  // Hace falta ~2x el tamaño: el archivo en OPFS y su copia en Descargas. (El motor
  // nativo escribe directamente en la carpeta final y comprueba el espacio él mismo.)
  if (info.size > 0 && !native) {
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

async function createDownload(plan, { filename, originalUrl, mime, backend }) {
  const s = await getSettings();
  backend ??= await preferredBackend(s);
  const native = backend.kind === 'native';
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
    backend: backend.kind,
    connections: native ? s.nativeConnections : s.connections,
    opfsName: `dl-${plan.id}.part`,
    status: 'queued',
    error: null,
    errorKind: null,
    notice: plan.mode === 'single' && !native ? 'El servidor no permite descargar por partes: una sola conexión y sin reanudación.' : null,
    createdAt: Date.now(),
    updatedAt: Date.now(),
  };
  await db.put('downloads', rec);
  ui.postMessage({ type: 'changed', id: rec.id });
  if (native) {
    // El motor nativo no pasa por la red de Chrome: Cookie y Referer van en la orden.
    await removeHeaderRule(rec.id);
    await backend.start(rec.id, s, rec, await nativeHeaders(rec));
  } else {
    // Tras redirecciones el dominio final puede ser otro: rehacer la regla con la URL final.
    await installHeaderRule(rec.id, rec);
    await backend.start(rec.id, s);
  }
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
  const s = await getSettings();
  const backend = await preferredBackend(s);
  const plan = await evaluate(url, { referer, forced: true, native: backend.kind === 'native' });
  if (!plan.ok) {
    // Si no hay espacio, que lo intente Chrome directamente.
    if (plan.reason === 'sin espacio') chrome.downloads.download({ url });
    throw new Error(plan.notify || 'No se pudo descargar ese enlace.');
  }
  return createDownload(plan, { originalUrl: url, backend });
}

// ================================================================= control

async function pause(id) {
  const rec = await db.get('downloads', id);
  if (!rec) return;
  await (await backendFor(rec)).pause(id);
  if (rec.backend === 'native') {
    // Si el motor no la tenía en marcha (p. ej. tras reiniciar) no avisará: marcarla aquí.
    await db.patch('downloads', id, (cur) => (cur.status === 'error' ? {} : { status: 'paused' }));
    ui.postMessage({ type: 'changed', id });
  }
  await removeHeaderRule(id);
  updateBadge();
}

async function resume(id) {
  const rec = await db.get('downloads', id);
  if (!rec) return;
  if (rec.backend !== 'native' && (rec.errorKind === 'delivery' || rec.status === 'ready')) return deliver(id);
  const backend = await backendFor(rec);
  await db.patch('downloads', id, { status: 'queued', error: null, errorKind: null });
  ui.postMessage({ type: 'changed', id });
  const s = await getSettings();
  if (rec.backend === 'native') {
    await backend.resume(id, s, rec, await nativeHeaders(rec));
  } else {
    await installHeaderRule(id, rec);
    await backend.resume(id, s);
  }
  updateBadge();
}

async function cancel(id) {
  const rec = await db.get('downloads', id);
  if (rec) await (await backendFor(rec)).cancel(id, rec).catch((e) => console.warn(e));
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
    const url = await (await getBrowser()).fileUrl(id);
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
  await (await getBrowser()).finalize(id).catch((e) => console.warn(e));
  const entry = {
    id,
    filename: basename(item?.filename) || rec.filename,
    path: item?.filename || '',
    size: item?.fileSize ?? rec.size,
    url: rec.originalUrl || rec.url,
    deliveryId,
    createdAt: rec.createdAt,
    completedAt: Date.now(),
    ...metricsOf(rec),
  };
  await db.put('history', entry);
  await db.del('segments', id);
  await db.del('downloads', id);
  await removeHeaderRule(id);
  ui.postMessage({ type: 'changed', id });
  chrome.notifications.clear(`turbodl-start-${id}`);
  notify(`turbodl-done-${id}`, 'Descarga terminada', doneMessage(entry));
  updateBadge();
}

/** Métricas que se guardan en el historial. */
function metricsOf(rec) {
  return {
    activeMs: activeMsOf(rec), // tiempo descargando (sin pausas ni cola)
    connections: rec.connections,
    backend: rec.backend || 'browser',
  };
}

function doneMessage(h) {
  const avg = h.activeMs > 0 && h.size ? formatSpeed((h.size * 1000) / h.activeMs) : '';
  return `${h.filename}\n${formatBytes(h.size)} en ${formatDuration(h.activeMs)}${avg ? ` · media ${avg}` : ''}`;
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
  // Archivos guardados por el motor nativo (no están en chrome.downloads).
  openPath: async (m) => (await requireNative()).open(m.path),
  showPath: async (m) => (await requireNative()).show(m.path),
  statPath: async (m) => (await requireNative()).stat(m.path),
  detectNative: async () => {
    await backendsReady;
    nativeCheck.at = 0;
    const n = await getNative();
    return n ? n.info : null;
  },
};

async function requireNative() {
  const n = await getNative();
  if (!n) throw new Error('El motor nativo no está instalado.');
  return n;
}

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
  const s = await getSettings();
  if (ctx.length) (await getBrowser()).updateSettings(s).catch(() => {});
  nativeCheck?.backend?.updateSettings(s).catch(() => {});
});

chrome.notifications.onClicked.addListener((nid) => {
  chrome.notifications.clear(nid);
  openManager();
});

// ================================================================= motor nativo

const segSavedAt = new Map();

function wireNative(n) {
  n.on('progress', (p) => {
    ui.postMessage({ type: 'progress', ...p });
    // Copia de los segmentos para que el gestor pinte la barra aunque esté en pausa.
    const now = Date.now();
    if (now - (segSavedAt.get(p.id) || 0) > 2000) {
      segSavedAt.set(p.id, now);
      db.put('segments', {
        id: p.id, bytesDone: p.bytesDone, updatedAt: now,
        segments: p.segments.map(([start, end, pos]) => ({ start, end, pos })),
      }).catch(() => {});
    }
  });
  n.on('info', async (i) => {
    await db.patch('downloads', i.id, {
      nativePartPath: i.partPath,
      nativeFinalPath: i.finalPath,
      filename: basename(i.finalPath),
      mode: i.rangeOK ? 'multi' : 'single',
      resumable: i.rangeOK,
      ...(i.size >= 0 ? { size: i.size } : {}),
    });
    ui.postMessage({ type: 'changed', id: i.id });
  });
  n.on('notice', async (m) => {
    await db.patch('downloads', m.id, { notice: m.message });
    ui.postMessage({ type: 'changed', id: m.id });
  });
  n.on('status', onNativeStatus);
}

async function onNativeStatus({ id, status, path, size, error, lost }) {
  const rec = await db.get('downloads', id);
  if (!rec) return;
  if (status === 'done') {
    await completeNative(rec, path, size);
  } else if (status === 'downloading') {
    await db.patch('downloads', id, { status: 'downloading', error: null, errorKind: null });
  } else if (status === 'paused') {
    await db.patch('downloads', id, (cur) => (cur.status === 'error' ? {} : {
      status: 'paused',
      ...(lost ? { notice: 'El motor nativo se cerró: pulsa Reanudar para seguir donde iba.' } : {}),
    }));
  } else if (status === 'error') {
    await db.patch('downloads', id, { status: 'error', error, errorKind: null });
    notify(`turbodl-err-${id}`, 'Error en la descarga', `${rec.filename}\n${error || ''}`);
  }
  ui.postMessage({ type: 'changed', id });
  updateBadge();
}

async function completeNative(rec, path, size) {
  const entry = {
    id: rec.id,
    filename: basename(path) || rec.filename,
    path,
    size: size || rec.size,
    url: rec.originalUrl || rec.url,
    deliveryId: null,
    native: true,
    createdAt: rec.createdAt,
    completedAt: Date.now(),
    ...metricsOf(rec),
  };
  await db.put('history', entry);
  await db.del('segments', rec.id);
  await db.del('downloads', rec.id);
  segSavedAt.delete(rec.id);
  ui.postMessage({ type: 'changed', id: rec.id });
  chrome.notifications.clear(`turbodl-start-${rec.id}`);
  notify(`turbodl-done-${rec.id}`, 'Descarga terminada', doneMessage(entry));
  updateBadge();
}

/** Cabeceras que el navegador habría mandado y que el motor nativo tiene que poner a mano. */
async function nativeHeaders(rec) {
  const h = { 'User-Agent': navigator.userAgent };
  if (rec.referer) h.Referer = rec.referer;
  const cookie = await cookieHeaderFor(rec.url);
  if (cookie) h.Cookie = cookie;
  return h;
}

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
  jobs: async () => (await getBrowser()).send('jobs'),
  native: async () => { const n = await getNative(); return n ? n.info : null; },
  rules: () => chrome.declarativeNetRequest.getSessionRules(),
};
