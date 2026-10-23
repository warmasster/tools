// Piezas de interfaz compartidas por el popup y los ajustes.

export const CAT_INFO = {
  ads: { name: 'Anuncios', color: '#8b5cf6', icon: '<path d="M3 11l18-5v12L3 14v-3z"/><path d="M11.6 16.8a3 3 0 1 1-5.8-1.6"/>' },
  privacy: { name: 'Rastreadores', color: '#22d3ee', icon: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/><path d="M3 3l18 18"/>' },
  malware: { name: 'Malware y estafas', color: '#f87171', icon: '<path d="M12 3l8 3v6c0 4.5-3.4 8.3-8 9-4.6-.7-8-4.5-8-9V6z"/><path d="M12 8v4"/><path d="M12 16h.01"/>' },
  cookies: { name: 'Avisos de cookies', color: '#fbbf24', icon: '<path d="M12 2a10 10 0 1 0 10 10 4 4 0 0 1-5-5 4 4 0 0 1-5-5"/><path d="M8.5 8.5h.01"/><path d="M16 15.5h.01"/><path d="M12 12h.01"/><path d="M11 17h.01"/><path d="M7 14h.01"/>' },
  annoyances: { name: 'Molestias', color: '#f472b6', icon: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/><path d="M4 2l16 20"/>' },
  popups: { name: 'Ventanas emergentes', color: '#34d399', icon: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/><path d="M9 13l6 4"/><path d="M15 13l-6 4"/>' },
  user: { name: 'Mis filtros', color: '#6366f1', icon: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>' },
  base: { name: 'Arreglos', color: '#5f6580', icon: '<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.8-3.8a6 6 0 0 1-7.9 7.9l-6.9 6.9a2.1 2.1 0 0 1-3-3l6.9-6.9a6 6 0 0 1 7.9-7.9z"/>' },
};

export const svg = (path, cls = '') => `<svg class="${cls}" viewBox="0 0 24 24" aria-hidden="true">${path}</svg>`;

/** Anillo SVG por categorías. parts: [[cat, n], ...] */
export function donut(parts, size = 92, stroke = 9) {
  const r = (size - stroke) / 2, c = 2 * Math.PI * r;
  const total = parts.reduce((s, [, n]) => s + n, 0);
  let off = 0;
  const gap = parts.length > 1 ? 2.5 : 0;
  const arcs = total ? parts.map(([cat, n]) => {
    const len = Math.max((n / total) * c - gap, 1.5);
    const el = `<circle r="${r}" cx="${size / 2}" cy="${size / 2}" fill="none" stroke="${CAT_INFO[cat]?.color || '#888'}" stroke-width="${stroke}"
      stroke-linecap="round" stroke-dasharray="${len} ${c}" stroke-dashoffset="${-off}" class="arc"/>`;
    off += (n / total) * c;
    return el;
  }).join('') : '';
  return `<svg class="donut" viewBox="0 0 ${size} ${size}" width="${size}" height="${size}">
    <circle r="${r}" cx="${size / 2}" cy="${size / 2}" fill="none" stroke="rgba(255,255,255,.07)" stroke-width="${stroke}"/>
    <g transform="rotate(-90 ${size / 2} ${size / 2})">${arcs}</g></svg>`;
}

/** Cuenta animada de 0 (o del valor actual) hasta n. */
export function countUp(el, n, fmt) {
  const from = Number(el.dataset.n || 0);
  el.dataset.n = n;
  if (from === n || matchMedia('(prefers-reduced-motion: reduce)').matches) { el.textContent = fmt(n); return; }
  const t0 = performance.now(), dur = Math.min(700, 250 + Math.abs(n - from) * 8);
  const step = (t) => {
    const k = Math.min(1, (t - t0) / dur);
    const e = 1 - Math.pow(1 - k, 3);
    el.textContent = fmt(Math.round(from + (n - from) * e));
    if (k < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

/** Últimos n días (de más antiguo a hoy) con sus totales por categoría. */
export function lastDays(days, n) {
  const out = [];
  const d = new Date();
  d.setHours(12, 0, 0, 0);
  d.setDate(d.getDate() - (n - 1));
  for (let i = 0; i < n; i++) {
    const k = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    out.push({ key: k, date: new Date(d), counts: days[k] || {} });
    d.setDate(d.getDate() + 1);
  }
  return out;
}

export const sumCounts = (o) => Object.entries(o || {}).reduce((s, [k, v]) => (k === 'since' ? s : s + (v || 0)), 0);

export function fmtBytes(kb) {
  if (kb < 1024) return `${Math.round(kb)} KB`;
  if (kb < 1024 * 1024) return `${(kb / 1024).toFixed(kb < 10 * 1024 ? 1 : 0).replace('.', ',')} MB`;
  return `${(kb / 1024 / 1024).toFixed(1).replace('.', ',')} GB`;
}
