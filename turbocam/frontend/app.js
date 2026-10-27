"use strict";
(() => {
  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => Array.from(el.querySelectorAll(s));

  // ---------- puente con Go (o datos de muestra en el navegador) ----------
  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const api = hasGo ? window.go.main.App : mockApi();
  const rt = window.runtime || mockRuntime();

  const I = {
    ok: '<path d="M20 6 9 17l-5-5"/>',
    err: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5"/><path d="M12 16.5v.01"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 7.5v.01"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    redo: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
    cam: '<path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/>',
    phone: '<rect x="6" y="2" width="12" height="20" rx="2.5"/><path d="M11 18h2"/>',
    screen: '<rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8"/><path d="M12 17v4"/>',
    more: '<circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/>',
    edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z"/>',
    out: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
    bat: '<rect x="2" y="7" width="17" height="10" rx="2"/><path d="M22 11v2"/>',
    bolt: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
    wifi: '<path d="M5 12.55a11 11 0 0 1 14.08 0"/><path d="M1.42 9a16 16 0 0 1 21.16 0"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><path d="M12 20h.01"/>',
    globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    plug: '<path d="M12 22v-5"/><path d="M9 8V2"/><path d="M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8z"/>',
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
    layers: '<path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5"/><path d="m2 12 10 5 10-5"/>',
    save: '<path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><path d="M17 21v-8H7v8"/><path d="M7 3v5h8"/>',
    play: '<path d="m6 3 14 9-14 9z"/>',
    crop: '<path d="M6 2v14a2 2 0 0 0 2 2h14"/><path d="M18 22V8a2 2 0 0 0-2-2H2"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${I[n]}</svg>`;
  const esc = (s) => String(s == null ? "" : s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const pct = (v) => Math.round(v * 100) + "%";

  const S = { st: null, tab: "connect", openAdj: null, editing: false, rendering: false };

  // ---------- ventana ----------
  $("#wb-min").onclick = () => rt.WindowMinimise();
  $("#wb-max").onclick = () => rt.WindowToggleMaximise();
  $("#wb-close").onclick = () => rt.Quit();
  $("#wb-settings").onclick = () => openSettings();
  $("#wb-float").onclick = () => api.SetWindowMode(true);
  $("#fb-exit").onclick = () => api.SetWindowMode(false);
  $("#fb-top").onclick = () => api.SetOnTop(!S.st.settings.onTop);
  $("#fb-freeze").onclick = () => api.ToggleFreeze();
  $("#fb-cover").onclick = () => api.ToggleCover();
  $(".titlebar").addEventListener("dblclick", (e) => { if (!e.target.closest("button,.pill")) rt.WindowToggleMaximise(); });

  // La vista previa solo se pide mientras la ventana se ve (y el móvil baja el ritmo si no).
  function onVisibility() {
    const v = document.visibilityState === "visible";
    api.SetVisible(v);
    previews(v);
  }
  document.addEventListener("visibilitychange", onVisibility);

  // ---------- acciones ----------
  $("#a-photo").onclick = photo;
  $("#a-freeze").onclick = () => api.ToggleFreeze();
  $("#a-cover").onclick = () => api.ToggleCover();
  $("#a-photos").onclick = () => api.OpenPhotos();

  async function photo() {
    const b = $("#a-photo");
    b.classList.remove("flash"); void b.offsetWidth; b.classList.add("flash");
    try {
      const p = await api.TakePhoto();
      photoToast(p);
    } catch (e) { toast("error", "No se pudo hacer la foto: " + errText(e)); }
  }

  function photoToast(path) {
    const name = path.split(/[\\/]/).pop();
    toast("ok", `Foto guardada: <b>${esc(name)}</b>`, { label: "Ver", run: () => api.RevealFile(path) });
  }

  // ---------- pestañas ----------
  $("#tabs").addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (!b) return;
    S.tab = b.dataset.t;
    render();
  });

  // ---------- estado ----------
  rt.EventsOn("state", (st) => { S.st = st; render(); });
  rt.EventsOn("toast", (t) => toast(t.kind === "error" ? "error" : "info", esc(t.msg)));
  rt.EventsOn("photo", (p) => photoToast(p));
  rt.EventsOn("close-request", () => closeDialog());

  function render() {
    const st = S.st;
    if (!st) return;
    document.body.classList.toggle("float", st.windowMode);
    renderPills(st);
    renderStage(st);
    renderActions(st);
    renderScene(st);
    $$("#tabs button").forEach((b) => b.classList.toggle("on", b.dataset.t === S.tab));
    $("#n-phones").textContent = st.engine.phones.length || "";
    $$(".pane").forEach((p) => p.classList.toggle("on", p.id === "p-" + S.tab));
    // No se repinta un panel mientras se arrastra un control o se escribe en él.
    const active = document.activeElement;
    const busy = active && active.closest && active.closest(".pane") && (active.matches("input,select") || S.dragging);
    if (!busy) {
      if (S.tab === "connect") renderConnect(st);
      if (S.tab === "phones") renderPhones(st);
      if (S.tab === "profiles") renderProfiles(st);
    } else if (S.tab === "phones") {
      updatePhoneStats(st);
    }
    if (!st.settings.welcomed && !S.welcomeShown) { S.welcomeShown = true; welcome(); }
  }

  function renderPills(st) {
    const e = st.engine, apps = uniqueApps(e.consumers);
    let h = "";
    if (apps.length) h += `<span class="pill live" title="Apps que están usando la cámara ahora mismo"><i class="dot"></i>En uso por <b>${esc(apps.join(", "))}</b></span>`;
    else h += `<span class="pill" title="Ninguna app está usando la cámara TurboCAM"><i class="dot"></i>Ninguna app usa la cámara</span>`;
    if (e.autoProfile) h += `<span class="pill prof">${ic("layers")}Perfil <b>${esc(e.autoProfile)}</b></span>`;
    if (st.camera.error) h += `<span class="pill" style="color:#fecaca;border-color:rgba(248,113,113,.4)">${ic("err")}<b>La cámara virtual no está registrada</b></span>`;
    setHTML($("#pills"), h);
  }

  function uniqueApps(cons) {
    const seen = new Set(), out = [];
    for (const c of cons || []) {
      const n = prettyApp(c.exe);
      if (!seen.has(n)) { seen.add(n); out.push(n); }
    }
    return out;
  }
  const APP_NAMES = { "discord.exe": "Discord", "chrome.exe": "Chrome", "msedge.exe": "Edge", "msedgewebview2.exe": "Edge", "firefox.exe": "Firefox", "obs64.exe": "OBS", "obs32.exe": "OBS", "zoom.exe": "Zoom", "ms-teams.exe": "Teams", "teams.exe": "Teams", "ffmpeg.exe": "ffmpeg", "skype.exe": "Skype", "slack.exe": "Slack", "brave.exe": "Brave", "opera.exe": "Opera", "whatsapp.exe": "WhatsApp", "telegram.exe": "Telegram", "vlc.exe": "VLC" };
  function prettyApp(exe) {
    const k = String(exe || "").toLowerCase();
    return APP_NAMES[k] || (exe || "?").replace(/\.exe$/i, "");
  }

  // ---------- escenario ----------
  let previewOn = false;
  function previewBase() {
    const [base, k] = (S.st && S.st.preview || "|").split("|");
    return { base, k };
  }
  function previews(on) {
    previewOn = on;
    const img = $("#out");
    const { base, k } = previewBase();
    const want = on && base ? `${base}/out?k=${k}` : "";
    if (img.dataset.src !== want) { img.dataset.src = want; img.src = want || "data:,"; }
    $$(".thumb img").forEach((t) => {
      const w = on && base ? `${base}/src/${t.dataset.id}?k=${k}` : "";
      if (t.dataset.src !== w) { t.dataset.src = w; t.src = w || "data:,"; }
    });
  }

  function renderStage(st) {
    const e = st.engine;
    const stage = $("#stage");
    stage.style.setProperty("--ar", `${e.outW} / ${e.outH}`);
    stage.style.setProperty("--arn", String(e.outW / e.outH));
    const status = e.status;
    const hasImg = status === "live" || status === "frozen" || status === "covered";
    stage.className = "stage" + (hasImg ? "" : " noimg") + " " + status;
    const badge = $("#badge");
    const B = {
      live: ["live", e.consumers.length ? "En directo" : "Listo"],
      frozen: ["frozen", "Congelada"],
      covered: ["covered", "Tapada"],
      paused: ["paused", "Móvil en pausa"],
      waiting: ["", "Sin imagen"],
    }[status] || ["", ""];
    badge.className = "badge " + B[0];
    badge.lastElementChild.textContent = B[1];
    $("#res").textContent = `${e.outW}×${e.outH} · ${hasImg && status === "live" ? Math.round(e.outFps) : e.fps} fps`;
    const title = $("#empty-title"), text = $("#empty-text");
    if (status === "paused") {
      title.textContent = "El móvil está en pausa";
      text.textContent = "Se ha bloqueado o has cambiado de app. Vuelve a TurboCAM en el móvil y la imagen seguirá sola.";
    } else if (e.phones.length && st.settings.scene.kind !== "screen") {
      title.textContent = "Esperando la imagen del móvil";
      text.textContent = "Pulsa «Empezar» en el móvil y da permiso a la cámara.";
    } else {
      title.textContent = "Esperando al móvil";
      text.textContent = "Escanea el código QR de la derecha con la cámara del móvil. No hace falta instalar nada.";
    }
    $("#fb-top").classList.toggle("on", st.settings.onTop);
    $("#fb-freeze").classList.toggle("on", e.frozen);
    $("#fb-cover").classList.toggle("on", e.covered);
    if (!previewOn && document.visibilityState === "visible") previews(true);
    else if (previewOn) previews(true);
  }

  function renderActions(st) {
    const e = st.engine, hk = st.settings.hotkeys;
    const fr = $("#a-freeze"), co = $("#a-cover");
    fr.classList.toggle("on", e.frozen);
    fr.querySelector("span").textContent = e.frozen ? "Descongelar" : "Congelar";
    co.classList.toggle("on", e.covered);
    co.classList.toggle("danger", e.covered);
    co.querySelector("span").textContent = e.covered ? "Destapar" : "Tapar";
    $("#k-photo").textContent = hk.photo || "";
    $("#k-freeze").textContent = hk.freeze || "";
    $("#k-cover").textContent = hk.cover || "";
  }

  // ---------- escena ----------
  $("#scene-kind").addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (!b) return;
    const sc = { ...S.st.settings.scene, kind: b.dataset.k };
    if (sc.kind === "pip" && !sc.pip) sc.pip = sc.main === "screen" ? firstPhone() : "screen";
    if (sc.kind === "pip" && sc.main === sc.pip) sc.main = sc.pip === "screen" ? firstPhone() : "screen";
    if (sc.kind === "phone" && sc.main === "screen") sc.main = firstPhone();
    api.SetScene(sc);
  });
  function firstPhone() { const p = S.st.engine.phones[0]; return p ? p.id : ""; }

  function sourceOptions(st, sel, withScreen) {
    let h = st.engine.phones.map((p) => `<option value="${esc(p.id)}" ${p.id === sel ? "selected" : ""}>📱 ${esc(p.name)}</option>`).join("");
    if (withScreen) h = `<option value="screen" ${sel === "screen" ? "selected" : ""}>🖥️ Pantalla</option>` + h;
    if (!h) h = `<option value="">Ningún móvil conectado</option>`;
    return h;
  }

  function renderScene(st) {
    const sc = st.settings.scene;
    $$("#scene-kind button").forEach((b) => b.classList.toggle("on", b.dataset.k === sc.kind));
    const body = $("#scene-body");
    if (document.activeElement && body.contains(document.activeElement) && document.activeElement.matches("input")) return;
    const region = sc.region && sc.region.w ? `${sc.region.w}×${sc.region.h}` : "pantalla principal entera";
    const screenCtl = `<button class="btn ghost sm" data-act="region">${ic("crop")}Elegir región</button>
      <span class="chip">${ic("screen")}${esc(region)}</span>
      <label class="field"><label class="toggle"><input type="checkbox" data-act="cursor" ${sc.cursor ? "checked" : ""}><i></i></label><span>Ratón</span></label>`;
    let h = "";
    if (sc.kind === "phone") {
      h = st.engine.phones.length > 1
        ? `<label class="field"><span>Móvil</span><select class="fsel" data-act="main">${sourceOptions(st, sc.main, false)}</select></label>`
        : `<span>${st.engine.phones.length ? "Se emite el móvil conectado." : "Conecta un móvil desde «Conectar»."} Con varios, elige aquí cuál sale.</span>`;
    } else if (sc.kind === "screen") {
      h = screenCtl;
    } else {
      const usesScreen = sc.main === "screen" || sc.pip === "screen";
      h = `<label class="field"><span>Fondo</span><select class="fsel" data-act="main">${sourceOptions(st, sc.main, true)}</select></label>
        <label class="field"><span>Encima</span><select class="fsel" data-act="pip">${sourceOptions(st, sc.pip, true)}</select></label>
        <span class="field"><span>Esquina</span><span class="corner" data-act="corner">${["tl", "tr", "bl", "br"].map((c) => `<button data-c="${c}" class="${sc.pipCorner === c ? "on" : ""}" title="${{ tl: "Arriba a la izquierda", tr: "Arriba a la derecha", bl: "Abajo a la izquierda", br: "Abajo a la derecha" }[c]}"></button>`).join("")}</span></span>
        <label class="field"><span>Tamaño</span><input type="range" min="0.15" max="0.5" step="0.01" value="${sc.pipSize}" data-act="size"></label>
        ${usesScreen ? screenCtl : ""}`;
    }
    if (st.engine.screenError && (sc.kind !== "phone")) h += `<span class="chip warn">${esc(st.engine.screenError)}</span>`;
    setHTML(body, h);
  }

  $("#scene-body").addEventListener("change", (e) => {
    const t = e.target, sc = { ...S.st.settings.scene };
    if (t.dataset.act === "main") sc.main = t.value;
    else if (t.dataset.act === "pip") sc.pip = t.value;
    else if (t.dataset.act === "cursor") sc.cursor = t.checked;
    else if (t.dataset.act === "size") sc.pipSize = Number(t.value);
    else return;
    if (sc.kind === "pip" && sc.main === sc.pip) {
      // No puede ser la misma fuente arriba y abajo: se intercambian.
      const old = S.st.settings.scene;
      if (t.dataset.act === "main") sc.pip = old.main; else sc.main = old.pip;
    }
    api.SetScene(sc);
  });
  $("#scene-body").addEventListener("click", (e) => {
    const c = e.target.closest(".corner button");
    if (c) { api.SetScene({ ...S.st.settings.scene, pipCorner: c.dataset.c }); return; }
    if (e.target.closest('[data-act="region"]')) regionPicker();
  });

  // ---------- conectar ----------
  const MODES = {
    internet: { label: "Internet", icon: "globe" },
    direct: { label: "Directa", icon: "plug" },
    lan: { label: "Wifi", icon: "wifi" },
  };

  function renderConnect(st) {
    const mode = st.settings.link, L = st.links[mode] || {};
    const statusText = linkStatus(mode, L);
    let qr;
    if (L.qr) qr = `<div class="qr-box"><img src="${L.qr}" alt="Código QR"></div>`;
    else {
      const prog = L.status === "preparing" && L.progress > 0 ? `<div class="prog"><i style="width:${Math.round(L.progress * 100)}%"></i></div>` : "";
      const spin = ["off", "error", "noupnp", "cgnat"].includes(L.status) ? "" : '<div class="spinner"></div>';
      qr = `<div class="qr-box wait"><div class="qr-wait">${spin}${prog}<span>${esc(statusText.wait)}</span></div></div>`;
    }
    const modes = Object.entries(MODES).map(([k, m]) => `<button data-mode="${k}" class="${k === mode ? "on" : ""}">${m.label}</button>`).join("");
    let tip = "";
    if (mode === "internet") tip = `<div class="tip">${ic("shield")}<span>Funciona desde cualquier sitio y sin avisos. La imagen pasa cifrada por Cloudflare: tiene algo más de retraso que la conexión directa.</span></div>`;
    else tip = `<div class="tip warn">${ic("shield")}<span>La primera vez el móvil dirá que <b>la conexión no es privada</b>: es el certificado propio de TurboCAM. Toca <b>Configuración avanzada → Continuar</b>. En iPhone, mejor usa «Internet».</span></div>`;
    if (mode === "lan") tip += `<div class="tip">${ic("wifi")}<span>Solo si el móvil está en la misma red que este PC. Es lo más rápido.</span></div>`;
    if (mode === "direct" && L.message) tip += `<div class="tip">${ic("info")}<span>${esc(L.message)}</span></div>`;

    const h = `<div class="card qr-card">
        <div class="seg qr-modes" id="modes">${modes}</div>
        ${qr}
        <div class="qr-status">${statusText.html}</div>
        ${L.url ? `<div class="qr-link"><code title="${esc(L.url)}">${esc(L.url)}</code><button class="ibtn" data-act="copy" title="Copiar el enlace">${ic("copy")}</button></div>` : ""}
        ${tip}
      </div>
      <div class="card"><div class="steps">
        <div class="step"><i>1</i><div><b>Escanea el código con el móvil</b>Con la cámara del móvil o la app de códigos QR.</div></div>
        <div class="step"><i>2</i><div><b>Pulsa «Empezar» y da permiso</b>El móvil ya está emitiendo. Ponlo en <b>modo negro</b> para que gaste menos.</div></div>
        <div class="step"><i>3</i><div><b>Elige «TurboCAM» como cámara</b>En Discord, Meet, Zoom, Teams u OBS, como cualquier webcam.</div></div>
      </div></div>
      <div class="card" style="display:flex;align-items:center;gap:10px">
        <p style="flex:1">El enlace lleva un código secreto. Si lo has compartido sin querer, cámbialo.</p>
        <button class="btn ghost sm" data-act="renew">${ic("redo")}Cambiar</button>
      </div>`;
    setHTML($("#p-connect"), h);
  }

  function linkStatus(mode, L) {
    const s = L.status;
    if (mode === "internet") {
      if (s === "online") return { html: `<b>Listo.</b> Funciona desde cualquier red.${L.message ? " " + esc(L.message) : ""}`, wait: "" };
      if (s === "checking") return { html: "Comprobando el enlace…", wait: "Comprobando el enlace…" };
      if (s === "preparing") return { html: esc(L.message), wait: "Preparando el enlace seguro (solo la primera vez)…" };
      if (s === "error") return { html: `<span style="color:var(--bad)">${esc(L.message || "Error")}</span>`, wait: "Cloudflare no responde; se reintenta solo. Mientras, prueba «Wifi» o «Directa»." };
      return { html: "Conectando con Cloudflare…", wait: "Conectando con Cloudflare…" };
    }
    if (mode === "direct") {
      if (s === "verified") return { html: "<b>Listo.</b> Tu PC responde desde Internet.", wait: "" };
      if (s === "ok" || s === "manual") return { html: "<b>Puerto abierto en el router.</b>", wait: "" };
      if (s === "searching") return { html: "Buscando el router…", wait: "Abriendo el puerto en el router (UPnP)…" };
      return { html: `<span style="color:var(--warn)">No disponible</span>`, wait: L.message || "No se puede abrir el puerto en el router. Usa «Internet»." };
    }
    if (L.url) return { html: "<b>Listo</b> para móviles en tu misma red.", wait: "" };
    return { html: "Este PC no está en ninguna red local.", wait: "Sin red local" };
  }

  $("#p-connect").addEventListener("click", async (e) => {
    const m = e.target.closest("#modes button");
    if (m) { api.SetLink(m.dataset.mode); return; }
    const a = e.target.closest("[data-act]");
    if (!a) return;
    if (a.dataset.act === "copy") {
      const L = S.st.links[S.st.settings.link];
      if (L && L.url) { await api.Copy(L.url); toast("ok", "Enlace copiado"); }
    }
    if (a.dataset.act === "renew") {
      const ok = await confirmDialog("¿Cambiar el enlace?", "Los móviles conectados se desconectarán y tendrán que escanear el código nuevo. Si alguno tenía TurboCAM instalado en la pantalla de inicio, tendrá que volver a instalarlo.", "Cambiar enlace");
      if (ok) { await api.RenewLink(); toast("ok", "Enlace cambiado"); }
    }
  });

  // ---------- móviles ----------
  const VIA = { internet: ["globe", "Cloudflare"], direct: ["plug", "Directa"], lan: ["wifi", "Wifi"], local: ["screen", "Este PC"] };
  const ROLE = { main: ["live", "En cámara"], pip: ["ind", "Recuadro"], spare: ["", "En espera"] };

  function phoneStats(p) {
    if (p.error) return "";
    if (!p.visible) return "En segundo plano";
    if (!p.live) return "Sin emitir";
    // En espera no sale en la cámara: solo manda una vista previa lenta, para que no
    // parezca que va a pocos fotogramas.
    if (p.role === "spare") return `En espera · vista previa a ${Math.round(p.fps)} fps`;
    const parts = [];
    if (p.width) parts.push(`${p.width}×${p.height}`);
    parts.push(`${Math.round(p.fps)} fps`);
    // Si la cámara del móvil da menos de lo que se pide, el cuello está ahí (casi
    // siempre poca luz), no en la red ni en TurboCAM.
    if (p.camFps > 0) {
      const want = Math.min((p.adjust && p.adjust.fps) || 30, 30);
      parts.push(`cámara ${Math.round(p.camFps)} fps` + (p.camFps < want * 0.75 ? " (¿poca luz?)" : ""));
    }
    if (p.kbps) parts.push(p.kbps > 1000 ? (p.kbps / 1000).toFixed(1).replace(".", ",") + " Mb/s" : Math.round(p.kbps) + " kb/s");
    if (p.quality) parts.push("calidad " + pct(p.quality));
    if (p.rtt) parts.push(p.rtt + " ms");
    return parts.join(" · ");
  }

  function battery(p) {
    if (p.battery < 0) return "";
    const lvl = Math.round(p.battery * 100);
    const cls = p.charging ? "ok" : lvl <= 15 ? "warn" : "";
    return `<span class="chip ${cls}" title="Batería">${ic(p.charging ? "bolt" : "bat")}${lvl}%</span>`;
  }

  function renderPhones(st) {
    const ps = st.engine.phones;
    if (!ps.length) {
      setHTML($("#p-phones"), `<div class="card empty-pane"><div class="tile">${ic("phone")}</div><b>Ningún móvil conectado</b><p style="margin-top:4px">Escanea el código QR de «Conectar». Puedes conectar varios y elegir cuál sale.</p></div>`);
      return;
    }
    const { base, k } = previewBase();
    const h = ps.map((p) => {
      const via = VIA[p.via] || VIA.lan, role = ROLE[p.role] || ROLE.spare;
      const caps = p.caps || {};
      const a = p.adjust;
      const open = S.openAdj === p.id;
      const src = previewOn && base ? `${base}/src/${p.id}?k=${k}` : "";
      const off = !p.visible ? "En pausa" : !p.live ? "Sin emitir" : "";
      return `<div class="card ph ${p.role === "main" ? "main" : ""}" data-id="${esc(p.id)}">
        <div class="ph-top">
          <div class="thumb"><img data-id="${esc(p.id)}" data-src="${esc(src)}" src="${esc(src || "data:,")}" alt="">${off ? `<div class="off">${off}</div>` : ""}</div>
          <div style="min-width:0">
            <div class="ph-name"><b title="${esc(p.model)}">${esc(p.name)}</b><button class="ibtn" data-act="rename" title="Cambiar el nombre">${ic("edit")}</button></div>
            <div class="ph-meta"><span class="chip ${role[0]}">${role[1]}</span><span class="chip" title="Conexión">${ic(via[0])}${via[1]}</span><span data-bat></span></div>
            <div class="ph-stats" data-stats></div>
          </div>
        </div>
        ${p.error ? `<div class="err-line">${esc(p.error)}</div>` : ""}
        <div class="ph-acts">
          ${p.role !== "main" ? `<button class="btn primary sm" data-act="show">${ic("play")}Poner en cámara</button>` : ""}
          <button class="btn ghost sm ${open ? "on" : ""}" data-act="adj">${ic("cam")}Imagen</button>
          <button class="btn ghost sm" data-act="kick" title="Desconectar este móvil">${ic("out")}</button>
          <button class="btn ghost sm" data-act="forget" title="Desconectar y olvidar sus ajustes">${ic("trash")}</button>
        </div>
        ${open ? adjustPanel(p, a, caps) : ""}
      </div>`;
    }).join("");
    setHTML($("#p-phones"), h);
    updatePhoneStats(st);
  }

  // Las cifras (fotogramas, velocidad, batería) cambian cada segundo: se cambia solo
  // su texto, sin rehacer el panel (si no, parpadea y se pierde lo que se toca).
  function updatePhoneStats(st) {
    for (const p of st.engine.phones) {
      const card = $(`.ph[data-id="${CSS.escape(p.id)}"]`);
      if (!card) continue;
      const stats = $("[data-stats]", card), bat = $("[data-bat]", card);
      const txt = phoneStats(p), b = battery(p);
      if (stats && stats.textContent !== txt) stats.textContent = txt;
      if (bat && bat._h !== b) { bat._h = b; bat.innerHTML = b; }
    }
  }

  function adjustPanel(p, a, caps) {
    const zmax = caps.zoom ? Math.min(caps.zoom.max, 10) : 5, zmin = caps.zoom ? caps.zoom.min : 1;
    const slider = (key, label, min, max, step, fmt) =>
      `<span>${label}</span><input type="range" data-adj="${key}" min="${min}" max="${max}" step="${step}" value="${a[key]}"><output>${fmt(a[key])}</output>`;
    const x = (v) => v.toFixed(1).replace(".", ",") + "×";
    const cams = (caps.cameras || []);
    return `<div class="adj">
      ${slider("zoom", "Zoom", zmin, zmax, 0.05, x)}
      ${a.zoom > 1 ? slider("panX", "Mover ←→", -1, 1, 0.01, (v) => Math.round(v * 100)) + slider("panY", "Mover ↑↓", -1, 1, 0.01, (v) => Math.round(v * 100)) : ""}
      ${slider("brightness", "Brillo", 0.5, 1.5, 0.01, pct)}
      ${slider("contrast", "Contraste", 0.5, 1.5, 0.01, pct)}
      ${slider("saturation", "Color", 0, 2, 0.01, pct)}
      <span>Cámara</span><div class="wide"><div class="seg mini" data-adjseg="facing"><button data-v="user" class="${a.facing === "user" ? "on" : ""}">Delantera</button><button data-v="environment" class="${a.facing === "environment" ? "on" : ""}">Trasera</button></div>
        ${caps.torch ? `<label class="field" title="Linterna"><label class="toggle"><input type="checkbox" data-adjck="torch" ${a.torch ? "checked" : ""}><i></i></label><span>Luz</span></label>` : ""}</div>
      ${cams.length > 2 ? `<span>Lente</span><div class="wide"><select class="fsel" data-adjsel="lens"><option value="">Automática</option>${cams.map((c) => `<option value="${esc(c.id)}" ${c.id === a.lens ? "selected" : ""}>${esc(c.label)}</option>`).join("")}</select></div>` : ""}
      <span>Espejo</span><div class="wide"><label class="toggle"><input type="checkbox" data-adjck="mirror" ${a.mirror ? "checked" : ""}><i></i></label></div>
      <span>Girar</span><div class="wide"><div class="seg mini" data-adjseg="rotate">${[0, 90, 180, 270].map((r) => `<button data-v="${r}" class="${a.rotate === r ? "on" : ""}">${r}°</button>`).join("")}</div></div>
      <span>Envío</span><div class="wide"><select class="fsel" data-adjsel="res" title="Resolución que manda el móvil">${[[0, "Automática"], [1080, "1080p"], [720, "720p"], [540, "540p"], [480, "480p"], [360, "360p"], [240, "240p"]].map(([v, l]) => `<option value="${v}" ${a.res === v ? "selected" : ""}>${l}</option>`).join("")}</select>
        <select class="fsel" data-adjsel="fps" title="Fotogramas por segundo que manda el móvil">${[[0, "fps del PC"], [30, "30 fps"], [24, "24 fps"], [20, "20 fps"], [15, "15 fps"], [10, "10 fps"]].map(([v, l]) => `<option value="${v}" ${a.fps === v ? "selected" : ""}>${l}</option>`).join("")}</select></div>
      <span>Encuadre</span><div class="wide"><div class="seg mini" data-adjseg="fit"><button data-v="cover" class="${a.fit === "cover" ? "on" : ""}">Rellenar</button><button data-v="contain" class="${a.fit === "contain" ? "on" : ""}">Completo</button></div></div>
      <span></span><div class="wide"><button class="btn ghost sm" data-act="reset">${ic("redo")}Volver a lo normal</button></div>
      ${caps.filters === false ? `<span></span><div class="wide" style="font-size:11.5px;color:var(--dim)">El navegador de este móvil no deja cambiar brillo ni color.</div>` : ""}
    </div>`;
  }

  function phoneByID(id) { return S.st.engine.phones.find((p) => p.id === id); }
  function sendAdjust(id, patch) {
    const p = phoneByID(id);
    if (!p) return;
    p.adjust = { ...p.adjust, ...patch };
    api.SetAdjust(id, p.adjust);
  }

  $("#p-phones").addEventListener("input", (e) => {
    const t = e.target;
    if (!t.dataset.adj) return;
    S.dragging = true;
    const out = t.nextElementSibling;
    const v = Number(t.value);
    if (out) out.textContent = t.dataset.adj === "zoom" ? v.toFixed(1).replace(".", ",") + "×" : ["panX", "panY"].includes(t.dataset.adj) ? Math.round(v * 100) : pct(v);
    throttledAdjust(t.closest(".ph").dataset.id, { [t.dataset.adj]: v });
  });
  $("#p-phones").addEventListener("change", (e) => {
    const t = e.target, id = t.closest(".ph") && t.closest(".ph").dataset.id;
    if (!id) return;
    S.dragging = false;
    if (t.dataset.adj) sendAdjust(id, { [t.dataset.adj]: Number(t.value) });
    if (t.dataset.adjck) sendAdjust(id, { [t.dataset.adjck]: t.checked });
    if (t.dataset.adjsel) sendAdjust(id, { [t.dataset.adjsel]: ["res", "fps"].includes(t.dataset.adjsel) ? Number(t.value) : t.value });
    t.blur();
  });
  let adjTimer = 0, adjPending = null;
  function throttledAdjust(id, patch) {
    adjPending = { id, patch: { ...(adjPending && adjPending.id === id ? adjPending.patch : {}), ...patch } };
    if (adjTimer) return;
    adjTimer = setTimeout(() => { adjTimer = 0; const p = adjPending; adjPending = null; if (p) sendAdjust(p.id, p.patch); }, 80);
  }

  $("#p-phones").addEventListener("click", async (e) => {
    const card = e.target.closest(".ph");
    if (!card) return;
    const id = card.dataset.id;
    const seg = e.target.closest("[data-adjseg] button");
    if (seg) {
      const key = seg.parentElement.dataset.adjseg;
      const v = key === "rotate" ? Number(seg.dataset.v) : seg.dataset.v;
      const patch = { [key]: v };
      if (key === "facing") { patch.lens = ""; patch.zoom = 1; }
      sendAdjust(id, patch);
      return;
    }
    const a = e.target.closest("[data-act]");
    if (!a) return;
    const p = phoneByID(id);
    switch (a.dataset.act) {
      case "show": api.ShowPhone(id); break;
      case "adj": S.openAdj = S.openAdj === id ? null : id; render(); break;
      case "kick": api.KickPhone(id); break;
      case "forget":
        if (await confirmDialog("¿Olvidar este móvil?", `Se desconecta «${p ? p.name : ""}» y se borran sus ajustes de imagen. Podrá volver a conectarse con el mismo código.`, "Olvidar")) api.ForgetPhone(id);
        break;
      case "rename": {
        const name = await promptDialog("Nombre del móvil", "Así sale en la lista y en las escenas.", p ? p.name : "");
        if (name != null && name.trim()) api.RenamePhone(id, name.trim());
        break;
      }
      case "reset": sendAdjust(id, { zoom: 1, panX: 0, panY: 0, brightness: 1, contrast: 1, saturation: 1, mirror: false, rotate: 0, fit: "cover", torch: false, res: 0, fps: 0 }); break;
    }
  });

  // ---------- perfiles ----------
  function renderProfiles(st) {
    const ps = st.settings.profiles || [];
    const apps = uniqueRaw(st.engine.consumers);
    let h = `<div class="card"><h3>${ic("layers")}Perfiles por app</h3><p>Un perfil guarda la escena y los retoques de imagen. Si le asignas apps, se pone solo cuando esa app abre la cámara, y al cerrarla todo vuelve a como estaba.</p>
      <div class="new-pf" style="margin-top:12px"><input class="inp" id="pf-name" placeholder="Nombre (p. ej. Reuniones)" maxlength="40"><button class="btn primary sm" data-act="new">${ic("save")}Guardar lo de ahora</button></div></div>`;
    if (!ps.length) h += `<div class="empty-pane">Todavía no hay perfiles.</div>`;
    h += ps.map((p) => {
      const on = st.engine.autoProfile === p.name;
      const desc = sceneDesc(st, p.scene);
      const suggestions = apps.filter((a) => !p.apps.includes(a.toLowerCase()));
      return `<div class="card pf ${on ? "on" : ""}" data-id="${esc(p.id)}">
        <div class="pf-top"><b>${esc(p.name)}</b>${on ? '<span class="chip ind">Activo</span>' : ""}
          <button class="ibtn" data-act="rename" title="Cambiar el nombre">${ic("edit")}</button>
          <button class="ibtn danger" data-act="del" title="Borrar">${ic("trash")}</button></div>
        <div class="pf-desc">${esc(desc)}</div>
        <div class="apps">${p.apps.map((a) => `<span class="app-chip">${esc(prettyApp(a))}<button data-act="rmapp" data-app="${esc(a)}" title="Quitar">${ic("x")}</button></span>`).join("")}
          <input class="app-add" data-act="addapp" placeholder="+ app (discord.exe)" list="dl-apps-${esc(p.id)}">
          <datalist id="dl-apps-${esc(p.id)}">${suggestions.map((a) => `<option value="${esc(a)}">`).join("")}${COMMON_APPS.filter((a) => !p.apps.includes(a)).map((a) => `<option value="${a}">`).join("")}</datalist>
          ${suggestions.map((a) => `<button class="chip" data-act="quickapp" data-app="${esc(a)}" style="cursor:pointer">+ ${esc(prettyApp(a))}</button>`).join("")}
        </div>
        <div class="pf-acts"><button class="btn ghost sm" data-act="apply">${ic("play")}Aplicar ahora</button><button class="btn ghost sm" data-act="over" title="Guardar en este perfil la escena y los retoques de ahora">${ic("save")}Actualizar con lo de ahora</button></div>
      </div>`;
    }).join("");
    setHTML($("#p-profiles"), h);
  }
  const COMMON_APPS = ["discord.exe", "chrome.exe", "msedge.exe", "obs64.exe", "zoom.exe", "ms-teams.exe", "firefox.exe", "slack.exe", "whatsapp.exe"];
  function uniqueRaw(cons) { return [...new Set((cons || []).map((c) => c.exe))]; }

  function sceneDesc(st, sc) {
    const name = (id) => id === "screen" ? "la pantalla" : (st.engine.phones.find((p) => p.id === id) || {}).name || "el móvil";
    if (sc.kind === "screen") return "Escena: la pantalla";
    if (sc.kind === "pip") return `Escena: ${name(sc.pip)} sobre ${name(sc.main)}`;
    return "Escena: " + name(sc.main);
  }

  function profileByID(id) { return (S.st.settings.profiles || []).find((p) => p.id === id); }

  $("#p-profiles").addEventListener("click", async (e) => {
    const a = e.target.closest("[data-act]");
    if (!a) return;
    if (a.dataset.act === "new") {
      const inp = $("#pf-name");
      const name = inp.value.trim() || "Perfil " + ((S.st.settings.profiles || []).length + 1);
      await api.SaveProfile(name);
      inp.value = "";
      toast("ok", `Perfil «${esc(name)}» guardado`);
      return;
    }
    const card = a.closest(".pf");
    if (!card) return;
    const p = profileByID(card.dataset.id);
    if (!p) return;
    switch (a.dataset.act) {
      case "apply": try { await api.ApplyProfile(p.id); toast("ok", `Perfil «${esc(p.name)}» aplicado`); } catch (err) { toast("error", errText(err)); } break;
      case "over": await api.OverwriteProfile(p.id); toast("ok", `Perfil «${esc(p.name)}» actualizado`); break;
      case "del": if (await confirmDialog("¿Borrar el perfil?", `«${p.name}» se borrará.`, "Borrar")) api.DeleteProfile(p.id); break;
      case "rename": {
        const n = await promptDialog("Nombre del perfil", "", p.name);
        if (n != null && n.trim()) api.UpdateProfile(p.id, n.trim(), p.apps);
        break;
      }
      case "rmapp": api.UpdateProfile(p.id, p.name, p.apps.filter((x) => x !== a.dataset.app)); break;
      case "quickapp": api.UpdateProfile(p.id, p.name, [...p.apps, a.dataset.app.toLowerCase()]); break;
    }
  });
  $("#p-profiles").addEventListener("keydown", (e) => {
    if (e.key !== "Enter") return;
    if (e.target.id === "pf-name") { $('[data-act="new"]', $("#p-profiles")).click(); return; }
    if (e.target.dataset.act !== "addapp") return;
    const card = e.target.closest(".pf"), p = profileByID(card.dataset.id);
    let v = e.target.value.trim().toLowerCase();
    if (!v) return;
    if (!v.endsWith(".exe")) v += ".exe";
    e.target.value = "";
    e.target.blur();
    if (!p.apps.includes(v)) api.UpdateProfile(p.id, p.name, [...p.apps, v]);
  });

  // ---------- ajustes ----------
  function openSettings() {
    const st = S.st, s = st.settings;
    const res = [["auto", "Automática"], ["1080", "1080p"], ["720", "720p"], ["540", "540p"], ["480", "480p"], ["360", "360p"]];
    const fps = [15, 24, 30];
    const keyRow = (act, label, desc) => `<div class="srow"><div><b>${label}</b><span class="s">${desc}</span>${st.hotkeyErrors && st.hotkeyErrors[act] ? `<span class="kerr">${esc(st.hotkeyErrors[act])}</span>` : ""}</div>
      <div class="ctl"><button class="keycap" data-key="${act}">${esc(s.hotkeys[act] || "Sin atajo")}</button><button class="ibtn" data-clear="${act}" title="Quitar el atajo">${ic("x")}</button></div></div>`;
    const cam = st.camera;
    const m = modal(`
      <div class="modal-head"><h2>Ajustes</h2><button class="ibtn" data-close>${ic("x")}</button></div>
      <div class="modal-body">
        <div class="sec">Imagen</div>
        <div class="srow"><div><b>Resolución</b><span class="s">En «Automática», la que pida la app que usa la cámara (Discord, Meet…).</span></div>
          <div class="ctl"><div class="seg" id="s-res">${res.map(([v, l]) => `<button data-v="${v}" class="${s.resolution === v ? "on" : ""}">${l}</button>`).join("")}</div></div></div>
        <div class="srow"><div><b>Fotogramas por segundo</b><span class="s">Menos fotogramas, menos batería y menos datos.</span></div>
          <div class="ctl"><div class="seg" id="s-fps">${fps.map((v) => `<button data-v="${v}" class="${s.fps === v ? "on" : ""}">${v}</button>`).join("")}</div></div></div>
        <div class="srow"><div><b>Ahorrar batería cuando nadie mira</b><span class="s">Si ninguna app usa la cámara y esta ventana no se ve, el móvil manda 1 imagen por segundo.</span></div>
          <div class="ctl"><label class="toggle"><input type="checkbox" id="s-idle" ${s.idle ? "checked" : ""}><i></i></label></div></div>

        <div class="sec">Atajos de teclado (funcionan desde cualquier programa)</div>
        ${keyRow("photo", "Hacer una foto", "Se guarda a la resolución del móvil.")}
        ${keyRow("freeze", "Congelar la imagen", "La cámara se queda con la última imagen.")}
        ${keyRow("cover", "Tapar la cámara", "Sale una tarjeta en vez de la imagen.")}

        <div class="sec">Fotos</div>
        <div class="srow"><div><b>Carpeta</b><span class="s path" title="${esc(st.photoDir)}">${esc(st.photoDir)}</span></div>
          <div class="ctl"><button class="btn ghost sm" id="s-photodir">Cambiar…</button></div></div>

        <div class="sec">Conexión</div>
        <div class="srow"><div><b>Abrir el puerto en el router (UPnP)</b><span class="s">Para la conexión «Directa» desde fuera de casa. Se cierra al salir.</span></div>
          <div class="ctl"><label class="toggle"><input type="checkbox" id="s-upnp" ${s.upnp ? "checked" : ""}><i></i></label></div></div>
        <div class="srow" style="display:block"><div><b>Dominio propio de Cloudflare (opcional)</b><span class="s">Con un túnel con nombre el enlace no cambia nunca y se puede instalar como app en el móvil. Pega el token del túnel y el nombre público.</span></div>
          <div class="dom"><input class="inp mono" id="s-cftoken" placeholder="Token del túnel" value="${s.cfToken ? "••••••••••••" : ""}">
          <div style="display:flex;gap:8px"><input class="inp mono" id="s-cfhost" placeholder="cam.tudominio.com" value="${esc(s.cfHost)}"><button class="btn ghost sm" id="s-cfsave">Guardar</button></div></div></div>

        <div class="sec">Cámara virtual</div>
        <div class="srow"><div><b>${cam.registered && !cam.error ? "Registrada para este usuario" : "No está registrada"}</b><span class="s">${esc(cam.error || "Aparece como «TurboCAM» en las apps. No hace falta ser administrador.")}</span></div>
          <div class="ctl"><button class="btn ghost sm" id="s-recam">${ic("redo")}Reparar</button></div></div>
      </div>
      <div class="modal-foot"><span class="ver">TurboCAM ${esc(st.version)}</span><button class="btn primary" data-close>Hecho</button></div>`);

    $("#s-res", m).onclick = (e) => { const b = e.target.closest("button"); if (b) { api.SetOutput(b.dataset.v, S.st.settings.fps); segOn(b); } };
    $("#s-fps", m).onclick = (e) => { const b = e.target.closest("button"); if (b) { api.SetOutput(S.st.settings.resolution, Number(b.dataset.v)); segOn(b); } };
    $("#s-idle", m).onchange = (e) => api.SetIdle(e.target.checked);
    $("#s-upnp", m).onchange = (e) => api.SetUPnP(e.target.checked);
    $("#s-photodir", m).onclick = async () => {
      const d = await api.PickPhotoDir();
      if (d) $(".path", m).textContent = d;
    };
    $("#s-cfsave", m).onclick = () => {
      const tok = $("#s-cftoken", m).value.trim(), host = $("#s-cfhost", m).value.trim();
      api.SetDomain(tok.startsWith("•") ? "•" : tok, host);
      toast("ok", tok || host ? "Dominio guardado: conectando…" : "Dominio quitado");
    };
    $("#s-recam", m).onclick = async () => {
      try { await api.ReinstallCamera(); toast("ok", "Cámara virtual registrada de nuevo"); }
      catch (e) { toast("error", errText(e)); }
    };
    for (const b of $$("[data-clear]", m)) b.onclick = async () => {
      await api.SetHotkey(b.dataset.clear, "");
      $(`[data-key="${b.dataset.clear}"]`, m).textContent = "Sin atajo";
    };
    for (const b of $$("[data-key]", m)) b.onclick = () => recordKey(b);
  }

  function segOn(b) { for (const x of b.parentElement.children) x.classList.toggle("on", x === b); }

  // Graba un atajo: espera a que se pulse una combinación con Ctrl, Alt, Mayús o Win.
  function recordKey(btn) {
    const act = btn.dataset.key, before = btn.textContent;
    btn.classList.add("rec");
    btn.textContent = "Pulsa la combinación…";
    const done = () => { btn.classList.remove("rec"); window.removeEventListener("keydown", onKey, true); };
    async function onKey(e) {
      e.preventDefault();
      e.stopPropagation();
      if (e.key === "Escape") { btn.textContent = before; done(); return; }
      if (["Control", "Alt", "Shift", "Meta"].includes(e.key)) return;
      let key = "";
      if (/^Key[A-Z]$/.test(e.code)) key = e.code.slice(3);
      else if (/^Digit\d$/.test(e.code)) key = e.code.slice(5);
      else if (/^F\d{1,2}$/.test(e.code)) key = e.code;
      else if (e.code === "Space") key = "Espacio";
      else if (e.code === "Pause") key = "Pausa";
      if (!key) return;
      const mods = [e.ctrlKey && "Ctrl", e.altKey && "Alt", e.shiftKey && "Mayús", e.metaKey && "Win"].filter(Boolean);
      if (!mods.length) { btn.textContent = "Añade Ctrl, Alt o Mayús"; return; }
      const combo = [...mods, key].join("+");
      done();
      try {
        const warn = await api.SetHotkey(act, combo);
        btn.textContent = combo;
        if (warn) toast("error", esc(warn));
      } catch (err) { btn.textContent = before; toast("error", errText(err)); }
    }
    window.addEventListener("keydown", onKey, true);
  }

  // ---------- región de la pantalla ----------
  async function regionPicker() {
    const mons = await api.Monitors();
    if (!mons || !mons.length) { toast("error", "No se encuentra ninguna pantalla"); return; }
    const cur = S.st.settings.scene.region;
    let mon = mons.find((m) => cur && cur.w && cur.x >= m.x && cur.x < m.x + m.w && cur.y >= m.y && cur.y < m.y + m.h) || mons.find((m) => m.primary) || mons[0];
    let sel = cur && cur.w ? { ...cur } : { x: mon.x, y: mon.y, w: mon.w, h: mon.h };
    const m = modal(`
      <div class="modal-head"><h2>Elegir región de la pantalla</h2><button class="ibtn" data-close>${ic("x")}</button></div>
      <div class="modal-body">
        <p style="margin:0 0 10px;color:var(--muted);font-size:12.5px">Arrastra sobre la imagen para marcar el trozo que quieres emitir. Se ajusta solo a la forma de la cámara.</p>
        <div class="mon-tabs">${mons.map((x) => `<button class="btn ghost sm ${x.index === mon.index ? "on" : ""}" data-mon="${x.index}">${ic("screen")}${esc(x.name)} · ${x.w}×${x.h}${x.primary ? " (principal)" : ""}</button>`).join("")}</div>
        <div class="picker" id="picker"><div class="loading"><div class="spinner"></div></div></div>
      </div>
      <div class="modal-foot"><button class="btn ghost" id="r-full">Pantalla entera</button><span class="grow"></span><button class="btn ghost" data-close>Cancelar</button><button class="btn primary" id="r-ok">Usar esta región</button></div>`, "wide");
    const picker = $("#picker", m);

    async function load() {
      picker.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
      try {
        const url = await api.MonitorShot(mon.index);
        picker.innerHTML = `<img src="${url}" alt=""><div class="sel"><span></span></div>`;
        draw();
      } catch (e) { picker.innerHTML = `<div class="loading">${esc(errText(e))}</div>`; }
    }
    function draw() {
      const box = $(".sel", picker);
      if (!box) return;
      const k = picker.clientWidth / mon.w;
      box.style.left = (sel.x - mon.x) * k + "px";
      box.style.top = (sel.y - mon.y) * k + "px";
      box.style.width = sel.w * k + "px";
      box.style.height = sel.h * k + "px";
      box.firstElementChild.textContent = `${sel.w}×${sel.h}`;
    }
    $$("[data-mon]", m).forEach((b) => b.onclick = () => {
      mon = mons.find((x) => x.index === Number(b.dataset.mon));
      sel = { x: mon.x, y: mon.y, w: mon.w, h: mon.h };
      $$("[data-mon]", m).forEach((x) => x.classList.toggle("on", x === b));
      load();
    });
    $("#r-full", m).onclick = () => { sel = { x: mon.x, y: mon.y, w: mon.w, h: mon.h }; draw(); };
    $("#r-ok", m).onclick = () => {
      api.SetScene({ ...S.st.settings.scene, region: { x: sel.x, y: sel.y, w: sel.w & ~1, h: sel.h & ~1 } });
      close(m);
    };
    // Arrastrar: la región guarda la forma de la salida (si no, saldría con bandas).
    picker.addEventListener("pointerdown", (e) => {
      if (!$("img", picker)) return;
      const r = picker.getBoundingClientRect(), k = mon.w / r.width;
      const ar = S.st.engine.outW / S.st.engine.outH;
      const x0 = (e.clientX - r.left) * k, y0 = (e.clientY - r.top) * k;
      picker.setPointerCapture(e.pointerId);
      const move = (ev) => {
        let x1 = Math.max(0, Math.min(mon.w, (ev.clientX - r.left) * k));
        let y1 = Math.max(0, Math.min(mon.h, (ev.clientY - r.top) * k));
        let w = Math.abs(x1 - x0), h = Math.abs(y1 - y0);
        if (w / Math.max(h, 1) > ar) h = w / ar; else w = h * ar;
        w = Math.min(w, mon.w); h = Math.min(h, mon.h);
        let x = x1 < x0 ? x0 - w : x0, y = y1 < y0 ? y0 - h : y0;
        x = Math.max(0, Math.min(mon.w - w, x)); y = Math.max(0, Math.min(mon.h - h, y));
        if (w >= 64 && h >= 36) { sel = { x: Math.round(mon.x + x), y: Math.round(mon.y + y), w: Math.round(w), h: Math.round(h) }; draw(); }
      };
      const up = () => { picker.removeEventListener("pointermove", move); picker.removeEventListener("pointerup", up); };
      picker.addEventListener("pointermove", move);
      picker.addEventListener("pointerup", up);
    });
    window.addEventListener("resize", draw);
    load();
  }

  // ---------- diálogos ----------
  function modal(html, cls) {
    const root = $("#modal-root");
    const m = document.createElement("div");
    m.className = "modal";
    m.innerHTML = `<div class="modal-card ${cls || ""}">${html}</div>`;
    root.appendChild(m);
    m.addEventListener("click", (e) => {
      if (e.target === m || e.target.closest("[data-close]")) close(m);
    });
    m.addEventListener("keydown", (e) => { if (e.key === "Escape") close(m); });
    setTimeout(() => { const f = $("input,button.primary", m); if (f) f.focus(); }, 30);
    return m;
  }
  function close(m) { if (m && m.parentNode) { m.remove(); if (m._done) m._done(); } }

  function confirmDialog(title, text, okLabel) {
    return new Promise((resolve) => {
      let ok = false;
      const m = modal(`<div class="modal-head"><h2>${esc(title)}</h2></div><div class="modal-body"><p style="margin:0;color:var(--muted)">${esc(text)}</p></div>
        <div class="modal-foot"><button class="btn ghost" data-close>Cancelar</button><button class="btn danger-solid" id="ok">${esc(okLabel)}</button></div>`);
      m._done = () => resolve(ok);
      $("#ok", m).onclick = () => { ok = true; close(m); };
    });
  }

  function promptDialog(title, text, value) {
    return new Promise((resolve) => {
      let v = null;
      const m = modal(`<div class="modal-head"><h2>${esc(title)}</h2></div><div class="modal-body">${text ? `<p style="margin:0 0 10px;color:var(--muted)">${esc(text)}</p>` : ""}<input class="inp" id="pv" maxlength="40" value="${esc(value)}" style="width:100%"></div>
        <div class="modal-foot"><button class="btn ghost" data-close>Cancelar</button><button class="btn primary" id="ok">Guardar</button></div>`);
      m._done = () => resolve(v);
      const inp = $("#pv", m);
      setTimeout(() => { inp.focus(); inp.select(); }, 40);
      inp.addEventListener("keydown", (e) => { if (e.key === "Enter") $("#ok", m).click(); });
      $("#ok", m).onclick = () => { v = inp.value; close(m); };
    });
  }

  function closeDialog() {
    if ($(".modal.closing")) return;
    const e = S.st ? S.st.engine : { consumers: [], phones: [] };
    const apps = uniqueApps(e.consumers);
    const why = apps.length ? `${apps.join(", ")} está usando la cámara: dejará de verse la imagen.` : "Hay móviles conectados: se desconectarán.";
    const m = modal(`<div class="modal-head"><h2>¿Cerrar TurboCAM?</h2></div><div class="modal-body"><p style="margin:0;color:var(--muted)">${esc(why)} Si solo quieres quitarlo de en medio, minimízalo.</p></div>
      <div class="modal-foot"><button class="btn ghost" id="c-min">Minimizar</button><button class="btn danger-solid" id="c-quit">Cerrar</button></div>`);
    m.classList.add("closing");
    $("#c-min", m).onclick = () => { close(m); rt.WindowMinimise(); };
    $("#c-quit", m).onclick = () => api.Quit();
  }

  function welcome() {
    const m = modal(`<div class="modal-head"><h2>Bienvenido a TurboCAM</h2></div>
      <div class="modal-body"><div class="steps">
        <div class="step"><i>1</i><div><b>Escanea el código QR con el móvil</b>Se abre una página y el móvil empieza a emitir. Sin instalar nada.</div></div>
        <div class="step"><i>2</i><div><b>Elige «TurboCAM» en tus apps</b>Discord, Meet, Zoom, Teams, OBS y el navegador lo ven como una webcam más.</div></div>
        <div class="step"><i>3</i><div><b>Ponlo en modo negro</b>La pantalla se queda negra para gastar poco, pero sigue emitiendo. Mejor si está enchufado.</div></div>
      </div>
      <div class="tip" style="margin-top:16px">${ic("info")}<span>Un navegador no puede grabar con el móvil bloqueado: TurboCAM lo mantiene despierto. Si cambias de app, se pausa y sigue solo al volver.</span></div></div>
      <div class="modal-foot"><button class="btn primary" data-close>Entendido</button></div>`);
    m._done = () => api.SetWelcomed();
  }

  // ---------- avisos ----------
  function toast(kind, html, action) {
    const t = document.createElement("div");
    t.className = "toast " + kind;
    t.innerHTML = ic(kind === "ok" ? "ok" : kind === "error" ? "err" : "info") + `<span>${html}</span>` + (action ? `<button class="btn ghost sm">${esc(action.label)}</button>` : "");
    if (action) $("button", t).onclick = () => { action.run(); dismiss(); };
    $("#toasts").appendChild(t);
    const dismiss = () => { t.classList.add("out"); setTimeout(() => t.remove(), 250); };
    setTimeout(dismiss, kind === "error" ? 6500 : 4200);
    while ($("#toasts").children.length > 4) $("#toasts").firstElementChild.remove();
  }

  function errText(e) { return String(e && e.message ? e.message : e).replace(/^Error:\s*/, ""); }

  // Cambia el HTML solo si ha cambiado (para no perder el foco ni reiniciar imágenes).
  function setHTML(el, h) {
    if (el._h === h) return;
    // Las miniaturas en vivo no se tocan si siguen ahí: se rehace solo lo demás.
    el._h = h;
    const keep = new Map($$(".thumb img", el).map((i) => [i.dataset.id + "|" + i.dataset.src, i]));
    el.innerHTML = h;
    for (const img of $$(".thumb img", el)) {
      const old = keep.get(img.dataset.id + "|" + img.dataset.src);
      if (old && img.dataset.src) img.replaceWith(old);
    }
  }

  // ---------- arranque ----------
  // Si la primera respuesta se pierde (arranque aún a medias), se vuelve a pedir.
  (function first(n) {
    let got = false;
    api.GetState().then((st) => { got = true; if (!S.st) { S.st = st; render(); onVisibility(); } }).catch(() => {});
    setTimeout(() => { if (!got && !S.st && n < 20) first(n + 1); }, 1500);
  })(0);

  // ---------- datos de muestra (solo al abrir el HTML en un navegador) ----------
  function mockRuntime() {
    const h = {};
    window.__emit = (n, d) => (h[n] || []).forEach((f) => f(d));
    return {
      EventsOn: (n, f) => { (h[n] = h[n] || []).push(f); },
      WindowMinimise() { }, WindowToggleMaximise() { }, Quit() { window.__emit("close-request"); },
    };
  }

  function mockApi() {
    const shot = (hue, label) => {
      const c = document.createElement("canvas"); c.width = 640; c.height = 360;
      const x = c.getContext("2d");
      const g = x.createLinearGradient(0, 0, 640, 360);
      g.addColorStop(0, `hsl(${hue},60%,30%)`); g.addColorStop(1, `hsl(${hue + 60},60%,18%)`);
      x.fillStyle = g; x.fillRect(0, 0, 640, 360);
      x.fillStyle = "rgba(255,255,255,.85)"; x.beginPath(); x.arc(320, 150, 62, 0, 7); x.fill();
      x.fillStyle = "rgba(255,255,255,.6)"; x.beginPath(); x.ellipse(320, 340, 150, 110, 0, 0, 7); x.fill();
      x.fillStyle = "#fff"; x.font = "600 20px Segoe UI"; x.fillText(label, 18, 34);
      return c.toDataURL("image/jpeg", .8);
    };
    const qr = (() => {
      const c = document.createElement("canvas"); c.width = c.height = 29;
      const x = c.getContext("2d"); x.fillStyle = "#fff"; x.fillRect(0, 0, 29, 29); x.fillStyle = "#000";
      for (let i = 0; i < 29; i++) for (let j = 0; j < 29; j++) if (((i * 7 + j * 13) ^ (i * j)) % 3 === 0) x.fillRect(i, j, 1, 1);
      for (const [a, b] of [[0, 0], [22, 0], [0, 22]]) { x.fillRect(a, b, 7, 7); x.fillStyle = "#fff"; x.fillRect(a + 1, b + 1, 5, 5); x.fillStyle = "#000"; x.fillRect(a + 2, b + 2, 3, 3); }
      return c.toDataURL();
    })();
    const A = { facing: "user", lens: "", zoom: 1, panX: 0, panY: 0, brightness: 1, contrast: 1, saturation: 1, mirror: false, rotate: 0, fit: "cover", torch: false, res: 0, fps: 0 };
    const st = {
      engine: {
        phones: [
          { id: "abc123def456", name: "Pixel 8", model: "Pixel 8", via: "internet", visible: true, live: true, error: "", battery: 0.82, charging: true, fps: 29.6, kbps: 4820, quality: 0.78, rtt: 64, camFps: 30, camW: 1280, camH: 720, width: 1280, height: 720, role: "main", caps: { zoom: { min: 1, max: 8, step: 0.1 }, torch: true, cameras: [{ id: "a", label: "Trasera" }, { id: "b", label: "Delantera" }, { id: "c", label: "Gran angular" }], filters: true }, adjust: { ...A }, connected: Date.now() },
          { id: "zzz999yyy888", name: "iPhone", model: "iPhone", via: "lan", visible: true, live: true, error: "", battery: 0.41, charging: false, fps: 4, kbps: 210, quality: 0.7, rtt: 12, width: 320, height: 180, role: "spare", caps: { zoom: null, torch: false, cameras: [], filters: false }, adjust: { ...A, facing: "environment" }, connected: Date.now() },
        ],
        consumers: [{ pid: 1, exe: "Discord.exe", width: 1280, height: 720, fps: 30 }],
        outW: 1280, outH: 720, fps: 30, outFps: 29.8, frozen: false, covered: false, autoProfile: "", screenError: "", status: "live",
      },
      settings: { link: "internet", resolution: "auto", fps: 30, idle: true, upnp: true, cfToken: "", cfHost: "", onTop: true, welcomed: true, scene: { kind: "phone", main: "abc123def456", pip: "", pipCorner: "br", pipSize: 0.3, region: { x: 0, y: 0, w: 0, h: 0 }, cursor: false }, hotkeys: { photo: "Ctrl+Alt+P", freeze: "Ctrl+Alt+F", cover: "Ctrl+Alt+T" }, profiles: [{ id: "p1", name: "Reuniones", scene: { kind: "phone", main: "abc123def456" }, adjust: { ...A }, apps: ["ms-teams.exe", "zoom.exe"] }] },
      links: {
        internet: { status: "online", url: "https://ejemplo-de-muestra.trycloudflare.com/c/3f2a9c…/", message: "", progress: 0, qr },
        direct: { status: "noupnp", url: "", message: "Tu router no responde por UPnP (puede estar desactivado en su configuración).", progress: 0, qr: "" },
        lan: { status: "online", url: "https://192.168.1.20:47851/c/3f2a9c…/", message: "", progress: 0, qr },
      },
      camera: { registered: true, error: "" }, hotkeyErrors: {}, preview: "", version: "1.0.0 (muestra)", pc: "PC-DE-MUESTRA", windowMode: false, photoDir: "C:\\Users\\muestra\\Pictures\\TurboCAM",
    };
    const outImg = shot(250, "Salida (muestra)");
    setTimeout(() => {
      const img = $("#out"); img.src = outImg;
      Object.defineProperty(img, "src", { set() { }, get() { return outImg; } });
    }, 50);
    const push = () => setTimeout(() => window.__emit("state", JSON.parse(JSON.stringify(st))), 30);
    // Como la app: las cifras de los móviles cambian cada segundo.
    setInterval(() => {
      for (const p of st.engine.phones) { p.fps = 28 + Math.round(Math.random() * 20) / 10; p.kbps = 2200 + Math.round(Math.random() * 400); p.rtt = 38 + Math.round(Math.random() * 8); }
      push();
    }, 1000);
    const sc = (f) => { f(st.settings.scene); push(); };
    return {
      GetState: async () => JSON.parse(JSON.stringify(st)),
      SetVisible() { }, SetLink(m) { st.settings.link = m; push(); },
      SetOutput(r, f) { st.settings.resolution = r; st.settings.fps = f; push(); },
      SetIdle(v) { st.settings.idle = v; push(); }, SetUPnP(v) { st.settings.upnp = v; push(); }, SetDomain() { },
      SetScene(s) { st.settings.scene = s; push(); }, ShowPhone(id) { sc((s) => { s.main = id; }); st.engine.phones.forEach((p) => { p.role = p.id === id ? "main" : "spare"; }); push(); },
      SetAdjust(id, a) { st.engine.phones.find((p) => p.id === id).adjust = a; push(); },
      RenamePhone(id, n) { st.engine.phones.find((p) => p.id === id).name = n; push(); },
      KickPhone(id) { st.engine.phones = st.engine.phones.filter((p) => p.id !== id); push(); }, ForgetPhone(id) { this.KickPhone(id); },
      RenewLink() { }, ToggleFreeze() { st.engine.frozen = !st.engine.frozen; st.engine.status = st.engine.frozen ? "frozen" : "live"; push(); },
      ToggleCover() { st.engine.covered = !st.engine.covered; st.engine.status = st.engine.covered ? "covered" : "live"; push(); },
      async TakePhoto() { return "C:\\Users\\muestra\\Pictures\\TurboCAM\\TurboCAM 2026-10-27 10.00.00.jpg"; },
      async SetHotkey(a, c) { st.settings.hotkeys[a] = c; push(); return ""; },
      async PickPhotoDir() { return ""; }, OpenPhotos() { }, RevealFile() { }, async Copy() { },
      async SaveProfile(n) { st.settings.profiles.push({ id: "p" + Date.now(), name: n, scene: { ...st.settings.scene }, adjust: { ...A }, apps: [] }); push(); },
      UpdateProfile(id, n, apps) { const p = st.settings.profiles.find((x) => x.id === id); p.name = n; p.apps = apps; push(); },
      OverwriteProfile() { }, DeleteProfile(id) { st.settings.profiles = st.settings.profiles.filter((p) => p.id !== id); push(); }, async ApplyProfile() { },
      async Monitors() { return [{ index: 0, name: "Pantalla 1", x: 0, y: 0, w: 2560, h: 1440, primary: true }, { index: 1, name: "Pantalla 2", x: 2560, y: 0, w: 1920, h: 1080, primary: false }]; },
      async MonitorShot() { return shot(200, "Escritorio (muestra)"); },
      async ReinstallCamera() { }, SetWindowMode(on) { st.windowMode = on; push(); }, SetOnTop(v) { st.settings.onTop = v; push(); },
      SetWelcomed() { st.settings.welcomed = true; }, Quit() { },
    };
  }
})();
