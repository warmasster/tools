// Ajustes en chrome.storage.local (solo SW, popup y gestor; el offscreen no tiene
// chrome.storage, así que el SW le pasa una copia con cada orden).

export const PER_HOST_MAX = 6; // Chrome no abre más de 6 conexiones HTTP/1.1 por servidor.
export const NATIVE_MAX = 64; // el motor nativo (Go) no tiene ese límite

export const DEFAULTS = {
  enabled: true,
  minSizeMB: 10,
  connections: 6, // por descarga (1-6)
  maxTotalConnections: 12, // entre todas las descargas
  excludedDomains: [],
  engine: 'auto', // 'auto' = motor nativo si está instalado; 'browser' = siempre el del navegador
  nativeConnections: 16,
  nativeDir: '', // vacío = carpeta Descargas del usuario
};

export async function getSettings() {
  const s = await chrome.storage.local.get('settings');
  return normalize({ ...DEFAULTS, ...(s.settings || {}) });
}

export async function setSettings(changes) {
  const next = normalize({ ...(await getSettings()), ...changes });
  await chrome.storage.local.set({ settings: next });
  return next;
}

function clampInt(v, min, max, def) {
  v = Math.round(Number(v));
  return Number.isFinite(v) ? Math.min(max, Math.max(min, v)) : def;
}

function normalize(s) {
  return {
    enabled: !!s.enabled,
    minSizeMB: Math.max(0, Number(s.minSizeMB) || 0),
    connections: clampInt(s.connections, 1, PER_HOST_MAX, DEFAULTS.connections),
    maxTotalConnections: clampInt(s.maxTotalConnections, 1, 64, DEFAULTS.maxTotalConnections),
    engine: s.engine === 'browser' ? 'browser' : 'auto',
    nativeConnections: clampInt(s.nativeConnections, 1, NATIVE_MAX, DEFAULTS.nativeConnections),
    nativeDir: String(s.nativeDir || ''),
    excludedDomains: (Array.isArray(s.excludedDomains) ? s.excludedDomains : String(s.excludedDomains || '').split(/[\s,]+/))
      .map((d) => d.trim().toLowerCase()).filter(Boolean),
  };
}
