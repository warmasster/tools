// Ajustes del usuario (chrome.storage.local, clave "settings").

export const CATEGORIES = [
  { id: 'ads', name: 'Anuncios', hint: 'También los de vídeo (YouTube, Twitch…). EasyList, EasyList Spanish, uBlock y Peter Lowe' },
  { id: 'privacy', name: 'Rastreadores', hint: 'EasyPrivacy y uBlock Privacy' },
  { id: 'malware', name: 'Malware y estafas', hint: 'uBlock Badware y URLhaus' },
  { id: 'cookies', name: 'Avisos de cookies', hint: 'EasyList Cookie: oculta los carteles' },
  { id: 'annoyances', name: 'Molestias', hint: 'Botones sociales, carteles de suscripción, avisos de notificaciones…' },
];

export const DEFAULTS = {
  enabled: true,
  cats: { ads: true, privacy: true, malware: true, cookies: false, annoyances: false },
  popups: true, // bloquear ventanas emergentes
  strictPopups: true, // también las que no están en las listas
  allow: [], // webs donde TurboBLOCK no actúa
  popupAllow: [], // webs que pueden abrir ventanas
  userFilters: '',
  autoUpdate: true,
  noNotifications: false, // contestar «no» a «¿Permitir notificaciones?»
  toasts: true, // aviso en la página al cerrar una ventana emergente
};

export async function getSettings() {
  const { settings } = await chrome.storage.local.get('settings');
  const s = { ...DEFAULTS, ...(settings || {}) };
  s.cats = { ...DEFAULTS.cats, ...(settings?.cats || {}) };
  return s;
}

export async function saveSettings(patch) {
  const s = await getSettings();
  const next = { ...s, ...patch };
  if (patch.cats) next.cats = { ...s.cats, ...patch.cats };
  await chrome.storage.local.set({ settings: next });
  return next;
}
