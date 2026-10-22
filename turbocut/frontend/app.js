(() => {
  "use strict";

  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const nf0 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 0 });
  const nf1 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 1 });
  const nf2 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 2 });
  const dtf = new Intl.DateTimeFormat("es-ES", { day: "numeric", month: "short", year: "numeric" });
  const dtfLong = new Intl.DateTimeFormat("es-ES", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const count = (n) => nf0.format(n || 0);

  function fmtBytes(n) {
    if (!isFinite(n) || n <= 0) return "0 B";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    const f = i === 0 ? nf0 : (n >= 100 ? nf0 : (i >= 3 && n < 10 ? nf2 : nf1));
    return f.format(n) + " " + u[i];
  }
  function splitBytes(n) { const s = fmtBytes(n); const k = s.lastIndexOf(" "); return [s.slice(0, k), s.slice(k + 1)]; }
  function pct(a, b) {
    if (!b || a <= 0) return "0 %";
    const p = a / b * 100;
    if (p < 0.1) return "<0,1 %";
    return (p >= 10 ? nf0 : nf1).format(p) + " %";
  }
  const fmtDate = (u) => u ? dtf.format(new Date(u * 1000)) : "—";
  function ago(unix) {
    const s = Math.max(0, Date.now() / 1000 - unix);
    if (s < 90) return "hace un momento";
    if (s < 3600) return "hace " + Math.round(s / 60) + " min";
    if (s < 86400) return "hace " + Math.round(s / 3600) + " h";
    const d = Math.round(s / 86400);
    return d === 1 ? "ayer" : "hace " + d + " días";
  }
  function fmtSecs(s) {
    if (s < 1) return nf2.format(Math.max(0.01, s)) + " s";
    if (s < 60) return nf1.format(s) + " s";
    return Math.floor(s / 60) + " min " + Math.round(s % 60) + " s";
  }
  const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };
  const store = {
    get(k, d) { try { const v = localStorage.getItem("tc:" + k); return v == null ? d : JSON.parse(v); } catch { return d; } },
    set(k, v) { try { localStorage.setItem("tc:" + k, JSON.stringify(v)); } catch { } },
  };

  // ---------- iconos ----------
  const P = {
    folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/>',
    file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>',
    film: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 3v18M17 3v18M3 7.5h4M3 12h18M3 16.5h4M17 7.5h4M17 16.5h4"/>',
    image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>',
    music: '<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
    game: '<path d="M6 11h4M8 9v4M15 12h.01M18 10h.01"/><path d="M17.32 5H6.68a4 4 0 0 0-3.98 3.59l-.7 6.94A3 3 0 0 0 5 19c1 0 1.5-.5 2-1l1.4-1.4A2 2 0 0 1 9.8 16h4.4a2 2 0 0 1 1.4.6L17 18c.5.5 1 1 2 1a3 3 0 0 0 3-3.47l-.7-6.94A4 4 0 0 0 17.32 5z"/>',
    archive: '<rect x="2" y="3" width="20" height="5" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8M10 12h4"/>',
    app: '<rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M15 2v2M15 20v2M2 15h2M2 9h2M20 15h2M20 9h2M9 2v2M9 20v2"/>',
    doc: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6M16 13H8M16 17H8M10 9H8"/>',
    code: '<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/>',
    brain: '<path d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z"/><path d="M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18Z"/><path d="M12 5v13"/>',
    cog: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/>',
    trash: '<path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
    reveal: '<path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
    enter: '<path d="M5 12h14M12 5l7 7-7 7"/>',
    play: '<path d="m6 3 14 9-14 9V3z"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    alert: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/>',
    chev: '<path d="m9 18 6-6-6-6"/>',
    chevd: '<path d="m6 9 6 6 6-6"/>',
    down: '<path d="M12 5v14M19 12l-7 7-7-7"/>',
    up: '<path d="M12 19V5M5 12l7-7 7 7"/>',
    bolt: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
    drive: '<path d="M22 12H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/><path d="M6 16h.01M10 16h.01"/>',
    usb: '<circle cx="10" cy="7" r="1"/><circle cx="4" cy="20" r="1"/><path d="M4.7 19.3 19 5M21 3l-3 1 2 2ZM9.26 7.68 5 12l2 5M10 14l5 2 3.5-3.5M18 12l1-1 1 1-1 1Z"/>',
    globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    gpu: '<rect x="2" y="6" width="20" height="12" rx="2"/><circle cx="9" cy="12" r="2.5"/><circle cx="16" cy="12" r="2.5"/>',
    apps: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    bug: '<path d="M8 2l1.88 1.88M14.12 3.88 16 2M9 7.13v-1a3 3 0 1 1 6 0v1"/><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6M12 20v-9M6.53 9C4.6 8.8 3 7.1 3 5M6 13H2M3 21c0-2.1 1.7-3.9 3.8-4M21 5c0 2.1-1.6 3.8-3.5 4M22 13h-4M17.2 17c2.1.1 3.8 1.9 3.8 4"/>',
    update: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>',
    box: '<path d="m7.5 4.27 9 5.15M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5M12 22V12"/>',
    moon: '<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>',
    memory: '<path d="M6 19v-3M10 19v-3M14 19v-3M18 19v-3M8 11V9M16 11V9M12 11V9M2 15h20M2 7a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v1.1a2 2 0 0 0 0 3.84V17a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-5.06a2 2 0 0 0 0-3.84Z"/>',
    temp: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
    lock: '<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
    broom: '<path d="m21.64 3.64-1.28-1.28a1.21 1.21 0 0 0-1.72 0L2.36 18.64a1.21 1.21 0 0 0 0 1.72l1.28 1.28a1.2 1.2 0 0 0 1.72 0L21.64 5.36a1.2 1.2 0 0 0 0-1.72M14 7l3 3M5 6v4M19 14v4M10 2v2M7 8H3M21 16h-4M11 3H9"/>',
    tup: '<path d="M22 7 13.5 15.5 8.5 10.5 2 17M16 7h6v6"/>',
    tdown: '<path d="M22 17 13.5 8.5 8.5 13.5 2 7M16 17h6v-6"/>',
    history: '<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5M12 7v5l4 2"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${P[n] || P.file}</svg>`;
  const catIcons = ["file", "film", "image", "music", "game", "archive", "app", "doc", "code", "brain", "cog"];

  // ---------- puente con Go ----------
  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const api = hasGo ? window.go.main.App : new Proxy({}, { get: () => () => Promise.reject("Abre TurboCUT.exe para usarlo") });
  const rt = window.runtime || { EventsOn() { }, WindowMinimise() { }, WindowToggleMaximise() { }, Quit() { } };

  // ---------- estado ----------
  const S = {
    info: null, cats: [], scan: null,
    cur: null, curPath: "", hist: [],
    listing: null, tab: "explorar", search: "",
    sel: new Map(), focus: null,
    types: null, typeMode: "cat", hlCat: -1,
    bigCat: -1, cands: null, cleanSel: new Set(), openCards: new Set(), growth: null,
    scanning: false,
    selMode: false, showMap: false,
  };
  const catColor = (k) => (S.cats[k] || S.cats[0] || { color: "#5b6478" }).color;
  const catLabel = (k) => (S.cats[k] || { label: "Otros" }).label;

  // ---------- vistas ----------
  function show(view) {
    for (const v of ["home", "scan", "results"]) $("#v-" + v).classList.toggle("on", v === view);
    S.view = view;
    renderPills();
    if (view === "results") requestAnimationFrame(() => { resizeTreemap(true); });
  }

  function renderPills() {
    const el = $("#pills");
    const admin = S.info && S.info.admin;
    const parts = [];
    if (S.view === "results" && S.scan) {
      parts.push(`<span class="pill path" title="${esc(S.scan.root)}">${ic(S.scan.isDrive ? "drive" : "folder")}<b>&lrm;${esc(S.scan.root)}&lrm;</b></span>`);
      parts.push(S.scan.mode === "mft"
        ? `<span class="pill turbo">${ic("bolt")}<b>Turbo</b> · MFT</span>`
        : `<span class="pill${admin ? "" : " clickable"}" id="pill-mode" title="${admin ? "" : "Pulsa para reiniciar en modo turbo"}">Modo normal</span>`);
      parts.push(`<span class="pill">${ic("temp")}<b>${fmtSecs(S.scan.seconds)}</b></span>`);
      parts.push(`<span class="pill">${count(S.scan.files)} archivos</span>`);
    } else if (admin) {
      parts.push(`<span class="pill turbo">${ic("bolt")}<b>Modo turbo activo</b></span>`);
    }
    el.innerHTML = parts.join("");
    const pm = $("#pill-mode");
    if (pm && !admin) pm.addEventListener("click", restartAdmin);
  }

  // ---------- inicio ----------
  async function renderHome() {
    const info = S.info;
    const drives = info.drives || [];
    $("#drives").innerHTML = drives.map((d, i) => {
      const used = d.total - d.free;
      const p = d.total ? used / d.total : 0;
      const name = d.path.replace(/\\$/, "");
      const kind = d.kind === "usb" ? "Extraíble" : d.kind === "red" ? "Unidad de red" : "Disco local";
      return `<button class="drive" data-path="${esc(d.path)}" style="animation-delay:${i * 0.05}s">
        <div class="drive-top">
          <div class="tile">${ic(d.kind === "red" ? "globe" : d.kind === "usb" ? "usb" : "drive")}</div>
          <div><div class="drive-name">${esc(name)}${d.label ? `<small>${esc(d.label)}</small>` : ""}</div>
          <div class="drive-sub">${esc(kind)}${d.fs ? " · " + esc(d.fs) : ""}${d.fs === "NTFS" && d.kind !== "red" ? (info.admin ? " · ⚡ turbo" : "") : ""}</div></div>
          <span class="drive-go">${ic("enter")}</span>
        </div>
        <div class="usage${p > 0.9 ? " full" : ""}"><i style="width:${(p * 100).toFixed(1)}%"></i></div>
        <div class="drive-foot"><span><b>${fmtBytes(d.free)}</b> libres</span><span>${fmtBytes(used)} de ${fmtBytes(d.total)}</span></div>
      </button>`;
    }).join("");
    $$("#drives .drive").forEach((b) => b.addEventListener("click", () => startScan(b.dataset.path)));
    $("#back-results").hidden = !S.scan;
    const tc = $("#turbo-card");
    if (info.admin) {
      tc.className = "turbo-card on";
      tc.innerHTML = `<div class="tile">${ic("bolt")}</div><div class="txt"><b>Modo turbo activo</b><p>En discos NTFS TurboCUT lee el índice del disco (la MFT) directamente: un disco entero en segundos y sin carpetas sin acceso.</p></div>`;
    } else {
      tc.className = "turbo-card";
      tc.innerHTML = `<div class="tile">${ic("bolt")}</div><div class="txt"><b>Activa el modo turbo</b><p>Con permiso de administrador TurboCUT lee el índice del disco (la MFT) en vez de abrir carpeta por carpeta: va varias veces más rápido y ve también las carpetas protegidas.</p></div>
        <button class="btn amber" id="turbo-go">${ic("bolt")}Reiniciar en modo turbo</button>`;
      $("#turbo-go").addEventListener("click", () => restartAdmin());
    }
  }

  async function restartAdmin() {
    try { await api.RestartAdmin(S.scan ? S.scan.root : ""); }
    catch (e) { toast(String(e), "error"); }
  }

  // ---------- escaneo ----------
  async function startScan(path) {
    if (S.scanning) return;
    hideCtx();
    $("#scan-title").textContent = "Analizando " + path;
    $("#scan-mode").textContent = "Preparando…";
    $("#scan-mode").className = "chip";
    for (const k of ["files", "dirs"]) $("#sc-" + k).textContent = "0";
    $("#sc-bytes").textContent = "0 B";
    $("#sc-time").textContent = "0 s";
    $("#scan-cur").textContent = " ";
    $("#scan-bar").classList.remove("det");
    $("#scan-bar i").style.width = "";
    show("scan");
    S.scanning = true;
    try { await api.Scan(path); }
    catch (e) { S.scanning = false; toast(String(e), "error"); show(S.scan ? "results" : "home"); }
  }

  function onProgress(p) {
    if (!S.scanning) return;
    const mft = p.mode === "mft";
    const m = $("#scan-mode");
    m.className = "chip" + (mft ? " turbo" : "");
    m.innerHTML = mft ? ic("bolt") + "Modo turbo · leyendo la MFT" : "Modo normal · recorriendo carpetas";
    const bar = $("#scan-bar");
    if (mft && p.total > 0) {
      bar.classList.add("det");
      $("#scan-bar i").style.width = Math.min(100, p.done / p.total * 100).toFixed(1) + "%";
      $("#sc-files").textContent = count(p.done);
      $("#sc-files").nextElementSibling.textContent = "registros leídos";
      $("#sc-dirs").textContent = Math.round(p.done / p.total * 100) + " %";
      $("#sc-dirs").nextElementSibling.textContent = "del índice";
      $("#sc-bytes").textContent = p.files ? fmtBytes(p.bytes) : "…";
      $("#scan-cur").textContent = "Leyendo el índice del disco de un tirón";
    } else {
      bar.classList.remove("det");
      $("#sc-files").textContent = count(p.files);
      $("#sc-files").nextElementSibling.textContent = "archivos";
      $("#sc-dirs").textContent = count(p.dirs);
      $("#sc-dirs").nextElementSibling.textContent = "carpetas";
      $("#sc-bytes").textContent = fmtBytes(p.bytes);
      if (p.cur) $("#scan-cur").textContent = p.cur;
    }
    $("#sc-time").textContent = fmtSecs(p.seconds);
  }

  async function onScanned(info) {
    S.scanning = false;
    S.scan = info;
    S.hist = [];
    S.sel.clear();
    S.cands = null; S.growth = null; S.cleanSel.clear(); S.openCards.clear();
    S.bigCat = -1; S.hlCat = -1;
    S.tab = "explorar";
    setTab("explorar", true);
    if (info.note) toast(info.note, "error");
    show("results");
    renderDisk();
    await go(info.rootId, { push: false });
    loadCleanupSummary();
  }

  function onScanFail(e) {
    S.scanning = false;
    if (e && e.error) toast("No se pudo analizar: " + e.error, "error");
    show(S.scan ? "results" : "home");
    if (!S.scan) refreshHome();
  }

  async function refreshHome() {
    try { S.info.drives = await api.Drives(); } catch { }
    renderHome();
  }

  // ---------- navegación ----------
  async function go(id, opt = {}) {
    const { push = true, focus = null, keepSearch = false } = opt;
    if (push && S.cur != null && S.cur !== id) S.hist.push({ id: S.cur, path: S.curPath });
    let L;
    try { L = await api.Listing(id); }
    catch (e) {
      // el elemento ya no existe (se borró o se volvió a leer): se busca por ruta
      const target = opt.path || S.curPath;
      if (target) {
        try { const nid = await api.Locate(target); if (nid !== id && nid >= 0) return go(nid, { ...opt, push: false }); } catch { }
      }
      toast(String(e), "error");
      return;
    }
    S.cur = id;
    S.curPath = L.path;
    S.listing = L;
    S.focus = focus;
    if (!keepSearch) clearSearch(false);
    renderCrumbs();
    if (!S.search) mainList.set(L.items, { total: L.node.size, more: L.more, moreSize: L.moreSize, paths: false, focus });
    loadTreemap();
    loadTypes();
    if (S.tab === "grandes") loadBig();
    $("#nav-back").disabled = S.hist.length === 0;
    $("#nav-up").disabled = L.crumbs.length <= 1;
  }

  function goUp() {
    const c = S.listing && S.listing.crumbs;
    if (c && c.length > 1) go(c[c.length - 2].id, { focus: S.cur });
  }
  function goBack() {
    const h = S.hist.pop();
    if (h) go(h.id, { push: false, path: h.path });
  }

  function renderCrumbs() {
    const c = S.listing.crumbs;
    // la ruta entera: si no cabe, se estrechan las carpetas del medio (nunca se ocultan)
    let html = "";
    c.forEach((x, i) => {
      if (i > 0) html += `<span class="crumb-sep">${ic("chev")}</span>`;
      const last = i === c.length - 1;
      const name = i === 0 ? x.name.replace(/\\$/, "") : x.name;
      html += `<button class="crumb${last ? " last" : ""}${i === 0 ? " root" : ""}" data-id="${x.id}" title="${esc(x.name)}">${esc(name)}</button>`;
    });
    $("#crumbs").innerHTML = html;
    $("#crumbs").title = S.curPath;
    $("#copy-path").title = "Copiar la ruta: " + S.curPath;
    $$("#crumbs .crumb:not(.last)").forEach((b) => b.addEventListener("click", () => go(+b.dataset.id)));
  }
  $("#copy-path").addEventListener("click", async (e) => {
      const b = e.currentTarget;
      try {
        await api.CopyPath(S.cur);
        b.classList.add("done"); b.innerHTML = ic("check");
        setTimeout(() => { b.classList.remove("done"); b.innerHTML = ic("copy"); }, 1400);
        toast("Ruta copiada");
      } catch (er) { toast(String(er), "error"); }
  });

  // ---------- lista virtual ----------
  const ROW = 40;
  function makeList(box, opt) {
    box.tabIndex = 0;
    box.innerHTML = `<div class="lhead"></div><div class="lscroll"><div class="lspace"></div></div>`;
    const head = $(".lhead", box), scroller = $(".lscroll", box), space = $(".lspace", box);
    const L = { items: [], total: 0, max: 0, more: 0, moreSize: 0, paths: false, sort: opt.sort || "size", dir: -1, focus: -1, empty: opt.empty };

    function sortItems() {
      const k = L.sort, d = L.dir;
      const f = {
        size: (a, b) => (a.size - b.size) * d || a.name.localeCompare(b.name, "es"),
        name: (a, b) => a.name.localeCompare(b.name, "es", { numeric: true }) * -d,
        files: (a, b) => ((a.dir ? a.files : 0) - (b.dir ? b.files : 0)) * d || (a.size - b.size) * -1,
        mtime: (a, b) => (a.mtime - b.mtime) * d,
      }[k];
      L.items.sort(f);
    }
    function renderHead() {
      const col = (k, label, cls = "") => `<span class="${cls}${L.sort === k ? " on" : ""}" data-k="${k}">${label}${L.sort === k ? ic(L.dir < 0 ? "down" : "up") : ""}</span>`;
      head.innerHTML = `<span></span><span></span>${col("name", L.paths ? "Nombre y ubicación" : "Nombre")}${col("size", "Tamaño", "")}<span class="r"></span>` +
        (L.paths ? "" : col("files", "Archivos", "r")) + col("mtime", "Modificado", "r") + `<span></span>`;
      $$("span[data-k]", head).forEach((s) => s.addEventListener("click", () => {
        const k = s.dataset.k;
        if (L.sort === k) L.dir = -L.dir; else { L.sort = k; L.dir = k === "name" ? 1 : -1; }
        if (k === "name") L.dir = L.sort === k ? L.dir : 1;
        const fid = L.items[L.focus] ? L.items[L.focus].id : null;
        sortItems(); renderHead();
        L.focus = fid == null ? -1 : L.items.findIndex((x) => x.id === fid);
        render();
      }));
    }
    function rowHTML(it, i) {
      const selected = S.sel.has(it.id);
      const bw = L.max > 0 ? Math.max(0.4, it.size / L.max * 100) : 0;
      let tags = "";
      if (it.flags & 8) tags += `<span class="tag">enlace</span>`;
      if (it.flags & 2) tags += `<span class="tag lock">sin acceso</span>`;
      if (it.flags & 64) tags += `<span class="tag cloud">en la nube</span>`;
      let delta = "";
      if (it.new) delta = ` <span class="dl new">nueva</span>`;
      else if (it.delta) delta = ` <span class="dl ${it.delta > 0 ? "up" : "down"}">${it.delta > 0 ? "▲" : "▼"} ${fmtBytes(Math.abs(it.delta))}</span>`;
      const icon = it.dir ? `<div class="ic dir">${ic("folder")}</div>` : `<div class="ic file" style="--cc:${catColor(it.cat)}">${ic(catIcons[it.cat] || "file")}</div>`;
      const sub = L.paths ? `<span class="p">${esc(it.path || "(aquí)")}</span>` : "";
      return `<div class="row-i${it.dir ? " dirrow" : ""}${selected ? " sel" : ""}${i === L.focus ? " focus" : ""}" data-i="${i}" style="top:${i * ROW}px">
        <span class="ck" data-ck>${ic("check")}</span>${icon}
        <div class="nm"><span class="t">${esc(it.name)}${tags}${delta}</span>${sub}</div>
        <div class="bar"><div class="b"><i style="width:${bw.toFixed(2)}%"></i></div><span class="pc">${pct(it.size, L.total)}</span></div>
        <span class="sz r">${fmtBytes(it.size)}</span>
        ${L.paths ? "" : `<span class="num r">${it.dir ? count(it.files) : ""}</span>`}
        <span class="num r">${fmtDate(it.mtime)}</span>
        <button class="ibtn danger act" data-del title="Borrar">${ic("trash")}</button>
      </div>`;
    }
    let raf = 0;
    function render() {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        const n = L.items.length;
        const h = scroller.clientHeight || 400;
        const top = scroller.scrollTop;
        const a = Math.max(0, Math.floor(top / ROW) - 6), b = Math.min(n, Math.ceil((top + h) / ROW) + 6);
        let html = "";
        for (let i = a; i < b; i++) html += rowHTML(L.items[i], i);
        if (L.more && b === n) html += `<div class="more-row" style="top:${n * ROW}px">y ${count(L.more)} elementos más pequeños (${fmtBytes(L.moreSize)})</div>`;
        if (!n) html = `<div class="empty">${L.empty ? L.empty() : `${ic("folder")}<b>Carpeta vacía</b>`}</div>`;
        space.style.height = n ? (n + (L.more ? 1 : 0)) * ROW + "px" : "100%";
        space.innerHTML = html;
      });
    }
    scroller.addEventListener("scroll", render, { passive: true });
    new ResizeObserver(render).observe(scroller);

    function rowOf(e) { const r = e.target.closest(".row-i"); return r ? +r.dataset.i : -1; }
    function setFocus(i, scroll = true) {
      if (i < 0 || i >= L.items.length) return;
      L.focus = i;
      if (scroll) {
        const y = i * ROW, h = scroller.clientHeight;
        if (y < scroller.scrollTop) scroller.scrollTop = y;
        else if (y + ROW > scroller.scrollTop + h) scroller.scrollTop = y + ROW - h;
      }
      render();
      if (opt.onFocus) opt.onFocus(L.items[i]);
    }
    space.addEventListener("click", (e) => {
      const i = rowOf(e);
      if (i < 0) return;
      const it = L.items[i];
      if (e.target.closest("[data-ck]")) { toggleSel(it, L); render(); return; }
      if (e.target.closest("[data-del]")) { setFocus(i, false); askDelete([selItem(it, L)]); return; }
      if (e.ctrlKey || S.selMode && !it.dir) { toggleSel(it, L); setFocus(i, false); box.focus(); return; }
      if (it.dir) { openItem(it, L); return; }
      setFocus(i, false);
      box.focus();
    });
    space.addEventListener("dblclick", (e) => {
      const i = rowOf(e);
      if (i < 0 || e.target.closest("[data-ck],[data-del]") || L.items[i].dir) return;
      openItem(L.items[i], L);
    });
    space.addEventListener("contextmenu", (e) => {
      const i = rowOf(e);
      if (i < 0) return;
      e.preventDefault();
      setFocus(i, false);
      showCtx(e.clientX, e.clientY, selItem(L.items[i], L));
    });
    box.addEventListener("keydown", (e) => {
      const n = L.items.length;
      if (!n) return;
      const page = Math.max(1, Math.floor(scroller.clientHeight / ROW) - 1);
      const k = e.key;
      if (k === "ArrowDown") setFocus(Math.min(n - 1, L.focus + 1));
      else if (k === "ArrowUp") setFocus(Math.max(0, L.focus - 1));
      else if (k === "PageDown") setFocus(Math.min(n - 1, L.focus + page));
      else if (k === "PageUp") setFocus(Math.max(0, L.focus - page));
      else if (k === "Home") setFocus(0);
      else if (k === "End") setFocus(n - 1);
      else if (k === "Enter" && L.items[L.focus]) openItem(L.items[L.focus], L);
      else if (k === " " && L.items[L.focus]) { toggleSel(L.items[L.focus], L); render(); }
      else if ((k === "a" || k === "A") && e.ctrlKey) { L.items.forEach((it) => { if (!S.sel.has(it.id)) toggleSel(it, L, true); }); render(); }
      else if (k === "Delete") {
        if (S.sel.size) askDelete([...S.sel.values()], e.shiftKey);
        else if (L.items[L.focus]) askDelete([selItem(L.items[L.focus], L)], e.shiftKey);
      } else return;
      e.preventDefault();
    });

    return {
      box,
      set(items, o = {}) {
        L.items = items.slice();
        L.total = o.total || 0;
        L.more = o.more || 0; L.moreSize = o.moreSize || 0;
        L.paths = !!o.paths;
        L.base = o.base || S.curPath;
        L.max = items.reduce((m, x) => Math.max(m, x.size), 0);
        if (!L.paths) L.max = Math.max(L.max, 1);
        if (L.sort !== "size" || L.dir !== -1) sortItems();
        box.classList.toggle("paths", L.paths);
        renderHead();
        L.focus = o.focus != null ? L.items.findIndex((x) => x.id === o.focus) : -1;
        scroller.scrollTop = 0;
        if (L.focus >= 0) setFocus(L.focus); else render();
        if (o.focus != null && L.focus >= 0) flashRow(L.focus);
      },
      render,
      focusId(id) { const i = L.items.findIndex((x) => x.id === id); if (i >= 0) { setFocus(i); flashRow(i); box.focus(); } return i >= 0; },
      state: L,
    };
    function flashRow(i) {
      setTimeout(() => { const r = $(`.row-i[data-i="${i}"]`, space); if (r) { r.classList.remove("flash"); void r.offsetWidth; r.classList.add("flash"); } }, 60);
    }
  }

  function fullPath(it, L) {
    const base = (L && L.base) || S.curPath;
    const join = (a, b) => a.replace(/\\$/, "") + "\\" + b;
    if (L && L.paths) return join(it.path ? join(base, it.path) : base, it.name);
    return join(base, it.name);
  }
  function selItem(it, L) { return { id: it.id, name: it.name, size: it.size, dir: it.dir, flags: it.flags, cat: it.cat, path: it.fullPath || fullPath(it, L) }; }
  function toggleSel(it, L, force) {
    if (S.sel.has(it.id) && !force) S.sel.delete(it.id); else S.sel.set(it.id, selItem(it, L));
    renderSelbar();
  }
  function renderSelbar() {
    const n = S.sel.size;
    $("#selbar").classList.toggle("on", n > 0 && (S.tab === "explorar" || S.tab === "grandes"));
    if (!n) return;
    let tot = 0;
    for (const v of S.sel.values()) tot += v.size;
    $("#sel-count").textContent = n === 1 ? "1 elemento seleccionado" : count(n) + " elementos seleccionados";
    $("#sel-size").textContent = fmtBytes(tot) + " en total";
  }

  function openItem(it, L) {
    if (it.dir) go(it.id);
    else api.Open(it.id).catch((e) => toast(String(e), "error"));
  }

  const mainList = makeList($("#list"), {
    empty: () => S.search ? `${ic("alert")}<b>Nada coincide con «${esc(S.search)}»</b><p>Se busca por nombre dentro de la carpeta actual y todas sus subcarpetas.</p>` : `${ic("folder")}<b>Carpeta vacía</b>`,
    onFocus: (it) => { TM.selected = it ? it.id : null; drawHL(); },
  });
  const bigList = makeList($("#big"), {
    empty: () => `${ic("file")}<b>No hay archivos aquí</b>`,
  });

  // ---------- búsqueda ----------
  const doSearch = debounce(async () => {
    const q = $("#q").value.trim();
    S.search = q;
    $(".search").classList.toggle("has", !!q);
    if (!q) { clearSearch(true); return; }
    if (S.tab !== "explorar") setTab("explorar");
    try {
      const items = await api.Search(S.cur, q);
      if (S.search !== q) return;
      mainList.set(items, { total: S.listing.node.size, paths: true, base: S.curPath });
    } catch (e) { toast(String(e), "error"); }
  }, 220);
  function clearSearch(rerender) {
    const had = !!S.search;
    S.search = "";
    $("#q").value = "";
    $(".search").classList.remove("has");
    if (rerender && had && S.listing) mainList.set(S.listing.items, { total: S.listing.node.size, more: S.listing.more, moreSize: S.listing.moreSize });
  }
  $("#q").addEventListener("input", doSearch);
  $("#q").addEventListener("keydown", (e) => {
    if (e.key === "Escape") { clearSearch(true); $("#list").focus(); }
    if (e.key === "ArrowDown") { e.preventDefault(); $("#list").focus(); }
  });
  $("#q-clear").addEventListener("click", () => { clearSearch(true); $("#q").focus(); });

  // ---------- mapa (treemap) ----------
  const TM = { data: null, rects: [], hover: null, selected: null, area: 0, w: 0, h: 0, req: 0 };

  async function loadTreemap() {
    const wrap = $("#tm-wrap");
    const w = wrap.clientWidth, h = wrap.clientHeight;
    if (!S.showMap || !w || !h || S.cur == null) return;
    const req = ++TM.req;
    try {
      const d = await api.Treemap(S.cur, w * h);
      if (req !== TM.req) return;
      TM.data = d; TM.area = w * h;
      drawTreemap();
    } catch { }
  }

  function shade(hex, f) {
    const n = parseInt(hex.slice(1), 16);
    let r = n >> 16, g = (n >> 8) & 255, b = n & 255;
    if (f > 0) { r += (255 - r) * f; g += (255 - g) * f; b += (255 - b) * f; }
    else { r *= 1 + f; g *= 1 + f; b *= 1 + f; }
    return `rgb(${r | 0},${g | 0},${b | 0})`;
  }
  const shadeCache = {};
  function cushion(ctx, color, x, y, w, h) {
    let c = shadeCache[color];
    if (!c) c = shadeCache[color] = [shade(color, 0.32), color, shade(color, -0.42)];
    if (w < 6 || h < 6) { ctx.fillStyle = c[1]; ctx.fillRect(x, y, w, h); return; }
    const g = ctx.createLinearGradient(x, y, x + w * 0.7, y + h);
    g.addColorStop(0, c[0]); g.addColorStop(0.45, c[1]); g.addColorStop(1, c[2]);
    ctx.fillStyle = g;
    ctx.fillRect(x, y, w, h);
  }

  function worst(row, sum, side) {
    let mx = 0, mn = Infinity;
    for (const a of row) { if (a > mx) mx = a; if (a < mn) mn = a; }
    const s2 = sum * sum, w2 = side * side;
    return Math.max(w2 * mx / s2, s2 / (w2 * mn));
  }
  // squarify: reparte rectángulos lo más cuadrados posible
  function squarify(kids, x, y, w, h, out) {
    const total = kids.reduce((s, k) => s + k.s, 0);
    if (total <= 0 || w <= 0 || h <= 0) return;
    const scale = (w * h) / total;
    let i = 0;
    while (i < kids.length) {
      const side = Math.min(w, h);
      const row = [kids[i].s * scale];
      let sum = row[0], best = worst(row, sum, side), j = i + 1;
      while (j < kids.length) {
        const a = kids[j].s * scale;
        row.push(a);
        const wr = worst(row, sum + a, side);
        if (wr > best) { row.pop(); break; }
        sum += a; best = wr; j++;
      }
      const thick = sum / side;
      if (w >= h) {
        let yy = y;
        for (let k = i; k < j; k++) { const hh = kids[k].s * scale / thick; out.push([kids[k], x, yy, thick, hh]); yy += hh; }
        x += thick; w -= thick;
      } else {
        let xx = x;
        for (let k = i; k < j; k++) { const ww = kids[k].s * scale / thick; out.push([kids[k], xx, y, ww, thick]); xx += ww; }
        y += thick; h -= thick;
      }
      i = j;
    }
  }

  function drawTreemap() {
    const wrap = $("#tm-wrap");
    const w = wrap.clientWidth, h = wrap.clientHeight;
    const dpr = window.devicePixelRatio || 1;
    TM.w = w; TM.h = h;
    for (const c of [$("#tm"), $("#tm-hl")]) { c.width = Math.round(w * dpr); c.height = Math.round(h * dpr); }
    const ctx = $("#tm").getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = "#07080d";
    ctx.fillRect(0, 0, w, h);
    TM.rects = [];
    const d = TM.data;
    $("#tm-empty").hidden = !!(d && d.s > 0);
    if (!d || d.s <= 0) { drawHL(); return; }
    ctx.font = "600 11px 'Segoe UI Variable Text', 'Segoe UI', sans-serif";
    ctx.textBaseline = "middle";
    const node = (n, x, y, w, h, depth, parent) => {
      if (w < 0.5 || h < 0.5) return;
      const r = { n, x, y, w, h, depth, parent };
      TM.rects.push(r);
      const dim = S.hlCat >= 0 && (n.d || n.i < 0 || n.k !== S.hlCat);
      if (n.d && n.c && n.c.length) {
        let ix = x, iy = y, iw = w, ih = h;
        if (depth > 0) {
          ctx.fillStyle = depth % 2 ? "#161927" : "#1c2032";
          ctx.fillRect(x, y, w - 1, h - 1);
          if (w > 56 && h > 40 && depth <= 5) {
            ctx.fillStyle = "rgba(255,255,255,.07)";
            ctx.fillRect(x, y, w - 1, 16);
            ctx.save();
            ctx.beginPath(); ctx.rect(x + 4, y, w - 10, 16); ctx.clip();
            ctx.fillStyle = S.hlCat >= 0 ? "#6b7190" : "#c9cce0";
            ctx.fillText(n.n || "", x + 6, y + 8.5);
            ctx.restore();
            iy += 17; ih -= 18; ix += 1; iw -= 3;
          } else { ix += 1; iy += 1; iw -= 3; ih -= 3; }
        }
        const out = [];
        squarify(n.c, ix, iy, iw, ih, out);
        for (const [c, cx, cy, cw, ch] of out) node(c, cx, cy, cw, ch, depth + 1, r);
        return;
      }
      ctx.globalAlpha = dim ? 0.16 : 1;
      if (n.i < 0) {
        ctx.fillStyle = "#232738";
        ctx.fillRect(x, y, w - 1, h - 1);
      } else if (n.d) {
        cushion(ctx, "#3a4160", x, y, w - 1, h - 1);
      } else {
        cushion(ctx, catColor(n.k), x, y, w - 1, h - 1);
      }
      ctx.globalAlpha = 1;
      if (w > 70 && h > 22 && n.i >= 0 && !dim) {
        ctx.save();
        ctx.beginPath(); ctx.rect(x + 3, y, w - 8, h); ctx.clip();
        ctx.fillStyle = "rgba(0,0,0,.55)";
        ctx.fillText(n.n || "", x + 6, y + 10.5);
        ctx.restore();
      }
    };
    node(d, 0, 0, w, h, 0, null);
    drawHL();
  }

  function rectAt(px, py) {
    for (let i = TM.rects.length - 1; i >= 0; i--) {
      const r = TM.rects[i];
      if (px >= r.x && px < r.x + r.w && py >= r.y && py < r.y + r.h) return r;
    }
    return null;
  }
  function drawHL() {
    const c = $("#tm-hl");
    const ctx = c.getContext("2d");
    const dpr = window.devicePixelRatio || 1;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, TM.w, TM.h);
    const outline = (r, color, lw) => { ctx.strokeStyle = color; ctx.lineWidth = lw; ctx.strokeRect(r.x + lw / 2, r.y + lw / 2, Math.max(0, r.w - lw - 1), Math.max(0, r.h - lw - 1)); };
    if (TM.selected != null) {
      const r = TM.rects.find((x) => x.n.i === TM.selected);
      if (r) { ctx.fillStyle = "rgba(255,255,255,.12)"; ctx.fillRect(r.x, r.y, r.w - 1, r.h - 1); outline(r, "#fff", 2); }
    }
    const h = TM.hover;
    if (h) {
      if (h.parent && h.parent.depth > 0) outline(h.parent, "rgba(196,181,253,.55)", 1);
      outline(h, "rgba(255,255,255,.9)", 1.5);
    }
  }
  function pathInMap(r) {
    const parts = [];
    for (let p = r; p && p.depth > 0; p = p.parent) parts.unshift(p.n.n);
    return parts;
  }
  const hl = $("#tm-hl");
  hl.addEventListener("mousemove", (e) => {
    const b = hl.getBoundingClientRect();
    const r = rectAt(e.clientX - b.left, e.clientY - b.top);
    if (r !== TM.hover) { TM.hover = r; drawHL(); }
    const tip = $("#tip");
    if (!r || r.depth === 0) { tip.hidden = true; return; }
    const parts = pathInMap(r);
    const name = r.n.i < 0 ? "Archivos pequeños" : r.n.n;
    const where = (r.n.i < 0 ? parts : parts.slice(0, -1)).join("\\");
    const color = r.n.i < 0 ? "#232738" : r.n.d ? "#6366f1" : catColor(r.n.k);
    const kind = r.n.i < 0 ? "varios archivos" : r.n.d ? "carpeta" : catLabel(r.n.k).toLowerCase();
    tip.innerHTML = `<b>${esc(name)}</b>${where ? `<div class="tp">${esc(where)}</div>` : ""}<div class="tz"><i style="background:${color}"></i><em>${fmtBytes(r.n.s)}</em><span>${pct(r.n.s, TM.data.s)} · ${esc(kind)}</span></div>`;
    tip.hidden = false;
    const app = $("#app").getBoundingClientRect();
    let x = e.clientX + 14, y = e.clientY + 16;
    const tw = tip.offsetWidth, th = tip.offsetHeight;
    if (x + tw > app.width - 8) x = e.clientX - tw - 14;
    if (y + th > app.height - 8) y = e.clientY - th - 12;
    tip.style.left = x + "px"; tip.style.top = y + "px";
  });
  hl.addEventListener("mouseleave", () => { TM.hover = null; drawHL(); $("#tip").hidden = true; });
  hl.addEventListener("click", (e) => {
    const r = TM.hover;
    if (!r || r.depth === 0) return;
    let target = r.n.i < 0 ? r.parent : r;
    if (!target || target.depth === 0) return;
    if (target.depth === 1) { mainList.focusId(target.n.i); TM.selected = target.n.i; drawHL(); }
    else go(target.parent.n.i, { focus: target.n.i });
  });
  hl.addEventListener("dblclick", (e) => {
    const r = TM.hover;
    if (!r) return;
    let t = r.n.i < 0 ? r.parent : r;
    while (t && !t.n.d) t = t.parent;
    if (t && t.depth > 0) go(t.n.i);
  });
  hl.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    const r = TM.hover;
    if (!r || r.depth === 0 || r.n.i < 0) return;
    const path = S.curPath.replace(/\\$/, "") + "\\" + pathInMap(r).join("\\");
    showCtx(e.clientX, e.clientY, { id: r.n.i, name: r.n.n, size: r.n.s, dir: !!r.n.d, flags: 0, path });
  });

  const resizeTreemap = debounce((force) => {
    const wrap = $("#tm-wrap");
    const a = wrap.clientWidth * wrap.clientHeight;
    if (!a) return;
    if (force || !TM.data || a > TM.area * 1.25 || a < TM.area * 0.6) loadTreemap(); else drawTreemap();
  }, 120);
  new ResizeObserver(() => resizeTreemap()).observe($("#tm-wrap"));

  // separador entre lista y mapa
  (() => {
    const sp = $("#splitter"), wrap = $("#tm-wrap");
    const saved = store.get("tmHeight", null);
    if (saved) wrap.style.height = saved + "px";
    let startY = 0, startH = 0;
    const move = (e) => {
      const pane = $("#p-explorar").clientHeight;
      const hgt = Math.max(120, Math.min(pane - 160, startH - (e.clientY - startY)));
      wrap.style.height = hgt + "px";
    };
    const up = () => {
      sp.classList.remove("drag");
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
      store.set("tmHeight", wrap.clientHeight);
    };
    sp.addEventListener("mousedown", (e) => {
      e.preventDefault();
      startY = e.clientY; startH = wrap.clientHeight;
      sp.classList.add("drag");
      window.addEventListener("mousemove", move);
      window.addEventListener("mouseup", up);
    });
  })();

  // ---------- panel lateral ----------
  function renderDisk() {
    const s = S.scan;
    if (!s) return;
    const used = s.total - s.free;
    const other = Math.max(0, used - s.scanned);
    const vol = s.root.slice(0, 2);
    const w = (v) => s.total ? Math.max(0, v / s.total * 100).toFixed(2) + "%" : "0";
    let html;
    if (s.isDrive) {
      const [n, u] = splitBytes(s.free);
      html = `<div class="panel-top"><span class="label">Disco ${esc(vol)}</span><span class="label" style="text-transform:none;letter-spacing:0">${pct(used, s.total)} usado</span></div>
        <div class="big">${n}<small>${u} libres</small></div><div class="sub">de ${fmtBytes(s.total)} en total</div>
        <div class="stack"><i class="s1" style="width:${w(s.scanned)}"></i><i class="s2" style="width:${w(other)}"></i><i class="s3" style="flex:1"></i></div>
        <div class="legend">
          <div class="lg"><i style="background:var(--grad)"></i><span>Archivos analizados</span><b>${fmtBytes(s.scanned)}</b></div>
          <div class="lg"><i style="background:#475069"></i><span>Sistema y sin acceso</span><b>${fmtBytes(other)}</b></div>
          <div class="lg"><i style="background:rgba(255,255,255,.12)"></i><span>Libre</span><b>${fmtBytes(s.free)}</b></div>
        </div>`;
    } else {
      const [n, u] = splitBytes(s.scanned);
      html = `<div class="panel-top"><span class="label">Esta carpeta</span></div>
        <div class="big">${n}<small>${u}</small></div><div class="sub">${pct(s.scanned, s.total)} del disco ${esc(vol)} · quedan ${fmtBytes(s.free)} libres</div>
        <div class="stack"><i class="s1" style="width:${w(s.scanned)}"></i><i class="s2" style="width:${w(other)}"></i><i class="s3" style="flex:1"></i></div>
        <div class="legend">
          <div class="lg"><i style="background:var(--grad)"></i><span>Esta carpeta</span><b>${fmtBytes(s.scanned)}</b></div>
          <div class="lg"><i style="background:#475069"></i><span>Resto del disco</span><b>${fmtBytes(other)}</b></div>
          <div class="lg"><i style="background:rgba(255,255,255,.12)"></i><span>Libre</span><b>${fmtBytes(s.free)}</b></div>
        </div>`;
    }
    html += `<div class="stats"><span><b>${count(s.files)}</b> archivos</span><span><b>${count(s.dirs)}</b> carpetas</span><span>analizado ${ago(s.when)}</span></div>`;
    if (s.denied > 0 && s.mode !== "mft") {
      html += `<div class="denied">${ic("lock")}<span>${count(s.denied)} carpetas sin acceso no se han podido medir.</span></div>`;
    }
    if (!S.info.admin) {
      html += `<div class="turbo-mini"><b>${ic("bolt")}Modo turbo</b>Lee el índice del disco directamente: varias veces más rápido y sin carpetas sin acceso.
        <button class="btn amber sm" id="side-turbo">${ic("bolt")}Reiniciar como administrador</button></div>`;
    }
    $("#disk").innerHTML = html;
    const b = $("#side-turbo");
    if (b) b.addEventListener("click", restartAdmin);
  }

  async function loadTypes() {
    const id = S.cur;
    try {
      const t = await api.Types(id);
      if (id !== S.cur) return;
      S.types = t;
      renderTypes();
    } catch { }
  }
  function renderTypes() {
    const t = S.types;
    if (!t) return;
    const total = t.cats.reduce((s, c) => s + c.size, 0) || 1;
    let html = "";
    if (S.typeMode === "cat") {
      html = t.cats.filter((c) => c.size > 0).map((c) => `<div class="trow${S.hlCat === c.cat ? " on" : ""}" data-cat="${c.cat}" title="Ver los archivos más grandes de este tipo">
        <i style="background:${catColor(c.cat)}"></i><span class="tn">${esc(catLabel(c.cat))}<small>${count(c.files)}</small></span><span class="tv">${fmtBytes(c.size)}</span>
        <div class="tb"><u style="width:${(c.size / total * 100).toFixed(2)}%;background:${catColor(c.cat)}"></u></div></div>`).join("");
    } else {
      const max = t.exts.length ? t.exts[0].size : 1;
      html = t.exts.slice(0, 18).map((e) => `<div class="trow" data-cat="${e.cat}">
        <i style="background:${catColor(e.cat)}"></i><span class="tn ext">.${esc(e.ext || "(sin extensión)")}<small>${count(e.files)}</small></span><span class="tv">${fmtBytes(e.size)}</span>
        <div class="tb"><u style="width:${(e.size / max * 100).toFixed(2)}%;background:${catColor(e.cat)}"></u></div></div>`).join("");
    }
    $("#types").innerHTML = html || `<div class="note">Sin archivos</div>`;
    $$("#types .trow").forEach((r) => {
      const cat = +r.dataset.cat;
      r.addEventListener("mouseenter", () => { S.hlCat = cat; drawTreemap(); });
      r.addEventListener("mouseleave", () => { S.hlCat = -1; drawTreemap(); });
      r.addEventListener("click", () => { S.bigCat = cat; setTab("grandes"); });
    });
  }
  $$("#types-mode button").forEach((b) => b.addEventListener("click", () => {
    S.typeMode = b.dataset.m;
    $$("#types-mode button").forEach((x) => x.classList.toggle("on", x === b));
    renderTypes();
  }));

  async function loadCleanupSummary() {
    try {
      S.cands = await api.Cleanup();
    } catch { S.cands = []; }
    if (!S.cleanSel.size) S.cands.filter((c) => c.level === "seguro" && c.id !== "papelera").forEach((c) => S.cleanSel.add(c.id));
    renderQuick();
    if (S.tab === "limpieza") renderClean();
  }
  function renderQuick() {
    const c = S.cands || [];
    const safe = c.filter((x) => x.level === "seguro").reduce((s, x) => s + x.size, 0);
    const review = c.filter((x) => x.level === "revisar").reduce((s, x) => s + x.size, 0);
    $("#clean-badge").textContent = safe > 0 ? fmtBytes(safe) : "";
    const [n, u] = splitBytes(safe);
    $("#quick").innerHTML = `<div class="panel-top"><span class="label">Limpieza rápida</span></div>
      <div class="q-big">${n}<small>${u}</small></div>
      <p>se pueden liberar sin riesgo (temporales, cachés, papelera…)${review > 0 ? `, y otros <b>${fmtBytes(review)}</b> que conviene revisar.` : "."}</p>
      <button class="btn primary sm" id="quick-go">${ic("broom")}Ver qué se puede borrar</button>`;
    $("#quick-go").addEventListener("click", () => setTab("limpieza"));
  }

  // ---------- pestañas ----------
  function setTab(tab, silent) {
    S.tab = tab;
    $$("#tabs button").forEach((b) => b.classList.toggle("on", b.dataset.tab === tab));
    for (const t of ["explorar", "grandes", "limpieza", "crecido"]) $("#p-" + t).hidden = t !== tab;
    renderSelbar();
    if (silent) return;
    if (tab === "grandes") loadBig();
    if (tab === "limpieza") { if (S.cands) renderClean(); else loadCleanupSummary(); }
    if (tab === "crecido") loadGrowth();
    if (tab === "explorar") requestAnimationFrame(() => { mainList.render(); resizeTreemap(); });
  }
  $$("#tabs button").forEach((b) => b.addEventListener("click", () => setTab(b.dataset.tab)));

  async function loadBig() {
    const id = S.cur;
    try {
      const items = await api.TopFiles(id, S.bigCat);
      if (id !== S.cur) return;
      const folder = S.listing ? S.listing.crumbs[S.listing.crumbs.length - 1].name : "";
      const total = items.reduce((s, x) => s + x.size, 0);
      $("#big-head").innerHTML = `<span>Los <b>${count(items.length)}</b> archivos más grandes de <b>${esc(folder.replace(/\\$/, ""))}</b> y sus subcarpetas · ${fmtBytes(total)}</span>` +
        (S.bigCat >= 0 ? `<span class="chip" style="cursor:pointer" id="big-cat"><i style="width:9px;height:9px;border-radius:3px;background:${catColor(S.bigCat)}"></i>Solo ${esc(catLabel(S.bigCat).toLowerCase())} ✕</span>` : "");
      const bc = $("#big-cat");
      if (bc) bc.addEventListener("click", () => { S.bigCat = -1; loadBig(); });
      bigList.set(items, { total: S.listing ? S.listing.node.size : 0, paths: true, base: S.curPath });
    } catch (e) { toast(String(e), "error"); }
  }

  // ---------- limpieza ----------
  function renderClean() {
    const c = S.cands || [];
    const el = $("#clean");
    if (!c.length) {
      el.innerHTML = `<div class="empty">${ic("check")}<b>No hay nada evidente que limpiar</b><p>No se han encontrado temporales, cachés ni descargas viejas en lo analizado.</p></div>`;
      return;
    }
    const safe = c.filter((x) => x.level === "seguro").reduce((s, x) => s + x.size, 0);
    const groups = [["seguro", "Seguro de borrar", "Se regeneran solos · se borran directamente, sin pasar por la papelera"], ["revisar", "Revisa antes de borrar", "Puede que aún los quieras: mira dónde están"], ["info", "Para que lo sepas", "Ocupan mucho, pero no se borran desde aquí"]];
    let html = `<div class="clean-head"><div class="tile">${ic("broom")}</div><div class="txt"><h2>Puedes liberar ${fmtBytes(safe)} sin riesgo</h2>
      <p>Temporales, cachés y papelera de lo que has analizado${S.scan && !S.scan.isDrive ? " (solo dentro de esta carpeta)" : ""}. Marca lo que quieras y pulsa limpiar.</p></div></div>`;
    for (const [lvl, title, sub] of groups) {
      const items = c.filter((x) => x.level === lvl);
      if (!items.length) continue;
      html += `<div class="group-t"><span>${title}</span><span style="color:var(--dim);font-weight:500">${sub}</span><span class="gl"></span></div><div class="cards">`;
      html += items.map((x, i) => cardHTML(x, i)).join("");
      html += `</div>`;
    }
    html += `<div class="clean-foot"><div class="sum" id="clean-sum"></div><button class="btn primary" id="clean-go">${ic("broom")}Limpiar lo marcado</button></div>`;
    el.innerHTML = html;
    $$("#clean .card").forEach((card) => {
      const id = card.dataset.id;
      $(".card-top", card).addEventListener("click", (e) => {
        if (e.target.closest("[data-ck]") && card.dataset.level !== "info") {
          if (S.cleanSel.has(id)) S.cleanSel.delete(id); else S.cleanSel.add(id);
          card.classList.toggle("on", S.cleanSel.has(id));
          $("[data-ck]", card).classList.toggle("on", S.cleanSel.has(id));
          updateCleanSum();
          return;
        }
        card.classList.toggle("open");
        if (card.classList.contains("open")) S.openCards.add(id); else S.openCards.delete(id);
      });
      $$("[data-reveal]", card).forEach((b) => b.addEventListener("click", (e) => {
        e.stopPropagation();
        api.RevealPath(b.dataset.reveal).catch((er) => toast(String(er), "error"));
      }));
      $$("[data-goto]", card).forEach((b) => b.addEventListener("click", (e) => {
        e.stopPropagation();
        setTab("explorar");
        go(+b.dataset.goto);
      }));
    });
    $("#clean-go").addEventListener("click", askClean);
    updateCleanSum();
  }
  function cardHTML(x, i) {
    const on = S.cleanSel.has(x.id);
    const open = S.openCards.has(x.id);
    const where = (x.where || []).map((w) => `<div class="where"><span class="wp" title="${esc(w.path)}">${esc(w.path)}</span><span class="wz">${fmtBytes(w.size)}</span>
      ${w.id >= 0 ? `<button class="ibtn" data-goto="${w.id}" title="Ver en TurboCUT">${ic("enter")}</button>` : `<button class="ibtn" data-reveal="${esc(w.path)}" title="Abrir en el Explorador">${ic("reveal")}</button>`}</div>`).join("");
    const what = x.id === "papelera" ? count(x.count) + " elementos" : x.count ? count(x.count) + (x.count === 1 ? " archivo" : " archivos") : "";
    return `<div class="card${on ? " on" : ""}${open ? " open" : ""}" data-id="${esc(x.id)}" data-level="${x.level}" style="animation-delay:${i * 0.03}s">
      <div class="card-top">
        ${x.level === "info" ? "<span></span>" : `<span class="ck${on ? " on" : ""}" data-ck>${ic("check")}</span>`}
        <div class="tile ${x.level}">${ic(x.icon)}</div>
        <div class="ct"><b>${esc(x.title)}${cardTag(x)}</b><p>${esc(x.desc)}</p></div>
        <div class="cs"><b>${fmtBytes(x.size)}</b><span>${what}</span></div>
        <span class="ibtn exp">${ic("chevd")}</span>
      </div>
      <div class="card-body">${x.tip ? `<div class="tipline">${esc(x.tip)}</div>` : ""}${where || `<div class="note">${x.level === "info" ? "" : "Sin ubicaciones"}</div>`}</div>
    </div>`;
  }
  function cardTag(x) {
    if (x.level === "revisar" && !x.permanent) return `<span class="lvl info" title="Podrás recuperarlo desde la papelera">a la papelera</span>`;
    if (x.level === "revisar" && x.permanent) return `<span class="lvl perm" title="No pasa por la papelera">se borra directamente</span>`;
    return "";
  }
  function updateCleanSum() {
    const c = (S.cands || []).filter((x) => S.cleanSel.has(x.id));
    const tot = c.reduce((s, x) => s + x.size, 0);
    const el = $("#clean-sum");
    if (el) el.innerHTML = c.length ? `<b>${fmtBytes(tot)}</b> marcados en ${c.length} ${c.length === 1 ? "grupo" : "grupos"}` : "Marca algún grupo para limpiarlo";
    const b = $("#clean-go");
    if (b) b.disabled = !c.length;
  }

  // ---------- qué ha crecido ----------
  async function loadGrowth() {
    const el = $("#grow");
    try { S.growth = await api.Growth(); } catch (e) { el.innerHTML = ""; return; }
    const g = S.growth;
    if (!g.prevTime) {
      el.innerHTML = `<div class="empty">${ic("history")}<b>Todavía no hay con qué comparar</b><p>TurboCUT recuerda el tamaño de las carpetas grandes cada vez que analizas. La próxima vez que analices ${esc(S.scan.root)} verás aquí qué ha crecido y qué ha bajado.</p></div>`;
      return;
    }
    const up = g.items.filter((x) => x.delta > 0), down = g.items.filter((x) => x.delta < 0);
    const tot = (S.scan.scanned || 0);
    const row = (x) => {
      const name = x.path.split("\\").filter(Boolean).pop() || x.path;
      return `<div class="grow-row${x.gone ? " gone" : ""}" ${x.id >= 0 ? `data-id="${x.id}"` : ""} title="${esc(x.path)}">
        <span class="gn">${esc(name)}${x.new ? ' <span class="dl new">nueva</span>' : ""}${x.gone ? ' <span class="dl down">ya no existe</span>' : ""}</span>
        <div class="gd"><b>${x.delta > 0 ? "+" : "−"}${fmtBytes(Math.abs(x.delta))}</b><span>${x.gone ? "" : "ahora " + fmtBytes(x.size)}</span></div>
        <span class="gp">${esc(x.path)}</span></div>`;
    };
    el.innerHTML = `<p class="grow-note">Comparado con el análisis del <b>${dtfLong.format(new Date(g.prevTime * 1000))}</b> (${ago(g.prevTime)}).</p>
      <div class="gcols">
        <div class="gcol up"><h3>${ic("tup")}Ha crecido</h3>${up.map(row).join("") || `<div class="note">Nada ha crecido de forma notable.</div>`}</div>
        <div class="gcol down"><h3>${ic("tdown")}Ha bajado</h3>${down.map(row).join("") || `<div class="note">Nada ha bajado de forma notable.</div>`}</div>
      </div>`;
    $$("#grow .grow-row[data-id]").forEach((r) => r.addEventListener("click", () => { setTab("explorar"); go(+r.dataset.id); }));
  }

  // ---------- borrar ----------
  const SYS = /^[a-z]:\\(windows|program files|program files \(x86\)|programdata|recovery|boot|system volume information|\$recycle\.bin|\$windows\.~bt|\$winreagent)(\\|$)/i;
  const USERS = /^[a-z]:\\users(\\[^\\]+)?(\\appdata)?$/i;
  let pendingDel = null;

  function askDelete(items, perm = false) {
    if (!items.length) return;
    hideCtx();
    pendingDel = { kind: "items", items };
    const total = items.reduce((s, x) => s + x.size, 0);
    const n = items.length;
    $("#del-title").textContent = n === 1 ? `¿Borrar «${items[0].name}»?` : `¿Borrar ${count(n)} elementos?`;
    const shown = items.slice().sort((a, b) => b.size - a.size).slice(0, 6);
    $("#del-list").innerHTML = shown.map((x) => `<div class="del-it">${x.dir ? `<div class="ic dir">${ic("folder")}</div>` : `<div class="ic file" style="--cc:${catColor(x.cat || 0)}">${ic(catIcons[x.cat || 0])}</div>`}
      <div style="min-width:0"><div class="t">${esc(x.name)}</div><div class="p">${esc(x.path)}</div></div><span class="z">${fmtBytes(x.size)}</span></div>`).join("") +
      (n > shown.length ? `<div class="del-more">y ${count(n - shown.length)} más…</div>` : "");
    $("#del-total").innerHTML = `<span>Espacio que se libera</span><b>${fmtBytes(total)}</b>`;
    const risky = items.filter((x) => SYS.test(x.path) || USERS.test(x.path) || (x.flags & 32));
    const warn = $("#del-warn");
    warn.hidden = !risky.length;
    if (risky.length) {
      warn.innerHTML = `<b>Cuidado:</b> ${risky.length === 1 ? "«" + esc(risky[0].name) + "» es" : count(risky.length) + " elementos son"} del sistema o de tu perfil de usuario. Borrarlo puede romper Windows o programas instalados.
        <label><input type="checkbox" id="del-risk"> Sé lo que hago, bórralo igualmente</label>`;
      $("#del-risk").addEventListener("change", updateDelBtn);
    }
    $("#del-perm-row").hidden = false;
    $("#del-perm").checked = perm;
    updateDelBtn();
    $("#m-delete").hidden = false;
  }
  function updateDelBtn() {
    const perm = $("#del-perm").checked;
    const risk = $("#del-risk");
    const blocked = !$("#del-warn").hidden && risk && !risk.checked;
    const b = $("#del-go");
    if (pendingDel && pendingDel.kind === "clean") return;
    b.innerHTML = perm ? ic("trash") + "Borrar para siempre" : ic("trash") + "Enviar a la papelera";
    b.disabled = blocked;
    $("#del-note").textContent = perm ? "No se podrá recuperar." : "Podrás recuperarlo desde la papelera. Ojo: hasta que la vacíes el espacio sigue ocupado.";
  }
  $("#del-perm").addEventListener("change", updateDelBtn);

  function askClean() {
    const c = (S.cands || []).filter((x) => S.cleanSel.has(x.id));
    if (!c.length) return;
    pendingDel = { kind: "clean", ids: c.map((x) => x.id) };
    const total = c.reduce((s, x) => s + x.size, 0);
    $("#del-title").textContent = `¿Limpiar ${c.length === 1 ? "«" + c[0].title + "»" : c.length + " grupos"}?`;
    $("#del-list").innerHTML = c.map((x) => `<div class="del-it"><div class="tile ${x.level}" style="width:28px;height:28px;border-radius:8px">${ic(x.icon)}</div>
      <div style="min-width:0"><div class="t">${esc(x.title)}</div><div class="p" style="direction:ltr">${x.permanent ? "se borra directamente" : "va a la papelera"}</div></div><span class="z">${fmtBytes(x.size)}</span></div>`).join("");
    $("#del-total").innerHTML = `<span>Espacio que se libera</span><b>${fmtBytes(total)}</b>`;
    const review = c.filter((x) => x.level === "revisar");
    const warn = $("#del-warn");
    warn.hidden = !review.length;
    if (review.length) warn.innerHTML = `<b>Revisa:</b> ${review.map((x) => esc(x.title)).join(", ")} ${review.length === 1 ? "puede tener" : "pueden tener"} cosas que aún quieras.`;
    $("#del-perm-row").hidden = true;
    $("#del-note").textContent = "Lo que esté en uso por algún programa se salta solo; no pasa nada.";
    const b = $("#del-go");
    b.disabled = false;
    b.innerHTML = ic("broom") + "Limpiar " + fmtBytes(total);
    $("#m-delete").hidden = false;
  }

  $("#del-go").addEventListener("click", async () => {
    const p = pendingDel;
    if (!p) return;
    $("#m-delete").hidden = true;
    pendingDel = null;
    try {
      let res;
      if (p.kind === "clean") {
        res = await api.Clean(p.ids);
        p.ids.forEach((id) => S.cleanSel.delete(id));
      } else {
        res = await api.Delete(p.items.map((x) => x.id), $("#del-perm").checked);
        p.items.forEach((x) => S.sel.delete(x.id));
      }
      await afterDelete(res, p.kind === "clean");
    } catch (e) { toast(String(e), "error"); }
  });

  async function afterDelete(res, clean) {
    S.scan = res.info;
    renderSelbar();
    const failed = res.failed || [];
    if (failed.length) {
      $("#res-title").textContent = res.freed > 0 ? "Hecho, con algunos avisos" : "No se pudo borrar";
      $("#res-body").innerHTML = `<div class="res-big">${fmtBytes(res.freed)}</div><div class="res-sub">liberados · ${count(res.deleted)} borrados · ${count(failed.length)} con problemas</div>` +
        failed.map((f) => `<div class="fail"><div class="fp">${esc(f.path)}</div><div class="fr">${esc(f.reason)}</div>${f.apps && f.apps.length ? `<div class="fa">${f.apps.map((a) => `<span>${esc(a)}</span>`).join("")}</div><div class="fr">Cierra ${f.apps.length === 1 ? "ese programa" : "esos programas"} y vuelve a intentarlo.</div>` : ""}</div>`).join("");
      $("#m-result").hidden = false;
    } else if (clean && res.kept > 0) {
      toast(`Liberados ${fmtBytes(res.freed)}. ${count(res.kept)} elementos estaban en uso y se han dejado.`);
    } else {
      toast(`Liberados ${fmtBytes(res.freed)}`);
    }
    renderDisk();
    renderPills();
    S.cands = null;
    S.growth = null;
    await go(S.cur, { push: false, focus: null, path: S.curPath });
    loadCleanupSummary();
  }

  // ---------- menú contextual ----------
  function showCtx(x, y, it) {
    const m = $("#ctx");
    m.innerHTML = `<div class="ch">${esc(it.name)} · ${fmtBytes(it.size)}</div>
      ${it.dir ? `<button data-a="enter">${ic("enter")}Entrar en la carpeta</button>` : `<button data-a="open">${ic("play")}Abrir</button>`}
      <button data-a="reveal">${ic("reveal")}Mostrar en el Explorador</button>
      <button data-a="copy">${ic("copy")}Copiar ruta</button>
      ${it.dir ? `<button data-a="refresh">${ic("refresh")}Volver a leer esta carpeta</button>` : ""}
      <hr>
      <button data-a="del" class="danger">${ic("trash")}Enviar a la papelera…</button>
      <button data-a="perm" class="danger">${ic("trash")}Borrar para siempre…</button>`;
    m.hidden = false;
    const app = $("#app").getBoundingClientRect();
    const w = m.offsetWidth, h = m.offsetHeight;
    m.style.left = Math.min(x, app.width - w - 8) + "px";
    m.style.top = Math.min(y, app.height - h - 8) + "px";
    $$("button", m).forEach((b) => b.addEventListener("click", async () => {
      hideCtx();
      const a = b.dataset.a;
      try {
        if (a === "enter") go(it.id);
        if (a === "open") await api.Open(it.id);
        if (a === "reveal") await api.Reveal(it.id);
        if (a === "copy") { await api.CopyPath(it.id); toast("Ruta copiada"); }
        if (a === "refresh") await refreshFolder(it.id);
        if (a === "del") askDelete([it]);
        if (a === "perm") askDelete([it], true);
      } catch (e) { toast(String(e), "error"); }
    }));
  }
  function hideCtx() { $("#ctx").hidden = true; }
  document.addEventListener("mousedown", (e) => { if (!e.target.closest("#ctx")) hideCtx(); });
  window.addEventListener("blur", hideCtx);

  async function refreshFolder(id) {
    const res = await api.Refresh(id);
    S.scan = res.info;
    renderDisk();
    S.cands = null;
    const target = id === S.cur ? res.id : S.cur;
    await go(target, { push: false, path: S.curPath });
    loadCleanupSummary();
    toast("Carpeta actualizada");
  }

  // ---------- ajustes ----------
  function openSettings() {
    const s = S.info.settings;
    $$("#s-measure button").forEach((b) => b.classList.toggle("on", b.dataset.v === s.measure));
    $("#s-hidden").checked = s.showHidden;
    $("#s-history").checked = s.history;
    $("#s-deltas").checked = !!s.showDeltas;
    const dm = String(s.deltaMin || 100);
    $$("#s-deltamin button").forEach((b) => b.classList.toggle("on", b.dataset.v === dm));
    if (!$("#s-deltamin button.on")) $("#s-deltamin button").classList.add("on");
    $("#s-admin").checked = s.alwaysAdmin;
    $("#s-ver").textContent = "TurboCUT " + S.info.version + (S.info.admin ? " · administrador" : "");
    $("#m-settings").hidden = false;
  }
  for (const g of ["#s-measure", "#s-deltamin"]) $$(g + " button").forEach((b) => b.addEventListener("click", () => $$(g + " button").forEach((x) => x.classList.toggle("on", x === b))));
  $("#s-save").addEventListener("click", async () => {
    const old = S.info.settings;
    const s = {
      measure: ($("#s-measure button.on") || {}).dataset?.v || "alloc",
      showHidden: $("#s-hidden").checked,
      history: $("#s-history").checked,
      showDeltas: $("#s-deltas").checked,
      deltaMin: +(($("#s-deltamin button.on") || {}).dataset?.v || 100),
      alwaysAdmin: $("#s-admin").checked,
    };
    try {
      S.info.settings = await api.SaveSettings(s);
      $("#m-settings").hidden = true;
      S.growth = null;
      if (S.tab === "crecido") loadGrowth();
      if (S.scan && (old.measure !== s.measure || old.showHidden !== s.showHidden || old.showDeltas !== s.showDeltas || old.deltaMin !== s.deltaMin)) {
        S.scan = await api.Summary();
        renderDisk();
        S.cands = null;
        await go(S.cur, { push: false });
        loadCleanupSummary();
      }
      toast("Ajustes guardados");
    } catch (e) { toast(String(e), "error"); }
  });

  // ---------- modales / toasts ----------
  $$(".modal").forEach((m) => {
    m.addEventListener("mousedown", (e) => { if (e.target === m) m.hidden = true; });
    $$("[data-close]", m).forEach((b) => b.addEventListener("click", () => { m.hidden = true; }));
  });
  function toast(text, kind = "ok") {
    const el = document.createElement("div");
    el.className = "toast " + kind;
    el.innerHTML = ic(kind === "error" ? "alert" : "check") + `<span>${esc(text)}</span>`;
    $("#toasts").appendChild(el);
    setTimeout(() => { el.classList.add("out"); setTimeout(() => el.remove(), 260); }, kind === "error" ? 6000 : 2600);
  }

  // ---------- botones y teclado ----------
  $("#wb-min").addEventListener("click", () => rt.WindowMinimise());
  $("#wb-max").addEventListener("click", () => rt.WindowToggleMaximise());
  $("#wb-close").addEventListener("click", () => rt.Quit());
  $(".titlebar").addEventListener("dblclick", (e) => { if (!e.target.closest("button,.pill")) rt.WindowToggleMaximise(); });
  $("#wb-settings").addEventListener("click", openSettings);
  $("#wb-home").addEventListener("click", () => { if (S.scanning) return; show("home"); refreshHome(); });
  $("#back-results").addEventListener("click", () => show("results"));
  $("#pick-folder").addEventListener("click", async () => {
    try { const p = await api.PickFolder(); if (p) startScan(p); } catch (e) { toast(String(e), "error"); }
  });
  $("#scan-cancel").addEventListener("click", () => api.CancelScan());
  $("#t-map").addEventListener("click", () => {
    S.showMap = !S.showMap;
    $("#t-map").classList.toggle("on", S.showMap);
    $("#p-explorar").classList.toggle("nomap", !S.showMap);
    if (S.showMap) { if (S.tab !== "explorar") setTab("explorar"); requestAnimationFrame(() => resizeTreemap(true)); }
    mainList.render();
  });
  $("#t-select").addEventListener("click", () => {
    S.selMode = !S.selMode;
    $("#t-select").classList.toggle("on", S.selMode);
    $("#list").classList.toggle("selmode", S.selMode);
    $("#big").classList.toggle("selmode", S.selMode);
    if (!S.selMode) S.sel.clear();
    renderSelbar();
    mainList.render(); bigList.render();
  });
  $("#nav-up").addEventListener("click", goUp);
  $("#nav-back").addEventListener("click", goBack);
  $("#refresh").addEventListener("click", () => refreshFolder(S.cur).catch((e) => toast(String(e), "error")));
  $("#sel-clear").addEventListener("click", () => { S.sel.clear(); renderSelbar(); mainList.render(); bigList.render(); });
  $("#sel-delete").addEventListener("click", () => askDelete([...S.sel.values()]));

  document.addEventListener("keydown", (e) => {
    const typing = e.target.closest("input");
    if (e.key === "Escape") {
      hideCtx();
      const open = $$(".modal").find((m) => !m.hidden);
      if (open) { open.hidden = true; return; }
    }
    if (S.view !== "results" || $$(".modal").some((m) => !m.hidden)) return;
    if (e.key === "f" && e.ctrlKey) { e.preventDefault(); $("#q").focus(); $("#q").select(); return; }
    if (typing) return;
    if (e.key === "Backspace") { e.preventDefault(); goUp(); }
    if (e.key === "ArrowLeft" && e.altKey) { e.preventDefault(); goBack(); }
    if (e.key === "F5") { e.preventDefault(); refreshFolder(S.cur).catch((er) => toast(String(er), "error")); }
  });
  window.addEventListener("mouseup", (e) => { if (e.button === 3 && S.view === "results") goBack(); });

  // arrastrar una carpeta a la ventana
  let dragDepth = 0;
  window.addEventListener("dragenter", (e) => { if (S.scanning) return; dragDepth++; $("#veil").classList.add("on"); e.preventDefault(); });
  window.addEventListener("dragleave", () => { dragDepth = Math.max(0, dragDepth - 1); if (!dragDepth) $("#veil").classList.remove("on"); });
  window.addEventListener("dragover", (e) => e.preventDefault());
  window.addEventListener("drop", (e) => { e.preventDefault(); dragDepth = 0; $("#veil").classList.remove("on"); });
  document.addEventListener("contextmenu", (e) => { if (!e.target.closest("input")) e.preventDefault(); });

  // ---------- eventos de Go ----------
  rt.EventsOn("progress", onProgress);
  rt.EventsOn("scanned", onScanned);
  rt.EventsOn("scanfail", onScanFail);
  rt.EventsOn("dropped", (p) => { $("#veil").classList.remove("on"); dragDepth = 0; startScan(p); });
  rt.EventsOn("busy", (t) => { $("#busy").classList.toggle("on", !!t); if (t) $("#busy-t").textContent = t; });

  // ---------- arranque ----------
  (async () => {
    try {
      S.info = await api.GetInfo();
    } catch (e) {
      S.info = { admin: false, drives: [], settings: {}, categories: [], version: "" };
    }
    S.cats = S.info.categories || [];
    renderHome();
    show("home");
    if (S.info.startScan) startScan(S.info.startScan);
  })();
  // refresca "analizado hace…" de vez en cuando
  setInterval(() => { if (S.view === "results") renderDisk(); }, 60000);
})();
