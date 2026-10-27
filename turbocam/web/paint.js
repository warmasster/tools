"use strict";
// Pinta un fotograma con los retoques (giro, espejo, filtros) según la geometría que
// calcula phone.js. La usan la página (fotos y navegadores sin workers) y los
// codificadores en segundo plano (encoder.js).
//
// g: { w, h, tx, ty, rot, mirror, cx, cy, cw, ch, bw, bh, ox, oy, filter, fill }
//   (cx, cy, cw, ch) es la parte del vídeo que se ve; se pinta a bw x bh con su
//   centro en (ox, oy) respecto al centro del vídeo, ya girado y escalado.
// cropped: src ya es esa parte, a bw x bh (createImageBitmap la recortó y escaló).
function paintFrame(cx, src, g, cropped) {
  cx.setTransform(1, 0, 0, 1, 0, 0);
  if (g.fill) {
    cx.fillStyle = "#000";
    cx.fillRect(0, 0, g.w, g.h);
  }
  cx.filter = g.filter || "none";
  cx.translate(g.tx, g.ty);
  if (g.mirror) cx.scale(-1, 1);
  if (g.rot) cx.rotate(g.rot * Math.PI / 180);
  const x = g.ox - g.bw / 2, y = g.oy - g.bh / 2;
  if (cropped) cx.drawImage(src, x, y, g.bw, g.bh);
  else cx.drawImage(src, g.cx, g.cy, g.cw, g.ch, x, y, g.bw, g.bh);
  cx.setTransform(1, 0, 0, 1, 0, 0);
  cx.filter = "none";
}
