"use strict";
// ---------- «Me saca del juego» y «Ratón y ventana del juego» ----------

function gamesKnown() {
  const seen = new Map();
  for (const s of state.sessions || []) if (s.exe && !seen.has(s.exe.toLowerCase())) seen.set(s.exe.toLowerCase(), s.game);
  const l = state.live;
  if (l && l.session && l.fps && l.fps.exe && !seen.has(l.fps.exe.toLowerCase())) seen.set(l.fps.exe.toLowerCase(), l.session.game);
  return [...seen.entries()].map(([exe, name]) => ({ exe, name }));
}

async function loadProfiles() {
  try { state.profiles = await App.Profiles(); } catch (e) { state.profiles = state.profiles || {}; }
}

fixPanels.focus = {
  title: "Me saca del juego",
  sub: "Mientras juegas, TurboMON apunta qué programa se pone delante del juego sin que hayas tocado nada.",
  load: async force => {
    try { state.thefts = await App.Thefts(); } catch (e) { state.thefts = state.thefts || []; }
    const exes = [...new Set((state.thefts || []).map(t => t.exe))];
    if (exes.length) { try { state.startup = await App.StartupInfo(exes); } catch (e) { } }
    render();
  },
  status: () => {
    const t = state.thefts;
    if (!t) return { level: "idle", text: "Vigilando mientras juegas" };
    const week = Date.now() / 1000 - 7 * 86400;
    const recent = t.filter(x => x.time > week).length;
    if (!t.length) return { level: "ok", text: "Nadie te ha sacado del juego" };
    const names = [...new Set(t.map(x => x.name))];
    return { level: recent ? "bad" : "warn", text: `${t.length} ${t.length === 1 ? "vez" : "veces"}: ${names.slice(0, 2).join(", ")}${names.length > 2 ? "…" : ""}` };
  },
  render: () => {
    const t = state.thefts;
    if (!t) return `<div class="empty">Cargando…</div>`;
    const groups = new Map();
    for (const x of t) {
      const g = groups.get(x.exe) || { exe: x.exe, name: x.name, path: x.path, count: 0, last: 0, games: new Set() };
      g.count++; g.last = Math.max(g.last, x.time); if (x.game) g.games.add(x.game);
      groups.set(x.exe, g);
    }
    const list = [...groups.values()].sort((a, b) => b.count - a.count).map(g => {
      const inStartup = state.startup && state.startup[g.exe];
      return `<div class="card thief"><div class="thief-h"><div class="inc-ic bad">${FIC.focus}</div><div class="t"><b>${esc(g.name)}</b><span>${esc(g.exe)} · ${g.count} ${g.count === 1 ? "vez" : "veces"} · la última ${when(g.last)}</span></div></div>
        <p class="sub" style="margin:8px 0 12px">Te sacó de ${[...g.games].map(esc).join(", ") || "un juego"} sin que hubieras tocado el teclado ni el ratón.${thiefHint(g.exe)}</p>
        <div class="btns-inline">${inStartup ? `<button class="btn sm primary" data-act="nostartup" data-arg="${esc(g.exe)}">Que no arranque con Windows</button>` : ""}
          <button class="btn sm ghost" data-act="kill" data-arg="${esc(g.exe)}" data-name="${esc(g.name)}">Cerrarlo ahora</button></div></div>`;
    }).join("");
    return `${t.length ? `<div class="fix-list">${list}</div>` : `<div class="allgood">${IC.ok}<div><b>Nadie te ha sacado del juego</b> en los últimos 60 días. Si te pasa, aquí saldrá qué programa fue.</div></div>`}
      <div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.info}</div><h2>Para que no te molesten las notificaciones</h2></div>
        <p class="sub" style="margin:0">En Windows 11: Configuración &gt; Sistema &gt; Notificaciones &gt; <b>Activar «No molestar» automáticamente</b> &gt; marca «Al jugar a un juego».</p></div>
      ${t.length ? `<div class="foot"><button class="link" data-act="clearthefts">Borrar la lista</button></div>` : ""}`;
  },
};

function thiefHint(exe) {
  const h = {
    "ravcpl64.exe": " Es el panel de audio de Realtek: suele saltar al conectar o desconectar unos cascos.",
    "steam.exe": " Steam se pone delante al terminar una descarga o al mostrar ofertas: desactiva esos avisos en Steam > Configuración > Notificaciones.",
    "steamwebhelper.exe": " Es Steam (sus ventanas de avisos u ofertas).",
    "discord.exe": " Discord se pone delante con algunas llamadas: revisa sus avisos.",
    "msedge.exe": " Edge se abre solo a veces tras actualizarse.",
    "onedrive.exe": " OneDrive avisa de sincronización: puedes pausarlo mientras juegas.",
    "consent.exe": " Es la ventana de permiso de administrador de Windows: algún programa pidió permiso.",
  }[exe] || "";
  return h ? esc(h) : "";
}

fixPanels.window = {
  title: "Ratón y ventana del juego",
  sub: "Que el ratón no se escape a la otra pantalla y forzar «ventana sin bordes» en juegos que no la tienen.",
  load: async () => { await loadProfiles(); if (!state.sessions) await loadSessions(); render(); },
  status: () => {
    const p = state.profiles || {};
    const n1 = Object.values(p).filter(x => x.lockCursor).length, n2 = Object.values(p).filter(x => x.borderless).length;
    if (!n1 && !n2) return { level: "idle", text: "Sin activar en ningún juego" };
    const parts = [];
    if (n1) parts.push(`ratón encerrado en ${n1}`);
    if (n2) parts.push(`sin bordes en ${n2}`);
    return { level: "ok", text: parts.join(" · ") };
  },
  render: () => {
    const games = gamesKnown();
    const p = state.profiles || {};
    const multi = state.comps ? (state.comps.monitors || []).length > 1 : true;
    const rows = games.map(g => {
      const pr = p[g.exe] || {};
      return `<div class="opt"><div class="t"><b>${esc(g.name)}</b><p>${esc(g.exe)}</p></div>
        <span class="optlbl">Ratón encerrado</span><button class="sw sm ${pr.lockCursor ? "on" : ""}" data-act="prof" data-arg="${esc(g.exe)}" data-key="lockCursor" data-name="${esc(g.name)}"></button>
        <span class="optlbl">Sin bordes</span><button class="sw sm ${pr.borderless ? "on" : ""}" data-act="prof" data-arg="${esc(g.exe)}" data-key="borderless" data-name="${esc(g.name)}"></button></div>`;
    }).join("");
    return `<div class="grid g2">
        <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#f472b6,#8b5cf6)">${PIC.mouse}</div><h2>Ratón encerrado</h2></div>
          <p class="sub" style="margin:0">Mientras el juego está delante, el ratón no puede salir de su ventana. Si cambias de ventana (Alt+Tab), se suelta solo.${multi ? "" : " (Con una sola pantalla casi no hace falta.)"}</p></div>
        <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#22d3ee,#6366f1)">${PIC.screen}</div><h2>Ventana sin bordes</h2></div>
          <p class="sub" style="margin:0">Para juegos que solo tienen «ventana» o «pantalla completa»: pon el juego en ventana y TurboMON le quita el marco y lo estira a toda la pantalla. Así el overlay se ve y el Alt+Tab es instantáneo.</p></div></div>
      <div class="card mt"><div class="card-h"><div class="ic">${IC.game}</div><h2>Tus juegos</h2></div>
        ${games.length ? rows : `<div class="empty">Aquí saldrán los juegos en cuanto juegues a alguno con TurboMON abierto.</div>`}</div>`;
  },
};

Object.assign(actions, {
  nostartup: async exe => {
    try { await App.DisableStartup(exe); toast("Ya no arrancará con Windows"); } catch (e) { toast(errMsg(e), true); }
    fixPanels.focus.load();
  },
  clearthefts: async () => { try { await App.ClearThefts(); } catch (e) { } fixPanels.focus.load(); },
  prof: async (exe, el) => {
    const key = el.dataset.key;
    const p = Object.assign({ name: el.dataset.name }, (state.profiles || {})[exe] || {});
    p[key] = !p[key];
    state.profiles = Object.assign({}, state.profiles, { [exe]: p });
    render();
    try { state.profiles = await App.SaveProfile(exe, p); } catch (e) { toast(errMsg(e), true); }
    render();
  },
});
