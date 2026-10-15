// Gestor de descargas. Lee el estado de IndexedDB y recibe el progreso en vivo del
// motor por BroadcastChannel; las órdenes van al service worker.

import * as db from '../lib/db.js';
import { formatBytes, formatSpeed, formatEta, hostOf } from '../lib/util.js';
import { mountSettingsForm, bindEnabledToggle } from './settings-form.js';

const $ = (id) => document.getElementById(id);
const ACTIVE = ['queued', 'downloading', 'ready', 'delivering'];
const LIVE_TTL = 2500; // ms que se considera vigente un mensaje de progreso

const STATUS = {
  queued: ['En cola', 'st-queued'],
  downloading: ['Descargando', 'st-downloading'],
  paused: ['En pausa', 'st-paused'],
  error: ['Error', 'st-error'],
  ready: ['Guardando…', 'st-saving'],
  delivering: ['Guardando en Descargas…', 'st-saving'],
};

let downloads = [];
let history = [];
let segs = new Map(); // id → registro de `segments`
const live = new Map(); // id → último mensaje de progreso (+ t)
const cards = new Map(); // clave → elemento
const dateFmt = new Intl.DateTimeFormat('es', { dateStyle: 'short', timeStyle: 'short' });

// ================================================================= órdenes

async function send(cmd, data = {}) {
  const r = await chrome.runtime.sendMessage({ target: 'background', cmd, ...data });
  if (r && !r.ok) throw new Error(r.error);
  return r?.result;
}

const ICONS = {
  pause: '<path d="M8 5v14M16 5v14"/>',
  play: '<path d="m7 4 13 8-13 8V4Z"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  retry: '<path d="M3 12a9 9 0 0 1 15.5-6.2L21 8M21 3v5h-5M21 12a9 9 0 0 1-15.5 6.2L3 16M3 21v-5h5"/>',
  open: '<path d="M14 3h7v7M21 3l-9 9M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
  chrome: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3.5"/><path d="M12 8.5h8.3M8.9 13.8 4.8 6.6M15 13.8l-4.2 7.1"/>',
};

function button(label, icon, onClick, { cls = '', iconOnly = false, disabled = false } = {}) {
  const b = document.createElement('button');
  b.className = `btn ${cls} ${iconOnly ? 'icon' : ''}`;
  b.type = 'button';
  b.title = label;
  b.setAttribute('aria-label', label);
  b.disabled = disabled;
  b.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${ICONS[icon]}</svg>${iconOnly ? '' : `<span>${label}</span>`}`;
  b.addEventListener('click', async () => {
    b.disabled = true;
    try { await onClick(); } catch (e) { alert(e.message); } finally { b.disabled = false; }
  });
  return b;
}

function actionsFor(rec) {
  const id = rec.id;
  const pause = () => button('Pausar', 'pause', () => send('pause', { id }));
  const resume = () => button('Reanudar', 'play', () => send('resume', { id }), { cls: 'primary' });
  const cancel = () => button('Cancelar', 'x', () => {
    if (confirm(`¿Cancelar «${rec.filename}»? Se borrará lo descargado.`)) return send('cancel', { id });
  }, { cls: 'danger', iconOnly: true });
  switch (rec.status) {
    case 'queued':
    case 'downloading': return [pause(), cancel()];
    case 'paused': return [resume(), cancel()];
    case 'error': return [
      button('Reintentar', 'retry', () => send('retry', { id }), { cls: 'primary' }),
      button('Descargar con Chrome normal', 'chrome', () => send('fallback', { id })),
      cancel(),
    ];
    default: return [];
  }
}

// ================================================================= tarjetas

function newCard(key) {
  const el = $('card-tpl').content.firstElementChild.cloneNode(true);
  el.dataset.key = key;
  cards.set(key, el);
  return el;
}

function fileType(name) {
  const m = /\.([a-z0-9]{1,5})$/i.exec(name || '');
  return m ? m[1].slice(0, 4) : 'bin';
}

function setText(el, sel, text) {
  const n = el.querySelector(sel);
  if (n.textContent !== text) n.textContent = text;
}

function fillActive(el, rec) {
  const [label, cls] = STATUS[rec.status] || [rec.status, ''];
  el.classList.toggle('is-error', rec.status === 'error');
  el.querySelector('.file-icon').textContent = fileType(rec.filename);
  setText(el, '.name', rec.filename);
  el.querySelector('.name').title = rec.filename;
  const pill = el.querySelector('.pill');
  pill.textContent = label;
  pill.className = `pill ${cls}`;
  const mode = rec.mode === 'single' ? '1 conexión, sin reanudación' : `hasta ${rec.connections || 6} conexiones`;
  setText(el, '.meta', `${hostOf(rec.url)} · ${rec.size ? formatBytes(rec.size) : 'tamaño desconocido'} · ${mode}`);
  el.querySelector('.meta').title = rec.originalUrl || rec.url;

  const notice = el.querySelector('.notice');
  notice.hidden = !rec.notice;
  notice.textContent = rec.notice || '';
  const err = el.querySelector('.err-msg');
  err.hidden = rec.status !== 'error';
  err.textContent = rec.error || 'Error desconocido.';

  // Los botones solo se rehacen si cambia el estado (no en cada tic de progreso).
  if (el.dataset.status !== rec.status) {
    el.dataset.status = rec.status;
    el.querySelector('.actions').replaceChildren(...actionsFor(rec));
  }
  fillProgress(el, rec);
}

function fillProgress(el, rec) {
  const p = live.get(rec.id);
  const fresh = p && Date.now() - p.t < LIVE_TTL && rec.status === 'downloading';
  const saved = segs.get(rec.id);
  const size = (fresh ? p.size : rec.size) || null;
  const done = fresh ? p.bytesDone : saved?.bytesDone ?? 0;
  const segments = fresh ? p.segments : (saved?.segments || []).map((s) => [s.start, s.end, s.pos, 0]);
  const finished = rec.status === 'ready' || rec.status === 'delivering';

  setText(el, '.pct', size ? `${Math.floor(((finished ? size : done) / size) * 100)} %` : '');
  setText(el, '.bytes', size ? `${formatBytes(finished ? size : done)} de ${formatBytes(size)}` : formatBytes(done));
  setText(el, '.speed', fresh ? formatSpeed(p.speed) : '');
  setText(el, '.eta', fresh && p.eta != null ? `quedan ${formatEta(p.eta)}` : '');
  setText(el, '.conns', fresh ? `${p.connections} ${p.connections === 1 ? 'conexión' : 'conexiones'}` : '');
  drawSegments(el.querySelector('.segbar'), segments, size, done, finished, rec.status);
}

/** Barra estilo IDM: cada segmento pinta su parte descargada; los activos, más claros. */
function drawSegments(canvas, segments, size, done, finished, status) {
  const dpr = devicePixelRatio || 1;
  const w = canvas.clientWidth, h = canvas.clientHeight;
  if (!w) return;
  if (canvas.width !== Math.round(w * dpr)) { canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr); }
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  const paused = status === 'paused' || status === 'error';
  const base = finished ? '#2fbf71' : paused ? '#6b7385' : '#3f7cf0';
  const bright = '#86b1ff';

  if (finished) { ctx.fillStyle = base; ctx.fillRect(0, 0, w, h); return; }
  if (!size) {
    // Tamaño desconocido: franja animada.
    const x = ((performance.now() / 12) % (w + 80)) - 80;
    ctx.fillStyle = base;
    ctx.fillRect(x, 0, 80, h);
    return;
  }
  for (const [start, end, pos, active] of segments) {
    const x0 = (start / size) * w;
    const x1 = (pos / size) * w;
    if (x1 > x0) {
      ctx.fillStyle = active && !paused ? bright : base;
      ctx.fillRect(x0, 0, x1 - x0, h);
    }
    if (active && !paused && end != null && pos < end) {
      ctx.fillStyle = '#ffffff';
      ctx.fillRect(Math.max(0, x1 - 1), 0, 2, h);
    }
    if (start > 0) { // separación entre segmentos
      ctx.fillStyle = 'rgba(14,16,21,.9)';
      ctx.fillRect(x0, 0, 1, h);
    }
  }
}

function fillHistory(el, h) {
  el.classList.add('history');
  el.querySelector('.file-icon').textContent = fileType(h.filename);
  setText(el, '.name', h.filename);
  el.querySelector('.name').title = h.path || h.filename;
  const pill = el.querySelector('.pill');
  pill.textContent = 'Terminada';
  pill.className = 'pill st-done';
  const meta = `${formatBytes(h.size)} · ${hostOf(h.url)} · ${dateFmt.format(h.completedAt)}`;
  setText(el, '.meta', meta);
  el.querySelector('.meta').title = h.url;
  if (el.dataset.ready) return;
  el.dataset.ready = '1';

  const open = button('Abrir', 'open', () => chrome.downloads.open(h.deliveryId));
  const show = button('Mostrar en carpeta', 'folder', () => chrome.downloads.show(h.deliveryId));
  const remove = button('Quitar de la lista', 'x', () => send('removeHistory', { id: h.id }), { cls: 'ghost', iconOnly: true });
  el.querySelector('.actions').replaceChildren(open, show, remove);

  // ¿Sigue existiendo el archivo?
  chrome.downloads.search({ id: h.deliveryId }).then(([item]) => {
    if (item && item.state === 'complete' && item.exists !== false) return;
    el.classList.add('missing');
    open.disabled = show.disabled = true;
    setText(el, '.meta', `${meta} · archivo movido o borrado`);
  });
}

// ================================================================= render

function render() {
  const act = [...downloads].sort((a, b) => b.createdAt - a.createdAt);
  const listA = $('active-list');
  const keep = new Set();
  act.forEach((rec, i) => {
    const key = `d${rec.id}`;
    keep.add(key);
    const el = cards.get(key) || newCard(key);
    fillActive(el, rec);
    if (listA.children[i] !== el) listA.insertBefore(el, listA.children[i] || null);
  });
  $('active-empty').hidden = act.length > 0;

  const hist = [...history].sort((a, b) => b.completedAt - a.completedAt);
  const listH = $('history-list');
  hist.forEach((h, i) => {
    const key = `h${h.id}`;
    keep.add(key);
    const el = cards.get(key) || newCard(key);
    fillHistory(el, h);
    if (listH.children[i] !== el) listH.insertBefore(el, listH.children[i] || null);
  });
  $('history-section').hidden = hist.length === 0;

  for (const [key, el] of cards) if (!keep.has(key)) { el.remove(); cards.delete(key); }
  updateHeader();
}

function updateHeader() {
  const now = Date.now();
  let speed = 0;
  for (const p of live.values()) if (now - p.t < LIVE_TTL) speed += p.speed;
  $('total-speed').textContent = formatSpeed(speed);
  $('active-count').textContent = downloads.filter((r) => ACTIVE.includes(r.status)).length;
  document.title = speed > 0 ? `${formatSpeed(speed)} · TurboDL` : 'TurboDL · Descargas';
}

async function refresh() {
  const [d, h, s] = await Promise.all([db.getAll('downloads'), db.getAll('history'), db.getAll('segments')]);
  downloads = d;
  history = h;
  segs = new Map(s.map((x) => [x.id, x]));
  render();
}

let refreshTimer;
function scheduleRefresh() {
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(refresh, 80);
}

// ================================================================= eventos

const channel = new BroadcastChannel('turbodl');
channel.onmessage = ({ data }) => {
  if (data?.type === 'progress') {
    live.set(data.id, { ...data, t: Date.now() });
    const rec = downloads.find((r) => r.id === data.id);
    const el = cards.get(`d${data.id}`);
    if (rec && el) fillProgress(el, rec);
    updateHeader();
  } else if (data?.type === 'changed') {
    scheduleRefresh();
  }
};

// Red de seguridad por si se pierde algún aviso, y para apagar velocidades caducadas.
setInterval(() => {
  if (downloads.length) refresh(); else updateHeader();
}, 3000);
// La franja animada de tamaño desconocido necesita repintarse.
setInterval(() => {
  for (const rec of downloads) if (!rec.size && rec.status === 'downloading') {
    const el = cards.get(`d${rec.id}`);
    if (el) fillProgress(el, rec);
  }
}, 50);
window.addEventListener('resize', () => render());

$('add-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const input = $('add-url');
  const msg = $('add-msg');
  const btn = e.submitter;
  msg.className = 'add-msg';
  msg.textContent = 'Comprobando el enlace…';
  btn.disabled = true;
  try {
    await send('add', { url: input.value.trim() });
    input.value = '';
    msg.textContent = '';
  } catch (err) {
    msg.className = 'add-msg err';
    msg.textContent = err.message;
  } finally {
    btn.disabled = false;
  }
});

$('clear-history').addEventListener('click', () => send('clearHistory'));

$('settings-btn').addEventListener('click', () => {
  const panel = $('settings-panel');
  panel.hidden = !panel.hidden;
  $('settings-btn').setAttribute('aria-expanded', String(!panel.hidden));
});

await bindEnabledToggle($('enabled'), (on) => {
  document.body.classList.toggle('off', !on);
  $('enabled-text').textContent = on ? 'Interceptar' : 'Interceptación desactivada';
});
await mountSettingsForm($('settings'), { saved: document.querySelector('#settings-panel .saved') });
await refresh();
