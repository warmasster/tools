// TurboVAULT en las páginas: un botón dentro de los campos de inicio de sesión,
// un menú con lo guardado para esta web y la oferta de guardar al enviar.
// Nada se rellena sin que lo pidas, y todo lo que se dibuja va en un shadow DOM
// cerrado, fuera del alcance de los scripts de la página.
(() => {
  if (window.__turbovault) return;
  window.__turbovault = true;

  const isTop = window === window.top;
  let sameOriginFrame = isTop;
  if (!isTop) {
    try { sameOriginFrame = window.top.location.origin === location.origin; } catch { sameOriginFrame = false; }
  }
  if (!sameOriginFrame) return; // marcos de otras webs: ni se tocan

  const bg = (msg) => new Promise((resolve, reject) => {
    try {
      chrome.runtime.sendMessage(msg, (r) => {
        if (chrome.runtime.lastError) { reject(new Error(chrome.runtime.lastError.message)); return; }
        if (!r) { resolve(null); return; }
        if (r.ok) resolve(r.data); else reject(Object.assign(new Error(r.error), { code: r.code }));
      });
    } catch (e) { reject(e); }
  });

  // ---------- clics de verdad ----------
  // La página no puede pulsar por ti: solo cuentan eventos reales (isTrusted)
  // sobre botones que llevan un momento totalmente visibles. Con la
  // IntersectionObserver v2 Chrome dice si algo los tapa, los vuelve
  // transparentes o los deforma (lo que haría una web para engañarte).
  let lastGesture = 0;
  for (const t of ['pointerdown', 'keydown']) addEventListener(t, (e) => { if (e.isTrusted) lastGesture = Date.now(); }, true);
  const visibleSince = new WeakMap();
  let io = null;
  function watch(el) {
    if (!io) {
      try {
        io = new IntersectionObserver((entries) => {
          for (const en of entries) {
            if (en.isVisible && en.intersectionRatio >= 0.6) { if (!visibleSince.has(en.target)) visibleSince.set(en.target, performance.now()); }
            else visibleSince.delete(en.target);
          }
        }, { trackVisibility: true, delay: 100, threshold: [0, 0.6, 1] });
      } catch { return; }
    }
    io.observe(el);
  }
  function realClick(e, el) {
    if (!e.isTrusted) return false;
    const t = visibleSince.get(el);
    return t !== undefined && performance.now() - t >= 350;
  }
  function onRealClick(el, fn) {
    watch(el);
    el.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (realClick(e, el)) fn();
    });
  }

  // ---------- campos ----------
  const USER_RE = /user|e-?mail|correo|usuario|login|account|cuenta|identifier|ident|dni|nif|phone|m[oó]vil|tel[eé]fono/i;
  const OTP_RE = /otp|totp|2fa|mfa|one.?time|verif|c[oó]digo|security.?code|auth.?code|token/i;
  const SIGNUP_RE = /registr|crear (una )?cuenta|create (an )?account|sign ?up|join|reg[ií]strate|alta/i;
  const SUBMIT_RE = /iniciar|entrar|acceder|log ?in|sign ?in|continuar|continue|siguiente|next|enviar|submit|registr|crear|create|sign ?up|guardar|save|cambiar|change|verif|confirm/i;

  const textTypes = new Set(['text', 'email', 'tel', '']);

  function visible(el) {
    if (!el || !el.isConnected || el.disabled) return false;
    const r = el.getBoundingClientRect();
    if (r.width < 24 || r.height < 12) return false;
    const cs = getComputedStyle(el);
    return cs.visibility !== 'hidden' && cs.display !== 'none' && parseFloat(cs.opacity || '1') > 0.05;
  }
  const attrs = (el) => [el.name, el.id, el.getAttribute('placeholder'), el.getAttribute('aria-label'), el.getAttribute('autocomplete')].join(' ');
  const scopeOf = (el) => el.form || el.closest('form') || document;
  const pwFields = (scope, onlyVisible = true) => [...scope.querySelectorAll('input[type=password]')].filter((x) => (onlyVisible ? visible(x) : x.isConnected));

  function isSignup(el) {
    const scope = scopeOf(el);
    if (scope.querySelector('input[autocomplete~="new-password"]')) return true;
    if (pwFields(scope).length >= 2) return true;
    const text = scope === document ? document.title : (scope.innerText || '').slice(0, 2000);
    return SIGNUP_RE.test(text) && !/iniciar sesi[oó]n|log ?in|sign ?in/i.test(text.slice(0, 200));
  }

  function kindOf(el) {
    if (!el || el.tagName !== 'INPUT' || el.readOnly) return null;
    const type = (el.getAttribute('type') || 'text').toLowerCase();
    const ac = (el.getAttribute('autocomplete') || '').toLowerCase();
    if (type === 'password') {
      if (ac.includes('new-password') || isSignup(el)) return 'newpassword';
      return 'password';
    }
    if (!textTypes.has(type)) return null;
    if (ac.includes('one-time-code')) return 'otp';
    const a = attrs(el);
    const ml = el.maxLength;
    if (OTP_RE.test(a) && ml >= 4 && ml <= 10) return 'otp';
    if (ac.includes('username') || ac === 'email' || type === 'email') return 'username';
    if (USER_RE.test(a)) {
      const scope = scopeOf(el);
      if (pwFields(scope).length || /login|signin|sign-in|identifier|username|user/i.test(a)) return 'username';
    }
    return null;
  }

  // El campo de usuario de un formulario: el último de texto antes de la contraseña.
  function usernameFor(pw) {
    const scope = scopeOf(pw);
    const inputs = [...scope.querySelectorAll('input')].filter((x) => visible(x) && textTypes.has((x.getAttribute('type') || 'text').toLowerCase()) && !x.readOnly);
    const ac = inputs.find((x) => /username|email/.test(x.getAttribute('autocomplete') || ''));
    if (ac) return ac;
    let best = null;
    for (const x of inputs) {
      if (pw.compareDocumentPosition(x) & Node.DOCUMENT_POSITION_PRECEDING) {
        if (!OTP_RE.test(attrs(x)) && !/search|buscar|captcha/i.test(attrs(x))) best = x;
      }
    }
    return best || inputs.find((x) => kindOf(x) === 'username') || null;
  }

  function setValue(el, value) {
    if (!el) return;
    el.focus({ preventScroll: true });
    const proto = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value');
    proto.set.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    el.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true, key: 'Unidentified' }));
  }

  // ---------- interfaz (shadow DOM cerrado) ----------
  const LOGO = '<svg viewBox="0 0 64 64"><defs><linearGradient id="g" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#8b5cf6"/><stop offset=".5" stop-color="#6366f1"/><stop offset="1" stop-color="#06b6d4"/></linearGradient></defs><rect x="2" y="2" width="60" height="60" rx="15" fill="url(#g)"/><path d="M22.5 29v-6.5a9.5 9.5 0 0 1 19 0V29" fill="none" stroke="#fff" stroke-width="5.2" stroke-linecap="round"/><rect x="15" y="27" width="34" height="25" rx="6.5" fill="#fff"/><circle cx="32" cy="37.5" r="3.9" fill="#4f46e5"/><rect x="30.2" y="38.5" width="3.6" height="8" rx="1.8" fill="#4f46e5"/></svg>';
  const CSS = `
    :host { all: initial; }
    * { box-sizing: border-box; font-family: "Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif; }
    .ic { position: fixed; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 7px; background: transparent; cursor: pointer; display: none; z-index: 2147483647; opacity: .92; transition: transform .12s, opacity .12s; }
    .ic:hover { opacity: 1; transform: scale(1.08); }
    .ic svg { width: 24px; height: 24px; display: block; filter: drop-shadow(0 2px 6px rgba(79,70,229,.45)); }
    .menu { position: fixed; z-index: 2147483647; width: 320px; max-height: 360px; overflow: auto; display: none; padding: 6px; border-radius: 14px; background: #12141f; color: #eceef8; border: 1px solid rgba(255,255,255,.12); box-shadow: 0 24px 60px -12px rgba(0,0,0,.75); font-size: 13px; line-height: 1.4; animation: in .14s ease both; }
    @keyframes in { from { opacity: 0; transform: translateY(-4px); } }
    .hd { display: flex; align-items: center; gap: 8px; padding: 6px 8px 8px; font-size: 11.5px; color: #9196ae; }
    .hd svg { width: 16px; height: 16px; }
    .hd b { color: #eceef8; font-weight: 700; }
    .it { width: 100%; display: flex; align-items: center; gap: 10px; padding: 8px; border: 0; border-radius: 10px; background: transparent; color: inherit; cursor: pointer; text-align: left; font-size: 13px; }
    .it:hover, .it:focus-visible { background: rgba(255,255,255,.07); outline: none; }
    .av { width: 30px; height: 30px; border-radius: 9px; flex: none; display: grid; place-items: center; font-weight: 800; font-size: 13px; color: #fff; background: var(--c, #4f46e5); text-transform: uppercase; }
    .tx { min-width: 0; flex: 1; }
    .tx b { display: block; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .tx span { display: block; font-size: 12px; color: #9196ae; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .msg { padding: 8px 10px 10px; color: #9196ae; font-size: 12.5px; }
    .msg b { color: #eceef8; display: block; font-size: 13.5px; margin-bottom: 2px; }
    .btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; border: 0; border-radius: 10px; padding: 8px 14px; font-weight: 650; font-size: 13px; cursor: pointer; color: #fff; background: linear-gradient(135deg,#8b5cf6,#6366f1 45%,#06b6d4); box-shadow: 0 8px 20px -10px rgba(99,102,241,.9); }
    .btn.ghost { background: rgba(255,255,255,.07); box-shadow: none; color: #eceef8; border: 1px solid rgba(255,255,255,.1); }
    .row { display: flex; gap: 6px; padding: 2px 8px 8px; flex-wrap: wrap; }
    hr { border: 0; border-top: 1px solid rgba(255,255,255,.08); margin: 4px 4px; }
    .ft { width: 100%; display: flex; align-items: center; gap: 8px; padding: 7px 8px; border: 0; border-radius: 9px; background: transparent; color: #9196ae; cursor: pointer; font-size: 12.5px; }
    .ft:hover { background: rgba(255,255,255,.06); color: #eceef8; }
    .gen .av { background: linear-gradient(135deg,#10b981,#06b6d4); }
    .code { font-family: "Cascadia Mono", Consolas, monospace; font-size: 26px; font-weight: 800; letter-spacing: 4px; color: #a5b4fc; padding: 4px 10px 6px; }
    .bar { position: fixed; top: 16px; right: 16px; z-index: 2147483647; width: 340px; display: none; padding: 14px; border-radius: 16px; background: #12141f; color: #eceef8; border: 1px solid rgba(255,255,255,.12); box-shadow: 0 30px 70px -14px rgba(0,0,0,.8); font-size: 13px; animation: in .2s ease both; }
    .bar .top { display: flex; align-items: flex-start; gap: 10px; }
    .bar .top svg { width: 30px; height: 30px; flex: none; }
    .bar .tt { flex: 1; min-width: 0; }
    .bar .tt b { display: block; font-size: 14px; font-weight: 700; }
    .bar .tt span { display: block; color: #9196ae; font-size: 12.5px; margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .bar .x { border: 0; background: transparent; color: #9196ae; cursor: pointer; font-size: 18px; line-height: 1; padding: 2px 4px; border-radius: 6px; }
    .bar .x:hover { color: #fff; background: rgba(255,255,255,.08); }
    .bar .acts { display: flex; gap: 6px; margin-top: 12px; justify-content: flex-end; }
    .ok { color: #34d399; }
  `;
  const COLORS = ['#7c3aed', '#4f46e5', '#0891b2', '#059669', '#d97706', '#db2777', '#2563eb', '#9333ea', '#0d9488', '#dc2626', '#65a30d', '#c026d3'];
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const colorOf = (s) => { let h = 0; for (const c of s || '?') h = (h * 31 + c.charCodeAt(0)) >>> 0; return COLORS[h % COLORS.length]; };

  let root = null, iconEl, menuEl, barEl;
  function ui() {
    if (root) return;
    const host = document.createElement('turbovault-ui');
    host.style.cssText = 'all: initial !important; position: fixed !important; top: 0 !important; left: 0 !important; width: 0 !important; height: 0 !important; z-index: 2147483647 !important;';
    (document.body || document.documentElement).appendChild(host);
    root = host.attachShadow({ mode: 'closed' });
    root.innerHTML = `<style>${CSS}</style><button class="ic" title="TurboVAULT" tabindex="-1">${LOGO}</button><div class="menu" role="menu"></div><div class="bar" role="dialog"></div>`;
    iconEl = root.querySelector('.ic');
    menuEl = root.querySelector('.menu');
    barEl = root.querySelector('.bar');
    iconEl.addEventListener('mousedown', (e) => { e.preventDefault(); e.stopPropagation(); });
    onRealClick(iconEl, () => { if (menuOpen()) closeMenu(); else openMenu(); });
    menuEl.addEventListener('mousedown', (e) => e.preventDefault()); // que el campo no pierda el foco
  }

  let current = null; // campo con el foco
  let currentKind = null;
  let dismissed = false; // el menú automático se cerró a mano en esta página
  let hideTimer = 0;
  const menuOpen = () => menuEl && menuEl.style.display === 'block';

  function place() {
    if (!current || !root) return;
    if (!current.isConnected || !visible(current)) { hideIcon(); closeMenu(); return; }
    const r = current.getBoundingClientRect();
    const s = 24;
    iconEl.style.left = `${Math.round(r.right - s - Math.min(8, r.height / 4) - (current.type === 'password' && r.width > 160 ? 28 : 0))}px`;
    iconEl.style.top = `${Math.round(r.top + (r.height - s) / 2)}px`;
    if (menuOpen()) {
      const mh = menuEl.offsetHeight || 200;
      const below = r.bottom + 6;
      const top = below + mh > innerHeight && r.top - mh - 6 > 0 ? r.top - mh - 6 : below;
      menuEl.style.top = `${Math.round(top)}px`;
      menuEl.style.left = `${Math.round(Math.max(8, Math.min(r.left, innerWidth - 330)))}px`;
    }
  }
  let rafPending = false;
  const schedulePlace = () => { if (!rafPending) { rafPending = true; requestAnimationFrame(() => { rafPending = false; place(); }); } };
  addEventListener('scroll', schedulePlace, { capture: true, passive: true });
  addEventListener('resize', schedulePlace, { passive: true });

  function showIcon() { ui(); iconEl.style.display = 'block'; place(); }
  function hideIcon() { if (iconEl) iconEl.style.display = 'none'; }
  function closeMenu() { if (menuEl) menuEl.style.display = 'none'; }

  document.addEventListener('focusin', (e) => {
    const el = e.composedPath ? e.composedPath()[0] : e.target;
    const k = kindOf(el);
    if (!k) return;
    clearTimeout(hideTimer);
    if (current !== el) closeMenu();
    current = el;
    currentKind = k;
    showIcon();
    // Solo se abre solo si el foco viene de ti (clic o teclado), no de la página.
    if (!el.value && !dismissed && Date.now() - lastGesture < 800) openMenu(true);
  }, true);
  document.addEventListener('focusout', () => {
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      const a = document.activeElement;
      if (a === current) return;
      hideIcon(); closeMenu();
    }, 180);
  }, true);
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && menuOpen()) { closeMenu(); dismissed = true; }
    if (e.isTrusted && e.key === 'ArrowDown' && menuOpen() && e.target === current) {
      const first = menuEl.querySelector('.it');
      if (first) { e.preventDefault(); first.focus(); }
    }
  }, true);
  document.addEventListener('mousedown', (e) => {
    if (!menuOpen()) return;
    const path = e.composedPath ? e.composedPath() : [];
    if (!path.some((n) => n && n.tagName === 'TURBOVAULT-UI') && e.target !== current) { closeMenu(); }
  }, true);

  // ---------- menú ----------
  let lastQuery = null;
  async function openMenu(auto = false) {
    ui();
    const forKind = currentKind;
    let res;
    try { res = await bg({ type: 'query' }); } catch { return; }
    lastQuery = res;
    if (!res || res.blocked || forKind !== currentKind || !current) return;
    const st = res.state || {};
    const matches = res.matches || [];
    // Abierto solo (al enfocar): no molestar si no hay nada útil que enseñar.
    if (auto && st.agent && st.unlocked && st.paired && !matches.length && currentKind !== 'newpassword') return;
    if (auto && !st.agent) return;
    renderMenu(res);
    menuEl.style.display = 'block';
    place();
  }

  function renderMenu(res) {
    const st = res.state || {};
    const matches = (res.matches || []).filter((m) => currentKind !== 'otp' || m.hasTotp);
    const host = res.host || location.hostname.replace(/^www\./, '');
    let h = `<div class="hd">${LOGO}<span><b>TurboVAULT</b> · ${esc(host)}</span></div>`;
    if (!st.agent) {
      h += `<div class="msg"><b>TurboVAULT no responde</b>${esc(st.error || 'Comprueba que está instalado.')}</div>`;
    } else if (!st.exists) {
      h += `<div class="msg"><b>Aún no tienes bóveda</b>Ábre TurboVAULT para crearla.</div><div class="row"><button class="btn" data-a="open">Abrir TurboVAULT</button></div>`;
    } else if (!st.unlocked) {
      h += `<div class="msg"><b>La bóveda está cerrada</b>Se abrirá TurboVAULT para que escribas tu contraseña maestra (nunca en esta página).</div><div class="row"><button class="btn" data-a="unlock">Abrir la bóveda</button></div>`;
    } else if (!st.paired) {
      h += `<div class="msg"><b>Vincula este navegador</b>TurboVAULT te pedirá confirmarlo con un código.</div><div class="row"><button class="btn" data-a="pair">Vincular</button></div>`;
    } else {
      if (currentKind === 'newpassword') {
        h += `<button class="it gen" data-a="gen"><div class="av">★</div><div class="tx"><b>Usar una contraseña segura</b><span>Generada al azar; al enviar te ofrece guardarla</span></div></button>`;
        if (matches.length) h += '<hr>';
      }
      for (const m of matches) {
        const label = currentKind === 'otp' ? `Código 2FA de ${m.title}` : m.title;
        h += `<button class="it" data-id="${esc(m.id)}"><div class="av" style="--c:${colorOf(m.host || m.title)}">${esc((m.title || '?').charAt(0))}</div>
          <div class="tx"><b>${esc(label)}</b><span>${esc(m.username || m.host || '')}</span></div></button>`;
      }
      if (!matches.length && currentKind !== 'newpassword') {
        h += `<div class="msg"><b>Nada guardado para esta web</b>Cuando inicies sesión, TurboVAULT te ofrecerá guardarla.</div>`;
      }
    }
    if (st.agent) h += `<hr><button class="ft" data-a="app">Abrir TurboVAULT</button>`;
    menuEl.innerHTML = h;
    menuEl.querySelectorAll('[data-id]').forEach((b) => onRealClick(b, () => fill(b.dataset.id)));
    menuEl.querySelectorAll('[data-a]').forEach((b) => onRealClick(b, () => action(b.dataset.a)));
  }

  async function action(a) {
    try {
      if (a === 'unlock') { await bg({ type: 'unlock' }); menuMsg('Escribe tu contraseña maestra en la ventana de TurboVAULT…'); }
      if (a === 'open' || a === 'app') { await bg({ type: 'openApp' }); closeMenu(); }
      if (a === 'pair') {
        const r = await bg({ type: 'pair' });
        menuEl.innerHTML = `<div class="hd">${LOGO}<span><b>TurboVAULT</b></span></div><div class="msg"><b>Confirma en TurboVAULT</b>Comprueba que la ventana enseña este mismo código:</div><div class="code">${esc(r.code)}</div>`;
      }
      if (a === 'gen') await generateInto(current);
    } catch (e) { menuMsg(e.message); }
  }
  function menuMsg(t) {
    if (!menuEl) return;
    menuEl.innerHTML = `<div class="hd">${LOGO}<span><b>TurboVAULT</b></span></div><div class="msg">${esc(t)}</div>`;
    menuEl.style.display = 'block';
    place();
  }

  // ---------- rellenar ----------
  async function fill(id) {
    let data;
    try { data = await bg({ type: currentKind === 'otp' ? 'totp' : 'fill', id }); } catch (e) { menuMsg(e.message); return; }
    closeMenu();
    applyFill(data, current, currentKind);
    data = null;
  }

  function applyFill(data, target, kind) {
    if (!data) return;
    if (kind === 'otp') { if (data.totp) setValue(target, data.totp); return; }
    const scope = target ? scopeOf(target) : document;
    const pws = pwFields(scope);
    let pw = pws.find((x) => (x.getAttribute('autocomplete') || '').includes('current-password')) || pws[0];
    if (target && target.type === 'password' && kind !== 'newpassword') pw = target;
    const user = pw ? usernameFor(pw) : (kind === 'username' ? target : null);
    if (user && data.username) setValue(user, data.username);
    if (pw && data.password) setValue(pw, data.password);
    if (!pw && kind === 'username' && target && data.username) setValue(target, data.username);
    const otp = [...scope.querySelectorAll('input')].find((x) => visible(x) && kindOf(x) === 'otp');
    if (otp && data.totp && !otp.value) setValue(otp, data.totp);
    (pw || user || target)?.focus({ preventScroll: true });
  }

  async function generateInto(target) {
    if (!target) return;
    const g = await bg({ type: 'generate' });
    closeMenu();
    const scope = scopeOf(target);
    const pws = pwFields(scope);
    let fields;
    if (pws.length >= 3) fields = pws.slice(1); // actual, nueva, repetir
    else if (pws.length === 2) fields = pws; // nueva y repetir
    else fields = [target];
    for (const f of fields) setValue(f, g.password);
    toast('Contraseña segura puesta. Al enviar, TurboVAULT te ofrecerá guardarla.');
  }

  // ---------- guardar al enviar ----------
  let lastCapture = { key: '', t: 0 };
  function captureFrom(el) {
    const scope = el ? scopeOf(el) : document;
    const pws = pwFields(scope, false).filter((x) => x.value);
    if (!pws.length) {
      // Paso del correo en inicios de sesión de dos pantallas.
      const u = [...scope.querySelectorAll('input')].find((x) => x.value && kindOf(x) === 'username');
      if (u) bg({ type: 'username', username: u.value }).catch(() => {});
      return;
    }
    let pw;
    const newPw = pws.find((x) => (x.getAttribute('autocomplete') || '').includes('new-password'));
    if (newPw) pw = newPw;
    else if (pws.length >= 3) pw = pws[1];
    else if (pws.length === 2) pw = pws[0].value === pws[1].value ? pws[0] : pws[1];
    else pw = pws[0];
    const user = usernameFor(pws[0]);
    const username = user ? user.value.trim() : '';
    const key = username + '\u0000' + pw.value;
    if (lastCapture.key === key && Date.now() - lastCapture.t < 4000) return;
    lastCapture = { key, t: Date.now() };
    bg({ type: 'captured', username, password: pw.value }).then((r) => { if (r) showBar(r); }).catch(() => {});
  }
  document.addEventListener('submit', (e) => { if (e.isTrusted || Date.now() - lastGesture < 1500) captureFrom(e.target); }, true);
  document.addEventListener('click', (e) => {
    if (!e.isTrusted) return;
    const b = e.target && e.target.closest ? e.target.closest('button, input[type=submit], input[type=button], [role=button]') : null;
    if (!b) return;
    const label = (b.innerText || b.value || b.getAttribute('aria-label') || '').trim();
    if (b.type === 'submit' || SUBMIT_RE.test(label)) captureFrom(b);
  }, true);
  document.addEventListener('keydown', (e) => {
    if (e.isTrusted && e.key === 'Enter' && e.target && e.target.tagName === 'INPUT' && (e.target.type === 'password' || kindOf(e.target) === 'username')) captureFrom(e.target);
  }, true);

  // ---------- barra de guardar ----------
  let barTimer = 0;
  function showBar(r) {
    if (!isTop) return; // la barra va siempre en la página principal
    ui();
    clearTimeout(barTimer);
    const who = r.username ? `<span>${esc(r.username)} · ${esc(r.host || '')}</span>` : `<span>${esc(r.host || '')}</span>`;
    let title, acts;
    switch (r.status) {
      case 'update':
        title = `¿Actualizar la contraseña de ${esc(r.title || r.host)}?`;
        acts = '<button class="btn ghost" data-b="dismiss">Ahora no</button><button class="btn" data-b="save">Actualizar</button>';
        break;
      case 'locked':
        title = '¿Guardar en TurboVAULT?';
        acts = '<button class="btn ghost" data-b="dismiss">Ahora no</button><button class="btn" data-b="save">Abrir y guardar</button>';
        break;
      case 'unpaired':
        title = '¿Guardar en TurboVAULT?';
        acts = '<button class="btn ghost" data-b="dismiss">Ahora no</button><button class="btn" data-b="save">Vincular y guardar</button>';
        break;
      case 'saved':
        title = `<span class="ok">✓</span> Guardada en TurboVAULT`;
        acts = '';
        barTimer = setTimeout(hideBar, 2600);
        break;
      default:
        title = `¿Guardar la contraseña de ${esc(r.host || 'esta web')}?`;
        acts = '<button class="btn ghost" data-b="never">Nunca aquí</button><button class="btn ghost" data-b="dismiss">Ahora no</button><button class="btn" data-b="save">Guardar</button>';
    }
    barEl.innerHTML = `<div class="top">${LOGO}<div class="tt"><b>${title}</b>${r.status === 'saved' ? '' : who}</div><button class="x" data-b="dismiss" title="Cerrar">×</button></div>${acts ? `<div class="acts">${acts}</div>` : ''}`;
    barEl.style.display = 'block';
    barEl.querySelectorAll('[data-b]').forEach((b) => {
      if (b.dataset.b === 'dismiss') b.addEventListener('click', (e) => { if (e.isTrusted) barAction('dismiss'); });
      else onRealClick(b, () => barAction(b.dataset.b));
    });
    if (r.status !== 'saved') barTimer = setTimeout(hideBar, 90000);
  }
  function hideBar() { if (barEl) barEl.style.display = 'none'; }
  async function barAction(a) {
    if (a === 'dismiss') { hideBar(); bg({ type: 'saveAction', action: 'dismiss' }).catch(() => {}); return; }
    try {
      const r = await bg({ type: 'saveAction', action: a });
      if (a === 'never') { hideBar(); return; }
      if (r && r.done === 'saved') showBar({ status: 'saved' });
      else if (r && r.needPair) barMsg(`Confirma en TurboVAULT que ves este código: <b style="font-family:Consolas,monospace;letter-spacing:2px">${esc(r.code)}</b>`);
      else if (r && r.waiting) barMsg('Escribe tu contraseña maestra en la ventana de TurboVAULT. Se guardará sola.');
    } catch (e) { barMsg(esc(e.message)); }
  }
  function barMsg(html) {
    const tt = barEl.querySelector('.tt span') || barEl.querySelector('.tt');
    if (tt) tt.innerHTML = html;
    const acts = barEl.querySelector('.acts');
    if (acts) acts.remove();
  }

  let toastEl = null;
  function toast(t) {
    ui();
    if (!toastEl) { toastEl = document.createElement('div'); toastEl.className = 'bar'; root.appendChild(toastEl); }
    toastEl.innerHTML = `<div class="top">${LOGO}<div class="tt"><b>TurboVAULT</b><span style="white-space:normal">${esc(t)}</span></div></div>`;
    toastEl.style.display = 'block';
    setTimeout(() => { toastEl.style.display = 'none'; }, 4000);
  }

  // ---------- mensajes del fondo ----------
  chrome.runtime.onMessage.addListener((msg) => {
    if (msg.type === 'state') {
      if (menuOpen() && current) openMenu();
    } else if (msg.type === 'fillNow') {
      if (!isTop || msg.origin !== location.origin) return;
      let target = document.activeElement && kindOf(document.activeElement) ? document.activeElement : null;
      if (!target) target = pwFields(document)[0] || [...document.querySelectorAll('input')].find((x) => visible(x) && kindOf(x) === 'username') || null;
      if (target) applyFill(msg.data, target, kindOf(target));
    } else if (msg.type === 'openMenu') {
      if (!document.hasFocus()) return;
      let el = document.activeElement && kindOf(document.activeElement) ? document.activeElement : null;
      if (!el) el = pwFields(document)[0] || [...document.querySelectorAll('input')].find((x) => visible(x) && kindOf(x));
      if (!el) return;
      el.focus();
      current = el; currentKind = kindOf(el);
      dismissed = false;
      showIcon();
      openMenu();
    } else if (msg.type === 'generateHere') {
      if (!document.hasFocus()) return;
      const el = document.activeElement;
      if (el && el.tagName === 'INPUT') { current = el; currentKind = 'newpassword'; generateInto(el).catch((e) => toast(e.message)); }
    } else if (msg.type === 'saveUpdate') {
      if (isTop) showBar(msg);
    }
  });

  // Al cargar: ¿quedó algo por guardar de la página anterior (tras enviar el formulario)?
  if (isTop) {
    setTimeout(() => { bg({ type: 'pendingSave' }).then((r) => { if (r) showBar(r); }).catch(() => {}); }, 400);
  }
})();
