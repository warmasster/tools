"use strict";
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
    ${fixStrip()}
    ${tiles(l)}
    <div class="grid g2 mt">
      <div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#fbbf24,#f97316);--tile-sh:rgba(245,158,11,.6)">${IC.wrench}</div><h2>Para mejorar</h2></div>${todoList(r, 4)}</div>
      ${gameCard(l)}
    </div>
    <div class="foot"><span>TurboMON gasta ahora <b>${n(a.cpu, 1)} % de CPU</b> y <b>${n(a.memMB, 0)} MB</b> de memoria</span><span>Historial de partidas: <b>${size(a.histBytes)}</b></span><span>Versión ${esc(a.version)}</span></div>`;
}

views.home = { render: renderHome, enter: () => { loadSessions(); warmFix(); } };
