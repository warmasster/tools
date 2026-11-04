"use strict";
// Arranque, navegación, datos comunes y acciones generales. Cada vista
// (v_*.js) se apunta en `views` con su render, y si quiere enter (al entrar) y
// after (después de pintar, para los canvas).

function render() {
  const v = views[state.view];
  if (!v) return;
  morph(document.getElementById(state.view), v.render());
  if (v.after) requestAnimationFrame(v.after);
  // aviso de FPS sin permiso y medalla de la revisión
  const b = $("#banner"), l = state.live;
  const showBanner = l && !l.fps.available && state.view === "home";
  b.hidden = !showBanner;
  if (showBanner) morph(b, `${IC.warn}<div class="txt"><b>Los FPS están desactivados.</b> Windows pide permiso de administrador una vez para poder verlos. El resto funciona igual.</div><button class="btn sm amber" data-act="enablefps">Activar FPS</button>`);
  const r = state.report, badge = $("#checks-badge");
  if (r) {
    const bad = r.results.filter(x => x.status === "bad").length, warn = r.results.filter(x => x.status === "warn").length;
    badge.textContent = bad + warn ? String(bad + warn) : "";
    badge.className = "badge" + (bad ? " bad" : "");
  }
  const fb = $("#fix-badge");
  if (fb) {
    const n = (typeof fixAlerts === "function") ? fixAlerts() : 0;
    fb.textContent = n ? String(n) : "";
  }
}

function goView(name, sub) {
  if (!views[name]) name = "home";
  state.view = name;
  if (sub !== undefined) state.sub[name] = sub;
  document.querySelectorAll("[data-v]").forEach(b => b.classList.toggle("on", b.dataset.v === name));
  document.querySelectorAll(".view").forEach(x => x.classList.toggle("on", x.id === "v-" + name));
  const inner = document.getElementById(name);
  inner.classList.remove("anim"); void inner.offsetWidth; inner.classList.add("anim");
  const v = views[name];
  if (v.enter) v.enter();
  render();
  document.getElementById("v-" + name).scrollTop = 0;
}

// ---------- datos comunes ----------
async function poll() {
  try {
    state.live = await App.Live();
    state.offline = false;
    // El overlay se puede poner y quitar desde la bandeja o con el atajo: los
    // ajustes se vuelven a leer si no cuadran, y cada 10 s por si acaso.
    if (state.settings && (state.live.agent.overlay !== state.settings.overlay.on || Date.now() - (poll.last || 0) > 10000)) {
      poll.last = Date.now();
      state.settings = await App.Settings();
    }
  } catch (e) {
    state.offline = true;
  }
  if (!document.hidden) render();
  setTimeout(poll, 1000);
}
async function loadSettings() { try { state.settings = await App.Settings(); } catch (e) { } render(); }
async function loadSessions() {
  try {
    state.sessions = await App.Sessions();
  } catch (e) { state.sessions = state.sessions || []; }
  render();
}
async function runChecks() {
  state.checking = true; render();
  try { state.report = await App.Checks(); } catch (e) { toast(errMsg(e), true); }
  state.checking = false; render();
}
async function saveSettings(mut) {
  const s = JSON.parse(JSON.stringify(state.settings));
  mut(s);
  state.settings = s; render();
  try { state.settings = await App.SaveSettings(s); } catch (e) { toast(errMsg(e), true); }
  render();
}

// ---------- acciones generales ----------
Object.assign(actions, {
  go: (a, el) => goView(a, el && el.dataset.sub),
  sub: (a, el) => { state.sub[state.view] = a; render(); const v = views[state.view]; if (v && v.subEnter) v.subEnter(a); },
  overlay: async () => {
    const on = !state.settings.overlay.on;
    state.settings.overlay.on = on; render();
    try { state.settings = await App.SetOverlay(on); } catch (e) { toast(errMsg(e), true); }
    toast(on ? "Overlay puesto (Ctrl+Alt+O para quitarlo)" : "Overlay quitado");
    render();
  },
  ovmode: a => saveSettings(s => { s.overlay.fpsOnly = a === "fps"; s.overlay.on = true; }),
  ovflag: a => saveSettings(s => { s.overlay[a] = !s.overlay[a]; }),
  corner: a => saveSettings(s => { s.overlay.corner = a; s.overlay.on = true; }),
  flag: a => saveSettings(s => { s[a] = !s[a]; }),
  fix: async id => {
    state.busy[id] = true; render();
    try { state.report = await App.Fix(id); toast("Arreglado"); } catch (e) { toast("No se pudo: " + errMsg(e), true); }
    delete state.busy[id]; render();
  },
  howto: id => { state.open.add(id); goView("checks"); setTimeout(() => { const el = document.querySelector(`.chk[data-key="${CSS.escape(id)}"]`); el && el.scrollIntoView({ block: "center", behavior: "smooth" }); }, 50); },
  toggle: id => { state.open.has(id) ? state.open.delete(id) : state.open.add(id); render(); },
  recheck: () => runChecks(),
  enablefps: async () => {
    toast("Windows te va a pedir permiso…");
    try { await App.EnableFPS(); toast("FPS activados"); } catch (e) { toast(errMsg(e), true); }
  },
  quitall: async () => {
    if (!await confirmBox("¿Cerrar TurboMON del todo?", "Se quitan el overlay y el icono de la bandeja, y deja de apuntar partidas hasta que lo vuelvas a abrir (o reinicies).", "Cerrar del todo", true)) return;
    App.QuitAgent();
  },
});

document.addEventListener("click", e => {
  const nb = e.target.closest("[data-v]");
  if (nb && !nb.dataset.act) return goView(nb.dataset.v);
  const b = e.target.closest("[data-act]");
  if (!b || b.disabled) return;
  const f = actions[b.dataset.act];
  if (f) f(b.dataset.arg, b, e);
});
document.addEventListener("input", e => {
  const k = e.target.dataset && e.target.dataset.range;
  if (!k || !state.settings) return;
  state.settings.overlay[k] = +e.target.value;
  render(); // la vista previa cambia mientras se arrastra
});
document.addEventListener("change", e => {
  const k = e.target.dataset && e.target.dataset.range;
  if (k) saveSettings(s => { s.overlay[k] = +e.target.value; });
});
$("#wb-min").onclick = () => RT.WindowMinimise();
$("#wb-max").onclick = () => RT.WindowToggleMaximise();
$("#wb-close").onclick = () => RT.Quit();
window.addEventListener("resize", () => { const v = views[state.view]; if (v && v.after) requestAnimationFrame(v.after); });
document.addEventListener("visibilitychange", () => !document.hidden && render());

loadSettings();
runChecks();
loadSessions();
poll();
goView("home");
