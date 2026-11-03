// Datos de muestra para ver la interfaz en un navegador (sin Wails).
// ?nofps  → como si no hubiera permiso para medir FPS
// ?nogame → sin juego abierto
// ?mal    → un PC con varias cosas mal configuradas
(function () {
  if (window.go && window.go.main) return;
  const q = new URLSearchParams(location.search);
  const noFPS = q.has("nofps"), noGame = q.has("nogame"), bad = q.has("mal");
  const GB = 1 << 30;
  const hist = { cpu: [], gpu: [], ram: [], fps: [], gpuTemp: [], cpuTemp: [], vram: [], disk: [], net: [] };
  let t = 0;
  let settings = {
    overlay: { on: true, fpsOnly: false, corner: "ti", scale: 100, opacity: 88, gameOnly: false, low1: true, frameMs: false, cpu: true, cpuTemp: true, gpu: true, gpuTemp: true, ram: true, vram: true, clock: false, power: false, hotkey: true },
    sessions: true, turboMode: true, turboPlan: true, turboPrio: true, turboStandby: true, welcomed: true,
  };
  const wave = (a, b, s) => a + (b - a) * (0.5 + 0.5 * Math.sin(t / s)) + (Math.random() - 0.5) * (b - a) * 0.15;
  function live() {
    t++;
    const game = !noGame;
    const fps = game ? Math.round(wave(118, 146, 7)) : 0;
    const cpu = game ? wave(38, 62, 5) : wave(4, 14, 6);
    const gpu = game ? wave(88, 99, 4) : wave(2, 9, 5);
    const ram = (game ? 17.8 : 11.2) + Math.sin(t / 9) * 0.3;
    const vram = game ? 6.4 + Math.sin(t / 8) * 0.2 : 1.1;
    const gt = game ? wave(66, 71, 10) : 41;
    const ct = game ? wave(61, 74, 6) : 38;
    const push = (k, v) => { hist[k].push(Math.round(v * 10) / 10); if (hist[k].length > 90) hist[k].shift(); };
    push("cpu", cpu); push("gpu", gpu); push("ram", ram); push("fps", fps); push("gpuTemp", gt); push("cpuTemp", ct); push("vram", vram);
    push("disk", wave(0, 8, 3)); push("net", wave(20, 400, 4));
    const perCore = Array.from({ length: 16 }, (_, i) => Math.max(0, Math.min(100, cpu + (Math.random() - 0.5) * 50 + (i === 3 ? 30 : 0))));
    return {
      time: Date.now(),
      cpu: { name: "Intel(R) Core(TM) i5-14400F", cores: 10, threads: 16, baseMHz: 2496, usage: cpu, maxCore: Math.max(...perCore), perCore, mhz: Math.round(game ? wave(4300, 4700, 3) : 1200), temp: ct, tempSrc: "hwinfo", powerW: game ? 74 : 12 },
      mem: { total: 32 * GB, used: ram * GB, available: (32 - ram) * GB, standby: 9.2 * GB, free: 3.1 * GB, commit: 24 * GB, commitLimit: 50 * GB, load: Math.round(ram / 32 * 100) },
      gpus: [{ name: "NVIDIA GeForce RTX 4060", vendor: "nvidia", vram: 8 * GB, dedicated: true, outputs: 2, usage: gpu, temp: gt, vramUsed: vram * GB, fanRPM: game ? 1850 : 0, clockMHz: game ? 2760 : 210, memClockMHz: 8501 }],
      diskRead: wave(0, 40e6, 3), diskWrite: wave(0, 8e6, 4), diskBusy: wave(0, 8, 3), netDown: wave(2e3, 900e3, 4), netUp: wave(1e3, 120e3, 5),
      battery: -1, plugged: true, uptime: 7200 + t, full: true,
      fps: { available: !noFPS, reason: noFPS ? "medir los FPS necesita permiso de administrador" : "", pid: 4242, exe: game ? "cyberpunk2077.exe" : "chrome.exe", name: game ? "Cyberpunk 2077" : "Google Chrome", isGame: game, presenting: game && !noFPS, fps: noFPS ? 0 : fps, low1: game && !noFPS ? Math.round(fps * 0.71) : 0, frameMs: fps ? Math.round(100000 / fps) / 100 : 0, maxMs: 21.4 },
      session: game && !noFPS ? { game: "Cyberpunk 2077", secs: 2710 + t, fpsAvg: 131.4, low1: 92.6, turbo: true } : null,
      hist: JSON.parse(JSON.stringify(hist)),
      agent: { version: "1.0.0", elevated: !noFPS, cpu: 0.08, memMB: 26.4, overlay: settings.overlay.on, histBytes: 48213, sessions: 6, task: !noFPS, hotkey: true, startedAgo: 9000 },
    };
  }
  const R = (id, group, title, status, value, detail, extra) => Object.assign({ id, group, title, status, value, detail, weight: 5 }, extra || {});
  function report() {
    const rs = bad ? [
      R("ram_xmp", "Memoria", "Velocidad de la RAM (XMP / EXPO)", "bad", "Va a 2133 de 3600 MT/s", "Tu RAM está hecha para ir a 3600 MT/s pero va a 2133: le falta activar el perfil XMP en la BIOS. Es seguro (es la velocidad que pone en la caja) y da más FPS, sobre todo en los juegos que tiran del procesador.", { weight: 15, howTo: ["Reinicia el PC y entra en la BIOS (suele ser la tecla Supr o F2 justo al encender).", "Busca «XMP», «EXPO», «DOCP» o «A-XMP».", "Ponlo en «Perfil 1», guarda con F10 y reinicia.", "Vuelve aquí y pulsa «Revisar otra vez»."] }),
      R("ram_channels", "Memoria", "Doble canal", "bad", "1 módulo", "Con un solo módulo la memoria trabaja en un canal: la mitad de velocidad.", { weight: 12 }),
      R("ram_amount", "Memoria", "Cantidad de RAM", "warn", "8 GB", "Con 8 GB muchos juegos actuales van justos.", { weight: 8 }),
      R("monitor_hz:\\\\.\\DISPLAY1", "Pantalla", "Hercios de tu pantalla", "bad", "60 de 165 Hz", "Tu pantalla puede ir a 165 Hz pero Windows la tiene a 60.", { weight: 15, fix: "hz:\\\\.\\DISPLAY1", fixLabel: "Poner a 165 Hz" }),
      R("monitor_gpu", "Pantalla", "Pantalla conectada a la gráfica", "ok", "En la NVIDIA GeForce RTX 3060", "La pantalla está conectada a la tarjeta gráfica, como debe.", { weight: 20 }),
      R("gpu_driver", "Gráfica", "Driver de la gráfica", "warn", "Del 12/02/2026 · 572.16", "El driver tiene 8 meses.", { weight: 6, howTo: ["Abre la «NVIDIA App» y pulsa «Descargar» en Controladores."] }),
      R("power_plan", "Energía", "Plan de energía", "bad", "Economizador", "Con «Economizador» Windows frena el procesador a propósito.", { weight: 8, fix: "plan", fixLabel: "Cambiar a más rendimiento" }),
      R("cpu_max", "Energía", "Procesador sin límite", "ok", "Al 100 %", "El procesador puede dar todo lo que tiene.", { weight: 10 }),
      R("game_mode", "Windows", "Modo de juego de Windows", "warn", "Desactivado", "El modo de juego da prioridad al juego.", { weight: 3, fix: "gamemode", fixLabel: "Activarlo" }),
      R("game_dvr", "Windows", "Grabación en segundo plano (Xbox)", "warn", "Grabando siempre", "La Xbox Game Bar está grabando siempre.", { weight: 5, fix: "dvr", fixLabel: "Dejar de grabar de fondo" }),
      R("disk_free", "Discos", "Espacio libre en C:", "ok", "120 GB libres", "Hay espacio de sobra.", { weight: 5 }),
      R("mouse_accel", "Windows", "Aceleración del ratón", "warn", "Activada", "Para apuntar en juegos estorba.", { weight: 2, fix: "mouse", fixLabel: "Desactivarla" }),
    ] : [
      R("ram_xmp", "Memoria", "Velocidad de la RAM (XMP / EXPO)", "ok", "DDR4 a 3200 MT/s", "Tu memoria va a 3200 MT/s, su velocidad de fábrica: el perfil XMP está activado.", { weight: 15 }),
      R("ram_channels", "Memoria", "Doble canal", "ok", "2 módulos", "La memoria trabaja en doble canal.", { weight: 12 }),
      R("ram_amount", "Memoria", "Cantidad de RAM", "ok", "32 GB", "Suficiente para jugar.", { weight: 8 }),
      R("monitor_hz:\\\\.\\DISPLAY1", "Pantalla", "Hercios de 24G4", "ok", "180 Hz", "Va a los hercios más altos que admite.", { weight: 15 }),
      R("monitor_hz:\\\\.\\DISPLAY2", "Pantalla", "Hercios de PHL24E1N1100A", "ok", "100 Hz", "Va a los hercios más altos que admite.", { weight: 15 }),
      R("monitor_gpu", "Pantalla", "Pantalla conectada a la gráfica", "ok", "En la NVIDIA GeForce RTX 4060", "La pantalla está conectada a la tarjeta gráfica, como debe.", { weight: 20 }),
      R("gpu_driver", "Gráfica", "Driver de la gráfica", "ok", "Del 24/08/2026 · 616.64", "El driver es reciente.", { weight: 6 }),
      R("hags", "Gráfica", "Programación de GPU acelerada por hardware", "ok", "Activada", "La necesita la generación de fotogramas (DLSS 3).", { weight: 0 }),
      R("power_plan", "Energía", "Plan de energía", "ok", "Alto rendimiento", "No limita el rendimiento.", { weight: 8 }),
      R("cpu_max", "Energía", "Procesador sin límite", "ok", "Al 100 %", "El procesador puede dar todo lo que tiene.", { weight: 10 }),
      R("game_mode", "Windows", "Modo de juego de Windows", "ok", "Activado", "Windows da prioridad al juego.", { weight: 3 }),
      R("game_dvr", "Windows", "Grabación en segundo plano (Xbox)", "ok", "Desactivada", "La Xbox Game Bar no está grabando de fondo.", { weight: 5 }),
      R("pagefile", "Windows", "Memoria virtual", "ok", "18 GB", "Windows tiene memoria virtual de reserva.", { weight: 6 }),
      R("mouse_accel", "Windows", "Aceleración del ratón", "warn", "Activada", "«Mejorar la precisión del puntero» hace que el ratón vaya más o menos lejos según lo rápido que lo muevas. Para apuntar en juegos estorba.", { weight: 2, fix: "mouse", fixLabel: "Desactivarla" }),
      R("startup", "Windows", "Programas que arrancan con Windows", "ok", "9 programas", "Pocos programas arrancan solos.", { weight: 3 }),
      R("disk_free", "Discos", "Espacio libre en C:", "ok", "296 GB libres", "Hay espacio de sobra.", { weight: 5 }),
      R("games_ssd", "Discos", "Juegos en SSD", "ok", "Todos en SSD", "Tus juegos están en SSD: cargan rápido.", { weight: 5 }),
    ];
    let s = 100;
    rs.forEach(r => { if (r.status === "bad") s -= r.weight; if (r.status === "warn") s -= r.weight / 2; });
    return { score: Math.max(0, Math.round(s)), results: rs, time: Date.now() / 1000 | 0 };
  }
  let rep = report();
  const now = Date.now() / 1000 | 0;
  const series = (n, base, amp) => Array.from({ length: n }, (_, i) => Math.round((base + Math.sin(i / 6) * amp + (Math.random() - .5) * amp) * 10) / 10);
  const mk = (id, game, exe, ago, secs, avg, low1, gpu, cpu, limit, cap) => ({
    id, game, exe, start: now - ago, secs, fpsAvg: avg, low1, low01: Math.round(low1 * 0.8), cpu: { avg: cpu, max: cpu + 21 }, maxCore: cpu + 30,
    gpu: { avg: gpu, max: 99 }, ram: { avg: 16.4, max: 18.2 }, vram: { avg: 6.1, max: 7.3 }, ramTotal: 32, vramTotal: 8,
    cpuTemp: { avg: 66, max: 78 }, gpuTemp: { avg: 67, max: 72 }, gpuName: "NVIDIA GeForce RTX 4060", limit, cap, turbo: true,
    series: { step: 5, fps: series(Math.min(400, secs / 5 | 0), avg, avg * .12), cpu: series(Math.min(400, secs / 5 | 0), cpu, 8), gpu: series(Math.min(400, secs / 5 | 0), gpu, 4), ram: series(Math.min(400, secs / 5 | 0), 16.4, .5) },
  });
  let sessions = [
    mk("1", "Cyberpunk 2077", "Cyberpunk2077.exe", 3600 * 20, 4310, 124.6, 88.1, 97, 46, "gpu"),
    mk("2", "Counter-Strike 2", "cs2.exe", 3600 * 44, 2905, 287.2, 151.9, 71, 58, "cpu"),
    mk("3", "Cyberpunk 2077", "Cyberpunk2077.exe", 3600 * 70, 3122, 118.3, 79.4, 98, 44, "gpu"),
    mk("4", "Hollow Knight", "hollow_knight.exe", 3600 * 96, 1830, 179.8, 176.2, 24, 12, "cap", 180),
    mk("5", "Fortnite", "FortniteClient-Win64-Shipping.exe", 3600 * 130, 5220, 162.4, 101.3, 86, 52, ""),
    mk("6", "Elden Ring", "eldenring.exe", 3600 * 200, 6700, 59.7, 51.2, 64, 38, "cap", 60),
  ];
  let procs = [
    { name: "Google Chrome", exe: "chrome.exe", count: 31, cpu: 3.2, memMB: 2140, canKill: true },
    { name: "Cyberpunk 2077", exe: "cyberpunk2077.exe", count: 1, cpu: 41.5, memMB: 9820, canKill: true },
    { name: "Discord", exe: "discord.exe", count: 7, cpu: 0.8, memMB: 612, canKill: true },
    { name: "Steam", exe: "steamwebhelper.exe", count: 9, cpu: 0.4, memMB: 488, canKill: true },
    { name: "Windows Explorer", exe: "explorer.exe", count: 1, cpu: 0.3, memMB: 151, canKill: false },
    { name: "Spotify", exe: "spotify.exe", count: 6, cpu: 0.6, memMB: 402, canKill: true },
    { name: "Antimalware Service Executable", exe: "msmpeng.exe", count: 1, cpu: 0.2, memMB: 260, canKill: false },
  ];
  const wait = (v, ms) => new Promise(r => setTimeout(() => r(JSON.parse(JSON.stringify(v))), ms || 60));
  window.go = { main: { App: {
    Live: () => wait(live(), 30),
    Settings: () => wait(settings),
    SaveSettings: s => { settings = s; return wait(settings); },
    SetOverlay: on => { settings.overlay.on = on; return wait(settings); },
    Purge: () => wait({ ok: true, freed: 6.3 * GB }, 500),
    Sessions: () => wait(sessions.map(s => Object.assign({}, s, { series: undefined }))),
    Session: id => wait(sessions.find(s => s.id === id)),
    DeleteSession: id => { sessions = sessions.filter(s => s.id !== id); return wait(null); },
    ClearSessions: () => { sessions = []; return wait(null); },
    Checks: () => wait(rep, 400),
    Fix: id => { rep.results.forEach(r => { if (r.fix === id) { r.status = "ok"; r.fix = ""; r.value = "Arreglado"; } }); let s = 100; rep.results.forEach(r => { if (r.status === "bad") s -= r.weight; if (r.status === "warn") s -= r.weight / 2; }); rep.score = Math.round(s); return wait(rep, 500); },
    Processes: () => wait(procs.map(p => Object.assign({}, p, { cpu: Math.max(0, p.cpu + (Math.random() - .5) * 2) }))),
    KillGroup: exe => { procs = procs.filter(p => p.exe !== exe); return wait(3); },
    EnableFPS: () => wait(null, 800),
    QuitAgent: () => wait(null),
    Version: () => wait("1.0.0"),
  } } };
  window.runtime = { WindowMinimise() {}, WindowToggleMaximise() {}, Quit() {}, BrowserOpenURL(u) { window.open(u); } };
})();
