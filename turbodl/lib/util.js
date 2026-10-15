// Utilidades compartidas (service worker, offscreen, worker y páginas).

export const MiB = 1024 * 1024;

export function formatBytes(n) {
  if (n == null || n < 0 || !Number.isFinite(n)) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(i === 0 ? 0 : n < 10 ? 2 : n < 100 ? 1 : 0)} ${units[i]}`;
}

export function formatSpeed(bps) {
  return bps > 0 ? `${formatBytes(bps)}/s` : '—';
}

export function formatEta(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return '—';
  seconds = Math.round(seconds);
  if (seconds < 60) return `${seconds} s`;
  const m = Math.floor(seconds / 60), s = seconds % 60;
  if (m < 60) return `${m} min ${String(s).padStart(2, '0')} s`;
  const h = Math.floor(m / 60);
  return `${h} h ${String(m % 60).padStart(2, '0')} min`;
}

export function hostOf(url) {
  try { return new URL(url).hostname.toLowerCase(); } catch { return ''; }
}

/** true si `host` es `domain` o un subdominio suyo. */
export function domainMatches(host, domain) {
  domain = domain.trim().toLowerCase().replace(/^\*?\.?/, '');
  if (!domain) return false;
  return host === domain || host.endsWith('.' + domain);
}

/** Nombre de archivo válido en Windows/macOS/Linux para chrome.downloads. */
export function sanitizeFilename(name) {
  name = String(name || '').replace(/[\u0000-\u001f\u007f<>:"/\\|?*~]/g, '_').trim();
  // Windows no admite nombres que terminen en punto o espacio, ni nombres reservados.
  name = name.replace(/[. ]+$/, '');
  if (/^(con|prn|aux|nul|com\d|lpt\d)(\..*)?$/i.test(name)) name = '_' + name;
  if (name.startsWith('.')) name = '_' + name;
  if (name.length > 200) {
    const dot = name.lastIndexOf('.');
    const ext = dot > 0 && name.length - dot <= 12 ? name.slice(dot) : '';
    name = name.slice(0, 200 - ext.length) + ext;
  }
  return name || 'descarga';
}

export function basename(path) {
  return String(path || '').split(/[\\/]/).pop();
}

export function filenameFromUrl(url) {
  try {
    const last = new URL(url).pathname.split('/').filter(Boolean).pop();
    return last ? decodeURIComponent(last) : '';
  } catch { return ''; }
}

/** Extrae el nombre de un Content-Disposition (soporta filename*=UTF-8''...). */
export function parseContentDisposition(header) {
  if (!header) return '';
  const star = /filename\*\s*=\s*([^']*)'[^']*'([^;]+)/i.exec(header);
  if (star) {
    try { return decodeURIComponent(star[2].trim().replace(/^"|"$/g, '')); } catch { /* sigue */ }
  }
  const plain = /filename\s*=\s*("((?:\\.|[^"\\])*)"|[^;]+)/i.exec(header);
  if (plain) return (plain[2] ?? plain[1]).replace(/\\(.)/g, '$1').trim();
  return '';
}

/** Content-Range: "bytes 0-0/12345" → {start, end, total} (total null si es "*"). */
export function parseContentRange(header) {
  const m = /^\s*bytes\s+(\d+)-(\d+)\/(\d+|\*)\s*$/i.exec(header || '');
  if (!m) return null;
  return { start: Number(m[1]), end: Number(m[2]), total: m[3] === '*' ? null : Number(m[3]) };
}

/** Validador para If-Range: solo ETag fuerte (los débiles W/ no valen) o Last-Modified. */
export function ifRangeValidator(rec) {
  if (rec.etag && !/^W\//i.test(rec.etag)) return rec.etag;
  return rec.lastModified || null;
}

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
