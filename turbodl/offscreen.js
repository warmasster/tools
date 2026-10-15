// Documento offscreen: el "motor". Vive mientras haya trabajo, a diferencia del
// service worker, que MV3 duerme a los 30 s de inactividad.
//
// - Lanza un Dedicated Worker por descarga y reparte las conexiones respetando el
//   máximo por descarga, el de Chrome por servidor (6) y el tope global.
// - Crea el blob URL del archivo terminado para que el SW lo entregue a Descargas.
//
// Solo tiene acceso a chrome.runtime: los ajustes se los pasa el SW en cada orden.

import * as db from './lib/db.js';
import { PER_HOST_MAX } from './lib/settings.js';
// Chrome agrupa el límite de 6 conexiones por origen (esquema + host + puerto).
const originOf = (url) => { try { return new URL(url).origin; } catch { return url; } };

const ui = new BroadcastChannel('turbodl'); // progreso de alta frecuencia → gestor
const jobs = new Map(); // id → { id, host, mode, worker, alloc, status, waiters }
const blobUrls = new Map(); // id → blob URL de la entrega
const reserved = new Map(); // origen → conexiones apartadas para sondeos del SW
let settings = { connections: 6, maxTotalConnections: 12 };

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg?.target !== 'offscreen') return;
  handle(msg).then(
    (result) => sendResponse({ ok: true, result }),
    (e) => sendResponse({ ok: false, error: String(e?.message || e) }),
  );
  return true;
});

async function handle(msg) {
  if (msg.settings) settings = msg.settings;
  switch (msg.cmd) {
    case 'ping': return true;
    case 'start': return start(msg.id);
    case 'pause': return pause(msg.id);
    case 'cancel': return cancel(msg.id);
    case 'settings': schedule(); return true;
    case 'reserve': return reserve(msg.host);
    case 'release': release(msg.host); return true;
    case 'fileUrl': return fileUrl(msg.id);
    case 'finalize': return finalize(msg.id);
    case 'cleanup': return cleanup(new Set(msg.knownIds));
    case 'jobs': return [...jobs.values()].map(({ id, alloc, status, mode }) => ({ id, alloc, status, mode }));
    default: throw new Error(`Orden desconocida: ${msg.cmd}`);
  }
}

// ---------------------------------------------------------------- órdenes

async function start(id) {
  if (jobs.has(id)) return true; // idempotente: el SW puede repetir la orden al arrancar
  const rec = await db.get('downloads', id);
  if (!rec) throw new Error('La descarga no existe');
  jobs.set(id, { id, host: originOf(rec.url), mode: rec.mode, worker: null, alloc: 0, status: null, waiters: [] });
  schedule();
  return true;
}

async function pause(id) {
  const job = jobs.get(id);
  if (job?.worker) await ask(job, 'pause', 'paused');
  jobs.delete(id);
  await setStatus(id, 'paused');
  schedule();
  return true;
}

async function cancel(id) {
  const job = jobs.get(id);
  if (job?.worker) {
    await ask(job, 'cancel', 'cancelled');
  } else {
    const rec = await db.get('downloads', id);
    if (rec) await removeFile(rec.opfsName);
  }
  jobs.delete(id);
  schedule();
  return true;
}

/** Envía una orden al worker y espera el mensaje de confirmación (con tope de 5 s). */
function ask(job, cmd, reply) {
  return new Promise((resolve) => {
    const t = setTimeout(() => { stopWorker(job); resolve(); }, 5000);
    job.waiters.push({ reply, resolve: () => { clearTimeout(t); resolve(); } });
    job.worker.postMessage({ cmd });
  });
}

// ---------------------------------------------------------------- reserva para sondeos

/**
 * Chrome no abre más de 6 conexiones por servidor y las del SW comparten ese cupo con
 * las nuestras. Si ya descargamos de ese servidor con 6, el sondeo de una descarga
 * nueva no conseguiría conexión y caducaría. Por eso el SW pide apartar una mientras
 * sondea; la conexión cedida se retoma después en el mismo punto.
 */
async function reserve(host) {
  const busy = [...jobs.values()].some((j) => j.host === host && j.alloc > 0);
  if (!busy) return false;
  reserved.set(host, (reserved.get(host) || 0) + 1);
  schedule();
  setTimeout(() => release(host), 10_000); // por si el SW nunca la devuelve
  await new Promise((r) => setTimeout(r, 50)); // que el worker llegue a cerrar la conexión
  return true;
}

function release(host) {
  const n = (reserved.get(host) || 0) - 1;
  if (n < 0) return;
  if (n === 0) reserved.delete(host); else reserved.set(host, n);
  schedule();
}

// ---------------------------------------------------------------- planificador

/**
 * Reparte conexiones por orden de llegada: cada descarga recibe lo que pide
 * (1 en modo single) sin pasar del límite por servidor ni del tope global.
 * Las que se quedan a 0 esperan en cola.
 */
function schedule() {
  let globalLeft = settings.maxTotalConnections;
  const hostLeft = new Map();
  for (const job of jobs.values()) {
    const hl = hostLeft.get(job.host) ?? PER_HOST_MAX - (reserved.get(job.host) || 0);
    const want = job.mode === 'single' ? 1 : settings.connections;
    const n = Math.max(0, Math.min(want, hl, globalLeft));
    hostLeft.set(job.host, hl - n);
    globalLeft -= n;
    if (n > 0 && !job.worker) spawn(job, n);
    else if (job.worker && job.alloc !== n) job.worker.postMessage({ cmd: 'setMaxConnections', n });
    job.alloc = n;
    const status = n > 0 ? 'downloading' : 'queued';
    if (job.status !== status) { job.status = status; setStatus(job.id, status); }
  }
}

function spawn(job, n) {
  const w = new Worker(new URL('./download-worker.js', import.meta.url), { type: 'module' });
  job.worker = w;
  w.onmessage = ({ data }) => onWorkerMessage(job, data);
  w.onerror = (e) => onWorkerMessage(job, { type: 'error', message: e.message || 'Error interno del motor' });
  w.postMessage({ cmd: 'init', id: job.id, maxConns: n });
}

function stopWorker(job) {
  job.worker?.terminate();
  job.worker = null;
}

async function onWorkerMessage(job, m) {
  const waiter = job.waiters.find((w) => w.reply === m.type);
  if (waiter) {
    job.waiters = job.waiters.filter((w) => w !== waiter);
    stopWorker(job);
    waiter.resolve();
    return;
  }
  switch (m.type) {
    case 'progress':
      ui.postMessage({ type: 'progress', id: job.id, ...m });
      break;
    case 'mode':
      job.mode = m.mode;
      schedule();
      break;
    case 'notice':
      ui.postMessage({ type: 'changed', id: job.id });
      break;
    case 'written':
      stopWorker(job);
      jobs.delete(job.id);
      schedule();
      await setStatus(job.id, 'ready');
      break;
    case 'error':
      stopWorker(job);
      jobs.delete(job.id);
      schedule();
      await setStatus(job.id, 'error', { error: m.message });
      break;
  }
}

/** Guarda el estado y avisa al SW (badge, entrega, notificaciones) y al gestor. */
async function setStatus(id, status, extra = {}) {
  const rec = await db.patch('downloads', id, { status, ...extra, ...(status === 'error' ? {} : { error: null }) });
  if (!rec) return;
  ui.postMessage({ type: 'changed', id });
  chrome.runtime.sendMessage({ target: 'background', type: 'status', id, status, error: extra.error }).catch(() => {});
}

// ---------------------------------------------------------------- archivos OPFS

async function fileUrl(id) {
  const rec = await db.get('downloads', id);
  if (!rec) throw new Error('La descarga no existe');
  if (blobUrls.has(id)) URL.revokeObjectURL(blobUrls.get(id));
  const root = await navigator.storage.getDirectory();
  const file = await (await root.getFileHandle(rec.opfsName)).getFile();
  // El File de OPFS no trae tipo y Chrome lo adivinaría (p. ej. text/plain) y cambiaría
  // la extensión a .txt. Con octet-stream respeta el nombre que ya decidimos. slice()
  // crea otro blob sobre los mismos datos, sin copiarlos a memoria.
  const url = URL.createObjectURL(file.slice(0, file.size, 'application/octet-stream'));
  blobUrls.set(id, url);
  return url;
}

async function finalize(id) {
  if (blobUrls.has(id)) URL.revokeObjectURL(blobUrls.get(id));
  blobUrls.delete(id);
  const rec = await db.get('downloads', id);
  await removeFile(rec?.opfsName || `dl-${id}.part`);
  return true;
}

async function removeFile(name) {
  const root = await navigator.storage.getDirectory();
  await root.removeEntry(name).catch(() => {});
}

/** Borra de OPFS los archivos que no tienen descarga asociada (restos de cierres bruscos). */
async function cleanup(knownIds) {
  const root = await navigator.storage.getDirectory();
  const removed = [];
  for await (const name of root.keys()) {
    const m = /^dl-(\d+)\.part$/.exec(name);
    const id = m && Number(m[1]);
    if (m && (knownIds.has(id) || jobs.has(id))) continue;
    await root.removeEntry(name, { recursive: true }).catch(() => {});
    removed.push(name);
  }
  return removed;
}
