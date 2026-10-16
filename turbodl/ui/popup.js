import * as db from '../lib/db.js';
import { formatBytes, formatSpeed, formatEta, formatDuration, activeMsOf } from '../lib/util.js';
import { mountSettingsForm, bindEnabledToggle, shortcutText } from './settings-form.js';
import { fileType, copyButton, metricsText, svg } from './common.js';

const $ = (id) => document.getElementById(id);
const ACTIVE = ['queued', 'downloading', 'ready', 'delivering'];
const SHOWN = 3; // descargas en la vista previa
const LIVE_TTL = 2500;
const live = new Map(); // id → último progreso (+ t)
let items = []; // lo que se está mostrando: { rec, done }

await bindEnabledToggle($('enabled'), (on) => {
  document.body.classList.toggle('off', !on);
  $('state-text').textContent = on ? 'Interceptación activada' : 'Interceptación desactivada';
});
await mountSettingsForm($('settings'), { saved: document.querySelector('.settings .saved') });

const openManager = async () => {
  await chrome.runtime.sendMessage({ target: 'background', cmd: 'openManager' });
  window.close();
};
$('open-manager').addEventListener('click', openManager);

// Los ajustes van plegados (la vista previa es lo principal); se recuerda si se abren.
const box = $('settings-box');
try { box.open = localStorage.getItem('turbodl.popup.settings') === 'open'; } catch { /* sin almacenamiento */ }
box.addEventListener('toggle', () => {
  try { localStorage.setItem('turbodl.popup.settings', box.open ? 'open' : 'closed'); } catch { /* da igual */ }
});

async function showEngine() {
  const [{ engine }, { settings }] = await Promise.all([chrome.storage.local.get('engine'), chrome.storage.local.get('settings')]);
  const native = engine?.native && settings?.engine !== 'browser';
  $('engine-text').textContent = native
    ? `Motor nativo (Go) ${engine.version} · ${settings?.nativeConnections ?? 16} conexiones`
    : 'Motor del navegador · máx. 6 conexiones por servidor';
}
showEngine();
chrome.storage.onChanged.addListener((c, area) => { if (area === 'local' && (c.engine || c.settings)) showEngine(); });
// Por si se acaba de instalar el motor: pedir al SW que lo vuelva a buscar.
chrome.runtime.sendMessage({ target: 'background', cmd: 'detectNative' }).catch(() => {});

const key = await shortcutText();
$('shortcut').innerHTML = key
  ? `Atajo para activar/desactivar: <kbd>${key}</kbd>`
  : 'Puedes asignar un atajo en chrome://extensions/shortcuts';

// ================================================================= vista previa

const STATUS_TEXT = { queued: 'En cola', paused: 'En pausa', ready: 'Guardando…', delivering: 'Guardando en Descargas…' };

function send(cmd, id) {
  return chrome.runtime.sendMessage({ target: 'background', cmd, id });
}

function iconButton(icon, title, onClick, cls = '') {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = `act ${cls}`;
  b.title = title;
  b.setAttribute('aria-label', title);
  b.innerHTML = svg(icon);
  b.addEventListener('click', (e) => {
    e.stopPropagation(); // la fila entera abre el gestor
    onClick(b);
  });
  return b;
}

/** Pausar / reanudar / reintentar y cancelar, según el estado. */
function actionButtons(rec) {
  const out = [];
  if (rec.status === 'downloading' || rec.status === 'queued') {
    out.push(iconButton('pause', 'Pausar', (b) => { b.disabled = true; send('pause', rec.id); }));
  } else if (rec.status === 'paused') {
    out.push(iconButton('play', 'Reanudar', (b) => { b.disabled = true; send('resume', rec.id); }, 'go'));
  } else if (rec.status === 'error') {
    out.push(iconButton('retry', 'Reintentar', (b) => { b.disabled = true; send('retry', rec.id); }, 'go'));
  }
  if (rec.status !== 'ready' && rec.status !== 'delivering') {
    // Cancelar borra lo descargado: hace falta un segundo clic para confirmarlo.
    out.push(iconButton('x', 'Cancelar', (b) => {
      if (b.classList.contains('confirm')) {
        b.disabled = true;
        send('cancel', rec.id);
        return;
      }
      b.classList.add('confirm');
      b.title = 'Pulsa otra vez para cancelar y borrar lo descargado';
      clearTimeout(b.t);
      b.t = setTimeout(() => { b.classList.remove('confirm'); b.title = 'Cancelar'; }, 3000);
    }, 'danger'));
  }
  return out;
}

function rowFor(item) {
  const { rec, done } = item;
  const el = document.createElement('div');
  el.className = 'dl';
  el.title = 'Abrir el gestor de descargas';
  el.innerHTML = '<div class="dl-top"><span class="ft"></span><span class="dl-name"></span></div><div class="bar"><i></i></div><div class="dl-meta"></div>';
  el.querySelector('.ft').textContent = fileType(rec.filename);
  el.querySelector('.dl-name').textContent = rec.filename;
  el.querySelector('.dl-top').append(
    ...(done ? [] : actionButtons(rec)),
    copyButton(done ? rec.url : rec.originalUrl || rec.url, { cls: 'act' }),
  );
  el.addEventListener('click', openManager);
  item.el = el;
  fillRow(item);
  return el;
}

function fillRow(item) {
  const { rec, done, el, segs } = item;
  const bar = el.querySelector('.bar i');
  const meta = el.querySelector('.dl-meta');
  el.classList.toggle('done', done);
  el.classList.toggle('paused', !done && rec.status === 'paused');
  el.classList.toggle('error', !done && rec.status === 'error');
  if (done) {
    bar.style.width = '100%';
    meta.textContent = `✓ ${metricsText(rec) || formatBytes(rec.size)}`;
    return;
  }
  const p = live.get(rec.id);
  const fresh = p && Date.now() - p.t < LIVE_TTL && rec.status === 'downloading';
  const size = (fresh ? p.size : rec.size) || 0;
  const finished = rec.status === 'ready' || rec.status === 'delivering';
  const bytes = finished ? size : fresh ? p.bytesDone : segs?.bytesDone ?? 0;
  const pct = size ? Math.min(100, Math.floor((bytes / size) * 100)) : 0;
  bar.style.width = `${pct}%`;
  const parts = [size ? `${pct} %` : formatBytes(bytes)];
  if (rec.status === 'error') {
    parts[0] = `Error: ${rec.error || 'desconocido'}`;
  } else if (STATUS_TEXT[rec.status]) {
    parts.push(STATUS_TEXT[rec.status]);
  } else if (fresh) {
    parts.push(p.waiting ? 'esperando al servidor…' : formatSpeed(p.speed));
    if (p.eta != null) parts.push(`quedan ${formatEta(p.eta)}`);
  }
  const ms = activeMsOf(rec);
  if (rec.status !== 'error' && ms >= 1000) parts.push(`lleva ${formatDuration(ms)}`);
  meta.textContent = parts.join(' · ');
}

async function renderRecent() {
  const [downloads, history, segs] = await Promise.all([db.getAll('downloads'), db.getAll('history'), db.getAll('segments')]);
  $('active-count').textContent = downloads.filter((r) => ACTIVE.includes(r.status)).length;
  const segById = new Map(segs.map((s) => [s.id, s]));
  // Primero las que están en marcha (las más nuevas arriba), luego las últimas terminadas.
  const act = downloads.sort((a, b) => b.createdAt - a.createdAt).map((rec) => ({ rec, done: false, segs: segById.get(rec.id) }));
  const fin = history.sort((a, b) => b.completedAt - a.completedAt).map((rec) => ({ rec, done: true }));
  items = [...act, ...fin].slice(0, SHOWN);
  $('recent-list').replaceChildren(...items.map(rowFor));
  $('recent-empty').hidden = items.length > 0;
}

function refreshSpeed() {
  const now = Date.now();
  let total = 0;
  for (const [id, p] of live) {
    if (now - p.t > LIVE_TTL) live.delete(id); else total += p.speed;
  }
  $('speed').textContent = formatSpeed(total);
}

const channel = new BroadcastChannel('turbodl');
let renderTimer;
channel.onmessage = ({ data }) => {
  if (data?.type === 'progress') {
    live.set(data.id, { ...data, t: Date.now() });
    const item = items.find((i) => !i.done && i.rec.id === data.id);
    if (item) fillRow(item);
  } else if (data?.type === 'changed') {
    clearTimeout(renderTimer);
    renderTimer = setTimeout(renderRecent, 80);
  }
};
setInterval(() => {
  refreshSpeed();
  for (const item of items) if (!item.done) fillRow(item); // "lleva X s" y velocidades caducadas
}, 1000);
renderRecent();
