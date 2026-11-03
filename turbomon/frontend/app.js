"use strict";
const App = window.go.main.App;
const RT = window.runtime;
const $ = s => document.querySelector(s);
const GB = 1 << 30;

const state = {
  view: "home", live: null, settings: null, report: null, checking: false, open: new Set(),
  sessions: null, sel: null, selData: null, procs: null, procSort: "mem", busy: {}, offline: false,
};

// ---------- utilidades ----------
const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const n = (v, d = 0) => (v == null || isNaN(v) ? "—" : Number(v).toLocaleString("es-ES", { minimumFractionDigits: d, maximumFractionDigits: d }));
const gb = (b, d = 1) => n(b / GB, d);
const rate = b => b >= 1 << 20 ? n(b / (1 << 20), 1) + " MB/s" : b >= 1024 ? n(b / 1024, 0) + " KB/s" : n(b, 0) + " B/s";
const size = b => b >= 1 << 20 ? n(b / (1 << 20), 1) + " MB" : n(b / 1024, 0) + " KB";
function dur(s) {
  s = Math.round(s || 0);
  if (s < 60) return s + " s";
  const h = Math.floor(s / 3600), m = Math.floor(s % 3600 / 60);
  return h ? `${h} h ${m} min` : `${m} min`;
}
function when(ts) {
  const d = new Date(ts * 1000), now = new Date();
  const hm = d.toLocaleTimeString("es-ES", { hour: "2-digit", minute: "2-digit" });
  const day = x => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((day(now) - day(d)) / 864e5);
  if (diff === 0) return "hoy, " + hm;
  if (diff === 1) return "ayer, " + hm;
  return d.toLocaleDateString("es-ES", { weekday: "short", day: "numeric", month: "short" }) + ", " + hm;
}
const fpsCls = v => v >= 60 ? "good" : v >= 30 ? "hot" : "vhot";
const tempCls = v => v >= 85 ? "vhot" : v >= 75 ? "hot" : "";
const pctBar = (v, cls = "") => `<div class="bar ${cls}"><i style="width:${Math.max(0, Math.min(100, v || 0)).toFixed(1)}%"></i></div>`;
const mainGPU = l => (l && l.gpus || []).slice().sort((a, b) => (b.dedicated - a.dedicated) || (b.vram - a.vram))[0];

const IC = {
  fps: '<svg viewBox="0 0 24 24"><path d="M12 14l4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/></svg>',
  cpu: '<svg viewBox="0 0 24 24"><rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"/></svg>',
  gpu: '<svg viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="2"/><circle cx="9" cy="12" r="3"/><path d="M15 10h4M15 14h4M6 18v3M10 18v3"/></svg>',
  ram: '<svg viewBox="0 0 24 24"><path d="M3 7h18v9H3z"/><path d="M7 7v9M11 7v9M15 7v9M19 16v3M5 16v3"/></svg>',
  disk: '<svg viewBox="0 0 24 24"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14a9 3 0 0 0 18 0V5"/><path d="M3 12a9 3 0 0 0 18 0"/></svg>',
  pc: '<svg viewBox="0 0 24 24"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>',
  warn: '<svg viewBox="0 0 24 24"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4M12 17h.01"/></svg>',
  ok: '<svg viewBox="0 0 24 24"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><path d="m9 11 3 3L22 4"/></svg>',
  chev: '<svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg>',
  bolt: '<svg viewBox="0 0 24 24"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></svg>',
  broom: '<svg viewBox="0 0 24 24"><path d="m13 11 9-9"/><path d="M14.6 12.6c.8.8.9 2.1.2 3L10 22l-8-8 6.4-4.8c.9-.7 2.2-.6 3 .2Z"/><path d="m6.8 10.4 6.8 6.8"/></svg>',
  list: '<svg viewBox="0 0 24 24"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg>',
  info: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>',
  trash: '<svg viewBox="0 0 24 24"><path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  wrench: '<svg viewBox="0 0 24 24"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>',
  refresh: '<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 0 1-15.5 6.3L3 16"/><path d="M3 12a9 9 0 0 1 15.5-6.3L21 8"/><path d="M21 3v5h-5M3 21v-5h5"/></svg>',
  shield: '<svg viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  net: '<svg viewBox="0 0 24 24"><path d="M12 3v12M7 10l5 5 5-5"/><path d="M5 21h14"/></svg>',
  game: '<svg viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="6"/><path d="M6 12h4M8 10v4"/></svg>',
};

// sparkline: valores → <svg> con línea y relleno.
function spark(vals, max, color = "url(#sg)", h = 38) {
  vals = (vals || []).slice(-60);
  if (vals.length < 2) return `<svg viewBox="0 0 100 ${h}" preserveAspectRatio="none"></svg>`;
  // Con máximo fijo (porcentajes) la escala va de 0 a ese máximo; si no, se ajusta
  // a lo que hay para que se vea cómo varía.
  const hi = max || Math.max(1, ...vals) * 1.08;
  const lo = max ? 0 : Math.min(...vals) * 0.85;
  const span = Math.max(hi - lo, 1e-6);
  const pts = vals.map((v, i) => [(i / (vals.length - 1)) * 100, h - 2 - ((Math.min(v, hi) - lo) / span) * (h - 4)]);
  const line = pts.map((p, i) => (i ? "L" : "M") + p[0].toFixed(2) + " " + p[1].toFixed(2)).join(" ");
  const id = "g" + Math.random().toString(36).slice(2, 7);
  return `<svg viewBox="0 0 100 ${h}" preserveAspectRatio="none"><defs><linearGradient id="${id}" x1="0" x2="1"><stop offset="0" stop-color="#8b5cf6"/><stop offset="1" stop-color="#22d3ee"/></linearGradient><linearGradient id="${id}f" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#6366f1" stop-opacity=".35"/><stop offset="1" stop-color="#6366f1" stop-opacity="0"/></linearGradient></defs>
    <path d="${line} L100 ${h} L0 ${h} Z" fill="url(#${id}f)" stroke="none"/><path d="${line}" stroke="${color === "url(#sg)" ? `url(#${id})` : color}" stroke-width="1.8" vector-effect="non-scaling-stroke"/></svg>`;
}

function ring(score, size = 118) {
  const r = 52, c = 2 * Math.PI * r;
  const col = score >= 90 ? "#34d399" : score >= 75 ? "#22d3ee" : score >= 50 ? "#fbbf24" : "#f87171";
  const off = c * (1 - (score || 0) / 100);
  return `<div class="ring" style="width:${size}px;height:${size}px"><svg viewBox="0 0 120 120"><circle class="bgc" cx="60" cy="60" r="${r}"/><circle class="fgc" cx="60" cy="60" r="${r}" stroke="${col}" stroke-dasharray="${c.toFixed(1)}" stroke-dashoffset="${off.toFixed(1)}" stroke-linecap="round"/></svg>
    <div class="val"><div><b class="num">${score == null ? "…" : score}</b><span>de 100</span></div></div></div>`;
}

// morph: aplica html sobre un elemento cambiando solo lo que cambia (así los
// botones no se rehacen cada segundo y los clics no se pierden).
function morph(el, html) {
  const tpl = document.createElement("template");
  tpl.innerHTML = html;
  patchChildren(el, tpl.content);
}
function patchChildren(a, b) {
  const an = Array.from(a.childNodes), bn = Array.from(b.childNodes);
  for (let i = 0; i < bn.length; i++) {
    const x = an[i], y = bn[i];
    if (!x) { a.appendChild(y.cloneNode(true)); continue; }
    if (x.nodeType !== y.nodeType || x.nodeName !== y.nodeName || (x.dataset && y.dataset && x.dataset.key !== y.dataset.key)) { a.replaceChild(y.cloneNode(true), x); continue; }
    if (x.nodeType === 3 || x.nodeType === 8) { if (x.nodeValue !== y.nodeValue) x.nodeValue = y.nodeValue; continue; }
    if (x.nodeName === "CANVAS") continue;
    const focused = x === document.activeElement;
    for (const at of Array.from(x.attributes)) if (!y.hasAttribute(at.name)) x.removeAttribute(at.name);
    for (const at of Array.from(y.attributes)) {
      if (focused && at.name === "value") continue;
      if (x.getAttribute(at.name) !== at.value) x.setAttribute(at.name, at.value);
    }
    if (x.nodeName === "INPUT" && !focused) {
      if (x.type === "range" || x.type === "text") { if (x.value !== y.getAttribute("value")) x.value = y.getAttribute("value") || ""; }
      if (x.type === "checkbox") x.checked = y.hasAttribute("checked");
    }
    patchChildren(x, y);
  }
  for (let i = an.length - 1; i >= bn.length; i--) a.removeChild(an[i]);
}

function toast(msg, err) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast on" + (err ? " err" : "");
  clearTimeout(toast.t);
  toast.t = setTimeout(() => (t.className = "toast"), 3600);
}

function confirmBox(title, text, ok, danger) {
  return new Promise(res => {
    const m = $("#modal");
    $("#modal-card").innerHTML = `<h2>${esc(title)}</h2><p>${esc(text)}</p><div class="btns"><button class="btn ghost" data-r="0">Cancelar</button><button class="btn ${danger ? "danger" : "primary"}" data-r="1">${esc(ok)}</button></div>`;
    m.hidden = false;
    m.onclick = e => {
      const b = e.target.closest("[data-r]");
      if (!b && e.target !== m) return;
      m.hidden = true;
      res(b ? b.dataset.r === "1" : false);
    };
  });
}

const errMsg = e => (e && e.message) || String(e || "error");

// ---------- inicio ----------
function scoreText(r) {
  if (!r) return ["Revisando tu PC…", "Un momento: miro la RAM, la pantalla, la gráfica y la energía."];
  const bad = r.results.filter(x => x.status === "bad").length, warn = r.results.filter(x => x.status === "warn").length;
  if (!bad && !warn) return ["Tu PC está a punto", "Todo lo que hemos revisado está bien configurado para jugar."];
  const parts = [];
  if (bad) parts.push(`${bad} ${bad === 1 ? "cosa importante" : "cosas importantes"}`);
  if (warn) parts.push(`${warn} ${warn === 1 ? "detalle" : "detalles"}`);
  const t = r.score >= 90 ? "Tu PC está casi a punto" : r.score >= 70 ? "Tu PC va bien, pero puede ir mejor" : "Tu PC puede rendir bastante más";
  return [t, "Hay " + parts.join(" y ") + " que mejorar. La mayoría se arregla con un clic."];
}

function overlayCard(s, l) {
  const on = s && s.overlay.on;
  const o = s ? s.overlay : {};
  return `<div class="card ov-card ${on ? "on" : ""}">
    <div class="ov-top"><div class="t"><b>Overlay en el juego</b><p>${on ? "Puesto: lo verás arriba del juego" : "Quitado"} · <span class="kbd">Ctrl+Alt+O</span> o un clic en el icono de la bandeja</p></div>
      <button class="sw ${on ? "on" : ""}" data-act="overlay" title="${on ? "Quitar" : "Poner"} el overlay"></button></div>
    <div class="ov-opts">
      <button class="chip ${!o.fpsOnly ? "on" : ""}" data-act="ovmode" data-arg="full">Completo</button>
      <button class="chip ${o.fpsOnly ? "on" : ""}" data-act="ovmode" data-arg="fps">Solo FPS</button>
      <button class="chip ${o.gameOnly ? "on" : ""}" data-act="ovflag" data-arg="gameOnly">Solo con un juego delante</button>
      <button class="link" data-act="go" data-arg="settings" style="margin-left:auto">Cambiar qué enseña →</button>
    </div></div>`;
}

function tiles(l) {
  const g = mainGPU(l);
  const f = l.fps;
  let fpsTile;
  if (!f.available) {
    fpsTile = `<div class="card big dim"><div class="lbl">${IC.fps}FPS</div><div class="v">—</div><div class="row">Necesita permiso una vez</div><div style="margin-top:auto"><button class="btn sm amber" data-act="enablefps">${IC.shield}Activar FPS</button></div></div>`;
  } else if (f.presenting && f.fps > 0) {
    fpsTile = `<div class="card big"><div class="lbl">${IC.fps}FPS<span class="sub2">${esc(f.name || f.exe)}</span></div>
      <div class="v ${fpsCls(f.fps)}">${n(f.fps)}</div>
      <div class="row"><span>1 % bajo <b class="${fpsCls(f.low1)}">${n(f.low1)}</b></span><span><b>${n(f.frameMs, 1)}</b> ms</span></div>
      <div class="spark">${spark(l.hist.fps)}</div></div>`;
  } else {
    fpsTile = `<div class="card big dim"><div class="lbl">${IC.fps}FPS</div><div class="v">—</div><div class="row">Abre un juego y aquí verás sus FPS</div><div class="spark">${spark(l.hist.fps)}</div></div>`;
  }
  const c = l.cpu;
  const cpuTile = `<div class="card big"><div class="lbl">${IC.cpu}Procesador${c.temp ? `<span class="sub2 ${tempCls(c.temp)}">${n(c.temp)} °C</span>` : ""}</div>
    <div class="v">${n(c.usage)}<small>%</small></div>
    <div class="row"><span>Hilo más ocupado <b>${n(c.maxCore)} %</b></span>${c.mhz ? `<span><b>${n(c.mhz / 1000, 1)}</b> GHz</span>` : ""}</div>
    <div class="spark">${spark(l.hist.cpu, 100)}</div></div>`;
  const gpuTile = g ? `<div class="card big"><div class="lbl">${IC.gpu}Gráfica${g.temp ? `<span class="sub2 ${tempCls(g.temp)}">${n(g.temp)} °C</span>` : ""}</div>
    <div class="v">${n(g.usage)}<small>%</small></div>
    <div class="row"><span>VRAM <b>${gb(g.vramUsed)}</b> / ${gb(g.vram, 0)} GB</span></div>
    <div class="spark">${spark(l.hist.gpu, 100)}</div></div>` : `<div class="card big dim"><div class="lbl">${IC.gpu}Gráfica</div><div class="v">—</div></div>`;
  const m = l.mem;
  const ramTile = `<div class="card big"><div class="lbl">${IC.ram}Memoria RAM<span class="sub2">${n(m.load)} %</span></div>
    <div class="v">${gb(m.used)}<small>/ ${gb(m.total, 0)} GB</small></div>
    <div class="row"><span>Libre de verdad <b>${gb(m.free)}</b> GB</span></div>
    <div class="spark">${spark(l.hist.ram, m.total / GB)}</div></div>`;
  return `<div class="grid g4 mt">${fpsTile}${cpuTile}${gpuTile}${ramTile}</div>`;
}

function todoList(r, max) {
  if (!r) return `<div class="empty">Revisando…</div>`;
  const items = r.results.filter(x => x.status === "bad" || x.status === "warn").sort((a, b) => (a.status === "bad" ? 0 : 1) - (b.status === "bad" ? 0 : 1) || b.weight - a.weight);
  if (!items.length) return `<div class="allgood">${IC.ok}<div><b>Nada que arreglar.</b> La RAM, la pantalla, la gráfica y la energía están bien.</div></div>`;
  return `<div class="todo">${items.slice(0, max).map(x => `<div class="todo-item"><span class="dot ${x.status}"></span>
      <div class="t"><b>${esc(x.title)}</b><span>${esc(x.value)}</span></div>
      ${x.fix ? `<button class="btn sm primary" data-act="fix" data-arg="${esc(x.fix)}" ${state.busy[x.fix] ? "disabled" : ""}>${IC.wrench}${esc(x.fixLabel || "Arreglar")}</button>` : `<button class="btn sm ghost" data-act="howto" data-arg="${esc(x.id)}">Cómo se arregla</button>`}
    </div>`).join("")}${items.length > max ? `<button class="link" data-act="go" data-arg="checks">Ver las ${items.length} en la revisión →</button>` : ""}</div>`;
}

function statBox(v, unit, label, cls = "") {
  return `<div class="stat"><b class="${cls}">${v}${unit ? `<small>${unit}</small>` : ""}</b><span>${label}</span></div>`;
}

function gameCard(l) {
  const s = l.session;
  if (s) {
    return `<div class="card last-game"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#059669);--tile-sh:rgba(16,185,129,.5)">${IC.game}</div><h2>Jugando ahora</h2><span class="meta">${s.turbo ? "Modo juego Turbo activo" : ""}</span></div>
      <div class="lg-top"><b>${esc(s.game)}</b><span>${dur(s.secs)}</span></div>
      <div class="stats">${statBox(n(s.fpsAvg), "FPS", "de media")}${statBox(n(s.low1), "FPS", "1 % bajo", fpsCls(s.low1))}</div>
      <div class="note">Al cerrar el juego se guarda en «Partidas» con su gráfica.</div></div>`;
  }
  const last = state.sessions && state.sessions[0];
  if (!last) {
    return `<div class="card last-game"><div class="card-h"><div class="ic">${IC.game}</div><h2>Partidas</h2></div>
      <div class="empty">Cuando juegues, TurboMON apuntará solo cómo te ha ido: <b>FPS medios, el 1 % bajo, CPU, gráfica y temperaturas</b>. No tienes que hacer nada.</div></div>`;
  }
  return `<div class="card last-game"><div class="card-h"><div class="ic">${IC.game}</div><h2>Última partida</h2><button class="link" data-act="opengame" data-arg="${esc(last.id)}">Ver detalle →</button></div>
    <div class="lg-top"><b>${esc(last.game)}</b><span>${when(last.start)} · ${dur(last.secs)}</span></div>
    <div class="stats">${statBox(n(last.fpsAvg), "FPS", "de media")}${statBox(n(last.low1), "FPS", "1 % bajo", fpsCls(last.low1))}${statBox(n(last.gpu.avg), "%", "gráfica")}${statBox(n(last.cpu.avg), "%", "procesador")}</div></div>`;
}

function renderHome() {
  const l = state.live, r = state.report;
  if (!l) return `<div class="empty">${state.offline ? "Arrancando TurboMON…" : "Leyendo sensores…"}</div>`;
  const [t, p] = scoreText(r);
  const a = l.agent;
  return `<div class="hero">
      <div class="card score-card">${ring(r ? r.score : null)}<div class="score-txt"><h1>${esc(t)}</h1><p>${esc(p)}</p>
        <button class="btn ${r && r.score < 90 ? "primary" : "ghost"}" data-act="go" data-arg="checks">${IC.list}Ver la revisión</button></div></div>
      ${overlayCard(state.settings, l)}
    </div>
    ${tiles(l)}
    <div class="grid g2 mt">
      <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#fbbf24,#f97316);--tile-sh:rgba(245,158,11,.6)">${IC.wrench}</div><h2>Para mejorar</h2></div>${todoList(r, 4)}</div>
      ${gameCard(l)}
    </div>
    <div class="foot"><span>TurboMON gasta ahora <b>${n(a.cpu, 1)} % de CPU</b> y <b>${n(a.memMB, 0)} MB</b> de memoria</span><span>Historial de partidas: <b>${size(a.histBytes)}</b></span><span>Versión ${esc(a.version)}</span></div>`;
}

// ---------- sensores ----------
function renderSensors() {
  const l = state.live;
  if (!l) return `<div class="empty">Leyendo sensores…</div>`;
  const c = l.cpu, m = l.mem, f = l.fps;
  const cores = (c.perCore || []).map(v => `<div class="core" title="${n(v)} %"><i style="height:${Math.max(3, v).toFixed(0)}%"></i></div>`).join("");
  const cpuCard = `<div class="card"><div class="card-h"><div class="ic">${IC.cpu}</div><h2>Procesador</h2><span class="meta">${esc(c.name)}</span></div>
    <div class="bigline"><span class="v">${n(c.usage)}<small>%</small></span>${c.temp ? `<span class="v2 ${tempCls(c.temp)}">${n(c.temp)}<small>°C</small></span>` : ""}${c.mhz ? `<span class="v2">${n(c.mhz / 1000, 2)}<small>GHz</small></span>` : ""}</div>
    <div class="cores">${cores}</div>
    <dl class="kv"><dt>Núcleos / hilos</dt><dd>${c.cores || "—"} / ${c.threads}</dd><dt>Hilo más ocupado</dt><dd>${n(c.maxCore)} %</dd>
    <dt>Frecuencia base</dt><dd>${n(c.baseMHz / 1000, 2)} GHz</dd>${c.powerW ? `<dt>Consumo</dt><dd>${n(c.powerW)} W</dd>` : ""}
    <dt>Temperatura</dt><dd>${c.temp ? n(c.temp, 1) + " °C" + " (HWiNFO)" : "—"}</dd></dl>
    ${c.temp ? "" : `<div class="note">Windows no da la temperatura del procesador sin un driver especial. Si tienes <b>HWiNFO</b> abierto con «Shared Memory Support» activado, TurboMON la lee de ahí.</div>`}
    <div class="chart">${spark(l.hist.cpu, 100, undefined, 70)}</div></div>`;
  const gpuCards = (l.gpus || []).map(g => `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#22d3ee,#6366f1)">${IC.gpu}</div><h2>Gráfica</h2><span class="meta">${esc(g.name)}</span></div>
    <div class="bigline"><span class="v">${n(g.usage)}<small>%</small></span>${g.temp ? `<span class="v2 ${tempCls(g.temp)}">${n(g.temp)}<small>°C</small></span>` : ""}${g.clockMHz ? `<span class="v2">${n(g.clockMHz)}<small>MHz</small></span>` : ""}</div>
    <div class="chart">${spark(g === mainGPU(l) ? l.hist.gpu : [], 100, undefined, 70)}</div>
    <dl class="kv"><dt>Memoria de vídeo (VRAM)</dt><dd>${gb(g.vramUsed)} / ${gb(g.vram, 0)} GB</dd></dl>${pctBar(g.vram ? g.vramUsed / g.vram * 100 : 0, g.vram && g.vramUsed / g.vram > .9 ? "warn" : "")}
    <dl class="kv" style="margin-top:10px"><dt>Reloj de la gráfica</dt><dd>${g.clockMHz ? n(g.clockMHz) + " MHz" : "—"}</dd><dt>Reloj de la memoria</dt><dd>${g.memClockMHz ? n(g.memClockMHz) + " MHz" : "—"}</dd>
    <dt>Ventilador</dt><dd>${g.fanRPM ? n(g.fanRPM) + " RPM" : "Parado (o sin dato)"}</dd><dt>Tipo</dt><dd>${g.dedicated ? "Dedicada" : "Integrada"} · ${g.outputs} pantalla${g.outputs === 1 ? "" : "s"}</dd></dl></div>`).join("");
  const used = m.used / m.total * 100, sb = Math.min(100 - used, m.standby / m.total * 100);
  const memCard = `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.ram}</div><h2>Memoria RAM</h2><span class="meta">${gb(m.total, 0)} GB</span></div>
    <div class="bigline"><span class="v">${gb(m.used)}<small>GB en uso</small></span><span class="v2">${n(m.load)}<small>%</small></span></div>
    <div class="membar"><i style="width:${used.toFixed(1)}%;background:linear-gradient(90deg,#8b5cf6,#6366f1)"></i><i style="width:${sb.toFixed(1)}%;background:rgba(34,211,238,.45)"></i></div>
    <div class="legend"><span><i style="background:#7c6cf5"></i>En uso ${gb(m.used)} GB</span><span><i style="background:rgba(34,211,238,.6)"></i>En espera ${gb(m.standby)} GB</span><span><i style="background:rgba(255,255,255,.12)"></i>Libre ${gb(m.free)} GB</span></div>
    <dl class="kv"><dt>Disponible para programas</dt><dd>${gb(m.available)} GB</dd><dt>Comprometida / límite</dt><dd>${gb(m.commit)} / ${gb(m.commitLimit, 0)} GB</dd></dl>
    <div class="note">La memoria <b>en espera</b> es caché: archivos que Windows guarda por si los vuelves a usar. Cuenta como disponible y Windows la suelta sola cuando un programa la necesita.</div>
    <div class="chart">${spark(l.hist.ram, m.total / GB, undefined, 60)}</div></div>`;
  const ioCard = `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#0891b2)">${IC.disk}</div><h2>Disco y red</h2></div>
    <dl class="kv"><dt>Disco: lectura</dt><dd>${rate(l.diskRead)}</dd><dt>Disco: escritura</dt><dd>${rate(l.diskWrite)}</dd><dt>Disco: actividad</dt><dd>${n(l.diskBusy)} %</dd></dl>
    <div class="chart" style="height:44px">${spark(l.hist.disk, 100, undefined, 44)}</div>
    <dl class="kv"><dt>Red: bajada</dt><dd>${rate(l.netDown)}</dd><dt>Red: subida</dt><dd>${rate(l.netUp)}</dd></dl>
    <div class="chart" style="height:44px">${spark(l.hist.net, undefined, undefined, 44)}</div>
    <dl class="kv"><dt>Encendido desde hace</dt><dd>${dur(l.uptime)}</dd>${l.battery >= 0 ? `<dt>Batería</dt><dd>${l.battery} % · ${l.plugged ? "enchufado" : "sin cargador"}</dd>` : ""}</dl></div>`;
  const fpsCard = `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#f472b6,#8b5cf6)">${IC.fps}</div><h2>FPS de lo que tienes delante</h2><span class="meta">${esc(f.name || f.exe || "")}</span></div>
    ${!f.available ? `<div class="empty">Para medir FPS hace falta dar permiso de administrador una vez. <button class="link" data-act="enablefps">Activar FPS</button></div>` : `
    <div class="bigline"><span class="v ${f.presenting ? fpsCls(f.fps) : ""}">${f.presenting ? n(f.fps) : "—"}<small>FPS</small></span>${f.presenting ? `<span class="v2">${n(f.low1)}<small>1 % bajo</small></span><span class="v2">${n(f.frameMs, 1)}<small>ms</small></span>` : ""}</div>
    <div class="chart" style="height:90px">${spark(l.hist.fps, undefined, undefined, 90)}</div>
    <dl class="kv"><dt>Programa</dt><dd>${esc(f.exe || "—")}</dd><dt>Peor fotograma (10 s)</dt><dd>${f.presenting ? n(f.maxMs, 1) + " ms" : "—"}</dd><dt>¿Es un juego?</dt><dd>${f.isGame ? "Sí: se está apuntando la partida" : "No (o aún no)"}</dd></dl>
    <div class="note">El <b>1 % bajo</b> son los FPS de los fotogramas más lentos: es lo que notas como tirones. Cuanto más cerca de los FPS medios, más suave.</div>`}</div>`;
  return `<div class="head"><div><h1>Sensores</h1><p class="sub">Todo en directo, una lectura por segundo.</p></div></div>
    <div class="grid g2">${cpuCard}${gpuCards || ""}${memCard}${fpsCard}${ioCard}</div>`;
}

// ---------- revisión ----------
function renderChecks() {
  const r = state.report;
  if (!r) return `<div class="empty">Revisando tu PC…</div>`;
  const [t, p] = scoreText(r);
  const groups = {};
  r.results.forEach(x => (groups[x.group] = groups[x.group] || []).push(x));
  const stTxt = { ok: "Bien", warn: "Mejorable", bad: "Mal", info: "Info" };
  const html = Object.keys(groups).map(g => `<div class="group"><h3>${esc(g)}</h3>${groups[g].map(x => {
    const open = state.open.has(x.id);
    return `<div class="chk ${x.status} ${open ? "open" : ""}" data-key="${esc(x.id)}">
      <div class="chk-row" data-act="toggle" data-arg="${esc(x.id)}"><span class="dot ${x.status}"></span><div class="t"><b>${esc(x.title)}</b></div>
        <span class="val">${esc(x.value)}</span><span class="pill-st ${x.status}">${stTxt[x.status]}</span><span class="chev">${IC.chev}</span></div>
      <div class="chk-body"><p>${esc(x.detail)}</p>${x.howTo && x.howTo.length ? `<ol>${x.howTo.map(h => `<li>${esc(h)}</li>`).join("")}</ol>` : ""}
        ${x.fix ? `<button class="btn sm primary" data-act="fix" data-arg="${esc(x.fix)}" ${state.busy[x.fix] ? "disabled" : ""}>${IC.wrench}${esc(x.fixLabel || "Arreglar")}</button>` : ""}</div>
    </div>`;
  }).join("")}</div>`).join("");
  return `<div class="check-head">${ring(r.score, 104)}<div class="grow"><h1>${esc(t)}</h1><p class="sub">${esc(p)} Revisado ${when(r.time)}.</p></div>
      <button class="btn ghost" data-act="recheck" ${state.checking ? "disabled" : ""}>${IC.refresh}Revisar otra vez</button></div>
    <div class="groups">${html}</div>`;
}

// ---------- partidas ----------
const limitTxt = { gpu: "Limita la gráfica", cpu: "Limita el procesador", cap: "FPS limitados" };
function prevOf(s) {
  const list = state.sessions || [];
  const i = list.findIndex(x => x.id === s.id);
  return list.slice(i + 1).find(x => x.game === s.game);
}
function delta(v, prev) {
  if (prev == null || !prev) return "";
  const d = (v - prev) / prev * 100;
  if (Math.abs(d) < 2) return `<span class="delta">=</span>`;
  return `<span class="delta ${d > 0 ? "up" : "down"}">${d > 0 ? "▲" : "▼"} ${n(Math.abs(d))} %</span>`;
}
function verdict(s) {
  if (s.limit === "gpu") return `<b>La gráfica va al máximo (${n(s.gpu.avg)} %):</b> es lo normal en juegos exigentes. Para más FPS, baja la calidad o la resolución, o activa DLSS/FSR.`;
  if (s.limit === "cpu") return `<b>Lo que frena es el procesador</b>: algún hilo va casi al 100 % mientras la gráfica (${n(s.gpu.avg)} %) tiene margen. Cierra lo que tengas de fondo; subir gráficos aquí casi no te quitaría FPS.`;
  if (s.limit === "cap") return `<b>Los FPS están clavados en ${s.cap}:</b> hay un límite puesto (sincronización vertical, el límite del juego o el de la gráfica). El equipo podría dar más.`;
  return `Ni la gráfica ni el procesador van al máximo: el juego va holgado.`;
}
function renderGames() {
  const list = state.sessions;
  if (!list) return `<div class="empty">Cargando partidas…</div>`;
  if (!list.length) return `<div class="head"><div><h1>Partidas</h1><p class="sub">Aquí aparecerán solas cuando juegues.</p></div></div>
    <div class="card"><div class="empty">TurboMON se da cuenta de cuándo tienes un juego delante y apunta cómo va: <b>FPS medios, 1 % bajo, CPU, gráfica, RAM y temperaturas</b>, con una gráfica de toda la partida. Cada una ocupa unos pocos KB.</div></div>`;
  const sel = state.selData;
  const items = list.map(s => `<button class="gitem ${state.sel === s.id ? "on" : ""}" data-act="opengame" data-arg="${esc(s.id)}" data-key="${esc(s.id)}">
      <div class="n"><b>${esc(s.game)}</b><span class="${fpsCls(s.fpsAvg)}">${n(s.fpsAvg)}<small> FPS</small></span></div>
      <div class="d"><span>${when(s.start)}</span><span>${dur(s.secs)}</span>${s.limit ? `<span class="limit ${s.limit}">${limitTxt[s.limit]}</span>` : ""}</div></button>`).join("");
  let detail = `<div class="card"><div class="empty">Elige una partida.</div></div>`;
  if (sel) {
    const p = prevOf(sel);
    detail = `<div class="card"><div class="card-h"><div class="ic">${IC.game}</div><h2>${esc(sel.game)}</h2><span class="meta">${when(sel.start)} · ${dur(sel.secs)}</span>
        <button class="btn sm ghost" data-act="delgame" data-arg="${esc(sel.id)}" title="Borrar esta partida">${IC.trash}</button></div>
      <div class="stats">
        ${statBox(n(sel.fpsAvg) + delta(sel.fpsAvg, p && p.fpsAvg), "", "FPS de media" + (p ? " (vs la anterior)" : ""), fpsCls(sel.fpsAvg))}
        ${statBox(n(sel.low1) + delta(sel.low1, p && p.low1), "", "1 % bajo", fpsCls(sel.low1))}
        ${statBox(n(sel.low01), "", "0,1 % bajo", fpsCls(sel.low01))}
        ${statBox(n(sel.gpu.avg), "%", `gráfica (máx. ${n(sel.gpu.max)})`)}
        ${statBox(n(sel.cpu.avg), "%", `procesador (hilo ${n(sel.maxCore)})`)}
        ${statBox(n(sel.ram.avg, 1), "GB", `RAM de ${n(sel.ramTotal, 0)}`)}
        ${sel.vramTotal ? statBox(n(sel.vram.avg, 1), "GB", `VRAM de ${n(sel.vramTotal, 0)}`) : ""}
        ${sel.gpuTemp.max ? statBox(n(sel.gpuTemp.max), "°C", "gráfica, lo más alto", tempCls(sel.gpuTemp.max)) : ""}
        ${sel.cpuTemp.max ? statBox(n(sel.cpuTemp.max), "°C", "procesador, lo más alto", tempCls(sel.cpuTemp.max)) : ""}
      </div>
      <div class="gchart"><canvas id="gchart"></canvas></div>
      <div class="legend"><span><i style="background:linear-gradient(90deg,#8b5cf6,#22d3ee)"></i>FPS</span><span><i style="background:#22d3ee;opacity:.6"></i>Gráfica %</span><span><i style="background:#fbbf24;opacity:.7"></i>Procesador %</span></div>
      <div class="verdict">${IC.info}<div>${verdict(sel)}${sel.turbo ? ` Con el modo juego Turbo${sel.purges ? ` (soltó la memoria en espera ${sel.purges} ${sel.purges === 1 ? "vez" : "veces"})` : ""}.` : ""}</div></div></div>`;
  }
  return `<div class="head"><div><h1>Partidas</h1><p class="sub">${list.length} guardadas · se apuntan solas cuando juegas.</p></div><span class="grow"></span>
      <button class="btn sm ghost" data-act="cleargames">${IC.trash}Borrar todas</button></div>
    <div class="games"><div class="glist">${items}</div><div>${detail}</div></div>`;
}

function drawGameChart() {
  const cv = document.getElementById("gchart");
  const s = state.selData;
  if (!cv || !s || !s.series) return;
  const dpr = window.devicePixelRatio || 1, w = cv.clientWidth, h = cv.clientHeight;
  cv.width = w * dpr; cv.height = h * dpr;
  const c = cv.getContext("2d");
  c.scale(dpr, dpr);
  c.clearRect(0, 0, w, h);
  const fps = s.series.fps || [], len = fps.length;
  if (len < 2) return;
  const maxF = Math.max(...fps) * 1.12 || 1;
  const L = 34, B = 18, T = 6, W = w - L - 4, H = h - B - T;
  c.font = "11px Segoe UI"; c.fillStyle = "#5f6580"; c.strokeStyle = "rgba(255,255,255,.06)"; c.lineWidth = 1;
  for (let i = 0; i <= 4; i++) {
    const y = T + H - H * i / 4;
    c.beginPath(); c.moveTo(L, y); c.lineTo(L + W, y); c.stroke();
    c.fillText(Math.round(maxF * i / 4), 2, y + 4);
  }
  const total = len * s.series.step;
  for (let i = 0; i <= 4; i++) {
    const x = L + W * i / 4;
    c.fillText(dur(total * i / 4).replace(" 0 min", ""), Math.min(x, L + W - 40), h - 3);
  }
  const X = i => L + W * i / (len - 1);
  const line = (vals, max, style, width, fill) => {
    c.beginPath();
    vals.forEach((v, i) => { const y = T + H - H * Math.min(v, max) / max; i ? c.lineTo(X(i), y) : c.moveTo(X(i), y); });
    c.strokeStyle = style; c.lineWidth = width; c.stroke();
    if (fill) { c.lineTo(X(vals.length - 1), T + H); c.lineTo(X(0), T + H); c.closePath(); c.fillStyle = fill; c.fill(); }
  };
  line(s.series.gpu || [], 100 * maxF / maxF, "rgba(34,211,238,.45)", 1.2);
  line(s.series.cpu || [], 100, "rgba(251,191,36,.5)", 1.2);
  const g = c.createLinearGradient(L, 0, L + W, 0); g.addColorStop(0, "#8b5cf6"); g.addColorStop(1, "#22d3ee");
  const fg = c.createLinearGradient(0, T, 0, T + H); fg.addColorStop(0, "rgba(99,102,241,.32)"); fg.addColorStop(1, "rgba(99,102,241,0)");
  line(fps, maxF, g, 2, fg);
}

// ---------- herramientas ----------
function renderTools() {
  const s = state.settings, l = state.live;
  if (!s || !l) return `<div class="empty">Cargando…</div>`;
  const elev = l.agent.elevated;
  const sw = (key, on) => `<button class="sw sm ${on ? "on" : ""}" data-act="flag" data-arg="${key}"></button>`;
  const procs = (state.procs || []).slice().sort((a, b) => state.procSort === "cpu" ? b.cpu - a.cpu : b.memMB - a.memMB).slice(0, 14);
  const maxMem = Math.max(1, ...procs.map(p => p.memMB)), maxCpu = Math.max(1, ...procs.map(p => p.cpu));
  const rows = procs.map(p => `<tr data-key="${esc(p.exe)}"><td class="n">${esc(p.name)}${p.count > 1 ? `<small>${p.count} procesos</small>` : ""}</td>
      <td class="r"><span class="mini"><i style="width:${(p.cpu / maxCpu * 100).toFixed(0)}%"></i></span>${n(p.cpu, 1)} %</td>
      <td class="r"><span class="mini"><i style="width:${(p.memMB / maxMem * 100).toFixed(0)}%"></i></span>${p.memMB >= 1024 ? n(p.memMB / 1024, 1) + " GB" : n(p.memMB) + " MB"}</td>
      <td class="r">${p.canKill ? `<button class="btn sm ghost" data-act="kill" data-arg="${esc(p.exe)}" data-name="${esc(p.name)}">Cerrar</button>` : `<span class="note" style="margin:0">de Windows</span>`}</td></tr>`).join("");
  return `<div class="head"><div><h1>Herramientas</h1><p class="sub">Lo que de verdad ayuda a que los juegos vayan mejor (y nada de placebo).</p></div></div>
    <div class="grid g2">
      <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#fbbf24,#f97316);--tile-sh:rgba(245,158,11,.6)">${IC.bolt}</div><h2>Modo juego Turbo</h2>${sw("turboMode", s.turboMode)}</div>
        <p class="sub" style="margin:0 0 6px">Se activa solo cuando empiezas a jugar y lo deja todo como estaba al cerrar el juego.</p>
        <div style="opacity:${s.turboMode ? 1 : .45}">
        <div class="opt"><div class="t"><b>Energía al máximo</b><p>Pone el plan «Alto/Máximo rendimiento» durante la partida.</p></div>${sw("turboPlan", s.turboPlan)}</div>
        <div class="opt"><div class="t"><b>Prioridad para el juego</b><p>Windows le da el procesador antes que a lo que haya de fondo.</p></div>${sw("turboPrio", s.turboPrio)}</div>
        <div class="opt"><div class="t"><b>Soltar memoria en espera si se acaba</b><p>Solo si queda menos de 1 GB libre (evita tirones en equipos justos de RAM).${elev ? "" : " Necesita el permiso de administrador."}</p></div>${sw("turboStandby", s.turboStandby)}</div></div></div>
      <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#22d3ee,#6366f1)">${IC.broom}</div><h2>Memoria en espera</h2><span class="meta">${gb(l.mem.standby)} GB ahora</span></div>
        <p class="sub" style="margin:0 0 12px">Es la caché de Windows. Normalmente es buena y se suelta sola; en algunos juegos con poca RAM, soltarla a mano quita tirones.</p>
        <div class="membar"><i style="width:${(l.mem.used / l.mem.total * 100).toFixed(1)}%;background:linear-gradient(90deg,#8b5cf6,#6366f1)"></i><i style="width:${(l.mem.standby / l.mem.total * 100).toFixed(1)}%;background:rgba(34,211,238,.45)"></i></div>
        <div class="legend"><span><i style="background:#7c6cf5"></i>En uso ${gb(l.mem.used)} GB</span><span><i style="background:rgba(34,211,238,.6)"></i>En espera ${gb(l.mem.standby)} GB</span><span><i style="background:rgba(255,255,255,.12)"></i>Libre ${gb(l.mem.free)} GB</span></div>
        ${elev ? `<button class="btn primary" data-act="purge" ${state.busy.purge ? "disabled" : ""}>${IC.broom}Liberar ahora</button>` : `<button class="btn amber" data-act="enablefps">${IC.shield}Dar permiso para poder liberarla</button>`}
        <div class="note"><b>¿Y lo de «limpiar la RAM» de otros programas?</b> Vacían la memoria de los programas abiertos: Windows la tiene que volver a cargar enseguida y lo que consigues son tirones. TurboMON no lo hace.</div></div>
    </div>
    <div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.list}</div><h2>Lo que más gasta ahora</h2>
        <button class="chip ${state.procSort === "mem" ? "on" : ""}" data-act="psort" data-arg="mem">Por memoria</button><button class="chip ${state.procSort === "cpu" ? "on" : ""}" data-act="psort" data-arg="cpu">Por procesador</button></div>
      ${state.procs ? `<table class="procs"><thead><tr><th>Programa</th><th class="r">Procesador</th><th class="r">Memoria</th><th></th></tr></thead><tbody>${rows}</tbody></table>` : `<div class="empty">Mirando…</div>`}
      <div class="note">Cierra lo que no uses antes de jugar (sobre todo el navegador con muchas pestañas). Lo que es parte de Windows no se puede cerrar desde aquí.</div></div>`;
}

// ---------- ajustes ----------
function ovPreview(s, l) {
  const o = s.overlay;
  const g = mainGPU(l) || {};
  const f = l && l.fps && l.fps.presenting ? l.fps : { fps: 144, low1: 101, frameMs: 6.9 };
  const fc = v => v >= 60 ? "#34d399" : v >= 30 ? "#fbbf24" : "#f87171";
  const rows = [];
  let top = `<span class="f" style="color:${fc(f.fps)}">${n(f.fps)}</span><span class="s">FPS</span>`;
  if (!o.fpsOnly && o.low1) top += `<span class="s">&nbsp; 1 %</span><span class="n" style="color:${fc(f.low1)}">${n(f.low1)}</span>`;
  if (!o.fpsOnly && o.frameMs) top += `<span class="s">&nbsp; ${n(f.frameMs, 1)} ms</span>`;
  rows.push(`<div class="r">${top}</div>`);
  if (!o.fpsOnly) {
    const c = l ? l.cpu : { usage: 41, temp: 66, mhz: 4600 };
    let cpu = "";
    if (o.cpu) cpu += `<span class="n">${n(c.usage)} %</span>`;
    if (o.cpuTemp && c.temp) cpu += `<span class="n">&nbsp;${n(c.temp)}°</span>`;
    if (o.clock && c.mhz) cpu += `<span class="s">&nbsp;${n(c.mhz / 1000, 1)} GHz</span>`;
    if (cpu) rows.push(`<div class="r"><span class="l">CPU</span>${cpu}</div>`);
    let gp = "";
    if (o.gpu) gp += `<span class="n">${n(g.usage || 97)} %</span>`;
    if (o.gpuTemp && (g.temp || !l)) gp += `<span class="n">&nbsp;${n(g.temp || 68)}°</span>`;
    if (o.clock && g.clockMHz) gp += `<span class="s">&nbsp;${n(g.clockMHz)} MHz</span>`;
    if (gp) rows.push(`<div class="r"><span class="l">GPU</span>${gp}</div>`);
    if (o.ram && l) rows.push(`<div class="r"><span class="l">RAM</span><span class="n">${gb(l.mem.used)}</span><span class="s">/ ${gb(l.mem.total, 0)} GB</span></div>`);
    if (o.vram && g.vram) rows.push(`<div class="r"><span class="l">VRAM</span><span class="n">${gb(g.vramUsed)}</span><span class="s">/ ${gb(g.vram, 0)} GB</span></div>`);
  }
  const k = o.scale / 100 * 0.82;
  const pos = { ti: "top:10px;left:10px", td: "top:10px;right:10px;transform-origin:top right", bi: "bottom:10px;left:10px;transform-origin:bottom left", bd: "bottom:10px;right:10px;transform-origin:bottom right" }[o.corner];
  return `<div class="ov-preview"><div class="scene"></div>${o.on ? `<div class="ovbox" style="${pos};transform:scale(${k.toFixed(2)});opacity:${o.opacity / 100}">${rows.join("")}</div>` : ""}</div>`;
}

function renderSettings() {
  const s = state.settings, l = state.live;
  if (!s) return `<div class="empty">Cargando…</div>`;
  const o = s.overlay;
  const chip = (k, label) => `<button class="chip ${o[k] ? "on" : ""}" data-act="ovflag" data-arg="${k}">${label}</button>`;
  const sw = (act, arg, on) => `<button class="sw sm ${on ? "on" : ""}" data-act="${act}" data-arg="${arg}"></button>`;
  const a = l ? l.agent : null;
  return `<div class="head"><div><h1>Ajustes</h1><p class="sub">Los cambios se guardan y se aplican al momento.</p></div></div>
    <div class="grid g2">
      <div class="card"><div class="card-h"><div class="ic">${IC.fps}</div><h2>Overlay</h2>${sw("overlay", "", o.on)}</div>
        ${ovPreview(s, l)}
        <div class="opt" style="margin-top:8px"><div class="t"><b>Qué enseña</b></div>
          <button class="chip ${!o.fpsOnly ? "on" : ""}" data-act="ovmode" data-arg="full">Completo</button><button class="chip ${o.fpsOnly ? "on" : ""}" data-act="ovmode" data-arg="fps">Solo FPS</button></div>
        <div class="chips" style="opacity:${o.fpsOnly ? .4 : 1}">${chip("low1", "1 % bajo")}${chip("frameMs", "Milisegundos")}${chip("cpu", "Uso de CPU")}${chip("cpuTemp", "Temp. CPU")}${chip("gpu", "Uso de gráfica")}${chip("gpuTemp", "Temp. gráfica")}${chip("ram", "RAM")}${chip("vram", "VRAM")}${chip("clock", "Frecuencias")}</div>
        <div class="opt"><div class="t"><b>Dónde</b><p>Esquina de la pantalla en la que está el juego.</p></div>
          <div class="corners">${["ti", "td", "bi", "bd"].map(c => `<button data-c="${c}" class="${o.corner === c ? "on" : ""}" data-act="corner" data-arg="${c}" title="${{ ti: "Arriba a la izquierda", td: "Arriba a la derecha", bi: "Abajo a la izquierda", bd: "Abajo a la derecha" }[c]}"></button>`).join("")}</div></div>
        <div class="opt"><div class="t"><b>Tamaño</b></div><div class="range" style="width:240px"><input type="range" min="75" max="200" step="5" value="${o.scale}" data-range="scale"><span>${o.scale} %</span></div></div>
        <div class="opt"><div class="t"><b>Opacidad</b></div><div class="range" style="width:240px"><input type="range" min="40" max="100" step="2" value="${o.opacity}" data-range="opacity"><span>${o.opacity} %</span></div></div>
        <div class="opt"><div class="t"><b>Solo con un juego delante</b><p>Se esconde en el escritorio, el navegador…</p></div>${sw("ovflag", "gameOnly", o.gameOnly)}</div>
        <div class="opt"><div class="t"><b>Atajo <span class="kbd">Ctrl+Alt+O</span></b><p>Pone y quita el overlay desde el juego.</p></div>${sw("ovflag", "hotkey", o.hotkey)}</div>
        <div class="note">El overlay se ve encima de los juegos en <b>ventana</b> o <b>pantalla completa sin bordes</b>. En «pantalla completa exclusiva» Windows no deja poner nada encima: cámbialo en las opciones gráficas del juego (casi todos lo tienen). No se mete en el juego, así que no da problemas con los antitrampas.</div>
      </div>
      <div style="display:flex;flex-direction:column;gap:14px">
        <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#059669);--tile-sh:rgba(16,185,129,.5)">${IC.shield}</div><h2>Medida de FPS</h2></div>
          ${a && a.elevated ? `<div class="allgood">${IC.ok}<div><b>Activada.</b> TurboMON arranca con Windows con permiso para ver los fotogramas (sin preguntarte cada vez).</div></div>`
            : `<p class="sub" style="margin:0 0 12px">Para medir los FPS, Windows exige permiso de administrador (es lo mismo que necesitan MSI Afterburner o PresentMon). Se pide <b>una vez</b> y TurboMON arranca así con Windows.</p><button class="btn amber" data-act="enablefps">${IC.shield}Activar FPS</button>`}</div>
        <div class="card"><div class="card-h"><div class="ic">${IC.game}</div><h2>Partidas</h2>${sw("flag", "sessions", s.sessions)}</div>
          <p class="sub" style="margin:0 0 10px">Apuntar cada partida (FPS, CPU, gráfica, temperaturas). Ocupa unos 3-15 KB por partida.</p>
          <dl class="kv"><dt>Guardadas</dt><dd>${state.sessions ? state.sessions.length : "—"}</dd><dt>Espacio</dt><dd>${a ? size(a.histBytes) : "—"}</dd></dl>
          <div style="margin-top:12px"><button class="btn sm danger" data-act="cleargames">${IC.trash}Borrar todas las partidas</button></div></div>
        <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#64748b,#334155)">${IC.info}</div><h2>TurboMON</h2></div>
          <dl class="kv"><dt>Versión</dt><dd>${a ? esc(a.version) : "—"}</dd><dt>Lo que gasta en segundo plano</dt><dd>${a ? `${n(a.cpu, 2)} % CPU · ${n(a.memMB)} MB` : "—"}</dd>
          <dt>Abierto desde hace</dt><dd>${a ? dur(a.startedAgo) : "—"}</dd></dl>
          <div class="note">Sin nada mirando (ni overlay, ni esta ventana, ni un juego), solo lee procesador y memoria cada 5 s. Al cerrar esta ventana, ella deja de gastar del todo: TurboMON sigue junto a la flechita de la barra de tareas.</div>
          <div style="margin-top:12px"><button class="btn sm ghost" data-act="quitall">Cerrar TurboMON del todo</button></div></div>
      </div>
    </div>`;
}

// ---------- pintar ----------
const views = { home: renderHome, sensors: renderSensors, checks: renderChecks, games: renderGames, tools: renderTools, settings: renderSettings };
function render() {
  const el = document.getElementById(state.view === "games" ? "games" : state.view);
  morph(el, views[state.view]());
  if (state.view === "games") requestAnimationFrame(drawGameChart);
  // aviso de FPS sin permiso y medalla de la revisión
  const b = $("#banner"), l = state.live;
  const showBanner = l && !l.fps.available && state.view === "home";
  b.hidden = !showBanner;
  if (showBanner) morph(b, `${IC.warn}<div class="txt"><b>Los FPS están desactivados.</b> Windows pide permiso de administrador una vez para poder verlos. El resto funciona igual.</div><button class="btn sm amber" data-act="enablefps">Activar FPS</button>`);
  const r = state.report, badge = $("#checks-badge");
  if (r) {
    const bad = r.results.filter(x => x.status === "bad").length, warn = r.results.filter(x => x.status === "warn").length;
    badge.textContent = bad + warn ? String(bad + warn) : "";
    badge.className = "badge" + (bad ? " bad" : "");
  }
}

function goView(v) {
  state.view = v;
  document.querySelectorAll(".nav button").forEach(b => b.classList.toggle("on", b.dataset.v === v));
  document.querySelectorAll(".view").forEach(x => x.classList.toggle("on", x.id === "v-" + v));
  const inner = document.getElementById(v);
  inner.classList.remove("anim"); void inner.offsetWidth; inner.classList.add("anim");
  if (v === "games" || v === "home" || v === "settings") loadSessions();
  if (v === "checks" && (!state.report || Date.now() / 1000 - state.report.time > 60)) runChecks();
  if (v === "tools") loadProcs();
  render();
}

// ---------- datos ----------
async function poll() {
  try {
    state.live = await App.Live();
    state.offline = false;
    // El overlay se puede poner y quitar desde la bandeja o con el atajo: los
    // ajustes se vuelven a leer si no cuadran, y cada 10 s por si acaso.
    if (state.settings && (state.live.agent.overlay !== state.settings.overlay.on || Date.now() - (poll.last || 0) > 10000)) {
      poll.last = Date.now();
      state.settings = await App.Settings();
    }
  } catch (e) {
    state.offline = true;
  }
  if (!document.hidden) render();
  setTimeout(poll, 1000);
}
async function loadSettings() { try { state.settings = await App.Settings(); } catch (e) { } render(); }
async function loadSessions() {
  try {
    state.sessions = await App.Sessions();
    if (state.view === "games" && state.sessions.length && !state.sessions.find(s => s.id === state.sel)) openGame(state.sessions[0].id);
  } catch (e) { state.sessions = state.sessions || []; }
  render();
}
async function openGame(id) {
  state.sel = id;
  render();
  try { state.selData = await App.Session(id); } catch (e) { state.selData = null; }
  render();
}
async function runChecks() {
  state.checking = true; render();
  try { state.report = await App.Checks(); } catch (e) { toast(errMsg(e), true); }
  state.checking = false; render();
}
async function loadProcs() {
  if (state.view !== "tools") return;
  try { state.procs = await App.Processes(); } catch (e) { }
  render();
  clearTimeout(loadProcs.t);
  loadProcs.t = setTimeout(loadProcs, 2500);
}
async function saveSettings(mut) {
  const s = JSON.parse(JSON.stringify(state.settings));
  mut(s);
  state.settings = s; render();
  try { state.settings = await App.SaveSettings(s); } catch (e) { toast(errMsg(e), true); }
  render();
}

// ---------- acciones ----------
const actions = {
  go: a => goView(a),
  overlay: async () => {
    const on = !state.settings.overlay.on;
    state.settings.overlay.on = on; render();
    try { state.settings = await App.SetOverlay(on); } catch (e) { toast(errMsg(e), true); }
    toast(on ? "Overlay puesto (Ctrl+Alt+O para quitarlo)" : "Overlay quitado");
    render();
  },
  ovmode: a => saveSettings(s => { s.overlay.fpsOnly = a === "fps"; s.overlay.on = true; }),
  ovflag: a => saveSettings(s => { s.overlay[a] = !s.overlay[a]; }),
  corner: a => saveSettings(s => { s.overlay.corner = a; s.overlay.on = true; }),
  flag: a => saveSettings(s => { s[a] = !s[a]; }),
  fix: async id => {
    state.busy[id] = true; render();
    try { state.report = await App.Fix(id); toast("Arreglado"); } catch (e) { toast("No se pudo: " + errMsg(e), true); }
    delete state.busy[id]; render();
  },
  howto: id => { state.open.add(id); goView("checks"); setTimeout(() => { const el = document.querySelector(`.chk[data-key="${CSS.escape(id)}"]`); el && el.scrollIntoView({ block: "center", behavior: "smooth" }); }, 50); },
  toggle: id => { state.open.has(id) ? state.open.delete(id) : state.open.add(id); render(); },
  recheck: () => runChecks(),
  opengame: id => { if (state.view !== "games") { state.sel = id; goView("games"); } openGame(id); },
  delgame: async id => {
    if (!await confirmBox("¿Borrar esta partida?", "Se borra del historial. No se puede deshacer.", "Borrar", true)) return;
    try { await App.DeleteSession(id); } catch (e) { toast(errMsg(e), true); }
    state.sel = null; state.selData = null; loadSessions();
  },
  cleargames: async () => {
    if (!await confirmBox("¿Borrar todas las partidas?", "Se borra todo el historial de partidas. No se puede deshacer.", "Borrar todas", true)) return;
    try { await App.ClearSessions(); } catch (e) { toast(errMsg(e), true); }
    state.sel = null; state.selData = null; loadSessions();
  },
  purge: async () => {
    state.busy.purge = true; render();
    try {
      const r = await App.Purge();
      if (r && r.ok) toast(`Liberados ${gb(r.freed)} GB de memoria en espera`); else toast((r && r.error) || "No se pudo", true);
    } catch (e) { toast(errMsg(e), true); }
    delete state.busy.purge; render();
  },
  enablefps: async () => {
    toast("Windows te va a pedir permiso…");
    try { await App.EnableFPS(); toast("FPS activados"); } catch (e) { toast(errMsg(e), true); }
  },
  psort: a => { state.procSort = a; render(); },
  kill: async (exe, el) => {
    const name = el.dataset.name || exe;
    if (!await confirmBox(`¿Cerrar ${name}?`, "Se cierran todos sus procesos. Si tenía algo sin guardar, se perderá.", "Cerrar", true)) return;
    try { const k = await App.KillGroup(exe); toast(`${name} cerrado (${k} proceso${k === 1 ? "" : "s"})`); } catch (e) { toast(errMsg(e), true); }
    loadProcs();
  },
  quitall: async () => {
    if (!await confirmBox("¿Cerrar TurboMON del todo?", "Se quitan el overlay y el icono de la bandeja, y deja de apuntar partidas hasta que lo vuelvas a abrir (o reinicies).", "Cerrar del todo", true)) return;
    App.QuitAgent();
  },
};

document.addEventListener("click", e => {
  const nb = e.target.closest(".nav button");
  if (nb) return goView(nb.dataset.v);
  const b = e.target.closest("[data-act]");
  if (!b || b.disabled) return;
  const f = actions[b.dataset.act];
  if (f) f(b.dataset.arg, b);
});
document.addEventListener("input", e => {
  const k = e.target.dataset && e.target.dataset.range;
  if (!k || !state.settings) return;
  state.settings.overlay[k] = +e.target.value;
  render(); // la vista previa cambia mientras se arrastra
});
document.addEventListener("change", e => {
  const k = e.target.dataset && e.target.dataset.range;
  if (k) saveSettings(s => { s.overlay[k] = +e.target.value; });
});
$("#wb-min").onclick = () => RT.WindowMinimise();
$("#wb-max").onclick = () => RT.WindowToggleMaximise();
$("#wb-close").onclick = () => RT.Quit();
window.addEventListener("resize", () => state.view === "games" && requestAnimationFrame(drawGameChart));
document.addEventListener("visibilitychange", () => !document.hidden && render());

loadSettings();
runChecks();
loadSessions();
poll();
goView("home");
