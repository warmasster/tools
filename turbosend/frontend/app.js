(() => {
  "use strict";
  const $ = (s, el = document) => el.querySelector(s);
  const api = window.go.main.App;
  const rt = window.runtime;

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
    if (bps < 1024) return ["0", "MB/s"];
    const u = ["B/s", "KB/s", "MB/s", "GB/s"];
    let i = 0, n = bps;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return [n >= 100 ? nf0.format(n) : nf1.format(n), u[i]];
  }
  const fmtSpeed = (b) => splitSpeed(b).join(" ");
  function fmtETA(s) {
    if (!isFinite(s) || s <= 0) return "";
    s = Math.round(s);
    if (s < 60) return s + " s";
    const m = Math.floor(s / 60);
    return m < 60 ? m + " min" : Math.floor(m / 60) + " h " + (m % 60) + " min";
  }
  const timeFmt = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });
  const dateFmt = new Intl.DateTimeFormat("es-ES", { day: "numeric", month: "short" });
  const dayFmt = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long" });
  function shortWhen(ms) {
    const d = new Date(ms), now = new Date();
    if (d.toDateString() === now.toDateString()) return timeFmt.format(d);
    const y = new Date(now); y.setDate(now.getDate() - 1);
    if (d.toDateString() === y.toDateString()) return "Ayer";
    return dateFmt.format(d);
  }
  function dayLabel(ms) {
    const d = new Date(ms), now = new Date();
    const diff = Math.round((new Date(now.getFullYear(), now.getMonth(), now.getDate()) - new Date(d.getFullYear(), d.getMonth(), d.getDate())) / 86400000);
    if (diff === 0) return "Hoy";
    if (diff === 1) return "Ayer";
    const s = dayFmt.format(d);
    return s.charAt(0).toUpperCase() + s.slice(1);
  }

  const I = {
    file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>',
    folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/>',
    image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>',
    video: '<path d="m16 13 5.2 3.5a.5.5 0 0 0 .8-.4V7.9a.5.5 0 0 0-.8-.4L16 11"/><rect x="2" y="6" width="14" height="12" rx="2"/>',
    music: '<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
    archive: '<rect x="2" y="3" width="20" height="5" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/>',
    doc: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M16 13H8"/><path d="M16 17H8"/>',
    app: '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M2 8h20"/>',
    copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>',
    reveal: '<path d="m6 14 1.45-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.55 6a2 2 0 0 1-1.94 1.5H4a2 2 0 0 1-2-2V5c0-1.1.9-2 2-2h3.93a2 2 0 0 1 1.66.9l.82 1.2a2 2 0 0 0 1.66.9H18a2 2 0 0 1 2 2v2"/>',
    open: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>',
    clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
    alert: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/>',
    zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
    cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>',
    home: '<path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 22V12h6v10"/>',
    msg: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    link: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
    wifi: '<path d="M5 13a10 10 0 0 1 14 0"/><path d="M8.5 16.5a5 5 0 0 1 7 0"/><path d="M2 8.82a15 15 0 0 1 20 0"/><path d="M12 20h.01"/>',
  };
  const svg = (n) => `<svg viewBox="0 0 24 24">${I[n] || I.file}</svg>`;
  const COLORS = ["#f472b6", "#fb923c", "#facc15", "#4ade80", "#2dd4bf", "#38bdf8", "#818cf8", "#c084fc", "#f87171", "#a3e635"];
  const ROOMGRADS = ["linear-gradient(135deg,#8b5cf6,#6366f1)", "linear-gradient(135deg,#f472b6,#a855f7)", "linear-gradient(135deg,#f97316,#ef4444)", "linear-gradient(135deg,#10b981,#06b6d4)", "linear-gradient(135deg,#3b82f6,#06b6d4)", "linear-gradient(135deg,#eab308,#f97316)", "linear-gradient(135deg,#14b8a6,#22c55e)", "linear-gradient(135deg,#6366f1,#ec4899)"];
  const hash = (s) => { let h = 0; for (const c of s) h = (h * 31 + c.charCodeAt(0)) | 0; return Math.abs(h); };
  const initial = (n) => (n || "?").trim().charAt(0).toUpperCase();
  const TYPES = [
    ["image", "jpg jpeg png gif webp heic bmp tif tiff svg raw cr2 nef arw psd avif", "linear-gradient(135deg,#f472b6,#a855f7)"],
    ["video", "mp4 mkv mov avi webm wmv flv m4v ts m2ts", "linear-gradient(135deg,#f43f5e,#f97316)"],
    ["music", "mp3 wav flac aac ogg m4a opus wma", "linear-gradient(135deg,#22c55e,#14b8a6)"],
    ["archive", "zip rar 7z tar gz bz2 xz zst iso", "linear-gradient(135deg,#f59e0b,#ef4444)"],
    ["doc", "pdf doc docx txt md odt rtf xls xlsx csv ppt pptx epub", "linear-gradient(135deg,#3b82f6,#06b6d4)"],
    ["app", "exe msi apk dmg appx msix jar", "linear-gradient(135deg,#6366f1,#8b5cf6)"],
  ];
  function typeOf(name, kind) {
    if (kind === "multi") return { icon: "folder", bg: "linear-gradient(135deg,#f59e0b,#f97316)" };
    const ext = (name.split(".").pop() || "").toLowerCase();
    for (const [icon, exts, bg] of TYPES) if (exts.split(" ").includes(ext)) return { icon, bg };
    return { icon: "file", bg: "linear-gradient(135deg,#64748b,#475569)" };
  }

  // ---------- estado ----------
  let S = null;        // estado global (cada 500 ms)
  let R = null;        // chat abierto
  let active = "";
  let typingWho = [];
  const els = new Map();

  function toast(text, kind = "ok", onClick) {
    const el = document.createElement("div");
    el.className = "toast " + kind + (onClick ? " click" : "");
    el.innerHTML = svg(kind === "error" ? "alert" : kind === "msg" ? "msg" : "check") + `<span>${esc(text)}</span>`;
    if (onClick) el.addEventListener("click", () => { onClick(); el.remove(); });
    $("#toasts").appendChild(el);
    setTimeout(() => { el.classList.add("out"); setTimeout(() => el.remove(), 260); }, kind === "error" ? 6000 : kind === "msg" ? 5000 : 2400);
  }
  const call = (p) => Promise.resolve(p).catch((e) => toast(String(e), "error"));

  // ---------- píldoras ----------
  function tunnelBadge(t) {
    const dom = t.mode === "domain";
    switch (t.status) {
      case "online": return ["ok", dom ? "Tu dominio" : "Listo"];
      case "checking": return ["wait", "Verificando"];
      case "starting": return ["wait", dom ? "Conectando dominio" : "Conectando"];
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
    const [tc, tt] = tunnelBadge(S.tunnel), [dc, dt] = directBadge(S.direct);
    const html = `<span class="pill"><i class="dot ${tc}"></i><b>Cloudflare</b>${tt}</span><span class="pill"><i class="dot ${dc}"></i><b>Directo</b>${dt}</span><span class="pill"><i class="dot ${S.lan ? "ok" : ""}"></i><b>Red local</b>${S.lan ? "Activa" : "—"}</span>`;
    const el = $("#pills");
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
  }

  // ---------- lista de chats ----------
  function renderRooms() {
    const q = $("#q").value.trim().toLowerCase();
    const list = S.rooms.filter((r) => !q || r.name.toLowerCase().includes(q) || (r.preview || "").toLowerCase().includes(q));
    let html = list.map((r) => {
      const av = `<div class="av big rav" style="--avg:${ROOMGRADS[hash(r.id) % ROOMGRADS.length]}">${esc(initial(r.name))}${r.online ? '<i class="st"></i>' : ""}</div>`;
      const badges = (r.pending ? `<span class="badge pend" title="Archivos esperando a que los aceptes">📥 ${r.pending}</span>` : "") + (r.unread && r.id !== active ? `<span class="badge">${r.unread}</span>` : "");
      return `<div class="ritem${r.id === active ? " on" : ""}${r.unread && r.id !== active ? " unread" : ""}" data-room="${esc(r.id)}">${av}<div class="ri-main"><div class="ri-top"><span class="ri-name">${esc(r.name)}</span><span class="ri-time">${shortWhen(r.updated)}</span></div><div class="ri-bot"><span class="ri-prev">${esc(r.preview || (r.members ? r.members + " persona(s)" : "Sin mensajes todavía"))}</span>${badges}</div></div></div>`;
    }).join("");
    if (!S.rooms.length) html = `<div class="rooms-empty">Aún no tienes chats.<br>Crea uno con el botón <b>+</b> o soltando archivos.</div>`;
    else if (!list.length) html = `<div class="rooms-empty">Nada coincide con «${esc(q)}»</div>`;
    const el = $("#rooms");
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
    const unread = S.rooms.reduce((a, r) => a + (r.id === active && document.hasFocus() ? 0 : r.unread), 0);
    const title = (unread ? `(${unread}) ` : "") + "TurboSEND";
    if (document.title !== title) { document.title = title; api.SetTitle(title); }
  }
  $("#rooms").addEventListener("click", (e) => { const it = e.target.closest("[data-room]"); if (it) openRoom(it.dataset.room); });
  $("#q").addEventListener("input", () => S && renderRooms());

  // ---------- abrir chat ----------
  async function openRoom(id) {
    active = id;
    api.SetActiveRoom(id);
    els.clear();
    $("#msgs").innerHTML = "";
    typingWho = [];
    await loadRoom(true);
    if (S) renderRooms();
    $("#text").focus();
  }
  function closeRoom() {
    active = "";
    R = null;
    api.SetActiveRoom("");
    $("#room").hidden = true;
    $("#welcome").hidden = false;
    if (S) renderRooms();
  }
  let loadTimer = null;
  function scheduleLoad() { clearTimeout(loadTimer); loadTimer = setTimeout(() => loadRoom(false), 120); }
  async function loadRoom(bottom) {
    if (!active) return;
    try { R = await api.GetRoom(active); } catch { closeRoom(); return; }
    $("#welcome").hidden = true;
    $("#room").hidden = false;
    renderRoom(bottom);
    if (R.unread && document.hasFocus()) api.MarkRead(active);
  }

  const member = (pid) => pid === "host" ? { name: (S && S.settings.hostName) || "Tú", color: -1 } : (R.members.find((m) => m.pid === pid) || { name: "Alguien", color: 0 });
  const colorOf = (m) => (m.color < 0 ? "#a78bfa" : COLORS[m.color % COLORS.length]);

  function renderRoomHead() {
    const on = R.members.filter((m) => m.online);
    $("#r-av").className = "av big rav";
    $("#r-av").style.setProperty("--avg", ROOMGRADS[hash(R.id) % ROOMGRADS.length]);
    $("#r-av").innerHTML = esc(initial(R.name)) + (on.length ? '<i class="st"></i>' : "");
    const nm = $("#r-name");
    if (nm.contentEditable !== "true") nm.textContent = R.name;
    let sub;
    if (typingWho.length) sub = typingWho.join(", ") + " escribiendo…";
    else if (!R.members.length) sub = "Nadie ha entrado todavía · pulsa Invitar";
    else if (on.length) sub = on.map((m) => m.name).join(", ") + (on.length === 1 ? " está conectado" : " están conectados");
    else sub = R.members.map((m) => m.name).join(", ") + " · desconectado" + (R.members.length > 1 ? "s" : "");
    $("#r-sub").textContent = sub;
    $("#r-members").innerHTML = R.members.slice(0, 6).map((m) => `<div class="av" style="--avc:${colorOf(m)}" title="${esc(m.name)}${m.online ? " · conectado" : ""}">${esc(initial(m.name))}${m.online ? '<i class="st"></i>' : ""}</div>`).join("") + (R.members.length > 6 ? `<span class="more">+${R.members.length - 6}</span>` : "");
  }

  function linkify(s) { return esc(s).replace(/\bhttps?:\/\/[^\s<]+[^\s<.,;:!?)\]'"]/g, (u) => `<a href="#" data-url="${u}">${u}</a>`); }
  const emojiOnly = (s) => /^(?:\p{Extended_Pictographic}|\p{Emoji_Component}|‍|️|\s){1,12}$/u.test(s) && !/[0-9#*]/.test(s);

  function fileHead(name, kind, meta) {
    const t = typeOf(name, kind);
    return `<div class="fc"><div class="tile" style="--tile:${t.bg}">${svg(t.icon)}</div><div class="fc-i"><div class="fc-n" title="${esc(name)}">${esc(name)}</div><div class="fc-m">${meta}</div></div></div>`;
  }
  const metaOf = (v) => fmtBytes(v.total) + (v.kind === "multi" ? ` · ${nf0.format(v.count)} archivo${v.count === 1 ? "" : "s"}` : "");

  function outCard(id) {
    const v = R.out[id];
    if (!v || v.gone) return fileHead("Envío retirado", "file", "Ya no se puede descargar");
    let h = fileHead(v.name, v.kind, metaOf(v) + (v.downloads ? ` · ${v.downloads} descarga${v.downloads === 1 ? "" : "s"}` : ""));
    if (v.preview && v.preview.length) h += `<div class="preview">${v.preview.map((p) => `<span>${esc(p)}</span>`).join("")}${v.count > v.preview.length ? `<span>+${nf0.format(v.count - v.preview.length)}</span>` : ""}</div>`;
    h += `<div class="livetx" data-live="${esc(id)}"></div>`;
    h += `<div class="fc-acts"><button class="fbtn" data-act="reveal-out" data-id="${esc(id)}">${svg("reveal")} Mostrar</button><button class="fbtn ghost danger" data-act="retire" data-id="${esc(id)}" title="Retirar (dejará de poder descargarse)">${svg("trash")}</button></div>`;
    return h;
  }

  function inCard(id) {
    const v = R.in[id];
    if (!v) return "";
    const who = member(v.from);
    const mem = R.members.find((m) => m.pid === v.from);
    let h = fileHead(v.name, v.kind, metaOf(v));
    switch (v.status) {
      case "offered":
        h += `<div class="fc-st">${svg("download")}<span>${esc(who.name)} quiere enviarte esto</span></div>`;
        h += `<div class="fc-acts"><button class="fbtn pri" data-act="accept" data-id="${esc(id)}">${svg("download")} Descargar</button><button class="fbtn ghost" data-act="reject" data-id="${esc(id)}">Rechazar</button></div>`;
        break;
      case "accepted":
      case "transferring":
        h += `<div class="pbar"><i data-bar></i></div><div class="pinfo"><span data-l>${mem && mem.online ? "Empezando…" : ""}</span><span data-r></span></div>`;
        if (!(mem && mem.online)) h += `<div class="fc-st warn">${svg("clock")}<span>${esc(who.name)} no tiene el chat abierto. Se enviará en cuanto vuelva a entrar.</span></div>`;
        h += `<div class="fc-acts"><button class="fbtn ghost" data-act="cancel-in" data-id="${esc(id)}">${svg("x")} Cancelar</button></div>`;
        break;
      case "done":
        h += `<div class="fc-st ok">${svg("check")}<span>Recibido y verificado</span></div>`;
        h += `<div class="fc-acts"><button class="fbtn pri" data-act="open-in" data-id="${esc(id)}">${svg("open")} Abrir</button><button class="fbtn" data-act="reveal-in" data-id="${esc(id)}">${svg("reveal")} Mostrar en carpeta</button></div>`;
        break;
      case "rejected": h += `<div class="fc-st">${svg("x")}<span>Lo rechazaste</span></div>`; break;
      case "cancelled": h += `<div class="fc-st">${svg("x")}<span>Cancelado</span></div>`; break;
      case "failed":
        h += `<div class="fc-st bad">${svg("alert")}<span>${esc(v.error || "Ha fallado")}</span></div><div class="fc-acts"><button class="fbtn pri" data-act="accept" data-id="${esc(id)}">Reintentar</button></div>`;
        break;
    }
    return h;
  }

  function msgHTML(m, prev, next) {
    if (m.kind === "system") return `<div class="sys">${esc(m.text)}</div>`;
    const mine = m.from === "host";
    const who = member(m.from);
    const sameP = prev && prev.from === m.from && prev.kind !== "system" && m.time - prev.time < 5 * 60e3;
    const sameN = next && next.from === m.from && next.kind !== "system" && next.time - m.time < 5 * 60e3;
    const cls = ["row", mine ? "me" : "", sameP ? "" : "first", sameN ? "" : "last"].join(" ");
    const av = `<div class="av" style="--avc:${colorOf(who)}">${esc(initial(who.name))}</div>`;
    const nm = !mine && !sameP ? `<span class="nm" style="--nmc:${colorOf(who)}">${esc(who.name)}</span>` : "";
    const tm = `<span class="tm">${timeFmt.format(new Date(m.time))}</span>`;
    let body = "";
    if (m.kind === "text") {
      const big = emojiOnly(m.text);
      body = `<div class="bub${big ? " emoji" : ""}">${big ? "" : nm}<span class="tx">${big ? esc(m.text) : linkify(m.text)}</span>${tm}</div>`;
    } else if (m.kind === "out") body = `<div class="bub file" data-out="${esc(m.bundle)}">${outCard(m.bundle)}${tm}</div>`;
    else if (m.kind === "in") body = `<div class="bub file" data-in="${esc(m.bundle)}">${nm}${inCard(m.bundle)}${tm}</div>`;
    return `<div class="${cls}">${av}${body}</div>`;
  }

  function renderRoom(bottom) {
    if (!R) return;
    renderRoomHead();
    const box = $("#msgs");
    const stick = bottom || box.scrollHeight - box.scrollTop - box.clientHeight < 140;
    const list = R.messages;
    if (!list.length) {
      els.clear();
      box.innerHTML = `<div class="empty-room"><div class="ic">${svg("link")}</div><b>Chat listo</b>Invita a alguien con el enlace, o arrastra aquí archivos para dejárselos preparados.<br><button class="btn primary sm" id="er-invite">${svg("link")} Invitar</button></div>`;
      $("#er-invite").onclick = (e) => { e.stopPropagation(); toggleInvite(); };
      return;
    }
    const empty = box.querySelector(".empty-room");
    if (empty) empty.remove();
    const seen = new Set();
    let prevEl = null, lastDay = "";
    for (let i = 0; i < list.length; i++) {
      const m = list[i];
      const day = dayLabel(m.time);
      if (day !== lastDay) {
        lastDay = day;
        const key = "day:" + day;
        seen.add(key);
        let d = els.get(key);
        if (!d) { d = document.createElement("div"); d.className = "day"; d.textContent = day; els.set(key, d); }
        const want = prevEl ? prevEl.nextSibling : box.firstChild;
        if (want !== d) box.insertBefore(d, want);
        prevEl = d;
      }
      const html = msgHTML(m, list[i - 1], list[i + 1]);
      seen.add(m.id);
      let el = els.get(m.id);
      if (!el) { el = document.createElement("div"); el.style.display = "contents"; els.set(m.id, el); }
      if (el._h !== html) { el.innerHTML = html; el._h = html; }
      const want = prevEl ? prevEl.nextSibling : box.firstChild;
      if (want !== el) box.insertBefore(el, want);
      prevEl = el;
    }
    for (const [k, el] of els) if (!seen.has(k)) { el.remove(); els.delete(k); }
    applyLive();
    if (stick) box.scrollTop = box.scrollHeight;
  }

  // progreso en vivo dentro de las tarjetas
  function applyLive() {
    if (!R || !S) return;
    const box = $("#msgs");
    for (const card of box.querySelectorAll("[data-in]")) {
      const v = R.in[card.dataset.in];
      if (!v || !card.querySelector("[data-bar]")) continue;
      const t = S.transfers.find((x) => x.shareId === v.id && x.dir === "in");
      const done = t ? Math.max(t.bytes, v.received) : v.received;
      const pct = v.total ? Math.min(100, (done / v.total) * 100) : 100;
      card.querySelector("[data-bar]").style.width = pct.toFixed(1) + "%";
      const l = card.querySelector("[data-l]"), r = card.querySelector("[data-r]");
      if (t && t.status === "active" && t.speed > 0) l.textContent = `${fmtSpeed(t.speed)} · quedan ${fmtETA((v.total - done) / t.speed)}`;
      else if (t && t.status === "active") l.textContent = "Recibiendo…";
      r.textContent = `${Math.floor(pct)}% · ${fmtBytes(done)}`;
    }
    for (const box2 of box.querySelectorAll("[data-live]")) {
      const id = box2.dataset.live;
      const list = S.transfers.filter((t) => t.shareId === id && t.dir !== "in" && t.status !== "stalled").slice(0, 4);
      const html = list.map((t) => {
        const p = t.total ? Math.min(100, (t.bytes / t.total) * 100) : 0;
        const right = t.status === "done" ? "✓ descargado" : `${Math.floor(p)}% · ${t.speed > 0 ? fmtSpeed(t.speed) : "…"}`;
        return `<div class="lt"><div class="lt-top"><span>${esc(t.who || t.ip)}${t.turbo ? " ⚡" : ""}</span><span>${right}</span></div><div class="pbar"><i style="width:${p.toFixed(1)}%"></i></div></div>`;
      }).join("");
      if (box2._h !== html) { box2.innerHTML = html; box2._h = html; }
    }
  }

  // clics dentro del chat
  $("#msgs").addEventListener("click", (e) => {
    const a = e.target.closest("a[data-url]");
    if (a) { e.preventDefault(); api.OpenURL(a.dataset.url); return; }
    const b = e.target.closest("[data-act]");
    if (!b) return;
    const id = b.dataset.id;
    switch (b.dataset.act) {
      case "accept": call(api.AcceptBundle(active, id)); break;
      case "reject": api.RejectBundle(active, id); break;
      case "cancel-in":
        if (b.dataset.confirm) api.CancelBundle(active, id);
        else { b.dataset.confirm = "1"; b.innerHTML = "¿Seguro? Pulsa otra vez"; setTimeout(() => { delete b.dataset.confirm; b.innerHTML = svg("x") + " Cancelar"; }, 3000); }
        break;
      case "open-in": api.OpenBundle(active, id); break;
      case "reveal-in": api.RevealBundle(active, id); break;
      case "reveal-out": api.RevealOut(id); break;
      case "retire":
        if (b.dataset.confirm) { api.DeleteBundle(active, id); toast("Envío retirado"); }
        else { b.dataset.confirm = "1"; b.style.color = "var(--bad)"; toast("Pulsa otra vez la papelera para retirarlo", "error"); setTimeout(() => { delete b.dataset.confirm; b.style.color = ""; }, 3000); }
        break;
    }
  });

  // ---------- escribir ----------
  const text = $("#text");
  function autosize() { text.style.height = "auto"; text.style.height = Math.min(160, text.scrollHeight) + "px"; text.style.overflowY = text.scrollHeight > 160 ? "auto" : "hidden"; }
  let lastTyping = 0;
  text.addEventListener("input", () => {
    autosize();
    if (active && Date.now() - lastTyping > 2500 && text.value.trim()) { lastTyping = Date.now(); api.Typing(active); }
  });
  text.addEventListener("keydown", (e) => { if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); } });
  $("#send").addEventListener("click", send);
  function send() {
    const t = text.value.trim();
    if (!t || !active) return;
    text.value = "";
    autosize();
    lastTyping = 0;
    call(api.SendText(active, t));
  }
  const attMenu = $("#att-menu");
  $("#att").addEventListener("click", (e) => { e.stopPropagation(); attMenu.hidden = !attMenu.hidden; });
  attMenu.addEventListener("click", (e) => {
    const b = e.target.closest("[data-pick]");
    if (!b) return;
    attMenu.hidden = true;
    call(b.dataset.pick === "folder" ? api.PickFolder(active) : api.PickFiles(active));
  });

  // ---------- cabecera del chat ----------
  $("#r-name").addEventListener("dblclick", startRename);
  function startRename() {
    const n = $("#r-name");
    n.contentEditable = "true";
    n.focus();
    const r = document.createRange(); r.selectNodeContents(n);
    const s = getSelection(); s.removeAllRanges(); s.addRange(r);
    const done = (save) => {
      n.contentEditable = "false";
      n.onkeydown = n.onblur = null;
      const v = n.textContent.trim();
      if (save && v && v !== R.name) api.RenameRoom(active, v);
      else n.textContent = R.name;
    };
    n.onkeydown = (e) => { if (e.key === "Enter") { e.preventDefault(); done(true); } if (e.key === "Escape") { e.preventDefault(); done(false); } };
    n.onblur = () => done(true);
  }

  function place(pop, anchor) {
    const r = anchor.getBoundingClientRect();
    pop.style.top = r.bottom + 8 + "px";
    pop.style.left = Math.max(12, Math.min(window.innerWidth - pop.offsetWidth - 12, r.right - pop.offsetWidth)) + "px";
  }
  function closePops() { $("#invite").hidden = true; $("#rmenu").hidden = true; attMenu.hidden = true; }
  document.addEventListener("click", (e) => { if (!e.target.closest(".pop, #r-invite, #r-menu, #att")) closePops(); });

  function linkRow(cls, icon, label, url, na) {
    return `<div class="link ${cls}"><span class="kind">${svg(icon)}${label}</span>${url ? `<span class="url" title="${esc(url)}">${esc(url.replace(/^https?:\/\//, ""))}</span>` : `<span class="url na">${esc(na)}</span>`}<button class="copy" data-copy="${esc(url || "")}" ${url ? "" : "disabled"}>${svg("copy")}<span>Copiar</span></button></div>`;
  }
  function inviteHTML() {
    const L = (R && R.links) || {};
    const t = S.tunnel, d = S.direct;
    const naCf = t.status === "off" ? "Túnel desactivado" : t.status === "error" ? "Reconectando…" : "Conectando con Cloudflare…";
    const naDir = d.status === "cgnat" ? "No disponible (CG-NAT)" : d.status === "noupnp" ? "No disponible (sin UPnP)" : d.status === "searching" ? "Buscando el router…" : "No disponible";
    return linkRow("direct", "zap", "Directo", L.direct, naDir) + linkRow("cf", "cloud", t.mode === "domain" ? "Tu dominio" : "Cloudflare", L.cloudflare, naCf) + linkRow("lan", "home", "Red local", L.lan, "Sin red local");
  }
  function toggleInvite() {
    const pop = $("#invite");
    $("#rmenu").hidden = true;
    if (!pop.hidden) { pop.hidden = true; return; }
    loadRoom(false).then(() => {
      $("#inv-links").innerHTML = inviteHTML();
      $("#inv-note").textContent = S.tunnel.mode === "domain" ? "Los enlaces de tu dominio no cambian." : "El enlace de Cloudflare cambia cada vez que abres TurboSEND. El directo ⚡ se mantiene mientras no cambie tu IP.";
      pop.hidden = false;
      place(pop, $("#r-invite"));
    });
  }
  $("#r-invite").addEventListener("click", (e) => { e.stopPropagation(); toggleInvite(); });
  $("#invite").addEventListener("click", (e) => {
    const b = e.target.closest("[data-copy]");
    if (b && b.dataset.copy) copyBtn(b, b.dataset.copy);
  });
  $("#inv-msg").addEventListener("click", () => {
    const L = R.links;
    const cf = S.tunnel.status === "online" ? L.cloudflare : "";
    const lines = [`¡Hola! Te abro un chat para pasarnos archivos (de cualquier tamaño) y escribirnos:`, ""];
    if (L.direct) lines.push("⚡ " + L.direct);
    if (cf) lines.push((L.direct ? "☁️ Si el rápido no va: " : "🔗 ") + cf);
    if (!L.direct && !cf && L.lan) lines.push("🏠 " + L.lan);
    lines.push("", "Se abre en el navegador, sin instalar nada.");
    call(api.CopyText(lines.join("\n")).then(() => toast("Invitación copiada: pégala en WhatsApp, Discord…")));
  });
  function copyBtn(b, text) {
    api.CopyText(text).then(() => {
      b.classList.add("done");
      b.innerHTML = svg("check") + "<span>Copiado</span>";
      setTimeout(() => { b.classList.remove("done"); b.innerHTML = svg("copy") + "<span>Copiar</span>"; }, 1500);
    });
  }

  $("#r-menu").addEventListener("click", (e) => {
    e.stopPropagation();
    const m = $("#rmenu");
    $("#invite").hidden = true;
    if (!m.hidden) { m.hidden = true; return; }
    for (const b of m.querySelectorAll("[data-auto]")) b.classList.toggle("sel", (R.auto || "") === b.dataset.auto);
    $("#auto-def").textContent = S.settings.autoAccept ? "(ahora: sí)" : "(ahora: no)";
    const del = m.querySelector('[data-rm="delete"]');
    delete del.dataset.confirm;
    del.lastChild.textContent = "Eliminar chat";
    m.hidden = false;
    place(m, $("#r-menu"));
  });
  $("#rmenu").addEventListener("click", (e) => {
    const a = e.target.closest("[data-auto]");
    if (a) { api.SetRoomAuto(active, a.dataset.auto); R.auto = a.dataset.auto; $("#rmenu").hidden = true; toast("Preferencia guardada"); return; }
    const b = e.target.closest("[data-rm]");
    if (!b) return;
    if (b.dataset.rm === "rename") { $("#rmenu").hidden = true; startRename(); }
    if (b.dataset.rm === "recv") { $("#rmenu").hidden = true; api.OpenRecvDir(); }
    if (b.dataset.rm === "delete") {
      if (b.dataset.confirm) { $("#rmenu").hidden = true; const id = active; closeRoom(); api.DeleteRoom(id); toast("Chat eliminado"); }
      else { b.dataset.confirm = "1"; b.lastChild.textContent = "Pulsa otra vez: se borra el chat y su enlace"; }
    }
  });

  // ---------- nuevo chat / bienvenida ----------
  async function newRoom() { const id = await api.NewRoom(); openRoom(id); }
  $("#new-room").addEventListener("click", newRoom);
  $("#w-new").addEventListener("click", newRoom);
  $("#w-files").addEventListener("click", () => call(api.PickFiles("")));
  $("#w-folder").addEventListener("click", () => call(api.PickFolder("")));

  // ---------- actividad ----------
  const layout = $("#layout");
  let showAct = window.innerWidth >= 1250;
  function setAct(v) { showAct = v; layout.classList.toggle("no-act", !v); $("#wb-act").classList.toggle("on", v); }
  setAct(showAct);
  $("#wb-act").addEventListener("click", () => setAct(!showAct));

  function renderActivity() {
    if (!showAct) return;
    const [a, au] = splitSpeed(S.speed), [b, bu] = splitSpeed(S.speedIn);
    $("#sp-up").textContent = a; $("#sp-up-u").textContent = au;
    $("#sp-down").textContent = b; $("#sp-down-u").textContent = bu;
    drawSpark(S.history || [], S.historyIn || []);
    const list = [...S.transfers.filter((t) => t.status === "active"), ...S.transfers.filter((t) => t.status !== "active")].slice(0, 8);
    const act = S.transfers.filter((t) => t.status === "active").length;
    $("#tx-count").textContent = act ? act + " activa" + (act === 1 ? "" : "s") : "";
    const html = list.length ? list.map((t) => {
      const p = t.total ? Math.min(100, (t.bytes / t.total) * 100) : 0;
      const right = t.status === "done" ? "Completada" : t.status === "stalled" ? "En pausa" : t.speed > 0 ? fmtSpeed(t.speed) : "…";
      const via = { cloudflare: "Cloudflare", direct: "Directo", lan: "Red local", local: "Este PC" }[t.via] || t.via;
      return `<div class="tx ${t.dir === "in" ? "in" : "out"} ${t.status}"><div class="tx-top"><span class="dir ${t.dir === "in" ? "in" : "out"}">${t.dir === "in" ? "↓" : "↑"}</span><span class="nm" title="${esc(t.label)}">${esc(t.dir === "in" ? "De " + t.who : t.label)}</span><span class="sp">${right}</span></div><div class="tx-bar"><i style="width:${p.toFixed(1)}%"></i></div><div class="tx-sub"><span>${esc(t.dir === "in" ? via : (t.who || t.ip) + " · " + via)}${t.turbo ? " · turbo" : ""}</span><span>${fmtBytes(t.bytes)} / ${fmtBytes(t.total)}</span></div></div>`;
    }).join("") : `<div class="tx-empty">Aquí verás en directo lo que entra y sale</div>`;
    const el = $("#transfers");
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
    renderNet();
  }

  function renderNet() {
    const t = S.tunnel, d = S.direct;
    const [tc, tt] = tunnelBadge(t), [dc, dt] = directBadge(d);
    let td;
    if (t.status === "online") td = `<code>${esc(t.url.replace("https://", ""))}</code>${t.mode === "domain" ? "" : "<br>Cambia cada vez que abres TurboSEND."}${t.message ? "<br>" + esc(t.message) : ""}`;
    else if (t.status === "preparing") td = esc(t.message) + `<div class="npb"><i style="width:${Math.round((t.progress || 0) * 100)}%"></i></div>`;
    else td = esc(t.message || (t.status === "off" ? "Desactivado en ajustes." : "Conectando…"));
    if (t.domainErr) td += `<br><span class="warn">${esc(t.domainErr)}</span>`;
    let dd = esc(d.message || "");
    if (d.publicIP && (d.status === "ok" || d.status === "verified" || d.status === "manual")) dd = `<code>${esc(d.publicIP)}:${d.extPort}</code><br>` + dd;
    const html = `
      <div class="net-row"><div class="net-ic cf">${svg("cloud")}</div><div><div class="net-t">${t.mode === "domain" ? "Tu dominio" : "Cloudflare"} <i class="dot ${tc}"></i><small>${tt}</small></div><div class="net-d">${td}</div><div class="net-a"><button class="mini" data-net="tunnel">Reiniciar</button><button class="mini" data-net="log">Registro</button></div></div></div>
      <div class="net-row"><div class="net-ic direct">${svg("zap")}</div><div><div class="net-t">Directo · UPnP <i class="dot ${dc}"></i><small>${dt}</small></div><div class="net-d">${dd}</div><div class="net-a"><button class="mini" data-net="direct">Reintentar</button></div></div></div>
      <div class="net-row"><div class="net-ic lan">${svg("home")}</div><div><div class="net-t">Red local</div><div class="net-d">${S.lan ? `<code>${esc(S.lan.replace("http://", ""))}</code>` : "Sin red local"}</div></div></div>
      ${S.serverErr ? `<div class="net-row"><div class="net-ic">${svg("alert")}</div><div class="net-d" style="color:var(--bad)">${esc(S.serverErr)}</div></div>` : ""}`;
    const el = $("#net");
    if (el._h !== html) { el.innerHTML = html; el._h = html; }
  }
  $("#net").addEventListener("click", (e) => {
    const b = e.target.closest("[data-net]");
    if (!b) return;
    if (b.dataset.net === "tunnel") { api.RestartTunnel(); toast("Reiniciando el túnel…"); }
    if (b.dataset.net === "direct") { api.RetryDirect(); toast("Buscando el router otra vez…"); }
    if (b.dataset.net === "log") openLog();
  });

  const spark = $("#spark");
  function drawSpark(up, down) {
    const dpr = devicePixelRatio || 1, w = spark.clientWidth, ht = spark.clientHeight;
    if (!w) return;
    if (spark.width !== w * dpr) { spark.width = w * dpr; spark.height = ht * dpr; }
    const ctx = spark.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, ht);
    const N = 120;
    const pad = (h) => Array(Math.max(0, N - h.length)).fill(0).concat(h.slice(-N));
    const U = pad(up), D = pad(down);
    const max = Math.max(1024 * 1024, ...U, ...D) * 1.15;
    const x = (i) => (i / (N - 1)) * w, y = (v) => ht - 2 - (v / max) * (ht - 6);
    ctx.strokeStyle = "rgba(255,255,255,.05)"; ctx.lineWidth = 1;
    for (let k = 1; k < 3; k++) { ctx.beginPath(); ctx.moveTo(0, (ht / 3) * k); ctx.lineTo(w, (ht / 3) * k); ctx.stroke(); }
    const line = (data, c1, c2, fillA) => {
      ctx.beginPath();
      data.forEach((v, i) => { if (!i) ctx.moveTo(x(i), y(v)); else ctx.quadraticCurveTo(x(i - 1), y(data[i - 1]), (x(i - 1) + x(i)) / 2, (y(data[i - 1]) + y(v)) / 2); });
      ctx.lineTo(w, y(data[N - 1]));
      const g = ctx.createLinearGradient(0, 0, w, 0); g.addColorStop(0, c1); g.addColorStop(1, c2);
      ctx.strokeStyle = g; ctx.lineWidth = 2; ctx.stroke();
      ctx.lineTo(w, ht); ctx.lineTo(0, ht); ctx.closePath();
      const f = ctx.createLinearGradient(0, 0, 0, ht); f.addColorStop(0, fillA); f.addColorStop(1, "rgba(0,0,0,0)");
      ctx.fillStyle = f; ctx.fill();
    };
    line(D, "#06b6d4", "#67e8f9", "rgba(34,211,238,.22)");
    line(U, "#8b5cf6", "#6366f1", "rgba(99,102,241,.28)");
  }

  // ---------- ajustes ----------
  let draftRecv = "";
  function openSettings() {
    const s = S.settings;
    $("#s-name").value = s.hostName;
    $("#s-sounds").checked = s.sounds;
    draftRecv = s.recvDir;
    $("#s-recv").textContent = s.recvDir;
    $("#s-auto").checked = s.autoAccept;
    $("#s-limit").value = s.autoLimitGB || 0;
    $("#s-tunnel").checked = s.tunnel;
    $("#s-upnp").checked = s.upnp;
    $("#s-showdirect").checked = s.showDirectOnTunnel;
    $("#s-lan").checked = s.lanLink;
    $("#s-force").checked = s.forceDirect;
    $("#s-conns").value = s.connections;
    $("#s-conns-v").textContent = s.connections;
    $("#s-port").value = s.port;
    $("#s-host").value = s.tunnelHost || "";
    $("#s-token").value = s.tunnelToken || "";
    $("#s-token").type = "password";
    $("#s-svc").textContent = "localhost:" + S.port;
    if (api.Version) api.Version().then((v) => { if (v) $("#s-ver").textContent = "TurboSEND " + v; }).catch(() => {});
    $("#settings").hidden = false;
  }
  $("#wb-settings").addEventListener("click", openSettings);
  $("#s-conns").addEventListener("input", (e) => ($("#s-conns-v").textContent = e.target.value));
  $("#s-recv-pick").addEventListener("click", async () => { const p = await api.PickRecvDir(); if (p) { draftRecv = p; $("#s-recv").textContent = p; } });
  $("#s-recv-open").addEventListener("click", () => api.OpenRecvDir());
  $("#s-token-show").addEventListener("click", () => { const t = $("#s-token"); t.type = t.type === "password" ? "text" : "password"; });
  $("#s-save").addEventListener("click", async () => {
    const s = {
      hostName: $("#s-name").value, sounds: $("#s-sounds").checked, recvDir: draftRecv,
      autoAccept: $("#s-auto").checked, autoLimitGB: +$("#s-limit").value || 0,
      tunnel: $("#s-tunnel").checked, upnp: $("#s-upnp").checked, showDirectOnTunnel: $("#s-showdirect").checked, lanLink: $("#s-lan").checked,
      forceDirect: $("#s-force").checked, connections: +$("#s-conns").value, port: +$("#s-port").value || S.settings.port,
      tunnelHost: $("#s-host").value, tunnelToken: $("#s-token").value,
    };
    const portChanged = s.port !== S.settings.port;
    const domChanged = s.tunnelHost.trim() !== (S.settings.tunnelHost || "") || s.tunnelToken.trim() !== (S.settings.tunnelToken || "");
    try {
      await api.SaveSettings(s);
      $("#settings").hidden = true;
      toast(portChanged ? "Guardado. El puerto nuevo se usará al reiniciar TurboSEND." : domChanged ? "Guardado. Reconectando el túnel…" : "Ajustes guardados");
    } catch (e) { toast(String(e), "error"); }
  });
  $("#s-log").addEventListener("click", openLog);
  $("#s-data").addEventListener("click", () => api.OpenDataDir());
  async function openLog() {
    const lines = await api.TunnelLog();
    $("#log").textContent = lines && lines.length ? lines.join("\n") : "Sin registro todavía.";
    $("#logm").hidden = false;
    $("#log").scrollTop = $("#log").scrollHeight;
  }
  document.addEventListener("click", (e) => { if (e.target.closest("[data-close]") || e.target.classList.contains("modal")) { const m = e.target.closest(".modal"); if (m) m.hidden = true; } });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") { document.querySelectorAll(".modal").forEach((m) => (m.hidden = true)); closePops(); } });

  // ---------- ventana ----------
  $("#wb-min").addEventListener("click", () => rt.WindowMinimise());
  $("#wb-max").addEventListener("click", () => rt.WindowToggleMaximise());
  $("#wb-close").addEventListener("click", () => rt.Quit());
  $(".titlebar").addEventListener("dblclick", (e) => { if (!e.target.closest("button,.pill")) rt.WindowToggleMaximise(); });

  // ---------- arrastrar ----------
  let depth = 0;
  const veil = $("#veil");
  const isFiles = (e) => e.dataTransfer && [...e.dataTransfer.types].includes("Files");
  window.addEventListener("dragenter", (e) => {
    if (!isFiles(e)) return;
    depth++;
    $("#veil-t").textContent = R && active ? `Suelta para enviar a «${R.name}»` : "Suelta para crear un chat";
    $("#veil-p").textContent = R && active ? "Aparecerá en el chat listo para descargar" : "Se crea un chat con un enlace para compartir";
    veil.classList.add("on");
  });
  window.addEventListener("dragleave", (e) => { if (isFiles(e) && --depth <= 0) { depth = 0; veil.classList.remove("on"); } });
  window.addEventListener("dragover", (e) => { if (isFiles(e)) e.preventDefault(); });
  window.addEventListener("drop", (e) => { depth = 0; veil.classList.remove("on"); if (isFiles(e)) e.preventDefault(); });

  // ---------- avisos ----------
  let actx = null;
  function ding() {
    if (!S || !S.settings.sounds) return;
    try {
      actx = actx || new AudioContext();
      const t0 = actx.currentTime;
      [[740, 0], [1110, 0.09]].forEach(([f, d]) => {
        const o = actx.createOscillator(), g = actx.createGain();
        o.type = "sine"; o.frequency.value = f;
        g.gain.setValueAtTime(0.0001, t0 + d); g.gain.exponentialRampToValueAtTime(0.1, t0 + d + 0.02); g.gain.exponentialRampToValueAtTime(0.0001, t0 + d + 0.35);
        o.connect(g).connect(actx.destination); o.start(t0 + d); o.stop(t0 + d + 0.4);
      });
    } catch {}
  }
  rt.EventsOn("notify", (n) => {
    const focused = document.hasFocus();
    if (n.room === active && focused) return;
    if (n.kind === "msg" || n.kind === "in-done") ding();
    if (!focused) api.FlashTaskbar();
    if (n.room !== active || !focused) {
      const room = S && S.rooms.find((r) => r.id === n.room);
      const txt = n.kind === "in-done" ? `Recibido en «${room ? room.name : "un chat"}»` : n.text;
      toast(txt, "msg", () => openRoom(n.room));
    }
  });
  window.addEventListener("focus", () => { if (active && R && R.unread) api.MarkRead(active); });

  // ---------- eventos desde Go ----------
  rt.EventsOn("state", (st) => {
    S = st;
    renderPills();
    renderRooms();
    renderActivity();
    applyLive();
    if (active && !S.rooms.some((r) => r.id === active)) closeRoom();
  });
  rt.EventsOn("roomchg", (id) => { if (id === active) scheduleLoad(); });
  rt.EventsOn("typing", (t) => {
    if (t.room !== active) return;
    typingWho = t.who || [];
    if (R) renderRoomHead();
    const el = $("#typing");
    el.innerHTML = typingWho.length ? `<span class="dots"><i></i><i></i><i></i></span>${esc(typingWho.join(", "))} está escribiendo…` : "";
    clearTimeout(el._t);
    el._t = setTimeout(() => { typingWho = []; el.innerHTML = ""; if (R) renderRoomHead(); }, 5000);
  });
  rt.EventsOn("open", (id) => openRoom(id));
  rt.EventsOn("busy", (on) => $("#busy").classList.toggle("on", !!on));
  rt.EventsOn("toast", (t) => toast(t.text, t.kind));

  api.GetState().then((st) => {
    S = st;
    renderPills(); renderRooms(); renderActivity();
    if (S.rooms.length) openRoom(S.rooms[0].id);
  });
  setInterval(() => { if (R) renderRoom(false); }, 60000);
})();
