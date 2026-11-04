"use strict";
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
    ${renderLab()}
    <div class="grid g2 mt">
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
    ${renderGovernor()}
    <div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.list}</div><h2>Lo que más gasta ahora</h2>
        <button class="chip ${state.procSort === "mem" ? "on" : ""}" data-act="psort" data-arg="mem">Por memoria</button><button class="chip ${state.procSort === "cpu" ? "on" : ""}" data-act="psort" data-arg="cpu">Por procesador</button></div>
      ${state.procs ? `<table class="procs"><thead><tr><th>Programa</th><th class="r">Procesador</th><th class="r">Memoria</th><th></th></tr></thead><tbody>${rows}</tbody></table>` : `<div class="empty">Mirando…</div>`}
      <div class="note">Cierra lo que no uses antes de jugar (sobre todo el navegador con muchas pestañas). Lo que es parte de Windows no se puede cerrar desde aquí.</div></div>`;
}

async function loadProcs() {
  if (state.view !== "opt") return;
  try { state.procs = await App.Processes(); } catch (e) { }
  render();
  clearTimeout(loadProcs.t);
  loadProcs.t = setTimeout(loadProcs, 2500);
}

// ---------- laboratorio ----------
const LIC = '<svg viewBox="0 0 24 24"><path d="M9 3h6M10 3v6L4.5 18.5A2 2 0 0 0 6.2 21.5h11.6a2 2 0 0 0 1.7-3L14 9V3"/><path d="M7 15h10"/></svg>';
const verdictTxt = {
  better: ["Mejora", "good"], worse: ["Empeora", "vhot"], mixed: ["Depende", "hot"], same: ["Da igual", ""], unclear: ["No está claro", ""],
};

async function loadLab() {
  if (state.view !== "opt") return;
  try { state.lab = await App.Lab(); } catch (e) { }
  render();
  clearTimeout(loadLab.t);
  loadLab.t = setTimeout(loadLab, state.lab && (state.lab.run || state.lab.pending) ? 2000 : 10000);
}

function pct(v) { return (v > 0 ? "+" : "") + n(v, 1) + " %"; }

// Cada punto es una pareja de bloques: a la derecha del cero, el ajuste fue mejor en esa pareja.
function labDots(diffs) {
  if (!diffs || !diffs.length) return "";
  const m = Math.max(5, ...diffs.map(Math.abs));
  const dots = diffs.map(d => `<i class="${d > 0 ? "up" : d < 0 ? "dn" : ""}" style="left:${(50 + d / m * 46).toFixed(1)}%"></i>`).join("");
  return `<div class="lab-dots" title="Cada punto es una pareja de bloques: a la derecha, el ajuste fue mejor"><span class="zero"></span>${dots}<em class="l">peor</em><em class="r">mejor</em></div>`;
}

function labResult(r) {
  const res = r.result;
  const [vt, vc] = verdictTxt[res.verdict] || ["", ""];
  const main = res.verdict === "better" || res.verdict === "worse" || res.verdict === "mixed"
    ? (Math.abs(res.fps.delta) >= Math.abs(res.low1.delta) || res.low1.p >= 0.02 ? `${pct(res.fps.delta)} FPS` : `${pct(res.low1.delta)} en el 1 % bajo`)
    : res.verdict === "same" ? "la diferencia es ruido" : `solo ${res.pairs} parejas`;
  const canApply = res.verdict === "better";
  return `<div class="lab-res v-${res.verdict}"><div class="lab-v"><b class="${vc}">${vt}</b><span>${esc(main)}</span></div>
    <div class="t"><b>${esc(r.title)}</b><span>${esc(r.game)} · ${when(r.start)}${r.partial ? " · incompleto" : ""}</span>
      <div class="lab-nums"><span>FPS <b>${n(res.fps.a)}</b> → <b>${n(res.fps.b)}</b></span><span>1 % bajo <b>${n(res.low1.a)}</b> → <b>${n(res.low1.b)}</b></span><span>tirones/min <b>${n(res.stutter.a, 1)}</b> → <b>${n(res.stutter.b, 1)}</b></span></div>
      ${labDots(res.diffs)}</div>
    <div class="lab-act">${canApply || r.applied ? `<button class="btn sm ${r.applied ? "ghost" : "primary"}" data-act="labapply" data-arg="${esc(r.id)}" data-on="${r.applied ? 0 : 1}">${r.applied ? "Quitar del juego" : "Ponerlo siempre"}</button>` : ""}
      <button class="link" data-act="labdel" data-arg="${esc(r.id)}">Borrar</button></div></div>`;
}

function renderLab() {
  const L = state.lab;
  if (!L) return `<div class="card lab"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#0ea5e9)">${LIC}</div><h2>Laboratorio</h2></div><div class="empty">Cargando…</div></div>`;
  const game = state.live && state.live.session && state.live.session.game;
  let body;
  if (L.run) {
    const r = L.run;
    body = `<div class="lab-run"><div class="t"><b>Probando «${esc(r.title)}» en ${esc(r.game)}</b>
        <span>${r.paused ? "En pausa: vuelve al juego para seguir." : `Bloque ${r.block} de ${r.blocks} · faltan unos ${Math.max(1, Math.round(r.left / 60))} min de juego`}</span></div>
        <button class="btn ghost sm" data-act="labstop">Parar</button></div>${pctBar(r.pct)}
      <p class="note">Juega normal. No te decimos qué bloque lleva el ajuste: así ni tú ni nadie se deja llevar por la sensación.</p>`;
  } else if (L.pending) {
    body = `<div class="lab-run"><div class="t"><b>«${esc(L.pending.title)}» empezará con tu próxima partida</b><span>En cuanto TurboMON vea un juego delante, empieza solo.</span></div>
        <button class="btn ghost sm" data-act="labstop">Cancelar</button></div>`;
  } else {
    const ns = state.noise;
    const core0Bad = ns && (ns.cores || []).some(c => c.id <= 1 && c.level === "bad");
    body = `<div class="lab-knobs">${(L.knobs || []).map(k => `<button class="lab-k" data-act="labstart" data-arg="${esc(k.id)}" data-title="${esc(k.title)}">
        <b>${esc(k.title)}</b><span>${esc(k.desc)}</span>${k.id === "nocore0" && core0Bad ? `<em class="chip warnc">Tu mapa de ruido lo recomienda</em>` : ""}</button>`).join("")}</div>`;
  }
  const hist = (L.history || []).slice(0, 6);
  return `<div class="card lab"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#0ea5e9)">${LIC}</div><h2>Laboratorio</h2><span class="chip ok">a ciegas</span></div>
      <p class="sub" style="margin:0 0 14px">¿Ese ajuste de los foros te da FPS de verdad? TurboMON lo enciende y lo apaga al azar cada 20 segundos mientras juegas, sin decirte cuándo, y compara.
      En unos 11 minutos de juego sabes si mejora, empeora o da igual.${game && !L.run && !L.pending ? ` Ahora mismo empezaría en <b>${esc(game)}</b>.` : ""}</p>
      ${body}
      ${hist.length ? `<div class="lab-hist"><div class="lab-hh">Resultados</div>${hist.map(labResult).join("")}</div>` : ""}</div>`;
}

// ---------- ahorro inteligente ----------
function renderGovernor() {
  const s = state.settings, l = state.live;
  const elev = l.agent.elevated;
  const last = (state.sessions || []).find(x => x.gov);
  let res = "";
  if (last) {
    const g = last.gov;
    if (g.unsupported) res = `<div class="verdict">${IC.info}<div>En tu última partida (${esc(last.game)}) Windows no hizo caso al tope de frecuencia en este procesador: el ahorro no hace nada aquí.</div></div>`;
    else if (!g.capSecs) res = `<div class="verdict">${IC.info}<div>En tu última partida (${esc(last.game)}) el juego necesitó todo el procesador: no había nada que ahorrar.</div></div>`;
    else {
      const pctT = Math.round(g.capSecs / g.secs * 100);
      const w = g.wattsOn && g.wattsOff ? g.wattsOff - g.wattsOn : 0;
      const tt = g.tempOn && g.tempOff ? g.tempOff - g.tempOn : 0;
      res = `<div class="gov-res"><div class="gov-big"><b>${n(g.mhzOn / 1000, 1)}<small>GHz</small></b><span>en vez de ${n((g.mhzOff || g.maxMHz) / 1000, 1)} GHz</span></div>
        <div class="t"><b>${esc(last.game)}: el ${pctT} % de la partida con el procesador más relajado y sin perder FPS</b>
        <span>${w > 1 ? `Unos ${n(w)} W menos de consumo` : "Menos consumo y calor"}${tt > 1 ? ` y ${n(tt)} °C menos` : ""}${!g.wattsOn ? " (abre HWiNFO para ver los vatios y grados exactos)" : ""}.</span></div></div>`;
    }
  }
  return `<div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#10b981);--tile-sh:rgba(16,185,129,.6)">${IC.bolt}</div><h2>Ahorro inteligente</h2><span class="chip">experimental</span>
      ${elev ? `<button class="sw sm ${s.governor ? "on" : ""}" data-act="flag" data-arg="governor"></button>` : ""}</div>
    <p class="sub" style="margin:0">Si tus FPS van limitados (sincronización vertical, un tope del juego) o la gráfica va al 100 %, el procesador está corriendo para nada. TurboMON le baja la frecuencia poco a poco mientras los fotogramas sigan saliendo a tiempo, y la sube al instante si hace falta. Menos calor, menos ruido y, en portátiles, más batería, con los mismos FPS.
    Usa una copia de tu plan de energía que se borra al acabar la partida.</p>
    ${elev ? "" : `<div class="btns-inline"><button class="btn sm amber" data-act="enablefps">${IC.shield}Dar permiso para usarlo</button></div>`}
    ${res}</div>`;
}

views.opt = { render: renderTools, enter: () => { loadProcs(); loadLab(); if (!state.sessions) loadSessions(); } };

Object.assign(actions, {
  purge: async () => {
    state.busy.purge = true; render();
    try {
      const r = await App.Purge();
      if (r && r.ok) toast(`Liberados ${gb(r.freed)} GB de memoria en espera`); else toast((r && r.error) || "No se pudo", true);
    } catch (e) { toast(errMsg(e), true); }
    delete state.busy.purge; render();
  },
  psort: a => { state.procSort = a; render(); },
  labstart: async (knob, el) => {
    const game = state.live && state.live.session && state.live.session.game;
    if (!await confirmBox(`¿Probar «${el.dataset.title}»?`, game ? `Empieza ya en ${game}. Sigue jugando normal unos 11 minutos: al acabar te avisa y lo deja todo como estaba.` : "Empezará en cuanto abras un juego. Juega normal unos 11 minutos: al acabar te avisa y lo deja todo como estaba.", "Probar")) return;
    try { state.lab = await App.LabAction({ action: "start", knob }); } catch (e) { return toast(errMsg(e), true); }
    loadLab();
  },
  labstop: async () => { try { state.lab = await App.LabAction({ action: "stop" }); } catch (e) { } loadLab(); },
  labapply: async (id, el) => {
    try { state.lab = await App.LabAction({ action: "apply", id, on: el.dataset.on === "1" }); toast(el.dataset.on === "1" ? "Puesto: se aplicará cada vez que abras ese juego" : "Quitado"); } catch (e) { return toast(errMsg(e), true); }
    loadLab();
  },
  labdel: async id => { try { state.lab = await App.LabAction({ action: "delete", id }); } catch (e) { } loadLab(); },
  kill: async (exe, el) => {
    const name = el.dataset.name || exe;
    if (!await confirmBox(`¿Cerrar ${name}?`, "Se cierran todos sus procesos. Si tenía algo sin guardar, se perderá.", "Cerrar", true)) return;
    try { const k = await App.KillGroup(exe); toast(`${name} cerrado (${k} proceso${k === 1 ? "" : "s"})`); } catch (e) { toast(errMsg(e), true); }
    loadProcs();
  },
});
