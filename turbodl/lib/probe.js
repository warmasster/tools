// Sondeo rápido: un GET con Range: bytes=0-0 para saber si el servidor permite
// descargar por partes, el tamaño total, los validadores y el tipo de contenido.

import { parseContentRange, parseContentDisposition } from './util.js';

/**
 * @returns {Promise<{status:number, rangeOk:boolean, size:number|null, etag:string|null,
 *   lastModified:string|null, contentType:string, encoded:boolean, finalUrl:string,
 *   filename:string}>}
 */
export async function probe(url, { timeout = 3000 } = {}) {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(new Error('timeout')), timeout);
  try {
    const res = await fetch(url, {
      headers: { Range: 'bytes=0-0' },
      credentials: 'include',
      cache: 'no-store',
      signal: ctrl.signal,
    });
    const h = res.headers;
    const cr = res.status === 206 ? parseContentRange(h.get('content-range')) : null;
    const len = Number(h.get('content-length'));
    const encoding = (h.get('content-encoding') || '').toLowerCase();
    return {
      status: res.status,
      // Con compresión los bytes del cuerpo no corresponden a los rangos pedidos.
      rangeOk: !!(cr && cr.start === 0 && cr.total != null) && (!encoding || encoding === 'identity'),
      size: cr?.total ?? (res.status === 200 && Number.isFinite(len) && h.has('content-length') ? len : null),
      etag: h.get('etag'),
      lastModified: h.get('last-modified'),
      contentType: (h.get('content-type') || '').toLowerCase(),
      encoded: !!encoding && encoding !== 'identity',
      finalUrl: res.url || url,
      filename: parseContentDisposition(h.get('content-disposition')),
    };
  } finally {
    clearTimeout(timer);
    ctrl.abort(); // no leer el cuerpo (en un 200 podría ser el archivo entero)
  }
}
