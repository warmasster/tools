// Piezas de interfaz compartidas por el gestor y el popup.

import { formatBytes, formatSpeed, formatDuration } from '../lib/util.js';

export const ICONS = {
  pause: '<path d="M8 5v14M16 5v14"/>',
  play: '<path d="m7 4 13 8-13 8V4Z"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  retry: '<path d="M3 12a9 9 0 0 1 15.5-6.2L21 8M21 3v5h-5M21 12a9 9 0 0 1-15.5 6.2L3 16M3 21v-5h5"/>',
  open: '<path d="M14 3h7v7M21 3l-9 9M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
  chrome: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3.5"/><path d="M12 8.5h8.3M8.9 13.8 4.8 6.6M15 13.8l-4.2 7.1"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
};

export const svg = (name) =>
  `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${ICONS[name]}</svg>`;

export function fileType(name) {
  const m = /\.([a-z0-9]{1,5})$/i.exec(name || '');
  return m ? m[1].slice(0, 4) : 'bin';
}

/** Botón que copia el enlace de descarga y confirma con un ✓ un momento. */
export function copyButton(url, { cls = 'btn icon' } = {}) {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = `${cls} copy`;
  b.title = 'Copiar enlace de descarga';
  b.setAttribute('aria-label', 'Copiar enlace de descarga');
  b.innerHTML = svg('link');
  b.addEventListener('click', async (e) => {
    e.stopPropagation(); // en el popup, la fila entera abre el gestor
    try {
      await navigator.clipboard.writeText(url);
      b.innerHTML = svg('check');
      b.classList.add('copied');
      b.title = '¡Copiado!';
    } catch {
      b.title = 'No se pudo copiar';
    }
    clearTimeout(b.t);
    b.t = setTimeout(() => {
      b.innerHTML = svg('link');
      b.classList.remove('copied');
      b.title = 'Copiar enlace de descarga';
    }, 1500);
  });
  return b;
}

/** "2,32 GB en 32 s · media 72,5 MB/s" para una descarga terminada (o '' si no hay datos). */
export function metricsText(h) {
  if (!(h.activeMs > 0)) return '';
  const avg = h.size ? ` · media ${formatSpeed((h.size * 1000) / h.activeMs)}` : '';
  return `${formatBytes(h.size)} en ${formatDuration(h.activeMs)}${avg}`;
}
