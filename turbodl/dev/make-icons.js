// Genera icons/icon{16,32,48,128}.png sin dependencias (dibujo vectorial simple con
// supermuestreo y codificador PNG propio sobre zlib).
//   node dev/make-icons.js

import { writeFileSync, mkdirSync } from 'node:fs';
import { deflateSync } from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const outDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'icons');
mkdirSync(outDir, { recursive: true });

// Formas en coordenadas 0..1.
function inRoundRect(x, y, r) {
  const cx = Math.min(Math.max(x, r), 1 - r), cy = Math.min(Math.max(y, r), 1 - r);
  return (x - cx) ** 2 + (y - cy) ** 2 <= r * r;
}
function inPolygon(x, y, pts) {
  let inside = false;
  for (let i = 0, j = pts.length - 1; i < pts.length; j = i++) {
    const [xi, yi] = pts[i], [xj, yj] = pts[j];
    if ((yi > y) !== (yj > y) && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) inside = !inside;
  }
  return inside;
}
// Flecha de descarga con un "rayo" de velocidad.
const arrow = [[0.40, 0.18], [0.60, 0.18], [0.60, 0.48], [0.76, 0.48], [0.50, 0.76], [0.24, 0.48], [0.40, 0.48]];
const bar = (x, y) => x >= 0.22 && x <= 0.78 && y >= 0.80 && y <= 0.87;
const lines = (x, y) => (y >= 0.27 && y <= 0.31 && x >= 0.16 && x <= 0.33) || (y >= 0.37 && y <= 0.41 && x >= 0.10 && x <= 0.33);

function pixel(x, y) {
  if (!inRoundRect(x, y, 0.22)) return null;
  if (inPolygon(x, y, arrow) || bar(x, y)) return [255, 255, 255];
  if (lines(x, y)) return [191, 219, 254];
  // Degradado azul → violeta.
  const t = (x + y) / 2;
  return [Math.round(37 + t * 87), Math.round(99 - t * 41), Math.round(235 + t * 2)];
}

function render(size) {
  const ss = 4;
  const data = Buffer.alloc(size * (size * 4 + 1));
  for (let py = 0; py < size; py++) {
    data[py * (size * 4 + 1)] = 0; // filtro "None"
    for (let px = 0; px < size; px++) {
      let r = 0, g = 0, b = 0, a = 0;
      for (let sy = 0; sy < ss; sy++) for (let sx = 0; sx < ss; sx++) {
        const c = pixel((px + (sx + 0.5) / ss) / size, (py + (sy + 0.5) / ss) / size);
        if (c) { r += c[0]; g += c[1]; b += c[2]; a++; }
      }
      const o = py * (size * 4 + 1) + 1 + px * 4;
      data[o] = a ? r / a : 0; data[o + 1] = a ? g / a : 0; data[o + 2] = a ? b / a : 0;
      data[o + 3] = Math.round((a / (ss * ss)) * 255);
    }
  }
  return png(size, size, data);
}

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});
function crc32(buf) {
  let c = 0xffffffff;
  for (const byte of buf) c = crcTable[(c ^ byte) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}
function chunk(type, body) {
  const len = Buffer.alloc(4); len.writeUInt32BE(body.length);
  const tb = Buffer.concat([Buffer.from(type), body]);
  const crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(tb));
  return Buffer.concat([len, tb, crc]);
}
function png(w, h, raw) {
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8; ihdr[9] = 6; // 8 bits, RGBA
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0)),
  ]);
}

for (const size of [16, 32, 48, 128]) {
  writeFileSync(join(outDir, `icon${size}.png`), render(size));
  console.log(`icons/icon${size}.png`);
}
