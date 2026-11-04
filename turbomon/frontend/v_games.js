"use strict";
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
// Las partidas juntas por juego (el más reciente primero).
function gameGroups() {
  const by = new Map();
  for (const s of state.sessions || []) {
    const k = (s.exe || s.game).toLowerCase();
    if (!by.has(k)) by.set(k, { exe: k, name: s.game, list: [], secs: 0 });
    const g = by.get(k);
    g.list.push(s);
    g.secs += s.secs;
  }
  return [...by.values()];
}

// driverChange: si desde que cambió el driver de la gráfica (o Windows) el juego va distinto.
function driverChange(list) {
  const asc = list.slice().reverse();
  for (const [key, label] of [["gpuDriver", "driver de la gráfica"], ["winBuild", "Windows"]]) {
    for (let i = asc.length - 1; i > 0; i--) {
      const a = asc[i - 1][key], b = asc[i][key];
      if (!a || !b || a === b) continue;
      const before = asc.slice(0, i).filter(x => x[key] === a).slice(-3);
      const after = asc.slice(i).filter(x => x[key] === b).slice(0, 3);
      const avg = v => v.reduce((t, x) => t + x.fpsAvg, 0) / v.length;
      const d = (avg(after) / avg(before) - 1) * 100;
      return { label, from: a, to: b, d, at: asc[i].start, n: before.length + after.length };
    }
  }
  return null;
}

function trendChart(list, chg) {
  const asc = list.slice(0, 24).reverse();
  if (asc.length < 2) return "";
  const max = Math.max(...asc.map(s => s.fpsAvg)) * 1.08 || 1;
  const cols = asc.map(s => `<button class="tb ${state.sel === s.id ? "on" : ""} ${chg && s.start === chg.at ? "chg" : ""}" data-act="opengame" data-arg="${esc(s.id)}" title="${esc(when(s.start))}: ${n(s.fpsAvg)} FPS (1 % bajo ${n(s.low1)})${s.gpuDriver ? " · driver " + esc(s.gpuDriver) : ""}">
      <i style="height:${(s.fpsAvg / max * 100).toFixed(1)}%"></i><em style="bottom:${(s.low1 / max * 100).toFixed(1)}%"></em></button>`).join("");
  return `<div class="trend"><div class="trend-bars">${cols}</div>
    <div class="legend"><span><i style="background:linear-gradient(90deg,#8b5cf6,#22d3ee)"></i>FPS de media por partida</span><span><i style="background:#fbbf24"></i>1 % bajo</span>${chg ? `<span><i style="background:rgba(251,191,36,.7)"></i>cambio de ${esc(chg.label)}</span>` : ""}</div></div>`;
}
function gameSettings(g) {
  const pr = (state.profiles || {})[g.exe] || {};
  const pf = state.prefetch && state.prefetch[g.exe];
  const sw = (key, on) => `<button class="sw sm ${on ? "on" : ""}" data-act="prof" data-arg="${esc(g.exe)}" data-key="${key}" data-name="${esc(g.name)}"></button>`;
  const labOn = [pr.cores && pr.cores !== "all" ? { cores: "pcores", pcores: "Solo núcleos de rendimiento", nosmt: "Un hilo por núcleo", nocore0: "Lejos del núcleo 0", ccd: "Solo el primer chip" }[pr.cores] || pr.cores : "", pr.priority === "high" ? "Prioridad alta" : "", pr.gpuHigh ? "Prioridad alta en la gráfica" : ""].filter(Boolean);
  let pre;
  if (!pf || !pf.disk) pre = `<div class="opt"><div class="t"><b>Precarga aprendida</b><p>Juega una vez con TurboMON abierto para saber en qué disco está.</p></div></div>`;
  else if (pf.disk === "nvme") pre = `<div class="opt"><div class="t"><b>Precarga aprendida</b><p>Está en un NVMe: carga tan rápido que la precarga no se notaría. No hace falta.</p></div><span class="chip ok">no hace falta</span></div>`;
  else pre = `<div class="opt"><div class="t"><b>Precarga aprendida</b><p>Está en un ${pf.disk === "hdd" ? "disco duro" : "SSD SATA"}. TurboMON aprende qué lee el juego al cargar y, al abrirlo, se lo adelanta con la prioridad más baja.
      ${pf.learned ? `Aprendido: ${n(pf.learned / GB, 1)} GB en ${n(pf.files)} archivos (${pf.runs} ${pf.runs === 1 ? "vez" : "veces"}).` : pf.learning ? "Aprendiendo ahora…" : "Aún no ha aprendido nada: juega una vez."}
      ${pf.lastWarm ? ` La última vez adelantó ${n(pf.lastWarm.bytes / GB, 1)} GB en ${n(pf.lastWarm.secs)} s.` : ""}</p></div>${sw("prefetch", pr.prefetch)}</div>`;
  return `<div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.wrench}</div><h2>Ajustes de este juego</h2><span class="meta">se ponen solos al abrirlo</span></div>
    <div class="opt"><div class="t"><b>Ratón encerrado</b><p>El ratón no se escapa a la otra pantalla mientras juegas.</p></div>${sw("lockCursor", pr.lockCursor)}</div>
    <div class="opt"><div class="t"><b>Ventana sin bordes</b><p>Pon el juego en ventana y TurboMON lo estira a toda la pantalla sin marco.</p></div>${sw("borderless", pr.borderless)}</div>
    ${pre}
    ${labOn.length ? `<div class="opt"><div class="t"><b>Del laboratorio</b><p>Ajustes que demostraron mejorar este juego.</p></div><div class="chips">${labOn.map(t => `<span class="chip ok">${esc(t)}</span>`).join("")}</div></div>` : ""}</div>`;
}

function sessionDetail(sel) {
  const p = prevOf(sel);
  const st = sel.stutter;
  const extra = [];
  if (st && st.secs >= 60) extra.push(`<span class="chip ${st.perMin >= 2 ? "warnc" : st.perMin < 0.5 ? "ok" : ""}">${n(st.perMin, 1)} tirones/min</span>`);
  if (sel.ping && sel.ping.avg) extra.push(`<span class="chip">ping ${n(sel.ping.avg)} ms</span>`);
  if (sel.gov && sel.gov.capSecs) extra.push(`<span class="chip ok">ahorro: ${n(sel.gov.mhzOn / 1000, 1)} GHz</span>`);
  if (sel.throttle >= 10) extra.push(`<span class="chip warnc">procesador frenado el ${n(sel.throttle)} %</span>`);
  if (sel.onIGPU) extra.push(`<span class="chip warnc">con la gráfica integrada</span>`);
  return `<div class="card mt"><div class="card-h"><div class="ic">${IC.game}</div><h2>Partida del ${when(sel.start)}</h2><span class="meta">${dur(sel.secs)}${sel.gpuDriver ? " · driver " + esc(sel.gpuDriver) : ""}</span>
      <button class="btn sm ghost" data-act="delgame" data-arg="${esc(sel.id)}" title="Borrar esta partida">${IC.trash}</button></div>
    ${extra.length ? `<div class="chips" style="margin-bottom:12px">${extra.join("")}</div>` : ""}
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
    <div class="verdict">${IC.info}<div>${verdict(sel)}${sel.turbo ? ` Con el modo juego Turbo${sel.purges ? ` (soltó la memoria en espera ${sel.purges} ${sel.purges === 1 ? "vez" : "veces"})` : ""}.` : ""}
      ${st && (st.suspects || st.period) ? ` <button class="link" data-act="gofix" data-arg="stutter">Ver los tirones</button>` : ""}</div></div></div>`;
}

function renderGames() {
  const list = state.sessions;
  if (!list) return `<div class="empty">Cargando partidas…</div>`;
  if (!list.length) return `<div class="head"><div><h1>Juegos</h1><p class="sub">Aquí aparecerán solos cuando juegues.</p></div></div>
    <div class="card"><div class="empty">TurboMON se da cuenta de cuándo tienes un juego delante y apunta cómo va: <b>FPS medios, 1 % bajo, tirones, CPU, gráfica, RAM y temperaturas</b>, con una gráfica de toda la partida. Cada una ocupa unos pocos KB.</div></div>`;
  const groups = gameGroups();
  const g = groups.find(x => x.exe === state.gsel) || groups[0];
  const items = groups.map(x => {
    const fps = x.list.slice(0, 10).reverse().map(s => s.fpsAvg);
    return `<button class="gitem ${x === g ? "on" : ""}" data-act="opengroup" data-arg="${esc(x.exe)}" data-key="${esc(x.exe)}">
      <div class="n"><b>${esc(x.name)}</b><span class="${fpsCls(x.list[0].fpsAvg)}">${n(x.list[0].fpsAvg)}<small> FPS</small></span></div>
      <div class="d"><span>${x.list.length} ${x.list.length === 1 ? "partida" : "partidas"} · ${dur(x.secs)}</span><span>${when(x.list[0].start)}</span></div>
      ${fps.length > 1 ? `<div class="gspark">${spark(fps, Math.max(...fps) * 1.1, "url(#sg)", 22)}</div>` : ""}</button>`;
  }).join("");
  const chg = driverChange(g.list);
  let chgNote = "";
  if (chg && chg.n >= 3 && Math.abs(chg.d) >= 5) {
    chgNote = chg.d < 0
      ? `<div class="verdict warnv">${IC.warn}<div><b>Desde que cambió el ${esc(chg.label)} (${esc(chg.from)} → ${esc(chg.to)}) vas un ${n(-chg.d)} % peor en ${esc(g.name)}.</b> Si no cambiaste la calidad gráfica, prueba a volver a la versión anterior o espera a la siguiente.</div></div>`
      : `<div class="allgood">${IC.ok}<div><b>Desde el ${esc(chg.label)} ${esc(chg.to)} vas un ${n(chg.d)} % mejor en ${esc(g.name)}.</b></div></div>`;
  }
  const sel = state.selData && g.list.some(s => s.id === state.selData.id) ? state.selData : null;
  const rows = g.list.slice(0, 30).map(s => `<button class="srow ${state.sel === s.id ? "on" : ""}" data-act="opengame" data-arg="${esc(s.id)}" data-key="${esc(s.id)}">
      <span class="d">${when(s.start)}</span><span>${dur(s.secs)}</span><b class="${fpsCls(s.fpsAvg)}">${n(s.fpsAvg)} <small>FPS</small></b><span>1 % ${n(s.low1)}</span>
      <span class="${s.stutter && s.stutter.perMin >= 2 ? "hot" : ""}">${s.stutter && s.stutter.secs >= 60 ? n(s.stutter.perMin, 1) + " tir./min" : ""}</span>${s.limit ? `<span class="limit ${s.limit}">${limitTxt[s.limit]}</span>` : "<span></span>"}</button>`).join("");
  const avgF = g.list.reduce((t, s) => t + s.fpsAvg * s.secs, 0) / Math.max(1, g.secs);
  return `<div class="head"><div><h1>Juegos</h1><p class="sub">${groups.length} ${groups.length === 1 ? "juego" : "juegos"} · ${list.length} partidas · se apuntan solas cuando juegas.</p></div><span class="grow"></span>
      <button class="btn sm ghost" data-act="cleargames">${IC.trash}Borrar todas</button></div>
    <div class="games"><div class="glist">${items}</div><div>
      <div class="card"><div class="card-h"><div class="ic">${IC.game}</div><h2>${esc(g.name)}</h2><span class="meta">${dur(g.secs)} en ${g.list.length} ${g.list.length === 1 ? "partida" : "partidas"} · ${n(avgF)} FPS de media</span></div>
        ${trendChart(g.list, chg && Math.abs(chg.d) >= 5 ? chg : null)}${chgNote}
        <div class="srows">${rows}</div></div>
      ${gameSettings(g)}
      ${sel ? sessionDetail(sel) : ""}</div></div>`;
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

async function openGame(id) {
  state.sel = id;
  render();
  try { state.selData = await App.Session(id); } catch (e) { state.selData = null; }
  render();
}

views.games = {
  render: renderGames,
  after: drawGameChart,
  enter: async () => {
    await loadSessions();
    loadProfiles();
    App.Prefetch().then(r => { state.prefetch = r; render(); }).catch(() => { });
    const groups = gameGroups();
    if (!groups.length) return;
    const g = groups.find(x => x.exe === state.gsel) || groups[0];
    state.gsel = g.exe;
    if (!g.list.find(s => s.id === state.sel)) openGame(g.list[0].id);
  },
};

Object.assign(actions, {
  opengame: id => {
    const s = (state.sessions || []).find(x => x.id === id);
    if (s) state.gsel = (s.exe || s.game).toLowerCase();
    if (state.view !== "games") { state.sel = id; goView("games"); }
    openGame(id);
  },
  opengroup: exe => {
    state.gsel = exe;
    const g = gameGroups().find(x => x.exe === exe);
    if (g) openGame(g.list[0].id); else render();
  },
  gofix: id => { state.sub.fix = id; goView("fix"); },
  delgame: async id => {
    if (!await confirmBox("¿Borrar esta partida?", "Se borra del historial. No se puede deshacer.", "Borrar", true)) return;
    try { await App.DeleteSession(id); } catch (e) { toast(errMsg(e), true); }
    state.sel = null; state.selData = null; views.games.enter();
  },
  cleargames: async () => {
    if (!await confirmBox("¿Borrar todas las partidas?", "Se borra todo el historial de partidas. No se puede deshacer.", "Borrar todas", true)) return;
    try { await App.ClearSessions(); } catch (e) { toast(errMsg(e), true); }
    state.sel = null; state.selData = null; loadSessions();
  },
});
