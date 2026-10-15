// Capa de backend: quién hace el trabajo pesado de descargar.
//
//   interface Backend {
//     start(id, settings)   empezar una descarga ya registrada en IndexedDB
//     pause(id)             parar guardando el progreso
//     resume(id, settings)  seguir donde se quedó
//     cancel(id)            parar y borrar el archivo temporal
//     onProgress(cb)        cb({id, bytesDone, size, speed, eta, connections, segments})
//   }
//
// InBrowserBackend: offscreen + workers + OPFS, todo dentro de Chrome.
// NativeBackend: turbodl-native.exe (Go) por Native Messaging, si está instalado.

export class Backend {
  async start(_id, _settings) { throw new Error('No implementado'); }
  async pause(_id) { throw new Error('No implementado'); }
  async resume(_id, _settings) { throw new Error('No implementado'); }
  async cancel(_id) { throw new Error('No implementado'); }
  onProgress(_cb) { throw new Error('No implementado'); }
}

// ---------------------------------------------------------------- en el navegador

const OFFSCREEN_URL = 'offscreen.html';
let creating = null;

export async function ensureOffscreen() {
  const url = chrome.runtime.getURL(OFFSCREEN_URL);
  const existing = await chrome.runtime.getContexts({ contextTypes: ['OFFSCREEN_DOCUMENT'], documentUrls: [url] });
  if (existing.length) return;
  // Evita crear dos a la vez si llegan varias órdenes seguidas.
  creating ??= chrome.offscreen.createDocument({
    url: OFFSCREEN_URL,
    reasons: ['BLOBS', 'WORKERS'],
    justification: 'Descargar por partes en segundo plano y entregar el archivo terminado.',
  }).catch((e) => { if (!/single offscreen/i.test(e.message)) throw e; })
    .finally(() => { creating = null; });
  await creating;
}

export class InBrowserBackend extends Backend {
  kind = 'browser';

  constructor() {
    super();
    this.channel = new BroadcastChannel('turbodl');
  }

  async send(cmd, data = {}) {
    await ensureOffscreen();
    let lastError;
    // Recién creado, el documento puede tardar un instante en registrar su listener.
    for (let i = 0; i < 5; i++) {
      try {
        const r = await chrome.runtime.sendMessage({ target: 'offscreen', cmd, ...data });
        if (r === undefined) throw new Error('Sin respuesta del motor');
        if (!r.ok) throw new Error(r.error);
        return r.result;
      } catch (e) {
        lastError = e;
        if (!/Receiving end|Sin respuesta/.test(e.message)) throw e;
        await new Promise((res) => setTimeout(res, 100 * (i + 1)));
      }
    }
    throw lastError;
  }

  start(id, settings) { return this.send('start', { id, settings }); }
  resume(id, settings) { return this.send('start', { id, settings }); }
  pause(id) { return this.send('pause', { id }); }
  cancel(id) { return this.send('cancel', { id }); }
  updateSettings(settings) { return this.send('settings', { settings }); }

  onProgress(cb) {
    this.channel.addEventListener('message', ({ data }) => { if (data?.type === 'progress') cb(data); });
  }

  // Extras propios de este backend (entrega y mantenimiento de OPFS).
  fileUrl(id) { return this.send('fileUrl', { id }); }
  finalize(id) { return this.send('finalize', { id }); }
  cleanup(knownIds) { return this.send('cleanup', { knownIds }); }

  /** Aparta una conexión hacia `host` (un origen) mientras el SW sondea (solo si el motor ya existe). */
  async reserveSlot(host) {
    const ctx = await chrome.runtime.getContexts({ contextTypes: ['OFFSCREEN_DOCUMENT'] });
    if (!ctx.length) return () => {};
    const ok = await this.send('reserve', { host }).catch(() => false);
    return ok ? () => this.send('release', { host }).catch(() => {}) : () => {};
  }
}

// ---------------------------------------------------------------- nativo (Go)

/**
 * Motor nativo (turbodl-native.exe) por Native Messaging. Descarga fuera de Chrome:
 * sin el límite de 6 conexiones por servidor, una conexión TCP por trabajador aunque
 * el servidor sea HTTP/2, y escribe directamente en la carpeta de destino.
 *
 * El puerto solo se mantiene abierto mientras hay descargas (un puerto nativo abierto
 * mantiene vivo el service worker); sin descargas se cierra a los 20 s y el proceso
 * termina. Al cerrarse, el host pausa lo que tenga guardando el progreso.
 */
export class NativeBackend extends Backend {
  static HOST = 'com.turbodl.native';
  kind = 'native';

  constructor() {
    super();
    this.port = null;
    this.reqId = 0;
    this.pending = new Map();
    this.active = new Set();
    this.handlers = { progress: [], status: [], info: [], notice: [] };
    this.idleTimer = null;
    this.info = null;
  }

  /** Devuelve una instancia si el host nativo está instalado, o null. */
  static async detect() {
    const b = new NativeBackend();
    try {
      b.info = await b.request('hello', {}, 3000);
      b.scheduleIdle();
      return b;
    } catch {
      b.disconnect();
      return null;
    }
  }

  connect() {
    if (this.port) return this.port;
    const port = chrome.runtime.connectNative(NativeBackend.HOST);
    port.onMessage.addListener((m) => this.onMessage(m));
    port.onDisconnect.addListener(() => {
      const why = chrome.runtime.lastError?.message || 'El motor nativo se cerró.';
      if (this.port === port) this.port = null;
      for (const p of this.pending.values()) p.reject(new Error(why));
      this.pending.clear();
      // Si se cerró con descargas en marcha (cierre inesperado), quedan en pausa.
      for (const id of this.active) this.emit('status', { id, status: 'paused', lost: true });
      this.active.clear();
    });
    this.port = port;
    return port;
  }

  disconnect() {
    clearTimeout(this.idleTimer);
    try { this.port?.disconnect(); } catch { /* ya cerrado */ }
    this.port = null;
  }

  scheduleIdle() {
    clearTimeout(this.idleTimer);
    if (!this.active.size) this.idleTimer = setTimeout(() => { if (!this.active.size) this.disconnect(); }, 20_000);
  }

  request(cmd, data = {}, timeout = 15_000) {
    const port = this.connect();
    clearTimeout(this.idleTimer);
    const reqId = ++this.reqId;
    return new Promise((resolve, reject) => {
      const t = setTimeout(() => { this.pending.delete(reqId); reject(new Error('El motor nativo no responde')); }, timeout);
      this.pending.set(reqId, {
        resolve: (v) => { clearTimeout(t); resolve(v); },
        reject: (e) => { clearTimeout(t); reject(e); },
      });
      try { port.postMessage({ reqId, cmd, ...data }); } catch (e) { this.pending.delete(reqId); clearTimeout(t); reject(e); }
    }).finally(() => this.scheduleIdle());
  }

  onMessage(m) {
    if (m.type === 'reply') {
      const p = this.pending.get(m.reqId);
      if (!p) return;
      this.pending.delete(m.reqId);
      m.ok ? p.resolve(m.result) : p.reject(new Error(m.error));
      return;
    }
    if (m.type === 'progress') {
      const p = m.progress;
      this.emit('progress', {
        id: m.id,
        bytesDone: p.bytesDone,
        size: p.size >= 0 ? p.size : null,
        speed: p.speed,
        eta: p.eta >= 0 ? p.eta : null,
        connections: p.connections,
        segments: (p.segments || []).map(([start, end, pos, active]) => [start, end >= 0 ? end : null, pos, active]),
      });
    } else if (m.type === 'status') {
      if (m.status !== 'downloading') this.active.delete(m.id);
      this.emit('status', m);
      this.scheduleIdle();
    } else if (m.type === 'info') {
      this.emit('info', { id: m.id, ...m.info });
    } else if (m.type === 'notice') {
      this.emit('notice', m);
    }
  }

  on(event, cb) { this.handlers[event].push(cb); }
  emit(event, data) { for (const cb of this.handlers[event]) { try { cb(data); } catch (e) { console.warn(e); } } }
  onProgress(cb) { this.on('progress', cb); }

  /** rec: el registro de IndexedDB; headers: Cookie, Referer y User-Agent del navegador. */
  async start(id, settings, rec, headers = {}) {
    this.active.add(id);
    await this.request('start', {
      id,
      url: rec.url,
      filename: rec.filename,
      partPath: rec.nativePartPath || '',
      dir: settings.nativeDir || '',
      headers,
      connections: settings.nativeConnections,
    });
  }

  resume(id, settings, rec, headers) { return this.start(id, settings, rec, headers); }
  pause(id) { return this.request('pause', { id }); }
  cancel(id, rec) { return this.request('cancel', { id, partPath: rec?.nativePartPath || '' }); }
  async updateSettings(settings) {
    if (this.port) await this.request('setConnections', { n: settings.nativeConnections });
  }
  open(path) { return this.request('open', { path }); }
  show(path) { return this.request('show', { path }); }
  stat(path) { return this.request('stat', { path }); }
}

/**
 * Los dos motores. Las descargas nuevas usan el nativo si está instalado (y el usuario
 * no ha elegido "solo navegador"); las que ya existían siguen con el suyo.
 */
export async function createBackends() {
  return { browser: new InBrowserBackend(), native: await NativeBackend.detect() };
}
