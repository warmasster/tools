import { CATEGORIES, getSettings, saveSettings } from '../lib/settings.js';
import { fmtNum, timeAgo } from '../lib/util.js';
import { CAT_INFO, svg, donut, countUp, lastDays, sumCounts, fmtBytes } from './common.js';

const $ = (id) => document.getElementById(id);
const send = (msg) => chrome.runtime.sendMessage(msg);

const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
let s = await getSettings();

// ---------- categorías ----------
function renderCats() {
  $('cats').innerHTML = '';
  for (const c of CATEGORIES) {
    const info = CAT_INFO[c.id];
    const row = document.createElement('div');
    row.className = 'row';
    row.title = c.hint;
    row.innerHTML = `<span class="cic" style="--c:${info.color}">${svg(info.icon)}</span>
      <div class="row-txt"><span></span><small></small></div>
      <label class="switch"><input type="checkbox"><span></span></label>`;
    row.querySelector('.row-txt span').textContent = c.name;
    row.querySelector('small').textContent = c.hint;
    const input = row.querySelector('input');
    input.checked = !!s.cats[c.id];
    row.classList.toggle('dis', !input.checked);
    input.setAttribute('aria-label', c.name);
    input.onchange = async () => {
      row.classList.toggle('dis', !input.checked);
      s = await saveSettings({ cats: { [c.id]: input.checked } });
    };
    $('cats').appendChild(row);
  }
  $('popups-ic').innerHTML = svg(CAT_INFO.popups.icon);
}

function renderState() {
  document.body.classList.toggle('off', !s.enabled);
  $('enabled').checked = s.enabled;
  $('state').querySelector('span').textContent = s.enabled ? 'Protección activada' : 'Desactivado en todas las webs';
  $('popups').checked = s.popups;
  $('popups-row').classList.toggle('dis', !s.popups);
  $('strict').classList.toggle('on', s.strictPopups);
  $('strict').disabled = !s.popups;
  $('strict-txt').textContent = !s.popups ? 'Desactivado' : s.strictPopups ? 'Modo estricto: todas las sospechosas' : 'Solo las de las listas';
}

// ---------- web actual ----------
async function renderSite() {
  if (!tab?.id) return;
  const info = await send({ type: 'tabInfo', tabId: tab.id });
  if (!info || info.error) return;
  if (!info.web) {
    $('host').textContent = 'Página del navegador';
    $('fav').hidden = true;
    $('site-on').disabled = true;
    $('pick').disabled = true;
    $('count').textContent = '—';
    $('count-cap').textContent = 'aquí no hay nada que bloquear';
    $('ring').innerHTML = donut([]);
    return;
  }
  $('host').textContent = info.host.replace(/^www\./, '');
  $('fav').src = `/_favicon/?pageUrl=${encodeURIComponent(info.url)}&size=32`;
  const on = !info.allowed && s.enabled;
  $('site-on').setAttribute('aria-pressed', String(!info.allowed));
  $('site-on-txt').textContent = info.allowed ? 'Desactivado en esta web' : 'Activado en esta web';
  $('site-on').disabled = !s.enabled;
  $('site').classList.toggle('paused', !on);

  const parts = Object.entries(info.byCat).filter(([, n]) => n > 0);
  if (info.popups) parts.push(['popups', info.popups]);
  parts.sort((a, b) => b[1] - a[1]);
  const total = parts.reduce((t, [, n]) => t + n, 0);
  countUp($('count'), total, fmtNum);
  $('count-cap').textContent = info.allowed ? 'TurboBLOCK está en pausa aquí' : total === 1 ? 'bloqueado en esta página' : 'bloqueados en esta página';
  $('ring').innerHTML = donut(parts, 72, 7);
  $('legend').innerHTML = parts.map(([c, n]) => `<span class="chip" style="--c:${CAT_INFO[c]?.color || '#888'}"><i></i>${CAT_INFO[c]?.name || c} <b>${fmtNum(n)}</b></span>`).join('');
}

// ---------- estadísticas y listas ----------
async function renderOverview() {
  const o = await send({ type: 'overview' });
  if (!o?.meta) return;
  const st = o.stats;
  const days = lastDays(st.days, 14);
  const todayN = sumCounts(days[days.length - 1].counts);
  const totalN = sumCounts(st.total);
  countUp($('st-today'), todayN, fmtNum);
  countUp($('st-total'), totalN, fmtNum);
  $('st-saved').textContent = fmtBytes(totalN * st.kbPerBlock);
  const vals = days.map((d) => sumCounts(d.counts));
  const max = Math.max(1, ...vals);
  const w = 100 / vals.length;
  $('spark').innerHTML = `<defs><linearGradient id="sg" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#8b5cf6"/><stop offset="1" stop-color="#06b6d4"/></linearGradient></defs>` +
    vals.map((v, i) => {
      const h = Math.max(1.5, (v / max) * 26);
      return `<rect x="${i * w + w * 0.18}" y="${28 - h}" width="${w * 0.64}" height="${h}" rx="1" fill="url(#sg)" opacity="${i === vals.length - 1 ? 1 : 0.55}"><title>${days[i].date.toLocaleDateString('es-ES', { day: 'numeric', month: 'short' })}: ${fmtNum(v)}</title></rect>`;
    }).join('');

  const u = o.updateState;
  const el = $('lists');
  el.classList.remove('err');
  el.title = '';
  let txt = `${fmtNum(o.rules)} reglas · listas ${timeAgo(o.meta.built)}`;
  if (u?.running) txt = 'Buscando listas nuevas…';
  else if (u?.reloading) txt = 'Listas nuevas: recargando…';
  else if (u && !u.native) { txt += ' · sin actualizador'; el.title = 'Abre TurboBLOCK.exe una vez para que las listas se actualicen solas'; }
  else if (u?.error) { el.classList.add('err'); el.title = u.error; }
  el.textContent = txt;
}

// ---------- acciones ----------
$('enabled').onchange = async (e) => { s = await saveSettings({ enabled: e.target.checked }); renderState(); setTimeout(renderSite, 350); };
$('popups').onchange = async (e) => { s = await saveSettings({ popups: e.target.checked }); renderState(); };
$('strict').onclick = async () => { s = await saveSettings({ strictPopups: !s.strictPopups }); renderState(); };
$('site-on').onclick = async () => {
  $('site-on').disabled = true;
  await send({ type: 'toggleSite', tabId: tab.id });
  setTimeout(() => { $('site-on').disabled = false; renderSite(); }, 450);
};
$('pick').onclick = async () => { await send({ type: 'pick', tabId: tab.id }); window.close(); };
$('reload').onclick = () => { chrome.tabs.reload(tab.id); window.close(); };
$('options').onclick = () => { chrome.runtime.openOptionsPage(); window.close(); };
$('stats').onclick = () => { chrome.tabs.create({ url: 'options.html#panel' }); window.close(); };
$('update').onclick = async () => {
  const b = $('update');
  b.disabled = true;
  b.classList.add('spin');
  $('lists').textContent = 'Buscando listas nuevas…';
  const r = await send({ type: 'update', force: true });
  b.disabled = false;
  b.classList.remove('spin');
  if (r && !r.native) { $('lists').textContent = r.error || 'Sin actualizador'; $('lists').classList.add('err'); return; }
  renderOverview();
};

chrome.storage.onChanged.addListener((ch) => { if (ch.updateState) renderOverview(); });

renderState();
renderCats();
renderSite();
renderOverview();
