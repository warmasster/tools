"use strict";
// ---------- «Me va a tirones» y «Tirones con dos pantallas» ----------

const stLevel = pm => pm == null ? "" : pm < 0.5 ? "good" : pm < 2 ? "hot" : "vhot";

// Consejo para cada culpable que suele salir.
const procAdvice = {
  "Antivirus de Windows": "Añade la carpeta del juego a las exclusiones (Seguridad de Windows > Protección antivirus > Administrar la configuración > Exclusiones).",
  "Windows Update": "Pausa las actualizaciones mientras juegas (Configuración > Windows Update > Pausar).",
  "Indexador de búsqueda de Windows": "Saca la carpeta de los juegos del indexador (Configuración > Privacidad y seguridad > Buscar en Windows).",
  "Steam (navegador interno)": "Cierra la ventana de la tienda de Steam y desactiva la superposición si no la usas.",
  "Steam": "Pausa las descargas de Steam mientras juegas.",
  "Discord": "Desactiva la aceleración por hardware de Discord (Ajustes > Avanzado) y la superposición del juego.",
  "Google Chrome": "Cierra las pestañas con vídeo o animaciones, o desactiva la aceleración por hardware del navegador.",
  "Microsoft Edge": "Cierra las pestañas con vídeo o animaciones, o desactiva la aceleración por hardware del navegador.",
  "Firefox": "Cierra las pestañas con vídeo o animaciones, o desactiva la aceleración por hardware del navegador.",
  "Opera GX": "Cierra las pestañas con vídeo o animaciones, o desactiva la aceleración por hardware del navegador.",
  "Wallpaper Engine": "Configúralo para que se pause con un juego en pantalla completa (Ajustes > Rendimiento).",
  "OneDrive": "Pausa la sincronización mientras juegas (icono de OneDrive > Pausar).",
  "Máquina virtual (WSL o Hyper-V)": "Cierra la máquina virtual o WSL mientras juegas («wsl --shutdown»).",
  "WSL (Linux en Windows)": "Cierra WSL mientras juegas («wsl --shutdown» en una terminal).",
  "Telemetría de Windows": "Es una tarea de Windows que se pasa un rato analizando: suele ir a ratos y desaparece sola.",
};
const rgbApps = ["iCUE (Corsair)", "G HUB (Logitech)", "NZXT CAM", "Armoury Crate (ASUS)", "Luces RGB de ASUS", "Razer Synapse", "SignalRGB", "OpenRGB", "MSI Afterburner", "RivaTuner"];

function suspectView(s) {
  let ic = IC.list, title = s.name, adv = "";
  switch (s.kind) {
    case "proc":
      ic = IC.cpu;
      adv = procAdvice[s.name] || (rgbApps.includes(s.name) ? "Los programas de luces y sensores leen el hardware muy a menudo y eso mete tirones: ciérralo mientras juegas o baja su frecuencia de lectura." : "Ciérralo mientras juegas y mira si se van los tirones.");
      break;
    case "disk": ic = IC.disk; title = "El juego cargando del disco"; adv = "Si el juego está en un disco duro, pásalo al SSD. Si ya está en SSD, son cargas del propio juego (texturas, zonas nuevas): suelen ir a menos tras la primera partida."; break;
    case "dpc": ic = IC.bolt; title = "Un driver ocupando el procesador"; adv = "Haz el mapa de ruido de abajo: te dice qué driver es."; break;
    case "limit": ic = IC.fps; title = "El procesador frenándose"; adv = "Se frena por temperatura o por potencia. Mira la temperatura en «Mi PC», limpia el polvo y revisa el plan de energía."; break;
    case "decode": ic = FIC.dual; title = "Un vídeo en otra ventana"; adv = "Mira «Tirones con dos pantallas» para quitarlo."; break;
  }
  return `<div class="susp"><div class="susp-ic">${ic}</div><div class="t"><div class="susp-h"><b>${esc(title)}</b><span class="chip warnc">${n(s.lift, 1)}× más tirones</span></div>
      <div class="susp-bar"><i style="width:${Math.min(100, s.share)}%"></i></div>
      <span class="susp-share">Estaba activo en el <b>${n(s.share)} %</b> de los tirones</span>
      <p>${esc(adv)}</p></div></div>`;
}

// Línea de tiempo de la partida: una raya por tirón. Si van con ritmo, las que lo siguen salen en ámbar.
function stutterTimeline(st) {
  const secs = Math.max(1, st.secs);
  const times = st.times || [];
  let phase = -1;
  if (st.period) {
    const h = new Array(st.period).fill(0);
    for (const t of times) for (const d of [-1, 0, 1]) h[(t + d + st.period) % st.period]++;
    phase = h.indexOf(Math.max(...h));
  }
  const fits = t => phase >= 0 && [0, 1, st.period - 1].includes(((t - phase) % st.period + st.period) % st.period);
  const ticks = times.map(t => `<line x1="${(t / secs * 1000).toFixed(1)}" x2="${(t / secs * 1000).toFixed(1)}" y1="6" y2="34" class="${fits(t) ? "fit" : ""}"/>`).join("");
  const marks = [];
  const step = secs > 3600 ? 1800 : secs > 1200 ? 600 : secs > 300 ? 120 : 60;
  for (let s = step; s < secs; s += step) marks.push(`<span style="left:${(s / secs * 100).toFixed(2)}%">${dur(s)}</span>`);
  return `<div class="tl"><svg viewBox="0 0 1000 40" preserveAspectRatio="none"><rect x="0" y="17" width="1000" height="6" rx="3" class="base"/>${ticks}</svg><div class="tl-marks">${marks.join("")}</div></div>`;
}

function stutterVerdict(s) {
  const st = s.stutter;
  if (st.secs < 60) return `<div class="verdict">${IC.info}<div>Todavía es pronto: en cuanto lleves un par de minutos jugando saldrá el análisis.</div></div>`;
  if (st.perMin < 0.5 && !st.suspects) return `<div class="allgood">${IC.ok}<div><b>Casi sin tirones.</b> ${st.hitches ? `${n(st.hitches)} en ${dur(st.secs)}: lo normal en cualquier juego.` : "Ni uno en toda la partida."}</div></div>`;
  let out = "";
  if (st.period) {
    out += `<div class="verdict warnv">${IC.warn}<div><b>Tus tirones van con reloj: cada ${n(st.period)} s</b> (el ${n(st.periodFit)} % cae justo en ese ritmo). Eso no lo hace el juego: es algo de fondo con un temporizador.
      ${st.suspects ? "Abajo tienes quién coincide." : "Ningún programa visible coincide: suele ser un driver (el wifi buscando redes, programas de luces o de sensores). El mapa de ruido de abajo lo busca."}</div></div>`;
  }
  if (st.suspects && st.suspects.length) {
    out += `<div class="susp-list">${st.suspects.map(suspectView).join("")}</div>`;
  } else if (!st.period && st.perMin >= 0.5) {
    out += `<div class="verdict">${IC.info}<div><b>Ningún programa de fondo coincide con los tirones.</b> Vienen del propio juego (preparando sombreadores, cargando zonas) o de un driver.
      Si pasan sobre todo al principio o al entrar en zonas nuevas, es el juego; si no, haz el mapa de ruido de abajo.</div></div>`;
  }
  return out;
}

function renderStutterSession(s) {
  const st = s.stutter;
  const lv = stLevel(st.perMin);
  return `<div class="card stut-card"><div class="stut-top">
      <div class="t"><b>${esc(s.game)}</b><span>${s.id ? when(s.start) + " · " : `<span class="chip ok live-dot">jugando ahora</span> `}${dur(st.secs || s.secs)} · ${n(s.fpsAvg)} FPS de media, 1 % bajo ${n(s.low1)}</span></div>
      <div class="stut-num ${lv}"><b>${n(st.perMin, 1)}</b><span>tirones por minuto</span></div>
      <div class="stut-num"><b>${st.worst ? n(st.worst) : "—"}<small>ms</small></b><span>el peor</span></div></div>
    ${st.secs >= 60 ? stutterTimeline(st) : ""}
    ${stutterVerdict(s)}</div>`;
}

function noiseMap(ns) {
  const cores = ns.cores || [];
  const classes = [...new Set(cores.map(c => c.class))].sort((a, b) => b - a);
  const hybrid = classes.length > 1;
  const groups = classes.map(cl => {
    const byPhys = {};
    for (const c of cores.filter(c => c.class === cl)) (byPhys[c.phys] = byPhys[c.phys] || []).push(c);
    const boxes = Object.values(byPhys).map(ths => `<div class="nm-core">${ths.map(c => {
      const cls = ns.running && ns.current === c.id ? "now" : c.done ? c.level : "";
      const tip = c.done ? `Hilo ${c.id}: pierde el ${n(c.stolen, 2)} % del tiempo; peor hueco ${n(c.maxUs)} µs; drivers ${n(c.dpc, 1)} %, interrupciones ${n(c.isr, 1)} %${c.worstDrv ? "; el peor, " + c.worstDrv : ""}` : `Hilo ${c.id}`;
      return `<div class="nm-t ${cls}" title="${esc(tip)}"><b>${c.id}</b><span>${c.done ? n(c.stolen, c.stolen < 1 ? 2 : 1) + " %" : ""}</span></div>`;
    }).join("")}</div>`).join("");
    return `<div class="nm-group">${hybrid ? `<div class="nm-lbl">${cl === classes[0] ? "Núcleos de rendimiento (P)" : "Núcleos de eficiencia (E)"}</div>` : ""}<div class="nm-cores">${boxes}</div></div>`;
  }).join("");
  return `<div class="nm">${groups}</div>`;
}

function noiseVerdict(ns) {
  const cores = (ns.cores || []).filter(c => c.done);
  if (!cores.length) return "";
  const worst = cores.slice().sort((a, b) => b.stolen - a.stolen)[0];
  const bad = cores.filter(c => c.level === "bad");
  let out = "";
  if (!bad.length && !cores.some(c => c.level === "warn")) {
    out = `<div class="allgood">${IC.ok}<div><b>Núcleos limpios.</b> Ningún núcleo pierde más del ${n(worst.stolen, 2)} % de su tiempo: los drivers no te están robando nada que se note.</div></div>`;
  } else {
    const first = worst.id <= 1;
    out = `<div class="verdict ${bad.length ? "badv" : "warnv"}">${IC.warn}<div><b>El hilo ${worst.id} pierde el ${n(worst.stolen, 1)} % de su tiempo</b> (drivers ${n(worst.dpc, 1)} %, interrupciones ${n(worst.isr, 1)} %; el peor hueco, ${worst.maxUs >= 1000 ? n(worst.maxUs / 1000, 1) + " ms" : n(worst.maxUs) + " µs"}).
      ${worst.stolen - worst.dpc - worst.isr > worst.stolen / 2 ? "La mayor parte se la llevan hilos de Windows con más prioridad que un juego (audio, red, el propio sistema)." : ""}
      ${first ? "Windows atiende casi todo lo suyo en el primer núcleo. Si el juego pone ahí un hilo importante, se nota en forma de tirones: el Laboratorio puede probar a sacarlo de ahí." : "Algo está usando ese núcleo a menudo."}
      ${ns.deep ? "Abajo tienes qué driver es." : ""}</div></div>`;
  }
  return out;
}

fixPanels.stutter = {
  title: "Me va a tirones",
  sub: "Cada tirón de tus partidas, a qué hora pasó y quién estaba haciendo algo justo entonces.",
  load: async open => {
    try { state.stutter = await App.Stutter(); } catch (e) { }
    try { const r = await App.Noise(); state.noise = r && r.state; } catch (e) { }
    render();
    const p = fixPanels.stutter;
    clearTimeout(p.t);
    if (state.view === "fix" && (state.sub.fix === "stutter" || state.sub.fix === "dual")) {
      p.t = setTimeout(() => p.load(true), state.noise && state.noise.running ? 600 : state.stutter && state.stutter.live ? 3000 : 15000);
    }
  },
  status: () => {
    const s = state.stutter;
    if (!s) return { level: "idle", text: "" };
    const last = s.live || s.sessions[0];
    if (!last) return { level: "idle", text: "Juega una partida y aquí saldrá el análisis" };
    const st = last.stutter;
    if (st.secs < 60) return { level: "idle", text: "Analizando la partida…" };
    if (st.suspects && st.suspects.length) return { level: "bad", text: `${st.suspects[0].name || "Algo"} coincide con tus tirones` };
    if (st.period) return { level: "bad", text: `Tirones cada ${st.period} s exactos` };
    if (st.perMin >= 2) return { level: "warn", text: `${n(st.perMin, 1)} tirones por minuto en ${last.game}` };
    return { level: "ok", text: `Pocos tirones en ${last.game}` };
  },
  render: () => {
    const s = state.stutter;
    if (!s) return `<div class="empty">Abriendo…</div>`;
    const list = (s.live ? [s.live] : []).concat(s.sessions || []);
    const sel = list.find(x => (x.id || "live") === state.stutSel) || list[0];
    let top;
    if (!sel) {
      top = `<div class="card"><div class="verdict">${IC.game}<div><b>Todavía no hay partidas.</b> Juega un rato: TurboMON apunta cada tirón y al acabar te dice si hay un culpable (otro programa, el disco, un driver…).${s.elevated ? "" : " Para ver los fotogramas hace falta el permiso de administrador."}</div></div>
        ${s.elevated ? "" : `<div class="btns-inline"><button class="btn sm amber" data-act="enablefps">Dar permiso</button></div>`}</div>`;
    } else {
      top = renderStutterSession(sel);
      if (list.length > 1) {
        top += `<div class="stut-pick">${list.slice(0, 8).map(x => {
          const id = x.id || "live";
          return `<button class="chip ${x === sel ? "on" : ""}" data-act="stutsel" data-arg="${esc(id)}">${esc(x.game)} <small class="${stLevel(x.stutter.perMin)}">${n(x.stutter.perMin, 1)}/min</small></button>`;
        }).join("")}</div>`;
      }
    }
    const ns = state.noise;
    const running = ns && ns.running;
    const drivers = ns && ns.drivers && ns.drivers.length ? `<div class="drv-list">${ns.drivers.filter(d => d.file !== "desconocido").slice(0, 6).map(d => `<div class="drv ${d.level}">
        <div class="t"><b>${esc(d.desc || d.file)}</b><span>${esc(d.file)} · ${n(d.count)} veces · ${n(d.totalMs, 1)} ms en total</span></div>
        <div class="drv-v"><b>${d.maxUs >= 1000 ? n(d.maxUs / 1000, 1) + "<small>ms</small>" : n(d.maxUs) + "<small>µs</small>"}</b><span>el peor</span></div></div>`).join("")}</div>
        <p class="note">Por debajo de 0,5 ms no se nota. Si un driver pasa de 1-2 ms a menudo, actualízalo (o el de la placa base, si es de audio, red o USB).</p>` : "";
    const noiseCard = `<div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.cpu}</div><h2>Mapa de ruido de tus núcleos</h2>
        ${running ? `<span class="meta">midiendo el hilo ${ns.current}…</span>` : `<button class="btn sm primary" data-act="noise">${ns ? "Repetir" : "Hacer el mapa"}</button>`}</div>
      ${ns ? "" : `<p class="sub" style="margin:0">Mide, hilo a hilo, cuánto tiempo le quitan al juego los drivers y las interrupciones de Windows (unos ${(state.live && state.live.cpu.threads) || 16} segundos). ${s.elevated ? "Dice también qué driver es." : ""}</p>`}
      ${ns ? (running ? pctBar(ns.pct) : "") + noiseMap(ns) : ""}
      ${ns && !running ? noiseVerdict(ns) : ""}
      ${ns && !running && !ns.deep ? `<p class="note">${esc(ns.deepErr || "Para saber qué driver es, hace falta el permiso de administrador.")}</p>` : ""}
      ${!running ? drivers : ""}</div>`;
    return top + noiseCard;
  },
};

// ---------- dos pantallas ----------
fixPanels.dual = {
  title: "Tirones con dos pantallas",
  sub: "Un vídeo o un directo en la otra pantalla puede meter tirones en el juego. Aquí se ve si te pasa y cómo quitarlo.",
  applies: () => state.stutter && state.stutter.monitors > 1,
  load: open => { if (open) { if (!state.comps) loadComps(); fixPanels.stutter.load(true); } },
  status: () => {
    const sp = dualSplit();
    if (sp && sp.with > Math.max(1, sp.without * 1.8)) return { level: "bad", text: `${n(sp.with / Math.max(0.1, sp.without), 1)}× más tirones con un vídeo puesto` };
    if (sp) return { level: "ok", text: "El vídeo de la otra pantalla no te afecta" };
    return { level: "idle", text: "Juega con un vídeo en la otra pantalla para medirlo" };
  },
  render: () => {
    const mons = (state.comps && state.comps.monitors) || [];
    const hz = [...new Set(mons.map(m => m.hz))];
    const sp = dualSplit();
    const igpu = state.comps && (state.comps.gpus || []).some(g => !g.dedicated) && (state.comps.gpus || []).some(g => g.dedicated);
    let measure;
    if (sp) {
      const max = Math.max(sp.with, sp.without, 0.1);
      const worse = sp.with > Math.max(1, sp.without * 1.8);
      measure = `<div class="dual-cmp">
          <div><span>Con un vídeo en la otra pantalla</span><div class="netbar ${worse ? "bad" : ""}"><i style="width:${(sp.with / max * 100).toFixed(1)}%"></i></div><b class="${worse ? "vhot" : ""}">${n(sp.with, 1)}/min</b></div>
          <div><span>Sin vídeo</span><div class="netbar"><i style="width:${(sp.without / max * 100).toFixed(1)}%"></i></div><b>${n(sp.without, 1)}/min</b></div></div>
        ${worse ? `<div class="verdict badv">${IC.warn}<div><b>El vídeo de la otra pantalla te mete tirones.</b> Prueba las soluciones de abajo, de la primera a la última.</div></div>` : `<div class="allgood">${IC.ok}<div><b>No te afecta.</b> Con vídeo tienes los mismos tirones que sin él.</div></div>`}`;
    } else {
      measure = `<p class="sub" style="margin:0">Todavía no hay datos: juega un rato con un vídeo (YouTube, Twitch…) en la otra pantalla y otro rato sin él. TurboMON compara los tirones de las dos formas.</p>`;
    }
    const tips = [
      "Juega en pantalla completa. Si usas «sin bordes», activa «Optimizaciones para juegos en ventana» (Configuración > Sistema > Pantalla > Gráficos > Cambiar la configuración predeterminada).",
      igpu ? "Pon el navegador en la gráfica integrada (Configuración > Sistema > Pantalla > Gráficos > el navegador > Ahorro de energía): el vídeo deja de pelearse con el juego." : "Desactiva la aceleración por hardware del navegador con el que ves el vídeo (Configuración > Sistema).",
      hz.length > 1 ? `Tus pantallas van a ${hz.slice(0, -1).join(", ")} y ${hz[hz.length - 1]} Hz. Con frecuencias distintas, Windows a veces se acompasa a la lenta mientras hay un vídeo: si puedes, ponlas a la misma, o actualiza el driver de la gráfica.` : "Actualiza el driver de la gráfica: estos tirones se han ido arreglando versión a versión.",
    ];
    return `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#38bdf8,#6366f1)">${FIC.dual}</div><h2>Tus pantallas</h2></div>
        <div class="dual-mons">${mons.map(m => `<div class="dual-mon ${m.primary ? "prim" : ""}"><b>${esc(m.name || "Pantalla")}</b><span>${m.width}×${m.height} · ${m.hz} Hz${m.primary ? " · principal" : ""}</span></div>`).join("") || `<div class="empty">Leyendo…</div>`}</div></div>
      <div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#fbbf24,#f97316);--tile-sh:rgba(245,158,11,.6)">${FIC.stutter}</div><h2>¿Te afecta?</h2></div>${measure}</div>
      <div class="card mt"><div class="card-h"><div class="ic">${IC.wrench}</div><h2>Cómo quitarlo</h2></div><ol class="tips">${tips.map(t => `<li>${esc(t)}</li>`).join("")}</ol></div>`;
  },
};

// dualSplit junta lo medido en las partidas con dos pantallas.
function dualSplit() {
  const s = state.stutter;
  if (!s) return null;
  let wS = 0, wH = 0, oS = 0, oH = 0;
  for (const x of (s.live ? [s.live] : []).concat(s.sessions || [])) {
    const d = x.stutter && x.stutter.decode;
    if (!d) continue;
    wS += d.withSecs; wH += d.with * d.withSecs / 60;
    const os = Math.max(0, x.stutter.secs - d.withSecs);
    oS += os; oH += d.without * os / 60;
  }
  if (wS < 60 || oS < 60) return null;
  return { with: wH / wS * 60, without: oH / oS * 60 };
}

Object.assign(actions, {
  stutsel: id => { state.stutSel = id; render(); },
  noise: async () => {
    if (!await confirmBox("¿Hacer el mapa de ruido?", `Tarda unos ${(state.live && state.live.cpu.threads) || 16} segundos y va ocupando los núcleos uno a uno. Mejor sin juegos abiertos.`, "Empezar")) return;
    try { const r = await App.StartNoise(); state.noise = r && r.state; } catch (e) { return toast(errMsg(e), true); }
    fixPanels.stutter.load(true);
  },
});
