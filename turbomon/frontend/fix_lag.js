"use strict";
// ---------- «Me va con lag» ----------
const gradeTxt = {
  "A+": ["Perfecta", "El ping casi no sube aunque descargues: tu conexión aguanta lo que le eches."],
  "A": ["Muy buena", "El ping sube un poco al descargar, pero no lo notarás jugando."],
  "B": ["Buena", "Al descargar a tope el ping sube algo. Si juegas online, evita descargas grandes mientras juegas."],
  "C": ["Regular", "Cuando algo descarga, el ping sube bastante: es «bufferbloat». Una descarga de Steam o una actualización te arruina la partida."],
  "D": ["Mala", "El ping se dispara con cualquier descarga o subida. Mira las soluciones de abajo."],
  "F": ["Muy mala", "Con la conexión cargada el ping se vuelve injugable. Mira las soluciones de abajo."],
};
const gradeCls = g => ({ "A+": "good", "A": "good", "B": "good", "C": "hot", "D": "vhot", "F": "vhot" })[g] || "";

fixPanels.lag = {
  title: "Me va con lag",
  sub: "El ping de verdad, qué programa usa internet mientras juegas y si tu conexión aguanta descargas.",
  load: async open => {
    if (open || state.sub.fix === "lag") {
      try { state.net = await App.Net(); } catch (e) { }
      if (!state.comps) loadComps();
      render();
      clearTimeout(fixPanels.lag.t);
      if (state.view === "fix" && state.sub.fix === "lag") fixPanels.lag.t = setTimeout(() => fixPanels.lag.load(true), 1000);
    }
  },
  status: () => {
    const b = state.bloat;
    if (b && b.grade && ["C", "D", "F"].includes(b.grade)) return { level: "warn", text: `Tu conexión: nota ${b.grade} con descargas` };
    const nt = state.net;
    if (nt && nt.inetPing && nt.inetPing.loss > 3) return { level: "warn", text: `Se pierden paquetes (${n(nt.inetPing.loss, 1)} %)` };
    if (b && b.grade) return { level: "ok", text: `Conexión con nota ${b.grade}` };
    return { level: "idle", text: "Mide tu conexión y mira qué la usa" };
  },
  render: () => {
    const nt = state.net;
    const c = state.comps;
    const def = c && (c.network || []).find(a => a.default);
    const pingTile = (title, st, sub, extra = "") => {
      const has = st && st.count > 0 && st.avg > 0;
      const live = st && st.last ? st.last : 0;
      return `<div class="card big ${has ? "" : "dim"}"><div class="lbl">${FIC.lag}${title}<span class="sub2">${esc(sub || "")}</span></div>
        <div class="v ${live >= 100 ? "vhot" : live >= 60 ? "hot" : ""}">${has ? n(live || st.avg) : "—"}<small>ms</small></div>
        <div class="row">${has ? `<span>media <b>${n(st.avg)}</b></span><span>saltos <b>${n(st.jitter, 1)}</b> ms</span><span>perdidos <b class="${st.loss > 1 ? "vhot" : ""}">${n(st.loss, 1)} %</b></span>` : extra}</div>
        <div class="spark">${spark((st && st.hist || []).filter(v => v >= 0).slice(-60))}</div></div>`;
    };
    let tiles = "";
    if (nt && !nt.elevated) {
      tiles = `<div class="verdict warnv">${IC.warn}<div>Para ver qué programa usa internet y el ping al servidor del juego, TurboMON necesita el mismo permiso que los FPS. <div class="btns-inline"><button class="btn sm amber" data-act="enablefps">Dar permiso</button></div></div></div>`;
    } else if (nt) {
      const srv = nt.server;
      const gameSub = srv ? srv.addr : (nt.game ? "buscando el servidor…" : "abre un juego online");
      tiles = `<div class="grid g2">${pingTile("Servidor del juego", srv && !srv.noPing ? nt.gamePing : null, gameSub, srv && srv.noPing ? "Este servidor no contesta al ping (muchos no lo hacen): mira el de internet." : (nt.game ? "Esperando…" : "Cuando juegues online saldrá aquí"))}
        ${pingTile("Internet", nt.inetPing, "8.8.8.8", "Midiendo…")}</div>`;
    }
    const procs = (nt && nt.procs) || [];
    const maxR = Math.max(1, ...procs.map(p => p.down + p.up));
    const procList = procs.length ? procs.map(p => `<div class="netp ${p.game ? "game" : p.down + p.up > 1 << 20 ? "hog" : ""}"><b>${esc(p.name)}${p.game ? ' <span class="chip ok">el juego</span>' : ""}</b>
        <div class="netbar"><i style="width:${((p.down + p.up) / maxR * 100).toFixed(1)}%"></i></div><span class="num">↓ ${rate(p.down)} · ↑ ${rate(p.up)}</span></div>`).join("")
      : `<div class="empty">${nt && nt.active ? "Ningún programa está usando la red ahora mismo." : "Abriendo…"}</div>`;
    const hog = procs.find(p => !p.game && p.down + p.up > 1 << 20);
    const b = state.bloat;
    let bloat;
    if (b && b.running) {
      bloat = `<div class="bloat-run"><div class="bloat-live"><b>${b.live ? n(b.live) : "—"}</b><span>ms ahora</span></div><div class="t"><b>${{ idle: "Midiendo el ping en reposo…", down: "Descargando a tope…", up: "Subiendo a tope…" }[b.phase] || "Midiendo…"}</b>${pctBar(b.pct)}</div></div>`;
    } else if (b && b.grade) {
      const [gt, gd] = gradeTxt[b.grade] || ["", ""];
      bloat = `<div class="bloat-res"><div class="grade ${gradeCls(b.grade)}">${esc(b.grade)}</div><div class="t"><b>${gt}</b><p>${gd}</p>
          <div class="bl-nums"><div><b>${n(b.idle)}</b><span>ms en reposo</span></div><div><b class="${b.down - b.idle > 60 ? "vhot" : ""}">${n(b.down)}</b><span>ms descargando (${n(b.downMbps)} Mbps)</span></div><div><b class="${b.up - b.idle > 60 ? "vhot" : ""}">${n(b.up)}</b><span>ms subiendo (${n(b.upMbps)} Mbps)</span></div></div></div></div>
        ${["C", "D", "F"].includes(b.grade) ? `<div class="note"><b>Cómo se arregla:</b> en la configuración del router busca «QoS», «SQM» o «Gestión de ancho de banda» y actívalo (pon la velocidad un 10 % por debajo de la que te da tu operador). Si no lo tiene, limita las descargas de Steam (Configuración &gt; Descargas &gt; Limitar el ancho de banda) mientras juegas.</div>` : ""}`;
    } else {
      bloat = `<p class="sub" style="margin:0 0 12px">Mide el ping en reposo, descargando a tope y subiendo a tope (unos 25 segundos). Si sube mucho, cualquier descarga de fondo te mete lag. Usa unos cientos de MB de datos.</p>`;
    }
    if (b && b.error) bloat += `<div class="note hot">${esc(b.error)}</div>`;
    const wifi = def && def.kind === "wifi" ? `<div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#38bdf8,#0ea5e9)">${PIC.wifi}</div><h2>Juegas por wifi</h2><span class="meta">${esc([def.wifiStd, def.wifiBand, def.wifiSignal ? "señal " + def.wifiSignal + " %" : ""].filter(Boolean).join(" · "))}</span></div>
        <div class="opt"><div class="t"><b>Apagar la búsqueda de redes mientras juegas</b><p>Windows busca redes wifi cada minuto, y cada búsqueda mete un salto de ping de 100-300 ms. TurboMON la apaga solo durante la partida y la vuelve a encender al acabar.</p></div>
          <button class="sw sm ${state.settings && state.settings.wifiNoScan ? "on" : ""}" data-act="flag" data-arg="wifiNoScan"></button></div>
        ${nt && nt.wifiOff ? `<div class="chips">${chipOk("Apagada ahora (estás jugando)")}</div>` : ""}</div>` : "";
    return `${tiles}
      <div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#0891b2)">${IC.net}</div><h2>Qué usa internet ahora</h2></div>
        <div class="netps">${procList}</div>
        ${hog ? `<div class="part-note warn">${IC.warn}<span><b>${esc(hog.name)}</b> está moviendo ${rate(hog.down + hog.up)}: si juegas online, eso te sube el ping. Ciérralo o páusalo mientras juegas.</span></div>` : ""}</div>
      <div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#fbbf24,#f97316);--tile-sh:rgba(245,158,11,.6)">${IC.fps}</div><h2>¿Aguanta tu conexión una descarga?</h2>
        ${b && b.running ? "" : `<button class="btn sm primary" data-act="bloat">${b && b.grade ? "Repetir" : "Hacer la prueba"}</button>`}</div>${bloat}</div>
      ${wifi}`;
  },
};

async function pollBloat() {
  try { state.bloat = await App.Bloat(); } catch (e) { }
  render();
  if (state.bloat && state.bloat.running) setTimeout(pollBloat, 300);
}

Object.assign(actions, {
  bloat: async () => {
    try { await App.StartBloat(); } catch (e) { return toast(errMsg(e), true); }
    pollBloat();
  },
});
