"use strict";
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

views.settings = { render: renderSettings, enter: () => loadSessions() };
