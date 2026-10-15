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
// Hoy solo existe InBrowserBackend (offscreen + workers + OPFS). NativeBackend es el
// hueco para un futuro programa nativo vía Native Messaging.

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

// ---------------------------------------------------------------- nativo (futuro)

export class NativeBackend extends Backend {
  static HOST = 'com.turbodl.native';

  /** Devuelve una instancia si el host nativo está instalado, o null. */
  static async detect() {
    // Futuro: chrome.runtime.connectNative(NativeBackend.HOST) y esperar un "hello".
    // Requiere añadir el permiso "nativeMessaging" al manifest.
    return null;
  }
}

/**
 * Punto único de elección del backend. Para usar el nativo cuando exista bastará con:
 *   return (await NativeBackend.detect()) ?? new InBrowserBackend();
 */
export async function createBackend() {
  return new InBrowserBackend();
}
