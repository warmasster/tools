(() => {
  "use strict";
  const { fmtBytes, fmtSpeed, fmtTime, esc, canTurbo, TurboJob, Uploader, nf0 } = window.EnvLib;
  const $ = (s, el = document) => el.querySelector(s);

  const roomId = decodeURIComponent(location.pathname.split("/").filter(Boolean)[1] || "");
  const API = "/api/r/" + encodeURIComponent(roomId);
  const LS = {
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch {} },
  };
  let mid = LS.get("env:mid:" + roomId);
  if (!mid) {
    mid = Array.from(crypto.getRandomValues(new Uint8Array(24)), (b) => b.toString(16).padStart(2, "0")).join("");
    LS.set("env:mid:" + roomId, mid);
  }

  const COLORS = ["#f472b6", "#fb923c", "#facc15", "#4ade80", "#2dd4bf", "#38bdf8", "#818cf8", "#c084fc", "#f87171", "#a3e635"];
  const I = {
    file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>',
    folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/>',
    image: '<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>',
    video: '<path d="m16 13 5.2 3.5a.5.5 0 0 0 .8-.4V7.9a.5.5 0 0 0-.8-.4L16 11"/><rect x="2" y="6" width="14" height="12" rx="2"/>',
    music: '<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
    archive: '<rect x="2" y="3" width="20" height="5" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/>',
    doc: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M16 13H8"/><path d="M16 17H8"/>',
    app: '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M2 8h20"/>',
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>',
    zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    alert: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/>',
    refresh: '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
    upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m17 8-5-5-5 5"/><path d="M12 3v12"/>',
  };
  const svg = (n) => `<svg viewBox="0 0 24 24">${I[n] || I.file}</svg>`;
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
  let S = null;            // {room, me, via, conns, chunk, inChunk, direct}
  let es = null;
  let online = false;
  let errCount = 0;
  const els = new Map();   // id -> elemento de mensaje
  const dls = new Map();   // envío del anfitrión -> {job, state, p, err, res}
  const ups = new Map();   // envío propio -> {files, up, p, state, err}
  const prog = new Map();  // progreso de envíos entrantes visto por el servidor
  let typingList = [];
  let hiddenUnread = 0;

  const msgs = $("#msgs");
  const text = $("#text");
  const sendBtn = $("#send");

  function toast(t, kind) {
    const el = document.createElement("div");
    el.className = "toast" + (kind ? " " + kind : "");
    el.textContent = t;
    $("#toasts").appendChild(el);
    setTimeout(() => el.remove(), kind === "error" ? 5000 : 2600);
  }

  async function api(path, opts) {
    const r = await fetch(API + path, Object.assign({ cache: "no-store", headers: { "Content-Type": "application/json", "X-Member": mid } }, opts || {}));
    let data = null;
    try { data = await r.json(); } catch {}
    if (!r.ok) throw new Error((data && data.error) || "HTTP " + r.status);
    return data;
  }

  // ---------- arranque ----------
  async function boot() {
    let data;
    try {
      data = await api("");
    } catch (e) {
      if (String(e.message).includes("not_found")) return gone("Este chat ya no existe", "Puede que lo hayan borrado o que el enlace esté mal copiado.");
      msgs.innerHTML = `<div class="sys">No se puede conectar con el ordenador de origen.<br>Puede que esté apagado o que haya cerrado TurboSEND.<br><br><button class="fbtn" id="rt">${svg("refresh")} Reintentar</button></div>`;
      $("#rt").onclick = boot;
      return;
    }
    S = data;
    if (!S.me) return askName();
    online = true;
    start();
  }

  function askName() {
    const host = S.room.hostName || "tu amigo";
    $("#join-t").textContent = `${host} te invita a un chat`;
    $("#join-name").value = LS.get("env:name") || "";
    $("#join").hidden = false;
    setTimeout(() => $("#join-name").focus(), 50);
    renderHeader();
  }
  $("#join-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = $("#join-name").value.trim();
    if (!name) return;
    try {
      S = await api("/join", { method: "POST", body: JSON.stringify({ mid, name }) });
      LS.set("env:name", name);
      $("#join").hidden = true;
      start();
    } catch (err) { toast(err.message, "error"); }
  });

  function start() {
    text.disabled = false;
    sendBtn.disabled = false;
    renderAll(true);
    connect();
  }

  // canal en tiempo real por long-polling (los túneles temporales no dejan pasar SSE)
  let since = -1, polling = false, stopped = false;
  const handlers = {
    hello: (d) => { S = Object.assign(S, d); renderAll(false); },
    msg: (m) => { addMessage(m); if (m.from !== S.me.pid) ping(); },
    out: (v) => { S.room.out[v.id] = v; renderAll(false); },
    in: (v) => {
      const old = S.room.in[v.id];
      if (old && old.files && !v.files) v.files = old.files;
      S.room.in[v.id] = v;
      const u = ups.get(v.id);
      if (u && (v.status === "rejected" || v.status === "cancelled") && u.up) u.up.cancel();
      if (u && v.status === "done") u.state = "done";
      renderAll(false);
    },
    members: (m) => { S.room.members = m; renderHeader(); },
    typing: (list) => { typingList = list.filter((t) => t.pid !== S.me.pid); renderTyping(); },
    progress: (p) => { prog.set(p.bundle, p); applyLive(); },
    upload: (d) => startUpload(d.bundle),
    room: (d) => { if (d.name) S.room.name = d.name; renderHeader(); },
    deleted: () => { stopped = true; gone("Este chat se ha cerrado", `${hostName()} ha borrado el chat.`); },
  };
  function connect() {
    if (polling) return;
    polling = true;
    (async () => {
      while (!stopped) {
        try {
          const r = await fetch(`${API}/poll?since=${since}`, { cache: "no-store", headers: { "X-Member": mid } });
          if (r.status === 404) { stopped = true; gone("Este chat ya no existe", "Puede que lo hayan borrado."); break; }
          if (r.status === 403) { stopped = true; location.reload(); break; }
          if (!r.ok) throw new Error("HTTP " + r.status);
          const d = await r.json();
          if (!online || errCount) { online = true; errCount = 0; renderHeader(); setBanner(""); }
          since = d.seq;
          for (const ev of d.events) {
            try { handlers[ev.name] && handlers[ev.name](ev.data); } catch (e) { console.error(e); }
          }
        } catch (e) {
          online = false;
          errCount++;
          renderHeader();
          if (errCount >= 2) setBanner(`${hostName()} está desconectado. En cuanto vuelva a abrir TurboSEND seguiréis por donde ibais.`);
          await new Promise((res) => setTimeout(res, Math.min(5000, 800 * errCount)));
        }
      }
      polling = false;
    })();
  }

  function gone(t, p) {
    text.disabled = true;
    sendBtn.disabled = true;
    $("#composer").style.display = "none";
    msgs.innerHTML = `<div class="sys" style="margin:auto"><b style="font-size:17px">${esc(t)}</b><br>${esc(p)}</div>`;
    $("#title").textContent = "TurboSEND";
    $("#sub-txt").textContent = "";
  }

  function setBanner(t, info) {
    const b = $("#banner");
    b.hidden = !t;
    b.className = "banner" + (info ? " info" : "");
    b.textContent = t;
  }

  const hostName = () => (S && S.room && S.room.hostName) || "El anfitrión";
  function member(pid) {
    if (pid === "host") return { name: hostName(), color: -1 };
    return (S.room.members || []).find((m) => m.pid === pid) || { name: "Alguien", color: 0 };
  }
  const colorOf = (m) => (m.color < 0 ? "#a78bfa" : COLORS[m.color % COLORS.length]);
  const initial = (n) => (n || "?").trim().charAt(0).toUpperCase();

  // ---------- cabecera ----------
  function renderHeader() {
    if (!S) return;
    const host = hostName();
    $("#title").textContent = host;
    document.title = (hiddenUnread ? `(${hiddenUnread}) ` : "") + `${host} · TurboSEND`;
    $("#host-av").textContent = initial(host);
    const dot = $("#sub .dot");
    dot.className = "dot " + (online ? "on" : errCount >= 2 ? "off" : "wait");
    $("#sub-txt").textContent = online ? "conectado" : errCount >= 2 ? "desconectado" : "conectando…";
    const others = (S.room.members || []).filter((m) => !S.me || m.pid !== S.me.pid);
    $("#members").innerHTML = others.slice(0, 5).map((m) =>
      `<div class="av" style="--avc:${colorOf(m)}" title="${esc(m.name)}${m.online ? " · conectado" : ""}">${esc(initial(m.name))}<i class="st ${m.online ? "on" : ""}"></i></div>`
    ).join("") + (others.length > 5 ? `<span class="more">+${others.length - 5}</span>` : "");
    const b = $("#boost");
    if (S.direct) { b.hidden = false; b.href = S.direct; b.title = "Conexión directa al PC de " + host + " (más rápida, sin pasar por Cloudflare)"; }
    else b.hidden = true;
  }

  function renderTyping() {
    const t = $("#typing");
    if (!typingList.length) { t.innerHTML = ""; return; }
    const names = typingList.map((x) => x.name);
    const who = names.length === 1 ? names[0] + " está escribiendo" : names.slice(0, -1).join(", ") + " y " + names.slice(-1) + " están escribiendo";
    t.innerHTML = `<span class="dots"><i></i><i></i><i></i></span>${esc(who)}…`;
    clearTimeout(renderTyping.t);
    renderTyping.t = setTimeout(() => { typingList = []; renderTyping(); }, 5000);
  }

  // ---------- mensajes ----------
  const timeFmt = new Intl.DateTimeFormat("es-ES", { hour: "2-digit", minute: "2-digit" });
  const dayFmt = new Intl.DateTimeFormat("es-ES", { weekday: "long", day: "numeric", month: "long" });
  function dayLabel(ms) {
    const d = new Date(ms), now = new Date();
    const a = new Date(d.getFullYear(), d.getMonth(), d.getDate()), b = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const diff = Math.round((b - a) / 86400000);
    if (diff === 0) return "Hoy";
    if (diff === 1) return "Ayer";
    const s = dayFmt.format(d);
    return s.charAt(0).toUpperCase() + s.slice(1);
  }
  function linkify(s) {
    return esc(s).replace(/\bhttps?:\/\/[^\s<]+[^\s<.,;:!?)\]'"]/g, (u) => `<a href="${u}" target="_blank" rel="noopener noreferrer">${u}</a>`);
  }
  const emojiOnly = (s) => /^(?:\p{Extended_Pictographic}|\p{Emoji_Component}|‍|️|\s){1,12}$/u.test(s) && !/[0-9#*]/.test(s);

  function addMessage(m) {
    if (S.room.messages.some((x) => x.id === m.id)) return;
    S.room.messages.push(m);
    renderAll(false);
  }

  function msgHTML(m, prev, next) {
    if (m.kind === "system") return `<div class="sys">${esc(m.text)}</div>`;
    const mine = S.me && m.from === S.me.pid;
    const who = member(m.from);
    const sameAsPrev = prev && prev.from === m.from && prev.kind !== "system" && m.time - prev.time < 5 * 60e3;
    const sameAsNext = next && next.from === m.from && next.kind !== "system" && next.time - m.time < 5 * 60e3;
    const cls = ["row", mine ? "me" : "", sameAsPrev ? "" : "first", sameAsNext ? "" : "last"].join(" ");
    const av = `<div class="av ${m.from === "host" ? "host" : ""}" style="--avc:${colorOf(who)}">${esc(initial(who.name))}</div>`;
    const nm = !mine && !sameAsPrev ? `<span class="nm" style="--nmc:${colorOf(who)}">${esc(who.name)}</span>` : "";
    const tm = `<span class="tm">${timeFmt.format(new Date(m.time))}</span>`;
    let body = "";
    if (m.kind === "text") {
      const big = emojiOnly(m.text);
      body = `<div class="bub${big ? " emoji" : ""}">${big ? "" : nm}<span class="tx">${big ? esc(m.text) : linkify(m.text)}</span>${tm}</div>`;
    } else if (m.kind === "out") {
      body = `<div class="bub file" data-out="${esc(m.bundle)}">${nm}${outCard(m.bundle)}${tm}</div>`;
    } else if (m.kind === "in") {
      body = `<div class="bub file" data-in="${esc(m.bundle)}">${nm}${inCard(m.bundle, mine)}${tm}</div>`;
    }
    return `<div class="${cls}">${av}${body}</div>`;
  }

  function fileHead(name, kind, meta) {
    const t = typeOf(name, kind);
    return `<div class="fc"><div class="tile" style="--tile:${t.bg}">${svg(t.icon)}</div><div class="fc-i"><div class="fc-n" title="${esc(name)}">${esc(name)}</div><div class="fc-m">${meta}</div></div></div>`;
  }
  const metaOf = (v) => fmtBytes(v.total) + (v.kind === "multi" ? ` · ${nf0.format(v.count)} archivo${v.count === 1 ? "" : "s"}` : "");
  const progBlock = (lanes) => `<div class="pbar"><i data-bar></i></div><div class="pinfo"><span data-l></span><span data-r></span></div>${lanes ? `<div class="lanes" data-lanes>${"<i><b></b></i>".repeat(lanes)}</div>` : ""}`;

  // tarjeta de un envío del anfitrión
  function outCard(id) {
    const v = S.room.out[id];
    if (!v || v.gone) return fileHead("Envío retirado", "file", `${esc(hostName())} lo ha quitado del chat`);
    let html = fileHead(v.name, v.kind, metaOf(v));
    if (v.preview && v.preview.length) html += `<div class="preview">${v.preview.map((p) => `<span>${esc(p)}</span>`).join("")}${v.count > v.preview.length ? `<span>+${nf0.format(v.count - v.preview.length)}</span>` : ""}</div>`;
    const d = dls.get(id);
    const m = S.me ? "?m=" + encodeURIComponent(S.me.pid) : "";
    const normal = v.kind === "multi" ? `/z/${encodeURIComponent(id)}${m}` : `/d/${encodeURIComponent(id)}/0${m}`;
    if (d && d.state === "run") {
      html += progBlock(d.lanes) + `<div class="fc-acts"><button class="fbtn ghost" data-act="dl-cancel" data-id="${esc(id)}">${svg("x")} Cancelar</button></div>`;
    } else if (d && d.state === "fail") {
      html += `<div class="fc-st bad">${svg("alert")}<span>${esc(d.err)}</span></div><div class="fc-acts"><button class="fbtn pri" data-act="dl-resume" data-id="${esc(id)}">${svg("refresh")} Reanudar</button><button class="fbtn ghost" data-act="dl-cancel" data-id="${esc(id)}">Cancelar</button></div>`;
    } else if (d && d.state === "done") {
      html += `<div class="fc-st ok">${svg("check")}<span>Descargado y verificado · ${fmtTime(d.res.secs)} · media ${fmtSpeed(d.res.avg)}</span></div>`;
    } else if (d && d.state === "normal") {
      html += `<div class="fc-st">${svg("download")}<span>Descargando con tu navegador: mira su barra de descargas.</span></div>`;
    } else {
      const got = LS.get("env:got:" + id);
      const turbo = canTurbo && (v.kind === "multi" || v.total >= 24 * 1024 * 1024);
      html += `<div class="fc-acts">`;
      if (turbo) {
        html += `<button class="fbtn pri" data-act="dl-turbo" data-id="${esc(id)}">${svg("zap")} ${v.kind === "multi" ? "Descargar en carpeta" : "Descargar"}</button>`;
        html += `<a class="fbtn ghost" href="${normal}" data-act="dl-normal" data-id="${esc(id)}" title="${v.kind === "multi" ? "Descargar todo como un ZIP" : "Descarga normal del navegador"}">${v.kind === "multi" ? "ZIP" : svg("download")}</a>`;
      } else {
        html += `<a class="fbtn pri" href="${normal}" data-act="dl-normal" data-id="${esc(id)}">${svg("download")} Descargar${v.kind === "multi" ? " (ZIP)" : ""}</a>`;
      }
      html += `</div>`;
      if (got) html += `<div class="fc-st ok">${svg("check")}<span>Ya lo descargaste</span></div>`;
    }
    return html;
  }

  // tarjeta de algo que un invitado envía al anfitrión
  function inCard(id, mine) {
    const v = S.room.in[id];
    if (!v) return fileHead("Archivos", "file", "");
    let html = fileHead(v.name, v.kind, metaOf(v));
    const u = ups.get(id);
    const host = hostName();
    const st = v.status;
    if (st === "done" || (u && u.state === "done")) {
      html += `<div class="fc-st ok">${svg("check")}<span>${mine ? "Entregado a " + esc(host) : "Entregado"}</span></div>`;
    } else if (st === "rejected") {
      html += `<div class="fc-st bad">${svg("x")}<span>${esc(host)} no lo ha aceptado</span></div>`;
    } else if (st === "cancelled") {
      html += `<div class="fc-st">${svg("x")}<span>Cancelado</span></div>`;
    } else if (st === "failed") {
      html += `<div class="fc-st bad">${svg("alert")}<span>${esc(v.error || "Ha fallado")}</span></div>`;
    } else if (st === "offered") {
      html += `<div class="fc-st">${svg("clock")}<span>${mine ? "Esperando a que " + esc(host) + " lo acepte…" : "Esperando a " + esc(host)}</span></div>`;
      if (mine) {
        if (!u || !u.files) html += `<div class="fc-st">${svg("alert")}<span>Mantén esta página abierta: cuando lo acepte, se enviará desde aquí.</span></div>`;
        html += `<div class="fc-acts"><button class="fbtn ghost" data-act="up-cancel" data-id="${esc(id)}">${svg("x")} Cancelar</button></div>`;
      }
    } else { // accepted / transferring
      const running = u && u.up;
      if (mine && !running && !(u && u.files)) {
        html += `<div class="fc-st">${svg("upload")}<span>${esc(host)} lo ha aceptado. Vuelve a elegir ${v.kind === "multi" ? "los archivos" : "el archivo"} para enviarlo${v.received ? " (seguirá donde se quedó)" : ""}.</span></div>`;
        html += `<div class="fc-acts"><button class="fbtn pri" data-act="reselect" data-id="${esc(id)}" data-kind="files">${svg("file")} Elegir archivos</button>${v.kind === "multi" ? `<button class="fbtn" data-act="reselect" data-id="${esc(id)}" data-kind="dir">${svg("folder")} Carpeta</button>` : ""}<button class="fbtn ghost" data-act="up-cancel" data-id="${esc(id)}">${svg("x")}</button></div>`;
      } else {
        html += progBlock(0);
        if (mine) html += `<div class="fc-acts"><button class="fbtn ghost" data-act="up-cancel" data-id="${esc(id)}">${svg("x")} Cancelar</button></div>`;
        if (u && u.state === "fail") html += `<div class="fc-st bad">${svg("alert")}<span>${esc(u.err)}</span></div><div class="fc-acts"><button class="fbtn pri" data-act="up-retry" data-id="${esc(id)}">${svg("refresh")} Reintentar</button></div>`;
      }
    }
    return html;
  }

  function nearBottom() { return msgs.scrollHeight - msgs.scrollTop - msgs.clientHeight < 140; }

  function renderAll(forceBottom) {
    if (!S) return;
    renderHeader();
    const stick = forceBottom || nearBottom();
    const list = S.room.messages;
    const seen = new Set();
    let prevEl = null;
    const skel = msgs.querySelector(".skel");
    if (skel) skel.remove();
    if (!list.length && !msgs.querySelector(".empty")) {
      msgs.innerHTML = `<div class="sys empty" style="margin:auto">Aquí aparecerá todo lo que os mandéis.<br>Escribe algo o arrastra archivos a esta ventana para enviárselos a <b>${esc(hostName())}</b>.</div>`;
    } else if (list.length) {
      const e = msgs.querySelector(".empty");
      if (e) e.remove();
    }
    if (S.room.older && list.length) {
      let o = els.get("older");
      if (!o) {
        o = document.createElement("div");
        o.className = "sys";
        o.innerHTML = `<button class="fbtn" data-act="older">${svg("clock")} Ver mensajes anteriores</button>`;
        els.set("older", o);
      }
      seen.add("older");
      if (msgs.firstChild !== o) msgs.insertBefore(o, msgs.firstChild);
      prevEl = o;
    }
    let lastDay = "";
    for (let i = 0; i < list.length; i++) {
      const m = list[i];
      const day = dayLabel(m.time);
      if (day !== lastDay) {
        lastDay = day;
        const key = "day:" + day;
        seen.add(key);
        let d = els.get(key);
        if (!d) { d = document.createElement("div"); d.className = "day"; d.textContent = day; els.set(key, d); }
        const want = prevEl ? prevEl.nextSibling : msgs.firstChild;
        if (want !== d) msgs.insertBefore(d, want);
        prevEl = d;
      }
      const html = msgHTML(m, list[i - 1], list[i + 1]);
      seen.add(m.id);
      let el = els.get(m.id);
      if (!el) { el = document.createElement("div"); el.style.display = "contents"; els.set(m.id, el); }
      if (el._h !== html) { el.innerHTML = html; el._h = html; }
      const want = prevEl ? prevEl.nextSibling : msgs.firstChild;
      if (want !== el) msgs.insertBefore(el, want);
      prevEl = el;
    }
    for (const [k, el] of els) if (!seen.has(k)) { el.remove(); els.delete(k); }
    applyLive();
    if (stick) msgs.scrollTop = msgs.scrollHeight;
  }

  // aplica los números de progreso sin rehacer las tarjetas
  function applyLive() {
    for (const [id, d] of dls) {
      if (d.state !== "run" || !d.p) continue;
      const card = msgs.querySelector(`[data-out="${CSS.escape(id)}"]`);
      if (!card) continue;
      setProg(card, d.p.done, d.p.total, d.p.speed, d.p.phase);
      const lanes = card.querySelectorAll("[data-lanes] i");
      d.p.lanes.forEach((l, k) => {
        if (!lanes[k]) return;
        lanes[k].className = l.retry ? "retry" : "";
        lanes[k].firstChild.style.width = Math.round(l.frac * 100) + "%";
      });
    }
    for (const card of msgs.querySelectorAll("[data-in]")) {
      const id = card.dataset.in;
      const u = ups.get(id);
      const v = S.room.in[id];
      if (!v || !card.querySelector("[data-bar]")) continue;
      if (u && u.p) setProg(card, u.p.done, u.p.total, u.p.speed, "");
      else {
        const p = prog.get(id);
        setProg(card, p ? p.bytes : v.received, v.total, p ? p.speed : 0, p ? "" : "Esperando…");
      }
    }
  }
  function setProg(card, done, total, speed, phase) {
    const pct = total ? Math.min(100, (done / total) * 100) : 100;
    const bar = card.querySelector("[data-bar]");
    if (bar) bar.style.width = pct.toFixed(1) + "%";
    const l = card.querySelector("[data-l]"), r = card.querySelector("[data-r]");
    if (l) l.textContent = phase || (speed > 0 ? `${fmtSpeed(speed)} · quedan ${fmtTime((total - done) / speed)}` : "Conectando…");
    if (r) r.textContent = `${Math.floor(pct)}% · ${fmtBytes(done)}`;
  }

  // ---------- descargas de lo que envía el anfitrión ----------
  async function startTurbo(id) {
    const v = S.room.out[id];
    if (!v) return;
    let targets;
    try {
      if (v.kind === "multi") targets = { dir: await showDirectoryPicker({ id: "enviador", mode: "readwrite", startIn: "downloads" }) };
      else targets = { single: await showSaveFilePicker({ id: "enviador", suggestedName: v.name, startIn: "downloads" }) };
    } catch (e) {
      if (e && e.name === "AbortError") return;
      return toast("Tu navegador no deja elegir dónde guardar; usa la descarga normal.", "error");
    }
    let info;
    try {
      const r = await fetch("/api/s/" + encodeURIComponent(id), { cache: "no-store" });
      if (!r.ok) throw new Error();
      info = await r.json();
    } catch { return toast("Ese envío ya no está disponible", "error"); }
    info.conns = S.conns || info.conns;
    info.m = S.me.pid;
    const d = { state: "run", p: null, lanes: Math.max(1, Math.min(16, info.conns || 4)) };
    d.job = new TurboJob(info, targets, {
      progress: (p) => { d.p = p; applyLive(); },
      done: (res) => { d.state = "done"; d.res = res; LS.set("env:got:" + id, "1"); renderAll(false); },
      fail: (e) => { d.state = "fail"; d.err = e && e.message ? e.message : String(e); renderAll(false); },
    });
    dls.set(id, d);
    renderAll(false);
    d.job.run();
  }

  // ---------- enviar archivos al anfitrión ----------
  async function offer(list) {
    if (!list.length) return;
    try {
      const r = await api("/offer", { method: "POST", body: JSON.stringify({ mid, files: list.map((x) => ({ rel: x.rel, size: x.file.size })) }) });
      ups.set(r.bundle, { files: list.map((x, i) => ({ i, file: x.file, size: x.file.size, rel: x.rel })), state: "wait" });
      renderAll(true);
    } catch (e) { toast(e.message, "error"); }
  }

  async function startUpload(bid) {
    const u = ups.get(bid);
    if (!u || !u.files) { renderAll(false); return; }
    if (u.up && u.state === "run") return;
    let st;
    try { st = await api(`/in/${encodeURIComponent(bid)}`); }
    catch (e) { u.state = "fail"; u.err = e.message; renderAll(false); return; }
    const missing = {};
    for (const f of st.files) missing[f.i] = f.missing;
    const files = u.files.filter((f) => missing[f.i] !== undefined);
    u.state = "run";
    u.up = new Uploader({ room: roomId, mid, bid, chunk: st.chunk, conns: S.conns, files, missing }, {
      progress: (p) => { u.p = p; applyLive(); },
      done: () => { u.state = "sent"; u.up = null; renderAll(false); },
      fail: (e) => { u.state = "fail"; u.err = e.message; u.up = null; renderAll(false); },
    });
    renderAll(false);
    u.up.run();
  }

  let reselectFor = null;
  function pick(kind, forBundle) {
    reselectFor = forBundle || null;
    const inp = kind === "dir" ? $("#f-dir") : $("#f-files");
    inp.value = "";
    inp.click();
  }
  function onPicked(input) {
    const list = [...input.files].map((f) => ({ file: f, rel: f.webkitRelativePath || f.name }));
    if (!list.length) return;
    if (reselectFor) return reselect(reselectFor, list);
    offer(list);
  }
  $("#f-files").addEventListener("change", (e) => onPicked(e.target));
  $("#f-dir").addEventListener("change", (e) => onPicked(e.target));

  // vuelve a asociar los archivos elegidos con un envío ya aceptado (tras recargar la página)
  function reselect(bid, list) {
    reselectFor = null;
    const v = S.room.in[bid];
    if (!v || !v.files) return;
    const byRel = new Map(list.map((x) => [x.rel, x.file]));
    const byNameSize = new Map(list.map((x) => [x.file.name + "|" + x.file.size, x.file]));
    const files = [];
    let missing = 0;
    v.files.forEach((f, i) => {
      const name = f.rel.split("/").pop();
      const file = (byRel.get(f.rel) && byRel.get(f.rel).size === f.size ? byRel.get(f.rel) : null) || byNameSize.get(name + "|" + f.size);
      if (file) files.push({ i, file, size: f.size, rel: f.rel });
      else missing++;
    });
    if (!files.length) return toast("Esos no son los archivos de este envío", "error");
    if (missing) toast(`Faltan ${missing} archivo(s) de este envío; se enviará el resto`, "error");
    ups.set(bid, { files, state: "wait" });
    startUpload(bid);
  }

  async function cancelUpload(bid) {
    const u = ups.get(bid);
    if (u && u.up) u.up.cancel();
    try { await api(`/in/${encodeURIComponent(bid)}/cancel`, { method: "POST" }); } catch {}
  }

  // ---------- clics en tarjetas ----------
  msgs.addEventListener("click", (e) => {
    const b = e.target.closest("[data-act]");
    if (!b) return;
    const id = b.dataset.id;
    switch (b.dataset.act) {
      case "dl-turbo": startTurbo(id); break;
      case "dl-normal":
        dls.set(id, { state: "normal" });
        LS.set("env:got:" + id, "1");
        setTimeout(() => renderAll(false), 50);
        break; // el enlace sigue su curso
      case "dl-cancel": { const d = dls.get(id); if (d && d.job) d.job.cancel(); dls.delete(id); renderAll(false); break; }
      case "dl-resume": { const d = dls.get(id); if (d && d.job) { d.state = "run"; renderAll(false); d.job.resume(); } break; }
      case "up-cancel": cancelUpload(id); break;
      case "up-retry": { const u = ups.get(id); if (u) { u.state = "wait"; startUpload(id); } break; }
      case "reselect": pick(b.dataset.kind, id); break;
      case "older": loadOlder(b); break;
    }
  });

  // Los chats largos llegan por partes: al entrar, solo los últimos mensajes.
  let loadingOlder = false;
  async function loadOlder(btn) {
    const first = S.room.messages[0];
    if (!first || loadingOlder) return;
    loadingOlder = true;
    btn.disabled = true;
    try {
      const d = await api(`/history?before=${encodeURIComponent(first.id)}`);
      const have = new Set(S.room.messages.map((m) => m.id));
      const add = (d.messages || []).filter((m) => !have.has(m.id));
      Object.assign(S.room.out, d.out || {});
      S.room.older = !!d.older && add.length > 0;
      const h = msgs.scrollHeight, top = msgs.scrollTop;
      S.room.messages = add.concat(S.room.messages);
      renderAll(false);
      msgs.scrollTop = top + (msgs.scrollHeight - h); // que no salte lo que estabas leyendo
    } catch (e) {
      toast(e.message, "error");
    } finally {
      loadingOlder = false;
      btn.disabled = false;
    }
  }

  // ---------- escribir ----------
  function autosize() { text.style.height = "auto"; text.style.height = Math.min(160, text.scrollHeight) + "px"; text.style.overflowY = text.scrollHeight > 160 ? "auto" : "hidden"; }
  let lastTyping = 0;
  text.addEventListener("input", () => {
    autosize();
    if (Date.now() - lastTyping > 2500 && text.value.trim()) {
      lastTyping = Date.now();
      api("/typing", { method: "POST", body: JSON.stringify({ mid }) }).catch(() => {});
    }
  });
  text.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); }
  });
  sendBtn.addEventListener("click", send);
  async function send() {
    const t = text.value.trim();
    if (!t) return;
    text.value = "";
    autosize();
    lastTyping = 0;
    try {
      const m = await api("/msg", { method: "POST", body: JSON.stringify({ mid, text: t }) });
      addMessage(m);
      msgs.scrollTop = msgs.scrollHeight;
    } catch (e) {
      text.value = t;
      autosize();
      toast(online ? e.message : "Sin conexión: el mensaje no se ha enviado", "error");
    }
  }

  // adjuntar
  const menu = $("#att-menu");
  $("#att").addEventListener("click", (e) => { e.stopPropagation(); menu.hidden = !menu.hidden; });
  document.addEventListener("click", () => { menu.hidden = true; });
  menu.addEventListener("click", (e) => { const b = e.target.closest("[data-pick]"); if (b) pick(b.dataset.pick); });

  // ---------- arrastrar y soltar ----------
  const veil = $("#veil");
  let depth = 0;
  const hasFiles = (e) => e.dataTransfer && [...e.dataTransfer.types].includes("Files");
  window.addEventListener("dragenter", (e) => {
    if (!hasFiles(e) || !S || !S.me) return;
    depth++;
    $("#veil-t").textContent = "Suelta para enviárselo a " + hostName();
    veil.classList.add("on");
  });
  window.addEventListener("dragleave", (e) => { if (hasFiles(e) && --depth <= 0) { depth = 0; veil.classList.remove("on"); } });
  window.addEventListener("dragover", (e) => { if (hasFiles(e)) e.preventDefault(); });
  window.addEventListener("drop", async (e) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    depth = 0;
    veil.classList.remove("on");
    if (!S || !S.me) return;
    const entries = [...e.dataTransfer.items].filter((i) => i.kind === "file").map((i) => i.webkitGetAsEntry && i.webkitGetAsEntry()).filter(Boolean);
    const plain = [...e.dataTransfer.files];
    let list = [];
    if (entries.length) {
      const walk = async (entry, path) => {
        if (entry.isFile) {
          const f = await new Promise((res, rej) => entry.file(res, rej));
          list.push({ file: f, rel: path + f.name });
        } else if (entry.isDirectory) {
          const reader = entry.createReader();
          let batch;
          do {
            batch = await new Promise((res, rej) => reader.readEntries(res, rej));
            for (const c of batch) await walk(c, path + entry.name + "/");
          } while (batch.length);
        }
      };
      try { for (const en of entries) await walk(en, ""); } catch { list = plain.map((f) => ({ file: f, rel: f.name })); }
    } else list = plain.map((f) => ({ file: f, rel: f.name }));
    offer(list);
  });

  // ---------- avisos ----------
  let actx = null;
  function ping() {
    if (!document.hidden) return;
    hiddenUnread++;
    renderHeader();
    try {
      actx = actx || new AudioContext();
      const o = actx.createOscillator(), g = actx.createGain();
      o.type = "sine"; o.frequency.setValueAtTime(880, actx.currentTime); o.frequency.exponentialRampToValueAtTime(1320, actx.currentTime + 0.08);
      g.gain.setValueAtTime(0.0001, actx.currentTime); g.gain.exponentialRampToValueAtTime(0.12, actx.currentTime + 0.02); g.gain.exponentialRampToValueAtTime(0.0001, actx.currentTime + 0.35);
      o.connect(g).connect(actx.destination); o.start(); o.stop(actx.currentTime + 0.4);
    } catch {}
  }
  document.addEventListener("visibilitychange", () => { if (!document.hidden) { hiddenUnread = 0; renderHeader(); } });
  window.addEventListener("beforeunload", (e) => {
    const busy = [...ups.values()].some((u) => u.up || u.state === "wait") || [...dls.values()].some((d) => d.state === "run");
    if (busy) { e.preventDefault(); e.returnValue = ""; }
  });

  boot();
})();
