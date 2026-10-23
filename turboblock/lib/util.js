// Utilidades compartidas por el service worker y las páginas de la extensión.
// (Los scripts de contenido no pueden importar módulos: copian lo que necesitan.)

// Debe coincidir con cosmetic.js y con fnv32 de native/compile.go.
export function fnv(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

function hash2(s) {
  let h = 5381;
  for (let i = 0; i < s.length; i++) h = Math.imul(h, 33) ^ s.charCodeAt(i);
  return (h >>> 0) | 1;
}

// Reparto de los datos cosméticos en chrome.storage.local: un script de contenido
// solo lee las porciones de las claves que le tocan.
export const SPEC_SHARDS = 512;
export const GEN_SHARDS = 512;
export const specShard = (key) => 'c' + (fnv(key) % SPEC_SHARDS);
export const genShard = (token) => 'g' + (fnv(token) % GEN_SHARDS);

// Filtro de Bloom con las clases/ids que tienen selectores genéricos: el script de
// contenido mira cada clase de la página aquí antes de pedir nada.
export function makeBloom(tokens) {
  const n = Math.max(tokens.length, 1);
  let m = 1024;
  while (m < n * 12) m *= 2;
  const k = 7;
  const bits = new Uint8Array(m >> 3);
  for (const t of tokens) {
    const h1 = fnv(t), h2 = hash2(t);
    for (let i = 0; i < k; i++) {
      const idx = (h1 + Math.imul(i, h2)) & (m - 1);
      bits[idx >> 3] |= 1 << (idx & 7);
    }
  }
  let bin = '';
  for (let i = 0; i < bits.length; i += 0x8000) bin += String.fromCharCode.apply(null, bits.subarray(i, i + 0x8000));
  return { m, k, b: btoa(bin) };
}

export function bloomHas(bloom, bin, t) {
  const h1 = fnv(t), h2 = hash2(t);
  for (let i = 0; i < bloom.k; i++) {
    const idx = (h1 + Math.imul(i, h2)) & (bloom.m - 1);
    if (!((bin.charCodeAt(idx >> 3) >> (idx & 7)) & 1)) return false;
  }
  return true;
}

// Dominio "registrable" aproximado (sin lista de sufijos públicos): las dos últimas
// etiquetas, o tres si la penúltima es corta (co.uk, com.es, gob.mx...).
export function siteOf(host) {
  if (!host) return '';
  if (/^\d+\.\d+\.\d+\.\d+$/.test(host) || host.includes(':')) return host;
  const p = host.split('.');
  if (p.length <= 2) return host;
  const sld = p[p.length - 2];
  const n = sld.length <= 3 && ['co', 'com', 'net', 'org', 'gov', 'gob', 'edu', 'ac', 'or', 'ne', 'go'].includes(sld) ? 3 : 2;
  return p.slice(-n).join('.');
}

export function hostOf(url) {
  try { return new URL(url).hostname; } catch { return ''; }
}

// ¿host es dominio o subdominio de alguno de la lista?
export function hostIn(host, list) {
  if (!host) return false;
  for (const d of list) {
    if (host === d || host.endsWith('.' + d)) return true;
  }
  return false;
}

// Patrones de coincidencia de Chrome para un dominio y sus subdominios.
export const matchPattern = (h) => `*://*.${h}/*`;

export function normalizeHost(s) {
  s = String(s || '').trim().toLowerCase();
  if (!s) return '';
  if (s.includes('://')) s = hostOf(s);
  s = s.replace(/^\*\./, '').replace(/^www\./, '').replace(/\/.*$/, '').replace(/:\d+$/, '');
  return /^[a-z0-9.-]+$/.test(s) && s.includes('.') || /^localhost$/.test(s) ? s : '';
}

export function fmtNum(n) {
  return new Intl.NumberFormat('es-ES').format(n || 0);
}

export function timeAgo(date) {
  if (!date) return 'nunca';
  const s = Math.max(0, (Date.now() - new Date(date).getTime()) / 1000);
  if (s < 60) return 'hace un momento';
  if (s < 3600) return `hace ${Math.round(s / 60)} min`;
  if (s < 86400) return `hace ${Math.round(s / 3600)} h`;
  const d = Math.round(s / 86400);
  return d === 1 ? 'hace 1 día' : `hace ${d} días`;
}
