// Genera los iconos (icons/icon{16,32,48,128}.png) y las imágenes transparentes
// de resources/ sin dependencias (dibujo con supermuestreo y PNG propio sobre zlib).
//   node dev/make-assets.js

import { writeFileSync, mkdirSync } from 'node:fs';
import { deflateSync } from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
mkdirSync(join(root, 'icons'), { recursive: true });
mkdirSync(join(root, 'resources'), { recursive: true });

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
// Escudo: arriba recto con hombros, abajo en curva hasta la punta.
const shield = [[0.5, 0.15], [0.78, 0.25], [0.78, 0.48]];
for (let t = 0.1; t < 1; t += 0.1) {
  const a = (t * Math.PI) / 2;
  shield.push([0.5 + 0.28 * Math.cos(a), 0.48 + 0.38 * Math.sin(a)]);
}
for (let t = 0.9; t > 0; t -= 0.1) {
  const a = (t * Math.PI) / 2;
  shield.push([0.5 - 0.28 * Math.cos(a), 0.48 + 0.38 * Math.sin(a)]);
}
shield.push([0.22, 0.48], [0.22, 0.25]);
const bar = (x, y) => x >= 0.35 && x <= 0.65 && y >= 0.445 && y <= 0.535;

// Degradado violeta → índigo → cian, como el resto de herramientas.
function grad(t) {
  const stops = [[0, [139, 92, 246]], [0.5, [99, 102, 241]], [1, [6, 182, 212]]];
  for (let i = 1; i < stops.length; i++) {
    if (t <= stops[i][0]) {
      const [t0, c0] = stops[i - 1], [t1, c1] = stops[i];
      const k = (t - t0) / (t1 - t0);
      return c0.map((v, j) => Math.round(v + (c1[j] - v) * k));
    }
  }
  return stops[stops.length - 1][1];
}
function pixel(x, y) {
  if (!inRoundRect(x, y, 0.23)) return null;
  const g = grad((x + y) / 2);
  if (inPolygon(x, y, shield)) return bar(x, y) ? g : [255, 255, 255];
  return g;
}

function render(size) {
  const ss = 4;
  const data = Buffer.alloc(size * (size * 4 + 1));
  for (let py = 0; py < size; py++) {
    data[py * (size * 4 + 1)] = 0;
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

function transparent(w, h) {
  const data = Buffer.alloc(h * (w * 4 + 1));
  return png(w, h, data);
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
  ihdr[8] = 8; ihdr[9] = 6;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0)),
  ]);
}

for (const size of [16, 32, 48, 128]) {
  writeFileSync(join(root, 'icons', `icon${size}.png`), render(size));
  console.log(`icons/icon${size}.png`);
}
for (const [w, h] of [[2, 2], [3, 2], [32, 32]]) {
  writeFileSync(join(root, 'resources', `${w}x${h}.png`), transparent(w, h));
}
writeFileSync(join(root, 'resources', '1x1.gif'), Buffer.from('47494638396101000100800000000000ffffff21f90401000000002c00000000010001000002024401003b', 'hex'));
// Audio mudo de 0,1 s: 4 tramas MPEG-1 Layer III (32 kbps, 44,1 kHz, mono) a cero.
const frame = Buffer.alloc(104);
frame.set([0xff, 0xfb, 0x10, 0xc4]);
writeFileSync(join(root, 'resources', 'noop-0.1s.mp3'), Buffer.concat([frame, frame, frame, frame]));
// Respuestas de anuncios de vídeo vacías (VAST y VMAP): el reproductor sigue con el vídeo.
const xml = '<?xml version="1.0" encoding="UTF-8"?>\n';
for (const v of ['2', '3', '4']) writeFileSync(join(root, 'resources', `noop-vast${v}.xml`), `${xml}<VAST version="${v}.0"></VAST>\n`);
writeFileSync(join(root, 'resources', 'noop-vmap1.xml'), `${xml}<vmap:VMAP xmlns:vmap="http://www.iab.net/videosuite/vmap" version="1.0"></vmap:VMAP>\n`);
console.log('resources/*.png, 1x1.gif, noop-0.1s.mp3, noop-vast*.xml, noop-vmap1.xml');
