"use strict";
// TurboCAM en el móvil: coge la cámara, dibuja cada fotograma con los retoques
// (encuadre, zoom, brillo, espejo…) al tamaño que pide el PC y lo manda como JPEG.
// El trabajo caro se hace aquí para que el PC casi solo tenga que copiar.
(() => {
  const $ = (s) => document.querySelector(s);
  const app = $("#app"), video = $("#video");

  // ---------- identidad del móvil ----------
  const store = {
    get(k) { try { return localStorage.getItem("turbocam." + k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem("turbocam." + k, v); } catch { /* modo privado */ } },
  };
  function newID() {
    const a = new Uint8Array(12);
    crypto.getRandomValues(a);
    return Array.from(a, (b) => "abcdefghijklmnopqrstuvwxyz0123456789"[b % 36]).join("");
  }
  const deviceID = store.get("id") || newID();
  store.set("id", deviceID);

  function guessModel() {
    const ua = navigator.userAgent;
    if (/iPhone/.test(ua)) return "iPhone";
    if (/iPad/.test(ua)) return "iPad";
    const m = ua.match(/Android [\d.]+; ([^;)]+?)(?: Build|\))/);
    if (m && m[1] && m[1] !== "K") return m[1].trim();
    if (/Android/.test(ua)) return "Android";
    return "Móvil";
  }
  let model = guessModel();
  // Chrome para Android ya no pone el modelo en el agente: se pide aparte.
  if (navigator.userAgentData && navigator.userAgentData.getHighEntropyValues) {
    navigator.userAgentData.getHighEntropyValues(["model"]).then((v) => {
      if (v.model) { model = v.model; if (S.connected) sendState(true); }
    }).catch(() => {});
  }

  // ---------- estado ----------
  const DEFAULT_ADJUST = { facing: "user", lens: "", zoom: 1, panX: 0, panY: 0, brightness: 1, contrast: 1, saturation: 1, mirror: false, rotate: 0, fit: "cover", torch: false };
  const S = {
    cfg: { role: "spare", w: 320, h: 180, fps: 2, adjust: { ...DEFAULT_ADJUST }, pc: "", name: "", frozen: false, covered: false, watched: false },
    stream: null, track: null, caps: {}, cameras: [],
    connected: false, transport: null, started: false, starting: false,
    seq: 0, inflight: 0, lastSent: 0, q: 0.72, sent: new Map(), rtt: 0, rttN: 0,
    skipped: 0, acked: 0, frames: 0, fpsNow: 0, bytes: 0, kbps: 0,
    black: false, wake: null, stopped: false, bye: "", filterOK: false,
    hidden: document.visibilityState === "hidden",
  };

  // ---------- conexión ----------
  const base = location.pathname.endsWith("/") ? location.pathname : location.pathname + "/";
  let wsFails = Number(store.get("wsFails") || 0);
  let retryDelay = 1000, retryTimer = 0;

  function connect() {
    clearTimeout(retryTimer);
    if (S.bye) return;
    if (S.transport) S.transport.close();
    S.transport = (wsFails >= 2 || !("WebSocket" in window)) ? httpTransport() : wsTransport();
  }

  function scheduleReconnect() {
    clearTimeout(retryTimer);
    if (S.bye) return;
    retryTimer = setTimeout(connect, retryDelay);
    retryDelay = Math.min(retryDelay * 1.6, 8000);
  }

  function onOpen() {
    S.connected = true;
    if (!S.bye) $("#notice").hidden = true; // quita «TurboCAM se ha cerrado»
    retryDelay = 1000;
    S.inflight = 0;
    S.sent.clear();
    sendState(true);
    render();
  }

  function onClosed() {
    const was = S.connected;
    S.connected = false;
    S.inflight = 0;
    if (was) render();
    scheduleReconnect();
  }

  function wsTransport() {
    const ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + base + "ws");
    ws.binaryType = "arraybuffer";
    let opened = false;
    const t = {
      kind: "ws",
      send(obj) { if (ws.readyState === 1) ws.send(JSON.stringify(obj)); },
      sendBlob(b) { if (ws.readyState === 1) ws.send(b); },
      backlog() { return ws.bufferedAmount; },
      close() { ws.onclose = null; try { ws.close(); } catch { } },
    };
    ws.onopen = () => {
      opened = true;
      if (wsFails) { wsFails = 0; store.set("wsFails", "0"); }
      ws.send(JSON.stringify({ t: "hello", id: deviceID, name: S.cfg.name || model, model }));
      onOpen();
    };
    ws.onmessage = (ev) => { if (typeof ev.data === "string") onMessage(ev.data); };
    ws.onclose = () => {
      // Safari no abre el WebSocket contra un certificado propio aunque la página
      // se haya aceptado: tras dos fallos seguidos se pasa a HTTP.
      if (!opened && location.protocol === "https:") { wsFails++; store.set("wsFails", String(wsFails)); }
      if (S.transport === t) onClosed();
    };
    return t;
  }

  function httpTransport() {
    const q = new URLSearchParams({ id: deviceID, name: S.cfg.name || model, model });
    const es = new EventSource(base + "ev?" + q);
    let conn = "", busy = 0;
    const post = (body, type) => {
      if (!conn) return;
      busy += body.size || body.length || 0;
      fetch(base + "x?c=" + conn, { method: "POST", body, headers: { "Content-Type": type }, cache: "no-store" })
        .then((r) => { if (r.status === 410) t.close(true); })
        .catch(() => {})
        .finally(() => { busy -= body.size || body.length || 0; });
    };
    const t = {
      kind: "http",
      send(obj) { post(JSON.stringify(obj), "application/json"); },
      sendBlob(b) { post(b, "application/octet-stream"); },
      backlog() { return busy; },
      close(reconnect) {
        es.close();
        if (reconnect && S.transport === t) onClosed();
      },
    };
    es.onmessage = (ev) => {
      if (!conn) {
        try { const m = JSON.parse(ev.data); if (m.t === "conn") { conn = m.c; onOpen(); return; } } catch { }
      }
      onMessage(ev.data);
    };
    es.onerror = () => { if (es.readyState === 2 && S.transport === t) onClosed(); };
    return t;
  }

  function send(obj) { if (S.connected && S.transport) S.transport.send(obj); }

  function onMessage(text) {
    let m;
    try { m = JSON.parse(text); } catch { return; }
    switch (m.t) {
      case "config": applyConfig(m); break;
      case "ack": onAck(m); break;
      case "pong": if (m.ts) addRTT(performance.now() - m.ts); break;
      case "photo": takePhoto(); break;
      case "bye": onBye(m.reason); break;
    }
  }

  function onBye(reason) {
    if (reason === "closed") {
      notice("💤", "TurboCAM se ha cerrado en el PC", "En cuanto vuelva a abrirse, la imagen seguirá sola.", null);
      return; // se reintenta solo
    }
    S.bye = reason || "bye";
    if (S.transport) S.transport.close();
    S.connected = false;
    stopCamera();
    const txt = {
      replaced: ["📱", "Abierto en otra pestaña", "Este móvil se ha conectado desde otra pestaña o ventana."],
      kicked: ["👋", "Desconectado desde el PC", "Puedes volver a conectarte cuando quieras."],
      full: ["🚦", "Hay demasiados móviles", "Desconecta alguno en el PC y vuelve a probar."],
    }[reason] || ["⚠️", "Se ha cortado la conexión", "Vuelve a intentarlo."];
    notice(txt[0], txt[1], txt[2], "Volver a conectar", () => location.reload());
  }

  // ---------- configuración que manda el PC ----------
  function applyConfig(c) {
    const prev = S.cfg;
    S.cfg = { ...prev, ...c, adjust: { ...DEFAULT_ADJUST, ...(c.adjust || {}) } };
    const a = S.cfg.adjust, pa = prev.adjust;
    if (S.started && (a.facing !== pa.facing || a.lens !== pa.lens)) startCamera();
    else if (S.track) applyTrackControls();
    syncSheet();
    render();
  }

  // ---------- cámara ----------
  async function startCamera() {
    if (S.starting) return;
    S.starting = true;
    const a = S.cfg.adjust;
    stopCamera(true);
    const video0 = { width: { ideal: 1920 }, height: { ideal: 1080 }, frameRate: { ideal: 30, max: 30 } };
    const tries = [];
    if (a.lens) tries.push({ ...video0, deviceId: { exact: a.lens } });
    tries.push({ ...video0, facingMode: a.facing });
    tries.push({ facingMode: a.facing });
    tries.push(true);
    let err = null;
    for (const v of tries) {
      try {
        S.stream = await navigator.mediaDevices.getUserMedia({ audio: false, video: v });
        err = null;
        break;
      } catch (e) {
        err = e;
        if (e && (e.name === "NotAllowedError" || e.name === "SecurityError")) break;
      }
    }
    S.starting = false;
    if (err || !S.stream) { cameraError(err); return; }
    S.track = S.stream.getVideoTracks()[0];
    S.track.addEventListener("ended", () => { if (!S.stopped && !S.hidden) startCamera(); });
    S.track.addEventListener("mute", () => sendState(true));
    S.track.addEventListener("unmute", () => sendState(true));
    video.srcObject = S.stream;
    try { await video.play(); } catch { }
    S.started = true;
    S.stopped = false;
    $("#start-err").hidden = true;
    app.dataset.screen = "live";
    readCaps();
    await listCameras();
    applyTrackControls();
    keepAwake();
    pump();
    sendState(true);
    render();
  }

  function stopCamera(keepScreen) {
    if (S.stream) S.stream.getTracks().forEach((t) => t.stop());
    S.stream = null;
    S.track = null;
    video.srcObject = null;
    S.started = false;
    if (!keepScreen) app.dataset.screen = "start";
  }

  function cameraError(e) {
    const name = e && e.name;
    let msg = "No se pudo abrir la cámara.";
    if (!window.isSecureContext) msg = "El navegador solo deja usar la cámara en páginas seguras (https). Abre el enlace del QR tal cual.";
    else if (name === "NotAllowedError") msg = "Has denegado el permiso de la cámara. Actívalo en los ajustes del navegador para esta página (el candado junto a la dirección) y vuelve a pulsar Empezar.";
    else if (name === "NotReadableError") msg = "Otra app está usando la cámara. Ciérrala y vuelve a probar.";
    else if (name === "NotFoundError" || name === "OverconstrainedError") msg = "No se encuentra esa cámara. Prueba con la otra.";
    const el = $("#start-err");
    el.textContent = msg;
    el.hidden = false;
    app.dataset.screen = "start";
    send({ t: "state", live: false, error: msg });
  }

  function readCaps() {
    let c = {};
    try { c = S.track.getCapabilities ? S.track.getCapabilities() : {}; } catch { }
    S.caps = {
      zoom: c.zoom ? { min: c.zoom.min, max: c.zoom.max, step: c.zoom.step || 0.1 } : null,
      torch: !!c.torch,
    };
  }

  async function listCameras() {
    try {
      const all = await navigator.mediaDevices.enumerateDevices();
      S.cameras = all.filter((d) => d.kind === "videoinput" && d.deviceId).map((d, i) => ({ id: d.deviceId, label: d.label || "Cámara " + (i + 1) }));
    } catch { S.cameras = []; }
  }

  function nativeZoom() { return S.caps.zoom && S.caps.zoom.max > S.caps.zoom.min; }

  function applyTrackControls() {
    if (!S.track || !S.track.applyConstraints) return;
    const a = S.cfg.adjust, adv = {};
    if (nativeZoom()) adv.zoom = Math.min(S.caps.zoom.max, Math.max(S.caps.zoom.min, a.zoom));
    if (S.caps.torch) adv.torch = !!a.torch;
    if (Object.keys(adv).length) S.track.applyConstraints({ advanced: [adv] }).catch(() => {});
  }

  // ---------- dibujo y envío ----------
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d", { alpha: false, desynchronized: true });
  S.filterOK = (() => { try { ctx.filter = "blur(1px)"; const ok = ctx.filter === "blur(1px)"; ctx.filter = "none"; return ok; } catch { return false; } })();

  // draw pinta el vídeo en el lienzo de w x h con los retoques.
  function draw(cv, cx, w, h, full) {
    const vw = video.videoWidth, vh = video.videoHeight;
    if (!vw || !vh) return false;
    if (cv.width !== w || cv.height !== h) { cv.width = w; cv.height = h; }
    const a = S.cfg.adjust;
    const rot = ((a.rotate || 0) % 360 + 360) % 360;
    const rw = rot % 180 ? vh : vw, rh = rot % 180 ? vw : vh;
    const digital = nativeZoom() ? 1 : a.zoom;
    const fit = a.fit === "contain" ? Math.min(w / rw, h / rh) : Math.max(w / rw, h / rh);
    const s = fit * digital;
    const maxX = Math.max(0, (rw * s - w) / 2), maxY = Math.max(0, (rh * s - h) / 2);
    cx.setTransform(1, 0, 0, 1, 0, 0);
    if (a.fit === "contain" || s * rw < w || s * rh < h) { cx.fillStyle = "#000"; cx.fillRect(0, 0, w, h); }
    if (S.filterOK) cx.filter = (a.brightness !== 1 || a.contrast !== 1 || a.saturation !== 1)
      ? `brightness(${a.brightness}) contrast(${a.contrast}) saturate(${a.saturation})` : "none";
    cx.translate(w / 2 - (a.panX || 0) * maxX, h / 2 - (a.panY || 0) * maxY);
    if (a.mirror) cx.scale(-1, 1);
    cx.rotate(rot * Math.PI / 180);
    cx.scale(s, s);
    cx.imageSmoothingQuality = full ? "high" : "medium";
    cx.drawImage(video, -vw / 2, -vh / 2);
    cx.setTransform(1, 0, 0, 1, 0, 0);
    return true;
  }

  function header(kind, n) {
    const b = new Uint8Array(5);
    b[0] = kind;
    new DataView(b.buffer).setUint32(1, n, true);
    return b;
  }

  // pump decide si toca mandar un fotograma. Se llama con cada fotograma nuevo del
  // vídeo (requestVideoFrameCallback) o, si no existe, con un temporizador.
  let pumpQueued = false;
  function pump() {
    if (pumpQueued || !S.started) return;
    pumpQueued = true;
    if (video.requestVideoFrameCallback) video.requestVideoFrameCallback(tick);
    else setTimeout(tick, 1000 / 30);
  }

  function tick() {
    pumpQueued = false;
    if (!S.started) return;
    const now = performance.now();
    const fps = Math.max(1, S.cfg.fps || 1);
    const due = now - S.lastSent >= 1000 / fps - 4;
    if (due && S.connected && !S.hidden && !S.encoding) {
      // Como mucho dos fotogramas sin confirmar: así nunca se acumula retraso.
      const backlog = S.transport ? S.transport.backlog() : 0;
      if (S.inflight >= 2 || backlog > 1.5e6) S.skipped++;
      else sendFrame(now);
    }
    pump();
  }

  function sendFrame(now) {
    if (!draw(canvas, ctx, S.cfg.w, S.cfg.h, false)) return;
    S.encoding = true;
    S.lastSent = now;
    const n = (S.seq = (S.seq + 1) >>> 0);
    canvas.toBlob((blob) => {
      S.encoding = false;
      if (!blob || !S.connected) return;
      S.inflight++;
      S.sent.set(n, performance.now());
      S.bytes += blob.size;
      S.transport.sendBlob(new Blob([header(1, n), blob]));
    }, "image/jpeg", S.q);
  }

  function onAck(m) {
    S.inflight = Math.max(0, S.inflight - 1);
    const t0 = S.sent.get(m.n);
    if (t0 !== undefined) { S.sent.delete(m.n); addRTT(performance.now() - t0); }
    if (S.sent.size > 30) S.sent.clear();
    S.acked++;
    S.frames++;
  }

  function addRTT(ms) {
    S.rtt = S.rttN ? S.rtt * 0.8 + ms * 0.2 : ms;
    S.rttN++;
  }

  // Calidad adaptable: baja si los fotogramas no llegan a tiempo, sube si sobra.
  setInterval(() => {
    S.fpsNow = S.frames; S.frames = 0;
    S.kbps = Math.round(S.bytes * 8 / 1000); S.bytes = 0;
    if (!S.connected || !S.started) return;
    const budget = 1000 / Math.max(1, S.cfg.fps);
    const behind = S.skipped > S.cfg.fps * 0.15 || S.rtt > budget * 3;
    if (S.cfg.fps > 4) {
      if (behind) S.q = Math.max(0.35, S.q - 0.06);
      else if (S.rtt < budget * 1.6) S.q = Math.min(0.88, S.q + 0.02);
    }
    S.skipped = 0;
    render();
  }, 1000);

  // Señal de vida y estado cada 4 s (también mide la latencia).
  setInterval(() => {
    if (!S.connected) return;
    send({ t: "ping", ts: performance.now() });
    sendState(false);
  }, 4000);

  let battery = null;
  if (navigator.getBattery) navigator.getBattery().then((b) => {
    battery = b;
    const up = () => sendState(true);
    b.addEventListener("levelchange", up);
    b.addEventListener("chargingchange", up);
  }).catch(() => {});

  let lastState = "";
  function sendState(force) {
    const st = {
      t: "state",
      visible: !S.hidden,
      live: !!(S.started && S.track && S.track.readyState === "live" && !S.track.muted),
      q: Math.round(S.q * 100) / 100,
      rtt: Math.round(S.rtt),
      caps: { ...S.caps, cameras: S.cameras, filters: S.filterOK, model },
    };
    if (battery) { st.battery = Math.round(battery.level * 100) / 100; st.charging = battery.charging; }
    const key = JSON.stringify(st);
    if (!force && key === lastState) { send({ t: "state", q: st.q, rtt: st.rtt }); return; }
    lastState = key;
    send(st);
  }

  // ---------- foto ----------
  // A la resolución del vídeo, con el mismo encuadre que lo que se emite.
  function takePhoto() {
    if (!S.started || !video.videoWidth) return;
    const c = document.createElement("canvas"), cx = c.getContext("2d", { alpha: false });
    const a = S.cfg.adjust, rot = (a.rotate || 0) % 180;
    let w = rot ? video.videoHeight : video.videoWidth;
    let h = Math.round(w * S.cfg.h / S.cfg.w);
    const vh = rot ? video.videoWidth : video.videoHeight;
    if (h > vh) { h = vh; w = Math.round(h * S.cfg.w / S.cfg.h); }
    if (!draw(c, cx, w & ~1, h & ~1, true)) return;
    c.toBlob((blob) => {
      if (blob && S.transport) S.transport.sendBlob(new Blob([header(2, 0), blob]));
      flash();
    }, "image/jpeg", 0.93);
  }

  function flash() {
    const f = document.createElement("div");
    f.style.cssText = "position:absolute;inset:0;background:#fff;z-index:60;opacity:.85;transition:opacity .35s";
    app.appendChild(f);
    requestAnimationFrame(() => { f.style.opacity = "0"; });
    setTimeout(() => f.remove(), 400);
    toast("📸 Foto guardada en el PC");
  }

  // ---------- pantalla ----------
  async function keepAwake() {
    if (!("wakeLock" in navigator) || S.hidden || !S.started) return;
    try {
      if (S.wake && !S.wake.released) return;
      S.wake = await navigator.wakeLock.request("screen");
    } catch { /* sin batería o no permitido: el modo negro sigue ayudando */ }
  }

  document.addEventListener("visibilitychange", () => {
    S.hidden = document.visibilityState === "hidden";
    if (S.hidden) {
      sendState(true);
      return;
    }
    // Al volver: la cámara puede haberse parado (Android la corta en segundo plano).
    if (S.started && (!S.track || S.track.readyState !== "live")) startCamera();
    else if (S.started) { video.play().catch(() => {}); pump(); }
    keepAwake();
    if (!S.connected && !S.bye) connect();
    sendState(true);
    render();
  });

  window.addEventListener("pageshow", (e) => { if (e.persisted && !S.connected) connect(); });
  if (navigator.mediaDevices && navigator.mediaDevices.addEventListener)
    navigator.mediaDevices.addEventListener("devicechange", () => listCameras().then(() => sendState(true)));

  function fullscreen(on) {
    const d = document;
    if (on && !d.fullscreenElement && d.documentElement.requestFullscreen) d.documentElement.requestFullscreen({ navigationUI: "hide" }).catch(() => {});
    if (!on && d.fullscreenElement) d.exitFullscreen().catch(() => {});
  }

  function setBlack(on) {
    S.black = on;
    $("#black").hidden = !on;
    const hint = $("#black-hint");
    hint.classList.remove("gone");
    if (on) {
      setTimeout(() => hint.classList.add("gone"), 3500);
      fullscreen(true);
    }
    render();
  }

  let lastTap = 0;
  $("#black").addEventListener("pointerdown", () => {
    const now = Date.now();
    if (now - lastTap < 350) setBlack(false);
    else { $("#black-hint").classList.remove("gone"); setTimeout(() => $("#black-hint").classList.add("gone"), 2500); }
    lastTap = now;
  });

  // Los controles se esconden solos para no tapar la imagen.
  let calmTimer = 0;
  function wake() {
    app.classList.remove("calm");
    clearTimeout(calmTimer);
    calmTimer = setTimeout(() => { if ($("#sheet").hidden) app.classList.add("calm"); }, 5000);
  }
  $("#stage").addEventListener("pointerdown", wake);

  // ---------- controles ----------
  function adjust(patch) {
    S.cfg.adjust = { ...S.cfg.adjust, ...patch };
    send({ t: "adjust", adjust: S.cfg.adjust });
    if ("facing" in patch || "lens" in patch) startCamera();
    else applyTrackControls();
    syncSheet();
    render();
  }

  $("#go").addEventListener("click", () => {
    S.bye = "";
    fullscreen(true);
    startCamera();
  });
  $("#b-flip").addEventListener("click", () => {
    wake();
    if (S.cameras.length > 2 && S.cfg.adjust.lens) {
      // Con varias lentes, va pasando por todas.
      const i = S.cameras.findIndex((c) => c.id === S.cfg.adjust.lens);
      adjust({ lens: S.cameras[(i + 1) % S.cameras.length].id });
      return;
    }
    const facing = S.cfg.adjust.facing === "user" ? "environment" : "user";
    adjust({ facing, lens: "", mirror: facing === "user" ? S.cfg.adjust.mirror : false, zoom: 1 });
  });
  $("#b-torch").addEventListener("click", () => { wake(); adjust({ torch: !S.cfg.adjust.torch }); });
  $("#b-black").addEventListener("click", () => setBlack(true));
  $("#b-full").addEventListener("click", () => { wake(); fullscreen(!document.fullscreenElement); });
  $("#b-tune").addEventListener("click", () => openSheet(true));
  $("#sheet-bg").addEventListener("click", () => openSheet(false));
  $("#b-stop").addEventListener("click", () => {
    openSheet(false);
    S.stopped = true;
    stopCamera();
    sendState(true);
    render();
  });

  function openSheet(on) {
    $("#sheet").hidden = !on;
    $("#sheet-bg").hidden = !on;
    if (on) { app.classList.remove("calm"); syncSheet(); }
    else wake();
  }

  const zr = $("#zoom-range");
  zr.addEventListener("input", () => {
    wake();
    S.cfg.adjust.zoom = Number(zr.value);
    applyTrackControls();
    render();
  });
  zr.addEventListener("change", () => adjust({ zoom: Number(zr.value) }));

  for (const k of ["brightness", "contrast", "saturation"]) {
    const el = $("#a-" + k);
    el.addEventListener("input", () => { S.cfg.adjust[k] = Number(el.value); syncSheet(); render(); });
    el.addEventListener("change", () => adjust({ [k]: Number(el.value) }));
  }
  $("#a-mirror").addEventListener("click", () => adjust({ mirror: !S.cfg.adjust.mirror }));
  $("#a-rotate").addEventListener("click", (e) => { const b = e.target.closest("button"); if (b) adjust({ rotate: Number(b.dataset.v) }); });
  $("#a-fit").addEventListener("click", (e) => { const b = e.target.closest("button"); if (b) adjust({ fit: b.dataset.v }); });
  $("#a-lens").addEventListener("change", (e) => adjust({ lens: e.target.value }));
  $("#a-reset").addEventListener("click", () => adjust({ ...DEFAULT_ADJUST, facing: S.cfg.adjust.facing, lens: S.cfg.adjust.lens }));

  // Instalar como app (solo con certificado de verdad: el enlace de Cloudflare).
  let installPrompt = null;
  window.addEventListener("beforeinstallprompt", (e) => { e.preventDefault(); installPrompt = e; $("#b-install").hidden = false; });
  $("#b-install").addEventListener("click", async () => {
    if (!installPrompt) return;
    installPrompt.prompt();
    await installPrompt.userChoice.catch(() => {});
    installPrompt = null;
    $("#b-install").hidden = true;
  });
  if ("serviceWorker" in navigator && window.isSecureContext) {
    navigator.serviceWorker.register("sw.js", { scope: base }).catch(() => { /* certificado propio: sin app instalable */ });
  }

  // ---------- pintar ----------
  const fmt1 = (v) => v.toFixed(1).replace(".", ",");
  const pct = (v) => Math.round(v * 100) + "%";

  function syncSheet() {
    const a = S.cfg.adjust;
    for (const k of ["brightness", "contrast", "saturation"]) {
      const el = $("#a-" + k);
      if (document.activeElement !== el) el.value = a[k];
      $("#v-" + k).textContent = pct(a[k]);
    }
    $("#filter-note").hidden = S.filterOK;
    $("#a-mirror").classList.toggle("on", !!a.mirror);
    $("#a-mirror").setAttribute("aria-checked", String(!!a.mirror));
    for (const b of $("#a-rotate").children) b.classList.toggle("on", Number(b.dataset.v) === a.rotate);
    for (const b of $("#a-fit").children) b.classList.toggle("on", b.dataset.v === a.fit);
    const sel = $("#a-lens");
    $("#lens-row").hidden = S.cameras.length < 2;
    const opts = [`<option value="">Automática (${a.facing === "user" ? "delantera" : "trasera"})</option>`]
      .concat(S.cameras.map((c) => `<option value="${esc(c.id)}">${esc(c.label)}</option>`)).join("");
    if (sel.dataset.k !== opts) { sel.innerHTML = opts; sel.dataset.k = opts; }
    sel.value = a.lens || "";
    const vw = video.videoWidth, vh = video.videoHeight;
    $("#info").innerHTML = [
      ["Conectado a", S.cfg.pc || "—"],
      ["Conexión", S.connected ? (S.transport.kind === "ws" ? "WebSocket" : "HTTP") + (location.hostname.endsWith("trycloudflare.com") ? " · Cloudflare" : " · directa") : "sin conexión"],
      ["Cámara", vw ? `${vw}×${vh}` : "—"],
      ["Se envía", `${S.cfg.w}×${S.cfg.h} · ${S.fpsNow} fps · ${S.kbps} kb/s`],
      ["Calidad", pct(S.q) + (S.rttN ? ` · ${Math.round(S.rtt)} ms` : "")],
    ].map(([k, v]) => `<span>${k}</span><b>${esc(v)}</b>`).join("");
  }

  function esc(s) { return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])); }

  function render() {
    const c = S.cfg, a = c.adjust;
    // Vista previa: el vídeo con los mismos retoques que lo que se envía.
    const z = nativeZoom() ? 1 : a.zoom;
    video.style.transform = `rotate(${a.rotate}deg) scale(${a.mirror ? -z : z}, ${z})`;
    video.style.filter = S.filterOK ? `brightness(${a.brightness}) contrast(${a.contrast}) saturate(${a.saturation})` : "";
    video.style.objectFit = "contain";
    placeFrame();

    // Estado
    const st = $("#status"), tx = $("#status-text");
    st.className = "status";
    let text;
    if (!S.connected) { st.classList.add("bad"); text = S.bye ? "Desconectado" : "Buscando el PC…"; }
    else if (S.stopped || !S.started) { text = "Conectado · sin emitir"; }
    else if (c.covered) { st.classList.add("idle"); text = "Tapada desde el PC"; }
    else if (c.frozen) { st.classList.add("idle"); text = "Imagen congelada en el PC"; }
    else if (c.role === "spare") { st.classList.add("idle"); text = "En espera en " + (c.pc || "el PC"); }
    else if (c.watched) { st.classList.add("live"); text = "En directo en " + (c.pc || "el PC"); }
    else if (c.fps <= 2) { st.classList.add("idle"); text = "En reposo: nadie está mirando"; }
    else { st.classList.add("ok"); text = "Conectado a " + (c.pc || "el PC"); }
    tx.textContent = text;
    const sc = $("#start-conn");
    sc.className = "conn " + (S.connected ? "ok" : "");
    sc.lastElementChild.textContent = S.connected ? "Conectado a " + (c.pc || "el PC") : "Conectando con el PC…";

    const res = $("#chip-res");
    res.hidden = !S.connected || !S.started || c.role === "spare";
    res.textContent = `${c.h}p · ${S.fpsNow} fps`;
    const mode = $("#chip-mode");
    mode.hidden = !(S.connected && S.started && S.q < 0.5 && c.fps > 4);
    mode.textContent = "Red lenta";

    // Botones
    $("#b-torch").hidden = !S.caps.torch;
    $("#b-torch").classList.toggle("on", !!a.torch);
    $("#b-full").hidden = !document.documentElement.requestFullscreen;
    const zoomMax = nativeZoom() ? S.caps.zoom.max : 5, zoomMin = nativeZoom() ? S.caps.zoom.min : 1;
    zr.min = zoomMin; zr.max = Math.min(zoomMax, 10); zr.step = nativeZoom() ? S.caps.zoom.step : 0.05;
    if (document.activeElement !== zr) zr.value = a.zoom;
    $("#zoom-val").textContent = fmt1(a.zoom) + "×";
    $("#zoom").hidden = !S.started;

    const dot = $("#black-dot");
    dot.className = "dot" + (!S.connected ? " bad" : c.fps <= 2 ? " idle" : "");
    $("#black-hint").textContent = !S.connected ? "Sin conexión con el PC: se reconecta solo. Toca dos veces para volver."
      : !S.started ? "No está emitiendo. Toca dos veces para volver."
      : "Sigue emitiendo. Toca dos veces para volver.";
    if (!$("#sheet").hidden) syncSheet();
  }

  // El recuadro marca lo que se ve en el PC (con el encuadre «rellenar»).
  function placeFrame() {
    const f = $("#frame"), st = $("#stage");
    const vw = video.videoWidth, vh = video.videoHeight;
    if (!vw || S.cfg.adjust.fit === "contain") { f.style.display = "none"; return; }
    const rot = (S.cfg.adjust.rotate || 0) % 180;
    const rw = rot ? vh : vw, rh = rot ? vw : vh;
    const W = st.clientWidth, H = st.clientHeight;
    const shown = Math.min(W / rw, H / rh); // escala del vídeo en pantalla (contain)
    const z = nativeZoom() ? 1 : S.cfg.adjust.zoom;
    const target = S.cfg.w / S.cfg.h;
    let fw = rw * shown, fh = fw / target;
    if (fh > rh * shown) { fh = rh * shown; fw = fh * target; }
    if (z > 1) { /* el zoom digital ya agranda el vídeo: el recuadro se queda igual */ }
    f.style.display = "block";
    f.style.width = fw + "px";
    f.style.height = fh + "px";
  }

  window.addEventListener("resize", () => render());

  // ---------- avisos ----------
  let noticeAction = null;
  function notice(ic, title, text, btn, action) {
    $("#notice-ic").textContent = ic;
    $("#notice-title").textContent = title;
    $("#notice-text").textContent = text;
    const b = $("#notice-btn");
    b.hidden = !btn;
    b.textContent = btn || "";
    noticeAction = action || null;
    $("#notice").hidden = false;
  }
  $("#notice-btn").addEventListener("click", () => { $("#notice").hidden = true; if (noticeAction) noticeAction(); });

  let toastTimer = 0;
  function toast(msg) {
    const t = $("#toast");
    t.textContent = msg;
    t.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { t.hidden = true; }, 2200);
  }

  // ---------- arranque ----------
  if (!window.isSecureContext || !navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    const el = $("#start-err");
    el.textContent = "Este navegador no deja usar la cámara aquí. Abre el enlace del código QR (empieza por https) con Chrome o Safari.";
    el.hidden = false;
    $("#go").disabled = true;
  }
  connect();
  render();
  wake();
  // Si el permiso ya se dio otra vez, se empieza sin tocar nada.
  if (navigator.permissions && navigator.permissions.query) {
    navigator.permissions.query({ name: "camera" }).then((p) => { if (p.state === "granted") startCamera(); }).catch(() => {});
  }
})();
