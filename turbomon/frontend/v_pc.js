"use strict";
// ---------- sensores ----------
function renderSensors() {
  const l = state.live;
  if (!l) return `<div class="empty">Leyendo sensores…</div>`;
  const c = l.cpu, m = l.mem, f = l.fps;
  const cores = (c.perCore || []).map(v => `<div class="core" title="${n(v)} %"><i style="height:${Math.max(3, v).toFixed(0)}%"></i></div>`).join("");
  const cpuCard = `<div class="card"><div class="card-h"><div class="ic">${IC.cpu}</div><h2>Procesador</h2><span class="meta">${esc(c.name)}</span></div>
    <div class="bigline"><span class="v">${n(c.usage)}<small>%</small></span>${c.temp ? `<span class="v2 ${tempCls(c.temp)}">${n(c.temp)}<small>°C</small></span>` : ""}${c.mhz ? `<span class="v2">${n(c.mhz / 1000, 2)}<small>GHz</small></span>` : ""}</div>
    <div class="cores">${cores}</div>
    <dl class="kv"><dt>Núcleos / hilos</dt><dd>${c.cores || "—"} / ${c.threads}</dd><dt>Hilo más ocupado</dt><dd>${n(c.maxCore)} %</dd>
    <dt>Frecuencia base</dt><dd>${n(c.baseMHz / 1000, 2)} GHz</dd>${c.powerW ? `<dt>Consumo</dt><dd>${n(c.powerW)} W</dd>` : ""}
    <dt>Temperatura</dt><dd>${c.temp ? n(c.temp, 1) + " °C" + " (HWiNFO)" : "—"}</dd></dl>
    ${c.temp ? "" : `<div class="note">Windows no da la temperatura del procesador sin un driver especial. Si tienes <b>HWiNFO</b> abierto con «Shared Memory Support» activado, TurboMON la lee de ahí.</div>`}
    <div class="chart">${spark(l.hist.cpu, 100, undefined, 70)}</div></div>`;
  const gpuCards = (l.gpus || []).map(g => `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#22d3ee,#6366f1)">${IC.gpu}</div><h2>Gráfica</h2><span class="meta">${esc(g.name)}</span></div>
    <div class="bigline"><span class="v">${n(g.usage)}<small>%</small></span>${g.temp ? `<span class="v2 ${tempCls(g.temp)}">${n(g.temp)}<small>°C</small></span>` : ""}${g.clockMHz ? `<span class="v2">${n(g.clockMHz)}<small>MHz</small></span>` : ""}</div>
    <div class="chart">${spark(g === mainGPU(l) ? l.hist.gpu : [], 100, undefined, 70)}</div>
    <dl class="kv"><dt>Memoria de vídeo (VRAM)</dt><dd>${gb(g.vramUsed)} / ${gb(g.vram, 0)} GB</dd></dl>${pctBar(g.vram ? g.vramUsed / g.vram * 100 : 0, g.vram && g.vramUsed / g.vram > .9 ? "warn" : "")}
    <dl class="kv" style="margin-top:10px"><dt>Reloj de la gráfica</dt><dd>${g.clockMHz ? n(g.clockMHz) + " MHz" : "—"}</dd><dt>Reloj de la memoria</dt><dd>${g.memClockMHz ? n(g.memClockMHz) + " MHz" : "—"}</dd>
    <dt>Ventilador</dt><dd>${g.fanRPM ? n(g.fanRPM) + " RPM" : "Parado (o sin dato)"}</dd><dt>Tipo</dt><dd>${g.dedicated ? "Dedicada" : "Integrada"} · ${g.outputs} pantalla${g.outputs === 1 ? "" : "s"}</dd></dl></div>`).join("");
  const used = m.used / m.total * 100, sb = Math.min(100 - used, m.standby / m.total * 100);
  const memCard = `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#a78bfa,#6366f1)">${IC.ram}</div><h2>Memoria RAM</h2><span class="meta">${gb(m.total, 0)} GB</span></div>
    <div class="bigline"><span class="v">${gb(m.used)}<small>GB en uso</small></span><span class="v2">${n(m.load)}<small>%</small></span></div>
    <div class="membar"><i style="width:${used.toFixed(1)}%;background:linear-gradient(90deg,#8b5cf6,#6366f1)"></i><i style="width:${sb.toFixed(1)}%;background:rgba(34,211,238,.45)"></i></div>
    <div class="legend"><span><i style="background:#7c6cf5"></i>En uso ${gb(m.used)} GB</span><span><i style="background:rgba(34,211,238,.6)"></i>En espera ${gb(m.standby)} GB</span><span><i style="background:rgba(255,255,255,.12)"></i>Libre ${gb(m.free)} GB</span></div>
    <dl class="kv"><dt>Disponible para programas</dt><dd>${gb(m.available)} GB</dd><dt>Comprometida / límite</dt><dd>${gb(m.commit)} / ${gb(m.commitLimit, 0)} GB</dd></dl>
    <div class="note">La memoria <b>en espera</b> es caché: archivos que Windows guarda por si los vuelves a usar. Cuenta como disponible y Windows la suelta sola cuando un programa la necesita.</div>
    <div class="chart">${spark(l.hist.ram, m.total / GB, undefined, 60)}</div></div>`;
  const ioCard = `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#34d399,#0891b2)">${IC.disk}</div><h2>Disco y red</h2></div>
    <dl class="kv"><dt>Disco: lectura</dt><dd>${rate(l.diskRead)}</dd><dt>Disco: escritura</dt><dd>${rate(l.diskWrite)}</dd><dt>Disco: actividad</dt><dd>${n(l.diskBusy)} %</dd></dl>
    <div class="chart" style="height:44px">${spark(l.hist.disk, 100, undefined, 44)}</div>
    <dl class="kv"><dt>Red: bajada</dt><dd>${rate(l.netDown)}</dd><dt>Red: subida</dt><dd>${rate(l.netUp)}</dd></dl>
    <div class="chart" style="height:44px">${spark(l.hist.net, undefined, undefined, 44)}</div>
    <dl class="kv"><dt>Encendido desde hace</dt><dd>${dur(l.uptime)}</dd>${l.battery >= 0 ? `<dt>Batería</dt><dd>${l.battery} % · ${l.plugged ? "enchufado" : "sin cargador"}</dd>` : ""}</dl></div>`;
  const fpsCard = `<div class="card"><div class="card-h"><div class="ic" style="--tile:linear-gradient(135deg,#f472b6,#8b5cf6)">${IC.fps}</div><h2>FPS de lo que tienes delante</h2><span class="meta">${esc(f.name || f.exe || "")}</span></div>
    ${!f.available ? `<div class="empty">Para medir FPS hace falta dar permiso de administrador una vez. <button class="link" data-act="enablefps">Activar FPS</button></div>` : `
    <div class="bigline"><span class="v ${f.presenting ? fpsCls(f.fps) : ""}">${f.presenting ? n(f.fps) : "—"}<small>FPS</small></span>${f.presenting ? `<span class="v2">${n(f.low1)}<small>1 % bajo</small></span><span class="v2">${n(f.frameMs, 1)}<small>ms</small></span>` : ""}</div>
    <div class="chart" style="height:90px">${spark(l.hist.fps, undefined, undefined, 90)}</div>
    <dl class="kv"><dt>Programa</dt><dd>${esc(f.exe || "—")}</dd><dt>Peor fotograma (10 s)</dt><dd>${f.presenting ? n(f.maxMs, 1) + " ms" : "—"}</dd><dt>¿Es un juego?</dt><dd>${f.isGame ? "Sí: se está apuntando la partida" : "No (o aún no)"}</dd></dl>
    <div class="note">El <b>1 % bajo</b> son los FPS de los fotogramas más lentos: es lo que notas como tirones. Cuanto más cerca de los FPS medios, más suave.</div>`}</div>`;
  return `<div class="grid g2">${cpuCard}${gpuCards || ""}${memCard}${fpsCard}${ioCard}</div>`;
}

// ---------- piezas ----------
const PIC = {
  board: '<svg viewBox="0 0 24 24"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 7h4v4H7zM15 7h2M15 11h2M7 15h10"/></svg>',
  win: '<svg viewBox="0 0 24 24"><path d="M3 5.5 10 4.5v7H3zM12 4.2 21 3v8.5h-9zM3 12.5h7v7l-7-1zM12 12.5h9V21l-9-1.2z"/></svg>',
  screen: '<svg viewBox="0 0 24 24"><rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></svg>',
  mouse: '<svg viewBox="0 0 24 24"><rect x="6" y="3" width="12" height="18" rx="6"/><path d="M12 7v4"/></svg>',
  kb: '<svg viewBox="0 0 24 24"><rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M7 14h10"/></svg>',
  pad: '<svg viewBox="0 0 24 24"><path d="M6 11h4M8 9v4M15 12h.01M18 10h.01"/><path d="M17.32 5H6.68a4 4 0 0 0-3.98 3.59l-.9 8.06A2 2 0 0 0 3.79 19c.82 0 1.58-.45 2-1.17L7 16h10l1.2 1.83c.42.72 1.18 1.17 2 1.17a2 2 0 0 0 2-2.35l-.9-8.06A4 4 0 0 0 17.32 5z"/></svg>',
  head: '<svg viewBox="0 0 24 24"><path d="M3 14v-2a9 9 0 0 1 18 0v2"/><rect x="2" y="14" width="5" height="7" rx="2"/><rect x="17" y="14" width="5" height="7" rx="2"/></svg>',
  mic: '<svg viewBox="0 0 24 24"><rect x="9" y="2" width="6" height="12" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v4"/></svg>',
  wifi: '<svg viewBox="0 0 24 24"><path d="M5 12.55a11 11 0 0 1 14 0M1.42 9a16 16 0 0 1 21.16 0M8.53 16.11a6 6 0 0 1 6.95 0M12 20h.01"/></svg>',
  cable: '<svg viewBox="0 0 24 24"><rect x="4" y="9" width="16" height="11" rx="2"/><path d="M9 9V5h6v4M9 14h.01M12 14h.01M15 14h.01"/></svg>',
  battery: '<svg viewBox="0 0 24 24"><rect x="2" y="7" width="18" height="10" rx="2"/><path d="M22 11v2M6 11v2"/></svg>',
  speed: '<svg viewBox="0 0 24 24"><path d="M12 14l4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/></svg>',
};
const yes = '<svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg>';
const chipOk = t => `<span class="chip ok">${yes}${t}</span>`;
const chipWarn = t => `<span class="chip warnc">${IC.warn}${t}</span>`;
const chipInfo = t => `<span class="chip">${t}</span>`;

function checkStatus(id) {
  const r = state.report && state.report.results.find(x => x.id === id || x.id.startsWith(id + ":"));
  return r ? r.status : "";
}
function biosDate(s) { // mm/dd/aaaa → dd/mm/aaaa
  const m = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/.exec(s || "");
  return m ? `${m[2].padStart(2, "0")}/${m[1].padStart(2, "0")}/${m[3]}` : s;
}
function dateES(s) { // aaaa-mm-dd → dd/mm/aaaa
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s || "");
  return m ? `${m[3]}/${m[2]}/${m[1]}` : s;
}
function sizeTB(b) {
  const tb = b / 1e12;
  return tb >= 0.95 ? n(tb, tb >= 10 || Math.abs(tb - Math.round(tb)) < 0.05 ? 0 : 1) + " TB" : n(b / 1e9, 0) + " GB";
}
function shortCPU(s) { return (s || "").replace(/\(R\)|\(TM\)|\(tm\)| CPU| Processor/g, "").replace(/ @.*$/, "").replace(/ \d+-Core$/, "").replace(/\s+/g, " ").trim(); }
function shortGPU(s) { return (s || "").replace("NVIDIA GeForce ", "GeForce ").replace("AMD Radeon ", "Radeon ").replace(/\(R\)|\(TM\)/g, "").trim(); }
const gens = { 1: "1.0", 2: "2.0", 3: "3.0", 4: "4.0", 5: "5.0" };

function partCard(icon, tile, title, sub, body, extra = "") {
  return `<div class="card part"><div class="part-h"><div class="ic" style="--tile:${tile}">${icon}</div><div class="part-t"><b>${title}</b>${sub ? `<span>${sub}</span>` : ""}</div>${extra}</div>${body}</div>`;
}

function renderParts() {
  const c = state.comps;
  if (!c) return `<div class="empty">Mirando qué piezas tiene tu PC…</div>`;
  const cpu = c.cpu;
  const coresTxt = cpu.pCores && cpu.eCores ? `${cpu.pCores} de rendimiento + ${cpu.eCores} de eficiencia` : `${cpu.cores} núcleos`;
  const mc = checkStatus("intel_microcode");
  const cpuCard = partCard(IC.cpu, "linear-gradient(135deg,#8b5cf6,#6366f1)", esc(shortCPU(cpu.name)), esc(cpu.vendor === "intel" ? "Intel" : cpu.vendor === "amd" ? "AMD" : ""),
    `<dl class="kv"><dt>Núcleos</dt><dd>${coresTxt} · ${cpu.threads} hilos</dd>
      ${cpu.l3KB ? `<dt>Caché</dt><dd>L2 ${n(cpu.l2KB / 1024, 1)} MB · L3 ${n(cpu.l3KB / 1024, 0)} MB${cpu.l3Groups > 1 ? ` (${cpu.l3Groups} chips)` : ""}</dd>` : ""}
      ${cpu.maxMHz ? `<dt>Frecuencia</dt><dd>${n(cpu.baseMHz / 1000, 1)} GHz base · hasta ${n(cpu.maxMHz / 1000, 1)} GHz</dd>` : ""}
      ${cpu.microcode ? `<dt>Microcódigo</dt><dd>0x${cpu.microcode.toString(16).toUpperCase()}</dd>` : ""}</dl>
     ${mc === "warn" ? `<div class="part-note warn">${IC.warn}<span>Tu BIOS no trae el microcódigo que protege a estos procesadores. <button class="link" data-act="howto" data-arg="intel_microcode">Ver qué hacer</button></span></div>` : mc === "ok" ? `<div class="chips mt-s">${chipOk("Microcódigo protegido")}</div>` : ""}`);
  const gpus = (c.gpus || []).filter(g => g.dedicated).concat((c.gpus || []).filter(g => !g.dedicated));
  const gpuCards = gpus.map(g => {
    const chips = [];
    if (g.linkWidth) chips.push(checkStatus("gpu_pcie") === "bad" ? chipWarn(`PCIe x${g.linkWidth} de x${g.maxWidth}`) : chipOk(`PCIe ${gens[g.maxGen] || ""} x${g.linkWidth}`));
    if (g.rebar === 1) chips.push(chipOk("Resizable BAR"));
    else if (g.rebar === 0) chips.push(chipWarn("Sin Resizable BAR"));
    const drv = g.vendor === "nvidia" ? nvVer(g.driverVersion) : g.driverVersion;
    return partCard(IC.gpu, "linear-gradient(135deg,#22d3ee,#6366f1)", esc((g.partner && !g.partner.includes("(") ? g.partner + " " : "") + shortGPU(g.name)), esc(g.dedicated ? "Gráfica dedicada" : "Gráfica integrada"),
      `<dl class="kv">${g.vram ? `<dt>Memoria de vídeo</dt><dd>${gb(g.vram, 0)} GB</dd>` : ""}
        ${drv ? `<dt>Driver</dt><dd>${esc(drv)}${g.driverDate ? " · " + dateES(g.driverDate) : ""}</dd>` : ""}
        ${g.partner && g.partner.includes("(") ? `<dt>Modelo</dt><dd>${esc(g.partner)}</dd>` : ""}
        <dt>Pantallas conectadas</dt><dd>${g.outputs}</dd></dl>
       ${chips.length ? `<div class="chips mt-s">${chips.join("")}</div>` : ""}`);
  }).join("");
  const ram = c.ram || [];
  const r0 = ram[0] || {};
  const sp = r0.configured || r0.speed;
  const ramChips = [];
  const xmp = checkStatus("ram_xmp"), ch = checkStatus("ram_channels");
  if (xmp) ramChips.push(xmp === "ok" ? chipOk("A su velocidad (XMP)") : chipWarn("Sin XMP"));
  if (ch) ramChips.push(ch === "ok" ? chipOk("Doble canal") : chipWarn("Un solo canal"));
  const ramCard = partCard(IC.ram, "linear-gradient(135deg,#a78bfa,#6366f1)", `${gb(c.ramTotal || ram.reduce((a, d) => a + d.sizeMB * 1048576, 0), 0)} GB ${esc(r0.type || "")}${sp ? ` a ${sp} MT/s` : ""}`, `${ram.length} módulo${ram.length === 1 ? "" : "s"}`,
    `<div class="mods">${ram.map(d => `<div class="mod"><b>${esc([d.maker, d.part].filter(Boolean).join(" "))}</b><span>${n(d.sizeMB / 1024, 0)} GB · ranura ${esc(d.locator || "?")}${d.voltage ? ` · ${n(d.voltage / 1000, 2)} V` : ""}${d.rated && d.rated > (d.configured || 0) ? ` · hecha para ${d.rated}` : ""}</span></div>`).join("")}</div>
     ${ramChips.length ? `<div class="chips mt-s">${ramChips.join("")}</div>` : ""}`);
  const b = c.board;
  const boardCard = partCard(PIC.board, "linear-gradient(135deg,#64748b,#334155)", esc([b.maker, b.model].filter(Boolean).join(" ") || [b.systemMaker, b.systemModel].filter(Boolean).join(" ") || "Placa base"), esc(b.laptop ? "Portátil" : "Placa base"),
    `<dl class="kv">${b.biosVersion ? `<dt>BIOS</dt><dd>${esc(b.biosVersion)}${b.biosDate ? " · " + biosDate(b.biosDate) : ""}</dd>` : ""}
      ${b.biosVendor ? `<dt>Firmware</dt><dd>${esc(b.biosVendor.replace(/,? LLC\.?/, ""))}</dd>` : ""}
      ${b.laptop && b.systemModel ? `<dt>Modelo</dt><dd>${esc(b.systemMaker + " " + b.systemModel)}</dd>` : ""}</dl>`);
  const disks = (c.disks || []).map(d => {
    const kind = { nvme: "SSD NVMe", ssd: "SSD", hdd: "Disco duro", usb: "USB" }[d.kind] || d.kind;
    const h = d.health;
    const test = state.diskTest && state.diskTest.results && state.diskTest.results[d.number];
    const running = state.diskTest && state.diskTest.running && state.diskTest.disk === d.number;
    const chips = [chipInfo(esc(kind + (d.bus && d.kind !== "nvme" ? " · " + d.bus : ""))), ...(d.letters || []).map(l => chipInfo(esc(l)))];
    if (d.rpm > 1) chips.push(chipInfo(`${n(d.rpm)} rpm`));
    if (d.tempC) chips.push(d.tempC >= 70 ? chipWarn(`${d.tempC} °C`) : chipInfo(`${d.tempC} °C`));
    let health = "";
    if (h && h.usedPct >= 0) {
      const life = Math.max(0, 100 - h.usedPct);
      health = `<div class="lifebar"><span>Vida útil</span>${pctBar(life, life < 20 ? "bad" : life < 40 ? "warn" : "")}<b>${life} %</b></div>`;
    }
    const facts = [];
    if (h && h.writtenTB) facts.push(`${n(h.writtenTB, h.writtenTB < 10 ? 1 : 0)} TB escritos`);
    if (h && h.powerOnHours) facts.push(`${n(h.powerOnHours)} h encendido`);
    if (h && h.reallocated) facts.push(`<span class="hot">${h.reallocated} sectores dañados</span>`);
    let speed = "";
    if (running) speed = `<div class="dtest"><span>${state.diskTest.phase === "read" ? "Midiendo lectura…" : "Midiendo escritura…"}</span>${pctBar(state.diskTest.pct)}</div>`;
    else if (test) speed = `<div class="dspeed"><div><b>${n(test.read)}</b><span>MB/s leyendo</span></div><div><b>${n(test.write)}</b><span>MB/s escribiendo</span></div>${d.maxMBps ? `<div class="dmax">tope de su conexión: ~${n(d.maxMBps)} MB/s</div>` : ""}</div>`;
    return `<div class="disk"><div class="disk-h"><div class="disk-ic ${d.kind}">${IC.disk}</div><div class="disk-t"><b>${esc(d.model || "Disco")}</b><span>${sizeTB(d.size)}${d.maker && !(d.model || "").toLowerCase().startsWith(d.maker.toLowerCase()) ? " · " + esc(d.maker) : ""}</span></div>
      <button class="btn sm ghost" data-act="disktest" data-arg="${d.number}" ${state.diskTest && state.diskTest.running ? "disabled" : ""}>${PIC.speed}${test ? "Repetir" : "Medir velocidad"}</button></div>
      <div class="chips">${chips.join("")}</div>${health}${facts.length ? `<div class="facts">${facts.join(" · ")}</div>` : ""}${speed}</div>`;
  }).join("");
  const diskCard = partCard(IC.disk, "linear-gradient(135deg,#34d399,#0891b2)", "Discos", `${(c.disks || []).length} disco${(c.disks || []).length === 1 ? "" : "s"}`, `<div class="disks">${disks}</div>
    <div class="note">«Medir velocidad» escribe y lee un archivo de 1 GB en ese disco (unos segundos) y lo borra.${(c.disks || []).some(d => d.health) ? "" : " La salud de los discos necesita el permiso de administrador (el mismo de los FPS)."}</div>`);
  const mons = (c.monitors || []).map(m => `<div class="row-item"><div class="ri-ic">${PIC.screen}</div><div class="ri-t"><b>${esc([m.maker, m.name].filter(x => x && x !== "Pantalla").join(" ") || "Pantalla")}</b><span>${m.inches ? n(m.inches, 0) + "″ · " : ""}${m.width}×${m.height} · ${m.hz} Hz${m.maxHz > m.hz ? ` (puede ${m.maxHz})` : ""}</span></div>
      <div class="chips">${m.primary ? chipInfo("Principal") : ""}${m.hdr ? chipInfo(m.hdrOn ? "HDR activado" : "Admite HDR") : ""}</div></div>`).join("");
  const monCard = partCard(PIC.screen, "linear-gradient(135deg,#f472b6,#8b5cf6)", "Pantallas", `${(c.monitors || []).length}`, `<div class="rows">${mons}</div>`);
  const nets = (c.network || []).map(a => `<div class="row-item ${a.up ? "" : "off"}"><div class="ri-ic">${a.kind === "wifi" ? PIC.wifi : PIC.cable}</div><div class="ri-t"><b>${a.kind === "wifi" ? "Wifi" : "Cable"} · ${esc(a.model)}</b>
      <span>${a.up ? [a.linkMbps ? (a.linkMbps >= 1000 ? n(a.linkMbps / 1000, a.linkMbps % 1000 ? 1 : 0) + " Gbps" : a.linkMbps + " Mbps") : "", a.wifiStd, a.wifiBand, a.wifiSignal ? "señal " + a.wifiSignal + " %" : ""].filter(Boolean).join(" · ") : "Sin conectar"}</span></div>
      <div class="chips">${a.default ? chipOk("Internet va por aquí") : ""}</div></div>`).join("");
  const netCard = partCard(PIC.cable, "linear-gradient(135deg,#38bdf8,#0ea5e9)", "Red", "", `<div class="rows">${nets || '<div class="empty">Sin adaptadores de red</div>'}</div>`);
  const kindIc = { raton: PIC.mouse, teclado: PIC.kb, mando: PIC.pad, audio: PIC.head, microfono: PIC.mic };
  const kindName = { raton: "Ratón", teclado: "Teclado", mando: "Mando", audio: "Audio", microfono: "Micrófono" };
  const pers = (c.peripherals || []).map(p => `<div class="row-item"><div class="ri-ic">${kindIc[p.kind] || ""}</div><div class="ri-t"><b>${esc(p.name)}</b><span>${kindName[p.kind] || ""}${p.maker ? " · " + esc(p.maker) : ""}</span></div>
      ${p.kind === "raton" ? `<button class="btn sm ghost" data-act="mousetest">Medir el ratón</button>` : ""}</div>`).join("");
  const perCard = partCard(PIC.mouse, "linear-gradient(135deg,#fbbf24,#f97316)", "Periféricos", "", `<div class="rows">${pers || '<div class="empty">No se ha encontrado ninguno</div>'}</div>
    ${state.mouseHz ? `<div class="part-note ${state.mouseHz < 450 ? "warn" : ""}">${state.mouseHz < 450 ? IC.warn : yes}<span>Tu ratón manda <b>${state.mouseHz} actualizaciones por segundo</b>${state.mouseHz < 450 ? ". Para jugar, ponlo a 1000 Hz en el programa del ratón (si lo permite): el movimiento es más preciso." : ": perfecto para jugar."}</span></div>` : ""}`);
  const o = c.os;
  const osCard = partCard(PIC.win, "linear-gradient(135deg,#0ea5e9,#2563eb)", esc(o.name || "Windows"), esc([o.version, o.build ? "compilación " + o.build : ""].filter(Boolean).join(" · ")), "");
  const bat = c.battery ? partCard(PIC.battery, "linear-gradient(135deg,#34d399,#059669)", `Batería al ${c.battery.health} %`, "de su capacidad de fábrica",
    `<dl class="kv"><dt>Capacidad</dt><dd>${n(c.battery.fullWh, 0)} de ${n(c.battery.designWh, 0)} Wh</dd>${c.battery.cycles ? `<dt>Ciclos de carga</dt><dd>${c.battery.cycles}</dd>` : ""}</dl>`) : "";
  return `<div class="pc-hero"><div class="pc-sum">${esc(c.summary)}</div></div>
    <div class="grid g2">${cpuCard}${gpuCards}${ramCard}${boardCard}${diskCard}${monCard}${netCard}${perCard}${osCard}${bat}</div>`;
}

function nvVer(v) {
  const p = (v || "").split(".");
  if (p.length !== 4) return v;
  const s = p[2].slice(-1) + p[3].padStart(4, "0");
  return s.slice(0, 3) + "." + s.slice(3);
}

function renderPC() {
  const sub = state.sub.pc || "parts";
  const tabs = `<div class="seg"><button class="${sub === "parts" ? "on" : ""}" data-act="sub" data-arg="parts">Piezas</button><button class="${sub === "live" ? "on" : ""}" data-act="sub" data-arg="live">En directo</button></div>`;
  return `<div class="head"><div><h1>Mi PC</h1><p class="sub">${sub === "parts" ? "El modelo exacto de cada pieza." : "Todo en directo, una lectura por segundo."}</p></div><span class="grow"></span>${tabs}</div>
    ${sub === "parts" ? renderParts() : renderSensors()}`;
}

async function loadComps(force) {
  if (!force && state.comps && Date.now() - state.compsAt < 30000) return;
  try { state.comps = await App.Components(); state.compsAt = Date.now(); } catch (e) { }
  render();
}

views.pc = { render: renderPC, enter: () => loadComps() };

async function pollDiskTest() {
  try { state.diskTest = await App.DiskTest(); } catch (e) { }
  render();
  if (state.diskTest && state.diskTest.running) setTimeout(pollDiskTest, 400);
  else if (state.diskTest && state.diskTest.error) toast(state.diskTest.error, true);
}

Object.assign(actions, {
  disktest: async a => {
    try { await App.StartDiskTest(+a); } catch (e) { return toast(errMsg(e), true); }
    pollDiskTest();
  },
  mousetest: () => mouseTest(),
});

// Mide cuántas veces por segundo manda el ratón su posición (con los eventos
// «en bruto» del navegador, que llegan sin agrupar).
function mouseTest() {
  const m = $("#modal");
  $("#modal-card").innerHTML = `<h2>Mide tu ratón</h2><p>Mueve el ratón rápido, en círculos, dentro del recuadro durante 3 segundos.</p>
    <div class="mtest" id="mtest"><b id="mhz">—</b><span>actualizaciones por segundo</span></div>
    <div class="btns"><button class="btn ghost" data-r="0">Cerrar</button></div>`;
  m.hidden = false;
  const area = $("#mtest");
  const times = [];
  let best = 0;
  const evName = "onpointerrawupdate" in window ? "pointerrawupdate" : "pointermove";
  const onMove = e => {
    const evs = e.getCoalescedEvents ? e.getCoalescedEvents() : [e];
    for (const ev of (evs.length ? evs : [e])) times.push(ev.timeStamp);
    while (times.length && times[times.length - 1] - times[0] > 500) times.shift();
    if (times.length > 20) {
      const d = [];
      for (let i = 1; i < times.length; i++) if (times[i] > times[i - 1]) d.push(times[i] - times[i - 1]);
      d.sort((a, b) => a - b);
      const med = d[Math.floor(d.length / 2)] || 1;
      const hz = Math.round(1000 / med);
      const nice = [125, 250, 500, 1000, 2000, 4000, 8000].reduce((a, b) => Math.abs(b - hz) < Math.abs(a - hz) ? b : a);
      best = Math.max(best, Math.abs(nice - hz) / nice < 0.25 ? nice : hz);
      $("#mhz").textContent = best.toLocaleString("es-ES");
    }
  };
  area.addEventListener(evName, onMove);
  m.onclick = e => {
    const b = e.target.closest("[data-r]");
    if (!b && e.target !== m) return;
    area.removeEventListener(evName, onMove);
    m.hidden = true;
    if (best) { state.mouseHz = best; render(); }
  };
}