(() => {
  "use strict";
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];

  // ---------- iconos ----------
  const P = {
    shrink: '<path d="m15 15 6 6"/><path d="m15 9 6-6"/><path d="M21 16v5h-5"/><path d="M21 8V3h-5"/><path d="M3 16v5h5"/><path d="m3 21 6-6"/><path d="M3 8V3h5"/><path d="M9 9 3 3"/>',
    convert: '<path d="m16 3 4 4-4 4"/><path d="M20 7H4"/><path d="m8 21-4-4 4-4"/><path d="M4 17h16"/>',
    send: '<path d="M14.5 21.5 22 2 2.5 9.5l8 3.5z"/><path d="M22 2 10.5 13"/>',
    phone: '<rect x="6" y="2" width="12" height="20" rx="3"/><path d="M11 18h2"/>',
    phoneDown: '<rect x="6" y="2" width="12" height="20" rx="3"/><path d="M12 7v6"/><path d="m9 10 3 3 3-3"/>',
    qr: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><path d="M14 14h3v3h-3zM20 14v.01M14 20h.01M17 20h4v-3"/>',
    shield: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>',
    shieldOk: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>',
    shieldX: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m14.5 9.5-5 5M9.5 9.5l5 5"/>',
    shieldQ: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="M12 8v4M12 16h.01"/>',
    lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    unlock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.9-1"/>',
    globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18"/>',
    folderPlus: '<path d="M12 10v6M9 13h6"/><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>',
    folder: '<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>',
    folderOpen: '<path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2"/>',
    file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
    film: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 3v18M17 3v18M3 7.5h4M3 12h18M3 16.5h4M17 7.5h4M17 16.5h4"/>',
    image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>',
    music: '<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
    doc: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h4"/>',
    archive: '<rect x="2" y="3" width="20" height="5" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8M10 12h4"/>',
    app: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M7 6.5h.01M10 6.5h.01"/>',
    drive: '<path d="M22 12H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11"/><path d="M6 16h.01M10 16h.01"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    alert: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4M12 17h.01"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>',
    copy: '<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
    ext: '<path d="M15 3h6v6M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    play: '<path d="M6 4v16l14-8z"/>',
    download: '<path d="M12 4v12"/><path d="m7 11 5 5 5-5"/><path d="M5 20h14"/>',
    upload: '<path d="M12 20V8"/><path d="m7 13 5-5 5 5"/><path d="M5 4h14"/>',
    trash: '<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
    move: '<path d="M5 9 2 12l3 3M9 5l3-3 3 3M15 19l-3 3-3-3M19 9l3 3-3 3M2 12h20M12 2v20"/>',
    eject: '<path d="M5 17h14M12 4 5 13h14z"/>',
    undo: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/>',
    bolt: '<path d="M13 2 4.5 13.5H12L11 22l8.5-11.5H12z"/>',
    cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    chat: '<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/>',
    key: '<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6M15.5 7.5l3 3L22 7l-3-3"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    link: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
    chev: '<path d="m9 18 6-6-6-6"/>',
    sparkle: '<path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/>',
    paste: '<rect x="8" y="2" width="8" height="4" rx="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/>',
    gauge: '<path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
    hash: '<path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18"/>',
    pen: '<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${P[n] || P.file}</svg>`;
  const chk = '<i><svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg></i>';

  const KIND = {
    video: { icon: "film", label: "Vídeo", color: "linear-gradient(135deg,#8b5cf6,#ec4899)" },
    audio: { icon: "music", label: "Audio", color: "linear-gradient(135deg,#f59e0b,#ef4444)" },
    image: { icon: "image", label: "Imagen", color: "linear-gradient(135deg,#06b6d4,#3b82f6)" },
    pdf: { icon: "doc", label: "PDF", color: "linear-gradient(135deg,#ef4444,#f97316)" },
    doc: { icon: "doc", label: "Documento", color: "linear-gradient(135deg,#3b82f6,#6366f1)" },
    exe: { icon: "app", label: "Programa", color: "linear-gradient(135deg,#64748b,#334155)" },
    archive: { icon: "archive", label: "Comprimido", color: "linear-gradient(135deg,#a16207,#ca8a04)" },
    text: { icon: "doc", label: "Texto", color: "linear-gradient(135deg,#64748b,#475569)" },
    folder: { icon: "folder", label: "Carpeta", color: "linear-gradient(135deg,#6366f1,#22d3ee)" },
    drive: { icon: "drive", label: "Unidad", color: "linear-gradient(135deg,#475569,#1e293b)" },
    other: { icon: "file", label: "Archivo", color: "linear-gradient(135deg,#475569,#334155)" },
  };
  const ACT = {
    home: { label: "", icon: "bolt", w: 440 },
    reduce: { label: "Reducir tamaño", icon: "shrink", w: 440 },
    convert: { label: "Convertir", icon: "convert", w: 440 },
    send: { label: "Enviar con TurboSEND", icon: "send", w: 420 },
    qr: { label: "Pasar al móvil", icon: "phone", w: 380 },
    receive: { label: "Recibir del móvil", icon: "phoneDown", w: 380 },
    safe: { label: "¿Es seguro?", icon: "shield", w: 420 },
    lock: { label: "¿Qué lo bloquea?", icon: "lock", w: 430 },
    origin: { label: "¿De dónde salió?", icon: "globe", w: 420 },
    folder: { label: "Nueva carpeta con esto", icon: "folderPlus", w: 400 },
  };

  // ---------- utilidades ----------
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const num = (n, d = 0) => Number(n).toLocaleString("es-ES", { maximumFractionDigits: d, minimumFractionDigits: d });
  function fmtSize(b) {
    if (b == null || isNaN(b)) return "—";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0, n = Math.abs(b);
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (b < 0 ? "−" : "") + (i === 0 ? n : num(n, n < 10 ? 2 : n < 100 ? 1 : 0)) + " " + u[i];
  }
  function fmtDur(s) {
    if (!s || s < 0) return "";
    s = Math.round(s);
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60;
    return h ? `${h}:${String(m).padStart(2, "0")}:${String(x).padStart(2, "0")}` : `${m}:${String(x).padStart(2, "0")}`;
  }
  function fmtEta(s) {
    if (s == null || !isFinite(s) || s < 0) return "calculando…";
    if (s < 5) return "unos segundos";
    if (s < 60) return `${Math.round(s / 5) * 5} s`;
    if (s < 3600) return `${Math.round(s / 60)} min`;
    return `${Math.floor(s / 3600)} h ${Math.round((s % 3600) / 60)} min`;
  }
  const plural = (n, a, b) => `${num(n)} ${n === 1 ? a : b}`;
  function fmtDate(t) {
    if (!t) return "";
    const d = new Date(typeof t === "number" ? t * 1000 : t);
    if (isNaN(d) || d.getFullYear() < 1980) return "";
    return d.toLocaleDateString("es-ES", { day: "numeric", month: "long", year: "numeric" }) + ", " + d.toLocaleTimeString("es-ES", { hour: "2-digit", minute: "2-digit" });
  }
  const debounce = (f, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => f(...a), ms); }; };
  const errText = (e) => String(e && e.message ? e.message : e || "Algo ha fallado").replace(/^Error:\s*/, "");

  // ---------- puente con Go ----------
  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const rt = window.runtime || { EventsOn() { }, EventsOff() { } };
  const api = hasGo ? window.go.main.App : mockApi();
  const on = (n, f) => rt.EventsOn(n, f);
  // Un único oyente por evento; cada vista pone su manejador y start() los limpia.
  const H = {}, subscribed = new Set();
  function sub(n, f) {
    H[n] = f;
    if (!subscribed.has(n)) { subscribed.add(n); rt.EventsOn(n, (d) => H[n] && H[n](d)); }
  }

  // ---------- estado ----------
  const S = { job: null, gen: -1, shown: false, view: "", width: 440, files: [], deps: {}, timers: [], busy: false };
  const body = $("#body"), foot = $("#foot");

  function clearTimers() { S.timers.forEach(clearInterval); S.timers = []; }
  const every = (f, ms) => { const t = setInterval(f, ms); S.timers.push(t); return t; };

  // Ajusta la altura de la ventana al contenido (y la enseña la primera vez).
  function fit(minH = 0) {
    requestAnimationFrame(() => requestAnimationFrame(() => {
      const tb = $(".tb").offsetHeight, ft = foot.hidden ? 0 : foot.offsetHeight;
      const v = body.firstElementChild;
      const cs = getComputedStyle(body);
      let h = tb + (v ? v.offsetHeight : 0) + parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom) + ft + 2;
      h = Math.max(h, minH, 180);
      h = Math.min(h, Math.max(420, screen.availHeight * 0.86 / (window.devicePixelRatio > 0 ? 1 : 1)));
      h = Math.ceil(h);
      if (!S.shown) {
        S.shown = true;
        api.Show(S.width, h);
      } else if (Math.abs(h - window.innerHeight) > 3) {
        api.Resize(S.width, h);
      }
    }));
  }

  function toast(text, kind = "ok") {
    const el = document.createElement("div");
    el.className = "toast " + kind;
    el.innerHTML = ic(kind === "ok" ? "check" : "alert") + `<span>${esc(text)}</span>`;
    $("#toasts").appendChild(el);
    setTimeout(() => { el.classList.add("out"); setTimeout(() => el.remove(), 250); }, 2600);
  }

  async function copy(text, btn) {
    try { await api.CopyText(text); } catch (_) { navigator.clipboard && navigator.clipboard.writeText(text); }
    if (btn) { btn.classList.add("done"); btn.innerHTML = ic("check"); setTimeout(() => { btn.classList.remove("done"); btn.innerHTML = ic("copy"); }, 1400); }
    else toast("Copiado");
  }

  function setFoot(html) {
    foot.hidden = !html;
    foot.innerHTML = html || "";
  }

  function setBody(html) {
    body.innerHTML = `<div class="view">${html}</div>`;
    body.scrollTop = 0;
  }

  // ---------- cabecera ----------
  const thumbCache = new Map();
  async function thumbURL(path, size = 96) {
    const k = path + "|" + size;
    if (thumbCache.has(k)) return thumbCache.get(k);
    const p = (async () => { try { return await api.Thumb(path, size); } catch (_) { return ""; } })();
    thumbCache.set(k, p);
    return p;
  }

  function fileTile(kind) {
    const k = KIND[kind] || KIND.other;
    return `<div class="tile" style="--tile:${k.color}">${ic(k.icon)}</div>`;
  }

  function setHeader(job, extraSub) {
    const a = ACT[job.action] || ACT.home;
    $("#act-badge").innerHTML = a.label ? ic(a.icon) + esc(a.label) : "";
    $("#act-badge").hidden = !a.label;
    const files = job.files || [];
    const th = $("#thumb");
    if (job.action === "home" || !files.length) {
      th.innerHTML = '<img src="logo.svg" alt="" class="icon">';
      $("#tb-title").textContent = "TurboMENU";
      $("#tb-sub").textContent = extraSub || "";
      return;
    }
    const f = files[0];
    th.innerHTML = fileTile(files.length > 1 ? "other" : f.kind);
    if (files.length === 1 || job.action === "receive") {
      $("#tb-title").textContent = f.name;
      thumbURL(f.path, 96).then((u) => {
        if (!u || S.job !== job) return;
        const isIcon = !["image", "video", "pdf"].includes(f.kind);
        th.innerHTML = `<img src="${u}" alt="" class="${isIcon ? "icon" : ""}">`;
      });
    } else {
      $("#tb-title").textContent = plural(files.length, "elemento", "elementos");
      const kinds = new Set(files.map((x) => x.kind));
      if (kinds.size === 1) th.innerHTML = fileTile(f.kind);
    }
    let sub = extraSub;
    if (sub == null) {
      if (files.length === 1) {
        const k = KIND[f.kind] || KIND.other;
        sub = f.isDir ? k.label : `${k.label}${f.ext ? " " + f.ext.toUpperCase() : ""} · <b>${fmtSize(f.size)}</b>`;
        if (f.err) sub = `<span style="color:#fca5a5">${esc(f.err)}</span>`;
      } else {
        sub = `${job.total ? "<b>" + fmtSize(job.total) + "</b> en total · " : ""}${esc(files.slice(0, 3).map((x) => x.name).join(", "))}${files.length > 3 ? "…" : ""}`;
      }
    }
    $("#tb-sub").innerHTML = sub;
  }

  // ---------- arranque de un trabajo ----------
  async function start(job) {
    clearTimers();
    for (const k in H) H[k] = null;
    S.job = job;
    S.gen = job.gen;
    S.deps = job.deps || {};
    S.view = job.action;
    S.busy = false;
    S.width = (ACT[job.action] || ACT.home).w;
    $("#wb-min").hidden = true;
    setFoot("");
    setHeader(job);
    const r = R[job.action] || R.home;
    try { await r(job); } catch (e) { fail(errText(e)); }
  }

  function fail(msg, retry) {
    setBody(`<div class="needbox"><div class="tile" style="--tile:linear-gradient(135deg,#f43f5e,#be123c);--tile-sh:rgba(244,63,94,.6)">${ic("alert")}</div><h3>No se ha podido</h3><p>${esc(msg)}</p></div>`);
    setFoot(`<span class="grow"></span>${retry ? '<button class="btn ghost" id="f-retry">Reintentar</button>' : ""}<button class="btn primary" id="f-close">Cerrar</button>`);
    $("#f-close").onclick = close;
    if (retry) $("#f-retry").onclick = retry;
    fit();
  }

  function close() { api.Close(); }

  // Rutas que llegan tarde (selección múltiple desde el menú clásico).
  on("paths", (job) => {
    if (!S.job || job.gen !== S.gen) return;
    if (S.busy) { S.job.files = job.files; return; }
    start(job);
  });
  on("job", (job) => { S.shown = false; start(job); });
  on("reset", () => { clearTimers(); S.job = null; S.gen = -1; S.shown = false; body.innerHTML = ""; setFoot(""); });

  // ========================================================================
  // Vistas
  // ========================================================================
  const R = {};

  // ---------- inicio ----------
  R.home = async () => {
    const info = await api.HomeInfo();
    const d = info.deps || {};
    const feats = [["shrink", "Reducir tamaño"], ["convert", "Convertir"], ["send", "Enviar con TurboSEND"], ["phone", "Pasar al móvil"], ["shield", "¿Es seguro?"], ["lock", "¿Qué lo bloquea?"], ["globe", "¿De dónde salió?"], ["folderPlus", "Nueva carpeta con esto"], ["paste", "Pegar como archivo"], ["phoneDown", "Recibir del móvil"]];
    const dep = (icon, ok, title, sub, btn) => `<div class="dep"><div class="di ${ok ? "ok" : ""}">${ic(ok ? "check" : icon)}</div><div><b>${title}</b><span>${sub}</span></div>${btn || ""}</div>`;
    setHeader({ action: "home" }, "Versión " + esc(info.version));
    setBody(`
      <div class="hero"><img src="logo.svg" alt=""><div><h1>Turbo<b>MENU</b></h1><p>Acciones rápidas en el clic derecho de cualquier archivo o carpeta.</p></div></div>
      <div class="status ${info.installed ? "ok" : "no"}">
        <div class="tile sm" style="${info.installed ? "--tile:linear-gradient(135deg,#10b981,#0ea5e9)" : ""}">${ic(info.installed ? "check" : "bolt")}</div>
        <div class="st"><b>${info.installed ? "Está en tu clic derecho" : "Aún no está en el clic derecho"}</b><span>${info.installed ? (info.modern ? "Busca «Turbo» al pulsar con el botón derecho." : "En Windows 11 está en «Mostrar más opciones» (o Mayús + clic derecho).") : "Añádelo para usarlo desde el Explorador."}</span></div>
        <button class="btn ${info.installed ? "ghost sm" : "primary sm"}" id="h-menu">${info.installed ? "Quitar" : "Añadir"}</button>
      </div>
      <div class="card" style="margin-top:8px"><div class="dep" style="grid-template-columns:30px minmax(0,1fr) auto"><div class="di">${ic("sparkle")}</div><div><b>Menú nuevo de Windows 11 <span class="tagk">beta</span></b><span>Que «Turbo» salga nada más pulsar con el botón derecho. Crea un certificado propio de TurboMENU en el que solo confía tu usuario.</span></div><label class="toggle"><input type="checkbox" id="h-mod" ${info.modern ? "checked" : ""}><i></i></label></div></div>
      <div class="label">Qué puedes hacer<span class="gl"></span></div>
      <div class="feat">${feats.map(([i, t]) => `<div>${ic(i)}<span>${t}</span></div>`).join("")}</div>
      <div class="label">Lo que usa por detrás<span class="gl"></span></div>
      <div class="card">
        ${dep("film", d.ffmpeg, "Motor de vídeo y audio", d.ffmpeg ? "Listo para reducir y convertir vídeos y audios" : "Se descarga una vez (unos 90 MB) al usarlo por primera vez", d.ffmpeg ? "" : '<button class="btn ghost sm" id="h-ff">Descargar</button>')}
        ${dep("send", d.turbosend, "TurboSEND", d.turbosend ? "Instalado: puedes enviar desde el clic derecho" : d.turbosendOld ? "Tu versión es antigua: actualízala para enviar desde el clic derecho" : "Necesario para «Enviar con TurboSEND»", d.turbosend ? "" : `<button class="btn ghost sm" id="h-ts">${d.turbosendOld ? "Actualizar" : "Instalar"}</button>`)}
        ${dep("shield", d.virustotal, "VirusTotal", d.virustotal ? "Clave guardada: comprueba con decenas de antivirus" : "Opcional: con una clave gratuita, «¿Es seguro?» consulta 70 antivirus", d.virustotal ? '<button class="btn ghost sm" id="h-vt">Quitar</button>' : '<button class="btn ghost sm" id="h-vt">Añadir</button>')}
        ${dep("doc", d.office, "Office o LibreOffice", d.office ? esc(d.office) + ": puedes pasar documentos a PDF" : "Necesario para pasar documentos de Word, Excel o PowerPoint a PDF", "")}
      </div>
      <div class="label">Rapidez<span class="gl"></span></div>
      <div class="card"><div class="dep" style="grid-template-columns:30px minmax(0,1fr) auto"><div class="di">${ic("bolt")}</div><div><b>Abrir al instante</b><span>Deja TurboMENU preparado al iniciar sesión (usa algo de memoria).</span></div><label class="toggle"><input type="checkbox" id="h-pre" ${info.prewarm ? "checked" : ""}><i></i></label></div></div>
    `);
    $("#h-menu").onclick = async () => {
      try {
        if (info.installed) await api.UninstallMenu(); else await api.InstallMenu();
        toast(info.installed ? "Quitado del clic derecho" : "Añadido al clic derecho");
        R.home();
      } catch (e) { toast(errText(e), "error"); }
    };
    const ff = $("#h-ff");
    if (ff) ff.onclick = () => downloadFFmpeg(ff).then(() => R.home());
    const ts = $("#h-ts");
    if (ts) ts.onclick = () => api.OpenTurboSETUP();
    $("#h-vt").onclick = () => d.virustotal ? api.SetVTKey("").then(() => R.home()) : vtKeyDialog(() => R.home());
    $("#h-pre").onchange = (e) => api.SetPrewarm(e.target.checked);
    $("#h-mod").onchange = async (e) => {
      const on = e.target.checked;
      e.target.disabled = true;
      toast(on ? "Añadiendo al menú nuevo… (unos segundos)" : "Quitando del menú nuevo…");
      try { await api.SetModernMenu(on); toast(on ? "Listo: pulsa con el botón derecho en un archivo" : "Quitado del menú nuevo"); }
      catch (err) { toast(errText(err), "error"); }
      R.home();
    };
    setFoot("");
    fit();
  };

  async function downloadFFmpeg(btn, onProgress) {
    if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>Descargando…'; }
    sub("ffmpeg-dl", (p) => {
      const pc = p.total ? Math.round((p.done / p.total) * 100) : 0;
      if (btn) btn.innerHTML = `<span class="spinner"></span>${pc} %`;
      onProgress && onProgress(p);
    });
    try {
      await api.DownloadFFmpeg();
      S.deps.ffmpeg = "ok";
      toast("Motor de vídeo listo");
    } catch (e) {
      toast(errText(e), "error");
      if (btn) { btn.disabled = false; btn.innerHTML = "Reintentar"; }
      throw e;
    } finally { H["ffmpeg-dl"] = null; }
  }

  function needFFmpeg(again) {
    setBody(`<div class="needbox">
      <div class="tile" style="--tile:${KIND.video.color}">${ic("film")}</div>
      <h3>Falta el motor de vídeo</h3>
      <p>Para trabajar con vídeo y audio, TurboMENU necesita FFmpeg. Se descarga una sola vez (unos 90 MB) de su página oficial y se queda guardado.</p>
      <div class="pbar" id="ff-bar" hidden><i></i></div>
    </div>`);
    setFoot(`<button class="btn ghost" id="f-close">Cancelar</button><span class="grow"></span><button class="btn primary" id="ff-go">${ic("download")}Descargar y seguir</button>`);
    $("#f-close").onclick = close;
    $("#ff-go").onclick = async () => {
      const bar = $("#ff-bar");
      bar.hidden = false;
      try {
        await downloadFFmpeg($("#ff-go"), (p) => { bar.firstElementChild.style.width = (p.total ? (p.done / p.total) * 100 : 5) + "%"; });
        again();
      } catch (_) { }
    };
    fit();
  }

  function needOffice() {
    setBody(`<div class="needbox">
      <div class="tile" style="--tile:${KIND.doc.color}">${ic("doc")}</div>
      <h3>Hace falta Office o LibreOffice</h3>
      <p>Para pasar documentos de Word, Excel o PowerPoint a PDF hace falta tener instalado Microsoft Office o LibreOffice (gratis). No se ha encontrado ninguno en este PC.</p>
    </div>`);
    setFoot(`<span class="grow"></span><button class="btn ghost" id="f-lo">${ic("ext")}Descargar LibreOffice</button><button class="btn primary" id="f-close">Cerrar</button>`);
    $("#f-close").onclick = close;
    $("#f-lo").onclick = () => api.OpenURL("https://es.libreoffice.org/descarga/libreoffice/");
    fit();
  }

  function needTurboSEND() {
    const old = S.deps.turbosendOld;
    setBody(`<div class="needbox">
      <div class="tile">${ic("send")}</div>
      <h3>${old ? "Hay que actualizar TurboSEND" : "TurboSEND no está instalado"}</h3>
      <p>${old ? "Tu TurboSEND es de una versión anterior que aún no sabe recibir envíos desde el clic derecho. Actualízalo con TurboSETUP y vuelve a probar." : "«Enviar con TurboSEND» usa TurboSEND por detrás para mandar archivos de cualquier tamaño por un enlace. Instálalo con TurboSETUP y vuelve a probar."}</p>
    </div>`);
    setFoot(`<button class="btn ghost" id="f-close">Cerrar</button><span class="grow"></span><button class="btn primary" id="f-ts">${ic("download")}${old ? "Actualizar" : "Instalar"} TurboSEND</button>`);
    $("#f-close").onclick = close;
    $("#f-ts").onclick = () => { api.OpenTurboSETUP(); close(); };
    fit();
  }

  // ---------- reducir y convertir ----------
  R.reduce = (job) => runPlan(job, "reduce");
  R.convert = (job) => runPlan(job, "convert");

  async function runPlan(job, mode) {
    setBody(`<div class="prog"><div class="pbar indet"><i></i></div></div>`);
    const plan = await api.Plan(mode);
    if (S.job !== job) return;
    if (plan.error) return fail(plan.error);
    if (plan.info) setHeader(job, plan.info);
    if (plan.needs === "ffmpeg" && !S.deps.ffmpeg) return needFFmpeg(() => runPlan(job, mode));
    if (plan.needs === "office" && !S.deps.office) return needOffice();
    S.plan = plan;
    S.sel = { replace: false };
    if (mode === "reduce") renderReduce(job, plan); else renderConvert(job, plan);
  }

  function optCard(o, selected) {
    const iconHtml = o.brand ? `<span class="opt-ic brand" style="--oc:${o.color}">${esc(o.brand)}</span>` : o.icon ? `<span class="opt-ic" style="--oc:${o.color || "rgba(255,255,255,.1)"}">${ic(o.icon)}</span>` : "";
    return `<button class="opt ${selected ? "on" : ""} ${o.disabled ? "dis" : ""}" data-id="${esc(o.id)}" ${o.disabled ? 'title="' + esc(o.needs || "") + '"' : ""}>
      ${o.rec ? '<span class="rec">Recomendado</span>' : ""}
      <span class="opt-top">${iconHtml}<b>${esc(o.title)}</b></span>
      ${o.sub || o.est ? `<span class="opt-sub"><span class="os">${esc(o.sub || "")}</span>${o.est ? `<span class="est">≈ ${fmtSize(o.est)}</span>` : ""}</span>` : ""}
      ${o.disabled && o.needs ? `<span class="needs">${esc(o.needs)}</span>` : ""}
    </button>`;
  }

  function renderReduce(job, plan) {
    const sel = S.sel;
    const orig = plan.origSize || job.total;
    let html = "";
    if (plan.kind === "image") {
      sel.quality = plan.image.quality;
      sel.scale = plan.image.scales[0].id;
      const single = job.files.length === 1;
      html += single ? `
        <div class="compare" id="cmp"><img id="cmp-b" alt=""><div class="after-wrap"><img id="cmp-a" alt=""></div><div class="divider"></div><span class="tag l">Antes</span><span class="tag r">Después</span><span class="loading" id="cmp-l"><span class="spinner"></span></span></div>
        <div class="size-line"><span>Quedará en <b id="est">…</b></span><span class="pct" id="est-pct"></span></div>` : `<div class="note">${ic("image")}<div class="nt">Se aplicará a las <b>${job.files.length} imágenes</b> la misma calidad y tamaño.</div></div>`;
      html += `<div class="label">Calidad<span class="gl"></span></div>
        <div class="qrow"><input type="range" class="range" id="q" min="10" max="100" value="${sel.quality}"><span class="qv"><b id="qv">${sel.quality}</b> %</span></div>
        <div class="label">Tamaño de la imagen<span class="gl"></span></div>
        <div class="seg" id="scales">${plan.image.scales.map((s, i) => `<button data-id="${s.id}" class="${i === 0 ? "on" : ""}">${esc(s.title)}${s.sub ? `<small>${esc(s.sub)}</small>` : ""}</button>`).join("")}</div>`;
    } else if (plan.kind === "video" || plan.kind === "audio") {
      const presets = plan.presets || [];
      const quals = plan.qualities || [];
      const last = (presets.find((p) => p.rec) || presets[0] || quals[1] || quals[0] || {}).id;
      sel.preset = last;
      if (presets.length) html += `<div class="label">Para compartir<span class="gl"></span></div><div class="opts">${presets.map((p) => optCard(p, p.id === sel.preset)).join("")}</div>`;
      if (quals.length) html += `<div class="label">Calidad<span class="gl"></span></div><div class="seg" id="quals">${quals.map((q) => `<button data-id="${q.id}" class="${q.id === sel.preset ? "on" : ""}">${esc(q.title)}<small>${q.est ? "≈ " + fmtSize(q.est) : ""}</small></button>`).join("")}</div>`;
      if (plan.custom) html += `<div class="label">O el peso exacto que quieres<span class="gl"></span></div>
        <div class="field" id="cust-f"><input id="cust" type="number" min="1" step="1" placeholder="Por ejemplo, 8" inputmode="decimal"><span class="unit">MB</span></div>`;
    } else {
      sel.preset = ((plan.presets || []).find((p) => p.rec) || (plan.presets || [])[0] || {}).id;
      html += `<div class="opts one">${(plan.presets || []).map((p) => optCard(p, p.id === sel.preset)).join("")}</div>`;
    }
    if (plan.notes) html += plan.notes.map((n) => `<div class="note ${n.kind || ""}">${ic(n.kind === "warn" ? "alert" : "info")}<div class="nt">${esc(n.text)}</div></div>`).join("");
    setBody(html);
    const canReplace = plan.kind !== "zip";
    setFoot(`${canReplace ? `<label class="chk" title="Si lo marcas, el original se sustituye por la versión reducida"><input type="checkbox" id="repl">${chk}Reemplazar el original</label>` : ""}<span class="grow"></span><button class="btn primary" id="go">${ic(plan.kind === "zip" ? "archive" : "shrink")}${plan.kind === "zip" ? "Comprimir" : "Reducir"}</button>`);
    const repl = $("#repl");
    if (repl) repl.onchange = () => (sel.replace = repl.checked);

    // selección de opciones
    const pick = (id) => {
      sel.preset = id; sel.targetMB = 0;
      $$(".opt").forEach((b) => b.classList.toggle("on", b.dataset.id === id));
      $$("#quals button").forEach((b) => b.classList.toggle("on", b.dataset.id === id));
      const c = $("#cust"); if (c) c.value = "";
    };
    $$(".opt:not(.dis)").forEach((b) => (b.onclick = () => pick(b.dataset.id)));
    $$("#quals button").forEach((b) => (b.onclick = () => pick(b.dataset.id)));
    // Estimaciones más finas que llegan después (se codifica una muestra).
    sub("plan-est", (e) => { const b = $(`#quals button[data-id="${e.id}"] small`); if (b) b.textContent = "≈ " + fmtSize(e.est); });
    const cust = $("#cust");
    if (cust) cust.oninput = () => {
      const v = parseFloat(String(cust.value).replace(",", "."));
      if (v > 0) { sel.preset = "target"; sel.targetMB = v; $$(".opt,#quals button").forEach((b) => b.classList.remove("on")); }
      $("#cust-f").classList.toggle("err", v > 0 && orig && v * 1048576 >= orig);
    };

    if (plan.kind === "image") {
      const q = $("#q"), qv = $("#qv");
      const paint = () => q.style.setProperty("--p", ((q.value - 10) / 90) * 100 + "%");
      paint();
      const prev = debounce(() => preview(job, sel), 90);
      q.oninput = () => { sel.quality = +q.value; qv.textContent = q.value; paint(); prev(); };
      $$("#scales button").forEach((b) => (b.onclick = () => { $$("#scales button").forEach((x) => x.classList.toggle("on", x === b)); sel.scale = b.dataset.id; prev(); }));
      const cmp = $("#cmp");
      if (cmp) {
        const move = (e) => { const r = cmp.getBoundingClientRect(); cmp.style.setProperty("--x", Math.max(0, Math.min(100, ((e.clientX - r.left) / r.width) * 100)) + "%"); };
        cmp.addEventListener("pointerdown", (e) => { cmp.setPointerCapture(e.pointerId); move(e); });
        cmp.addEventListener("pointermove", (e) => { if (e.buttons) move(e); });
        preview(job, sel);
      }
    }
    $("#go").onclick = () => execute(job, "reduce", sel);
    fit();
  }

  let prevSeq = 0;
  async function preview(job, sel) {
    const seq = ++prevSeq;
    const l = $("#cmp-l"); if (l) l.hidden = false;
    try {
      const r = await api.ImagePreview(job.files[0].path, sel.quality, sel.scale);
      if (seq !== prevSeq || S.job !== job) return;
      if (r.before && $("#cmp-b").src !== r.before) $("#cmp-b").src = r.before;
      $("#cmp-a").src = r.after;
      $("#est").textContent = fmtSize(r.size);
      const pct = r.origSize ? Math.round((1 - r.size / r.origSize) * 100) : 0;
      const ep = $("#est-pct");
      ep.textContent = pct > 0 ? `−${pct} %` : pct < 0 ? `+${-pct} % (pesaría más)` : "igual";
      ep.style.color = pct > 0 ? "" : "var(--warn)";
    } catch (e) {
      if (seq === prevSeq) $("#est").textContent = "—";
    } finally { if (seq === prevSeq && l) l.hidden = true; }
  }

  function renderConvert(job, plan) {
    const sel = S.sel;
    const targets = plan.targets || [];
    const groups = [];
    for (const t of targets) {
      let g = groups.find((x) => x.name === (t.group || ""));
      if (!g) groups.push((g = { name: t.group || "", items: [] }));
      g.items.push(t);
    }
    const first = targets.find((t) => !t.disabled);
    sel.target = first && first.id;
    let html = "";
    for (const g of groups) {
      html += `<div class="label">${esc(g.name || "Formatos")}<span class="gl"></span></div><div class="opts">`;
      html += g.items.map((t) => `<button class="opt fmt ${t.id === sel.target ? "on" : ""} ${t.disabled ? "dis" : ""}" data-id="${esc(t.id)}" title="${esc(t.desc)}">
        <span class="ext">${esc(t.ext)}</span><span class="fd"><b>${esc(t.name)}${t.recent ? ' <small style="color:#a5b4fc;font-weight:600">· reciente</small>' : ""}</b><span>${esc(t.disabled ? t.needs : t.desc)}</span></span></button>`).join("");
      html += `</div>`;
    }
    if (!targets.length) html = `<div class="needbox"><div class="tile" style="--tile:${KIND.other.color}">${ic("convert")}</div><h3>No hay conversiones para esto</h3><p>TurboMENU no sabe convertir este tipo de archivo a otro formato.</p></div>`;
    html += `<div id="gifopts"></div>`;
    setBody(html);
    setFoot(targets.length ? `<span class="hint" id="cv-desc"></span><span class="grow"></span><button class="btn primary" id="go">${ic("convert")}Convertir</button>` : `<span class="grow"></span><button class="btn primary" id="f-close">Cerrar</button>`);
    if (!targets.length) { $("#f-close").onclick = close; return fit(); }
    const pick = (id) => {
      sel.target = id;
      $$(".opt.fmt").forEach((b) => b.classList.toggle("on", b.dataset.id === id));
      const t = targets.find((x) => x.id === id);
      const g = $("#gifopts");
      if (t && t.gif) {
        sel.gifFps = sel.gifFps || 12; sel.gifWidth = sel.gifWidth || 480;
        g.innerHTML = `<div class="label">Ajustes del GIF<span class="gl"></span></div>
          <div class="seg" id="gw">${[320, 480, 640].map((w) => `<button data-v="${w}" class="${w === sel.gifWidth ? "on" : ""}">${w} px<small>${w === 320 ? "ligero" : w === 480 ? "normal" : "nítido"}</small></button>`).join("")}</div>
          <div class="seg" id="gf" style="margin-top:6px">${[8, 12, 15, 24].map((f) => `<button data-v="${f}" class="${f === sel.gifFps ? "on" : ""}">${f} fps</button>`).join("")}</div>`;
        $$("#gw button").forEach((b) => (b.onclick = () => { sel.gifWidth = +b.dataset.v; $$("#gw button").forEach((x) => x.classList.toggle("on", x === b)); }));
        $$("#gf button").forEach((b) => (b.onclick = () => { sel.gifFps = +b.dataset.v; $$("#gf button").forEach((x) => x.classList.toggle("on", x === b)); }));
      } else g.innerHTML = "";
      fit();
    };
    $$(".opt.fmt:not(.dis)").forEach((b) => { b.onclick = () => pick(b.dataset.id); b.ondblclick = () => { pick(b.dataset.id); execute(job, "convert", sel); }; });
    if (sel.target) pick(sel.target);
    $("#go").onclick = () => execute(job, "convert", sel);
    fit();
  }

  // ---------- ejecución con progreso y resultado ----------
  async function execute(job, mode, sel) {
    S.busy = true;
    $("#wb-min").hidden = false;
    const files = job.files;
    const multi = files.length > 1;
    setBody(`<div class="prog">
      <div class="prog-top"><b id="pg-t">${mode === "reduce" ? "Reduciendo…" : "Convirtiendo…"}</b><span id="pg-p">0 %</span></div>
      <div class="pbar" id="pg-bar"><i></i></div>
      <div class="prog-sub"><span id="pg-eta">Calculando el tiempo…</span><span id="pg-sp"></span></div>
    </div>${multi ? `<div class="items" id="pg-items">${files.map((f, i) => `<div class="item" data-i="${i}"><div class="ic">${ic((KIND[f.kind] || KIND.other).icon)}</div><div class="nm"><b>${esc(f.name)}</b><span>${fmtSize(f.size)}</span></div><span class="rt">En espera</span><div class="pbar sm"><i></i></div></div>`).join("")}</div>` : ""}`);
    setFoot(`<span class="grow"></span><button class="btn ghost" id="cancel">Cancelar</button>`);
    $("#cancel").onclick = () => { api.Cancel("job"); $("#cancel").disabled = true; $("#cancel").textContent = "Cancelando…"; };
    fit();
    sub("job-progress", (p) => {
      if (S.job !== job) return;
      const pc = Math.max(0, Math.min(100, Math.floor((p.overall || 0) * 100)));
      $("#pg-bar i").style.width = pc + "%";
      $("#pg-p").textContent = pc + " %";
      $("#pg-eta").textContent = p.eta != null && p.eta >= 0 ? `Quedan ${fmtEta(p.eta)}` : p.phase || "Calculando el tiempo…";
      $("#pg-sp").textContent = p.speed ? `${num(p.speed, 1)}× tiempo real` : multi ? `${p.index + 1} de ${files.length}` : "";
      if (multi) {
        $$("#pg-items .item").forEach((el) => {
          const i = +el.dataset.i;
          if (i === p.index) { el.querySelector(".rt").textContent = Math.floor((p.frac || 0) * 100) + " %"; el.querySelector(".pbar i").style.width = (p.frac || 0) * 100 + "%"; }
          else if (i < p.index && !el.classList.contains("ok") && !el.classList.contains("err")) { el.querySelector(".rt").textContent = "✓"; el.querySelector(".pbar i").style.width = "100%"; }
        });
      }
    });
    let res;
    try {
      res = await api.Run({ mode, preset: sel.preset || "", targetMB: sel.targetMB || 0, quality: sel.quality || 0, scale: sel.scale || "", target: sel.target || "", gifFps: sel.gifFps || 0, gifWidth: sel.gifWidth || 0, replace: !!sel.replace });
    } catch (e) {
      res = { error: errText(e) };
    } finally { H["job-progress"] = null; }
    S.busy = false;
    $("#wb-min").hidden = true;
    if (S.job !== job) return;
    showResult(job, mode, res);
  }

  function showResult(job, mode, res) {
    if (res.canceled) { toast("Cancelado", "error"); return start(S.job); }
    const results = res.results || [];
    const ok = results.filter((r) => !r.error && r.out);
    if (res.error || !ok.length) {
      const msg = res.error || (results[0] && results[0].error) || "No se pudo terminar";
      return fail(msg, () => start(S.job));
    }
    const before = ok.reduce((a, r) => a + (r.before || 0), 0), after = ok.reduce((a, r) => a + (r.after || 0), 0);
    const pct = before ? Math.round((1 - after / before) * 100) : 0;
    const one = ok.length === 1 && results.length === 1;
    const outs = ok.map((r) => r.out);
    const bigger = mode === "reduce" && after >= before;
    let head;
    if (mode === "reduce") {
      head = `<h2>${fmtSize(before)}<span class="arrow">→</span>${fmtSize(after)}${pct > 0 ? `<span class="pct">−${pct} %</span>` : `<span class="pct up">${pct === 0 ? "igual" : "+" + -pct + " %"}</span>`}</h2>`;
    } else {
      head = `<h2>${one ? esc(baseName(ok[0].out)) : plural(ok.length, "archivo convertido", "archivos convertidos")}</h2>`;
    }
    const sub = one ? (mode === "reduce" ? esc(baseName(ok[0].out)) : fmtSize(after)) : `${plural(ok.length, "archivo", "archivos")}${results.length > ok.length ? ` · ${results.length - ok.length} con errores` : ""}`;
    let list = "";
    if (!one) {
      list = `<div class="items">${results.map((r) => `<div class="item ${r.error ? "err" : "ok"}"><div class="ic">${ic(r.error ? "alert" : "check")}</div><div class="nm"><b>${esc(baseName(r.out || r.in))}</b><span>${esc(r.error || (mode === "reduce" ? `${fmtSize(r.before)} → ${fmtSize(r.after)}` : fmtSize(r.after)))}</span></div>${r.out ? `<span class="acts"><button class="ibtn" data-open="${esc(r.out)}" title="Abrir">${ic("play")}</button><button class="ibtn" data-reveal="${esc(r.out)}" title="Mostrar en la carpeta">${ic("folderOpen")}</button></span>` : ""}</div>`).join("")}</div>`;
    }
    const dts = S.deps.turbosend;
    setBody(`<div class="result"><div class="ring ${bigger ? "warn" : ""}">${ic(bigger ? "alert" : "check")}</div>${head}<p>${sub}</p>
      ${bigger ? `<div class="note warn" style="text-align:left">${ic("alert")}<div class="nt">El resultado no pesa menos que el original. Prueba con una calidad más baja.</div></div>` : ""}
      ${res.replaced ? `<div class="note" style="text-align:left">${ic("info")}<div class="nt">Se ha reemplazado el original.</div></div>` : ""}
      <div class="actions">
        ${one ? `<button class="btn ghost" id="r-open">${ic("play")}Abrir</button>` : ""}
        <button class="btn ghost ${one ? "" : "full"}" id="r-reveal">${ic("folderOpen")}Mostrar en la carpeta</button>
        <button class="btn ghost ${dts ? "" : "off"}" id="r-send" title="${dts ? "" : "Necesita TurboSEND"}">${ic("send")}Enviar con TurboSEND</button>
        <button class="btn ghost" id="r-qr">${ic("phone")}Pasar al móvil</button>
      </div>
      ${dts ? "" : '<p class="dimtxt" style="margin-top:8px">Para «Enviar con TurboSEND» hace falta tener TurboSEND instalado.</p>'}
    </div>${list}`);
    setFoot(`<span class="grow"></span><button class="btn primary" id="f-close">Hecho</button>`);
    $("#f-close").onclick = close;
    if (one) $("#r-open").onclick = () => api.Open(outs[0]);
    $("#r-reveal").onclick = () => api.Reveal(outs[0]);
    $("#r-send").onclick = () => chain("send", outs);
    $("#r-qr").onclick = () => chain("qr", outs);
    $$("[data-open]").forEach((b) => (b.onclick = () => api.Open(b.dataset.open)));
    $$("[data-reveal]").forEach((b) => (b.onclick = () => api.Reveal(b.dataset.reveal)));
    fit();
  }

  const baseName = (p) => String(p || "").split(/[\\/]/).pop();

  // Pasa los archivos resultantes a otra acción en la misma ventana.
  async function chain(action, paths) {
    const job = await api.SetJob(action, paths);
    start(job);
  }

  // ---------- enviar con TurboSEND ----------
  R.send = async (job) => {
    if (!S.deps.turbosend) return needTurboSEND();
    setBody(`<div class="prog"><div class="prog-top"><b>Conectando con TurboSEND…</b></div><div class="pbar indet"><i></i></div></div>`);
    fit();
    let rooms;
    try { rooms = await api.SendRooms(); } catch (e) { return fail(errText(e), () => start(S.job)); }
    if (S.job !== job) return;
    const sel = { room: "" };
    const big = job.total > 2 * 1073741824 && job.files.some((f) => f.kind === "video");
    const initials = (n) => (String(n).trim().split(/\s+/).map((w) => w[0]).join("").slice(0, 2) || "?").toUpperCase();
    setBody(`
      ${big ? `<div class="note warn" style="margin-top:0">${ic("alert")}<div class="nt"><b>Es bastante grande (${fmtSize(job.total)}).</b> Si quieres que llegue antes, redúcelo primero.<br><button class="btn ghost sm" id="s-red">${ic("shrink")}Reducir antes</button></div></div>` : ""}
      <div class="label">¿A quién se lo mandas?<span class="gl"></span></div>
      <div class="rooms" id="rooms">
        <button class="room on" data-id=""><span class="av new">${ic("plus")}</span><span><b>Chat nuevo</b><span>Crea un enlace nuevo para compartirlo</span></span><span></span></button>
        ${rooms.slice(0, 8).map((r) => `<button class="room" data-id="${esc(r.id)}"><span class="av">${esc(initials(r.name))}</span><span><b>${esc(r.name)}</b><span>${esc(r.preview || (r.members ? plural(r.members, "persona", "personas") : "Sin nadie todavía"))}</span></span>${r.online ? `<span class="on-dot">● ${r.online} en línea</span>` : "<span></span>"}</button>`).join("")}
      </div>
      <div id="newname" style="margin-top:10px"><div class="field"><input id="s-name" placeholder="Nombre del chat (opcional)" maxlength="40"></div></div>
    `);
    setFoot(`<span class="hint">${job.files.some((f) => f.isDir) ? "Las carpetas se envían con todo lo de dentro." : ""}</span><span class="grow"></span><button class="btn primary" id="go">${ic("send")}Enviar</button>`);
    $$(".room").forEach((b) => (b.onclick = () => { sel.room = b.dataset.id; $$(".room").forEach((x) => x.classList.toggle("on", x === b)); $("#newname").hidden = !!sel.room; fit(); }));
    const red = $("#s-red"); if (red) red.onclick = () => chain("reduce", job.files.map((f) => f.path));
    $("#go").onclick = async () => {
      $("#go").disabled = true; $("#go").innerHTML = '<span class="spinner"></span>Enviando…';
      try {
        const r = await api.SendTo(sel.room, sel.room ? "" : $("#s-name").value.trim(), job.files.map((f) => f.path));
        sendWatch(job, r);
      } catch (e) { toast(errText(e), "error"); $("#go").disabled = false; $("#go").innerHTML = ic("send") + "Enviar"; }
    };
    fit();
  };

  function sendWatch(job, r) {
    S.busy = true;
    setBody(`<div class="result" style="padding-bottom:0"><div class="ring">${ic("check")}</div><h2 id="sw-t">Listo para descargar</h2><p id="sw-s">Comparte este enlace. Quien lo abra podrá descargarlo desde el navegador.</p></div>
      <div id="sw-links" style="margin-top:14px"><div class="pbar indet"><i></i></div></div>
      <div class="items" id="sw-tr"></div>
      <div class="note">${ic("info")}<div class="nt">TurboSEND se queda abierto (minimizado) para que puedan descargarlo. Si lo cierras, el enlace deja de funcionar.</div></div>`);
    setFoot(`<button class="btn ghost" id="sw-open">${ic("chat")}Ver el chat</button><span class="grow"></span><button class="btn primary" id="f-close">Hecho</button>`);
    $("#f-close").onclick = close;
    $("#sw-open").onclick = () => api.ShowTurboSEND(r.room);
    let shown = "";
    const tick = async () => {
      let st;
      try { st = await api.SendStatus(r.room, r.bundle); } catch (e) { $("#sw-s").textContent = errText(e); return; }
      if (S.job !== job) return;
      const L = st.links || {};
      const key = [L.cloudflare, L.direct].join("|");
      if (key !== shown) {
        shown = key;
        const rows = [];
        if (L.cloudflare) rows.push(["cloud", "Enlace seguro (Cloudflare)", L.cloudflare]);
        if (L.direct) rows.push(["bolt", "Directo por tu IP (más rápido)", L.direct]);
        $("#sw-links").innerHTML = rows.length ? rows.map(([i, t, u]) => `<div class="label">${ic(i)}${t}<span class="gl"></span></div><div class="linkrow"><code>${esc(u)}</code><button class="ibtn" data-copy="${esc(u)}" title="Copiar">${ic("copy")}</button></div>`).join("")
          : `<div class="prog-top"><span>Preparando el enlace…</span></div><div class="pbar indet"><i></i></div>`;
        $$("[data-copy]").forEach((b) => (b.onclick = () => copy(b.dataset.copy, b)));
        if (rows.length && !S.copiedOnce) { S.copiedOnce = true; copy(rows[0][2]); toast("Enlace copiado: pégalo en el chat"); }
        fit();
      }
      const tr = st.transfers || [];
      const dl = (st.bundle && st.bundle.downloads) || 0;
      $("#sw-t").textContent = tr.length ? "Descargando…" : dl ? `Descargado ${plural(dl, "vez", "veces")}` : "Listo para descargar";
      $("#sw-tr").innerHTML = tr.map((t) => `<div class="item"><div class="ic">${ic("download")}</div><div class="nm"><b>${esc(t.who || "Alguien")}</b><span>${fmtSize(t.bytes)} de ${fmtSize(t.total)}${t.speed ? " · " + fmtSize(t.speed) + "/s" : ""}</span></div><span class="rt">${t.total ? Math.floor((t.bytes / t.total) * 100) : 0} %</span><div class="pbar sm"><i style="width:${t.total ? (t.bytes / t.total) * 100 : 0}%"></i></div></div>`).join("");
    };
    S.copiedOnce = false;
    tick();
    every(tick, 1000);
    fit();
  }

  // ---------- pasar al móvil / recibir ----------
  R.qr = (job) => qrView(job, "send");
  R.receive = (job) => qrView(job, "receive");

  async function qrView(job, mode) {
    S.busy = true;
    const help = mode === "send" ? "Escanéalo con la cámara del móvil y se descarga. <b>No hace falta instalar nada.</b>" : "Escanéalo con la cámara del móvil y elige las fotos o archivos que quieras pasar al PC.";
    if (mode === "receive") setHeader(job, `Se guardará en <b>${esc(job.files[0] ? job.files[0].name : "")}</b>`);
    setBody(`<div class="qrbox">
      <div class="qr wait" id="qr"><div><div class="spinner"></div>Preparando el enlace…<div class="dimtxt" id="qr-msg" style="margin-top:4px"></div></div></div>
      <div class="qr-tabs" id="qr-tabs" hidden></div>
      <div class="qr-help">${help}</div>
      <div class="linkrow" id="qr-link" hidden><code id="qr-url"></code><button class="ibtn" id="qr-copy" title="Copiar el enlace">${ic("copy")}</button></div>
    </div><div class="items" id="qr-items"></div>`);
    setFoot(`<span class="hint">${ic("clock").replace("<svg", '<svg style="width:13px;height:13px;vertical-align:-2px"')} Al cerrar, el enlace deja de funcionar.</span><span class="grow"></span><button class="btn primary" id="f-close">${mode === "receive" ? "Hecho" : "Cerrar"}</button>`);
    $("#f-close").onclick = close;
    fit(380);
    const st = { tab: "", key: "" };
    const render = (q) => {
      if (!q || S.job !== job) return;
      const cfOK = !!q.cloudflare, dOK = !!q.direct && (q.dirStatus === "verified" || q.dirStatus === "ok");
      // Por defecto Cloudflare (funciona siempre); directo si es lo único listo y está comprobado.
      if (!st.tab) { if (cfOK) st.tab = "cf"; else if (dOK && q.dirStatus === "verified") st.tab = "direct"; }
      const tabs = $("#qr-tabs");
      tabs.hidden = !(cfOK || dOK);
      const dot = (s) => (s === "online" || s === "verified" || s === "ok" ? "on" : s === "error" || s === "cgnat" || s === "noupnp" ? "bad" : "wait");
      tabs.innerHTML = `<button class="qr-tab ${st.tab === "cf" ? "on" : ""} ${cfOK ? "" : "off"}" data-t="cf"><span class="dot ${dot(q.cfStatus)}"></span>${ic("cloud").replace("<svg", '<svg style="width:13px;height:13px"')}Seguro</button>
        <button class="qr-tab ${st.tab === "direct" ? "on" : ""} ${dOK ? "" : "off"}" data-t="direct" title="${esc(q.dirMessage || "")}"><span class="dot ${dot(q.dirStatus)}"></span>${ic("bolt").replace("<svg", '<svg style="width:13px;height:13px"')}Directo</button>`;
      $$(".qr-tab:not(.off)", tabs).forEach((b) => (b.onclick = () => { st.tab = b.dataset.t; st.key = ""; render(S.lastQR); }));
      const url = st.tab === "cf" ? q.cloudflare : st.tab === "direct" ? q.direct : "";
      const svg = st.tab === "cf" ? q.qrCF : q.qrDirect;
      if (url && st.key !== url) {
        st.key = url;
        $("#qr").className = "qr";
        $("#qr").innerHTML = svg + '<div class="qlogo"><img src="logo.svg" alt=""></div>';
        $("#qr-link").hidden = false;
        $("#qr-url").textContent = url;
        $("#qr-copy").onclick = () => copy(url, $("#qr-copy"));
        fit();
      } else if (!url) {
        const m = $("#qr-msg");
        if (m) m.textContent = q.cfStatus === "preparing" && q.cfProgress ? `Descargando lo necesario… ${Math.round(q.cfProgress * 100)} %` : q.cfStatus === "error" ? q.cfMessage || "" : "";
      }
      // actividad
      const items = mode === "send" ? (q.downloads || []).slice(-6).reverse().map((d) => `<div class="item ${d.done ? "ok" : ""}"><div class="ic">${ic(d.done ? "check" : "download")}</div><div class="nm"><b>${esc(d.name)}</b><span>${d.done ? "Descargado en el móvil" : `${fmtSize(d.bytes)} de ${fmtSize(d.total)}${d.speed ? " · " + fmtSize(d.speed) + "/s" : ""}`}</span></div><span class="rt">${d.done ? "" : (d.total ? Math.floor((d.bytes / d.total) * 100) : 0) + " %"}</span>${d.done ? "" : `<div class="pbar sm"><i style="width:${d.total ? (d.bytes / d.total) * 100 : 0}%"></i></div>`}</div>`)
        : (q.uploads || []).map((u) => `<div class="item ${u.done ? "ok" : ""}"><div class="ic">${ic(u.done ? "check" : "upload")}</div><div class="nm"><b>${esc(u.name)}</b><span>${u.done ? fmtSize(u.size) + " · guardado" : `${fmtSize(u.got)} de ${fmtSize(u.size)}`}</span></div>${u.done ? `<span class="acts"><button class="ibtn" data-open="${esc(u.final)}" title="Abrir">${ic("play")}</button></span>` : `<span class="rt">${u.size ? Math.floor((u.got / u.size) * 100) : 0} %</span><div class="pbar sm"><i style="width:${u.size ? (u.got / u.size) * 100 : 0}%"></i></div>`}</div>`);
      const html = items.join("");
      const box = $("#qr-items");
      if (box.innerHTML !== html) { box.innerHTML = html; $$("[data-open]", box).forEach((b) => (b.onclick = () => api.Open(b.dataset.open))); fit(); }
    };
    sub("qr", (q) => { S.lastQR = q; render(q); });
    try { await api.StartQR(mode); } catch (e) { H.qr = null; return fail(errText(e)); }
    const q = await api.QRState();
    S.lastQR = q; render(q);
  }

  // ---------- ¿es seguro? ----------
  R.safe = (job) => {
    const f = job.files[0];
    if (!f || f.isDir) return fail("Elige un archivo, no una carpeta.");
    S.busy = true;
    setBody(`<div class="verdict wait" id="vd"><div class="vic"><span class="spinner" style="width:24px;height:24px;border-width:3px"></span></div><div><h2>Comprobando…</h2><p id="vd-p">Calculando la huella del archivo</p></div></div>
      <div class="checks" id="checks"></div>
      <div id="vt-cta"></div>
      <details class="hashes" id="hashes" hidden><summary>${ic("chev")}Huellas del archivo</summary><div id="hash-list" style="margin-top:6px"></div></details>`);
    setFoot(`<span class="grow"></span><button class="btn primary" id="f-close">Cerrar</button>`);
    $("#f-close").onclick = close;
    fit(300);
    const rows = {
      virustotal: { icon: "shield", title: "Antivirus en la nube" },
      defender: { icon: "shield", title: "Microsoft Defender" },
      signature: { icon: "pen", title: "Firma digital" },
      origin: { icon: "globe", title: "De dónde viene" },
      known: { icon: "hash", title: "Archivo conocido" },
    };
    const state = (k, ev) => {
      const e = ev[k === "virustotal" ? "virusTotal" : k];
      const err = ev.errors && ev.errors[k];
      if (k === "virustotal" && !S.deps.virustotal) return ["na", "Sin activar", "Añade una clave gratuita para comprobarlo con 70 antivirus"];
      if (err) return ["na", "No disponible", err];
      if (!e) return null;
      switch (k) {
        case "virustotal":
          if (!e.found) return ["warn", "Nadie lo ha analizado antes", "Puedes enviarlo para que lo analicen"];
          return [e.malicious >= 5 ? "bad" : e.malicious > 0 ? "warn" : "ok", e.summary, e.detections && e.detections.length ? '<div class="engines">' + e.detections.slice(0, 8).map((d) => `<span>${esc(d.engine)}: ${esc(d.label)}</span>`).join("") + "</div>" : e.firstSeen ? "Visto por primera vez el " + fmtDate(e.firstSeen) : "", true];
        case "defender":
          if (!e.available) return ["na", "No disponible", e.summary];
          return [e.clean ? "ok" : "bad", e.summary, e.threats && e.threats.length ? esc(e.threats.join(", ")) : ""];
        case "signature":
          return [e.status === "valid" ? "ok" : e.status === "unsigned" ? "warn" : "bad", e.summary, e.issuer ? "Emitido por " + esc(e.issuer) : ""];
        case "origin":
          return [e.marked ? (e.site ? "warn" : "warn") : "ok", e.summary, e.host ? esc(e.host) : ""];
        case "known":
          return [e.known ? (e.malicious ? "bad" : "ok") : "na", e.summary, e.product ? esc(e.product) : ""];
      }
    };
    const vIcon = { clean: "shieldOk", suspicious: "shieldQ", dangerous: "shieldX", unknown: "shield" };
    const vCls = { clean: "clean", suspicious: "sus", dangerous: "bad", unknown: "unk" };
    let done = false;
    const draw = (u) => {
      if (S.job !== job) return;
      const ev = u.evidence, v = u.verdict;
      done = u.step === "done";
      const vd = $("#vd");
      if (done || (v && v.level !== "unknown")) {
        vd.className = "verdict " + (vCls[v.level] || "unk");
        vd.innerHTML = `<div class="vic">${done ? ic(vIcon[v.level] || "shield") : '<span class="spinner" style="width:24px;height:24px;border-width:3px"></span>'}</div><div><h2>${esc(v.title)}</h2><p>${esc(v.summary)}</p></div>`;
      }
      const order = ["virustotal", "defender", "signature", "origin", "known"];
      $("#checks").innerHTML = order.map((k) => {
        const s = state(k, ev);
        const r = rows[k];
        if (!s) return `<div class="check"><span class="ci na"><span class="spinner" style="width:12px;height:12px;border-width:2px"></span></span><div class="ct"><b>${r.title}</b><span>Comprobando…</span></div><span></span></div>`;
        const [lvl, t, sub, raw] = s;
        return `<div class="check"><span class="ci ${lvl}">${ic(lvl === "ok" ? "check" : lvl === "bad" ? "x" : lvl === "warn" ? "alert" : "info")}</span><div class="ct"><b>${esc(r.title)}</b><span>${esc(t)}</span>${sub ? `<span>${raw ? sub : sub}</span>` : ""}</div>${k === "virustotal" && ev.virusTotal && ev.virusTotal.link ? `<button class="ibtn" data-url="${esc(ev.virusTotal.link)}" title="Ver la ficha">${ic("ext")}</button>` : "<span></span>"}</div>`;
      }).join("");
      $$("[data-url]").forEach((b) => (b.onclick = () => api.OpenURL(b.dataset.url)));
      const cta = $("#vt-cta");
      if (!S.deps.virustotal) cta.innerHTML = `<div class="note">${ic("key")}<div class="nt"><b>Compruébalo con 70 antivirus.</b> Con una clave gratuita de VirusTotal se consulta la huella del archivo (el archivo no se sube).<br><button class="btn ghost sm" id="vt-add">${ic("key")}Añadir clave</button></div></div>`;
      else if (ev.virusTotal && !ev.virusTotal.found && !S.vtAsked) cta.innerHTML = `<div class="note warn">${ic("upload")}<div class="nt"><b>Nadie lo ha analizado antes.</b> ¿Quieres enviarlo a VirusTotal para que lo analicen? El archivo se subirá a su servicio.<br><button class="btn ghost sm" id="vt-up">${ic("upload")}Enviar a analizar</button></div></div>`;
      else if (!S.vtUploading) cta.innerHTML = "";
      const a = $("#vt-add"); if (a) a.onclick = () => vtKeyDialog(() => { S.deps.virustotal = true; R.safe(job); });
      const up = $("#vt-up"); if (up) up.onclick = () => vtUpload(job, f);
      if (ev.hashes) {
        $("#hashes").hidden = false;
        $("#hash-list").innerHTML = [["SHA-256", ev.hashes.sha256], ["SHA-1", ev.hashes.sha1], ["MD5", ev.hashes.md5]].map(([n, h]) => `<div class="hash"><span>${n}</span><code>${esc(h)}</code><button class="ibtn" data-h="${esc(h)}" title="Copiar">${ic("copy")}</button></div>`).join("");
        $$("[data-h]").forEach((b) => (b.onclick = () => copy(b.dataset.h, b)));
      }
      if (done) S.busy = false;
      fit();
    };
    sub("safe", draw);
    sub("safe-progress", (p) => { const el = $("#vd-p"); if (el && p.total) el.textContent = `Calculando la huella… ${Math.floor((p.done / p.total) * 100)} %`; });
    api.StartSafe(f.path);
  };

  async function vtUpload(job, f) {
    S.vtAsked = true; S.vtUploading = true;
    const cta = $("#vt-cta");
    cta.innerHTML = `<div class="prog"><div class="prog-top"><b id="vu-t">Subiendo a VirusTotal…</b><span id="vu-p"></span></div><div class="pbar" id="vu-bar"><i></i></div><div class="prog-sub"><span>El análisis tarda uno o dos minutos.</span></div></div>`;
    fit();
    sub("vt-upload", (u) => {
      if (u.stage === "upload" && u.total) { $("#vu-bar i").style.width = (u.sent / u.total) * 100 + "%"; $("#vu-p").textContent = Math.floor((u.sent / u.total) * 100) + " %"; }
      else { $("#vu-bar").className = "pbar indet"; $("#vu-t").textContent = "Analizando con decenas de antivirus…"; $("#vu-p").textContent = ""; }
    });
    try {
      await api.VTUpload(f.path);
      S.vtUploading = false;
      R.safe(job);
    } catch (e) { S.vtUploading = false; cta.innerHTML = `<div class="note bad">${ic("alert")}<div class="nt">${esc(errText(e))}</div></div>`; fit(); }
  }

  function vtKeyDialog(done) {
    const prevBody = body.innerHTML, prevFoot = foot.innerHTML, prevHidden = foot.hidden;
    setBody(`<div class="needbox" style="padding-bottom:0"><div class="tile">${ic("key")}</div><h3>Clave gratuita de VirusTotal</h3>
      <p>Crea una cuenta gratis en VirusTotal, entra en tu perfil → <b>API key</b>, cópiala y pégala aquí. Solo se guarda en este PC.</p></div>
      <div class="field big" id="k-f"><input id="k" placeholder="Pega aquí la clave (64 caracteres)" spellcheck="false" autocomplete="off"></div>
      <p class="dimtxt" style="margin:8px 2px 0">Con la cuenta gratuita se pueden hacer 4 consultas por minuto.</p>`);
    setFoot(`<button class="btn ghost" id="k-site">${ic("ext")}Conseguir la clave</button><span class="grow"></span><button class="btn ghost" id="k-x">Cancelar</button><button class="btn primary" id="k-ok">Guardar</button>`);
    $("#k-site").onclick = () => api.OpenURL("https://www.virustotal.com/gui/my-apikey");
    $("#k-x").onclick = () => { body.innerHTML = prevBody; foot.innerHTML = prevFoot; foot.hidden = prevHidden; start(S.job); };
    $("#k").focus();
    $("#k-ok").onclick = async () => {
      try { await api.SetVTKey($("#k").value); S.deps.virustotal = true; toast("Clave guardada"); done(); }
      catch (e) { $("#k-f").classList.add("err"); toast(errText(e), "error"); }
    };
    $("#k").onkeydown = (e) => { if (e.key === "Enter") $("#k-ok").click(); };
    fit();
  }

  // ---------- ¿de dónde salió? ----------
  R.origin = async (job) => {
    const f = job.files[0];
    const o = await api.GetOrigin(f.path);
    if (S.job !== job) return;
    if (!o || !o.marked) {
      setBody(`<div class="needbox"><div class="tile" style="--tile:linear-gradient(135deg,#475569,#334155);--tile-sh:rgba(0,0,0,.4)">${ic("globe")}</div><h3>Sin origen registrado</h3><p>${esc((o && o.summary) || "Este archivo no tiene origen registrado.")}</p>${f.mod ? `<p class="dimtxt">Modificado el ${fmtDate(f.mod)}</p>` : ""}</div>`);
      setFoot(`<span class="grow"></span><button class="btn primary" id="f-close">Cerrar</button>`);
      $("#f-close").onclick = close;
      return fit();
    }
    const site = o.site || "";
    const page = o.referrer || "", link = o.host || "";
    setBody(`<div class="origin">
      <div class="site"><div class="fav" id="fav">${ic("globe")}</div><div style="min-width:0"><div class="dom">${esc(site || o.zone || "Internet")}</div><div class="when">${o.created ? "Descargado el " + esc(fmtDate(o.created)) : esc(o.zone || "")}</div></div></div>
      <div class="kv">
        ${page ? `<div><span>Página</span><div class="linkrow"><code>${esc(page)}</code><button class="ibtn" data-c="${esc(page)}" title="Copiar">${ic("copy")}</button></div></div>` : ""}
        ${link ? `<div><span>Enlace exacto</span><div class="linkrow"><code>${esc(link)}</code><button class="ibtn" data-c="${esc(link)}" title="Copiar">${ic("copy")}</button></div></div>` : ""}
      </div>
      ${o.fromArchive || (!page && !link) ? `<div class="note">${ic("info")}<div class="nt">${esc(o.summary)}</div></div>` : ""}
    </div>`);
    setFoot(`<button class="linkbtn" id="o-unmark" title="Quita el aviso de «este archivo viene de internet» al abrirlo">Quitar la marca de internet</button><span class="grow"></span>${link ? `<button class="btn ghost" id="o-copy">${ic("copy")}Copiar enlace</button>` : ""}${page || link ? `<button class="btn primary" id="o-open">${ic("ext")}Abrir la web</button>` : '<button class="btn primary" id="f-close">Cerrar</button>'}`);
    $$("[data-c]").forEach((b) => (b.onclick = () => copy(b.dataset.c, b)));
    const oc = $("#o-copy"); if (oc) oc.onclick = () => copy(link);
    const oo = $("#o-open"); if (oo) oo.onclick = () => api.OpenURL(page || link);
    const fc = $("#f-close"); if (fc) fc.onclick = close;
    $("#o-unmark").onclick = async () => { try { await api.RemoveMark(f.path); toast("Marca quitada"); $("#o-unmark").remove(); } catch (e) { toast(errText(e), "error"); } };
    if (site) { const img = new Image(); img.onload = () => { const fv = $("#fav"); if (fv) { fv.innerHTML = ""; fv.appendChild(img); } }; img.src = `https://icons.duckduckgo.com/ip3/${encodeURIComponent(site)}.ico`; }
    fit();
  };

  // ---------- ¿qué lo bloquea? ----------
  R.lock = async (job) => {
    const isDrive = job.files.some((f) => f.kind === "drive");
    setBody(`<div class="prog"><div class="prog-top"><b>Buscando qué lo está usando…</b></div><div class="pbar indet"><i></i></div></div>`);
    fit();
    const rep = await api.LockScan();
    if (S.job !== job) return;
    renderLock(job, rep, isDrive);
  };

  function renderLock(job, rep, isDrive) {
    const procs = (rep.procs || []).filter((p) => p.canClose || p.canUnlock).concat((rep.procs || []).filter((p) => !(p.canClose || p.canUnlock)));
    const kindTag = { service: "servicio", explorer: "Explorador", system: "sistema", critical: "sistema" };
    let html;
    if (!procs.length) {
      html = `<div class="needbox"><div class="tile" style="--tile:linear-gradient(135deg,#10b981,#0ea5e9);--tile-sh:rgba(16,185,129,.5)">${ic("unlock")}</div><h3>Nada lo está bloqueando</h3><p>${isDrive ? "Ya se puede expulsar con seguridad." : "Ningún programa lo está usando ahora mismo: ya puedes borrarlo, moverlo o renombrarlo."}</p></div>`;
    } else {
      html = `<div class="label">${plural(procs.length, "programa lo está usando", "programas lo están usando")}<span class="gl"></span></div>` +
        procs.map((p) => `<div class="proc" data-pid="${p.pid}"><div class="pi">${p.icon ? `<img src="${p.icon}" alt="">` : ic("app")}</div>
          <div class="pn"><b>${esc(p.name)}${kindTag[p.kind] ? `<span class="tagk">${kindTag[p.kind]}</span>` : ""}</b><span class="st">${esc(p.hint || (p.fileCount > 1 ? plural(p.fileCount, "archivo", "archivos") : p.files && p.files.length ? baseName(p.files[0]) : p.exeName || ""))}</span></div>
          ${p.canClose || p.canUnlock ? `<button class="btn ghost sm" data-free="${p.pid}">${ic("unlock")}Liberar</button>` : '<span class="dimtxt">Sin permiso</span>'}</div>`).join("");
    }
    if (rep.warnings && rep.warnings.length) html += rep.warnings.map((w) => `<div class="note warn">${ic("alert")}<div class="nt">${esc(w)}</div></div>`).join("");
    setBody(html);
    const n = procs.length;
    if (isDrive) {
      setFoot(`<button class="btn ghost" id="l-re">${ic("convert")}Volver a mirar</button><span class="grow"></span><button class="btn primary" id="l-eject">${ic("eject")}${n ? "Liberar y expulsar" : "Expulsar"}</button>`);
    } else {
      setFoot(`<button class="btn ghost" id="l-re" title="Volver a mirar">${ic("convert")}</button><span class="grow"></span><button class="btn ghost" id="l-move">${ic("move")}${n ? "Liberar y mover" : "Mover"}</button><button class="btn danger" id="l-del">${ic("trash")}${n ? "Liberar y borrar" : "Borrar"}</button>`);
    }
    $("#l-re").onclick = () => R.lock(job);
    $$("[data-free]").forEach((b) => (b.onclick = () => freeOne(+b.dataset.free, false)));
    const all = async () => {
      for (const p of procs) {
        if (!(p.canClose || p.canUnlock)) continue;
        const el = $(`.proc[data-pid="${p.pid}"]`);
        if (el && el.classList.contains("freed")) continue;
        const ok = await freeOne(p.pid, false);
        if (!ok) return false;
      }
      return true;
    };
    const after = async (op, dest) => {
      if (!(await all())) return;
      try {
        await api.LockAfter(op, dest || "");
        toast(op === "delete" ? "Enviado a la papelera" : op === "move" ? "Movido" : "Ya puedes quitar el USB");
        setTimeout(close, 900);
      } catch (e) { toast(errText(e), "error"); }
    };
    const del = $("#l-del"); if (del) del.onclick = () => after("delete");
    const mv = $("#l-move"); if (mv) mv.onclick = async () => { const d = await api.PickFolder("¿Adónde lo muevo?", job.files[0].dir); if (d) after("move", d); };
    const ej = $("#l-eject"); if (ej) ej.onclick = () => after("eject");
    fit();
  }

  // Libera un proceso. Si no se cierra por las buenas, ofrece forzarlo.
  async function freeOne(pid, force) {
    const el = $(`.proc[data-pid="${pid}"]`);
    const btn = el && el.querySelector("[data-free]");
    if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>'; }
    let r;
    try { r = await api.LockRelease(pid, force); } catch (e) { r = { ok: false, message: errText(e) }; }
    if (!el) return r.ok;
    if (r.ok) {
      el.classList.add("freed");
      el.querySelector(".st").textContent = r.message || "Liberado";
      btn.outerHTML = `<span class="rt" style="color:var(--ok);font-size:12px;font-weight:650">${ic("check").replace("<svg", '<svg style="width:14px;height:14px;vertical-align:-3px"')} Liberado</span>`;
      return true;
    }
    const st = el.querySelector(".st");
    st.className = "st " + (r.status === "unsaved" ? "warn" : "bad");
    st.textContent = r.message || "No se ha podido cerrar";
    if (r.canForce !== false) {
      btn.disabled = false; btn.className = "btn danger sm"; btn.innerHTML = "Forzar";
      btn.onclick = () => freeOne(pid, true);
    } else { btn.remove(); }
    return false;
  }

  // ---------- nueva carpeta con esto ----------
  R.folder = async (job) => {
    const names = await api.SuggestFolderNames();
    if (S.job !== job) return;
    const files = job.files;
    setBody(`<div class="label" style="margin-top:2px">Nombre de la carpeta<span class="gl"></span></div>
      <div class="field big"><input id="fn" value="${esc(names[0] || "Nueva carpeta")}" maxlength="120" spellcheck="false"></div>
      ${names.length > 1 ? `<div class="chips">${names.slice(1).map((n) => `<button class="chip-s" data-n="${esc(n)}">${esc(n)}</button>`).join("")}</div>` : ""}
      <div class="stack"><div class="thumbs" id="fthumbs">${files.slice(0, 5).map((f, i) => `<div data-i="${i}">${ic((KIND[f.kind] || KIND.other).icon)}</div>`).join("")}</div>
      <span class="hint">Se meterán ${plural(files.length, "elemento", "elementos")} en «${esc(files[0] ? baseName(files[0].dir) : "")}»</span></div>`);
    setFoot(`<span class="dimtxt">Intro para crearla</span><span class="grow"></span><button class="btn ghost" id="f-close">Cancelar</button><button class="btn primary" id="go">${ic("folderPlus")}Crear carpeta</button>`);
    $("#f-close").onclick = close;
    const fn = $("#fn");
    setTimeout(() => { fn.focus(); fn.select(); }, 30);
    $$(".chip-s").forEach((b) => (b.onclick = () => { fn.value = b.dataset.n; fn.focus(); fn.select(); }));
    fn.onkeydown = (e) => { if (e.key === "Enter") $("#go").click(); };
    files.slice(0, 5).forEach((f, i) => thumbURL(f.path, 48).then((u) => { const el = $(`#fthumbs [data-i="${i}"]`); if (u && el) el.innerHTML = `<img src="${u}" class="${["image", "video"].includes(f.kind) ? "" : "icon"}">`; }));
    $("#go").onclick = async () => {
      $("#go").disabled = true;
      try {
        const r = await api.MakeFolder(fn.value);
        folderDone(job, r);
      } catch (e) { toast(errText(e), "error"); $("#go").disabled = false; }
    };
    fit();
  };

  function folderDone(job, r) {
    const moved = (r.moved || []).length, failed = (r.failed || []).length;
    setBody(`<div class="result"><div class="ring ${failed ? "warn" : ""}">${ic(failed ? "alert" : "check")}</div><h2>${esc(baseName(r.folder))}</h2>
      <p>${plural(moved, "elemento movido", "elementos movidos")}${failed ? ` · ${failed} no se pudieron mover (${esc(r.errors[0] || "")})` : ""}</p>
      <div class="countdown" id="cd"><i></i></div></div>`);
    setFoot(`<button class="btn ghost" id="f-undo">${ic("undo")}Deshacer</button><span class="grow"></span><button class="btn ghost" id="f-open">${ic("folderOpen")}Abrir</button><button class="btn primary" id="f-close">Hecho</button>`);
    $("#f-close").onclick = close;
    $("#f-open").onclick = () => { api.Open(r.folder); close(); };
    $("#f-undo").onclick = async () => { clearTimers(); try { await api.UndoFolder(r); toast("Deshecho"); setTimeout(close, 700); } catch (e) { toast(errText(e), "error"); } };
    api.Reveal(r.folder);
    // Se cierra sola a los 6 s salvo que el ratón esté encima.
    let left = 6000, last = Date.now();
    const bar = $("#cd i");
    bar.style.transition = "none";
    every(() => {
      const now = Date.now();
      if (!document.querySelector(".win:hover")) left -= now - last;
      last = now;
      bar.style.width = Math.max(0, (left / 6000) * 100) + "%";
      if (left <= 0) { clearTimers(); close(); }
    }, 100);
    fit();
  }

  // ---------- teclado y ventana ----------
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") { e.preventDefault(); close(); }
  });
  $("#wb-close").onclick = close;
  $("#wb-min").onclick = () => api.Minimise();
  document.addEventListener("contextmenu", (e) => { if (!e.target.closest("input, code")) e.preventDefault(); });

  // ---------- inicio ----------
  (async () => {
    try {
      const job = await api.GetJob();
      if (job.action) start(job);
    } catch (e) { fail(errText(e)); }
  })();

  // ========================================================================
  // Datos de prueba para ver la interfaz en un navegador normal (sin Go).
  // ========================================================================
  function mockApi() {
    const scen = (location.hash || "#reduce").slice(1);
    const [action, kind] = scen.split("-");
    const mk = (name, kind, size) => ({ path: "C:\\Users\\Ana\\Vídeos\\" + name, name, ext: name.split(".").pop(), dir: "C:\\Users\\Ana\\Vídeos", kind, size, mod: 1761234567 });
    const files = {
      video: [mk("Vacaciones playa 2026.mp4", "video", 47185920)],
      image: [mk("IMG_20260814_183012.jpg", "image", 6291456)],
      pdf: [mk("Contrato alquiler.pdf", "pdf", 18874368)],
      multi: [mk("Vacaciones playa 01.jpg", "image", 4194304), mk("Vacaciones playa 02.jpg", "image", 5194304), mk("Vacaciones playa 03.jpg", "image", 3194304)],
      exe: [mk("SteamSetup.exe", "exe", 2411724)],
      drive: [{ path: "E:\\", name: "E:", kind: "drive", isDir: true }],
      folder: [{ path: "C:\\Users\\Ana\\Descargas", name: "Descargas", kind: "folder", isDir: true, dir: "C:\\Users\\Ana" }],
    }[kind || (action === "safe" || action === "origin" ? "exe" : action === "folder" ? "multi" : action === "receive" ? "folder" : "video")] || [];
    const job = { action, files, total: files.reduce((a, f) => a + (f.size || 0), 0), gen: 1, version: "1.0.0", deps: { ffmpeg: "x", turbosend: "x", virustotal: false, office: "" } };
    const wait = (ms, v) => new Promise((r) => setTimeout(() => r(v), ms));
    const ev = {};
    rt.EventsOn = (n, f) => ((ev[n] = ev[n] || []).push(f), () => { });
    const fire = (n, d) => (ev[n] || []).forEach((f) => f(d));
    const svgImg = (c1, c2) => "data:image/svg+xml," + encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 260"><defs><linearGradient id="a" x2="1" y2="1"><stop offset="0" stop-color="${c1}"/><stop offset="1" stop-color="${c2}"/></linearGradient></defs><rect width="400" height="260" fill="url(#a)"/><circle cx="300" cy="70" r="36" fill="#fde68a"/><path d="M0 200 120 120l90 70 70-50 120 90v70H0z" fill="#0f172a" opacity=".55"/></svg>`);
    return new Proxy({}, {
      get: (_, name) => async (...a) => {
        switch (name) {
          case "GetJob": return job;
          case "Show": case "Resize": { const w = document.querySelector(".win"); w.style.height = a[1] + "px"; w.style.width = a[0] + "px"; return; }
          case "Thumb": return files[0] && files[0].kind === "image" ? svgImg("#0ea5e9", "#8b5cf6") : "";
          case "HomeInfo": return { installed: true, modern: false, prewarm: false, version: "1.0.0", deps: job.deps };
          case "Plan": {
            if (a[0] === "reduce") {
              if (kind === "image" || kind === "multi") return { kind: "image", origSize: job.total, image: { quality: 80, scales: [{ id: "100", title: "Original", sub: "4000 × 3000" }, { id: "75", title: "75 %", sub: "3000 × 2250" }, { id: "50", title: "50 %", sub: "2000 × 1500" }, { id: "25", title: "25 %", sub: "1000 × 750" }] } };
              if (kind === "pdf") return { kind: "pdf", origSize: job.total, presets: [{ id: "screen", title: "Para leer en pantalla", sub: "Lo más ligero: ideal para enviar", est: 3145728, icon: "phone", color: "linear-gradient(135deg,#8b5cf6,#06b6d4)", rec: true }, { id: "print", title: "Para imprimir", sub: "Mantiene la nitidez de las imágenes", est: 9437184, icon: "doc", color: "linear-gradient(135deg,#475569,#334155)" }] };
              return { kind: "video", info: "Vídeo MP4 · <b>45 MB</b> · 1080p · 2:31", needs: "ffmpeg", origSize: job.total, custom: true, presets: [{ id: "discord", title: "Para Discord", sub: "Menos de 10 MB", brand: "D", color: "#5865f2", est: 10066329, rec: true }, { id: "whatsapp", title: "Para WhatsApp", sub: "Menos de 16 MB", brand: "W", color: "#25d366", est: 16106127 }], qualities: [{ id: "high", title: "Alta", est: 28311552 }, { id: "medium", title: "Media", est: 15728640 }, { id: "low", title: "Baja", est: 7340032 }] };
            }
            return { kind: "video", targets: [{ id: "mp4", ext: "mp4", name: "MP4", desc: "Se ve en todas partes", group: "Vídeo", recent: true }, { id: "webm", ext: "webm", name: "WEBM", desc: "Ideal para webs, más ligero", group: "Vídeo" }, { id: "mkv", ext: "mkv", name: "MKV", desc: "Guarda varias pistas y subtítulos", group: "Vídeo" }, { id: "mov", ext: "mov", name: "MOV", desc: "Para Mac y edición", group: "Vídeo" }, { id: "gif", ext: "gif", name: "GIF animado", desc: "Para mandar como animación", group: "Animación", gif: true }, { id: "mp3", ext: "mp3", name: "MP3", desc: "Solo el sonido, compatible con todo", group: "Solo audio" }, { id: "m4a", ext: "m4a", name: "M4A", desc: "Solo el sonido, mejor calidad", group: "Solo audio" }, { id: "wav", ext: "wav", name: "WAV", desc: "Sin pérdida, pesa mucho", group: "Solo audio" }] };
          }
          case "ImagePreview": await wait(120); return { before: svgImg("#0ea5e9", "#8b5cf6"), after: svgImg("#0891b2", "#7c3aed"), size: Math.round(job.total * (a[1] / 100) * 0.4 * (a[2] / 100)), origSize: job.total };
          case "Run": for (let i = 0; i <= 20; i++) { await wait(80); fire("job-progress", { overall: i / 20, frac: i / 20, index: 0, eta: 20 - i, speed: 3.4 }); } return { results: files.map((f) => ({ in: f.path, out: f.path.replace(/(\.\w+)$/, " (reducido)$1"), before: f.size, after: Math.round(f.size * 0.19) })) };
          case "SendRooms": await wait(300); return [{ id: "a", name: "Marta", online: 1, members: 1, preview: "Tú: 📎 fotos.zip" }, { id: "b", name: "Grupo familia", members: 4, preview: "Papá: ¡gracias!" }];
          case "SendTo": await wait(400); return { room: "a", bundle: "x" };
          case "SendStatus": return { links: { cloudflare: "https://ejemplo-turbo.trycloudflare.com/r/abc", direct: "http://83.45.12.9:47831/r/abc" }, bundle: { downloads: 0 }, transfers: [] };
          case "StartQR": setTimeout(() => fire("qr", { cloudflare: "https://ejemplo.trycloudflare.com/q/abc/", cfStatus: "online", dirStatus: "verified", direct: "http://83.45.12.9:47832/q/abc/", qrCF: '<svg viewBox="0 0 21 21"><path d="M0 0h7v7H0zM14 0h7v7h-7zM0 14h7v7H0zM9 9h3v3H9zM15 15h3v3h-3z"/></svg>', qrDirect: '<svg viewBox="0 0 21 21"><path d="M0 0h7v7H0zM14 0h7v7h-7zM0 14h7v7H0z"/></svg>', downloads: [{ name: files[0] ? files[0].name : "x", bytes: 2e7, total: 4.7e7, speed: 5e6 }], uploads: [{ name: "IMG_2041.HEIC", size: 3e6, got: 3e6, done: true, final: "x" }, { name: "VID_2042.MOV", size: 9e7, got: 3e7 }] }), 1200); return;
          case "QRState": return { cfStatus: "starting", dirStatus: "searching" };
          case "StartSafe": setTimeout(() => fire("safe", { step: "signature", evidence: { signature: { status: "valid", summary: "Firmado por Valve Corp.", issuer: "DigiCert" }, origin: { marked: true, site: "steampowered.com", summary: "Descargado de steampowered.com", host: "https://cdn.akamai.steamstatic.com/client/installer/SteamSetup.exe" } }, verdict: { level: "unknown", title: "Comprobando…", summary: "" } }), 300);
            setTimeout(() => fire("safe", { step: "done", evidence: { hashes: { sha256: "a3f1c0ffee7b2d4e9a8c6b5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f", sha1: "0123456789abcdef0123456789abcdef01234567", md5: "0123456789abcdef0123456789abcdef" }, signature: { status: "valid", summary: "Firmado por Valve Corp.", issuer: "DigiCert" }, defender: { available: true, clean: true, summary: "Microsoft Defender no ha encontrado nada" }, origin: { marked: true, site: "steampowered.com", summary: "Descargado de steampowered.com", host: "https://cdn.akamai.steamstatic.com/client/installer/SteamSetup.exe" }, known: { known: false, summary: "No aparece en la base de archivos conocidos" } }, verdict: { level: "clean", title: "Parece seguro", summary: "Firmado por Valve Corp. y Defender no encuentra nada" } }), 1300);
            return;
          case "GetOrigin": return { marked: true, zoneId: 3, zone: "Internet", site: "steampowered.com", referrer: "https://store.steampowered.com/about/", host: "https://cdn.akamai.steamstatic.com/client/installer/SteamSetup.exe", created: "2026-10-21T18:42:00Z", summary: "Descargado de steampowered.com" };
          case "LockScan": await wait(400); return { procs: [{ pid: 4120, name: "Microsoft Word", exe: "WINWORD.EXE", kind: "app", files: [files[0] ? files[0].path : ""] }, { pid: 880, name: "Explorador de Windows", exe: "explorer.exe", kind: "explorer", files: ["a", "b"] }] };
          case "LockRelease": await wait(600); return a[0] === 4120 && !a[1] ? { ok: false, status: "unsaved", message: "Puede tener cambios sin guardar" } : { ok: true, message: "Liberado" };
          case "SuggestFolderNames": return ["Vacaciones playa", "Vacaciones playa 2026", "Fotos agosto 2026", "Fotos"];
          case "MakeFolder": return { folder: "C:\\x\\" + a[0], moved: files.map((f) => f.path), from: [], failed: [] };
          case "SetJob": return { ...job, action: a[0], gen: 2 };
          default: return null;
        }
      },
    });
  }
})();
