(() => {
  "use strict";

  // ---------- utilidades ----------
  const $ = (s, el = document) => el.querySelector(s);
  const card = $("#card");
  const nf1 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 1 });
  const nf0 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 0 });

  function fmtBytes(n) {
    if (!isFinite(n) || n < 0) n = 0;
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? nf0.format(n) : nf1.format(n)) + " " + u[i];
  }
  const fmtSpeed = (bps) => fmtBytes(bps) + "/s";
  function fmtTime(s) {
    if (!isFinite(s) || s < 0) return "—";
    s = Math.round(s);
    if (s < 60) return s + " s";
    const m = Math.floor(s / 60), r = s % 60;
    if (m < 60) return m + " min " + (r ? r + " s" : "");
    const h = Math.floor(m / 60);
    return h + " h " + (m % 60) + " min";
  }
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

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
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>',
    zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
    cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>',
    home: '<path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 22V12h6v10"/>',
    lock: '<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/>',
    alert: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    folderDown: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/><path d="M12 10v6"/><path d="m15 13-3 3-3-3"/>',
    rocket: '<path d="M4.5 16.5c-1.5 1.3-2 5-2 5s3.7-.5 5-2c.7-.8.7-2.1-.1-2.9a2.2 2.2 0 0 0-2.9-.1z"/><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.9 12.9 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.4 22.4 0 0 1-4 2z"/><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0"/><path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    refresh: '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
  };
  const svg = (name, extra = "") =>
    `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" ${extra}>${I[name]}</svg>`;

  const TYPES = [
    ["image", ["jpg", "jpeg", "png", "gif", "webp", "heic", "bmp", "tif", "tiff", "svg", "raw", "cr2", "nef", "arw", "psd", "avif"], "linear-gradient(135deg,#f472b6,#a855f7)", "rgba(217,70,239,.55)"],
    ["video", ["mp4", "mkv", "mov", "avi", "webm", "wmv", "flv", "m4v", "ts", "m2ts"], "linear-gradient(135deg,#f43f5e,#f97316)", "rgba(244,63,94,.55)"],
    ["music", ["mp3", "wav", "flac", "aac", "ogg", "m4a", "opus", "wma"], "linear-gradient(135deg,#22c55e,#14b8a6)", "rgba(20,184,166,.55)"],
    ["archive", ["zip", "rar", "7z", "tar", "gz", "bz2", "xz", "zst"], "linear-gradient(135deg,#f59e0b,#ef4444)", "rgba(245,158,11,.5)"],
    ["code", ["js", "ts", "py", "go", "rs", "c", "cpp", "h", "java", "json", "html", "css", "xml", "yml", "yaml", "sh", "ps1", "sql"], "linear-gradient(135deg,#64748b,#334155)", "rgba(100,116,139,.5)"],
    ["doc", ["pdf", "doc", "docx", "txt", "md", "odt", "rtf", "xls", "xlsx", "csv", "ppt", "pptx", "epub"], "linear-gradient(135deg,#3b82f6,#06b6d4)", "rgba(59,130,246,.55)"],
    ["app", ["exe", "msi", "apk", "dmg", "appx", "msix", "bat", "jar"], "linear-gradient(135deg,#6366f1,#8b5cf6)", "rgba(99,102,241,.55)"],
    ["disc", ["iso", "img", "vhd", "vhdx", "vmdk", "bin"], "linear-gradient(135deg,#0ea5e9,#6366f1)", "rgba(14,165,233,.55)"],
  ];
  function typeOf(name) {
    const ext = (name.split(".").pop() || "").toLowerCase();
    for (const [icon, exts, bg, sh] of TYPES) if (exts.includes(ext)) return { icon, bg, sh, ext };
    return { icon: "file", bg: null, sh: null, ext: name.includes(".") ? ext : "" };
  }

  // ---------- CRC32 (slicing-by-4) ----------
  const CRC_T = (() => {
    const t = new Int32Array(1024);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      t[n] = c;
    }
    for (let n = 0; n < 256; n++) {
      let c = t[n];
      for (let k = 1; k < 4; k++) { c = t[c & 0xff] ^ (c >>> 8); t[k * 256 + n] = c; }
    }
    return t;
  })();
  function crcUpdate(crc, b) {
    const T = CRC_T, len = b.length;
    let i = 0;
    for (; i + 4 <= len; i += 4) {
      crc ^= b[i] | (b[i + 1] << 8) | (b[i + 2] << 16) | (b[i + 3] << 24);
      crc = T[768 + (crc & 0xff)] ^ T[512 + ((crc >>> 8) & 0xff)] ^ T[256 + ((crc >>> 16) & 0xff)] ^ T[crc >>> 24];
    }
    for (; i < len; i++) crc = T[(crc ^ b[i]) & 0xff] ^ (crc >>> 8);
    return crc;
  }

  // ---------- estado ----------
  const id = decodeURIComponent(location.pathname.split("/").filter(Boolean)[1] || "");
  let info = null;
  const canTurbo = window.isSecureContext && "showSaveFilePicker" in window && "showDirectoryPicker" in window && !!window.ReadableStream;

  async function load() {
    try {
      const r = await fetch("/api/s/" + encodeURIComponent(id), { cache: "no-store" });
      if (r.status === 404) return showError("Este envío ya no existe", "Puede que quien te lo mandó lo haya borrado o que el enlace esté mal copiado.", false);
      if (!r.ok) throw new Error("HTTP " + r.status);
      info = await r.json();
      document.title = info.name + " · Enviador";
      renderReady();
    } catch (e) {
      showError("No se puede conectar", "El ordenador que envía los archivos no responde. Puede que esté apagado o que haya cerrado Enviador.", true, load);
    }
  }

  function viaBadge() {
    if (info.via === "cloudflare") return `<span class="badge cf">${svg("lock")} Conexión cifrada vía Cloudflare</span>`;
    if (info.via === "lan") return `<span class="badge lan">${svg("home")} Red local · máxima velocidad</span>`;
    if (info.via === "direct") return `<span class="badge direct">${svg("zap")} Conexión directa</span>`;
    return `<span class="badge">${svg("home")} Este equipo</span>`;
  }

  function headHTML() {
    const multi = info.kind === "multi";
    const t = multi ? { icon: "folder", bg: "linear-gradient(135deg,#f59e0b,#f97316)", sh: "rgba(245,158,11,.5)", ext: "" } : typeOf(info.name);
    const style = t.bg ? `style="--tile:${t.bg};--tile-shadow:${t.sh}"` : "";
    const ext = !multi && t.ext && t.ext.length <= 5 ? `<span class="ext">${esc(t.ext)}</span>` : "";
    const count = multi ? `<span class="sep"></span><span>${nf0.format(info.files.length)} archivo${info.files.length === 1 ? "" : "s"}</span>` : "";
    return `
      <div class="head">
        <div class="ftile" ${style}>${svg(t.icon)}${ext}</div>
        <div class="eyebrow">Te han enviado</div>
        <h1 class="title">${esc(info.name)}</h1>
        <div class="meta"><b>${fmtBytes(info.total)}</b>${count}</div>
        ${viaBadge()}
      </div>`;
  }

  function fileListHTML() {
    const MAX = 300;
    const rows = info.files.slice(0, MAX).map((f, i) => {
      const parts = f.rel.split("/");
      const nm = parts.pop();
      const dir = parts.length ? `<small>${esc(parts.join("/"))}/</small>` : "";
      return `<div class="frow"><span class="ic">${svg(typeOf(nm).icon)}</span><span class="nm" title="${esc(f.rel)}">${dir}${esc(nm)}</span><span class="sz">${fmtBytes(f.size)}</span><a href="/d/${encodeURIComponent(id)}/${i}" download title="Descargar solo este archivo">${svg("download")}</a></div>`;
    }).join("");
    const more = info.files.length > MAX ? `<div class="files-more">y ${nf0.format(info.files.length - MAX)} archivos más…</div>` : "";
    return `<div class="files"><div class="files-head"><span>Contenido</span><span>${nf0.format(info.files.length)} · ${fmtBytes(info.total)}</span></div><div class="files-list">${rows}${more}</div></div>`;
  }

  function renderReady() {
    const multi = info.kind === "multi";
    const big = info.total >= 24 * 1024 * 1024;
    let actions = "";
    if (multi) {
      if (canTurbo) {
        actions += `<button class="btn primary" id="go-turbo">${svg("rocket")}<span class="lbl">Descargar todo en una carpeta<span class="sub">Turbo · ${info.conns} conexiones · integridad verificada</span></span></button>`;
        actions += `<a class="btn ghost" href="/z/${encodeURIComponent(id)}" id="go-zip">${svg("archive")} Descargar como ZIP · ${fmtBytes(info.zipSize)}</a>`;
      } else {
        actions += `<a class="btn primary" href="/z/${encodeURIComponent(id)}" id="go-zip">${svg("download")}<span class="lbl">Descargar todo (ZIP)<span class="sub">${fmtBytes(info.zipSize)}</span></span></a>`;
      }
    } else if (canTurbo && big) {
      actions += `<button class="btn primary" id="go-turbo">${svg("rocket")}<span class="lbl">Descargar<span class="sub">Turbo · ${info.conns} conexiones · integridad verificada</span></span></button>`;
      actions += `<button class="linkish" id="go-normal">Prefiero la descarga normal del navegador</button>`;
    } else {
      actions += `<a class="btn primary" href="/d/${encodeURIComponent(id)}/0" id="go-normal">${svg("download")}<span class="lbl">Descargar<span class="sub">${fmtBytes(info.total)}</span></span></a>`;
    }

    let extra = "";
    if (info.direct) {
      extra += `<div class="boost"><div class="zap">${svg("zap")}</div><p><b>¿Quieres más velocidad?</b> Hay una conexión directa al PC de origen, sin pasar por Cloudflare.</p><a href="${esc(info.direct)}" rel="noreferrer">Probar ⚡</a></div>`;
    }
    if (info.via === "direct" || info.via === "lan") {
      extra += `<p class="hint">Conexión directa sin intermediarios. Si el navegador avisa de <b>“descarga no segura”</b> es solo porque no usa HTTPS: elige <b>Conservar</b>.</p>`;
    }
    card.innerHTML = headHTML() + `<div class="actions">${actions}</div>` + extra + (multi ? fileListHTML() : "");

    const t = $("#go-turbo");
    if (t) t.addEventListener("click", () => startTurbo());
    const n = $("#go-normal");
    if (n && n.tagName === "BUTTON") n.addEventListener("click", () => normalDownload("/d/" + encodeURIComponent(id) + "/0"));
    else if (n) n.addEventListener("click", (e) => { e.preventDefault(); normalDownload(n.getAttribute("href")); });
    const z = $("#go-zip");
    if (z) z.addEventListener("click", (e) => { e.preventDefault(); normalDownload(z.getAttribute("href"), true); });
  }

  function normalDownload(href, zip) {
    const a = document.createElement("a");
    a.href = href;
    a.download = "";
    document.body.appendChild(a);
    a.click();
    a.remove();
    const back = `<button class="linkish" id="back">Volver</button>`;
    card.innerHTML = headHTML() + `
      <div class="done" style="margin-top:22px">
        <p style="color:var(--txt);font-size:15px">${svg("download", 'style="width:18px;height:18px;vertical-align:-3px;margin-right:6px;color:var(--c)"')}La descarga ha empezado en tu navegador.</p>
        <p>Puedes ver el progreso en la barra de descargas. ${zip ? "El ZIP se va generando al vuelo, sin compresión, a máxima velocidad." : "Si se corta, el navegador puede reanudarla."}</p>
      </div>
      <div class="actions">${back}</div>`;
    $("#back").addEventListener("click", renderReady);
  }

  function showError(title, msg, retry, fn) {
    card.innerHTML = `<div class="err"><div class="eic">${svg("alert")}</div><h2>${esc(title)}</h2><p>${esc(msg)}</p>${retry ? `<div class="actions" style="width:100%"><button class="btn ghost" id="retry">${svg("refresh")} Reintentar</button></div>` : ""}</div>`;
    if (retry) $("#retry").addEventListener("click", fn || (() => location.reload()));
  }

  // ---------- descarga turbo ----------
  async function startTurbo() {
    const multi = info.kind === "multi";
    let targets;
    try {
      if (multi) {
        const dir = await window.showDirectoryPicker({ id: "enviador", mode: "readwrite", startIn: "downloads" });
        targets = { dir };
      } else {
        const fh = await window.showSaveFilePicker({ id: "enviador", suggestedName: info.name, startIn: "downloads" });
        targets = { single: fh };
      }
    } catch (e) {
      if (e && e.name === "AbortError") return;
      normalDownload(multi ? "/z/" + encodeURIComponent(id) : "/d/" + encodeURIComponent(id) + "/0", multi);
      return;
    }
    const job = new TurboJob(info, targets);
    job.run();
  }

  // Pocas peticiones largas (una por "tramo") en paralelo. El servidor manda cada
  // bloque de 8 MB seguido de su CRC32; se verifica y se escribe en su sitio.
  // Si una conexión acaba antes, le quita la mitad del trabajo pendiente a otra.
  // Los archivos pequeños viajan agrupados en una sola petición.
  class TurboJob {
    constructor(info, targets) {
      this.info = info;
      this.targets = targets;
      this.tid = Array.from(crypto.getRandomValues(new Uint8Array(9)), (b) => b.toString(16).padStart(2, "0")).join("");
      this.B = Math.min(info.chunk || 8 << 20, 16 << 20);
      this.conns = Math.max(1, Math.min(16, info.conns || 4));
      this.total = info.total;
      this.doneBytes = 0;
      this.verified = 0;
      this.retries = 0;
      this.samples = [];
      this.pendingWrite = 0;
      this.dirCache = new Map();
      this.ctrls = new Set();
      this.active = new Set();
      const B = this.B;
      this.files = info.files.map((f, i) => {
        const blocks = Math.ceil(f.size / B);
        return { i, rel: f.rel, size: f.size, blocks, left: blocks, w: null, opening: null, chain: Promise.resolve(), closed: false };
      });
      this.blocksTotal = this.files.reduce((a, f) => a + f.blocks, 0);
      this.queue = [];
      let batch = null;
      for (const f of this.files) {
        if (f.size === 0) continue;
        if (f.size <= B && !targets.single) {
          if (!batch || batch.bytes + f.size > 64 << 20 || batch.files.length >= 400) {
            batch = { type: "batch", files: [], pos: 0, bytes: 0, got: 0, tries: 0 };
            this.queue.push(batch);
          }
          batch.files.push(f);
          batch.bytes += f.size;
          continue;
        }
        const per = Math.max(1, Math.min(32, Math.ceil(f.blocks / this.conns)));
        for (let b = 0; b < f.blocks; b += per) {
          this.queue.push({ type: "seg", f, start: b, next: b, end: Math.min(f.blocks, b + per), got: 0, tries: 0 });
        }
      }
      this.lanes = Array.from({ length: this.conns }, () => ({ item: null, retry: false }));
    }

    blockLen(f, b) { return Math.min(this.B, f.size - b * this.B); }

    async writableFor(f) {
      if (f.w) return f.w;
      if (f.opening) return f.opening;
      f.opening = (async () => {
        let fh;
        if (this.targets.single) fh = this.targets.single;
        else {
          const parts = f.rel.split("/").map(safeName);
          const name = parts.pop();
          let dir = this.targets.dir, key = "";
          for (const p of parts) {
            key += "/" + p;
            let d = this.dirCache.get(key);
            if (!d) { d = await dir.getDirectoryHandle(p, { create: true }); this.dirCache.set(key, d); }
            dir = d;
          }
          fh = await dir.getFileHandle(await freeName(dir, name), { create: true });
        }
        f.w = await fh.createWritable({ keepExistingData: false });
        return f.w;
      })();
      return f.opening;
    }

    async run() {
      this.renderProgress();
      this.started = performance.now();
      this.fatal = null;
      this.timer = setInterval(() => this.updateUI(), 250);
      try {
        for (const f of this.files) if (f.size === 0 && !f.closed) { const w = await this.writableFor(f); f.closed = true; await w.close(); }
        await Promise.all(this.lanes.map((lane) => this.worker(lane)));
        if (this.fatal) throw this.fatal;
        this.phase = "Guardando en disco…";
        this.updateUI();
        await Promise.all(this.files.map((f) => f.chain));
        if (this.fatal) throw this.fatal;
        await Promise.all(this.files.filter((f) => f.w && !f.closed).map((f) => { f.closed = true; return f.w.close(); }));
        clearInterval(this.timer);
        fetch(`/fin/${encodeURIComponent(this.info.id)}?t=${this.tid}`, { method: "POST", keepalive: true }).catch(() => {});
        this.renderDone();
      } catch (e) {
        clearInterval(this.timer);
        if (this.cancelled) return;
        this.renderFail(e);
      }
    }

    steal() {
      let best = null, rem = 0;
      for (const it of this.active) {
        if (it.type !== "seg") continue;
        const r = it.end - it.next - 1; // el bloque en curso se lo queda su dueño
        if (r > rem) { rem = r; best = it; }
      }
      if (!best || rem < 3) return null;
      const mid = best.end - Math.floor(rem / 2);
      const n = { type: "seg", f: best.f, start: mid, next: mid, end: best.end, got: 0, tries: 0 };
      best.end = mid;
      return n;
    }

    async worker(lane) {
      while (!this.fatal && !this.cancelled) {
        const it = this.queue.shift() || this.steal();
        if (!it) return;
        this.active.add(it);
        lane.item = it;
        let wait = 0;
        try {
          if (it.type === "seg") await this.runSeg(it);
          else await this.runBatch(it);
          lane.retry = false;
        } catch (e) {
          this.doneBytes -= it.got;
          it.got = 0;
          if (this.cancelled) return;
          this.queue.unshift(it);
          if (e && e.fatal) { this.setFatal(e); return; }
          if (this.fatal) return;
          it.tries++;
          this.retries++;
          lane.retry = true;
          if (it.tries > 8) { this.setFatal(new Error("La conexión falla demasiado. Puedes reanudar donde se quedó.")); return; }
          wait = Math.min(8000, 400 * 2 ** (it.tries - 1));
        } finally {
          this.active.delete(it);
          lane.item = null;
        }
        if (wait) await sleep(wait);
      }
    }

    async open(url) {
      const ctrl = new AbortController();
      this.ctrls.add(ctrl);
      try {
        const res = await fetch(url, { cache: "no-store", signal: ctrl.signal });
        if (res.status === 410 || res.status === 404) {
          const err = new Error(res.status === 410 ? "El archivo ya no está disponible en el ordenador de origen." : "El envío ya no existe.");
          err.fatal = true;
          throw err;
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        const reader = res.body.getReader();
        return { reader, done: () => { this.ctrls.delete(ctrl); reader.cancel().catch(() => {}); } };
      } catch (e) {
        this.ctrls.delete(ctrl);
        throw e;
      }
    }

    // Lee bloques de [len datos + 4 de CRC]; next() da la longitud del siguiente
    // bloque (o -1 para parar) y onBlock() recibe los datos ya verificados.
    async readBlocks(reader, it, next, onBlock) {
      let len = next();
      if (len < 0) return;
      let buf = new Uint8Array(len + 4), pos = 0;
      for (;;) {
        const { done, value } = await reader.read();
        if (done) throw new Error("conexión cortada");
        let p = 0;
        while (p < value.length) {
          const take = Math.min(value.length - p, buf.length - pos);
          buf.set(value.subarray(p, p + take), pos);
          const before = Math.min(pos, len);
          pos += take;
          p += take;
          const add = Math.min(pos, len) - before;
          it.got += add;
          this.doneBytes += add;
          if (pos === buf.length) {
            const data = buf.subarray(0, len);
            const expect = ((buf[len] << 24) | (buf[len + 1] << 16) | (buf[len + 2] << 8) | buf[len + 3]) >>> 0;
            if (((crcUpdate(-1, data) ^ -1) >>> 0) !== expect) throw new Error("CRC no coincide");
            it.got = 0;
            this.verified++;
            await onBlock(data);
            len = next();
            if (len < 0) return;
            buf = new Uint8Array(len + 4);
            pos = 0;
          }
        }
      }
    }

    async runSeg(it) {
      const f = it.f, B = this.B;
      const off = it.next * B;
      const len = Math.min(f.size, it.end * B) - off;
      if (len <= 0) return;
      const { reader, done } = await this.open(`/c/${encodeURIComponent(this.info.id)}/${f.i}?off=${off}&len=${len}&bs=${B}&t=${this.tid}`);
      try {
        await this.readBlocks(reader, it,
          () => (it.next < it.end ? this.blockLen(f, it.next) : -1),
          async (data) => { const b = it.next; it.next++; await this.write(f, b, data); });
      } finally { done(); }
    }

    async runBatch(it) {
      const list = it.files.slice(it.pos).map((f) => f.i).join(",");
      const { reader, done } = await this.open(`/m/${encodeURIComponent(this.info.id)}?f=${list}&t=${this.tid}`);
      try {
        await this.readBlocks(reader, it,
          () => (it.pos < it.files.length ? it.files[it.pos].size : -1),
          async (data) => { const f = it.files[it.pos]; it.pos++; await this.write(f, 0, data); });
      } finally { done(); }
    }

    async write(f, block, data) {
      const w = await this.writableFor(f);
      this.pendingWrite += data.length;
      f.chain = f.chain
        .then(() => w.write({ type: "write", position: block * this.B, data }))
        .then(() => {
          this.pendingWrite -= data.length;
          if (--f.left === 0 && !this.targets.single) { f.closed = true; return w.close(); }
        }, (e) => {
          this.pendingWrite -= data.length;
          this.verified--;
          this.doneBytes -= data.length;
          this.queue.unshift({ type: "seg", f, start: block, next: block, end: block + 1, got: 0, tries: 0 });
          this.setFatal(new Error("No se pudo escribir en el disco: " + (e && e.message ? e.message : e)));
        });
      // si el disco va más lento que la red, frenamos la descarga
      while (this.pendingWrite > 384 * 1024 * 1024 && !this.fatal && !this.cancelled) await sleep(30);
    }

    setFatal(e) {
      if (this.fatal) return;
      this.fatal = e;
      for (const c of this.ctrls) { try { c.abort(); } catch {} }
    }

    cancel() {
      this.cancelled = true;
      clearInterval(this.timer);
      for (const c of this.ctrls) { try { c.abort(); } catch {} }
      for (const f of this.files) if (f.w && !f.closed) { f.closed = true; f.w.abort().catch(() => {}); }
      renderReady();
    }

    resume() {
      for (const l of this.lanes) { l.item = null; l.retry = false; }
      for (const j of this.queue) j.tries = 0;
      this.samples = [];
      this.phase = "";
      this.run();
    }

    speed() {
      const now = performance.now();
      this.samples.push([now, this.doneBytes]);
      while (this.samples.length > 2 && now - this.samples[0][0] > 3000) this.samples.shift();
      const [t0, b0] = this.samples[0];
      const dt = (now - t0) / 1000;
      return dt > 0.2 ? Math.max(0, (this.doneBytes - b0) / dt) : 0;
    }

    laneFraction(it) {
      if (!it) return 0;
      if (it.type === "batch") {
        const cur = it.files[it.pos];
        return Math.min(1, (it.pos + (cur ? it.got / Math.max(1, cur.size) : 0)) / it.files.length);
      }
      const span = it.end - it.start;
      return span > 0 ? Math.min(1, (it.next - it.start + it.got / this.B) / span) : 1;
    }

    renderProgress() {
      const lanes = this.lanes.map(() => `<div class="lane"><i></i></div>`).join("");
      card.innerHTML = headHTML() + `
        <div class="prog">
          <div class="pct-row"><div class="pct" id="pct">0<small>%</small></div><div class="phase" id="phase">Conectando…</div></div>
          <div class="bar" id="bar"><i></i></div>
          <div class="stats">
            <div class="stat"><span>Velocidad</span><b id="st-speed">—</b></div>
            <div class="stat"><span>Restante</span><b id="st-eta">—</b></div>
            <div class="stat"><span>Descargado</span><b id="st-done">0 B</b></div>
          </div>
          <div class="lanes"><span class="lbl">${this.conns} conexion${this.conns === 1 ? "" : "es"}</span>${lanes}</div>
          <div class="verify" id="verify">${svg("shield")}<span>Verificando cada bloque con CRC32</span></div>
        </div>
        <div class="actions"><button class="btn ghost small" id="cancel">${svg("x")} Cancelar</button></div>`;
      $("#cancel").addEventListener("click", () => this.cancel());
      this.el = {
        pct: $("#pct"), phase: $("#phase"), bar: $("#bar i"), speed: $("#st-speed"), eta: $("#st-eta"),
        done: $("#st-done"), verify: $("#verify span"), lanes: [...card.querySelectorAll(".lane")],
      };
    }

    updateUI() {
      if (!this.el) return;
      const done = Math.max(0, Math.min(this.doneBytes, this.total));
      const p = this.total ? (done / this.total) * 100 : 100;
      const sp = this.speed();
      this.el.pct.innerHTML = (p >= 99.95 && p < 100 ? "99,9" : nf1.format(Math.floor(p * 10) / 10)) + "<small>%</small>";
      this.el.bar.style.width = p.toFixed(2) + "%";
      this.el.speed.textContent = sp > 0 ? fmtSpeed(sp) : "—";
      this.el.eta.textContent = sp > 0 ? fmtTime((this.total - done) / sp) : "—";
      this.el.done.textContent = fmtBytes(done);
      this.el.phase.innerHTML = this.phase ? esc(this.phase) : (sp > 0 ? `de <b>${fmtBytes(this.total)}</b>` : "Conectando…");
      this.el.verify.textContent = `${nf0.format(this.verified)} de ${nf0.format(this.blocksTotal)} bloques verificados` + (this.retries ? ` · ${this.retries} reintento${this.retries === 1 ? "" : "s"}` : "");
      this.lanes.forEach((l, k) => {
        const el = this.el.lanes[k];
        if (!el) return;
        el.className = "lane" + (l.item ? "" : " idle") + (l.retry ? " retry" : "");
        el.firstChild.style.width = Math.round(this.laneFraction(l.item) * 100) + "%";
      });
      document.title = `${Math.floor(p)}% · ${this.info.name}`;
    }

    renderDone() {
      const secs = (performance.now() - this.started) / 1000;
      const avg = this.total / Math.max(secs, 0.001);
      document.title = "✓ " + this.info.name;
      card.innerHTML = `
        <div class="done">
          <div class="check"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg></div>
          <h2>¡Descarga completada!</h2>
          <p>${esc(this.info.name)} · ${fmtBytes(this.total)}</p>
          <p style="font-size:13px">Todo ha llegado íntegro: ${nf0.format(this.blocksTotal)} bloque${this.blocksTotal === 1 ? "" : "s"} verificado${this.blocksTotal === 1 ? "" : "s"}.</p>
          <div class="summary">
            <div class="stat"><span>Tiempo</span><b>${fmtTime(secs)}</b></div>
            <div class="stat"><span>Media</span><b>${fmtSpeed(avg)}</b></div>
          </div>
        </div>
        <div class="actions"><button class="linkish" id="again">Volver al envío</button></div>`;
      $("#again").addEventListener("click", renderReady);
      confetti();
    }

    renderFail(e) {
      const msg = e && e.message ? e.message : String(e);
      const left = this.total - Math.max(0, Math.min(this.doneBytes, this.total));
      card.innerHTML = headHTML() + `
        <div class="err" style="margin-top:20px">
          <div class="eic" style="width:56px;height:56px;border-radius:16px">${svg("alert")}</div>
          <h2>Descarga en pausa</h2>
          <p>${esc(msg)}</p>
          <p style="font-size:13px">Lo ya descargado se conserva: faltan ${fmtBytes(left)}.</p>
        </div>
        <div class="actions"><button class="btn primary" id="resume">${svg("refresh")} Reanudar</button><button class="linkish" id="cancel2">Cancelar descarga</button></div>`;
      $("#resume").addEventListener("click", () => this.resume());
      $("#cancel2").addEventListener("click", () => this.cancel());
    }
  }

  function safeName(s) {
    s = s.replace(/[<>:"\\|?*\x00-\x1f]/g, "_").replace(/[. ]+$/, "");
    return s || "_";
  }
  async function freeName(dir, name) {
    const dot = name.lastIndexOf(".");
    const base = dot > 0 ? name.slice(0, dot) : name, ext = dot > 0 ? name.slice(dot) : "";
    for (let n = 1; n < 1000; n++) {
      const cand = n === 1 ? name : `${base} (${n})${ext}`;
      try { await dir.getFileHandle(cand); } catch { return cand; }
    }
    return name;
  }
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  function confetti() {
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const c = document.createElement("canvas");
    c.className = "confetti";
    document.body.appendChild(c);
    const ctx = c.getContext("2d");
    const dpr = devicePixelRatio || 1;
    c.width = innerWidth * dpr; c.height = innerHeight * dpr;
    ctx.scale(dpr, dpr);
    const cols = ["#8b5cf6", "#6366f1", "#22d3ee", "#34d399", "#fbbf24", "#f472b6"];
    const ps = Array.from({ length: 140 }, () => ({
      x: innerWidth / 2 + (Math.random() - .5) * 120, y: innerHeight * .38,
      vx: (Math.random() - .5) * 13, vy: -Math.random() * 13 - 4,
      r: Math.random() * 6 + 3, c: cols[(Math.random() * cols.length) | 0],
      a: Math.random() * Math.PI, va: (Math.random() - .5) * .3,
    }));
    let t = 0;
    (function frame() {
      ctx.clearRect(0, 0, innerWidth, innerHeight);
      for (const p of ps) {
        p.vy += .32; p.vx *= .99; p.x += p.vx; p.y += p.vy; p.a += p.va;
        ctx.save(); ctx.translate(p.x, p.y); ctx.rotate(p.a);
        ctx.fillStyle = p.c; ctx.globalAlpha = Math.max(0, 1 - t / 140);
        ctx.fillRect(-p.r / 2, -p.r / 4, p.r, p.r / 2);
        ctx.restore();
      }
      if (++t < 150) requestAnimationFrame(frame); else c.remove();
    })();
  }

  // Gancho para pruebas automáticas: /s/<id>#test expone el descargador.
  if (location.hash === "#test") window.__turbo = (targets, conns) => { const j = new TurboJob(Object.assign({}, info, conns ? { conns } : {}), targets); j.run(); return j; };

  load();
})();
