// Librería común de la página del chat: formato, CRC32, descarga turbo y subida por bloques.
window.EnvLib = (() => {
  "use strict";

  const nf1 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 1 });
  const nf0 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 0 });
  function fmtBytes(n) {
    if (!isFinite(n) || n < 0) n = 0;
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? nf0.format(n) : nf1.format(n)) + " " + u[i];
  }
  const fmtSpeed = (b) => fmtBytes(b) + "/s";
  function fmtTime(s) {
    if (!isFinite(s) || s < 0) return "—";
    s = Math.round(s);
    if (s < 60) return s + " s";
    const m = Math.floor(s / 60), r = s % 60;
    if (m < 60) return m + " min" + (r ? " " + r + " s" : "");
    return Math.floor(m / 60) + " h " + (m % 60) + " min";
  }
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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
  const crc32 = (b) => (crcUpdate(-1, b) ^ -1) >>> 0;

  const canTurbo = window.isSecureContext && "showSaveFilePicker" in window && "showDirectoryPicker" in window && !!window.ReadableStream;

  class Speedo {
    constructor() { this.s = []; }
    get(bytes) {
      const now = performance.now();
      this.s.push([now, bytes]);
      while (this.s.length > 2 && now - this.s[0][0] > 3000) this.s.shift();
      const [t0, b0] = this.s[0];
      const dt = (now - t0) / 1000;
      return dt > 0.2 ? Math.max(0, (bytes - b0) / dt) : 0;
    }
    reset() { this.s = []; }
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

  // ---------- descarga turbo ----------
  // Pocas peticiones largas en paralelo; cada bloque de 8 MB llega con su CRC32 y se
  // escribe en su sitio. Si una conexión acaba antes, roba la mitad del trabajo a otra.
  // hooks: progress(p), done(r), fail(err), phase(texto)
  class TurboJob {
    constructor(info, targets, hooks) {
      this.info = info;
      this.targets = targets;
      this.hooks = hooks || {};
      this.tid = Array.from(crypto.getRandomValues(new Uint8Array(9)), (b) => b.toString(16).padStart(2, "0")).join("");
      this.B = Math.min(info.chunk || 8 << 20, 16 << 20);
      this.conns = Math.max(1, Math.min(16, info.conns || 4));
      this.total = info.total;
      this.doneBytes = 0;
      this.verified = 0;
      this.retries = 0;
      this.speedo = new Speedo();
      this.pendingWrite = 0;
      this.dirCache = new Map();
      this.ctrls = new Set();
      this.active = new Set();
      this.mq = info.m ? "&m=" + encodeURIComponent(info.m) : "";
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
      this.started = performance.now();
      this.fatal = null;
      this.phase = "";
      this.timer = setInterval(() => this.report(), 250);
      try {
        for (const f of this.files) if (f.size === 0 && !f.closed) { const w = await this.writableFor(f); f.closed = true; await w.close(); }
        await Promise.all(this.lanes.map((lane) => this.worker(lane)));
        if (this.fatal) throw this.fatal;
        this.phase = "Guardando en disco…";
        this.report();
        await Promise.all(this.files.map((f) => f.chain));
        if (this.fatal) throw this.fatal;
        await Promise.all(this.files.filter((f) => f.w && !f.closed).map((f) => { f.closed = true; return f.w.close(); }));
        clearInterval(this.timer);
        fetch(`/fin/${encodeURIComponent(this.info.id)}?t=${this.tid}`, { method: "POST", keepalive: true }).catch(() => {});
        const secs = (performance.now() - this.started) / 1000;
        this.hooks.done && this.hooks.done({ secs, avg: this.total / Math.max(secs, 0.001), blocks: this.blocksTotal });
      } catch (e) {
        clearInterval(this.timer);
        if (this.cancelled) return;
        this.hooks.fail && this.hooks.fail(e);
      }
    }

    steal() {
      let best = null, rem = 0;
      for (const it of this.active) {
        if (it.type !== "seg") continue;
        const r = it.end - it.next - 1;
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
          const err = new Error(res.status === 410 ? "El archivo ya no está disponible en el ordenador de origen." : "Este envío ya no existe.");
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
            if (crc32(data) !== expect) throw new Error("CRC no coincide");
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
      const { reader, done } = await this.open(`/c/${encodeURIComponent(this.info.id)}/${f.i}?off=${off}&len=${len}&bs=${B}&t=${this.tid}${this.mq}`);
      try {
        await this.readBlocks(reader, it,
          () => (it.next < it.end ? this.blockLen(f, it.next) : -1),
          async (data) => { const b = it.next; it.next++; await this.write(f, b, data); });
      } finally { done(); }
    }

    async runBatch(it) {
      const list = it.files.slice(it.pos).map((f) => f.i).join(",");
      const { reader, done } = await this.open(`/m/${encodeURIComponent(this.info.id)}?f=${list}&t=${this.tid}${this.mq}`);
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
    }

    resume() {
      for (const l of this.lanes) { l.item = null; l.retry = false; }
      for (const j of this.queue) j.tries = 0;
      this.speedo.reset();
      this.run();
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

    report() {
      const done = Math.max(0, Math.min(this.doneBytes, this.total));
      this.hooks.progress && this.hooks.progress({
        done, total: this.total, speed: this.speedo.get(this.doneBytes), verified: this.verified, blocks: this.blocksTotal,
        retries: this.retries, phase: this.phase,
        lanes: this.lanes.map((l) => ({ active: !!l.item, retry: l.retry, frac: this.laneFraction(l.item) })),
      });
    }
  }

  // ---------- subida por bloques (invitado -> anfitrión) ----------
  // files: [{i, file, size}], missing: {i: [bloques]}. Cada bloque va con su CRC32.
  class Uploader {
    constructor(opts, hooks) {
      this.o = opts;
      this.hooks = hooks || {};
      this.jobs = [];
      this.total = 0;
      this.doneBytes = 0;
      for (const f of opts.files) {
        this.total += f.size;
        const miss = opts.missing[f.i];
        const all = Math.ceil(f.size / opts.chunk);
        const set = miss ? new Set(miss) : null;
        for (let c = 0; c < all; c++) {
          const len = Math.min(opts.chunk, f.size - c * opts.chunk);
          if (set && !set.has(c)) { this.doneBytes += len; continue; }
          this.jobs.push({ f, c, len, sent: 0, tries: 0 });
        }
      }
      this.inflight = new Set();
      this.speedo = new Speedo();
    }

    live() {
      let b = this.doneBytes;
      for (const j of this.inflight) b += j.sent;
      return Math.min(b, this.total);
    }

    async run() {
      this.started = performance.now();
      this.timer = setInterval(() => {
        const live = this.live();
        this.hooks.progress && this.hooks.progress({ done: live, total: this.total, speed: this.speedo.get(live) });
      }, 250);
      try {
        const n = Math.max(6, Math.min(8, this.o.conns || 6));
        await Promise.all(Array.from({ length: n }, () => this.worker()));
        clearInterval(this.timer);
        if (this.cancelled) return;
        if (this.fatal) throw this.fatal;
        this.hooks.done && this.hooks.done({ secs: (performance.now() - this.started) / 1000 });
      } catch (e) {
        clearInterval(this.timer);
        if (!this.cancelled) this.hooks.fail && this.hooks.fail(e);
      }
    }

    async worker() {
      while (!this.fatal && !this.cancelled) {
        const j = this.jobs.shift();
        if (!j) return;
        this.inflight.add(j);
        try {
          await this.send(j);
          this.doneBytes += j.len;
        } catch (e) {
          if (this.cancelled) return;
          if (e && e.fatal) { this.fatal = e; return; }
          j.tries++;
          if (j.tries > 8) { this.fatal = new Error("La conexión falla demasiado."); this.jobs.unshift(j); return; }
          this.jobs.unshift(j);
          await sleep(Math.min(8000, 400 * 2 ** (j.tries - 1)));
        } finally {
          j.sent = 0;
          this.inflight.delete(j);
        }
      }
    }

    async send(j) {
      const off = j.c * this.o.chunk;
      let buf;
      try {
        buf = new Uint8Array(await j.f.file.slice(off, off + j.len).arrayBuffer());
      } catch (e) {
        const err = new Error("No se puede leer «" + j.f.file.name + "». ¿Lo has movido o borrado?");
        err.fatal = true;
        throw err;
      }
      const crc = crc32(buf).toString(16);
      await new Promise((resolve, reject) => {
        const x = new XMLHttpRequest();
        this.xhrs = this.xhrs || new Set();
        this.xhrs.add(x);
        x.open("PUT", `/api/r/${encodeURIComponent(this.o.room)}/in/${encodeURIComponent(this.o.bid)}/${j.f.i}?off=${off}&mid=${encodeURIComponent(this.o.mid)}`);
        x.setRequestHeader("X-CRC32", crc);
        x.setRequestHeader("Content-Type", "application/octet-stream");
        x.upload.onprogress = (e) => { j.sent = e.loaded; };
        x.onload = () => {
          this.xhrs.delete(x);
          if (x.status === 204 || x.status === 200) return resolve();
          let msg = "HTTP " + x.status;
          try { msg = JSON.parse(x.responseText).error || msg; } catch {}
          const err = new Error(msg);
          if (x.status === 403 || x.status === 409 || x.status === 404) err.fatal = true;
          reject(err);
        };
        x.onerror = () => { this.xhrs.delete(x); reject(new Error("conexión cortada")); };
        x.onabort = () => { this.xhrs.delete(x); reject(new Error("cancelado")); };
        x.send(buf);
      });
    }

    cancel() {
      this.cancelled = true;
      clearInterval(this.timer);
      if (this.xhrs) for (const x of this.xhrs) x.abort();
    }
  }

  return { fmtBytes, fmtSpeed, fmtTime, esc, sleep, crc32, canTurbo, TurboJob, Uploader, nf0, nf1 };
})();
