(() => {
  "use strict";

  const $ = (s, el = document) => el.querySelector(s);
  const nf1 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 1 });
  const nf0 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 0 });
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  function fmtBytes(n) {
    if (!isFinite(n) || n < 0) n = 0;
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? nf0.format(n) : nf1.format(n)) + " " + u[i];
  }
  function splitSpeed(bps) {
    const u = ["B/s", "KB/s", "MB/s", "GB/s"];
    let i = 0, n = bps;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    if (bps < 1024) return ["0", "MB/s"];
    return [n >= 100 ? nf0.format(n) : nf1.format(n), u[i]];
  }
  const fmtSpeed = (b) => splitSpeed(b).join(" ");
  function fmtETA(s) {
    if (!isFinite(s) || s <= 0) return "";
    s = Math.round(s);
    if (s < 60) return s + " s";
    const m = Math.floor(s / 60);
    if (m < 60) return m + " min";
    return Math.floor(m / 60) + " h " + (m % 60) + " min";
  }
  function ago(unix) {
    const s = Math.max(0, Date.now() / 1000 - unix);
    if (s < 45) return "ahora mismo";
    if (s < 3600) return "hace " + Math.round(s / 60) + " min";
    if (s < 86400) return "hace " + Math.round(s / 3600) + " h";
    const d = Math.round(s / 86400);
    return d === 1 ? "ayer" : "hace " + d + " días";
  }

  const I = {
    file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>',
    folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/>',
    image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>',
    video: '<path d="m16 13 5.2 3.5a.5.5 0 0 0 .8-.4V7.9a.5.5 0 0 0-.8-.4L16 11"/><rect x="2" y="6" width="14" height="12" rx="2"/>',
    music: '<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
    archive: '<rect x="2" y="3" width="20" height="5" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/>',
    code: '<path d="m16 18 6-6-6-6"/><path d="m8 6-6 6 6 6"/>',
    doc: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M16 13H8"/><path d="M16 17H8"/><path d="M10 9H8"/>',
    app: '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M2 8h20"/><path d="M6 6h.01"/><path d="M9 6h.01"/>',
    disc: '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="2"/>',
    copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
    ext: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    reveal: '<path d="m6 14 1.45-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.55 6a2 2 0 0 1-1.94 1.5H4a2 2 0 0 1-2-2V5c0-1.1.9-2 2-2h3.93a2 2 0 0 1 1.66.9l.82 1.2a2 2 0 0 0 1.66.9H18a2 2 0 0 1 2 2v2"/>',
    edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
    cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>',
    home: '<path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 22V12h6v10"/>',
    send: '<path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/>',
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>',
    alert: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/>',
    msg: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    inbox: '<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>',
    rocket: '<path d="M4.5 16.5c-1.5 1.3-2 5-2 5s3.7-.5 5-2c.7-.8.7-2.1-.1-2.9a2.2 2.2 0 0 0-2.9-.1z"/><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.9 12.9 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.4 22.4 0 0 1-4 2z"/>',
  };
  const svg = (n) => `<svg viewBox="0 0 24 24">${I[n] || I.file}</svg>`;

  const TYPES = [
    ["image", ["jpg", "jpeg", "png", "gif", "webp", "heic", "bmp", "tif", "tiff", "svg", "raw", "cr2", "nef", "arw", "psd", "avif"], "linear-gradient(135deg,#f472b6,#a855f7)", "rgba(217,70,239,.6)"],
    ["video", ["mp4", "mkv", "mov", "avi", "webm", "wmv", "flv", "m4v", "ts", "m2ts"], "linear-gradient(135deg,#f43f5e,#f97316)", "rgba(244,63,94,.6)"],
    ["music", ["mp3", "wav", "flac", "aac", "ogg", "m4a", "opus", "wma"], "linear-gradient(135deg,#22c55e,#14b8a6)", "rgba(20,184,166,.6)"],
    ["archive", ["zip", "rar", "7z", "tar", "gz", "bz2", "xz", "zst"], "linear-gradient(135deg,#f59e0b,#ef4444)", "rgba(245,158,11,.55)"],
    ["code", ["js", "ts", "py", "go", "rs", "c", "cpp", "h", "java", "json", "html", "css", "xml", "yml", "yaml", "sh", "ps1", "sql"], "linear-gradient(135deg,#64748b,#334155)", "rgba(100,116,139,.55)"],
    ["doc", ["pdf", "doc", "docx", "txt", "md", "odt", "rtf", "xls", "xlsx", "csv", "ppt", "pptx", "epub"], "linear-gradient(135deg,#3b82f6,#06b6d4)", "rgba(59,130,246,.6)"],
    ["app", ["exe", "msi", "apk", "dmg", "appx", "msix", "bat", "jar"], "linear-gradient(135deg,#6366f1,#8b5cf6)", "rgba(99,102,241,.6)"],
    ["disc", ["iso", "img", "vhd", "vhdx", "vmdk", "bin"], "linear-gradient(135deg,#0ea5e9,#6366f1)", "rgba(14,165,233,.6)"],
  ];
  function typeOf(name, kind) {
    if (kind === "multi") return { icon: "folder", bg: "linear-gradient(135deg,#f59e0b,#f97316)", sh: "rgba(245,158,11,.55)" };
    const ext = (name.split(".").pop() || "").toLowerCase();
    for (const [icon, exts, bg, sh] of TYPES) if (exts.includes(ext)) return { icon, bg, sh };
    return { icon: "file", bg: null, sh: null };
  }

  // ---------- puente con Go (o datos de ejemplo si se abre en un navegador) ----------
  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const api = hasGo ? window.go.main.App : mockApi();
  const rt = window.runtime || {
    EventsOn: (n, cb) => mockEvents[n] = cb,
    WindowMinimise() {}, WindowToggleMaximise() {}, Quit() {},
  };
  const mockEvents = {};

  // ---------- estado ----------
  let S = null;
  const shareCache = new Map();
  let highlight = null;

  function render(st) {
    S = st;
    renderPills();
    renderShares();
    renderTransfers();
    renderNet();
    renderSpeed();
  }

  // ---------- píldoras de estado ----------
  function tunnelBadge(t) {
    switch (t.status) {
      case "online": return ["ok", "Listo"];
      case "checking": return ["wait", "Verificando"];
      case "starting": return ["wait", "Conectando"];
      case "preparing": return ["wait", "Descargando " + Math.round((t.progress || 0) * 100) + "%"];
      case "error": return ["bad", "Reconectando"];
      default: return ["off", "Apagado"];
    }
  }
  function directBadge(d) {
    switch (d.status) {
      case "verified": return ["ok", "Comprobado"];
      case "ok": return ["ok", "Puerto abierto"];
      case "manual": return ["ok", "Forzado"];
      case "searching": return ["wait", "Buscando router"];
      case "cgnat": return ["bad", "CG-NAT"];
      case "noupnp": return ["bad", "Sin UPnP"];
      case "error": return ["bad", "Error"];
      default: return ["off", "Apagado"];
    }
  }
  function renderPills() {
    const [tc, tt] = tunnelBadge(S.tunnel);
    const [dc, dt] = directBadge(S.direct);
    const html = `
      <span class="pill" title="${esc(S.tunnel.url || S.tunnel.message || "")}"><i class="dot ${tc}"></i><b>Cloudflare</b>${tt}</span>
      <span class="pill" title="${esc(S.direct.message || "")}"><i class="dot ${dc}"></i><b>Directo</b>${dt}</span>
      <span class="pill" title="${esc(S.lan)}"><i class="dot ${S.lan ? "ok" : "off"}"></i><b>Red local</b>${S.lan ? "Activa" : "—"}</span>`;
    const el = $("#pills");
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
  }

  // ---------- envíos ----------
  function linkRow(cls, icon, label, url, na) {
    const u = url ? `<span class="url" title="${esc(url)}">${esc(url.replace(/^https?:\/\//, ""))}</span>` : `<span class="url na">${esc(na)}</span>`;
    return `<div class="link ${cls}">
      <span class="kind">${svg(icon)}${label}</span>${u}
      <button class="ibtn open" data-act="open" data-url="${esc(url || "")}" title="Abrir la página de descarga" ${url ? "" : "disabled"}>${svg("ext")}</button>
      <button class="copy" data-act="copy" data-url="${esc(url || "")}" ${url ? "" : "disabled"}>${svg("copy")}<span>Copiar</span></button>
    </div>`;
  }

  function naTunnel() {
    const t = S.tunnel;
    if (t.status === "off") return "Túnel desactivado en ajustes";
    if (t.status === "error") return "Reconectando con Cloudflare…";
    if (t.status === "preparing") return "Preparando Cloudflare…";
    return "Conectando con Cloudflare…";
  }
  function naDirect() {
    const d = S.direct;
    if (d.status === "searching") return "Buscando el router…";
    if (d.status === "cgnat") return "No disponible: tu operadora usa CG-NAT";
    if (d.status === "noupnp") return "No disponible: el router no tiene UPnP";
    if (d.status === "off") return "Desactivado";
    return "No disponible";
  }

  function shareHTML(sh) {
    const t = typeOf(sh.name, sh.kind);
    const style = t.bg ? `style="--tile:${t.bg};--tile-sh:${t.sh}"` : "";
    const count = sh.kind === "multi" ? `<span class="s"></span><span>${nf0.format(sh.count)} archivo${sh.count === 1 ? "" : "s"}</span>` : "";
    const dls = sh.downloads ? `<span class="s"></span><span class="dl">${svg("download")}${sh.downloads} descarga${sh.downloads === 1 ? "" : "s"}</span>` : "";
    const prev = sh.kind === "multi" && sh.preview && sh.preview.length
      ? `<div class="preview">${sh.preview.map((p) => `<span title="${esc(p)}">${esc(p.split("/").pop())}</span>`).join("")}${sh.count > sh.preview.length ? `<span>+${nf0.format(sh.count - sh.preview.length)}</span>` : ""}</div>`
      : "";
    const L = sh.links;
    const cfURL = S.tunnel.status === "online" ? L.cloudflare : "";
    const cfNA = L.cloudflare && S.tunnel.status === "checking" ? "Verificando que el enlace funciona…" : naTunnel();
    const anyLink = cfURL || L.direct || L.lan;
    return `
      <div class="sh-top">
        <div class="tile" ${style}>${svg(t.icon)}</div>
        <div class="sh-info">
          <div class="sh-name" data-id="${sh.id}" title="${esc(sh.name)}">${esc(sh.name)}</div>
          <div class="sh-meta"><span>${fmtBytes(sh.total)}</span>${count}<span class="s"></span><span>${ago(sh.created)}</span>${dls}</div>
        </div>
        <div class="sh-acts">
          <button class="ibtn" data-act="rename" title="Renombrar">${svg("edit")}</button>
          <button class="ibtn" data-act="reveal" title="Mostrar en el Explorador">${svg("reveal")}</button>
          <button class="ibtn danger" data-act="delete" title="Eliminar envío (el enlace dejará de funcionar)">${svg("trash")}</button>
        </div>
      </div>
      ${prev}
      <div class="links">
        ${linkRow("direct", "zap", "Directo", L.direct, naDirect())}
        ${linkRow("cf", "cloud", "Cloudflare", cfURL, cfNA)}
        ${linkRow("lan", "home", "Red local", L.lan, "Sin red local")}
      </div>
      <div class="sh-foot">
        <span class="grow"></span>
        <button class="btn primary sm" data-act="message" ${anyLink ? "" : "disabled"}>${svg("msg")} Copiar mensaje para enviar</button>
      </div>
      <div class="sh-tx"></div>`;
  }

  function renderShares() {
    const box = $("#shares");
    $("#app").classList.toggle("has-shares", S.shares.length > 0);
    $("#count").textContent = S.shares.length ? nf0.format(S.shares.length) : "";
    $("#list-head").style.display = S.shares.length ? "" : "none";
    const seen = new Set();
    let prev = null;
    for (const sh of S.shares) {
      seen.add(sh.id);
      let el = shareCache.get(sh.id);
      const html = shareHTML(sh);
      if (!el) {
        el = document.createElement("div");
        el.className = "share";
        el.dataset.id = sh.id;
        shareCache.set(sh.id, el);
      }
      if (el._h !== html && !el.querySelector('[contenteditable="true"]')) {
        el.innerHTML = html;
        el._h = html;
      }
      const want = prev ? prev.nextSibling : box.firstChild;
      if (want !== el) box.insertBefore(el, want);
      prev = el;
      if (highlight === sh.id) {
        highlight = null;
        el.classList.add("flash");
        el.scrollIntoView({ behavior: "smooth", block: "nearest" });
        setTimeout(() => el.classList.remove("flash"), 1600);
      }
    }
    for (const [id, el] of shareCache) if (!seen.has(id)) { el.remove(); shareCache.delete(id); }
  }

  // ---------- transferencias ----------
  const viaName = { cloudflare: "Cloudflare", direct: "Directo", lan: "Red local", local: "Este PC" };
  const viaIcon = { cloudflare: "cloud", direct: "zap", lan: "home", local: "home" };
  function txHTML(t) {
    const p = t.total ? Math.min(100, (t.bytes / t.total) * 100) : 0;
    let right;
    if (t.status === "done") right = "Completada";
    else if (t.status === "stalled") right = "En pausa";
    else right = t.speed > 0 ? fmtSpeed(t.speed) : "…";
    const eta = t.status === "active" && t.speed > 0 ? " · " + fmtETA((t.total - t.bytes) / t.speed) : "";
    return `<div class="tx ${t.status}" data-id="${esc(t.id)}">
      <div class="tx-top"><span class="nm" title="${esc(t.label)}">${esc(t.label)}</span><span class="sp">${right}</span></div>
      <div class="tx-bar"><i style="width:${p.toFixed(1)}%"></i></div>
      <div class="tx-sub"><span class="via ${t.via}" title="${esc(t.ip)}">${svg(viaIcon[t.via] || "home")}<span>${viaName[t.via] || t.via}${t.turbo ? " ⚡ turbo" : ""} · ${esc(t.ip)}</span></span><span class="amt">${t.status === "done" ? fmtBytes(t.total) : fmtBytes(t.bytes) + " / " + fmtBytes(t.total) + eta}</span></div>
    </div>`;
  }
  function patchList(box, items, html) {
    const existing = new Map([...box.children].filter((c) => c.dataset.id).map((c) => [c.dataset.id, c]));
    const seen = new Set();
    let prev = null;
    for (const it of items) {
      seen.add(it.id);
      let el = existing.get(it.id);
      const h = html(it);
      if (!el) {
        const tmp = document.createElement("div");
        tmp.innerHTML = h;
        el = tmp.firstElementChild;
        el._h = h;
      } else if (el._h !== h) {
        // actualizar sin recrear para que la barra anime
        const tmp = document.createElement("div");
        tmp.innerHTML = h;
        const n = tmp.firstElementChild;
        el.className = n.className;
        el.querySelector(".tx-top").innerHTML = n.querySelector(".tx-top").innerHTML;
        el.querySelector(".tx-sub").innerHTML = n.querySelector(".tx-sub").innerHTML;
        el.querySelector(".tx-bar i").style.width = n.querySelector(".tx-bar i").style.width;
        el._h = h;
      }
      const want = prev ? prev.nextSibling : box.firstChild;
      if (want !== el) box.insertBefore(el, want);
      prev = el;
    }
    for (const [id, el] of existing) if (!seen.has(id)) el.remove();
    [...box.children].filter((c) => !c.dataset.id).forEach((c) => c.remove());
  }
  function renderTransfers() {
    const box = $("#transfers");
    // activas primero y, como mucho, las 8 más recientes
    const list = [...S.transfers.filter((t) => t.status === "active"), ...S.transfers.filter((t) => t.status !== "active")].slice(0, 8);
    const active = S.transfers.filter((t) => t.status === "active").length;
    $("#tx-count").textContent = active ? active + " activa" + (active === 1 ? "" : "s") : "";
    if (!list.length) {
      if (!box.querySelector(".tx-empty")) box.innerHTML = `<div class="tx-empty">${svg("inbox")}Cuando alguien descargue, lo verás aquí en tiempo real</div>`;
    } else {
      patchList(box, list, txHTML);
    }
    // dentro de cada tarjeta, solo las activas de ese envío
    for (const [id, el] of shareCache) {
      const mine = S.transfers.filter((t) => t.shareId === id && t.status !== "stalled").slice(0, 3);
      patchList(el.querySelector(".sh-tx"), mine, txHTML);
    }
  }

  // ---------- conexión ----------
  function renderNet() {
    const t = S.tunnel, d = S.direct;
    const [tc, tt] = tunnelBadge(t);
    const [dc, dt] = directBadge(d);
    let tDesc = "";
    if (t.status === "online") tDesc = `<code>${esc(t.url.replace("https://", ""))}</code>${t.message ? "<br>" + esc(t.message) : ""}`;
    else if (t.status === "preparing") tDesc = esc(t.message) + `<div class="pbar"><i style="width:${Math.round((t.progress || 0) * 100)}%"></i></div>`;
    else tDesc = esc(t.message || (t.status === "off" ? "Desactivado en ajustes." : "Conectando…"));
    if (t.status === "online" || t.status === "checking") tDesc += `<br><span style="color:var(--dim)">La dirección cambia cada vez que abres TurboSEND.</span>`;

    let dDesc = esc(d.message || "");
    if (d.publicIP && (d.status === "ok" || d.status === "verified" || d.status === "manual")) dDesc = `<code>${esc(d.publicIP)}:${d.extPort}</code><br>` + dDesc;
    if (d.status === "cgnat" && d.routerIP) dDesc += `<br><span style="color:var(--dim)">IP del router: ${esc(d.routerIP)} · IP pública: ${esc(d.publicIP || "?")}</span>`;

    const html = `
      <div class="net-row"><div class="net-ic cf">${svg("cloud")}</div><div>
        <div class="net-t">Cloudflare <i class="dot ${tc}"></i><span style="font-weight:500;color:var(--muted);font-size:12px">${tt}</span></div>
        <div class="net-d">${tDesc}</div>
        <div class="net-a"><button class="mini" data-net="tunnel">Reiniciar túnel</button><button class="mini" data-net="log">Registro</button></div>
      </div></div>
      <div class="net-row"><div class="net-ic direct">${svg("zap")}</div><div>
        <div class="net-t">Directo · UPnP <i class="dot ${dc}"></i><span style="font-weight:500;color:var(--muted);font-size:12px">${dt}</span></div>
        <div class="net-d">${dDesc}</div>
        <div class="net-a"><button class="mini" data-net="direct">Reintentar</button></div>
      </div></div>
      <div class="net-row"><div class="net-ic lan">${svg("home")}</div><div>
        <div class="net-t">Red local</div>
        <div class="net-d">${S.lan ? `<code>${esc(S.lan.replace("http://", ""))}</code><br>Para gente conectada a tu mismo wifi o router.` : "Sin red local detectada."}</div>
      </div></div>
      ${S.serverErr ? `<div class="net-row"><div class="net-ic" style="background:rgba(248,113,113,.15);color:var(--bad)">${svg("alert")}</div><div class="net-d" style="color:var(--bad)">${esc(S.serverErr)}</div></div>` : ""}`;
    const el = $("#net");
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
  }

  // ---------- velocidad + gráfica ----------
  const spark = $("#spark");
  function renderSpeed() {
    const [n, u] = splitSpeed(S.speed);
    $("#speed").textContent = n;
    $("#speed-u").textContent = u;
    const active = S.transfers.filter((t) => t.status === "active");
    $("#live").classList.toggle("on", active.length > 0);
    $("#speed-sub").textContent = active.length
      ? `Enviando a ${active.length} ${active.length === 1 ? "persona" : "personas"}`
      : "Nadie está descargando ahora";
    drawSpark(S.history || []);
  }
  function drawSpark(h) {
    const dpr = devicePixelRatio || 1;
    const w = spark.clientWidth, ht = spark.clientHeight;
    if (spark.width !== w * dpr) { spark.width = w * dpr; spark.height = ht * dpr; }
    const ctx = spark.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, ht);
    const N = 120;
    const data = Array(Math.max(0, N - h.length)).fill(0).concat(h.slice(-N));
    const max = Math.max(1024 * 1024, ...data) * 1.15;
    const x = (i) => (i / (N - 1)) * w;
    const y = (v) => ht - 2 - (v / max) * (ht - 6);
    // líneas guía
    ctx.strokeStyle = "rgba(255,255,255,.05)";
    ctx.lineWidth = 1;
    for (let k = 1; k < 3; k++) { ctx.beginPath(); ctx.moveTo(0, (ht / 3) * k); ctx.lineTo(w, (ht / 3) * k); ctx.stroke(); }
    ctx.beginPath();
    data.forEach((v, i) => {
      if (i === 0) ctx.moveTo(x(i), y(v));
      else {
        const xc = (x(i - 1) + x(i)) / 2;
        ctx.quadraticCurveTo(x(i - 1), y(data[i - 1]), xc, (y(data[i - 1]) + y(v)) / 2);
      }
    });
    ctx.lineTo(w, y(data[N - 1]));
    const line = ctx.createLinearGradient(0, 0, w, 0);
    line.addColorStop(0, "#8b5cf6"); line.addColorStop(.5, "#6366f1"); line.addColorStop(1, "#22d3ee");
    ctx.strokeStyle = line; ctx.lineWidth = 2; ctx.stroke();
    ctx.lineTo(w, ht); ctx.lineTo(0, ht); ctx.closePath();
    const fill = ctx.createLinearGradient(0, 0, 0, ht);
    fill.addColorStop(0, "rgba(99,102,241,.35)"); fill.addColorStop(1, "rgba(99,102,241,0)");
    ctx.fillStyle = fill; ctx.fill();
    const last = data[N - 1];
    if (last > 0) {
      ctx.beginPath(); ctx.arc(w - 3, y(last), 3.5, 0, Math.PI * 2);
      ctx.fillStyle = "#22d3ee"; ctx.shadowColor = "#22d3ee"; ctx.shadowBlur = 10; ctx.fill(); ctx.shadowBlur = 0;
    }
  }

  // ---------- toasts ----------
  function toast(text, kind = "ok") {
    const el = document.createElement("div");
    el.className = "toast " + kind;
    el.innerHTML = svg(kind === "error" ? "alert" : "check") + `<span>${esc(text)}</span>`;
    $("#toasts").appendChild(el);
    setTimeout(() => { el.classList.add("out"); setTimeout(() => el.remove(), 260); }, kind === "error" ? 6000 : 2400);
  }

  // ---------- acciones ----------
  function buildMessage(sh) {
    const L = sh.links;
    const lines = [`Te paso «${sh.name}» (${fmtBytes(sh.total)}${sh.kind === "multi" ? ", " + sh.count + " archivos" : ""}).`, ""];
    if (L.direct) lines.push("⚡ Enlace rápido (directo): " + L.direct);
    if (L.cloudflare && S.tunnel.status === "online") lines.push((L.direct ? "☁️ Si el rápido no va: " : "🔗 Enlace: ") + L.cloudflare);
    if (!L.direct && !(L.cloudflare && S.tunnel.status === "online") && L.lan) lines.push("🏠 Enlace (misma red): " + L.lan);
    lines.push("", "Se descarga desde el navegador, sin instalar nada.");
    return lines.join("\n");
  }

  async function copy(text, btn) {
    try {
      await api.CopyText(text);
      if (btn) {
        btn.classList.add("done");
        const span = btn.querySelector("span");
        const old = span ? span.textContent : "";
        if (span) span.textContent = "Copiado";
        const ic = btn.querySelector("svg");
        if (ic) ic.innerHTML = I.check;
        setTimeout(() => { btn.classList.remove("done"); if (span) span.textContent = old; if (ic) ic.innerHTML = I.copy; }, 1500);
      } else toast("Copiado al portapapeles");
    } catch (e) { toast("No se pudo copiar", "error"); }
  }

  function startRename(el, id) {
    const name = el.querySelector(".sh-name");
    name.contentEditable = "true";
    name.focus();
    const r = document.createRange();
    r.selectNodeContents(name);
    const sel = getSelection(); sel.removeAllRanges(); sel.addRange(r);
    const finish = (save) => {
      name.contentEditable = "false";
      name.removeEventListener("keydown", onKey);
      name.removeEventListener("blur", onBlur);
      const v = name.textContent.trim();
      if (save && v) api.RenameShare(id, v);
      el._h = null;
    };
    const onKey = (e) => {
      if (e.key === "Enter") { e.preventDefault(); finish(true); }
      if (e.key === "Escape") { e.preventDefault(); finish(false); }
    };
    const onBlur = () => finish(true);
    name.addEventListener("keydown", onKey);
    name.addEventListener("blur", onBlur);
  }

  $("#shares").addEventListener("click", (e) => {
    const b = e.target.closest("[data-act]");
    if (!b) return;
    const card = b.closest(".share");
    const id = card && card.dataset.id;
    const sh = S.shares.find((s) => s.id === id);
    switch (b.dataset.act) {
      case "copy": copy(b.dataset.url, b); break;
      case "open": if (b.dataset.url) api.OpenURL(b.dataset.url); break;
      case "message": if (sh) copy(buildMessage(sh), null), toast("Mensaje copiado: pégalo en WhatsApp, Discord…"); break;
      case "reveal": api.RevealShare(id); break;
      case "rename": startRename(card, id); break;
      case "delete":
        if (b.dataset.confirm) { api.DeleteShare(id); toast("Envío eliminado"); }
        else {
          b.dataset.confirm = "1";
          b.style.background = "rgba(248,113,113,.18)"; b.style.color = "var(--bad)";
          b.title = "Pulsa otra vez para confirmar";
          toast("Pulsa otra vez la papelera para confirmar", "error");
          setTimeout(() => { delete b.dataset.confirm; b.style.background = ""; b.style.color = ""; }, 3000);
        }
        break;
    }
  });
  $("#shares").addEventListener("dblclick", (e) => {
    const n = e.target.closest(".sh-name");
    if (n) startRename(n.closest(".share"), n.dataset.id);
  });

  $("#net").addEventListener("click", (e) => {
    const b = e.target.closest("[data-net]");
    if (!b) return;
    if (b.dataset.net === "tunnel") { api.RestartTunnel(); toast("Reiniciando el túnel…"); }
    if (b.dataset.net === "direct") { api.RetryDirect(); toast("Buscando el router otra vez…"); }
    if (b.dataset.net === "log") openLog();
  });

  $("#pick-files").addEventListener("click", () => api.PickFiles().catch((e) => toast(String(e), "error")));
  $("#pick-folder").addEventListener("click", () => api.PickFolder().catch((e) => toast(String(e), "error")));

  // ---------- ventana ----------
  $("#wb-min").addEventListener("click", () => rt.WindowMinimise());
  $("#wb-max").addEventListener("click", () => rt.WindowToggleMaximise());
  $("#wb-close").addEventListener("click", () => rt.Quit());
  $(".titlebar").addEventListener("dblclick", (e) => { if (!e.target.closest("button,.pill")) rt.WindowToggleMaximise(); });

  // ---------- arrastrar y soltar ----------
  let dragDepth = 0;
  const veil = $("#veil");
  const isFiles = (e) => e.dataTransfer && [...e.dataTransfer.types].includes("Files");
  window.addEventListener("dragenter", (e) => { if (!isFiles(e)) return; dragDepth++; veil.classList.add("on"); });
  window.addEventListener("dragleave", (e) => { if (!isFiles(e)) return; if (--dragDepth <= 0) { dragDepth = 0; veil.classList.remove("on"); } });
  window.addEventListener("dragover", (e) => { if (isFiles(e)) e.preventDefault(); });
  window.addEventListener("drop", (e) => { dragDepth = 0; veil.classList.remove("on"); if (isFiles(e)) e.preventDefault(); });

  // ---------- ajustes ----------
  const modal = $("#settings");
  function openSettings() {
    const s = S.settings;
    $("#s-tunnel").checked = s.tunnel;
    $("#s-upnp").checked = s.upnp;
    $("#s-showdirect").checked = s.showDirectOnTunnel;
    $("#s-force").checked = s.forceDirect;
    $("#s-conns").value = s.connections;
    $("#s-conns-v").textContent = s.connections;
    $("#s-port").value = s.port;
    modal.hidden = false;
  }
  $("#s-conns").addEventListener("input", (e) => $("#s-conns-v").textContent = e.target.value);
  $("#wb-settings").addEventListener("click", openSettings);
  $("#s-save").addEventListener("click", async () => {
    const s = {
      tunnel: $("#s-tunnel").checked,
      upnp: $("#s-upnp").checked,
      showDirectOnTunnel: $("#s-showdirect").checked,
      forceDirect: $("#s-force").checked,
      connections: +$("#s-conns").value,
      port: +$("#s-port").value || S.settings.port,
    };
    const portChanged = s.port !== S.settings.port;
    try {
      await api.SaveSettings(s);
      modal.hidden = true;
      toast(portChanged ? "Guardado. El puerto nuevo se usará al reiniciar TurboSEND." : "Ajustes guardados");
    } catch (e) { toast(String(e), "error"); }
  });
  $("#s-log").addEventListener("click", openLog);
  $("#s-data").addEventListener("click", () => api.OpenDataDir());
  async function openLog() {
    const lines = await api.TunnelLog();
    $("#log").textContent = (lines && lines.length ? lines.join("\n") : "Sin registro todavía.");
    $("#logm").hidden = false;
    const pre = $("#log"); pre.scrollTop = pre.scrollHeight;
  }
  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-close]") || e.target.classList.contains("modal")) e.target.closest(".modal").hidden = true;
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") document.querySelectorAll(".modal").forEach((m) => m.hidden = true);
  });

  // ---------- eventos desde Go ----------
  rt.EventsOn("state", render);
  rt.EventsOn("toast", (t) => toast(t.text, t.kind));
  rt.EventsOn("busy", (on) => $("#busy").classList.toggle("on", !!on));
  rt.EventsOn("created", (id) => { highlight = id; toast("Envío creado: ya puedes compartir el enlace"); if (S) renderShares(); });
  api.GetState().then(render);
  setInterval(() => { if (S) renderShares(); }, 30000);

  // ---------- modo demostración (solo si se abre fuera de la app) ----------
  function mockApi() {
    const now = Date.now() / 1000;
    const st = {
      port: 47821, serverErr: "", lan: "http://192.168.1.34:47821",
      tunnel: { status: "online", url: "https://silver-river-paints-quiet.trycloudflare.com", message: "", progress: 0 },
      direct: { status: "verified", publicIP: "81.43.120.77", extPort: 47821, localIP: "192.168.1.34", message: "Comprobado: tu PC responde desde Internet." },
      settings: { port: 47821, tunnel: true, upnp: true, showDirectOnTunnel: true, forceDirect: false, connections: 6 },
      shares: [
        { id: "aB3xK9", name: "Vacaciones Asturias 2026", kind: "multi", count: 214, total: 7.3e9, created: now - 120, downloads: 1, preview: ["Vacaciones Asturias 2026/IMG_0001.HEIC", "Vacaciones Asturias 2026/IMG_0002.HEIC", "Vacaciones Asturias 2026/video_playa.mp4", "Vacaciones Asturias 2026/IMG_0004.HEIC"] },
        { id: "Qw7pL2", name: "Proyecto final - render 4K.mp4", kind: "file", count: 1, total: 5.2e9, created: now - 3600 * 5, downloads: 3, preview: ["Proyecto final - render 4K.mp4"] },
      ],
      transfers: [
        { id: "t1", shareId: "Qw7pL2", label: "Proyecto final - render 4K.mp4", via: "direct", ip: "88.12.4.201", total: 5.2e9, bytes: 3.1e9, speed: 96e6, status: "active", turbo: false, started: Date.now() },
        { id: "t2", shareId: "aB3xK9", label: "Vacaciones Asturias 2026", via: "cloudflare", ip: "2a0c:5a80::1", total: 7.3e9, bytes: 1.2e9, speed: 31e6, status: "active", turbo: true, started: Date.now() - 1000 },
      ],
      speed: 127e6, history: Array.from({ length: 120 }, (_, i) => i < 40 ? 0 : 90e6 + Math.sin(i / 5) * 25e6 + Math.random() * 15e6),
    };
    for (const s of st.shares) s.links = { cloudflare: st.tunnel.url + "/s/" + s.id, direct: "http://81.43.120.77:47821/s/" + s.id, lan: st.lan + "/s/" + s.id };
    setInterval(() => {
      for (const t of st.transfers) { t.bytes = Math.min(t.total, t.bytes + t.speed / 2); t.speed *= 0.97 + Math.random() * 0.06; }
      st.speed = st.transfers.reduce((a, t) => a + t.speed, 0);
      st.history = st.history.slice(1).concat(st.speed);
      mockEvents.state && mockEvents.state(JSON.parse(JSON.stringify(st)));
    }, 500);
    const ok = () => Promise.resolve();
    return {
      GetState: () => Promise.resolve(JSON.parse(JSON.stringify(st))),
      CopyText: (t) => navigator.clipboard ? navigator.clipboard.writeText(t) : ok(),
      OpenURL: (u) => window.open(u, "_blank"), PickFiles: ok, PickFolder: ok,
      DeleteShare: ok, RenameShare: ok, RevealShare: ok, RestartTunnel: ok, RetryDirect: ok,
      SaveSettings: ok, OpenDataDir: ok, TunnelLog: () => Promise.resolve(["2026-10-14T10:00:00Z INF Requesting new quick Tunnel on trycloudflare.com..."]),
    };
  }
})();
