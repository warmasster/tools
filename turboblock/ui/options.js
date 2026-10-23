import { CATEGORIES, DEFAULTS, getSettings, saveSettings } from '../lib/settings.js';
import { fmtNum, timeAgo, normalizeHost } from '../lib/util.js';
import { CAT_INFO, svg, countUp, lastDays, sumCounts, fmtBytes } from './common.js';

const $ = (id) => document.getElementById(id);
const send = (msg) => chrome.runtime.sendMessage(msg);
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]);

const ICONS = {
  chart: '<path d="M3 3v18h18"/><path d="M7 15l4-4 3 3 5-6"/>',
  shield: '<path d="M12 3l8 3v6c0 4.5-3.4 8.3-8 9-4.6-.7-8-4.5-8-9V6z"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18a14 14 0 0 1 0-18"/>',
  pen: CAT_INFO.user.icon,
  list: '<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3 6h.01"/><path d="M3 12h.01"/><path d="M3 18h.01"/>',
  save: '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4"/><path d="M12 8h.01"/>',
  popups: CAT_INFO.popups.icon,
  bolt: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
  bell: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>',
  belloff: CAT_INFO.annoyances.icon,
  refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
};
for (const el of document.querySelectorAll('[data-ico]')) el.insertAdjacentHTML('afterbegin', svg(ICONS[el.dataset.ico] || ''));

let s = await getSettings();
const version = chrome.runtime.getManifest().version;
$('ver').textContent = 'versión ' + version;
$('ver2').textContent = version;

function toast(msg, kind = 'ok') {
  const t = $('toast');
  t.textContent = msg;
  t.className = 'toast ' + kind;
  t.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => { t.hidden = true; }, 2200);
}

// ---------- navegación ----------
function route() {
  const id = (location.hash || '#panel').slice(1);
  const page = $('p-' + id) ? id : 'panel';
  for (const p of document.querySelectorAll('.page')) p.classList.toggle('on', p.id === 'p-' + page);
  for (const a of document.querySelectorAll('nav a')) a.classList.toggle('on', a.getAttribute('href') === '#' + page);
  if (page === 'panel') renderPanel();
}
addEventListener('hashchange', route);
let resizeT = 0;
addEventListener('resize', () => { clearTimeout(resizeT); resizeT = setTimeout(() => { if (location.hash === '#panel' || !location.hash) renderPanel(); }, 150); });

function renderSide() {
  $('side-state').classList.toggle('off', !s.enabled);
  $('side-state').querySelector('span').textContent = s.enabled ? 'Protección activada' : 'Desactivado';
}

// ---------- panel ----------
let overview = null;
async function renderPanel() {
  overview = await send({ type: 'overview' });
  if (!overview?.meta) return;
  const st = overview.stats;
  const total = sumCounts(st.total);
  const days = lastDays(st.days, 30);
  const today = sumCounts(days[days.length - 1].counts);
  const yesterday = sumCounts(days[days.length - 2].counts);
  countUp($('k-total'), total, fmtNum);
  countUp($('k-today'), today, fmtNum);
  countUp($('k-popups'), st.total.popups || 0, fmtNum);
  $('k-saved').textContent = fmtBytes(total * st.kbPerBlock);
  $('k-since').textContent = st.total.since ? `desde el ${new Date(st.total.since).toLocaleDateString('es-ES', { day: 'numeric', month: 'long', year: 'numeric' })}` : 'empieza a contar ahora';
  $('k-today-s').textContent = yesterday ? `ayer ${fmtNum(yesterday)}` : 'en las últimas horas';
  chart(days);

  const cats = Object.entries(st.total).filter(([k, v]) => k !== 'since' && v > 0).sort((a, b) => b[1] - a[1]);
  const max = Math.max(1, ...cats.map(([, v]) => v));
  $('by-cat').innerHTML = cats.length ? cats.map(([c, v]) => barRow(c, v, max)).join('') : '<p class="muted">Aún no hay datos: navega un poco.</p>';

  const rc = Object.entries(overview.meta.categories).filter(([c]) => overview.cats.includes(c)).map(([c, m]) => [c, m.network]).sort((a, b) => b[1] - a[1]);
  const rmax = Math.max(1, ...rc.map(([, v]) => v));
  $('rules-cat').innerHTML = rc.map(([c, v]) => barRow(c, v, rmax)).join('');
  const filters = Object.entries(overview.meta.categories).filter(([c]) => overview.cats.includes(c)).reduce((t, [, m]) => t + m.filters, 0);
  $('rules-note').textContent = `${fmtNum(filters)} filtros de las listas convertidos en ${fmtNum(overview.rules)} reglas de Chrome.`;
  requestAnimationFrame(() => { for (const i of document.querySelectorAll('.track i')) i.style.width = i.dataset.w; });
}

function barRow(c, v, max) {
  const info = CAT_INFO[c] || { name: c, color: '#888', icon: '' };
  return `<div class="bar-row" style="--c:${info.color}"><span class="cic">${svg(info.icon)}</span><span>${info.name}</span><b>${fmtNum(v)}</b>
    <div class="track"><i data-w="${Math.max(2, (v / max) * 100)}%"></i></div></div>`;
}

const ORDER = ['ads', 'privacy', 'malware', 'cookies', 'annoyances', 'popups', 'user'];
function chart(days) {
  const W = Math.max(320, $('chart').clientWidth || 900), H = 210, padL = 40, padB = 22, top = 8;
  const cw = (W - padL) / days.length;
  const totals = days.map((d) => sumCounts(d.counts));
  const max = niceMax(Math.max(4, ...totals));
  const y = (v) => top + (H - padB - top) * (1 - v / max);
  let out = '<g class="grid">';
  for (let i = 0; i <= 4; i++) {
    const v = (max / 4) * i;
    out += `<line x1="${padL}" x2="${W}" y1="${y(v)}" y2="${y(v)}"/><text x="${padL - 8}" y="${y(v) + 3.5}" text-anchor="end">${short(v)}</text>`;
  }
  out += '</g>';
  days.forEach((d, i) => {
    const x = padL + i * cw + cw * 0.18, w = cw * 0.64;
    let acc = 0;
    out += `<g class="col" data-i="${i}"><rect x="${padL + i * cw}" y="0" width="${cw}" height="${H}" fill="transparent"/>`;
    for (const c of ORDER) {
      const v = d.counts[c] || 0;
      if (!v) continue;
      const y1 = y(acc + v), y0 = y(acc);
      out += `<rect class="bar" x="${x}" y="${y1}" width="${w}" height="${Math.max(0.5, y0 - y1)}" rx="2" fill="${CAT_INFO[c].color}"/>`;
      acc += v;
    }
    if (!acc) out += `<rect x="${x}" y="${y(0) - 1.5}" width="${w}" height="1.5" rx="1" fill="rgba(255,255,255,.08)"/>`;
    if (i % 5 === 4 || i === days.length - 1) out += `<text class="xl" x="${x + w / 2}" y="${H - 5}" text-anchor="middle">${d.date.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' })}</text>`;
    out += '</g>';
  });
  $('chart').innerHTML = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">${out}</svg>`;
  $('chart-legend').innerHTML = ORDER.filter((c) => days.some((d) => d.counts[c])).map((c) => `<span style="--c:${CAT_INFO[c].color}"><i></i>${CAT_INFO[c].name}</span>`).join('');

  let tip = null;
  $('chart').onmousemove = (e) => {
    const g = e.target.closest('.col');
    if (!g) { tip?.remove(); tip = null; return; }
    const d = days[+g.dataset.i];
    tip ||= Object.assign(document.createElement('div'), { className: 'tip' });
    if (!tip.isConnected) $('chart').appendChild(tip);
    const rows = ORDER.filter((c) => d.counts[c]).map((c) => `<div style="--c:${CAT_INFO[c].color}"><i></i>${CAT_INFO[c].name}<em>${fmtNum(d.counts[c])}</em></div>`).join('');
    tip.innerHTML = `<b>${d.date.toLocaleDateString('es-ES', { weekday: 'long', day: 'numeric', month: 'long' })}</b>${rows || '<div>Nada bloqueado</div>'}`;
    const r = $('chart').getBoundingClientRect();
    tip.style.left = Math.min(Math.max(e.clientX - r.left, 80), r.width - 80) + 'px';
    tip.style.top = (e.clientY - r.top - 12) + 'px';
  };
  $('chart').onmouseleave = () => { tip?.remove(); tip = null; };
}
function niceMax(v) {
  const p = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p;
  return 10 * p;
}
const short = (v) => (v >= 1e6 ? (v / 1e6).toFixed(1).replace('.0', '') + ' M' : v >= 1000 ? (v / 1000).toFixed(v >= 1e4 ? 0 : 1).replace('.0', '') + ' k' : String(Math.round(v)));

// ---------- protección ----------
function renderProtection() {
  $('cats').innerHTML = '';
  for (const c of CATEGORIES) {
    const info = CAT_INFO[c.id];
    const l = document.createElement('label');
    l.className = 'opt';
    l.innerHTML = `<span class="cic" style="--c:${info.color}">${svg(info.icon)}</span><div><b>${esc(c.name)}</b><small>${esc(c.hint)}</small></div>
      <span class="meta"></span><span class="switch"><input type="checkbox"><span></span></span>`;
    const input = l.querySelector('input');
    input.checked = !!s.cats[c.id];
    l.classList.toggle('dis', !input.checked);
    const m = overview?.meta?.categories?.[c.id];
    if (m) l.querySelector('.meta').textContent = `${fmtNum(m.network)} reglas`;
    input.onchange = async () => {
      l.classList.toggle('dis', !input.checked);
      s = await saveSettings({ cats: { [c.id]: input.checked } });
      toast(input.checked ? `${c.name}: activado` : `${c.name}: desactivado`);
    };
    $('cats').appendChild(l);
  }
  const bind = (id, key, msgOn, msgOff) => {
    $(id).checked = !!s[key];
    $(id).onchange = async () => { s = await saveSettings({ [key]: $(id).checked }); toast($(id).checked ? msgOn : msgOff); syncOpt(); };
  };
  bind('popups', 'popups', 'Ventanas emergentes: se bloquean', 'Ventanas emergentes: permitidas');
  bind('strict', 'strictPopups', 'Modo estricto activado', 'Solo las ventanas de las listas');
  bind('toasts', 'toasts', 'Se avisará en la página', 'Sin avisos en la página');
  bind('nonotif', 'noNotifications', 'Las webs ya no pueden pedir notificaciones', 'Las webs pueden volver a pedir notificaciones');
  bind('auto', 'autoUpdate', 'Las listas se actualizarán solas', 'Actualización automática desactivada');
  syncOpt();
}
function syncOpt() {
  for (const id of ['strict', 'toasts']) {
    $(id).disabled = !s.popups;
    $(id).closest('.opt').classList.toggle('dis', !s.popups);
  }
}
$('shortcuts').onclick = (e) => { e.preventDefault(); chrome.tabs.create({ url: 'chrome://extensions/shortcuts' }); };

// ---------- webs ----------
function hostList(listId, key) {
  const ul = $(listId);
  const items = s[key];
  ul.innerHTML = items.length ? '' : '<li class="empty">Ninguna</li>';
  for (const h of items) {
    const li = document.createElement('li');
    li.innerHTML = `<img alt="" src="/_favicon/?pageUrl=${encodeURIComponent('https://' + h)}&size=32"><span>${esc(h)}</span><button title="Quitar" aria-label="Quitar ${esc(h)}">✕</button>`;
    li.querySelector('button').onclick = async () => {
      s = await saveSettings({ [key]: s[key].filter((x) => x !== h) });
      hostList(listId, key);
      toast(`${h} quitada`);
    };
    ul.appendChild(li);
  }
}
function bindAdd(inputId, btnId, listId, key) {
  const add = async () => {
    const hosts = $(inputId).value.split(/[\s,]+/).map(normalizeHost).filter(Boolean);
    if (!hosts.length) { toast('Escribe un dominio, por ejemplo ejemplo.com', 'err'); return; }
    s = await saveSettings({ [key]: [...new Set([...s[key], ...hosts])].sort() });
    $(inputId).value = '';
    hostList(listId, key);
    toast(hosts.length === 1 ? `${hosts[0]} añadida` : `${hosts.length} webs añadidas`);
  };
  $(btnId).onclick = add;
  $(inputId).onkeydown = (e) => { if (e.key === 'Enter') add(); };
}
bindAdd('allow-in', 'allow-add', 'allow-list', 'allow');
bindAdd('pop-in', 'pop-add', 'pop-list', 'popupAllow');

// ---------- mis filtros ----------
function countFilters() {
  const n = $('filters').value.split(/\r?\n/).filter((l) => l.trim() && !l.trim().startsWith('!')).length;
  $('filters-count').textContent = n === 1 ? '1 filtro' : `${fmtNum(n)} filtros`;
}
$('filters').value = s.userFilters;
$('filters').oninput = countFilters;
$('save-filters').onclick = async () => {
  s = await saveSettings({ userFilters: $('filters').value });
  await send({ type: 'reapply' });
  await renderErrors();
  toast('Filtros aplicados');
};
async function renderErrors() {
  const { status } = await chrome.storage.local.get('status');
  $('errors').innerHTML = '';
  for (const e of status?.userErrors || []) {
    const d = document.createElement('div');
    d.textContent = `Línea ${e.line}: ${e.why}${e.text ? ' — ' + e.text : ''}`;
    $('errors').appendChild(d);
  }
}

// ---------- listas ----------
async function renderLists() {
  const o = overview || await send({ type: 'overview' });
  if (!o?.meta) return;
  const u = o.updateState;
  let txt = `${fmtNum(o.rules)} reglas activas · compiladas ${timeAgo(o.meta.built)}.`;
  if (u?.running) txt = 'Buscando listas nuevas…';
  else if (u && !u.native) txt += ' El actualizador no está instalado: abre TurboBLOCK.exe una vez y se actualizarán solas.';
  else if (u?.error) txt += ` Última comprobación con error: ${u.error}`;
  else if (u?.lastCheck) txt += ` Última comprobación ${timeAgo(u.lastCheck)}.`;
  $('update-state').textContent = txt;
  $('lists').innerHTML = o.meta.lists.map((l) => {
    const info = CAT_INFO[l.category] || { name: l.category, color: '#888' };
    return `<tr><td>${esc(l.title)}</td><td><span class="tag" style="--c:${info.color}"><i></i>${esc(info.name)}</span></td><td>${esc(l.version || '—')}</td></tr>`;
  }).join('');
}
$('update').onclick = async () => {
  $('update').disabled = true;
  $('update-state').textContent = 'Buscando listas nuevas…';
  const r = await send({ type: 'update', force: true });
  $('update').disabled = false;
  overview = null;
  await renderLists();
  if (r?.reloading) toast('Listas nuevas: recargando la extensión…');
  else if (r?.native && !r.error) toast('Las listas ya estaban al día');
  else toast(r?.error || 'No se pudo actualizar', 'err');
};

// ---------- copia de seguridad ----------
$('export').onclick = async () => {
  const data = await chrome.storage.local.get(['settings', 'statsDays', 'statsTotal']);
  const blob = new Blob([JSON.stringify({ app: 'TurboBLOCK', version, saved: new Date().toISOString(), ...data }, null, 2)], { type: 'application/json' });
  const a = Object.assign(document.createElement('a'), { href: URL.createObjectURL(blob), download: `turboblock-${new Date().toISOString().slice(0, 10)}.json` });
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 2000);
  toast('Copia guardada');
};
$('import-file').onchange = async (e) => {
  const f = e.target.files[0];
  e.target.value = '';
  if (!f) return;
  try {
    const data = JSON.parse(await f.text());
    if (data.app !== 'TurboBLOCK' || !data.settings) throw new Error('no es una copia de TurboBLOCK');
    const clean = { ...DEFAULTS, ...data.settings, cats: { ...DEFAULTS.cats, ...data.settings.cats } };
    const patch = { settings: clean };
    if (data.statsDays) patch.statsDays = data.statsDays;
    if (data.statsTotal) patch.statsTotal = data.statsTotal;
    await chrome.storage.local.set(patch);
    s = await getSettings();
    initAll();
    $('copy-msg').textContent = `Recuperada la copia del ${new Date(data.saved).toLocaleString('es-ES')}.`;
    toast('Copia recuperada');
  } catch (err) {
    toast('No se pudo leer: ' + err.message, 'err');
  }
};
$('reset-stats').onclick = async () => {
  if (!confirm('¿Poner a cero todas las estadísticas?')) return;
  await chrome.storage.local.remove(['statsDays', 'statsTotal']);
  toast('Estadísticas a cero');
  renderPanel();
};

// ---------- inicio ----------
function initAll() {
  renderSide();
  renderProtection();
  hostList('allow-list', 'allow');
  hostList('pop-list', 'popupAllow');
  $('filters').value = s.userFilters;
  countFilters();
  renderErrors();
  renderLists();
}
chrome.storage.onChanged.addListener(async (ch) => {
  if (ch.settings) { s = await getSettings(); renderSide(); }
  if (ch.updateState) { overview = null; renderLists(); }
});
overview = await send({ type: 'overview' });
initAll();
route();
