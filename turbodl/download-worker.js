// Worker de una descarga. Hace los fetch por rangos y escribe en OPFS.
//
// Solo puede haber un FileSystemSyncAccessHandle abierto por archivo, por eso todas
// las conexiones de una descarga viven en este mismo worker. Las escrituras son
// síncronas: entre dos `await` nadie más toca el archivo ni los segmentos, así que
// no hacen falta cerrojos.
//
// Mensajes que recibe:  init {id, maxConns} · setMaxConnections {n} · pause · cancel
// Mensajes que envía:   progress · mode · notice · written · paused · cancelled · error

import * as db from './lib/db.js';
import { MiB, parseContentRange, ifRangeValidator, sleep } from './lib/util.js';

const MIN_SEGMENT = 1 * MiB; // al robar trabajo, cada mitad debe tener al menos esto
const MAX_RETRIES = 5;
const PERSIST_MS = 1000;
const PROGRESS_MS = 500;
const STALL_MS = 30_000; // sin recibir bytes durante este tiempo → reintentar
const SPEED_WINDOW_MS = 3000;

let rec; // registro de `downloads`
let segs = []; // [{start, end, pos}]; end = null si el tamaño es desconocido (modo single)
let access; // FileSystemSyncAccessHandle
let maxConns = 0;
let gen = 0; // sube al reiniciar la descarga; las conexiones de otra generación se descartan
let stopping = false;
let finished = false;
const active = new Map(); // segmento → { ctrl, yielded }
const samples = []; // [{t, bytes}] para la media móvil de velocidad
let timers = [];

class HttpError extends Error {
  constructor(status) {
    super(`El servidor respondió ${status}`);
    this.status = status;
    // Estos errores no se arreglan reintentando (enlace caducado, sin permiso...).
    this.fatal = [401, 403, 404, 410].includes(status);
  }
}

class ChangedError extends Error {
  constructor(message, fresh) { super(message); this.fresh = fresh; }
}

self.onmessage = async ({ data }) => {
  try {
    switch (data.cmd) {
      case 'init': await init(data.id, data.maxConns); break;
      case 'setMaxConnections': setMaxConnections(data.n); break;
      case 'pause': await stop('paused'); break;
      case 'cancel': await stop('cancelled'); break;
    }
  } catch (e) {
    fail(e);
  }
};

// ---------------------------------------------------------------- arranque

async function init(id, n) {
  maxConns = n;
  rec = await db.get('downloads', id);
  if (!rec) throw new Error('La descarga ya no existe');
  const saved = await db.get('segments', id);

  const root = await navigator.storage.getDirectory();
  const fh = await root.getFileHandle(rec.opfsName, { create: true });
  access = await fh.createSyncAccessHandle();

  if (rec.mode === 'single') {
    // Sin soporte de Range no hay reanudación posible: siempre desde cero.
    if (saved && segBytes(saved.segments) > 0) notice('Este servidor no permite reanudar: la descarga empieza de cero.');
    resetSingle();
  } else if (saved?.segments?.length && access.getSize() === rec.size) {
    segs = saved.segments.map((s) => ({ start: s.start, end: s.end, pos: s.pos }));
  } else {
    if (saved?.segments?.length) notice('El archivo temporal no estaba completo: la descarga empieza de cero.');
    resetMulti();
  }

  timers.push(setInterval(persist, PERSIST_MS));
  timers.push(setInterval(reportProgress, PROGRESS_MS));
  reportProgress();
  fill();
  checkDone();
}

function resetMulti() {
  // Reparto inicial en partes iguales; luego el robo de trabajo equilibra.
  const parts = Math.max(1, Math.min(rec.connections || 6, Math.floor(rec.size / MIN_SEGMENT)));
  const step = Math.ceil(rec.size / parts);
  segs = [];
  for (let s = 0; s < rec.size; s += step) segs.push({ start: s, end: Math.min(rec.size, s + step), pos: s });
  if (rec.size === 0) segs = [{ start: 0, end: 0, pos: 0 }];
  // Reservar el tamaño final evita que el archivo crezca a saltos con cada escritura.
  access.truncate(0);
  access.truncate(rec.size);
}

function resetSingle() {
  segs = [{ start: 0, end: rec.size > 0 ? rec.size : null, pos: 0 }];
  access.truncate(0);
}

// ---------------------------------------------------------------- reparto de conexiones

function setMaxConnections(n) {
  maxConns = n;
  // Si nos quitan conexiones, cerramos las sobrantes; su segmento queda a medias y
  // lo retoma otra conexión cuando vuelva a haber hueco.
  const extra = [...active.values()].slice(n);
  for (const c of extra) { c.yielded = true; c.ctrl?.abort(); }
  fill();
}

function isDone(s) { return s.end !== null && s.pos >= s.end; }

function pickSegment() {
  // 1) Un segmento pendiente que no esté descargando nadie.
  const idle = segs.find((s) => !isDone(s) && !active.has(s));
  if (idle) return idle;
  if (rec.mode === 'single') return null;
  // 2) Robo de trabajo: partir por la mitad el segmento activo con más bytes pendientes.
  let best = null;
  for (const s of active.keys()) {
    if (isDone(s)) continue;
    if (!best || s.end - s.pos > best.end - best.pos) best = s;
  }
  if (!best || best.end - best.pos < 2 * MIN_SEGMENT) return null;
  const mid = best.pos + Math.floor((best.end - best.pos) / 2);
  const half = { start: mid, end: best.end, pos: mid };
  // La conexión que descargaba `best` pidió hasta el final antiguo, pero el bucle de
  // lectura mira `end` en cada trozo y corta al llegar al nuevo límite.
  best.end = mid;
  segs.splice(segs.indexOf(best) + 1, 0, half);
  return half;
}

function fill() {
  if (stopping || finished) return;
  while (active.size < maxConns) {
    const seg = pickSegment();
    if (!seg) break;
    const conn = { ctrl: null, yielded: false };
    active.set(seg, conn);
    runConnection(seg, conn, gen)
      .catch(fail)
      .finally(() => {
        if (active.get(seg) === conn) active.delete(seg);
        if (!stopping) { checkDone(); fill(); }
      });
  }
}

// ---------------------------------------------------------------- una conexión

async function runConnection(seg, conn, myGen) {
  let retries = 0;
  while (!stopping && !conn.yielded && myGen === gen && !isDone(seg)) {
    const ctrl = new AbortController();
    conn.ctrl = ctrl;
    const startPos = seg.pos;
    try {
      await downloadRange(seg, ctrl, myGen, conn);
    } catch (e) {
      if (stopping || conn.yielded || myGen !== gen) return;
      if (e instanceof ChangedError) { restartChanged(e.message, e.fresh); return; }
      if (e.fatal) throw e;
      if (seg.pos > startPos) retries = 0; // hubo progreso: el fallo es puntual
      if (++retries > MAX_RETRIES) throw new Error(`Demasiados fallos de conexión (${e.message || e})`);
      if (rec.mode === 'single') {
        // Sin Range no podemos seguir donde iba: vuelta a empezar.
        seg.pos = 0;
        access.truncate(0);
      }
      conn.ctrl = null; // en espera: no cuenta como conexión activa
      await sleep(1000 * 2 ** (retries - 1));
    }
  }
}

async function downloadRange(seg, ctrl, myGen, conn) {
  const headers = {};
  if (rec.mode === 'multi') {
    headers.Range = `bytes=${seg.pos}-${seg.end - 1}`;
    const v = ifRangeValidator(rec);
    if (v) headers['If-Range'] = v;
  }

  let stall = setTimeout(() => ctrl.abort(new Error('Conexión parada')), STALL_MS);
  try {
    const res = await fetch(rec.url, {
      headers, signal: ctrl.signal, credentials: 'include', cache: 'no-store',
    });
    if (myGen !== gen || stopping) { ctrl.abort(); return; }

    if (rec.mode === 'multi') {
      if (res.status === 200) { ctrl.abort(); handleFullResponse(res); return; }
      if (res.status !== 206) throw new HttpError(res.status);
      const cr = parseContentRange(res.headers.get('content-range'));
      if (!cr || cr.start !== seg.pos) throw new Error('Content-Range inesperado');
      if (cr.total !== rec.size) {
        throw new ChangedError('El tamaño del archivo ha cambiado en el servidor: se reinicia desde cero.', {
          size: cr.total, etag: res.headers.get('etag'), lastModified: res.headers.get('last-modified'),
        });
      }
    } else if (!res.ok) {
      throw new HttpError(res.status);
    }

    const reader = res.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      // Tras cada await: si nos han parado o la descarga se ha reiniciado, no escribir.
      if (myGen !== gen || stopping || conn.yielded) { ctrl.abort(); return; }
      if (done) break;
      clearTimeout(stall);
      stall = setTimeout(() => ctrl.abort(new Error('Conexión parada')), STALL_MS);

      let chunk = value;
      if (seg.end !== null && chunk.byteLength > seg.end - seg.pos) chunk = chunk.subarray(0, seg.end - seg.pos);
      access.write(chunk, { at: seg.pos });
      seg.pos += chunk.byteLength;
      if (seg.end !== null && seg.pos >= seg.end) {
        // Llegamos al final de nuestro trozo (quizá recortado por un robo): soltar la conexión.
        ctrl.abort();
        return;
      }
    }
    if (seg.end === null) { seg.end = seg.pos; return; }
    if (seg.pos < seg.end) throw new Error('La conexión se cerró antes de tiempo');
  } finally {
    clearTimeout(stall);
  }
}

/** Un 200 en lugar de 206 en modo multiconexión. */
function handleFullResponse(res) {
  const etag = res.headers.get('etag');
  const lastModified = res.headers.get('last-modified');
  const changed = (rec.etag && etag && etag !== rec.etag)
    || (!rec.etag && rec.lastModified && lastModified && lastModified !== rec.lastModified);
  if (changed) {
    // If-Range no coincidió: el servidor tiene otra versión del archivo.
    const len = Number(res.headers.get('content-length'));
    restartChanged('El archivo ha cambiado en el servidor: se reinicia desde cero.', {
      etag, lastModified, size: Number.isFinite(len) && len > 0 ? len : rec.size,
    });
  } else {
    // El servidor ignora Range: abortar el modo multiconexión y seguir con una sola.
    gen++;
    abortAll();
    rec = { ...rec, mode: 'single', resumable: false };
    resetSingle();
    db.patch('downloads', rec.id, { mode: 'single', resumable: false }).catch(() => {});
    self.postMessage({ type: 'mode', mode: 'single' });
    notice('El servidor dejó de aceptar descargas por partes: se continúa con una sola conexión desde el principio.');
    persist();
  }
}

function restartChanged(message, fresh = {}) {
  gen++;
  abortAll();
  rec = { ...rec, ...fresh };
  db.patch('downloads', rec.id, { etag: rec.etag, lastModified: rec.lastModified, size: rec.size }).catch(() => {});
  if (rec.mode === 'single') resetSingle(); else resetMulti();
  notice(message);
  persist();
}

function abortAll() {
  for (const c of active.values()) c.ctrl?.abort();
}

// ---------------------------------------------------------------- final, pausa, error

function checkDone() {
  if (finished || stopping || active.size > 0) return;
  if (!segs.every(isDone)) return;
  finished = true;
  clearTimers();
  const size = segs.reduce((m, s) => Math.max(m, s.end), 0);
  if (rec.mode === 'single') access.truncate(size); // tamaño real si no se conocía
  access.flush();
  access.close();
  access = null;
  const done = { id: rec.id, segments: snapshot(), bytesDone: size, updatedAt: Date.now() };
  db.put('segments', done).finally(() => {
    reportProgress();
    self.postMessage({ type: 'written', size });
  });
}

async function stop(kind) {
  if (finished) return;
  stopping = true;
  clearTimers();
  abortAll();
  if (access) {
    access.flush();
    if (kind === 'paused') await saveSegments();
    access.close();
    access = null;
  }
  if (kind === 'cancelled') {
    const root = await navigator.storage.getDirectory();
    await root.removeEntry(rec.opfsName).catch(() => {});
  }
  self.postMessage({ type: kind });
}

function fail(e) {
  if (stopping || finished) return;
  stopping = true;
  clearTimers();
  abortAll();
  const after = access ? (access.flush(), saveSegments()) : Promise.resolve();
  after.catch(() => {}).finally(() => {
    try { access?.close(); } catch { /* ya cerrado */ }
    access = null;
    self.postMessage({ type: 'error', message: errorText(e) });
  });
}

function errorText(e) {
  const m = String(e?.message || e);
  if (e?.name === 'QuotaExceededError') return 'No queda espacio en disco para seguir descargando.';
  if (e instanceof HttpError) {
    if (e.status === 403 || e.status === 401) return `El servidor denegó el acceso (${e.status}). Puede que el enlace haya caducado o haga falta iniciar sesión.`;
    if (e.status === 404 || e.status === 410) return `El archivo ya no existe en el servidor (${e.status}).`;
  }
  if (/Failed to fetch|NetworkError/i.test(m)) return 'No se pudo conectar con el servidor.';
  return m;
}

function clearTimers() { timers.forEach(clearInterval); timers = []; }

// ---------------------------------------------------------------- estado y progreso

function snapshot() { return segs.map((s) => ({ start: s.start, end: s.end, pos: s.pos })); }

function segBytes(list) { return list.reduce((n, s) => n + (s.pos - s.start), 0); }

/**
 * Guarda los segmentos. El orden importa: primero copiamos las posiciones, después
 * flush() y por último escribimos en IndexedDB. Así las posiciones guardadas nunca
 * van por delante de los datos que de verdad están en disco.
 */
function saveSegments() {
  const list = snapshot();
  access?.flush();
  return db.put('segments', { id: rec.id, segments: list, bytesDone: segBytes(list), updatedAt: Date.now() });
}

function persist() {
  if (!access || stopping || finished) return;
  saveSegments().catch(() => {});
}

function reportProgress() {
  const now = performance.now();
  const bytes = segBytes(segs);
  samples.push({ t: now, bytes });
  while (samples.length > 2 && now - samples[0].t > SPEED_WINDOW_MS) samples.shift();
  const first = samples[0];
  const speed = now > first.t ? ((bytes - first.bytes) * 1000) / (now - first.t) : 0;
  const total = rec.mode === 'single' && !(rec.size > 0) ? null : rec.size;
  self.postMessage({
    type: 'progress',
    bytesDone: bytes,
    size: total,
    speed: Math.max(0, speed),
    eta: total && speed > 0 ? (total - bytes) / speed : null,
    connections: [...active.values()].filter((c) => c.ctrl).length,
    segments: segs.map((s) => [s.start, s.end, s.pos, active.has(s) ? 1 : 0]),
  });
}

function notice(message) {
  db.patch('downloads', rec.id, { notice: message }).catch(() => {});
  self.postMessage({ type: 'notice', message });
}
