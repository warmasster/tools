"use strict";
// ---------- solucionar ----------
// Una tarjeta por problema («se me cierra el juego», «me va a tirones»…). Cada
// una abre su detalle. Los detalles están en fix_*.js.

const FIC = {
  crash: '<svg viewBox="0 0 24 24"><path d="M12 9v4M12 17h.01"/><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/></svg>',
  focus: '<svg viewBox="0 0 24 24"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>',
  stutter: '<svg viewBox="0 0 24 24"><path d="M2 12h3l2-6 3 12 2-8 2 4h3l2-2h3"/></svg>',
  lag: '<svg viewBox="0 0 24 24"><path d="M5 12.55a11 11 0 0 1 14 0M1.42 9a16 16 0 0 1 21.16 0M8.53 16.11a6 6 0 0 1 6.95 0M12 20h.01"/></svg>',
  dual: '<svg viewBox="0 0 24 24"><rect x="1" y="4" width="13" height="10" rx="1.5"/><rect x="10" y="9" width="13" height="10" rx="1.5"/></svg>',
  stable: '<svg viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/></svg>',
  back: '<svg viewBox="0 0 24 24"><path d="m15 18-6-6 6-6"/></svg>',
};

// fixCards devuelve las tarjetas que tocan en este PC, con su estado.
// Cada detalle (fix_*.js) se apunta en fixPanels.
const fixPanels = {};

function fixCards() {
  const out = [];
  for (const [id, p] of Object.entries(fixPanels)) {
    if (p.applies && !p.applies()) continue;
    const st = p.status ? p.status() : { level: "ok", text: "" };
    out.push({ id, title: p.title, icon: FIC[id] || "", ...st });
  }
  return out;
}

// fixAlerts: cuántas cosas hay que mirar (para la medalla de la pestaña).
function fixAlerts() {
  return fixCards().filter(c => c.level === "bad").length;
}

// warmFix carga lo de todos los detalles (sin abrirlos) para la medalla y la
// franja de avisos de Inicio. Como mucho cada 5 minutos.
function warmFix(force) {
  if (!force && Date.now() - (state.fixAt || 0) < 300000) return;
  state.fixAt = Date.now();
  for (const p of Object.values(fixPanels)) if (p.load) try { p.load(false); } catch (e) { }
}

// fixStrip: los problemas graves, en una franja que se pulsa (como mucho 3).
function fixStrip() {
  const bad = fixCards().filter(c => c.level === "bad").slice(0, 3);
  if (!bad.length) return "";
  return `<div class="fix-strip">${bad.map(c => `<button class="fs-item" data-act="gofix" data-arg="${c.id}"><span class="fs-ic">${c.icon}</span><span class="t"><b>${esc(c.title)}</b><span>${esc(c.text)}</span></span>${IC.chev}</button>`).join("")}</div>`;
}

function renderFix() {
  const sub = state.sub.fix;
  if (sub && fixPanels[sub]) {
    const p = fixPanels[sub];
    return `<div class="head"><div><button class="link back" data-act="sub" data-arg="">${FIC.back}Solucionar</button><h1>${esc(p.title)}</h1>${p.sub ? `<p class="sub">${esc(p.sub)}</p>` : ""}</div></div>${p.render()}`;
  }
  const cards = fixCards();
  return `<div class="head"><div><h1>¿Algo va mal?</h1><p class="sub">Elige lo que te pasa. TurboMON lo vigila de fondo y te dice la causa en lenguaje normal.</p></div></div>
    <div class="fix-grid">${cards.map(c => `<button class="fix-card ${c.level}" data-act="sub" data-arg="${c.id}">
        <div class="fix-ic">${c.icon}</div><div class="fix-t"><b>${esc(c.title)}</b><span>${esc(c.text || "")}</span></div><span class="chev">${IC.chev}</span></button>`).join("")}</div>`;
}

views.fix = {
  render: renderFix,
  enter: () => { const p = fixPanels[state.sub.fix]; for (const q of Object.values(fixPanels)) if (q.load) q.load(q === p); },
  subEnter: id => { const p = fixPanels[id]; if (p && p.load) p.load(true); },
  after: () => { const p = fixPanels[state.sub.fix]; if (p && p.after) p.after(); },
};
