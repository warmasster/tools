// Datos de muestra para ver la interfaz en un navegador (sin Wails).
// ?nofps  → como si no hubiera permiso para medir FPS
// ?nogame → sin juego abierto
// ?mal    → un PC con varias cosas mal configuradas
(function () {
  if (window.go && window.go.main) return;
  const q = new URLSearchParams(location.search);
  const noFPS = q.has("nofps"), noGame = q.has("nogame"), bad = q.has("mal");
  const stTimes = []; for (let t = 47; t < 2700; t += 60) stTimes.push(t); [130, 410, 415, 980, 1520, 2210].forEach(t => stTimes.push(t)); stTimes.sort((a, b) => a - b);
  const stutterData = {
    monitors: bad ? 2 : 1, elevated: !noFPS,
    live: noGame ? undefined : { game: "Cyberpunk 2077", start: Date.now() / 1000 - 1500 | 0, secs: 1500, fpsAvg: 98.4, low1: 61.2, stutter: { secs: 1490, hitches: 31, stutterSecs: 29, perMin: 1.2, worst: 48.6, times: stTimes.filter(t => t < 1490), period: bad ? 60 : 0, periodFit: 88, suspects: bad ? [{ kind: "proc", name: "iCUE (Corsair)", share: 76, lift: 14.2, hits: 22 }, { kind: "dpc", share: 21, lift: 3.1, hits: 6 }] : undefined, decode: bad ? { with: 3.8, without: 0.7, withSecs: 620 } : undefined } },
    sessions: [
      { id: "s1", game: "Counter-Strike 2", start: Date.now() / 1000 - 86400 | 0, secs: 2700, fpsAvg: 312, low1: 141, monitors: 2, stutter: { secs: 2690, hitches: 9, stutterSecs: 9, perMin: 0.2, worst: 22.1, times: [300, 912, 1400, 1401, 1800, 2002, 2400, 2500, 2610] } },
      { id: "s2", game: "Baldur's Gate 3", start: Date.now() / 1000 - 3 * 86400 | 0, secs: 5400, fpsAvg: 88, low1: 42, stutter: { secs: 5380, hitches: 260, stutterSecs: 230, perMin: 2.9, worst: 140.2, times: Array.from({ length: 230 }, (_, i) => Math.floor(i * 23.3 + (i % 7) * 3)), suspects: [{ kind: "disk", share: 64, lift: 4.5, hits: 147 }] } },
    ],
  };
  let noiseSt = null;
  const labSt = {
    elevated: !noFPS,
    knobs: [{ id: "pcores", title: "Solo núcleos de rendimiento", desc: "Saca el juego de los núcleos de eficiencia (E), que son más lentos." }, { id: "nosmt", title: "Un hilo por núcleo", desc: "Como apagar el Hyper-Threading o el SMT, pero solo para el juego." }, { id: "nocore0", title: "Lejos del núcleo 0", desc: "Windows atiende casi todas las interrupciones en el primer núcleo: el juego deja de usarlo." }, { id: "high", title: "Prioridad alta", desc: "Windows le da el procesador al juego antes que a todo lo demás." }, { id: "gpuhigh", title: "Prioridad alta en la gráfica", desc: "La gráfica atiende antes al juego que al navegador, Discord o el escritorio." }],
    history: [
      { id: "l1", exe: "cyberpunk2077.exe", game: "Cyberpunk 2077", knob: "pcores", title: "Solo núcleos de rendimiento", start: Date.now() / 1000 - 7200 | 0, applied: false, result: { pairs: 14, verdict: "better", fps: { a: 96.4, b: 101.1, delta: 4.4, lo: 1.9, hi: 6.2, p: 0.002 }, low1: { a: 58.2, b: 63.9, delta: 9.1, p: 0.004 }, stutter: { a: 1.4, b: 0.9, delta: -0.5, p: 0.08 }, diffs: [6.2, 9.8, 3.1, 12.4, 7.7, -1.2, 10.3, 8.8, 5.5, 14.1, 9.2, 2.4, 11.6, 8.1] } },
      { id: "l2", exe: "cs2.exe", game: "Counter-Strike 2", knob: "high", title: "Prioridad alta", start: Date.now() / 1000 - 2 * 86400 | 0, result: { pairs: 14, verdict: "same", fps: { a: 311, b: 312.4, delta: 0.4, p: 0.62 }, low1: { a: 140.2, b: 139.5, delta: -0.5, p: 0.81 }, stutter: { a: 0.2, b: 0.2, delta: 0, p: 1 }, diffs: [1.2, -2.1, 0.4, -0.8, 2.6, -1.9, 0.3, 1.1, -0.6, -1.4, 0.9, 0.2, -2.3, 1.7] } },
    ],
  };
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
      R("intel_microcode", "Procesador", "Microcódigo del procesador (Intel 13.ª/14.ª gen.)", "warn", "0x123", "Tu procesador usa el chip «Raptor Lake»… actualizar la BIOS es lo que recomienda Intel.", { weight: 10, howTo: ["Tu placa es una Gigabyte B760M D3HP DDR4 con la BIOS F2 (05/09/2024)."] }),
      R("gpu_pcie", "Gráfica", "Ranura de la gráfica", "ok", "PCIe 4.0 x8", "Tu gráfica está hecha para 8 carriles y va con los 8.", { weight: 15 }),
      R("rebar", "Gráfica", "Resizable BAR", "ok", "Activado", "El procesador puede ver toda la memoria de la gráfica.", { weight: 5 }),
      R("net_cable", "Red", "Velocidad del cable de red", "ok", "1000 Mbps", "Juegas por cable a buena velocidad.", { weight: 6 }),
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
    mk("7", "Cyberpunk 2077", "Cyberpunk2077.exe", 3600 * 260, 2950, 112.4, 71.8, 98, 43, "gpu"),
    mk("8", "Cyberpunk 2077", "Cyberpunk2077.exe", 3600 * 300, 4100, 114.9, 76.0, 98, 45, "gpu"),
  ];
  sessions[3].gov = { secs: 1830, capSecs: 1610, avgCap: 3300, maxMHz: 4700, mhzOn: 3280, mhzOff: 4650, wattsOn: 38.5, wattsOff: 61.2, tempOn: 54, tempOff: 63 };
  sessions[0].stutter = { secs: 4310, hitches: 48, stutterSecs: 44, perMin: 0.7, worst: 61.2 };
  sessions[1].stutter = { secs: 2905, hitches: 9, stutterSecs: 9, perMin: 0.2, worst: 22.1 };
  sessions[2].stutter = { secs: 3122, hitches: 160, stutterSecs: 140, perMin: 3.1, worst: 98.4, period: 60, periodFit: 81, suspects: [{ kind: "proc", name: "iCUE (Corsair)", share: 71, lift: 9.8, hits: 99 }] };
  sessions[0].path = "C:\\Juegos\\Cyberpunk 2077\\bin\\x64\\Cyberpunk2077.exe";
  sessions[1].gpuDriver = "566.36"; sessions[0].gpuDriver = "572.16"; sessions[2].gpuDriver = "566.36"; sessions[6].gpuDriver = "566.36"; sessions[7].gpuDriver = "566.36";
  let procs = [
    { name: "Google Chrome", exe: "chrome.exe", count: 31, cpu: 3.2, memMB: 2140, canKill: true },
    { name: "Cyberpunk 2077", exe: "cyberpunk2077.exe", count: 1, cpu: 41.5, memMB: 9820, canKill: true },
    { name: "Discord", exe: "discord.exe", count: 7, cpu: 0.8, memMB: 612, canKill: true },
    { name: "Steam", exe: "steamwebhelper.exe", count: 9, cpu: 0.4, memMB: 488, canKill: true },
    { name: "Windows Explorer", exe: "explorer.exe", count: 1, cpu: 0.3, memMB: 151, canKill: false },
    { name: "Spotify", exe: "spotify.exe", count: 6, cpu: 0.6, memMB: 402, canKill: true },
    { name: "Antimalware Service Executable", exe: "msmpeng.exe", count: 1, cpu: 0.2, memMB: 260, canKill: false },
  ];
  const comps = {
    summary: "Intel Core i5-14400F · RTX 4060 8 GB · 32 GB DDR4-3200 · 1 TB NVMe",
    cpu: { name: "Intel(R) Core(TM) i5-14400F", vendor: "intel", cores: 10, threads: 16, pCores: 6, eCores: 4, baseMHz: 2496, maxMHz: 4700, l2KB: 9728, l3KB: 20480, l3Groups: 1, microcode: 0x123, family: 6, model: 183 },
    board: { maker: "Gigabyte", model: "B760M D3HP DDR4", biosVendor: "American Megatrends International, LLC.", biosVersion: "F2", biosDate: "05/09/2024", laptop: false },
    os: { name: "Windows 11 Home", version: "25H2", build: "26200.9711" },
    ramTotal: 32 * GB,
    ram: [{ sizeMB: 16384, type: "DDR4", speed: 2400, configured: 3200, rated: 3200, locator: "DDR4-A2", maker: "Kingston", part: "KF3200C16D4/16GX", voltage: 1350 },
      { sizeMB: 16384, type: "DDR4", speed: 2400, configured: 3200, rated: 3200, locator: "DDR4-B2", maker: "Kingston", part: "KF3200C16D4/16GX", voltage: 1350 }],
    gpus: [{ name: "NVIDIA GeForce RTX 4060", vendor: "nvidia", partner: "MSI", vram: 8 * GB, dedicated: true, outputs: 2, driverVersion: "32.0.16.1664", driverDate: "2026-08-24", linkGen: 4, linkWidth: 8, maxGen: 4, maxWidth: 8, rebar: 1, barMB: 8192 }],
    disks: [{ number: 0, model: "KIOXIA-EXCERIA SSD", maker: "Kioxia", bus: "NVMe", kind: "nvme", size: 1000204886016, letters: ["C:"], tempC: 34, health: { usedPct: 3, sparePct: 100, writtenTB: 18.4, readTB: 31.2, powerOnHours: 4120, powerCycles: 980, unsafeShutdowns: 22 } },
      { number: 1, model: "ST2000DM008-2FR102", maker: "Seagate", bus: "SATA", kind: "hdd", size: 2000398934016, letters: ["D:"], rpm: 7200, tempC: 38, maxMBps: 250, health: { usedPct: -1, powerOnHours: 15200, reallocated: 0, writtenTB: 0 } }],
    monitors: [{ device: "\\\\.\\DISPLAY1", name: "24G4", maker: "AOC", width: 1920, height: 1080, hz: 180, maxHz: 180, inches: 24, hdr: true, hdrOn: false, primary: true }, ...(bad ? [{ device: "\\\\.\\DISPLAY2", name: "VG248", maker: "ASUS", width: 1920, height: 1080, hz: 60, maxHz: 75, inches: 24, primary: false }] : []),
      { device: "\\\\.\\DISPLAY2", name: "PHL24E1N1100A", maker: "Philips", width: 1920, height: 1080, hz: 100, maxHz: 100, inches: 24, hdr: true, hdrOn: false }],
    network: [{ name: "Ethernet", model: "Realtek PCIe GbE Family Controller", kind: "cable", up: true, default: true, linkMbps: 1000 },
      { name: "Wi-Fi", model: "Realtek RTL8188EU Wireless LAN 802.11n USB 2.0", kind: "wifi", up: false }],
    peripherals: [{ kind: "raton", name: "2.4G Wireless Mouse", maker: "LXDDZ" }, { kind: "teclado", name: "HyperX Alloy Core RGB", maker: "HP" },
      { kind: "mando", name: "Twin USB Joystick" }, { kind: "audio", name: "NEWSKILL SOBEK 7.1 V2 HEADSET · Altavoces" }, { kind: "microfono", name: "NEWSKILL SOBEK 7.1 V2 HEADSET · Micrófono" }],
  };
  let diskTest = { running: false, results: {} };  const now0 = Date.now() / 1000 | 0;
  const crashes = {
    days: 60, crashes: 1, gpuResets: 5, reboots: 2, hwErrors: 0, otherApps: 7, unstable: true,
    advice: "Tu procesador es un Intel de 13.ª/14.ª generación con el chip «Raptor Lake» y tu BIOS no trae el microcódigo que lo protege. Con reinicios o errores como estos, lo primero es actualizar la BIOS (en «Revisión» tienes los pasos).",
    incidents: [
      { id: "reboot-1", time: now0 - 86400, kind: "reboot", level: "bad", title: "El PC se reinició de golpe", unstable: true, cause: "El PC se apagó o reinició sin pantallazo azul y sin avisar. Las causas típicas: un cuelgue total por inestabilidad, un corte de luz, la fuente de alimentación o el calor.", steps: ["Si fue un corte de luz o alguien lo apagó, olvídalo.", "Si tienes overclock o undervolt, quítalo y prueba unos días.", "Pasa la prueba de «¿Es estable mi PC?»."] },
      { id: "crash-2", time: now0 - 9 * 86400, kind: "crash", level: "bad", app: "Maneater", game: true, title: "Fallo del juego", module: "Maneater-Win64-Shipping.exe", code: "0xc0000005", cause: "El juego intentó usar memoria que no era suya. Si solo pasa con un juego, es del juego; si pasa con varios, puede ser la RAM o el procesador inestables.", steps: ["Verifica los archivos del juego y actualízalo."] },
      { id: "gpu-3", time: now0 - 14 * 86400, kind: "gpu", level: "bad", count: 2, title: "La gráfica dejó de responder", cause: "La gráfica se quedó colgada y Windows reinició su driver.", steps: ["Instala el último driver de la gráfica.", "Quita cualquier overclock o undervolt de la gráfica."] },
      { id: "power-4", time: now0 - 28 * 86400, kind: "power", level: "info", title: "Apagado con el botón", cause: "El PC se apagó manteniendo pulsado el botón de encendido." },
      { id: "gpu-5", time: now0 - 44 * 86400, kind: "gpu", level: "bad", count: 6, title: "La gráfica dejó de responder", cause: "La gráfica se quedó colgada y Windows reinició su driver.", steps: ["Instala el último driver de la gráfica."] },
    ],
  };
  let stab = null;
  const gp = [], ip = [];
  let bloatSt = null;
  let thefts = q.has("mal") ? [{ time: now0 - 3600 * 5, game: "Cyberpunk 2077", exe: "ravcpl64.exe", name: "Realtek HD Audio Manager", idleMs: 8000 }, { time: now0 - 86400 * 2, game: "Counter-Strike 2", exe: "ravcpl64.exe", name: "Realtek HD Audio Manager", idleMs: 12000 }, { time: now0 - 86400 * 6, game: "Cyberpunk 2077", exe: "steamwebhelper.exe", name: "Steam", idleMs: 30000 }] : [];
  let profiles = { "cyberpunk2077.exe": { name: "Cyberpunk 2077", lockCursor: true, borderless: false } };  const wait = (v, ms) => new Promise(r => setTimeout(() => r(JSON.parse(JSON.stringify(v))), ms || 60));
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
    Components: () => wait(comps, 200),
    StartDiskTest: d => { diskTest = { running: true, disk: d, phase: "write", pct: 0, results: diskTest.results }; const t0 = Date.now(); const tick = () => { const p = (Date.now() - t0) / 30; if (p < 100) { diskTest.phase = "write"; diskTest.pct = p; } else if (p < 200) { diskTest.phase = "read"; diskTest.pct = p - 100; } else { diskTest.running = false; diskTest.results[d] = d === 1 ? { read: 212, write: 198 } : { read: 1517, write: 1484 }; return; } setTimeout(tick, 100); }; tick(); return wait(null); },
    DiskTest: () => wait(diskTest),
    Crashes: () => wait(crashes, 300),
    Net: () => { const p = (b) => b + Math.random() * 4; gp.push(p(23)); ip.push(p(19)); if (gp.length > 60) { gp.shift(); ip.shift(); } return wait({ active: true, elevated: !noFPS, wifiOff: false, game: noGame ? undefined : "Cyberpunk 2077", server: noGame ? undefined : { addr: "185.50.104.12:27015", udp: true, noPing: false }, gamePing: { last: gp[gp.length - 1], avg: 24.1, p95: 31, jitter: 2.3, loss: 0, count: gp.length, hist: gp }, inetPing: { last: ip[ip.length - 1], avg: 20.2, p95: 24, jitter: 1.2, loss: 0, count: ip.length, hist: ip }, procs: [{ name: "Cyberpunk 2077", exe: "cyberpunk2077.exe", down: 42000, up: 18000, game: true }, ...(q.has("mal") ? [{ name: "Steam", exe: "steam.exe", down: 38e6, up: 120000 }] : []), { name: "Discord", exe: "discord.exe", down: 12000, up: 9000 }, { name: "Google Chrome", exe: "chrome.exe", down: 3500, up: 1200 }] }); },
    StartBloat: () => { const t0 = Date.now(); bloatSt = { running: true, phase: "idle", pct: 0 }; const tick = () => { const s = (Date.now() - t0) / 1000; bloatSt.live = 19 + Math.random() * 3 + (s > 6 ? (s < 14 ? 140 : 60) : 0); bloatSt.pct = Math.min(100, s / 22 * 100); bloatSt.phase = s < 6 ? "idle" : s < 14 ? "down" : "up"; if (s >= 22) { bloatSt = { running: false, phase: "done", pct: 100, idle: 19.4, down: 162.3, up: 71.0, downMbps: 287, upMbps: 48, grade: "C" }; return; } setTimeout(tick, 200); }; tick(); return wait(null); },
    Bloat: () => wait(bloatSt),
    Thefts: () => wait(thefts),
    ClearThefts: () => { thefts = []; return wait(null); },
    StartupInfo: exes => wait(Object.fromEntries(exes.map(e => [e, e === "ravcpl64.exe"]))),
    DisableStartup: () => wait(null, 300),
    Profiles: () => wait(profiles),
    SaveProfile: (exe, p) => { profiles[exe] = p; return wait(profiles); },
    Stutter: () => wait(stutterData),
    Noise: () => wait({ state: noiseSt }),
    StartNoise: () => { const t0 = Date.now(); const cores = Array.from({ length: 16 }, (_, i) => ({ id: i, phys: i < 12 ? i >> 1 : 6 + (i - 12), class: i < 12 ? 1 : 0 })); noiseSt = { running: true, pct: 0, current: 0, cores, deep: !noFPS, drivers: [] }; const tick = () => { const k = Math.floor((Date.now() - t0) / 350); for (let i = 0; i < Math.min(k, 16); i++) { const c = noiseSt.cores[i]; if (c.done) continue; c.done = true; c.stolen = i === 0 ? 4.81 : i === 1 ? 2.94 : i === 13 ? 1.56 : 0.05 + Math.random() * 0.3; c.maxUs = i === 1 ? 1340 : i === 0 ? 102 : 30 + Math.random() * 200; c.dpc = i === 0 ? 4.18 : 0.2; c.isr = i === 0 ? 1.49 : 0.1; c.level = c.stolen >= 3 || c.maxUs >= 2000 ? "bad" : c.stolen >= 1 || c.maxUs >= 500 ? "warn" : "ok"; if (i === 0) { c.worstDrv = "nvlddmkm.sys"; c.worstDrvUs = 1240; } } noiseSt.pct = Math.min(100, k / 16 * 100); noiseSt.current = Math.min(k, 15); if (k >= 16) { noiseSt.running = false; noiseSt.at = Date.now() / 1000 | 0; if (!noFPS) noiseSt.drivers = [{ file: "nvlddmkm.sys", desc: "Driver de la gráfica NVIDIA", count: 48210, totalMs: 212.4, maxUs: 1240, over500: 12, level: "warn" }, { file: "ndis.sys", desc: "Red (Windows)", count: 9312, totalMs: 31.2, maxUs: 286, level: "ok" }, { file: "storport.sys", desc: "Discos (Windows)", count: 5120, totalMs: 12.9, maxUs: 141, level: "ok" }, { file: "HDAudBus.sys", desc: "Audio de la placa", count: 2210, totalMs: 4.1, maxUs: 88, level: "ok" }]; else noiseSt.deepErr = "para saber qué driver es, hace falta el permiso de administrador"; return; } setTimeout(tick, 150); }; tick(); return wait({ state: noiseSt }); },    Prefetch: () => wait({ "cyberpunk2077.exe": { disk: "nvme", learned: 0 }, "bg3.exe": { disk: "hdd", learned: 1.3 * GB, files: 412, runs: 3, lastWarm: { at: Date.now() / 1000 - 3 * 86400 | 0, bytes: 1.3 * GB, secs: 14.2 } }, "cs2.exe": { disk: "ssd", learned: 0, learning: false } }),
    Lab: () => wait(labSt),
    LabAction: b => { if (b.action === "start") { const k = labSt.knobs.find(k => k.id === b.knob); if (noGame) labSt.pending = { knob: b.knob, title: k.title, pairs: 14 }; else { const t0 = Date.now(); labSt.run = { knob: b.knob, title: k.title, game: "Cyberpunk 2077", block: 1, blocks: 28, pct: 0, left: 644 }; const tick = () => { if (!labSt.run) return; const s = (Date.now() - t0) / 1000 * 40; labSt.run.pct = Math.min(100, s / 560 * 100); labSt.run.block = Math.min(28, 1 + Math.floor(s / 20)); labSt.run.left = Math.max(0, 644 - s); if (s >= 560) { labSt.history.unshift({ id: "x" + Date.now(), exe: "cyberpunk2077.exe", game: "Cyberpunk 2077", knob: b.knob, title: k.title, start: Date.now() / 1000 | 0, result: { pairs: 14, verdict: "same", fps: { a: 101.2, b: 101.6, delta: 0.3, p: 0.71 }, low1: { a: 64, b: 63.1, delta: -1.1, p: 0.6 }, stutter: { a: 1.1, b: 1.2, delta: 0.1, p: 0.8 }, diffs: [2.1, -1.4, 0.3, -3.2, 1.8, -0.6, 2.5, -2.2, 0.9, -1.1, 0.4, -0.2, 1.3, -1.9] } }); labSt.run = null; return; } setTimeout(tick, 250); }; tick(); } } else if (b.action === "stop") { labSt.run = null; labSt.pending = null; } else if (b.action === "apply") { const r = labSt.history.find(r => r.id === b.id); if (r) r.applied = b.on; } else if (b.action === "delete") { labSt.history = labSt.history.filter(r => r.id !== b.id); } return wait(labSt); },    Stability: () => wait({ state: stab }),
    StartStability: () => { const t0 = Date.now(); const th = 16; stab = { running: true, phase: "single", pct: 0, current: 0, ramMB: 2048, cores: Array.from({ length: th }, (_, i) => ({ logical: i, errors: 0, items: 0 })) }; const tick = () => { const s = (Date.now() - t0) / 1000; if (s < 16 * 0.6) { const c = Math.floor(s / 0.6); stab.current = c; for (let i = 0; i < c; i++) stab.cores[i].items = 1800; stab.pct = c / 16 * 60; } else if (s < 14) { stab.phase = "all"; stab.cores.forEach(c => c.items = 4200); stab.pct = 60 + (s - 9.6) / 4.4 * 25; } else if (s < 17) { stab.phase = "ram"; stab.pct = 85 + (s - 14) / 3 * 15; } else { stab.running = false; stab.phase = "done"; stab.finished = Date.now() / 1000 | 0; if (location.search.includes("mal")) stab.cores[7].errors = 3; return; } setTimeout(tick, 200); }; tick(); return wait({ state: stab }); },
    StopStability: () => { if (stab) { stab.running = false; stab.stopped = true; stab.phase = "done"; } return wait({ state: stab }); },
  } } };
  window.runtime = { WindowMinimise() {}, WindowToggleMaximise() {}, Quit() {}, BrowserOpenURL(u) { window.open(u); } };
})();
