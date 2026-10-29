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
  const DEFAULT_ADJUST = { facing: "user", lens: "", zoom: 1, panX: 0, panY: 0, brightness: 1, contrast: 1, saturation: 1, mirror: false, rotate: 0, fit: "cover", torch: false, res: 0, fps: 0 };
  const S = {
    cfg: { role: "spare", w: 320, h: 180, fps: 2, adjust: { ...DEFAULT_ADJUST }, pc: "", name: "", frozen: false, covered: false, watched: false },
    stream: null, track: null, caps: {}, cameras: [],
    connected: false, transport: null, started: false, starting: false,
    seq: 0, inflight: 0, nextAt: 0, q: 0.65, sent: new Map(), rtt: 0, rttN: 0,
    frames: 0, fpsNow: 0, bytes: 0, kbps: 0, sentCount: 0, netBusy: 0, cpuBusy: 0,
    encMs: 0, avgSize: 0, scaleStep: 0, up: 0, down: 0, sentW: 0, sentH: 0,
    camFrames: 0, camFps: 0, capKey: "", lastAck: 0, lastPong: 0, rttMin: 0, rttMinAt: 0,
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
    everConnected = true;
    clearTimeout(lostTimer);
    lostTimer = 0;
    if (!S.bye) $("#notice").hidden = true; // quita «TurboCAM se ha cerrado»
    retryDelay = 1000;
    S.inflight = 0;
    S.sent.clear();
    S.lastAck = S.lastPong = performance.now();
    H.forceKey = true; // el PC empieza a decodificar de cero
    sendState(true);
    render();
  }

  function onClosed() {
    rtcStop(false);
    const was = S.connected;
    S.connected = false;
    S.inflight = 0;
    if (was) render();
    scheduleReconnect();
    if (was || !everConnected) watchLost();
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
      case "pong": S.lastPong = performance.now(); if (m.ts) addRTT(performance.now() - m.ts); break;
      case "key": H.forceKey = true; break;
      case "rtc": rtcAnswer(m.sdp); break;
      case "ice": if (R.pc && m.c && m.c.candidate) R.pc.addIceCandidate(m.c).catch(() => {}); break;
      case "rtcok": R.live = true; sendState(true); render(); break;
      case "rtcoff": rtcStop(true); break;
      case "codec": if (m.h264 === false) h264Fail("el PC no decodifica H.264"); break;
      case "photo": takePhoto(); break;
      case "bye": onBye(m.reason); break;
    }
  }

  function onBye(reason) {
    if (S.bye === "forgotten") return; // ya lo dice el aviso de «olvidado»
    if (reason === "closed") {
      notice("💤", "TurboCAM se ha cerrado en el PC", "Si vuelve a abrirse con el mismo enlace, la imagen seguirá sola. Si el enlace cambia, escanea el código nuevo.",
        null, null, "Escanear el código nuevo", openScan);
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
    else if (S.track) { applyTrackControls(); checkCapture(); }
    syncSheet();
    render();
  }

  // ---------- cámara ----------
  async function startCamera() {
    if (S.starting) return;
    S.starting = true;
    const a = S.cfg.adjust;
    stopCamera(true);
    clearTimeout(reopenTimer);
    reopenTimer = 0;
    const [cw, ch] = captureSize();
    S.capKey = cw + "x" + ch;
    const video0 = { width: { ideal: cw }, height: { ideal: ch }, frameRate: { ideal: 30, max: 30 } };
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
    startFrames();
    sendState(true);
    render();
  }

  // La cámara se abre a la resolución justa para lo que se manda (más si hay zoom
  // digital): abrirla a 1080p para mandar 360p obliga al móvil a mover el triple de
  // píxeles en cada fotograma, y muchos móviles no pasan de 15 fps a 1080p.
  const CAPTURES = [[640, 480], [1280, 720], [1920, 1080]];
  function captureSize() {
    const z = nativeZoom() ? 1 : Math.max(1, S.cfg.adjust.zoom || 1);
    const [w, h] = sendSize(true);
    const long = Math.max(w, h) * z, short = Math.min(w, h) * z;
    for (const c of CAPTURES) if (c[0] >= long * 0.95 && c[1] >= short * 0.95) return c;
    return CAPTURES[CAPTURES.length - 1];
  }

  // Si el PC pide otro tamaño, la cámara se vuelve a abrir, con un respiro por si el
  // cambio dura poco (reabrirla corta la imagen medio segundo).
  let reopenTimer = 0;
  function checkCapture() {
    if (!S.started || S.starting) return;
    const [w, h] = captureSize();
    if (w + "x" + h === S.capKey) { clearTimeout(reopenTimer); reopenTimer = 0; return; }
    if (!reopenTimer) reopenTimer = setTimeout(() => {
      reopenTimer = 0;
      const [w2, h2] = captureSize();
      if (S.started && w2 + "x" + h2 !== S.capKey) startCamera();
    }, 1500);
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
  //
  // Cada fotograma: createImageBitmap recorta y escala en la GPU solo la parte que se
  // ve, un codificador en segundo plano lo gira, le pone los filtros y lo pasa a JPEG,
  // y la página lo manda. Hay varios codificadores a la vez, así que el JPEG (lo más
  // caro) no limita los fotogramas por segundo. Sin workers, se hace aquí mismo.
  const probe = document.createElement("canvas").getContext("2d");
  S.filterOK = (() => { try { probe.filter = "blur(1px)"; return probe.filter === "blur(1px)"; } catch { return false; } })();

  // geometry calcula cómo poner el vídeo en un lienzo de w x h con los retoques.
  function geometry(w, h, vw = video.videoWidth, vh = video.videoHeight) {
    if (!vw || !vh) return null;
    const a = S.cfg.adjust;
    const rot = ((a.rotate || 0) % 360 + 360) % 360;
    const rw = rot % 180 ? vh : vw, rh = rot % 180 ? vw : vh;
    const contain = a.fit === "contain";
    const s = (contain ? Math.min(w / rw, h / rh) : Math.max(w / rw, h / rh)) * (nativeZoom() ? 1 : a.zoom);
    const maxX = Math.max(0, (rw * s - w) / 2), maxY = Math.max(0, (rh * s - h) / 2);
    const tx = w / 2 - (a.panX || 0) * maxX, ty = h / 2 - (a.panY || 0) * maxY;
    // Las esquinas del lienzo llevadas al vídeo dan la parte que se ve.
    const r = rot * Math.PI / 180, cos = Math.round(Math.cos(r)), sin = Math.round(Math.sin(r));
    let x0 = vw, y0 = vh, x1 = 0, y1 = 0;
    for (const [px, py] of [[0, 0], [w, 0], [0, h], [w, h]]) {
      let ux = px - tx;
      const uy = py - ty;
      if (a.mirror) ux = -ux;
      const vx = (ux * cos + uy * sin) / s + vw / 2, vy = (-ux * sin + uy * cos) / s + vh / 2;
      x0 = Math.min(x0, vx); x1 = Math.max(x1, vx); y0 = Math.min(y0, vy); y1 = Math.max(y1, vy);
    }
    x0 = Math.max(0, Math.floor(x0)); y0 = Math.max(0, Math.floor(y0));
    x1 = Math.min(vw, Math.ceil(x1)); y1 = Math.min(vh, Math.ceil(y1));
    const cw = x1 - x0, ch = y1 - y0;
    if (cw < 2 || ch < 2) return null;
    const custom = a.brightness !== 1 || a.contrast !== 1 || a.saturation !== 1;
    return {
      w, h, tx, ty, rot, mirror: !!a.mirror, cx: x0, cy: y0, cw, ch,
      bw: Math.max(1, Math.round(cw * s)), bh: Math.max(1, Math.round(ch * s)),
      ox: ((x0 + x1) / 2 - vw / 2) * s, oy: ((y0 + y1) / 2 - vh / 2) * s,
      filter: S.filterOK && custom ? `brightness(${a.brightness}) contrast(${a.contrast}) saturate(${a.saturation})` : "",
      fill: contain || rw * s < w - 1 || rh * s < h - 1,
    };
  }

  // Codificadores. encode devuelve { blob, ms } o null.
  const enc = (() => {
    const canWork = typeof Worker === "function" && typeof OffscreenCanvas === "function" &&
      "convertToBlob" in OffscreenCanvas.prototype && typeof createImageBitmap === "function";
    const n = canWork ? Math.max(1, Math.min(3, (navigator.hardwareConcurrency || 2) - 2)) : 0;
    const workers = [];
    let nextID = 1, failed = !canWork;
    const waiting = new Map();
    for (let i = 0; i < n; i++) {
      const wk = new Worker("encoder.js");
      wk.busy = false;
      wk.onmessage = (e) => {
        wk.busy = false;
        const done = waiting.get(e.data.id);
        waiting.delete(e.data.id);
        if (e.data.error) { failed = true; if (done) done(null); return; }
        if (done) done({ blob: e.data.blob, ms: e.data.ms });
      };
      wk.onerror = () => { wk.busy = false; failed = true; };
      workers.push(wk);
    }
    // Plan B: en la página, de uno en uno.
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d", { alpha: false });
    let mainBusy = false;
    function encodeHere(g, q, frame) {
      mainBusy = true;
      const t0 = performance.now();
      if (canvas.width !== g.w || canvas.height !== g.h) { canvas.width = g.w; canvas.height = g.h; }
      paintFrame(ctx, frame || video, g, false);
      if (frame) frame.close();
      return new Promise((res) => canvas.toBlob((blob) => {
        mainBusy = false;
        res(blob ? { blob, ms: performance.now() - t0 } : null);
      }, "image/jpeg", q));
    }
    return {
      get count() { return failed ? 1 : workers.length; },
      idle() { return failed ? !mainBusy : workers.some((w) => !w.busy); },
      // frame: un VideoFrame de la cámara (se cierra aquí) o null para usar el vídeo.
      async encode(g, q, frame) {
        if (failed) return encodeHere(g, q, frame);
        const wk = workers.find((w) => !w.busy);
        if (!wk) { if (frame) frame.close(); return null; }
        wk.busy = true;
        let bitmap;
        try {
          bitmap = await createImageBitmap(frame || video, g.cx, g.cy, g.cw, g.ch, { resizeWidth: g.bw, resizeHeight: g.bh, resizeQuality: "medium" });
        } catch {
          wk.busy = false;
          failed = true; // este navegador no recorta así: plan B
          return null;
        } finally {
          if (frame) frame.close();
        }
        const id = nextID++;
        return new Promise((res) => {
          waiting.set(id, res);
          wk.postMessage({ id, bitmap, g, q }, [bitmap]);
        });
      },
    };
  })();

  function header(kind, n) {
    const b = new Uint8Array(5);
    b[0] = kind;
    new DataView(b.buffer).setUint32(1, n, true);
    return b;
  }

  // Tamaño que se manda: el que pide el PC o, en automático, algo menos si el móvil
  // no da para más.
  const SCALES = [1, 0.75, 0.5];
  // full: sin la reducción automática (para elegir a qué resolución abrir la cámara).
  function sendSize(full) {
    let { w, h } = S.cfg;
    const k = S.cfg.adjust.res || full ? 1 : SCALES[S.scaleStep];
    if (k < 1) { w = Math.round(w * k / 2) * 2; h = Math.round(h * k / 2) * 2; }
    return [Math.max(w, 64), Math.max(h, 36)];
  }

  // Fotogramas de la cámara. Con MediaStreamTrackProcessor llegan todos tal cual
  // salen de la cámara, aunque la pantalla esté en negro o el vídeo no se pinte; si
  // no existe, se sigue al vídeo de la página (requestVideoFrameCallback) o, en
  // último caso, un temporizador.
  const hasProcessor = typeof MediaStreamTrackProcessor === "function";
  function startFrames() {
    if (hasProcessor && S.track) readFrames(S.track);
    else pump();
  }

  async function readFrames(track) {
    let reader;
    try {
      reader = new MediaStreamTrackProcessor({ track, maxBufferSize: 1 }).readable.getReader();
    } catch { pump(); return; }
    for (;;) {
      let r;
      try { r = await reader.read(); } catch { break; }
      if (r.done) break;
      const f = r.value;
      if (S.track !== track) { f.close(); break; }
      S.camFrames++;
      if (!tick(f)) f.close();
    }
    try { reader.cancel(); } catch { /* ya cerrado */ }
  }

  let pumpQueued = false, lastPresented = 0;
  function pump() {
    if (pumpQueued || !S.started) return;
    pumpQueued = true;
    if (video.requestVideoFrameCallback) video.requestVideoFrameCallback(onVideoFrame);
    else setTimeout(onVideoFrame, 1000 / 30);
  }

  function onVideoFrame(_, meta) {
    pumpQueued = false;
    if (!S.started) return;
    if (meta && meta.presentedFrames) {
      S.camFrames += Math.max(1, Math.min(4, meta.presentedFrames - lastPresented));
      lastPresented = meta.presentedFrames;
    } else S.camFrames++;
    tick(null);
    pump();
  }

  // En camino a la vez: los que caben en el tiempo de ida y vuelta (más uno), para
  // no frenar por la latencia de internet sin dejar que se acumule retraso.
  function maxInflight(interval) {
    if (!S.rttN) return 3;
    if (useH264()) return Math.max(3, Math.min(10, Math.ceil(S.rtt / interval) + 3));
    return Math.max(2, Math.min(6, Math.ceil(S.rtt / interval) + 1));
  }

  // tick decide si toca mandar este fotograma. Devuelve true si se queda con frame
  // (y entonces lo cierra él).
  function tick(frame) {
    if (!S.started) return false;
    const now = performance.now();
    const interval = 1000 / Math.max(1, S.cfg.fps || 1);
    // Los fotogramas de la cámara no llegan clavados (25 ms, luego 40…): se admite
    // uno hasta medio intervalo antes de su hora, y la hora siguiente cuenta desde la
    // que tocaba, no desde la de llegada. Así no se pierden por el baile.
    if (now < S.nextAt - interval * 0.5 || !S.connected || S.hidden) return false;
    if (R.pc) rtcFeed(now, frame);
    if (R.live) { takeSlot(now); return false; } // ya va todo por WebRTC
    const backlog = S.transport ? S.transport.backlog() : 0;
    const h264 = useH264();
    if (h264 ? !h264Idle() : !enc.idle()) { S.cpuBusy++; return false; }
    // Con H.264 cada fotograma es pequeño: en la cola de salida no se deja más de un
    // cuarto de segundo de vídeo (si no, se acumula retraso).
    const maxBacklog = h264 ? Math.max(64e3, H.br / 32) : Math.max(400e3, S.avgSize * 4);
    if (S.inflight >= maxInflight(interval) || backlog > maxBacklog) { S.netBusy++; return false; }
    return h264 ? sendH264(now, frame) : sendFrame(now, frame);
  }

  function takeSlot(now) {
    const interval = 1000 / Math.max(1, S.cfg.fps || 1);
    S.nextAt = Math.max(S.nextAt, now - interval * 0.5) + interval;
  }

  // ---------- H.264 por hardware ----------
  //
  // El móvil comprime con su codificador de vídeo (el mismo que usa para grabar), que
  // casi no gasta CPU y deja cada fotograma en unos pocos KB: diez veces menos que un
  // JPEG. Si el navegador o el PC no pueden, se sigue con JPEG.
  const H = {
    ok: typeof VideoEncoder === "function" && typeof VideoFrame === "function" && typeof OffscreenCanvas === "function",
    failed: "", enc: null, cfg: null, key: "", configuring: null, hw: false,
    br: 0, minBr: 0, maxBr: 0, cfgBr: 0, cfgAt: 0, up: 0,
    forceKey: true, lastKey: 0, canvas: null, ctx: null, crop: null, lastOut: 0, resets: [],
  };

  // Si el codificador deja de dar fotogramas (algunos móviles se cuelgan al volver de
  // segundo plano o tras un cambio de tamaño), se rehace; si pasa mucho, a JPEG.
  setInterval(() => {
    const now = performance.now();
    if (!H.enc || H.enc.state !== "configured" || H.enc.encodeQueueSize === 0) return;
    if (now - H.lastOut < 2000 || now - H.cfgAt < 2000) return;
    H.resets = H.resets.filter((t) => now - t < 60000).concat(now);
    if (H.resets.length > 3) { h264Fail("el codificador se cuelga"); return; }
    try { H.enc.close(); } catch { }
    H.enc = null;
    H.key = "";
  }, 1000);

  // ?jpeg en el enlace: sin H.264 (para comparar).
  if (new URLSearchParams(location.search).has("jpeg")) H.failed = "desactivado en el enlace";

  // Por el plan B por HTTP los envíos pueden llegar desordenados, y el H.264 hay que
  // decodificarlo en orden: ahí se sigue con JPEG.
  function useH264() { return H.ok && !H.failed && S.cfg.h264 === true && !!S.transport && S.transport.kind === "ws"; }
  function h264Idle() { return !H.enc || H.enc.state !== "configured" || H.enc.encodeQueueSize < 2; }

  function h264Fail(why) {
    if (H.failed) return;
    H.failed = why || "error";
    try { if (H.enc && H.enc.state !== "closed") H.enc.close(); } catch { }
    H.enc = null;
    H.key = "";
    sendState(true);
  }

  // Bits por píxel: de lo justo a muy bueno. Con pocos fotogramas por segundo se
  // cuenta como si fueran 10 (cada uno lleva más cambio).
  function brFor(w, h, fps, k) { return Math.round(w * h * Math.max(fps, 10) * k); }

  // Nivel H.264 según los macrobloques por fotograma y por segundo.
  function avcLevel(w, h, fps) {
    const mbs = Math.ceil(w / 16) * Math.ceil(h / 16), rate = mbs * fps;
    if (mbs <= 1620 && rate <= 40500) return "1E";
    if (mbs <= 3600 && rate <= 108000) return "1F";
    if (mbs <= 8192 && rate <= 245760) return "28";
    return "2A";
  }

  // h264Ready: el codificador está listo para w x h a fps; si no, lo prepara (tarda
  // unos milisegundos) y de mientras no se manda nada.
  function h264Ready(w, h, fps) {
    const key = w + "x" + h + "@" + fps;
    if (H.key === key && H.enc && H.enc.state === "configured") return true;
    if (!H.configuring) H.configuring = h264Configure(w, h, fps, key).finally(() => { H.configuring = null; });
    return false;
  }

  async function h264Configure(w, h, fps, key) {
    const lvl = avcLevel(w, h, fps);
    const minBr = brFor(w, h, fps, 0.02), maxBr = brFor(w, h, fps, 0.15);
    // Al cambiar de tamaño se conserva lo aprendido de la red (en proporción).
    const br = H.maxBr ? Math.max(minBr, Math.min(maxBr, Math.round(H.br / H.maxBr * maxBr))) : brFor(w, h, fps, 0.07);
    let chosen = null, hw = false;
    outer: for (const accel of ["prefer-hardware", "no-preference"]) {
      for (const prof of ["42E0", "4D00", "6400"]) {
        const c = {
          codec: "avc1." + prof + lvl, width: w, height: h, bitrate: br, framerate: fps,
          latencyMode: "realtime", hardwareAcceleration: accel, avc: { format: "annexb" },
        };
        try {
          const r = await VideoEncoder.isConfigSupported(c);
          if (r.supported) { chosen = c; hw = accel === "prefer-hardware"; break outer; }
        } catch { /* esa combinación no */ }
      }
    }
    if (!chosen) { h264Fail("el navegador no comprime H.264"); return; }
    try {
      if (!H.enc || H.enc.state === "closed") H.enc = new VideoEncoder({ output: onChunk, error: (e) => h264Fail(String(e && e.message || e)) });
      H.enc.configure(chosen);
    } catch (e) { h264Fail(String(e && e.message || e)); return; }
    Object.assign(H, { cfg: chosen, key, hw, br, minBr, maxBr, cfgBr: br, cfgAt: performance.now(), lastOut: performance.now(), forceKey: true });
    sendState(true);
  }

  // Cambiar la tasa de bits sin rehacer el codificador (Chrome lo admite al vuelo).
  function h264SetBitrate(br) {
    if (!H.enc || H.enc.state !== "configured" || !H.cfg) return;
    if (Math.abs(br - H.cfgBr) < H.cfgBr * 0.08 || performance.now() - H.cfgAt < 1500) return;
    try {
      H.cfg = { ...H.cfg, bitrate: br };
      H.enc.configure(H.cfg);
      H.cfgBr = br;
      H.cfgAt = performance.now();
    } catch (e) { h264Fail(String(e && e.message || e)); }
  }

  // makeFrame prepara el fotograma que se manda, de w x h y con los retoques. Sin
  // retoques, se recorta el de la cámara sin copiar nada (el codificador lo escala);
  // si no, se pinta en un lienzo. Devuelve un VideoFrame (hay que cerrarlo) o null.
  function makeFrame(now, frame, w, h) {
    const vw = video.videoWidth, vh = video.videoHeight;
    if (frame && (frame.displayWidth !== vw || frame.displayHeight !== vh || frame.rotation)) frame = null;
    const g = geometry(w, h, vw, vh);
    if (!g) return null;
    try {
      if (frame && !g.rot && !g.mirror && !g.filter && !g.fill && H.crop !== false) {
        const vr = frame.visibleRect || { x: 0, y: 0 };
        return new VideoFrame(frame, {
          visibleRect: { x: (vr.x + g.cx) & ~1, y: (vr.y + g.cy) & ~1, width: g.cw & ~1, height: g.ch & ~1 },
          timestamp: frame.timestamp,
        });
      }
      if (!H.canvas || H.canvas.width !== w || H.canvas.height !== h) {
        H.canvas = new OffscreenCanvas(w, h);
        H.ctx = H.canvas.getContext("2d", { alpha: false });
      }
      paintFrame(H.ctx, frame || video, g, false);
      return new VideoFrame(H.canvas, { timestamp: Math.round(now * 1000) });
    } catch {
      if (H.crop === null) { H.crop = false; return null; } // recortar así no va: siempre por el lienzo
      throw new Error("no se pudo preparar el fotograma");
    }
  }

  function sendH264(now, frame) {
    const [w, h] = sendSize();
    const fps = Math.max(1, S.cfg.fps || 1);
    if (!h264Ready(w, h, fps)) return false;
    let vf;
    try { vf = makeFrame(now, frame, w, h); } catch (e) { h264Fail(e.message); return false; }
    if (!vf) return false;
    const key = H.forceKey || now - H.lastKey > 6000;
    if (key) { H.forceKey = false; H.lastKey = now; }
    takeSlot(now);
    S.sentW = w; S.sentH = h;
    try { H.enc.encode(vf, { keyFrame: key }); } catch (e) { h264Fail(String(e && e.message || e)); }
    vf.close();
    return false; // el fotograma de la cámara lo cierra quien llama
  }

  function onChunk(chunk) {
    H.lastOut = performance.now();
    if (!S.connected || !S.transport) { H.forceKey = true; return; }
    const n = (S.seq = (S.seq + 1) >>> 0);
    const b = new Uint8Array(6 + chunk.byteLength);
    b[0] = 3;
    new DataView(b.buffer).setUint32(1, n, true);
    b[5] = chunk.type === "key" ? 1 : 0;
    chunk.copyTo(b.subarray(6));
    S.avgSize = S.avgSize ? S.avgSize * 0.9 + chunk.byteLength * 0.1 : chunk.byteLength;
    S.inflight++;
    S.sentCount++;
    S.sent.set(n, performance.now());
    S.bytes += chunk.byteLength;
    S.transport.sendBlob(b);
  }

  function sendFrame(now, frame) {
    const [w, h] = sendSize();
    // El fotograma de la cámara se usa si viene igual que lo enseña el vídeo; si
    // viene sin girar (móvil en vertical), se coge del vídeo, que ya lo gira.
    const vw = video.videoWidth, vh = video.videoHeight;
    if (frame && (frame.displayWidth !== vw || frame.displayHeight !== vh || frame.rotation)) frame = null;
    const g = geometry(w, h, vw, vh);
    if (!g) return false;
    takeSlot(now);
    S.sentW = w; S.sentH = h;
    const n = (S.seq = (S.seq + 1) >>> 0);
    enc.encode(g, S.q, frame).then((r) => {
      if (!r || !S.connected || !S.transport) return;
      S.encMs = S.encMs ? S.encMs * 0.85 + r.ms * 0.15 : r.ms;
      S.avgSize = S.avgSize ? S.avgSize * 0.9 + r.blob.size * 0.1 : r.blob.size;
      S.inflight++;
      S.sentCount++;
      S.sent.set(n, performance.now());
      S.bytes += r.blob.size;
      S.transport.sendBlob(new Blob([header(1, n), r.blob]));
    });
    return !!frame;
  }

  // ---------- WebRTC ----------
  //
  // Si se puede, el vídeo va directo al PC por UDP, como en una videollamada: sin
  // pasar por Cloudflare, con menos retraso, y el navegador ajusta solo la tasa de
  // bits y reenvía lo que se pierda. La negociación va por la conexión de siempre, y
  // mientras tanto (o si no sale) el vídeo sigue por ella.
  const R = {
    ok: typeof RTCPeerConnection === "function" && typeof VideoFrame === "function" &&
      (typeof MediaStreamTrackGenerator === "function" || "captureStream" in HTMLCanvasElement.prototype),
    pc: null, gen: null, writer: null, cv: null, cvTrack: null, sender: null,
    live: false, startAt: 0, failAt: 0, fails: 0, key: "", kbps: 0, hw: false, fps: 0, rtt: 0, impl: "", bytes: 0, bytesAt: 0,
  };

  // ?nortc en el enlace: sin WebRTC (para comparar).
  if (new URLSearchParams(location.search).has("nortc")) R.ok = false;

  function rtcWanted() {
    return R.ok && S.cfg.rtc === true && S.cfg.h264 === true && S.connected && S.started && !S.hidden &&
      !R.pc && performance.now() >= R.failAt;
  }

  async function rtcStart() {
    const caps = RTCRtpSender.getCapabilities ? RTCRtpSender.getCapabilities("video") : null;
    const h264 = caps ? caps.codecs.filter((c) => /h264/i.test(c.mimeType)) : [];
    if (caps && !h264.length) { R.ok = false; return; } // este navegador no manda H.264 por WebRTC
    let track;
    if (typeof MediaStreamTrackGenerator === "function") {
      R.gen = new MediaStreamTrackGenerator({ kind: "video" });
      R.writer = R.gen.writable.getWriter();
      track = R.gen;
    } else {
      R.cv = document.createElement("canvas");
      R.cvTrack = R.cv.captureStream(0).getVideoTracks()[0];
      track = R.cvTrack;
    }
    try { track.contentHint = "motion"; } catch { }
    const pc = R.pc = new RTCPeerConnection({ iceServers: [{ urls: ["stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"] }], bundlePolicy: "max-bundle" });
    R.live = false;
    R.startAt = performance.now();
    R.key = "";
    const tr = pc.addTransceiver(track, { direction: "sendonly" });
    if (tr.setCodecPreferences && caps) {
      // Primero el perfil que mejor llevan los codificadores por hardware.
      const rank = (c) => (/42e01f/.test(c.sdpFmtpLine || "") ? 0 : /42001f/.test(c.sdpFmtpLine || "") ? 1 : 2) + (/packetization-mode=1/.test(c.sdpFmtpLine || "") ? 0 : 5);
      const rtx = caps.codecs.filter((c) => /rtx/i.test(c.mimeType));
      try { tr.setCodecPreferences(h264.sort((a, b) => rank(a) - rank(b)).concat(rtx)); } catch { }
    }
    R.sender = tr.sender;
    pc.onicecandidate = (e) => { if (e.candidate && R.pc === pc) send({ t: "ice", c: e.candidate.toJSON() }); };
    pc.onconnectionstatechange = () => {
      const st = pc.connectionState;
      if (st === "failed" || st === "closed") { if (R.pc === pc) rtcStop(true); }
      else if (st === "disconnected") setTimeout(() => { if (R.pc === pc && pc.connectionState === "disconnected") rtcStop(true); }, 3000);
    };
    try {
      await pc.setLocalDescription(await pc.createOffer());
      if (R.pc === pc) send({ t: "rtc", sdp: pc.localDescription.sdp });
    } catch { rtcStop(true); }
  }

  async function rtcAnswer(sdp) {
    const pc = R.pc;
    if (!pc || !sdp) return;
    try { await pc.setRemoteDescription({ type: "answer", sdp }); rtcParams(); } catch { rtcStop(true); }
  }

  // Tope de tasa de bits y de fotogramas, y que ante poca red baje la resolución antes
  // que los fotogramas por segundo (para una cámara se nota más el tirón).
  function rtcParams() {
    if (!R.sender || !R.pc) return;
    const [w, h] = sendSize(), fps = Math.max(1, S.cfg.fps || 1);
    const key = w + "x" + h + "@" + fps;
    if (key === R.key) return;
    R.key = key;
    try {
      const prm = R.sender.getParameters();
      if (!prm.encodings || !prm.encodings.length) prm.encodings = [{}];
      prm.encodings[0].maxBitrate = brFor(w, h, fps, 0.15);
      prm.encodings[0].maxFramerate = fps;
      prm.degradationPreference = "maintain-framerate";
      R.sender.setParameters(prm).catch(() => {});
    } catch { }
  }

  function rtcFeed(now, frame) {
    const [w, h] = sendSize();
    if (R.writer) {
      if (R.writer.desiredSize !== null && R.writer.desiredSize < 0) return; // aún con el anterior
      let vf;
      try { vf = makeFrame(now, frame, w, h); } catch { return; }
      if (vf) R.writer.write(vf).catch(() => {}); // el generador se queda con él y lo cierra
    } else if (R.cv) {
      const vw = video.videoWidth, vh = video.videoHeight;
      const g = geometry(w, h, vw, vh);
      if (!g) return;
      if (R.cv.width !== w || R.cv.height !== h) { R.cv.width = w; R.cv.height = h; }
      paintFrame(R.cv.getContext("2d", { alpha: false }), video, g, false);
      if (R.cvTrack.requestFrame) R.cvTrack.requestFrame();
    }
    if (!R.live) return;
    S.sentW = w; S.sentH = h;
  }

  // fail: no ha salido o se ha caído; se vuelve a probar más tarde (cada vez más tarde).
  function rtcStop(fail) {
    const pc = R.pc;
    if (!pc && !R.gen && !R.cvTrack) return;
    const wasLive = R.live;
    R.pc = null;
    R.live = false;
    try { pc && pc.close(); } catch { }
    try { R.writer && R.writer.close().catch(() => {}); } catch { }
    try { R.gen && R.gen.stop(); } catch { }
    try { R.cvTrack && R.cvTrack.stop(); } catch { }
    R.gen = R.writer = R.cv = R.cvTrack = R.sender = null;
    R.bytes = R.bytesAt = 0;
    if (fail) {
      R.fails++;
      R.failAt = performance.now() + Math.min(120000, 5000 * 2 ** Math.min(R.fails, 5));
      if (S.connected) send({ t: "rtcstop" });
    }
    H.forceKey = true; // el vídeo vuelve por la conexión de siempre: el PC necesita un clave
    if (wasLive) { sendState(true); render(); }
  }

  // Números del codificador de WebRTC (para la hoja y el PC).
  async function rtcStats() {
    if (!R.sender || !R.live) return;
    try {
      (await R.sender.getStats()).forEach((r) => {
        if (r.type === "outbound-rtp" && r.kind === "video") {
          if (R.bytesAt && r.timestamp > R.bytesAt) S.kbps = Math.round((r.bytesSent - R.bytes) * 8 / (r.timestamp - R.bytesAt));
          R.bytes = r.bytesSent; R.bytesAt = r.timestamp;
          R.fps = Math.round(r.framesPerSecond || 0);
          R.kbps = Math.round((r.targetBitrate || 0) / 1000);
          R.impl = r.encoderImplementation || "";
          R.hw = r.powerEfficientEncoder === true || /mediacodec|hardware|external/i.test(R.impl) && !/openh264|libvpx|software/i.test(R.impl);
          // En el móvil, comprimir por software calienta y acaba bajando el ritmo: si
          // WebRTC no tiene el codificador por hardware y WebCodecs sí, mejor sin WebRTC.
          if (r.powerEfficientEncoder === false && H.hw && /Android|iPhone|iPad/.test(navigator.userAgent)) {
            rtcStop(true);
            R.failAt = Infinity;
          }
          if (r.frameWidth) { S.sentW = r.frameWidth; S.sentH = r.frameHeight; }
        }
        if (r.type === "remote-inbound-rtp" && r.kind === "video" && r.roundTripTime) R.rtt = Math.round(r.roundTripTime * 1000);
      });
    } catch { }
  }

  function onAck(m) {
    S.lastAck = performance.now();
    S.inflight = Math.max(0, S.inflight - 1);
    const t0 = S.sent.get(m.n);
    if (t0 !== undefined) { S.sent.delete(m.n); addRTT(performance.now() - t0); }
    if (S.sent.size > 60) S.sent.clear();
    S.frames++;
  }

  function addRTT(ms) {
    S.rtt = S.rttN ? S.rtt * 0.8 + ms * 0.2 : ms;
    S.rttN++;
    // La mínima de los últimos 10 s es la latencia de la red sin colas: si la media se
    // aleja mucho, se está acumulando vídeo por el camino.
    const now = performance.now();
    if (!S.rttMin || ms < S.rttMin || now - S.rttMinAt > 10000) { S.rttMin = ms; S.rttMinAt = now; }
  }

  // Cada segundo se ajusta solo:
  //  - la red no da abasto (fotogramas esperando a la red) → menos calidad;
  //  - el móvil no da abasto (todos los codificadores ocupados) → en automático,
  //    menos resolución (bajar la calidad apenas ahorra trabajo al móvil);
  //  - sobra de todo → poco a poco, más calidad y más resolución.
  setInterval(() => {
    S.fpsNow = S.frames; S.frames = 0;
    S.camFps = S.started ? S.camFrames : 0; S.camFrames = 0;
    checkCapture(); // por si el cambio de tamaño llegó mientras se abría la cámara
    if (rtcWanted()) rtcStart();
    else if (R.pc && !R.live && performance.now() - R.startAt > 12000) rtcStop(true); // no conecta
    else if (R.pc && (!S.started || S.cfg.rtc !== true)) rtcStop(false);
    if (R.pc) { rtcParams(); rtcStats(); }
    if (R.live) { S.fpsNow = R.fps; }
    S.kbps = Math.round(S.bytes * 8 / 1000); S.bytes = 0;
    const target = Math.max(1, S.cfg.fps), interval = 1000 / target;
    const sent = S.sentCount, net = S.netBusy, cpu = S.cpuBusy;
    S.sentCount = 0; S.netBusy = 0; S.cpuBusy = 0;
    if (!S.connected || !S.started || target <= 4 || S.hidden) { render(); return; }
    if (R.live) { render(); return; } // por WebRTC lo ajusta el propio navegador
    const short = sent < target * 0.85;
    if (useH264() && H.cfg) {
      // Tasa de bits: baja deprisa si la red no da abasto o si crece la cola por el
      // camino (la latencia sube sobre la mínima); sube despacio si va holgado.
      const queued = S.rttN > 3 && S.rtt > S.rttMin + Math.max(80, interval * 3);
      if ((short && net > target * 0.2) || queued) { H.br = Math.max(H.minBr, Math.round(H.br * 0.75)); H.up = 0; }
      else if (!short && net === 0 && ++H.up >= 3) { H.br = Math.min(H.maxBr, Math.round(H.br * 1.15)); H.up = 0; }
      h264SetBitrate(H.br);
      S.q = H.maxBr ? H.br / H.maxBr : S.q;
    } else if (short && net > target * 0.2) S.q = Math.max(0.3, S.q - 0.05);
    else if (!short && net === 0 && S.rtt < interval * 3) S.q = Math.min(0.82, S.q + 0.02);
    if (!S.cfg.adjust.res) {
      if (short && cpu > target * 0.2) {
        S.up = 0;
        if (++S.down >= 2 && S.scaleStep < SCALES.length - 1) { S.scaleStep++; S.down = 0; }
      } else if (cpu === 0 && (useH264() || S.encMs < interval * enc.count * 0.35)) {
        S.down = 0;
        if (++S.up >= 5 && S.scaleStep > 0) { S.scaleStep--; S.up = 0; }
      } else { S.down = 0; S.up = 0; }
    }
    render();
  }, 1000);

  // Señal de vida y estado cada 4 s (también mide la latencia).
  setInterval(() => {
    if (!S.connected) return;
    send({ t: "ping", ts: performance.now() });
    sendState(false);
  }, 4000);

  // Una conexión puede quedarse muerta sin cerrarse (el túnel se atasca, el móvil
  // cambia de wifi a datos…): si no llega nada del PC en un rato, se rehace.
  setInterval(() => {
    if (!S.connected || S.bye) return;
    const now = performance.now();
    const stuck = (S.inflight > 0 && now - S.lastAck > 5000) || now - S.lastPong > 11000;
    if (stuck) { S.connected = false; S.inflight = 0; connect(); render(); }
  }, 1000);

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
      camW: video.videoWidth, camH: video.videoHeight,
      codec: R.live || useH264() ? "h264" : "jpeg", hw: R.live ? R.hw : useH264() && H.hw,
      kbps: R.live ? R.kbps : useH264() ? Math.round(H.cfgBr / 1000) : 0,
    };
    if (R.live && R.rtt) st.rtt = R.rtt;
    if (battery) { st.battery = Math.round(battery.level * 100) / 100; st.charging = battery.charging; }
    const key = JSON.stringify(st);
    if (!force && key === lastState) { send({ t: "state", q: st.q, rtt: st.rtt, cam: S.camFps, kbps: st.kbps }); return; }
    lastState = key;
    st.cam = S.camFps;
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
    const g = geometry(w & ~1, h & ~1);
    if (!g) return;
    c.width = g.w; c.height = g.h;
    cx.imageSmoothingQuality = "high";
    paintFrame(cx, video, g, false);
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
    else if (S.started) { video.play().catch(() => {}); if (!hasProcessor) pump(); }
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
    else { applyTrackControls(); checkCapture(); }
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
  $("#a-reset").addEventListener("click", () => adjust({ ...DEFAULT_ADJUST, facing: S.cfg.adjust.facing, lens: S.cfg.adjust.lens, res: S.cfg.adjust.res, fps: S.cfg.adjust.fps }));
  $("#a-res").addEventListener("change", (e) => { S.scaleStep = 0; adjust({ res: Number(e.target.value) }); });
  $("#a-fps").addEventListener("change", (e) => adjust({ fps: Number(e.target.value) }));

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
    $("#a-res").value = String(a.res || 0);
    $("#a-fps").value = String(a.fps || 0);
    const vw = video.videoWidth, vh = video.videoHeight;
    $("#info").innerHTML = [
      ["Conectado a", S.cfg.pc || "—"],
      ["Conexión", !S.connected ? "sin conexión" : R.live ? "WebRTC · directa al PC"
        : (S.transport.kind === "ws" ? "WebSocket" : "HTTP") + (location.hostname.endsWith("trycloudflare.com") ? " · Cloudflare" : " · directa") + (R.pc ? " (probando WebRTC…)" : "")],
      ["Cámara", vw ? `${vw}×${vh} · ${S.camFps} fps` + (camSlow() ? " (¿poca luz?)" : "") : "—"],
      ["Se envía", `${S.sentW || S.cfg.w}×${S.sentH || S.cfg.h} · ${S.fpsNow} fps · ${S.kbps} kb/s`],
      ["Calidad", R.live ? (R.rtt ? `${R.rtt} ms` : "—") : pct(S.q) + (S.rttN ? ` · ${Math.round(S.rtt)} ms` : "")],
      ["Compresión", R.live ? `H.264${R.hw ? " por hardware" : ""} · WebRTC` + (R.kbps ? ` · ${fmt1(R.kbps / 1000)} Mb/s` : "")
        : useH264() ? `H.264${H.hw ? " por hardware" : ""}` + (H.cfgBr ? ` · ${fmt1(H.cfgBr / 1e6)} Mb/s` : "")
        : `JPEG · ${enc.count} ${enc.count === 1 ? "hilo" : "hilos"}` + (S.encMs ? ` · ${Math.round(S.encMs)} ms por imagen` : "") + (H.failed ? ` (sin H.264: ${H.failed})` : "")],
    ].map(([k, v]) => `<span>${k}</span><b>${esc(v)}</b>`).join("");
  }

  // La cámara da menos fotogramas de los que se piden: casi siempre es poca luz (el
  // móvil alarga la exposición), y eso no se arregla desde aquí.
  function camSlow() {
    return S.started && S.camFps > 0 && S.camFps < Math.min(S.cfg.fps, 30) * 0.75;
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
    res.textContent = `${S.sentH || c.h}p · ${S.fpsNow} fps`;
    const mode = $("#chip-mode");
    const slowCam = camSlow(), slowNet = !R.live && S.q < (useH264() ? 0.3 : 0.45), slowPhone = !c.adjust.res && S.scaleStep > 0;
    mode.hidden = !(S.connected && S.started && c.fps > 4 && (slowCam || slowNet || slowPhone));
    mode.textContent = slowCam ? `Cámara a ${S.camFps} fps: ¿poca luz?` : slowNet ? "Red lenta" : "Resolución reducida";

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
  let noticeAction = null, noticeAction2 = null;
  function notice(ic, title, text, btn, action, btn2, action2) {
    $("#notice-ic").textContent = ic;
    $("#notice-title").textContent = title;
    $("#notice-text").textContent = text;
    const b = $("#notice-btn"), b2 = $("#notice-btn2");
    b.hidden = !btn;
    b.textContent = btn || "";
    b2.hidden = !btn2;
    b2.textContent = btn2 || "";
    noticeAction = action || null;
    noticeAction2 = action2 || null;
    $("#notice").hidden = false;
  }
  $("#notice-btn").addEventListener("click", () => { $("#notice").hidden = true; if (noticeAction) noticeAction(); });
  $("#notice-btn2").addEventListener("click", () => { $("#notice").hidden = true; if (noticeAction2) noticeAction2(); });

  // ---------- otro PC: escanear un código ----------
  //
  // Cada enlace de Cloudflare es un sitio distinto para el navegador, y cambia cada
  // vez que se abre TurboCAM en el PC: desde aquí se puede pasar al nuevo sin salir.
  const LINK_RE = /^https:\/\/[^/\s]+\/c\/[0-9a-f]{32}\/?$/i;
  let scanStream = null, scanTimer = 0, resumeAfterScan = false;

  async function openScan() {
    openSheet(false);
    $("#notice").hidden = true;
    resumeAfterScan = S.started;
    if (S.started) stopCamera(true);
    $("#scan").hidden = false;
    $(".scan-box").classList.remove("found");
    $("#scan-input").value = "";
    const msg = $("#scan-msg");
    msg.textContent = "Apunta al código QR de la ventana de TurboCAM en el PC.";
    let det = null;
    try { if ("BarcodeDetector" in window) det = new BarcodeDetector({ formats: ["qr_code"] }); } catch { det = null; }
    if (!det) { msg.textContent = "Este navegador no lee códigos desde aquí: escanéalo con la app de la cámara o pega el enlace."; return; }
    try {
      scanStream = await navigator.mediaDevices.getUserMedia({ audio: false, video: { facingMode: { ideal: "environment" } } });
    } catch {
      msg.textContent = "No se pudo abrir la cámara para escanear. Pega el enlace.";
      return;
    }
    const v = $("#scan-video");
    v.srcObject = scanStream;
    v.play().catch(() => {});
    const loop = async () => {
      if (!scanStream) return;
      try {
        for (const c of await det.detect(v)) if (goTo(c.rawValue, true)) return;
      } catch { /* aún sin imagen */ }
      scanTimer = setTimeout(loop, 200);
    };
    loop();
  }

  function closeScan(resume) {
    clearTimeout(scanTimer);
    if (scanStream) scanStream.getTracks().forEach((t) => t.stop());
    scanStream = null;
    $("#scan-video").srcObject = null;
    $("#scan").hidden = true;
    if (resume && resumeAfterScan && !S.bye) startCamera();
  }

  function goTo(url, fromCamera) {
    url = String(url || "").trim();
    if (!LINK_RE.test(url)) {
      $("#scan-msg").textContent = fromCamera ? "Ese código no es de TurboCAM: busca el de la ventana de TurboCAM." : "Ese enlace no es de TurboCAM.";
      return false;
    }
    if (!url.endsWith("/")) url += "/";
    $(".scan-box").classList.add("found");
    closeScan(false);
    location.href = url;
    return true;
  }

  $("#scan-form").addEventListener("submit", (e) => { e.preventDefault(); goTo($("#scan-input").value, false); });
  $("#scan-cancel").addEventListener("click", () => closeScan(true));
  $("#b-scan").addEventListener("click", openScan);
  $("#start-scan").addEventListener("click", openScan);

  // ---------- olvidar este PC ----------
  $("#b-forget").addEventListener("click", () => {
    openSheet(false);
    notice("🧹", "¿Olvidar este PC?", "El móvil se desconecta y deja de entrar solo. En el PC se borran sus ajustes. Para volver, se escanea el código otra vez.",
      "Olvidar", forget, "Cancelar", () => {});
  });

  async function forget() {
    send({ t: "forget" });
    S.bye = "forgotten";
    stopCamera();
    const t = S.transport;
    S.connected = false;
    setTimeout(() => { if (t) t.close(); }, 400); // que salga el «olvídame» antes de cerrar
    try { Object.keys(localStorage).filter((k) => k.startsWith("turbocam.")).forEach((k) => localStorage.removeItem(k)); } catch { /* modo privado */ }
    try {
      if (navigator.serviceWorker) for (const r of await navigator.serviceWorker.getRegistrations()) await r.unregister();
      if (window.caches) for (const k of await caches.keys()) await caches.delete(k);
    } catch { /* nada que borrar */ }
    notice("🧹", "PC olvidado", "Este móvil ya no se conecta a ese PC. Para volver, o para usar otro, escanea su código.", "Escanear un código", openScan);
    render();
  }

  // ---------- ¿dónde está el PC? ----------
  //
  // Si no conecta en unos segundos, se mira si es que el PC está apagado o que el
  // enlace ya no vale (TurboCAM se ha vuelto a abrir y Cloudflare ha dado otro, o se
  // ha renovado el secreto), y se ofrece escanear el nuevo.
  let everConnected = false, lostTimer = 0;
  function watchLost() {
    if (lostTimer) return; // ya está contando (los reintentos no lo reinician)
    lostTimer = setTimeout(async () => {
      lostTimer = 0;
      if (S.connected || S.bye || !$("#scan").hidden) return;
      let state = "off";
      try {
        const r = await fetch(base + "alive?t=" + Date.now(), { cache: "no-store" });
        state = r.ok ? "ok" : r.status === 404 ? "gone" : "off";
      } catch { state = "off"; }
      if (S.connected || S.bye || state === "ok") return;
      if (state === "gone") {
        notice("🔗", "Este enlace ya no vale", "En el PC se ha cambiado el enlace de TurboCAM. Escanea el código nuevo de su ventana.", "Escanear el código nuevo", openScan);
      } else {
        notice("📡", "No encuentro el PC", "TurboCAM no responde en este enlace. Si lo has vuelto a abrir en el PC, el enlace de Cloudflare es otro: escanea el código nuevo. Si no, sigo intentándolo solo.",
          "Escanear el código nuevo", openScan, "Seguir esperando", watchLost);
      }
    }, everConnected ? 15000 : 6000);
  }

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
  watchLost();
  render();
  wake();
  // Si el permiso ya se dio otra vez, se empieza sin tocar nada.
  if (navigator.permissions && navigator.permissions.query) {
    navigator.permissions.query({ name: "camera" }).then((p) => { if (p.state === "granted") startCamera(); }).catch(() => {});
  }
})();
