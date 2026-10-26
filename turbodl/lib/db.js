// IndexedDB compartida por todos los contextos de la extensión (mismo origen).
//
// Almacenes:
//   downloads  metadatos y estado de cada descarga en curso (clave: id numérico)
//   segments   segmentos {start, end, pos} y bytes hechos; lo escribe solo el worker
//   history    descargas terminadas, para que el gestor pueda abrirlas o mostrarlas
//
// Los segmentos van en un almacén aparte para que el worker (que los guarda cada ~1 s)
// y el service worker (que cambia estados) nunca se pisen al escribir.

const DB_NAME = 'turbodl';
const DB_VERSION = 1;

let dbPromise = null;

export function openDB() {
  if (!dbPromise) {
    dbPromise = new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = () => {
        // Solo se crea lo que falte: así, el día que suba DB_VERSION, una base que ya
        // existe no falla al abrirse (createObjectStore lanzaría ConstraintError).
        const db = req.result;
        if (!db.objectStoreNames.contains('downloads')) {
          const dl = db.createObjectStore('downloads', { keyPath: 'id' });
          dl.createIndex('deliveryId', 'deliveryId');
        }
        if (!db.objectStoreNames.contains('segments')) db.createObjectStore('segments', { keyPath: 'id' });
        if (!db.objectStoreNames.contains('history')) db.createObjectStore('history', { keyPath: 'id' });
      };
      req.onsuccess = () => {
        const db = req.result;
        // Si otra versión pide actualizar, cerramos para no bloquearla.
        db.onversionchange = () => { db.close(); dbPromise = null; };
        resolve(db);
      };
      req.onerror = () => { dbPromise = null; reject(req.error); };
    });
  }
  return dbPromise;
}

function done(req) {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function tx(store, mode, fn) {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const t = db.transaction(store, mode);
    let result;
    Promise.resolve(fn(t.objectStore(store))).then((r) => { result = r; }, reject);
    t.oncomplete = () => resolve(result);
    t.onerror = () => reject(t.error);
    t.onabort = () => reject(t.error || new Error('Transacción abortada'));
  });
}

export const get = (store, key) => tx(store, 'readonly', (s) => done(s.get(key)));
export const getAll = (store) => tx(store, 'readonly', (s) => done(s.getAll()));
export const put = (store, value) => tx(store, 'readwrite', (s) => done(s.put(value)));
export const del = (store, key) => tx(store, 'readwrite', (s) => done(s.delete(key)));

/**
 * Tiempo activo de una descarga: suma lo que pasa en "downloading" (sin pausas ni
 * cola). Se calcula aquí porque todos los cambios de estado, de los dos motores y
 * de cualquier contexto, pasan por patch().
 */
function trackActiveTime(cur, next, now) {
  if (next.status === cur.status) return;
  if (cur.status === 'downloading' && cur.runningSince) {
    next.activeMs = (cur.activeMs || 0) + Math.max(0, now - cur.runningSince);
    next.runningSince = null;
  }
  if (next.status === 'downloading') next.runningSince = now;
}

/**
 * Lee, modifica y escribe un registro dentro de una misma transacción, así dos
 * contextos que cambian campos distintos a la vez no se sobrescriben.
 * Devuelve el registro actualizado o null si no existía.
 */
export function patch(store, key, changes) {
  return tx(store, 'readwrite', async (s) => {
    const cur = await done(s.get(key));
    if (!cur) return null;
    const now = Date.now();
    const next = { ...cur, ...(typeof changes === 'function' ? changes(cur) : changes), updatedAt: now };
    if (store === 'downloads') trackActiveTime(cur, next, now);
    await done(s.put(next));
    return next;
  });
}

export function getByIndex(store, index, value) {
  return tx(store, 'readonly', (s) => done(s.index(index).get(value)));
}
