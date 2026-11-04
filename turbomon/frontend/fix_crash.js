"use strict";
// ---------- «Se me cierra el juego» ----------
const KIC = {
  crash: '<svg viewBox="0 0 24 24"><path d="M18 6 6 18M6 6l12 12"/></svg>',
  hang: '<svg viewBox="0 0 24 24"><rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/></svg>',
  gpu: '<svg viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="2"/><circle cx="9" cy="12" r="3"/><path d="M15 10h4M15 14h4"/></svg>',
  bsod: '<svg viewBox="0 0 24 24"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4M9 9h.01M15 9h.01M9 13c1-1 5-1 6 0"/></svg>',
  reboot: '<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>',
  power: '<svg viewBox="0 0 24 24"><path d="M12 2v10M18.4 6.6a9 9 0 1 1-12.8 0"/></svg>',
  hw: '<svg viewBox="0 0 24 24"><rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"/></svg>',
};

fixPanels.crash = {
  title: "Se me cierra el juego",
  sub: "Cierres, cuelgues de la gráfica, pantallazos y reinicios de los últimos 60 días, explicados.",
  load: async force => {
    if (!force && state.crashes) return;
    try { state.crashes = await App.Crashes(); } catch (e) { }
    render();
  },
  status: () => {
    const r = state.crashes;
    if (!r) return { level: "idle", text: "Mirando el registro de Windows…" };
    const week = Date.now() / 1000 - 7 * 86400;
    const serious = r.incidents.filter(i => i.level === "bad");
    if (!serious.length) return { level: "ok", text: "Ningún juego se ha cerrado en 60 días" };
    const parts = [];
    if (r.crashes) parts.push(`${r.crashes} cierre${r.crashes === 1 ? "" : "s"}`);
    if (r.gpuResets) parts.push(`${r.gpuResets} cuelgue${r.gpuResets === 1 ? "" : "s"} de la gráfica`);
    if (r.reboots) parts.push(`${r.reboots} reinicio${r.reboots === 1 ? "" : "s"}`);
    return { level: serious.some(i => i.time > week) ? "bad" : "warn", text: parts.join(", ") + " en 60 días" };
  },
  render: () => {
    const r = state.crashes;
    if (!r) return `<div class="empty">Mirando el registro de Windows…</div>`;
    const tiles = [
      ["crash", r.crashes, "juegos cerrados"], ["gpu", r.gpuResets, "cuelgues de la gráfica"],
      ["reboot", r.reboots, "reinicios o pantallazos"], ["hw", r.hwErrors, "errores de hardware"],
    ].map(([k, v, l]) => `<div class="stat inc-stat ${v ? "has" : ""}"><div class="inc-ic ${k}">${KIC[k]}</div><b>${v}</b><span>${l}</span></div>`).join("");
    const advice = r.advice ? `<div class="verdict warnv">${IC.warn}<div><b>Ojo:</b> ${esc(r.advice)}
      <div class="btns-inline"><button class="btn sm amber" data-act="go" data-arg="checks">Ver la revisión</button><button class="btn sm ghost" data-act="sub" data-arg="stable">Probar la estabilidad</button></div></div></div>` : "";
    const list = r.incidents.length ? r.incidents.map(i => {
      const open = state.open.has(i.id);
      return `<div class="inc ${i.level} ${open ? "open" : ""}" data-key="${esc(i.id)}">
        <div class="inc-row" data-act="toggle" data-arg="${esc(i.id)}"><div class="inc-ic ${i.kind}">${KIC[i.kind] || ""}</div>
          <div class="t"><b>${esc(i.title)}</b><span>${when(i.time)}${i.app ? " · " + esc(i.app) : ""}${i.count > 1 ? ` · ${i.count} veces seguidas` : ""}</span></div>
          <span class="chev">${IC.chev}</span></div>
        <div class="inc-body"><p>${esc(i.cause)}</p>${i.module ? `<p class="dimp">Dónde falló: <b>${esc(i.module)}</b>${i.code ? " · código " + esc(i.code) : ""}</p>` : ""}
          ${i.steps && i.steps.length ? `<ol>${i.steps.map(s => `<li>${esc(s)}</li>`).join("")}</ol>` : ""}</div></div>`;
    }).join("") : `<div class="allgood">${IC.ok}<div><b>Todo tranquilo.</b> En los últimos 60 días no se ha cerrado ningún juego ni se ha colgado la gráfica.</div></div>`;
    return `<div class="grid g4">${tiles}</div>${advice}
      <div class="incs mt">${list}</div>
      ${r.otherApps ? `<div class="note">Además se cerraron ${r.otherApps} veces programas que no son juegos (navegadores, instaladores…): no se enseñan aquí.</div>` : ""}`;
  },
};
