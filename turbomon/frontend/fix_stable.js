"use strict";
// ---------- «¿Es estable mi PC?» ----------
fixPanels.stable = {
  title: "¿Es estable mi PC?",
  sub: "Busca núcleos que se equivocan y errores en la RAM, como hacen Google y Meta con sus servidores.",
  load: async force => {
    try { const r = await App.Stability(); state.stab = r && r.state; } catch (e) { }
    render();
    if (state.stab && state.stab.running) { clearTimeout(fixPanels.stable.t); fixPanels.stable.t = setTimeout(() => fixPanels.stable.load(), 700); }
  },
  status: () => {
    const s = state.stab;
    if (!s) return { level: "idle", text: "Sin probar todavía" };
    if (s.running) return { level: "idle", text: "Probando ahora…" };
    const bad = (s.cores || []).filter(c => c.errors > 0).length;
    if (bad || s.ramErrors) return { level: "bad", text: bad ? `${bad} hilo${bad === 1 ? "" : "s"} con errores` : "Errores en la RAM" };
    if (s.stopped) return { level: "idle", text: "La última prueba se paró antes de acabar" };
    return { level: "ok", text: "Estable en la última prueba (" + when(s.finished) + ")" };
  },
  render: () => {
    const s = state.stab;
    const running = s && s.running;
    const cores = (s && s.cores) || Array.from({ length: (state.live && state.live.cpu.threads) || 16 }, (_, i) => ({ logical: i, errors: 0, items: 0 }));
    const grid = cores.map(c => {
      let cls = "";
      if (c.errors > 0) cls = "err";
      else if (running && s.phase === "single" && s.current === c.logical) cls = "now";
      else if (running && s.phase === "all") cls = "now";
      else if (c.items > 0) cls = "okc";
      return `<div class="stc ${cls}" title="Hilo ${c.logical}: ${n(c.items)} cálculos, ${c.errors} errores"><b>${c.logical}</b>${c.errors ? `<span>${c.errors}</span>` : ""}</div>`;
    }).join("");
    const phaseTxt = !s ? "" : { single: "Hilo a hilo, a la frecuencia máxima de un núcleo", all: "Todos los hilos a la vez (calor y tensión mínima)", ram: `Comprobando ${n(s.ramMB / 1024, 1)} GB de RAM`, done: "" }[s.phase] || "";
    let result = "";
    if (s && !running) {
      const bad = (s.cores || []).filter(c => c.errors > 0);
      const items = (s.cores || []).reduce((a, c) => a + c.items, 0);
      if (bad.length || s.ramErrors) {
        result = `<div class="verdict badv">${IC.warn}<div><b>${bad.length ? `${bad.length === 1 ? "El hilo " + bad[0].logical + " ha" : bad.length + " hilos han"} dado resultados erróneos` : "La RAM ha dado errores"}.</b>
          Un PC estable no se equivoca nunca en estos cálculos. Es la señal típica de un overclock o undervolt demasiado justo, de un XMP inestable o de un procesador que se degrada (los Intel de 13.ª y 14.ª generación sin la BIOS nueva).
          <ol><li>Quita overclocks y undervolts (en la BIOS o en programas como Ryzen Master, Intel XTU o el Curve Optimizer).</li><li>${s.ramErrors ? "Prueba la RAM con un perfil XMP/EXPO más suave." : "Si tienes un Intel de 13.ª/14.ª gen., actualiza la BIOS (en «Revisión» tienes los pasos)."}</li><li>Vuelve a pasar la prueba.</li></ol></div></div>`;
      } else if (s.stopped) {
        result = `<div class="verdict">${IC.info}<div>La prueba se paró antes de acabar. Hasta donde llegó, no hubo errores.</div></div>`;
      } else {
        result = `<div class="allgood">${IC.ok}<div><b>Estable.</b> ${n(items)} cálculos en ${(s.cores || []).length} hilos y ${n(s.ramMB / 1024, 1)} GB de RAM sin un solo error (${when(s.finished)}).
          Si aun así tienes cierres o reinicios, mira la temperatura y la fuente de alimentación.</div></div>`;
      }
    }
    const hw = state.crashes ? state.crashes.hwErrors : null;
    return `<div class="card stab-card"><div class="stab-top"><div class="t"><b>${running ? "Probando…" : "Prueba de unos 2 minutos"}</b>
        <p>${running ? esc(phaseTxt) : "El procesador irá al máximo un rato: mejor sin juegos abiertos. Cada cuadro es un hilo; se pone verde si no se equivoca."}</p></div>
        ${running ? `<button class="btn ghost" data-act="stabstop">Parar</button>` : `<button class="btn primary" data-act="stabstart">${IC.shield}${s ? "Repetir la prueba" : "Empezar la prueba"}</button>`}</div>
      ${running ? pctBar(s.pct) : ""}
      <div class="stab-grid">${grid}</div></div>
      ${result}
      ${hw !== null ? `<div class="card mt"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#64748b,#334155)">${IC.cpu}</div><h2>Errores de hardware corregidos</h2><span class="meta">últimos 60 días</span></div>
        <p class="sub" style="margin:0">${hw ? `Windows ha apuntado <b>${hw}</b> errores que el procesador o la RAM corrigieron solos. En un PC estable no aparece ninguno: son el primer aviso de inestabilidad.` : "Ninguno: el procesador y la RAM no han tenido que corregir nada. Buena señal."}</p></div>` : ""}`;
  },
};

Object.assign(actions, {
  stabstart: async () => {
    if (!await confirmBox("¿Empezar la prueba?", "Tarda unos 2 minutos y pone el procesador al máximo. Cierra los juegos antes. Puedes seguir usando el PC, pero irá más lento.", "Empezar")) return;
    try { const r = await App.StartStability(); state.stab = r && r.state; } catch (e) { return toast(errMsg(e), true); }
    fixPanels.stable.load();
  },
  stabstop: async () => { try { await App.StopStability(); } catch (e) { } fixPanels.stable.load(); },
});
