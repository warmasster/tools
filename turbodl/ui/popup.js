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
