// TurboVAULT en las páginas: un botón dentro de cada campo de inicio de sesión
// (de color con la bóveda abierta y gris si está cerrada), un menú con lo
// guardado para esta web y la oferta de guardar al enviar. Nada se rellena sin
// que lo pidas, y todo lo que se dibuja va en un shadow DOM cerrado, fuera del
// alcance de los scripts de la página.
(() => {
  // Si ya había una copia en esta página (de antes de recargar la extensión),
  // se retira y se queda esta.
  if (typeof window.__turbovault === 'function') { try { window.__turbovault(); } catch { /* nada */ } }

  const isTop = window === window.top;
  let sameOriginFrame = isTop;
  if (!isTop) {
    try { sameOriginFrame = window.top.location.origin === location.origin; } catch { sameOriginFrame = false; }
  }
  if (!sameOriginFrame) return; // marcos de otras webs: ni se tocan
  window.__turbovault = teardown;

  // Al recargar o quitar la extensión, esta copia se queda sin conexión: se
  // retira sola (iconos fuera, la página como estaba).
  let dead = false;
  const alive = () => { try { return !!(chrome.runtime && chrome.runtime.id); } catch { return false; } };
  const on = (target, type, fn, opts) => target.addEventListener(type, (e) => { if (!dead) fn(e); }, opts);

  const bg = (msg) => new Promise((resolve, reject) => {
    if (!alive()) { teardown(); reject(new Error('La extensión se ha recargado')); return; }
    try {
      chrome.runtime.sendMessage(msg, (r) => {
        if (chrome.runtime.lastError) { reject(new Error(chrome.runtime.lastError.message)); return; }
        if (!r) { resolve(null); return; }
        if (r.ok) resolve(r.data); else reject(Object.assign(new Error(r.error), { code: r.code }));
      });
    } catch (e) { if (!alive()) teardown(); reject(e); }
  });

  // ---------- clics de verdad ----------
  // La página no puede pulsar por ti: solo cuentan eventos reales (isTrusted)
  // sobre botones que llevan un momento totalmente visibles. Con la
  // IntersectionObserver v2 Chrome dice si algo los tapa, los vuelve
  // transparentes o los deforma (lo que haría una web para engañarte). Por eso
  // nada de lo que se pulsa lleva transparencias, filtros ni animaciones.
  let lastGesture = 0;
  for (const t of ['pointerdown', 'keydown']) on(window, t, (e) => { if (e.isTrusted) lastGesture = Date.now(); }, true);
  const VISIBLE_MS = 300;
  const visibleSince = new WeakMap();
  const ioV2 = typeof IntersectionObserverEntry !== 'undefined' && 'isVisible' in IntersectionObserverEntry.prototype;
  let io = null;
  function watch(el) {
    if (!ioV2) return;
    if (!io) {
      io = new IntersectionObserver((entries) => {
        for (const en of entries) {
          if (en.isVisible && en.intersectionRatio >= 0.6) { if (!visibleSince.has(en.target)) visibleSince.set(en.target, en.time); }
          else visibleSince.delete(en.target);
        }
      }, { trackVisibility: true, delay: 100, threshold: [0, 0.6, 1] });
    }
    io.observe(el);
  }
  function realClick(e, el) {
    if (!e.isTrusted) return false;
    if (!ioV2) return true;
    const t = visibleSince.get(el);
    return t !== undefined && performance.now() - t >= VISIBLE_MS;
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

  // Mientras tiene el foco, el campo lleva autocomplete="off" para que Chrome
  // no saque sus sugerencias encima del menú; aquí se guarda el valor de la
  // página para devolverlo al salir (y para entender el campo).
  const acSaved = new WeakMap();
  const acOf = (el) => ((acSaved.has(el) ? acSaved.get(el) : el.getAttribute('autocomplete')) || '').toLowerCase();

  function visible(el) {
    if (!el || !el.isConnected || el.disabled) return false;
    const r = el.getBoundingClientRect();
    if (r.width < 24 || r.height < 12) return false;
    // Fuera de la página por arriba o por la izquierda (left: -9999px y parecidos): una
    // web puede esconder ahí campos para que se rellenen sin que lo veas. (Por la
    // derecha o por abajo no se mira: las webs que se desplazan dentro de un recuadro
    // tienen campos legítimos ahí; en las de derecha a izquierda, tampoco a la izquierda.)
    const rtl = getComputedStyle(document.documentElement).direction === 'rtl';
    if (r.bottom + scrollY <= 0 || (!rtl && r.right + scrollX <= 0)) return false;
    const cs = getComputedStyle(el);
    if (cs.visibility === 'hidden' || cs.display === 'none' || parseFloat(cs.opacity || '1') <= 0.05) return false;
    // Transparente por estar dentro de algo transparente.
    for (let p = el.parentElement, i = 0; p && i < 12; p = p.parentElement, i++) {
      if (parseFloat(getComputedStyle(p).opacity || '1') <= 0.05) return false;
    }
    return true;
  }
  const attrs = (el) => [el.name, el.id, el.getAttribute('placeholder'), el.getAttribute('aria-label'), acOf(el)].join(' ');
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
    const ac = acOf(el);
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
    const ac = inputs.find((x) => /username|email/.test(acOf(x)));
    if (ac) return ac;
    let best = null;
    for (const x of inputs) {
      if (pw.compareDocumentPosition(x) & Node.DOCUMENT_POSITION_PRECEDING) {
        if (!OTP_RE.test(attrs(x)) && !/search|buscar|captcha/i.test(attrs(x))) best = x;
      }
    }
    return best || inputs.find((x) => kindOf(x) === 'username') || null;
  }

  let filling = false; // rellenando: los cambios de foco no abren el menú
  function setValue(el, value) {
    if (!el) return;
    el.focus({ preventScroll: true });
    const proto = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value');
    proto.set.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    el.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true, key: 'Unidentified' }));
  }

  function quietAutocomplete(el) {
    if (!el || el.type === 'password' || acSaved.has(el)) return;
    acSaved.set(el, el.getAttribute('autocomplete'));
    el.setAttribute('autocomplete', 'off');
  }
  function restoreAutocomplete(el) {
    if (!acSaved.has(el)) return;
    const v = acSaved.get(el);
    acSaved.delete(el);
    if (el.getAttribute('autocomplete') !== 'off') return; // la página lo cambió entretanto
    if (v === null) el.removeAttribute('autocomplete'); else el.setAttribute('autocomplete', v);
  }

  // ---------- interfaz (shadow DOM cerrado) ----------
  const DEFS = '<svg class="defs" width="0" height="0" aria-hidden="true"><defs>'
    + '<linearGradient id="tv-g" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#8b5cf6"/><stop offset=".5" stop-color="#6366f1"/><stop offset="1" stop-color="#06b6d4"/></linearGradient>'
    + '<linearGradient id="tv-gg" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#b4b4bd"/><stop offset="1" stop-color="#7c7c88"/></linearGradient>'
    + '</defs></svg>';
  const logo = (off) => {
    const k = off ? '#7c7c88' : '#4f46e5';
    return `<svg viewBox="0 0 64 64"><rect x="2" y="2" width="60" height="60" rx="15" fill="url(#tv-${off ? 'gg' : 'g'})"/><path d="M22.5 29v-6.5a9.5 9.5 0 0 1 19 0V29" fill="none" stroke="#fff" stroke-width="5.2" stroke-linecap="round"/><rect x="15" y="27" width="34" height="25" rx="6.5" fill="#fff"/><circle cx="32" cy="37.5" r="3.9" fill="${k}"/><rect x="30.2" y="38.5" width="3.6" height="8" rx="1.8" fill="${k}"/></svg>`;
  };
  const LOGO = logo(false);
  const LOGO_OFF = logo(true);
  const CSS = `
    :host { all: initial; }
    * { box-sizing: border-box; font-family: "Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif; }
    .defs { position: absolute; width: 0; height: 0; overflow: hidden; }
    .ic { position: fixed; top: 0; left: 0; width: 22px; height: 22px; padding: 0; margin: 0; border: 0; border-radius: 6px; background: transparent; cursor: pointer; display: none; z-index: 2147483646; box-shadow: 0 2px 8px -2px rgba(79,70,229,.55); }
    .ic.off { box-shadow: 0 2px 6px -2px rgba(0,0,0,.3); }
    .ic:hover { box-shadow: 0 0 0 3px rgba(99,102,241,.3), 0 2px 8px -2px rgba(79,70,229,.6); }
    .ic.off:hover { box-shadow: 0 0 0 3px rgba(140,140,150,.3); }
    .ic svg { width: 100%; height: 100%; display: block; }
    .menu { position: fixed; top: 0; left: 0; z-index: 2147483647; width: 320px; max-height: 360px; overflow: auto; display: none; padding: 6px; border-radius: 14px; background: #12141f; color: #eceef8; border: 1px solid rgba(255,255,255,.12); box-shadow: 0 24px 60px -12px rgba(0,0,0,.75); font-size: 13px; line-height: 1.4; }
    .hd { display: flex; align-items: center; gap: 8px; padding: 6px 8px 8px; font-size: 11.5px; color: #9196ae; }
    .hd svg { width: 16px; height: 16px; flex: none; }
    .hd b { color: #eceef8; font-weight: 700; }
    .it { width: 100%; display: flex; align-items: center; gap: 10px; padding: 8px; border: 0; border-radius: 10px; background: transparent; color: inherit; cursor: pointer; text-align: left; font-size: 13px; }
    .it:hover, .it:focus-visible { background: #232634; outline: none; }
    .av { width: 30px; height: 30px; border-radius: 9px; flex: none; display: grid; place-items: center; font-weight: 800; font-size: 13px; color: #fff; background: var(--c, #4f46e5); text-transform: uppercase; }
    .tx { min-width: 0; flex: 1; }
    .tx b { display: block; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .tx span { display: block; font-size: 12px; color: #9196ae; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .msg { padding: 8px 10px 10px; color: #9196ae; font-size: 12.5px; }
    .msg b { color: #eceef8; display: block; font-size: 13.5px; margin-bottom: 2px; }
    .btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; border: 0; border-radius: 10px; padding: 8px 14px; font-weight: 650; font-size: 13px; cursor: pointer; color: #fff; background: linear-gradient(135deg,#8b5cf6,#6366f1 45%,#06b6d4); box-shadow: 0 8px 20px -10px rgba(99,102,241,.9); }
    .btn.ghost { background: #1e2130; box-shadow: none; color: #eceef8; border: 1px solid rgba(255,255,255,.1); }
    .row { display: flex; gap: 6px; padding: 2px 8px 8px; flex-wrap: wrap; }
    hr { border: 0; border-top: 1px solid rgba(255,255,255,.08); margin: 4px 4px; }
    .ft { width: 100%; display: flex; align-items: center; gap: 8px; padding: 7px 8px; border: 0; border-radius: 9px; background: transparent; color: #9196ae; cursor: pointer; font-size: 12.5px; }
    .ft:hover { background: #1e2130; color: #eceef8; }
    .gen .av { background: linear-gradient(135deg,#10b981,#06b6d4); }
    .code { font-family: "Cascadia Mono", Consolas, monospace; font-size: 26px; font-weight: 800; letter-spacing: 4px; color: #a5b4fc; padding: 4px 10px 6px; }
    .bar { position: fixed; top: 16px; right: 16px; z-index: 2147483647; width: 340px; display: none; padding: 14px; border-radius: 16px; background: #12141f; color: #eceef8; border: 1px solid rgba(255,255,255,.12); box-shadow: 0 30px 70px -14px rgba(0,0,0,.8); font-size: 13px; }
    .bar .top { display: flex; align-items: flex-start; gap: 10px; }
    .bar .top svg { width: 30px; height: 30px; flex: none; }
    .bar .tt { flex: 1; min-width: 0; }
    .bar .tt b { display: block; font-size: 14px; font-weight: 700; }
    .bar .tt span { display: block; color: #9196ae; font-size: 12.5px; margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .bar .x { border: 0; background: transparent; color: #9196ae; cursor: pointer; font-size: 18px; line-height: 1; padding: 2px 4px; border-radius: 6px; }
    .bar .x:hover { color: #fff; background: #232634; }
    .bar .acts { display: flex; gap: 6px; margin-top: 12px; justify-content: flex-end; }
    .ok { color: #34d399; }
  `;
  const COLORS = ['#7c3aed', '#4f46e5', '#0891b2', '#059669', '#d97706', '#db2777', '#2563eb', '#9333ea', '#0d9488', '#dc2626', '#65a30d', '#c026d3'];
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const colorOf = (s) => { let h = 0; for (const c of s || '?') h = (h * 31 + c.charCodeAt(0)) >>> 0; return COLORS[h % COLORS.length]; };
  const pageHost = () => location.hostname.replace(/^www\./, '');

  let root = null, hostEl = null, menuEl, barEl;
  function ui() {
    if (root) return;
    hostEl = document.createElement('turbovault-ui');
    hostEl.style.cssText = 'all: initial !important; display: block !important; position: fixed !important; top: 0 !important; left: 0 !important; width: 0 !important; height: 0 !important; overflow: visible !important; z-index: 2147483647 !important;';
    document.documentElement.appendChild(hostEl);
    root = hostEl.attachShadow({ mode: 'closed' });
    root.innerHTML = `<style>${CSS}</style>${DEFS}<div class="menu" role="menu"></div><div class="bar" role="dialog"></div>`;
    menuEl = root.querySelector('.menu');
    barEl = root.querySelector('.bar');
    menuEl.addEventListener('mousedown', (e) => e.preventDefault()); // que el campo no pierda el foco
  }

  // El menú tiene que verse por encima de todo: si el formulario está en un
  // diálogo modal de la página (que va por encima de cualquier z-index y deja
  // inerte lo de fuera), la interfaz se mete dentro de ese diálogo.
  function modalOf(el) {
    const d = el && el.closest ? el.closest('dialog') : null;
    try { return d && d.matches(':modal') ? d : null; } catch { return null; }
  }
  function placeHost() {
    let want = current && current.isConnected ? modalOf(current) : null;
    if (!want) {
      for (const d of document.querySelectorAll('dialog[open]')) { try { if (d.matches(':modal')) want = d; } catch { /* nada */ } }
    }
    want = want || document.documentElement;
    if (hostEl.parentNode !== want) want.appendChild(hostEl);
  }

  // ---------- los iconos de los campos ----------
  const MAX_FIELDS = 30;
  const fields = new Map(); // campo → { icon, shown }
  let pageState = null; // estado de TurboVAULT (agente, abierta, vinculado)
  let matches = null; // lo guardado para esta web (solo con la bóveda abierta y vinculada)
  const isOpen = (st) => !!(st && st.agent && st.unlocked && st.paired);
  const iconOn = () => !!(pageState && pageState.agent && pageState.unlocked);

  const ro = typeof ResizeObserver === 'function' ? new ResizeObserver(() => layoutSoon()) : null;

  function track(el) {
    ui();
    const icon = document.createElement('button');
    icon.type = 'button';
    icon.tabIndex = -1;
    icon.setAttribute('aria-hidden', 'true');
    paintIcon(icon);
    icon.addEventListener('mousedown', (e) => { e.preventDefault(); e.stopPropagation(); });
    onRealClick(icon, () => iconClicked(el));
    root.appendChild(icon);
    fields.set(el, { icon, shown: false });
    if (ro) ro.observe(el);
  }
  function untrack(el) {
    const f = fields.get(el);
    if (!f) return;
    f.icon.remove();
    if (io) io.unobserve(f.icon);
    if (ro) ro.unobserve(el);
    fields.delete(el);
    restoreAutocomplete(el);
  }
  function paintIcon(icon) {
    const on = iconOn();
    icon.className = on ? 'ic' : 'ic off';
    icon.innerHTML = on ? LOGO : LOGO_OFF;
    icon.title = !pageState || !pageState.agent ? 'TurboVAULT no responde' : on ? 'TurboVAULT' : 'TurboVAULT: la bóveda está cerrada';
  }
  let paintedOn = null;
  function paintIcons() {
    const key = `${iconOn()}|${pageState && pageState.agent}`;
    if (key === paintedOn) return;
    paintedOn = key;
    for (const f of fields.values()) paintIcon(f.icon);
  }

  // Buscar campos: al cargar, cuando la página cambia y, por si acaso, cada poco.
  let scanTimer = 0;
  function scanSoon() { if (!scanTimer && !dead) scanTimer = setTimeout(scan, 300); }
  function scan() {
    clearTimeout(scanTimer);
    scanTimer = 0;
    if (dead) return;
    if (!alive()) { teardown(); return; }
    const list = document.getElementsByTagName('input');
    const n = Math.min(list.length, 800);
    for (let i = 0; i < n && fields.size < MAX_FIELDS; i++) {
      const el = list[i];
      if (!fields.has(el) && kindOf(el)) track(el);
    }
    for (const el of [...fields.keys()]) if (!el.isConnected) untrack(el);
    if (fields.size) { ensureState(); startTick(); layoutSoon(); }
  }

  let tickTimer = 0, ticks = 0;
  function startTick() {
    if (tickTimer) return;
    tickTimer = setInterval(() => {
      if (!alive()) { teardown(); return; }
      if (document.hidden) return;
      if (++ticks % 4 === 0) scan();
      if (!fields.size && !menuOpen()) { clearInterval(tickTimer); tickTimer = 0; return; }
      layout();
    }, 400);
  }

  let rafPending = false;
  function layoutSoon() {
    if (rafPending || !root) return;
    rafPending = true;
    requestAnimationFrame(() => { rafPending = false; layout(); });
  }

  // Qué hay encima de un punto, sin contar la interfaz de TurboVAULT.
  function topAt(x, y) {
    if (x < 0 || y < 0 || x >= innerWidth || y >= innerHeight) return null;
    for (const e of document.elementsFromPoint(x, y)) if (e !== hostEl) return e;
    return null;
  }
  // ¿Lo que se ve en ese punto es el propio campo? (Si está dentro de un shadow
  // DOM, el navegador dice el elemento que lo contiene.)
  function isField(hit, el) {
    for (let n = el; n; n = n.getRootNode().host) if (n === hit) return true;
    return false;
  }

  // Dónde va el icono de un campo (null si no se debe ver).
  function iconPos(el) {
    if (el.disabled || el.readOnly) return null;
    const r = el.getBoundingClientRect();
    if (r.width < 60 || r.height < 16 || r.bottom <= 0 || r.right <= 0 || r.top >= innerHeight || r.left >= innerWidth) return null;
    if (el.checkVisibility && !el.checkVisibility({ opacityProperty: true, visibilityProperty: true, checkOpacity: true, checkVisibilityCSS: true })) return null;
    const s = Math.round(Math.max(16, Math.min(22, r.height - 10)));
    const pad = Math.max(4, Math.min(8, (r.height - s) / 2));
    let x = r.right - s - pad;
    const y = r.top + (r.height - s) / 2;
    for (let i = 0; i < 3; i++) {
      const hit = topAt(x + s / 2, y + s / 2);
      if (!hit || isField(hit, el)) return { x, y, s }; // sin hit: el punto cae fuera de la ventana
      // Un botón pequeño dentro del campo (ver contraseña, borrar…): el icono va a su izquierda.
      const hr = hit.getBoundingClientRect();
      if (hr.width <= 64 && hr.height <= r.height + 8 && hr.left > r.left + r.width / 2 && hr.left - s - 4 > r.left + r.width / 2) {
        x = hr.left - s - 4;
        continue;
      }
      return null; // lo tapa otra cosa (un aviso, un menú de la página…)
    }
    return null;
  }

  function layout() {
    if (!root) return;
    if (!hostEl.isConnected || hostEl.parentNode !== document.documentElement || document.querySelector('dialog[open]')) placeHost();
    const o = hostEl.getBoundingClientRect(); // origen de las posiciones (por si va dentro de un diálogo)
    for (const [el, f] of fields) {
      if (!el.isConnected) { untrack(el); continue; }
      const p = iconPos(el);
      if (!p) {
        if (f.shown) { f.icon.style.display = 'none'; f.shown = false; }
        continue;
      }
      const css = `display:block;left:${Math.round(p.x - o.left)}px;top:${Math.round(p.y - o.top)}px;width:${p.s}px;height:${p.s}px`;
      if (css !== f.css || !f.shown) { f.icon.style.cssText = css; f.css = css; }
      f.shown = true;
    }
    placeMenu(o);
  }
  on(window, 'scroll', layoutSoon, { capture: true, passive: true });
  on(window, 'resize', layoutSoon, { passive: true });

  // ---------- foco ----------
  let current = null; // campo del menú
  let currentKind = null;
  let dismissed = false; // el menú automático se cerró a mano en esta página
  let iconFocusing = false; // el foco lo pone TurboVAULT al pulsar el icono
  let hideTimer = 0;
  const menuOpen = () => !!menuEl && menuEl.style.display === 'block';

  on(document, 'focusin', (e) => {
    const el = e.composedPath ? e.composedPath()[0] : e.target;
    if (!el || el.tagName !== 'INPUT') return;
    let k = fields.has(el) ? null : kindOf(el);
    if (!fields.has(el)) {
      if (!k) return;
      if (fields.size < MAX_FIELDS) { track(el); ensureState(); startTick(); }
    }
    k = k || kindOf(el);
    if (!k) return;
    clearTimeout(hideTimer);
    if (current !== el) closeMenu();
    current = el;
    currentKind = k;
    quietAutocomplete(el);
    layoutSoon();
    // Solo se abre solo si el foco viene de ti (clic o teclado), no de la página.
    if (!iconFocusing && !filling && !el.value && !dismissed && Date.now() - lastGesture < 800) openMenu(true);
  }, true);
  on(document, 'focusout', (e) => {
    const el = e.composedPath ? e.composedPath()[0] : e.target;
    if (el && el.tagName === 'INPUT') restoreAutocomplete(el);
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      const a = document.activeElement;
      if (a === current || (hostEl && a === hostEl)) return;
      closeMenu();
    }, 180);
  }, true);
  on(document, 'keydown', (e) => {
    if (e.key === 'Escape' && menuOpen()) { closeMenu(); dismissed = true; }
    if (e.isTrusted && e.key === 'ArrowDown' && menuOpen() && e.target === current) {
      const first = menuEl.querySelector('.it');
      if (first) { e.preventDefault(); first.focus(); }
    }
  }, true);
  on(document, 'mousedown', (e) => {
    if (!menuOpen()) return;
    const path = e.composedPath ? e.composedPath() : [];
    if (!path.includes(hostEl) && e.target !== current) closeMenu();
  }, true);

  // Un clic en el icono abre el menú (y otro lo cierra).
  function iconClicked(el) {
    if (menuOpen() && current === el && performance.now() - menuShownAt > 300) { closeMenu(); return; }
    current = el;
    currentKind = kindOf(el);
    dismissed = false;
    if (document.activeElement !== el) {
      iconFocusing = true;
      try { el.focus({ preventScroll: true }); } finally { iconFocusing = false; }
    }
    openMenu(false);
  }

  // ---------- estado ----------
  let stateAsked = 0;
  function ensureState() {
    if (pageState || (stateAsked && performance.now() - stateAsked < 5000)) return;
    stateAsked = performance.now();
    bg({ type: 'query' }).then(applyQuery).catch(() => {});
  }
  function applyQuery(res) {
    if (!res || res.blocked) return;
    if (res.state) pageState = res.state;
    matches = isOpen(pageState) ? (res.matches || []) : null;
    paintIcons();
  }

  // ---------- menú ----------
  let menuSeq = 0, menuShownAt = 0, menuHTML = '';
  function showMenu() {
    if (!menuOpen()) { menuEl.style.display = 'block'; menuShownAt = performance.now(); }
    placeHost();
    placeMenu(hostEl.getBoundingClientRect());
  }
  function closeMenu() {
    menuSeq++;
    if (menuEl) menuEl.style.display = 'none';
  }
  function placeMenu(o) {
    if (!menuOpen() || !current) return;
    if (!current.isConnected) { closeMenu(); return; }
    const r = current.getBoundingClientRect();
    const mh = menuEl.offsetHeight || 200;
    const below = r.bottom + 6;
    const top = below + mh > innerHeight && r.top - mh - 6 > 0 ? r.top - mh - 6 : below;
    menuEl.style.top = `${Math.round(top - o.top)}px`;
    menuEl.style.left = `${Math.round(Math.max(8, Math.min(r.left, innerWidth - 330)) - o.left)}px`;
  }

  // Al enfocar solo se enseña si hay algo útil (abrir la bóveda, vincular, lo guardado).
  function worthAuto(res) {
    const st = res.state || {};
    if (!st.agent) return false;
    if (st.unlocked && st.paired && !(res.matches || []).length && currentKind !== 'newpassword') return false;
    return true;
  }

  async function openMenu(auto = false) {
    ui();
    const el = current, kind = currentKind;
    if (!el) return;
    const seq = ++menuSeq;
    // Al instante con lo que ya se sabe; luego se confirma con TurboVAULT.
    if (pageState && (!isOpen(pageState) || matches)) {
      const cached = { state: pageState, matches: matches || [], host: pageHost() };
      if (!auto || worthAuto(cached)) { renderMenu(cached); showMenu(); }
    }
    let res;
    try { res = await bg({ type: 'query', user: !auto }); } catch { return; }
    if (!res || res.blocked || el !== current || kind !== currentKind || seq !== menuSeq) return;
    applyQuery(res);
    if (auto && !worthAuto(res)) return;
    renderMenu(res);
    showMenu();
  }

  function renderMenu(res) {
    const st = res.state || {};
    const list = (res.matches || []).filter((m) => currentKind !== 'otp' || m.hasTotp);
    const host = res.host || pageHost();
    let h = `<div class="hd">${LOGO}<span><b>TurboVAULT</b> · ${esc(host)}${insecure ? ' · <b style="color:#fbbf24">sin https</b>' : ''}</span></div>`;
    if (!st.agent) {
      h += `<div class="msg"><b>TurboVAULT no responde</b>${esc(st.error || 'Comprueba que está instalado.')}</div>`;
    } else if (!st.exists) {
      h += '<div class="msg"><b>Aún no tienes bóveda</b>Ábre TurboVAULT para crearla.</div><div class="row"><button class="btn" data-a="open">Abrir TurboVAULT</button></div>';
    } else if (!st.unlocked) {
      h += '<div class="msg"><b>La bóveda está cerrada</b>Se abrirá TurboVAULT para que escribas tu contraseña maestra (nunca en esta página).</div><div class="row"><button class="btn" data-a="unlock">Abrir la bóveda</button></div>';
    } else if (!st.paired) {
      h += '<div class="msg"><b>Vincula este navegador</b>TurboVAULT te pedirá confirmarlo con un código.</div><div class="row"><button class="btn" data-a="pair">Vincular</button></div>';
    } else {
      if (currentKind === 'newpassword') {
        h += '<button class="it gen" data-a="gen"><div class="av">★</div><div class="tx"><b>Usar una contraseña segura</b><span>Generada al azar; al enviar te ofrece guardarla</span></div></button>';
        if (list.length) h += '<hr>';
      }
      for (const m of list) {
        const label = currentKind === 'otp' ? `Código 2FA de ${m.title}` : m.title;
        h += `<button class="it" data-id="${esc(m.id)}"><div class="av" style="--c:${colorOf(m.host || m.title)}">${esc((m.title || '?').charAt(0))}</div>
          <div class="tx"><b>${esc(label)}</b><span>${esc(m.username || m.host || '')}</span></div></button>`;
      }
      if (!list.length && currentKind !== 'newpassword') {
        h += '<div class="msg"><b>Nada guardado para esta web</b>Cuando inicies sesión, TurboVAULT te ofrecerá guardarla.</div>';
      }
    }
    if (st.agent) h += '<hr><button class="ft" data-a="app">Abrir TurboVAULT</button>';
    // Si no cambia nada no se vuelve a dibujar: los botones siguen «a la vista» y se pueden pulsar ya.
    if (h === menuHTML && menuEl.childElementCount) return;
    menuHTML = h;
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
        setMenu(`<div class="hd">${LOGO}<span><b>TurboVAULT</b></span></div><div class="msg"><b>Confirma en TurboVAULT</b>Comprueba que la ventana enseña este mismo código:</div><div class="code">${esc(r.code)}</div>`);
      }
      if (a === 'gen') await generateInto(current);
    } catch (e) { menuMsg(e.message); }
  }
  function setMenu(h) {
    if (!menuEl) return;
    menuHTML = h;
    menuEl.innerHTML = h;
    showMenu();
  }
  function menuMsg(t) { setMenu(`<div class="hd">${LOGO}<span><b>TurboVAULT</b></span></div><div class="msg">${esc(t)}</div>`); }

  // ---------- rellenar ----------
  // Web sin https (salvo las de este PC): lo que se rellena viaja sin cifrar.
  const insecure = location.protocol === 'http:' && !/^(localhost|127\.0\.0\.1|\[::1\])$|\.localhost$/.test(location.hostname);

  async function fill(id, sure = false) {
    if (insecure && !sure && currentKind !== 'otp') {
      setMenu(`<div class="hd">${LOGO}<span><b>TurboVAULT</b></span></div><div class="msg"><b>Esta web no usa https</b>Lo que rellenes viaja sin cifrar: cualquiera en tu misma red (un wifi público, por ejemplo) podría verlo.</div><div class="row"><button class="btn" data-sure="1">Rellenar igualmente</button></div>`);
      const b = menuEl.querySelector('[data-sure]');
      if (b) onRealClick(b, () => fill(id, true));
      return;
    }
    let data;
    try { data = await bg({ type: currentKind === 'otp' ? 'totp' : 'fill', id }); } catch (e) { menuMsg(e.message); return; }
    closeMenu();
    filling = true;
    try { applyFill(data, current, currentKind); } finally { filling = false; }
    data = null;
  }

  function applyFill(data, target, kind) {
    if (!data) return;
    if (kind === 'otp') { if (data.totp) setValue(target, data.totp); return; }
    const scope = target ? scopeOf(target) : document;
    const pws = pwFields(scope);
    let pw = pws.find((x) => acOf(x).includes('current-password')) || pws[0];
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
    let list;
    if (pws.length >= 3) list = pws.slice(1); // actual, nueva, repetir
    else if (pws.length === 2) list = pws; // nueva y repetir
    else list = [target];
    filling = true;
    try { for (const f of list) setValue(f, g.password); } finally { filling = false; }
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
    const newPw = pws.find((x) => acOf(x).includes('new-password'));
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
  on(document, 'submit', (e) => { if (e.isTrusted || Date.now() - lastGesture < 1500) captureFrom(e.target); }, true);
  on(document, 'click', (e) => {
    if (!e.isTrusted) return;
    const b = e.target && e.target.closest ? e.target.closest('button, input[type=submit], input[type=button], [role=button]') : null;
    if (!b) return;
    const label = (b.innerText || b.value || b.getAttribute('aria-label') || '').trim();
    if (b.type === 'submit' || SUBMIT_RE.test(label)) captureFrom(b);
  }, true);
  on(document, 'keydown', (e) => {
    if (e.isTrusted && e.key === 'Enter' && e.target && e.target.tagName === 'INPUT' && (e.target.type === 'password' || kindOf(e.target) === 'username')) captureFrom(e.target);
  }, true);

  // ---------- barra de guardar ----------
  let barTimer = 0;
  function showBar(r) {
    if (!isTop) return; // la barra va siempre en la página principal
    ui();
    placeHost();
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
        title = '<span class="ok">✓</span> Guardada en TurboVAULT';
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
  const onBgMessage = (msg) => {
    if (dead) return;
    if (msg.type === 'state') {
      if (!msg.state) return;
      const was = isOpen(pageState);
      pageState = msg.state;
      if (!isOpen(pageState)) matches = null;
      if (!fields.size && !menuOpen()) return;
      paintIcons();
      if (isOpen(pageState) && !was) {
        // Se acaba de abrir (o vincular): se pide lo guardado para esta web.
        bg({ type: 'query' }).then((r) => {
          applyQuery(r);
          if (menuOpen() && current && r && !r.blocked) renderMenu(r);
        }).catch(() => {});
      } else if (menuOpen() && current) {
        renderMenu({ state: pageState, matches: matches || [], host: pageHost() });
      }
    } else if (msg.type === 'fillNow') {
      if (!isTop || msg.origin !== location.origin) return;
      let target = document.activeElement && kindOf(document.activeElement) ? document.activeElement : null;
      if (!target) target = pwFields(document)[0] || [...document.querySelectorAll('input')].find((x) => visible(x) && kindOf(x) === 'username') || null;
      if (target) {
        filling = true;
        try { applyFill(msg.data, target, kindOf(target)); } finally { filling = false; }
      }
    } else if (msg.type === 'openMenu') {
      if (!document.hasFocus()) return;
      let el = document.activeElement && kindOf(document.activeElement) ? document.activeElement : null;
      if (!el) el = pwFields(document)[0] || [...document.querySelectorAll('input')].find((x) => visible(x) && kindOf(x));
      if (!el) return;
      iconFocusing = true;
      try { el.focus(); } finally { iconFocusing = false; }
      current = el;
      currentKind = kindOf(el);
      dismissed = false;
      if (!fields.has(el) && fields.size < MAX_FIELDS) { track(el); startTick(); }
      layoutSoon();
      openMenu(false);
    } else if (msg.type === 'generateHere') {
      if (!document.hasFocus()) return;
      const el = document.activeElement;
      if (el && el.tagName === 'INPUT') { current = el; currentKind = 'newpassword'; generateInto(el).catch((e) => toast(e.message)); }
    } else if (msg.type === 'saveUpdate') {
      if (isTop) showBar(msg);
    }
  };
  chrome.runtime.onMessage.addListener(onBgMessage);

  function teardown() {
    if (dead) return;
    dead = true;
    try { mo.disconnect(); } catch { /* nada */ }
    try { chrome.runtime.onMessage.removeListener(onBgMessage); } catch { /* sin extensión */ }
    clearInterval(tickTimer);
    clearTimeout(scanTimer);
    clearTimeout(hideTimer);
    clearTimeout(barTimer);
    for (const el of [...fields.keys()]) restoreAutocomplete(el);
    if (current) restoreAutocomplete(current);
    fields.clear();
    if (io) io.disconnect();
    if (ro) ro.disconnect();
    if (hostEl) hostEl.remove();
    if (window.__turbovault === teardown) window.__turbovault = undefined;
  }

  // ---------- arranque ----------
  const mo = new MutationObserver(scanSoon);
  mo.observe(document.documentElement, { childList: true, subtree: true });
  scan();
  on(window, 'load', scanSoon);

  // Al cargar: ¿quedó algo por guardar de la página anterior (tras enviar el formulario)?
  if (isTop) {
    setTimeout(() => { bg({ type: 'pendingSave' }).then((r) => { if (r) showBar(r); }).catch(() => {}); }, 400);
  }
})();
