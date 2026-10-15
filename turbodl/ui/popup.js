import * as db from '../lib/db.js';
import { formatSpeed } from '../lib/util.js';
import { mountSettingsForm, bindEnabledToggle, shortcutText } from './settings-form.js';

const ACTIVE = ['queued', 'downloading', 'ready', 'delivering'];
const speeds = new Map(); // id → {speed, t}

await bindEnabledToggle(document.getElementById('enabled'), (on) => {
  document.body.classList.toggle('off', !on);
  document.getElementById('state-text').textContent = on ? 'Interceptación activada' : 'Interceptación desactivada';
});
await mountSettingsForm(document.getElementById('settings'), { saved: document.querySelector('.settings .saved') });

document.getElementById('open-manager').addEventListener('click', async () => {
  await chrome.runtime.sendMessage({ target: 'background', cmd: 'openManager' });
  window.close();
});

async function showEngine() {
  const [{ engine }, { settings }] = await Promise.all([chrome.storage.local.get('engine'), chrome.storage.local.get('settings')]);
  const native = engine?.native && settings?.engine !== 'browser';
  document.getElementById('engine-text').textContent = native
    ? `Motor nativo (Go) ${engine.version} · ${settings?.nativeConnections ?? 16} conexiones`
    : 'Motor del navegador · máx. 6 conexiones por servidor';
}
showEngine();
chrome.storage.onChanged.addListener((c, area) => { if (area === 'local' && (c.engine || c.settings)) showEngine(); });
// Por si se acaba de instalar el motor: pedir al SW que lo vuelva a buscar.
chrome.runtime.sendMessage({ target: 'background', cmd: 'detectNative' }).catch(() => {});

const key = await shortcutText();
document.getElementById('shortcut').innerHTML = key
  ? `Atajo para activar/desactivar: <kbd>${key}</kbd>`
  : 'Puedes asignar un atajo en chrome://extensions/shortcuts';

async function refreshCount() {
  const all = await db.getAll('downloads');
  document.getElementById('active-count').textContent = all.filter((r) => ACTIVE.includes(r.status)).length;
}

function refreshSpeed() {
  const now = Date.now();
  let total = 0;
  for (const [id, s] of speeds) {
    if (now - s.t > 2000) speeds.delete(id); else total += s.speed;
  }
  document.getElementById('speed').textContent = formatSpeed(total);
}

const channel = new BroadcastChannel('turbodl');
channel.onmessage = ({ data }) => {
  if (data.type === 'progress') speeds.set(data.id, { speed: data.speed, t: Date.now() });
  else if (data.type === 'changed') refreshCount();
};
setInterval(refreshSpeed, 500);
refreshCount();
