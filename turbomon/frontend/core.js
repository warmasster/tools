"use strict";
const App = window.go.main.App;
const RT = window.runtime;
const $ = s => document.querySelector(s);
const GB = 1 << 30;

const state = {
  view: "home", live: null, settings: null, report: null, checking: false, open: new Set(),
  sessions: null, sel: null, selData: null, procs: null, procSort: "mem", busy: {}, offline: false, sub: {},
};

// Vistas: cada v_*.js se apunta aquí con { render, enter, after }.
const views = {};

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


// Acciones de los botones (data-act): cada vista añade las suyas.
const actions = {};
