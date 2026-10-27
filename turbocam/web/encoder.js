"use strict";
// Codificador de TurboCAM: pinta el fotograma y lo pasa a JPEG fuera del hilo de la
// página. Hay varios a la vez (uno por núcleo libre), así que mientras uno comprime
// un fotograma otro ya está con el siguiente.
importScripts("paint.js");

let canvas = null, ctx = null;

self.onmessage = async (e) => {
  const { id, bitmap, g, q } = e.data;
  const t0 = performance.now();
  try {
    if (!canvas || canvas.width !== g.w || canvas.height !== g.h) {
      canvas = new OffscreenCanvas(g.w, g.h);
      ctx = canvas.getContext("2d", { alpha: false });
    }
    paintFrame(ctx, bitmap, g, true);
    bitmap.close();
    const blob = await canvas.convertToBlob({ type: "image/jpeg", quality: q });
    self.postMessage({ id, blob, ms: performance.now() - t0 });
  } catch (err) {
    try { bitmap.close(); } catch { /* ya cerrado */ }
    self.postMessage({ id, error: String(err && err.message || err) });
  }
};
