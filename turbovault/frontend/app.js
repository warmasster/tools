/* TurboVAULT: interfaz. Todo lo secreto lo guarda el agente; aquí solo se
   enseña lo que se pide y se borra en cuanto la bóveda se cierra. */
(() => {
  'use strict';

  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const api = hasGo ? window.go.main.App : window.__tvMock();
  const rt = hasGo ? window.runtime : api.__rt;

  // ---------- utilidades ----------
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };

  const I = {
    all: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    star: '<path d="M12 3.5l2.6 5.3 5.9.9-4.2 4.1 1 5.8-5.3-2.8-5.3 2.8 1-5.8-4.2-4.1 5.9-.9z"/>',
    key: '<circle cx="7.5" cy="15.5" r="4.5"/><path d="m10.7 12.3 8.8-8.8"/><path d="m16 7 3 3"/><path d="m18.5 4.5 2 2"/>',
    note: '<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/><path d="M8 13h8M8 17h5"/>',
    otp: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M12 8v4l2.5 1.5"/>',
    dice: '<rect x="3" y="3" width="18" height="18" rx="4"/><circle cx="8.5" cy="8.5" r="1.2" fill="currentColor"/><circle cx="15.5" cy="15.5" r="1.2" fill="currentColor"/><circle cx="15.5" cy="8.5" r="1.2" fill="currentColor"/><circle cx="8.5" cy="15.5" r="1.2" fill="currentColor"/><circle cx="12" cy="12" r="1.2" fill="currentColor"/>',
    heart: '<path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.6l-1-1a5.5 5.5 0 0 0-7.8 7.8l1 1L12 21l7.8-7.6 1-1a5.5 5.5 0 0 0 0-7.8z"/><path d="M3.5 12h4l2-3 3 6 2-3h6"/>',
    act: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
    drive: '<path d="M22 12H2"/><path d="M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.1z"/><path d="M6 16h.01M10 16h.01"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
    gear: '<path d="M12.2 2h-.4a2 2 0 0 0-2 2v.2a2 2 0 0 1-1 1.7l-.4.3a2 2 0 0 1-2 0l-.2-.1a2 2 0 0 0-2.7.7l-.2.4a2 2 0 0 0 .7 2.7l.2.1a2 2 0 0 1 1 1.7v.5a2 2 0 0 1-1 1.7l-.2.1a2 2 0 0 0-.7 2.7l.2.4a2 2 0 0 0 2.7.7l.2-.1a2 2 0 0 1 2 0l.4.3a2 2 0 0 1 1 1.7v.2a2 2 0 0 0 2 2h.4a2 2 0 0 0 2-2v-.2a2 2 0 0 1 1-1.7l.4-.3a2 2 0 0 1 2 0l.2.1a2 2 0 0 0 2.7-.7l.2-.4a2 2 0 0 0-.7-2.7l-.2-.1a2 2 0 0 1-1-1.7v-.5a2 2 0 0 1 1-1.7l.2-.1a2 2 0 0 0 .7-2.7l-.2-.4a2 2 0 0 0-2.7-.7l-.2.1a2 2 0 0 1-2 0l-.4-.3a2 2 0 0 1-1-1.7V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2.5"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
    eyeOff: '<path d="M9.9 4.2A10 10 0 0 1 12 4c6.5 0 10 8 10 8a17 17 0 0 1-2.2 3.2"/><path d="M6.6 6.6A17 17 0 0 0 2 12s3.5 8 10 8a9.7 9.7 0 0 0 5.4-1.6"/><path d="m2 2 20 20"/><path d="M9.9 9.9a3 3 0 0 0 4.2 4.2"/>',
    edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
    plus: '<path d="M12 5v14"/><path d="M5 12h14"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    ok: '<path d="M20 6 9 17l-5-5"/>',
    err: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5"/><path d="M12 16.5v.01"/>',
    warn: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
    lock: '<rect x="4" y="11" width="16" height="10" rx="2.5"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    unlock: '<rect x="4" y="11" width="16" height="10" rx="2.5"/><path d="M8 11V7a4 4 0 0 1 7.7-1.5"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
    link: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    more: '<circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/>',
    undo: '<path d="M3 7v6h6"/><path d="M21 17a9 9 0 0 0-15-6.7L3 13"/>',
    folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z"/>',
    usb: '<path d="M10 7V4h4v3"/><rect x="7" y="7" width="10" height="9" rx="2"/><path d="M12 16v5"/>',
    upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m17 8-5-5-5 5"/><path d="M12 3v12"/>',
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>',
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
    shieldOk: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/>',
    globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    browser: '<rect x="2" y="4" width="20" height="16" rx="2.5"/><path d="M2 9h20"/><path d="M6 6.5h.01M9 6.5h.01"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    cpu: '<rect x="5" y="5" width="14" height="14" rx="2"/><path d="M9 1v4M15 1v4M9 19v4M15 19v4M1 9h4M1 15h4M19 9h4M19 15h4"/>',
    wand: '<path d="m15 4-1 1"/><path d="M19.5 8.5 21 7"/><path d="m3 21 12-12"/><path d="M16 3v2M20 7h2M19 3l-1 1M14.5 9.5 17 12"/>',
    zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${I[n] || ''}</svg>`;

  async function call(cmd, args) {
    const r = await api.Call(cmd, args === undefined ? null : args);
    if (!r || !r.ok) {
      const e = new Error((r && r.error) || 'Error');
      e.code = r && r.code;
      throw e;
    }
    return r.data === undefined ? null : r.data;
  }

  // Que el navegador interno no recuerde lo que se escribe en ningún campo.
  const noAuto = (root) => root.querySelectorAll && root.querySelectorAll('input, textarea').forEach((x) => { if (!x.hasAttribute('autocomplete')) x.setAttribute('autocomplete', 'off'); });
  new MutationObserver((ms) => ms.forEach((m) => m.addedNodes.forEach((n) => n.nodeType === 1 && (noAuto(n), n.matches && n.matches('input, textarea') && !n.hasAttribute('autocomplete') && n.setAttribute('autocomplete', 'off'))))).observe(document.documentElement, { childList: true, subtree: true });
  noAuto(document);

  // ---------- avisos ----------
  function toast(msg, kind = 'ok', opts = {}) {
    const el = document.createElement('div');
    el.className = `toast ${kind}`;
    const iconName = kind === 'error' ? 'err' : kind === 'secret' ? 'shieldOk' : kind === 'warn' ? 'warn' : 'ok';
    el.innerHTML = `${ic(iconName)}<span>${esc(msg)}</span>`;
    if (opts.action) {
      const b = document.createElement('button');
      b.className = 'btn xs ghost';
      b.textContent = opts.action;
      b.onclick = () => { opts.onAction(); close(); };
      el.appendChild(b);
    }
    let bar;
    if (opts.bar) {
      bar = document.createElement('i');
      bar.className = 'bar';
      el.appendChild(bar);
      bar.animate([{ transform: 'scaleX(1)' }, { transform: 'scaleX(0)' }], { duration: opts.bar * 1000, easing: 'linear', fill: 'forwards' });
    }
    $('#toasts').appendChild(el);
    const ttl = opts.ttl || (opts.bar ? opts.bar * 1000 : kind === 'error' ? 6000 : 3200);
    const t = setTimeout(close, ttl);
    function close() { clearTimeout(t); el.classList.add('out'); setTimeout(() => el.remove(), 260); }
    return close;
  }
  const fail = (e) => toast(e.message || String(e), 'error');

  // ---------- modales ----------
  function modal({ title, body, foot, narrow = true, onClose, cls = '' }) {
    const m = document.createElement('div');
    m.className = 'modal';
    m.innerHTML = `<div class="modal-card ${narrow ? 'narrow' : ''} ${cls}">
      <div class="modal-head"><h2>${esc(title)}</h2><button class="ibtn" data-close title="Cerrar">${ic('x')}</button></div>
      <div class="modal-body">${body}</div>${foot ? `<div class="modal-foot">${foot}</div>` : ''}</div>`;
    $('#app').appendChild(m);
    const close = () => { m.remove(); document.removeEventListener('keydown', onKey, true); onClose && onClose(); };
    const onKey = (e) => { if (e.key === 'Escape') { e.stopPropagation(); close(); } };
    document.addEventListener('keydown', onKey, true);
    m.addEventListener('mousedown', (e) => { if (e.target === m) close(); });
    $$('[data-close]', m).forEach((b) => (b.onclick = close));
    const first = $('input:not([type=checkbox]), textarea', m);
    if (first) setTimeout(() => first.focus(), 30);
    return { el: m, close, shake: () => { const c = $('.modal-card', m); c.classList.remove('shake'); void c.offsetWidth; c.classList.add('shake'); } };
  }

  function confirmBox(title, text, okText = 'Aceptar', danger = false) {
    return new Promise((resolve) => {
      let done = false;
      const m = modal({
        title, body: `<p class="mtext">${text}</p>`,
        foot: `<button class="btn ghost sm" data-close>Cancelar</button><button class="btn ${danger ? 'danger-solid' : 'primary'} sm" id="cf-ok">${esc(okText)}</button>`,
        onClose: () => { if (!done) resolve(false); },
      });
      $('#cf-ok', m.el).onclick = () => { done = true; m.close(); resolve(true); };
      setTimeout(() => $('#cf-ok', m.el).focus(), 30);
    });
  }

  // Pide la contraseña maestra (para exportar, restaurar…).
  function askMaster(title, text, okText, check) {
    return new Promise((resolve) => {
      let done = false;
      const m = modal({
        title,
        body: `<p class="mtext">${text}</p>
          <label class="field"><span>Contraseña maestra</span><div class="pw"><input type="password" id="am-pw" autocomplete="off" spellcheck="false"></div></label>
          <div class="hint bad" id="am-err"></div>`,
        foot: `<button class="btn ghost sm" data-close>Cancelar</button><button class="btn primary sm" id="am-ok">${esc(okText)}</button>`,
        onClose: () => { if (!done) resolve(null); },
      });
      const go = async () => {
        const pw = $('#am-pw', m.el).value;
        if (!pw) return;
        const b = $('#am-ok', m.el);
        b.classList.add('busy');
        try {
          const res = await check(pw);
          done = true;
          m.close();
          resolve(res === undefined ? pw : res);
        } catch (e) {
          $('#am-err', m.el).textContent = e.message;
          m.shake();
          $('#am-pw', m.el).select();
        } finally { b.classList.remove('busy'); }
      };
      $('#am-ok', m.el).onclick = go;
      $('#am-pw', m.el).addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
    });
  }

  // ---------- menú contextual ----------
  let ctxEl = null;
  function closeCtx() { if (ctxEl) { ctxEl.remove(); ctxEl = null; } }
  function ctxMenu(x, y, items) {
    closeCtx();
    const m = document.createElement('div');
    m.className = 'ctx';
    for (const it of items) {
      if (it === '-') { m.appendChild(document.createElement('hr')); continue; }
      const b = document.createElement('button');
      if (it.danger) b.className = 'danger';
      b.innerHTML = `${ic(it.icon)}<span>${esc(it.label)}</span>`;
      b.onclick = () => { closeCtx(); it.run(); };
      m.appendChild(b);
    }
    $('#app').appendChild(m);
    const r = m.getBoundingClientRect();
    m.style.left = Math.min(x, innerWidth - r.width - 8) + 'px';
    m.style.top = Math.min(y, innerHeight - r.height - 8) + 'px';
    ctxEl = m;
  }
  document.addEventListener('mousedown', (e) => { if (ctxEl && !ctxEl.contains(e.target)) closeCtx(); });
  document.addEventListener('contextmenu', (e) => { if (!e.target.closest('input, textarea')) e.preventDefault(); });

  // ---------- estado ----------
  const S = {
    state: null, skew: 0, mode: '', agentOk: true,
    items: [], trash: [], view: 'items', filter: 'all', q: '', sort: 'name',
    sel: null, detail: null, reveal: false, revealTimer: 0,
    health: null, healthTab: 'weak', settings: null, gen: null, genOpts: null,
    lockStep: 'welcome', pairShown: false,
  };

  const AV_COLORS = ['#7c3aed', '#4f46e5', '#0891b2', '#059669', '#d97706', '#db2777', '#2563eb', '#9333ea', '#0d9488', '#dc2626', '#65a30d', '#c026d3'];
  function hostOf(u) {
    try { return new URL(u.includes('://') ? u : 'https://' + u).hostname.replace(/^www\./, ''); } catch { return ''; }
  }
  function avatar(it, cls = '') {
    const host = (it.urls && it.urls[0] && hostOf(it.urls[0])) || '';
    const base = host || it.title || '?';
    let h = 0;
    for (const c of base) h = (h * 31 + c.charCodeAt(0)) >>> 0;
    const color = AV_COLORS[h % AV_COLORS.length];
    const inner = it.type === 'note' ? ic('note') : esc((it.title || host || '?').trim().charAt(0) || '?');
    return `<div class="av ${cls}" style="--avc:${color}">${inner}</div>`;
  }
  const SCORE_LABEL = ['Muy débil', 'Débil', 'Aceptable', 'Fuerte', 'Muy fuerte'];
  const SCORE_COLOR = ['#f43f5e', '#fb7185', '#fbbf24', '#34d399', '#22d3ee'];

  const fmtMin = (m) => (m >= 60 ? `${m / 60} h` : `${m} min`);

  function fmtDate(sec) {
    if (!sec) return '—';
    const d = new Date(sec * 1000);
    return d.toLocaleDateString('es-ES', { day: 'numeric', month: 'short', year: 'numeric' });
  }
  function ago(ms) {
    const s = Math.round((Date.now() - ms) / 1000);
    if (s < 60) return 'ahora mismo';
    if (s < 3600) return `hace ${Math.round(s / 60)} min`;
    if (s < 86400) return `hace ${Math.round(s / 3600)} h`;
    const d = Math.round(s / 86400);
    return d === 1 ? 'ayer' : `hace ${d} días`;
  }

  // ---------- conexión con el agente ----------
  rt.EventsOn('state', (st) => onState(typeof st === 'string' ? JSON.parse(st) : st));
  rt.EventsOn('agent', (a) => onAgent(a));
  rt.EventsOn('prompt', (m) => onPrompt(m));

  function onAgent(a) {
    S.agentOk = a.ok;
    const el = $('#agent-down');
    if (a.ok) { el.hidden = true; refreshState(); return; }
    el.hidden = false;
    el.innerHTML = `<div class="lock-card"><div class="spinner" style="margin-bottom:16px"></div>
      <h1>Conectando con <b>TurboVAULT</b>…</h1><p class="sub">${esc(a.error || '')}</p>
      ${a.other ? `<button class="btn ghost sm" id="kill-other">${ic('x')}Cerrar el otro TurboVAULT</button>` : ''}</div>`;
    const k = $('#kill-other');
    if (k) k.onclick = async () => { try { await api.KillOtherAgent(); toast('Cerrado. Reconectando…'); } catch (e) { fail(e); } };
  }

  async function refreshState() {
    try { onState(await call('state')); } catch { /* llegará por evento */ }
  }

  function onState(st) {
    if (!st) return;
    const prev = S.state;
    S.state = st;
    S.skew = Date.now() - st.now;
    const wasOpen = prev && prev.unlocked;
    document.body.classList.toggle('locked', !st.unlocked);
    if (!st.unlocked) {
      if (wasOpen || !prev) wipe();
      renderLock();
    } else {
      $('#lock').hidden = S.mode === '' ? true : $('#lock').hidden;
      if (!wasOpen) onUnlocked();
      else if (prev.count !== st.count) loadItems();
      else if (prev.extBad !== st.extBad && S.view === 'items') renderBanners();
    }
    $('#session').hidden = !st.unlocked || S.mode !== '';
    $('#wb-lock').hidden = !st.unlocked;
    tickSession();
    if (st.unlocked && st.pair && !S.pairShown) showPair(st.pair);
    if (!st.pair && S.pairModal) { S.pairModal.close(); S.pairModal = null; }
  }

  function onPrompt(m) {
    if (m === 'unlock' || m === 'pair') {
      if (S.mode === '' && S.state && S.state.unlocked) {
        if (m === 'pair' && S.state.pair) showPair(S.state.pair);
        return;
      }
      if (S.mode === '' && S.state && !S.state.unlocked) { renderLock(); return; }
      setMode(m);
    } else if (S.mode !== '') {
      setMode('');
    }
  }

  async function setMode(m) {
    S.mode = m;
    document.body.classList.toggle('compact', m !== '');
    try { await api.SetMode(m); } catch { /* sin ventana */ }
    if (S.state) onState(S.state);
    if (m === '' && S.state && S.state.unlocked) { $('#lock').hidden = true; loadItems(); renderView(); }
  }

  // Borra todo lo que se haya enseñado al cerrarse la bóveda.
  function wipe() {
    S.items = []; S.trash = []; S.sel = null; S.detail = null; S.reveal = false; S.health = null;
    clearTimeout(S.revealTimer);
    $('#list').innerHTML = '';
    $('#detail').innerHTML = '';
    ['#gen', '#health', '#activity', '#backup', '#settings'].forEach((s) => ($(s).innerHTML = ''));
    $$('.modal').forEach((m) => m.remove());
    $('#side').innerHTML = '';
    $('#count').textContent = '';
    $('#banners').innerHTML = '';
    qIn.value = ''; S.q = ''; $('#q-wrap').classList.remove('has');
    closeCtx();
    stopOtp();
  }

  async function onUnlocked() {
    if (S.mode === 'unlock') {
      showUnlockedDone();
      return;
    }
    if (S.mode === 'pair') {
      if (S.state.pair) showPair(S.state.pair);
      return;
    }
    $('#lock').hidden = true;
    await loadItems();
    renderView();
    autoBreach();
  }

  async function autoBreach() {
    try { S.settings = await call('settings'); } catch { return; }
    if (!S.settings.autoBreach) return;
    try {
      const h = await call('breach');
      S.health = h; renderSide();
      if (S.view === 'health') renderHealth();
      if (h.breached && h.breached.length) toast(`${h.breached.length} ${h.breached.length === 1 ? 'contraseña sale' : 'contraseñas salen'} en filtraciones`, 'warn', { ttl: 9000, action: 'Ver', onAction: () => { S.healthTab = 'breached'; go('health'); } });
    } catch { /* sin internet: se intenta la próxima vez */ }
  }

  // ---------- temporizador de la sesión ----------
  setInterval(tickSession, 1000);
  function tickSession() {
    const st = S.state;
    if (!st || !st.unlocked || !st.expires) return;
    const left = Math.max(0, st.expires - (Date.now() - S.skew));
    const total = (st.sessionMin || 10) * 60000;
    const h = Math.floor(left / 3600000), m = Math.floor((left % 3600000) / 60000), s = Math.floor((left % 60000) / 1000);
    $('#session-left').textContent = h ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`;
    $('#session-ring').style.strokeDashoffset = String(47.12 * (1 - left / total));
    $('#session').classList.toggle('soon', left < 60000);
    if (st.waitUntil) renderWait();
  }
  // Clic en el contador: a Ajustes, al selector del tiempo.
  $('#session').onclick = async () => {
    if (S.mode !== '' || !S.state || !S.state.unlocked) return;
    go('settings');
    for (let i = 0; i < 20 && !$('#st-session'); i++) await sleep(50);
    const row = $('#st-session-row');
    if (row) { row.scrollIntoView({ block: 'center', behavior: 'smooth' }); row.classList.add('flash'); setTimeout(() => row.classList.remove('flash'), 1400); $('#st-session').focus(); }
  };

  // ---------- ventana ----------
  $('#wb-min').onclick = () => { lockIfMin(); rt.WindowMinimise(); };
  $('#wb-max').onclick = () => rt.WindowToggleMaximise();
  $('#wb-close').onclick = async () => {
    if (S.mode === '' && S.state && S.state.unlocked && S.settings && S.settings.lockOnClose) { try { await call('lock'); } catch { /* nada */ } }
    rt.Quit();
  };
  // «Cerrar al minimizar»: con el botón, con la barra de tareas o con Win+D.
  function lockIfMin() {
    if (S.mode === '' && S.state && S.state.unlocked && S.settings && S.settings.lockOnMin) lockNow();
  }
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'hidden') lockIfMin(); });
  setInterval(async () => {
    if (!(S.settings && S.settings.lockOnMin && S.state && S.state.unlocked) || !rt.WindowIsMinimised) return;
    try { if (await rt.WindowIsMinimised()) lockIfMin(); } catch { /* nada */ }
  }, 1000);
  $('#wb-lock').onclick = () => lockNow();
  async function lockNow() {
    try { await call('lock'); } catch (e) { fail(e); }
  }

  // =====================================================================
  // pantalla de bloqueo / creación
  // =====================================================================
  function renderLock() {
    const st = S.state;
    const el = $('#lock');
    el.hidden = false;
    if (S.mode === '' && st && st.unlocked) { el.hidden = true; return; }
    if (st && st.damaged) return renderDamaged(el);
    if (st && !st.exists) return renderCreate(el);
    renderUnlock(el);
  }

  function renderDamaged(el) {
    el.innerHTML = `<div class="lock-card">
      <div class="ok-big" style="background:rgba(248,113,113,.12);color:var(--bad)">${ic('warn')}</div>
      <h1>No se puede leer la bóveda</h1>
      <p class="sub">${esc(S.state.damaged)}. Puedes restaurar una copia de seguridad.</p>
      <button class="btn primary" id="lk-restore">${ic('upload')}Restaurar una copia</button>
      <div class="lock-links"><button class="link" id="lk-folder">Abrir la carpeta de datos</button></div></div>`;
    $('#lk-restore').onclick = restoreFlow;
    $('#lk-folder').onclick = async () => api.ShowInFolder(await api.DataFolder());
  }

  function renderUnlock(el) {
    const st = S.state || {};
    let sub = 'Escribe tu contraseña maestra para abrir la bóveda.';
    if (S.mode === 'unlock') sub = 'El navegador necesita tu bóveda. Escribe la contraseña maestra aquí: nunca pasa por el navegador.';
    if (S.mode === 'pair' || st.pair) sub = 'Abre la bóveda para vincular el navegador.';
    el.innerHTML = `<div class="lock-card" id="lk-card">
      <img class="logo" src="logo.svg" alt="">
      <h1>Turbo<b>VAULT</b></h1>
      <p class="sub">${esc(sub)}</p>
      <form id="lk-form" autocomplete="off">
        <label class="field big"><div class="pw"><input type="password" id="lk-pw" placeholder="Contraseña maestra" spellcheck="false" autocomplete="off">
          <button type="button" class="ibtn in-btn" id="lk-eye" title="Ver">${ic('eye')}</button></div></label>
        <div class="hint warn" id="lk-caps" hidden>Bloq Mayús está activado.</div>
        <button class="btn primary wide" id="lk-go" type="submit">${ic('unlock')}Abrir</button>
        <div class="lock-err" id="lk-err"></div>
      </form>
      <div class="lock-links">
        ${S.mode !== '' ? '<button class="link" id="lk-full">Abrir TurboVAULT completo</button>' : '<button class="link" id="lk-restore">Restaurar una copia</button>'}
      </div></div>`;
    const pw = $('#lk-pw');
    setTimeout(() => pw.focus(), 50);
    $('#lk-eye').onclick = () => { pw.type = pw.type === 'password' ? 'text' : 'password'; pw.focus(); };
    pw.addEventListener('keyup', (e) => { $('#lk-caps').hidden = !(e.getModifierState && e.getModifierState('CapsLock')); });
    $('#lk-form').onsubmit = async (e) => {
      e.preventDefault();
      if (!pw.value) return;
      const b = $('#lk-go');
      b.classList.add('busy');
      $('#lk-err').textContent = '';
      try {
        await call('unlock', { master: pw.value });
        pw.value = '';
      } catch (err) {
        $('#lk-err').textContent = err.code === 'wrong' ? 'Contraseña maestra incorrecta.' : err.message;
        const c = $('#lk-card');
        c.classList.remove('shake'); void c.offsetWidth; c.classList.add('shake');
        pw.select();
      } finally { b.classList.remove('busy'); }
    };
    const r = $('#lk-restore');
    if (r) r.onclick = restoreFlow;
    const f = $('#lk-full');
    if (f) f.onclick = () => setMode('');
    renderWait();
  }

  function renderWait() {
    const st = S.state;
    const b = $('#lk-go');
    if (!b || !st) return;
    const left = st.waitUntil ? Math.ceil((st.waitUntil - (Date.now() - S.skew)) / 1000) : 0;
    if (left > 0) {
      b.disabled = true;
      $('#lk-err').textContent = `Demasiados intentos fallidos. Espera ${left} s.`;
    } else if (b.disabled) {
      b.disabled = false;
      $('#lk-err').textContent = '';
    }
  }

  function showUnlockedDone() {
    const el = $('#lock');
    el.hidden = false;
    el.innerHTML = `<div class="lock-card"><div class="ok-big">${ic('ok')}</div>
      <h1>Bóveda abierta</h1><p class="sub">Vuelve al navegador. Se cerrará sola en ${fmtMin(S.state.sessionMin)}.</p></div>`;
    setTimeout(() => { if (S.mode === 'unlock') rt.Quit(); }, 1100);
  }

  // ---------- crear la bóveda ----------
  function renderCreate(el) {
    if (S.lockStep === 'welcome') {
      el.innerHTML = `<div class="lock-card wide">
        <img class="logo" src="logo.svg" alt="">
        <h1>Bienvenido a Turbo<b>VAULT</b></h1>
        <p class="sub">Tus contraseñas, cifradas y guardadas solo en este PC. Nada se sube a ninguna nube.</p>
        <div class="feat">
          <div>${ic('lock')}<b>AES-256</b>Cifrado de verdad</div>
          <div>${ic('cpu')}<b>Argon2id</b>Lento para atacantes</div>
          <div>${ic('usb')}<b>Copias</b>En tu disco o USB</div>
        </div>
        <button class="btn primary wide" id="cr-start">${ic('key')}Crear mi bóveda</button>
        <div class="lock-links"><button class="link" id="lk-restore">Ya tengo una copia de TurboVAULT</button></div></div>`;
      $('#cr-start').onclick = () => { S.lockStep = 'master'; renderLock(); };
      $('#lk-restore').onclick = restoreFlow;
      return;
    }
    el.innerHTML = `<div class="lock-card wide" id="cr-card">
      <div class="steps"><i class="on"></i><i class="on"></i><i></i></div>
      <h1>Elige tu contraseña maestra</h1>
      <p class="sub">Es la única que tendrás que recordar. No se guarda en ningún sitio: si la olvidas, nadie (tampoco TurboVAULT) puede recuperar tus datos.</p>
      <form id="cr-form" autocomplete="off">
        <label class="field"><span>Contraseña maestra <em>· mínimo 12 caracteres</em></span>
          <div class="pw"><input type="password" id="cr-pw" spellcheck="false" autocomplete="off">
          <button type="button" class="ibtn in-btn" id="cr-eye" title="Ver">${ic('eye')}</button></div>
          <div class="strength"><i id="cr-bar"></i></div>
          <div class="sinfo"><b id="cr-label">&nbsp;</b><span id="cr-crack"></span></div>
          <div class="hint" id="cr-warn"></div>
        </label>
        <button type="button" class="btn ghost sm" id="cr-suggest">${ic('dice')}Sugerir una frase segura</button>
        <label class="field" style="margin-top:14px"><span>Repítela</span>
          <div class="pw"><input type="password" id="cr-pw2" spellcheck="false" autocomplete="off"></div></label>
        <label class="chk"><input type="checkbox" id="cr-ok"><span>Entiendo que si olvido la contraseña maestra <b>no hay forma de recuperar</b> mis contraseñas. La apuntaré en papel y la guardaré en un sitio seguro.</span></label>
        <button class="btn primary wide" id="cr-go" type="submit" style="margin-top:16px" disabled>${ic('lock')}Crear bóveda</button>
        <div class="lock-err" id="cr-err"></div>
      </form></div>`;
    const pw = $('#cr-pw'), pw2 = $('#cr-pw2'), ok = $('#cr-ok'), go = $('#cr-go');
    let score = 0;
    setTimeout(() => pw.focus(), 50);
    $('#cr-eye').onclick = () => { const t = pw.type === 'password' ? 'text' : 'password'; pw.type = t; pw2.type = t; };
    const check = debounce(async () => {
      if (!pw.value) { meter('#cr', null); score = 0; upd(); return; }
      try {
        const s = await call('strength', { password: pw.value, master: true });
        score = pw.value.length >= 12 ? s.score : Math.min(s.score, 1);
        meter('#cr', s, pw.value.length < 12 ? 'Faltan caracteres: mínimo 12.' : '');
      } catch { /* nada */ }
      upd();
    }, 120);
    const upd = () => {
      go.disabled = !(score >= 3 && pw.value === pw2.value && ok.checked);
      $('#cr-err').textContent = pw2.value && pw.value !== pw2.value ? 'No coinciden.' : '';
    };
    pw.addEventListener('input', check);
    pw2.addEventListener('input', upd);
    ok.addEventListener('change', upd);
    $('#cr-suggest').onclick = async () => {
      try {
        const g = await call('suggest');
        pw.value = g.password; pw2.value = ''; pw.type = 'text'; pw2.type = 'text';
        check();
        toast('Apúntala en papel antes de seguir. Después repítela abajo.', 'ok', { ttl: 5000 });
        pw2.focus();
      } catch (e) { fail(e); }
    };
    $('#cr-form').onsubmit = async (e) => {
      e.preventDefault();
      if (go.disabled) return;
      go.classList.add('busy');
      try {
        await call('create', { master: pw.value });
        pw.value = ''; pw2.value = '';
        S.lockStep = 'done';
        showCreated();
      } catch (err) { $('#cr-err').textContent = err.message; } finally { go.classList.remove('busy'); }
    };
  }

  function showCreated() {
    const el = $('#lock');
    el.hidden = false;
    el.innerHTML = `<div class="lock-card wide">
      <div class="steps"><i class="on"></i><i class="on"></i><i class="on"></i></div>
      <div class="ok-big">${ic('ok')}</div>
      <h1>Tu bóveda está lista</h1>
      <p class="sub">Tres cosas recomendadas para empezar:</p>
      <div class="tlist" style="width:100%;text-align:left">
        <div class="titem"><div class="tile" style="--tile:linear-gradient(135deg,#f59e0b,#ef4444)">${ic('download')}</div><div class="txt"><b>Traer tus contraseñas de Chrome</b><span>Y borrarlas de Chrome después.</span></div><button class="btn ghost sm" data-go="import">Importar</button></div>
        <div class="titem"><div class="tile" style="--tile:linear-gradient(135deg,#10b981,#06b6d4)">${ic('usb')}</div><div class="txt"><b>Copia automática en un disco externo</b><span>Por si pierdes o se rompe el PC.</span></div><button class="btn ghost sm" data-go="backup">Configurar</button></div>
        <div class="titem"><div class="tile">${ic('browser')}</div><div class="txt"><b>Extensión de Chrome</b><span>Para rellenar contraseñas al iniciar sesión.</span></div><button class="btn ghost sm" data-go="ext">Preparar</button></div>
      </div>
      <button class="btn primary wide" id="cr-enter" style="margin-top:16px">Entrar en mi bóveda</button></div>`;
    const enter = async (view) => {
      $('#lock').hidden = true;
      await loadItems();
      go(view || 'items');
    };
    $('#cr-enter').onclick = () => enter();
    $$('[data-go]', el).forEach((b) => (b.onclick = () => {
      const g = b.dataset.go;
      if (g === 'import') { enter('backup').then(importFlow); return; }
      if (g === 'backup') { enter('backup'); return; }
      enter('settings').then(setupExtension);
    }));
  }

  // medidor de fuerza compartido (prefijo de ids)
  function meter(prefix, s, extra = '') {
    const bar = $(prefix + '-bar'), label = $(prefix + '-label'), crack = $(prefix + '-crack'), warn = $(prefix + '-warn');
    if (!s) {
      if (bar) bar.style.width = '0';
      if (label) label.innerHTML = '&nbsp;';
      if (crack) crack.textContent = '';
      if (warn) warn.textContent = '';
      return;
    }
    if (bar) { bar.style.width = `${(s.score + 1) * 20}%`; bar.style.background = SCORE_COLOR[s.score]; }
    if (label) { label.textContent = s.label; label.style.color = SCORE_COLOR[s.score]; }
    if (crack) crack.textContent = s.crack === 'al instante' ? 'Se adivina al instante' : `Tardarían ${s.crack} en adivinarla`;
    if (warn) {
      warn.textContent = extra || s.warning || (s.tips && s.tips[0]) || '';
      warn.className = 'hint' + (s.score < 3 ? ' warn' : '');
    }
  }

  // ---------- vincular navegador ----------
  function showPair(p) {
    S.pairShown = true;
    if (S.pairModal) return;
    const m = modal({
      title: 'Vincular navegador',
      body: `<div style="text-align:center;padding:6px 0 4px">
        <div class="tile" style="margin:0 auto 12px">${ic('browser')}</div>
        <p class="mtext"><b>${esc(p.browser || 'Un navegador')}</b> quiere usar TurboVAULT para rellenar contraseñas.</p>
        <div class="pair-code">${esc(p.code)}</div>
        <p class="mtext">Comprueba que la extensión enseña <b>el mismo código</b>. Si no has sido tú, pulsa «Rechazar».</p></div>`,
      foot: `<button class="btn ghost sm" id="pr-no">Rechazar</button><button class="btn primary sm" id="pr-yes">${ic('ok')}Vincular</button>`,
      onClose: () => { S.pairModal = null; S.pairShown = false; },
    });
    S.pairModal = m;
    const finish = () => { if (S.mode === 'pair') setTimeout(() => rt.Quit(), 500); };
    $('#pr-yes', m.el).onclick = async () => {
      try { await call('approvePair', { code: p.code }); m.close(); toast('Navegador vinculado'); finish(); } catch (e) { fail(e); }
    };
    $('#pr-no', m.el).onclick = async () => {
      try { await call('rejectPair', {}); } catch { /* nada */ }
      m.close(); finish();
    };
  }

  // =====================================================================
  // navegación
  // =====================================================================
  const NAV = [
    { sec: 'Bóveda' },
    { id: 'all', label: 'Todo', icon: 'all' },
    { id: 'fav', label: 'Favoritas', icon: 'star' },
    { id: 'login', label: 'Inicios de sesión', icon: 'key' },
    { id: 'note', label: 'Notas seguras', icon: 'note' },
    { id: 'otp', label: 'Con 2FA', icon: 'otp' },
    { sec: 'Herramientas' },
    { id: 'gen', label: 'Generador', icon: 'dice', view: 'gen' },
    { id: 'health', label: 'Salud', icon: 'heart', view: 'health' },
    { id: 'activity', label: 'Actividad', icon: 'act', view: 'activity' },
    { sec: 'Ajustes' },
    { id: 'backup', label: 'Copias e importar', icon: 'drive', view: 'backup' },
    { id: 'trash', label: 'Papelera', icon: 'trash' },
    { id: 'settings', label: 'Ajustes', icon: 'gear', view: 'settings' },
  ];

  function renderSide() {
    const counts = {
      all: S.items.length,
      fav: S.items.filter((i) => i.fav).length,
      login: S.items.filter((i) => i.type !== 'note').length,
      note: S.items.filter((i) => i.type === 'note').length,
      otp: S.items.filter((i) => i.hasTotp).length,
      trash: S.trash.length,
    };
    const cur = S.view === 'items' ? S.filter : S.view;
    let h = '';
    for (const n of NAV) {
      if (n.sec) { h += `<div class="sec">${esc(n.sec)}</div>`; continue; }
      let right = counts[n.id] !== undefined && counts[n.id] > 0 ? `<small>${counts[n.id]}</small>` : '';
      if (n.id === 'health' && S.health) {
        const sc = S.health.score;
        right = `<span class="badge ${sc >= 80 ? 'good' : sc >= 50 ? 'mid' : 'bad'}">${sc}</span>`;
      }
      h += `<button class="nav-i ${cur === n.id ? 'on' : ''}" data-nav="${n.id}">${ic(n.icon)}<span>${esc(n.label)}</span>${right}</button>`;
    }
    h += `<div class="side-foot">${ic('shieldOk')}Cifrada en este PC</div>`;
    $('#side').innerHTML = h;
    $$('[data-nav]').forEach((b) => (b.onclick = () => {
      const n = NAV.find((x) => x.id === b.dataset.nav);
      go(n.view || 'items', n.view ? null : n.id);
    }));
  }

  function go(view, filter) {
    S.view = view;
    if (filter) { S.filter = filter; S.sel = null; S.detail = null; stopOtp(); }
    $$('.view').forEach((v) => v.classList.remove('on'));
    $('#v-' + view).classList.add('on');
    renderSide();
    renderView();
  }

  function renderView() {
    if (!S.state || !S.state.unlocked) return;
    switch (S.view) {
      case 'items': renderList(); renderDetail(); break;
      case 'gen': renderGen(); break;
      case 'health': renderHealth(); break;
      case 'activity': renderActivity(); break;
      case 'backup': renderBackup(); break;
      case 'settings': renderSettings(); break;
    }
  }

  async function loadItems() {
    try {
      const [items, trash] = await Promise.all([call('items', { trash: false }), call('items', { trash: true })]);
      S.items = items || [];
      S.trash = trash || [];
    } catch (e) { if (e.code !== 'locked') fail(e); return; }
    renderSide();
    if (S.view === 'items') { renderList(); if (S.sel && !findItem(S.sel)) { S.sel = null; S.detail = null; renderDetail(); } }
    if (!S.health) call('health').then((h) => { S.health = h; renderSide(); }).catch(() => {});
  }
  const findItem = (id) => S.items.find((i) => i.id === id) || S.trash.find((i) => i.id === id);

  // =====================================================================
  // lista
  // =====================================================================
  const qIn = $('#q');
  qIn.addEventListener('input', () => { S.q = qIn.value; $('#q-wrap').classList.toggle('has', !!S.q); renderList(); });
  $('#q-clear').onclick = () => { qIn.value = ''; S.q = ''; $('#q-wrap').classList.remove('has'); renderList(); qIn.focus(); };
  $('#sort').onchange = (e) => { S.sort = e.target.value; renderList(); };
  $('#add').onclick = () => openEditor(null);

  function visibleItems() {
    let list = S.filter === 'trash' ? S.trash.slice() : S.items.slice();
    switch (S.filter) {
      case 'fav': list = list.filter((i) => i.fav); break;
      case 'login': list = list.filter((i) => i.type !== 'note'); break;
      case 'note': list = list.filter((i) => i.type === 'note'); break;
      case 'otp': list = list.filter((i) => i.hasTotp); break;
    }
    const q = S.q.trim().toLowerCase();
    if (q) {
      const words = q.split(/\s+/);
      list = list.filter((i) => {
        const hay = `${i.title} ${i.username || ''} ${(i.urls || []).join(' ')} ${(i.tags || []).join(' ')}`.toLowerCase();
        return words.every((w) => hay.includes(w));
      });
    }
    const byName = (a, b) => a.title.localeCompare(b.title, 'es', { sensitivity: 'base' });
    switch (S.sort) {
      case 'used': list.sort((a, b) => (b.used || 0) - (a.used || 0) || byName(a, b)); break;
      case 'updated': list.sort((a, b) => (b.updated || 0) - (a.updated || 0) || byName(a, b)); break;
      case 'weak': list.sort((a, b) => (a.hasPassword ? a.score : 9) - (b.hasPassword ? b.score : 9) || byName(a, b)); break;
      default: list.sort(byName);
    }
    return list;
  }

  function renderList() {
    if (S.view !== 'items') return;
    const list = visibleItems();
    const trash = S.filter === 'trash';
    $('#count').textContent = list.length ? `${list.length} ${list.length === 1 ? 'entrada' : 'entradas'}` : '';
    $('#add').hidden = trash;
    renderBanners();
    if (!list.length) {
      const empty = trash ? ['trash', 'La papelera está vacía', 'Lo que borres se queda aquí 30 días por si te arrepientes.']
        : S.q ? ['all', 'Nada coincide', 'Prueba con otra palabra.']
        : S.items.length ? ['all', 'Nada por aquí', 'Esta lista está vacía.']
        : ['key', 'Tu bóveda está vacía', 'Añade tu primera contraseña, impórtalas de Chrome o deja que la extensión las guarde al iniciar sesión.'];
      $('#list').innerHTML = `<div class="empty">${ic(empty[0])}<b>${esc(empty[1])}</b><p>${esc(empty[2])}</p>
        ${!S.items.length && !trash && !S.q ? `<div class="row-btns"><button class="btn primary sm" id="em-add">${ic('plus')}Nueva entrada</button><button class="btn ghost sm" id="em-imp">${ic('download')}Importar de Chrome</button></div>` : ''}</div>`;
      const a = $('#em-add'), b = $('#em-imp');
      if (a) a.onclick = () => openEditor(null);
      if (b) b.onclick = () => { go('backup'); importFlow(); };
      return;
    }
    let h = '';
    if (trash) h += `<div style="display:flex;justify-content:flex-end;padding:0 0 8px"><button class="btn ghost xs danger-txt" id="empty-trash">${ic('trash')}Vaciar papelera</button></div>`;
    for (const it of list) {
      const sub = it.type === 'note' ? 'Nota segura' : (it.username || (it.urls && it.urls[0] ? hostOf(it.urls[0]) : '') || '—');
      const dots = [];
      if (it.hasPassword && it.score <= 1) dots.push('<i class="weak" title="Contraseña débil"></i>');
      if (it.hasTotp) dots.push('<i class="otp" title="Con 2FA"></i>');
      const acts = trash
        ? `<button class="ibtn" data-act="untrash" title="Restaurar">${ic('undo')}</button>`
        : it.type === 'note' ? '' : `${it.username ? `<button class="ibtn" data-act="cuser" title="Copiar usuario">${ic('user')}</button>` : ''}${it.hasPassword ? `<button class="ibtn" data-act="cpass" title="Copiar contraseña">${ic('key')}</button>` : ''}`;
      h += `<div class="row-i ${S.sel === it.id ? 'sel' : ''}" data-id="${esc(it.id)}" tabindex="0">
        ${avatar(it)}
        <div class="t"><b><span>${esc(it.title)}</span>${it.fav ? ic('star') : ''}</b><small>${esc(sub)}</small></div>
        <div class="meta"><div class="dots">${dots.join('')}</div>${acts}</div></div>`;
    }
    $('#list').innerHTML = h;
    const et = $('#empty-trash');
    if (et) et.onclick = emptyTrash;
  }

  function renderBanners() {
    const b = $('#banners');
    let h = '';
    const st = S.settings;
    if (S.state && S.state.extBad) {
      h += `<div class="banner" style="background:rgba(244,63,94,.09);border-color:rgba(244,63,94,.35);color:#fecdd3">${ic('warn')}<div class="grow"><b>La extensión del navegador ha sido modificada</b> (${esc(S.state.extBad)}). No recibe contraseñas hasta que la reinstales.</div>
        <button class="btn xs ghost" id="bn-ext">Reinstalar</button></div>`;
    }
    if (S.filter === 'all' && S.items.length >= 3 && st && !(st.backups || []).length && !sessionStorage.getItem('hideBackupBanner')) {
      h += `<div class="banner">${ic('usb')}<div class="grow"><b>Aún no tienes copia automática.</b> Si pierdes o se rompe el PC, perderías tus contraseñas.</div>
        <button class="btn xs ghost" id="bn-go">Configurar</button><button class="ibtn" id="bn-x" title="Ahora no">${ic('x')}</button></div>`;
    }
    b.innerHTML = h;
    const g = $('#bn-go'), x = $('#bn-x'), be = $('#bn-ext');
    if (be) be.onclick = setupExtension;
    if (g) g.onclick = () => go('backup');
    if (x) x.onclick = () => { sessionStorage.setItem('hideBackupBanner', '1'); renderBanners(); };
  }

  $('#list').addEventListener('click', (e) => {
    const row = e.target.closest('.row-i');
    if (!row) return;
    const act = e.target.closest('[data-act]');
    if (act) {
      e.stopPropagation();
      const id = row.dataset.id;
      if (act.dataset.act === 'cuser') copyField(id, 'username');
      if (act.dataset.act === 'cpass') copyField(id, 'password');
      if (act.dataset.act === 'untrash') untrash(id);
      return;
    }
    select(row.dataset.id);
  });
  $('#list').addEventListener('dblclick', (e) => {
    const row = e.target.closest('.row-i');
    if (row && S.filter !== 'trash') { const it = findItem(row.dataset.id); if (it && it.hasPassword) copyField(it.id, 'password'); }
  });
  $('#list').addEventListener('contextmenu', (e) => {
    const row = e.target.closest('.row-i');
    if (!row) return;
    const it = findItem(row.dataset.id);
    if (!it) return;
    select(it.id);
    if (S.filter === 'trash') {
      ctxMenu(e.clientX, e.clientY, [
        { icon: 'undo', label: 'Restaurar', run: () => untrash(it.id) },
        { icon: 'trash', label: 'Borrar para siempre', danger: true, run: () => purge(it.id) },
      ]);
      return;
    }
    const items = [];
    if (it.username) items.push({ icon: 'user', label: 'Copiar usuario (Ctrl+B)', run: () => copyField(it.id, 'username') });
    if (it.hasPassword) items.push({ icon: 'key', label: 'Copiar contraseña (Ctrl+C)', run: () => copyField(it.id, 'password') });
    if (it.hasTotp) items.push({ icon: 'otp', label: 'Copiar código 2FA (Ctrl+Mayús+C)', run: () => copyField(it.id, 'totp') });
    if (items.length) items.push('-');
    items.push({ icon: 'edit', label: 'Editar', run: () => editSelected() });
    items.push({ icon: 'star', label: it.fav ? 'Quitar de favoritas' : 'Marcar como favorita', run: () => toggleFav(it) });
    items.push('-');
    items.push({ icon: 'trash', label: 'Mover a la papelera', danger: true, run: () => del(it.id) });
    ctxMenu(e.clientX, e.clientY, items);
  });

  async function select(id) {
    if (S.sel === id && S.detail) return;
    S.sel = id;
    S.reveal = false;
    S.detail = null;
    $$('.row-i').forEach((r) => r.classList.toggle('sel', r.dataset.id === id));
    renderDetail();
    try {
      const d = await call('get', { id });
      if (S.sel !== id) return;
      S.detail = d;
      renderDetail();
    } catch (e) { fail(e); }
  }

  // =====================================================================
  // detalle
  // =====================================================================
  let otpTimer = 0;
  function stopOtp() { clearInterval(otpTimer); otpTimer = 0; }

  function renderDetail() {
    const el = $('#detail');
    stopOtp();
    if (!S.sel) {
      el.innerHTML = `<div class="empty-d">${ic('shieldOk')}<b>Elige una entrada</b><span>O pulsa «Nueva» para guardar una contraseña.</span></div>`;
      return;
    }
    const base = findItem(S.sel);
    if (!base) { el.innerHTML = ''; return; }
    const d = S.detail;
    const it = d ? d.item : base;
    const sec = d ? d.secret : null;
    const trash = !!it.deleted;
    const host = it.urls && it.urls[0] ? hostOf(it.urls[0]) : '';
    let h = `<div class="d-hero"><div class="d-id">${avatar(it, 'lg')}
      <div class="txt"><h2>${esc(it.title)}</h2><div class="sub">${esc(it.type === 'note' ? 'Nota segura' : host || 'Sin web')}</div></div>
      ${trash ? '' : `<button class="ibtn star ${it.fav ? 'on' : ''}" id="d-fav" title="Favorita">${ic('star')}</button>`}
      <button class="ibtn" id="d-more" title="Más">${ic('more')}</button></div>`;
    const chips = [];
    if (it.hasPassword) chips.push(`<span class="chip s${it.score}">${ic('shield')}${SCORE_LABEL[it.score] || ''}</span>`);
    if (it.hasTotp) chips.push(`<span class="chip cy">${ic('otp')}2FA</span>`);
    if (it.uses) chips.push(`<span class="chip">${ic('zap')}Usada ${it.uses} ${it.uses === 1 ? 'vez' : 'veces'}</span>`);
    if (trash) chips.push(`<span class="chip s1">${ic('trash')}En la papelera</span>`);
    if (chips.length) h += `<div class="d-chips">${chips.join('')}</div>`;
    h += trash
      ? `<div class="d-actions"><button class="btn primary sm" id="d-untrash">${ic('undo')}Restaurar</button><button class="btn ghost sm danger-txt" id="d-purge">${ic('trash')}Borrar para siempre</button></div>`
      : `<div class="d-actions"><button class="btn primary sm" id="d-edit">${ic('edit')}Editar</button>${host ? `<button class="btn ghost sm" id="d-open" title="Abrir la web">${ic('link')}Abrir web</button>` : ''}</div>`;
    h += '</div>';

    if (!d) {
      el.innerHTML = h + '<div class="d-sec"><div class="spinner"></div></div>';
      bindDetail(it);
      return;
    }
    // campos
    if (it.type !== 'note') {
      h += '<div class="d-sec">';
      h += `<div class="frow"><div><div class="k">Usuario</div><div class="v ${it.username ? '' : 'none'}">${esc(it.username || 'Sin usuario')}</div></div>
        <div class="acts">${it.username ? `<button class="ibtn" data-copy="username" title="Copiar">${ic('copy')}</button>` : ''}</div></div>`;
      if (sec.password) {
        h += `<div class="frow"><div style="min-width:0"><div class="k">Contraseña</div><div class="v mono ${S.reveal ? 'reveal' : 'secret'}" id="d-pw">${S.reveal ? colorPw(sec.password) : '•'.repeat(Math.min(16, sec.password.length))}</div></div>
          <div class="acts"><button class="ibtn" id="d-eye" title="${S.reveal ? 'Ocultar' : 'Ver'}">${ic(S.reveal ? 'eyeOff' : 'eye')}</button><button class="ibtn" data-copy="password" title="Copiar">${ic('copy')}</button></div></div>`;
      }
      if (d.totp) {
        h += `<div class="frow"><div><div class="k">Código 2FA</div><div class="otp" id="d-otp"><span class="code">${esc(fmtOtp(d.totp.code))}</span>
          <svg class="ring" viewBox="0 0 20 20"><circle class="bgc" cx="10" cy="10" r="7.5" stroke-width="3" stroke="rgba(255,255,255,.12)"/><circle class="fg" id="otp-ring" cx="10" cy="10" r="7.5" stroke-width="3" stroke-dasharray="47.12"/></svg></div></div>
          <div class="acts"><button class="ibtn" data-copy="totp" title="Copiar">${ic('copy')}</button></div></div>`;
      }
      h += '</div>';
      if (it.urls && it.urls.length) {
        h += `<div class="d-sec"><div class="sec-h"><span class="label">Webs</span></div><div class="urls">`;
        for (const u of it.urls) {
          const insecure = /^http:\/\//i.test(u);
          h += `<div class="url">${insecure ? `<span class="warn" title="Sin https: la contraseña viaja sin cifrar">${ic('warn')}</span>` : ''}<span class="grow" title="Clic para copiar" data-curl="${esc(u)}">${esc(u)}</span><button class="ibtn sm" data-ourl="${esc(u)}" title="Abrir">${ic('link')}</button></div>`;
        }
        h += '</div></div>';
      }
    }
    if (sec.notes) {
      h += `<div class="d-sec"><div class="sec-h"><span class="label">Notas</span><button class="ibtn" data-copy="notes" title="Copiar">${ic('copy')}</button></div>
        <div class="notes-box ${it.type === 'note' || S.reveal ? '' : 'blur'}" id="d-notes" title="Clic para ver">${esc(sec.notes)}</div></div>`;
    }
    if (sec.fields && sec.fields.length) {
      h += `<div class="d-sec"><div class="sec-h"><span class="label">Campos</span></div>`;
      sec.fields.forEach((f, i) => {
        const hidden = f.hidden && !S.reveal;
        h += `<div class="frow"><div style="min-width:0"><div class="k">${esc(f.name || 'Campo')}</div><div class="v ${f.hidden ? 'mono' : ''} ${hidden ? 'secret' : ''}">${hidden ? '••••••••' : esc(f.value)}</div></div>
          <div class="acts"><button class="ibtn" data-copy="field:${i}" title="Copiar">${ic('copy')}</button></div></div>`;
      });
      h += '</div>';
    }
    if (sec.history && sec.history.length) {
      h += `<div class="d-sec"><div class="sec-h"><span class="label">Contraseñas anteriores</span><span class="mini">${sec.history.length}</span></div><div class="hist">`;
      sec.history.forEach((o, i) => {
        h += `<div class="frow"><div style="min-width:0"><div class="k">Hasta el ${esc(fmtDate(o.until))}</div><div class="v mono ${S.reveal ? 'reveal' : 'secret'}">${S.reveal ? esc(o.password) : '••••••••'}</div></div>
          <div class="acts"><button class="ibtn" data-copy="history:${i}" title="Copiar">${ic('copy')}</button></div></div>`;
      });
      h += '</div></div>';
    }
    h += `<div class="d-sec"><div class="facts">
      <div class="fact"><b>${esc(fmtDate(it.created))}</b><span>Creada</span></div>
      <div class="fact"><b>${esc(fmtDate(it.updated))}</b><span>Modificada</span></div>
      ${it.pwChanged ? `<div class="fact"><b>${esc(fmtDate(it.pwChanged))}</b><span>Contraseña cambiada</span></div>` : ''}
      <div class="fact"><b>${esc(it.used ? fmtDate(it.used) : 'Nunca')}</b><span>Último uso</span></div></div></div>`;
    el.innerHTML = h;
    bindDetail(it);
    if (d.totp) startOtp(it.id, d.totp);
  }

  const fmtOtp = (c) => (c.length === 6 ? c.slice(0, 3) + ' ' + c.slice(3) : c);
  function colorPw(p) {
    return [...p].map((ch) => (/\d/.test(ch) ? `<span class="d">${esc(ch)}</span>` : /[^\p{L}\d]/u.test(ch) ? `<span class="s">${esc(ch)}</span>` : esc(ch))).join('');
  }

  function startOtp(id, t) {
    let code = t.code, left = t.left, period = t.period || 30;
    const tick = async () => {
      const ring = $('#otp-ring'), box = $('#d-otp');
      if (!ring || S.sel !== id) { stopOtp(); return; }
      ring.style.strokeDashoffset = String(47.12 * (1 - left / period));
      box.classList.toggle('soon', left <= 5);
      $('.code', box).textContent = fmtOtp(code);
      left--;
      if (left < 0) {
        try { const n = await call('totp', { id }); code = n.code; left = n.left; period = n.period; } catch { stopOtp(); }
      }
    };
    tick();
    otpTimer = setInterval(tick, 1000);
  }

  function bindDetail(it) {
    const on = (sel, fn) => { const b = $(sel); if (b) b.onclick = fn; };
    on('#d-fav', () => toggleFav(it));
    on('#d-edit', () => editSelected());
    on('#d-untrash', () => untrash(it.id));
    on('#d-purge', () => purge(it.id));
    on('#d-open', () => api.OpenURL(it.urls[0].includes('://') ? it.urls[0] : 'https://' + it.urls[0]));
    on('#d-eye', () => toggleReveal());
    on('#d-notes', () => $('#d-notes').classList.remove('blur'));
    on('#d-more', (e) => {
      const r = e.currentTarget.getBoundingClientRect();
      const items = it.deleted ? [
        { icon: 'undo', label: 'Restaurar', run: () => untrash(it.id) },
        { icon: 'trash', label: 'Borrar para siempre', danger: true, run: () => purge(it.id) },
      ] : [
        { icon: 'edit', label: 'Editar', run: () => editSelected() },
        { icon: 'copy', label: 'Duplicar', run: () => duplicate() },
        '-',
        { icon: 'trash', label: 'Mover a la papelera', danger: true, run: () => del(it.id) },
      ];
      ctxMenu(r.left - 180, r.bottom + 6, items);
    });
    $$('[data-copy]', $('#detail')).forEach((b) => (b.onclick = () => copyField(it.id, b.dataset.copy, b)));
    $$('[data-curl]', $('#detail')).forEach((b) => (b.onclick = async () => { try { await call('copyText', { text: b.dataset.curl, secret: false }); toast('Web copiada'); } catch (e) { fail(e); } }));
    $$('[data-ourl]', $('#detail')).forEach((b) => (b.onclick = () => { const u = b.dataset.ourl.replace(/^\*\./, ''); api.OpenURL(u.includes('://') ? u : 'https://' + u); }));
  }

  function toggleReveal() {
    S.reveal = !S.reveal;
    clearTimeout(S.revealTimer);
    if (S.reveal) S.revealTimer = setTimeout(() => { S.reveal = false; renderDetail(); }, ((S.settings && S.settings.revealSec) || 30) * 1000);
    renderDetail();
  }

  async function copyField(id, field, btn) {
    try {
      const r = await call('copy', { id, field });
      if (btn) { btn.classList.add('done'); setTimeout(() => btn.classList.remove('done'), 1200); }
      const what = { username: 'Usuario copiado', password: 'Contraseña copiada', totp: 'Código 2FA copiado', notes: 'Notas copiadas' }[field] || 'Copiado';
      if (r && r.clearIn) toast(`${what} · se borra del portapapeles en ${r.clearIn} s`, 'secret', { bar: r.clearIn });
      else toast(what);
    } catch (e) { fail(e); }
  }

  async function toggleFav(it) {
    try { await call('fav', { id: it.id, on: !it.fav }); await loadItems(); if (S.detail && S.detail.item.id === it.id) S.detail.item.fav = !it.fav; renderDetail(); } catch (e) { fail(e); }
  }
  async function del(id) {
    try {
      await call('delete', { id });
      if (S.sel === id) { S.sel = null; S.detail = null; }
      await loadItems(); renderDetail();
      toast('Movida a la papelera', 'ok', { action: 'Deshacer', onAction: () => untrash(id) });
    } catch (e) { fail(e); }
  }
  async function untrash(id) {
    try { await call('untrash', { id }); await loadItems(); if (S.sel === id) { S.detail = null; select(id); } toast('Restaurada'); } catch (e) { fail(e); }
  }
  async function purge(id) {
    if (!(await confirmBox('Borrar para siempre', 'Esta entrada desaparecerá de la bóveda. No se puede deshacer.', 'Borrar', true))) return;
    try { await call('purge', { id }); S.sel = null; S.detail = null; await loadItems(); renderDetail(); toast('Borrada'); } catch (e) { fail(e); }
  }
  async function emptyTrash() {
    if (!(await confirmBox('Vaciar papelera', `Se borrarán ${S.trash.length} entradas para siempre.`, 'Vaciar', true))) return;
    try { await call('emptyTrash'); S.sel = null; S.detail = null; await loadItems(); renderDetail(); toast('Papelera vacía'); } catch (e) { fail(e); }
  }
  function editSelected() { if (S.detail) openEditor(S.detail); else if (S.sel) select(S.sel).then(() => S.detail && openEditor(S.detail)); }
  function duplicate() {
    if (!S.detail) return;
    const d = JSON.parse(JSON.stringify(S.detail));
    d.item.id = ''; d.item.title += ' (copia)';
    d.secret.history = [];
    openEditor(d);
  }

  // =====================================================================
  // editor
  // =====================================================================
  function openEditor(d, preset = {}) {
    const isNew = !d || !d.item.id;
    const it = d ? d.item : { type: preset.type || (S.filter === 'note' ? 'note' : 'login'), title: '', urls: [], username: '', fav: false };
    const sec = d ? d.secret : { password: preset.password || '', notes: '', totp: '', fields: [] };
    const urls = it.urls && it.urls.length ? it.urls.slice() : [''];
    const fields = (sec.fields || []).map((f) => ({ ...f }));
    const m = modal({
      title: isNew ? 'Nueva entrada' : 'Editar entrada',
      narrow: false,
      body: `<div class="seg" id="ed-type" style="margin-bottom:14px"><button data-t="login">${ic('key')} Inicio de sesión</button><button data-t="note">${ic('note')} Nota segura</button></div>
        <label class="field"><span>Nombre</span><input id="ed-title" value="${esc(it.title)}" placeholder="Ej.: Gmail, Banco, Steam…" spellcheck="false"></label>
        <div class="ed-login">
          <div class="field"><span>Webs <em>· solo la web exacta; escribe *.dominio.com para incluir sus subdominios</em></span><div class="multi" id="ed-urls"></div>
            <button class="btn ghost xs" id="ed-addurl" style="margin-top:6px">${ic('plus')}Otra web</button></div>
          <label class="field"><span>Usuario o correo</span><input id="ed-user" value="${esc(it.username || '')}" spellcheck="false" autocomplete="off"></label>
          <label class="field"><span>Contraseña</span>
            <div class="pw"><input type="password" id="ed-pw" class="mono" value="${esc(sec.password || '')}" spellcheck="false" autocomplete="off">
              <button type="button" class="ibtn in-btn" id="ed-eye" title="Ver">${ic('eye')}</button>
              <button type="button" class="ibtn side-btn" id="ed-gen" title="Generar una contraseña segura">${ic('dice')}</button></div>
            <div class="strength"><i id="ed-bar"></i></div>
            <div class="sinfo"><b id="ed-label">&nbsp;</b><span id="ed-crack"></span></div>
            <div class="hint" id="ed-warn"></div></label>
          <label class="field"><span>Clave 2FA <em>· opcional: el texto o el enlace otpauth:// que da la web al activar la verificación en dos pasos</em></span>
            <input id="ed-totp" class="mono" value="${esc(sec.totp || '')}" spellcheck="false" autocomplete="off" placeholder="JBSW Y3DP EHPK 3PXP"></label>
        </div>
        <label class="field"><span>Notas</span><textarea id="ed-notes" spellcheck="false">${esc(sec.notes || '')}</textarea></label>
        <div class="field"><span>Campos extra <em>· PIN, preguntas de seguridad…</em></span><div class="multi" id="ed-fields"></div>
          <button class="btn ghost xs" id="ed-addfield" style="margin-top:6px">${ic('plus')}Añadir campo</button></div>
        <label class="chk"><input type="checkbox" id="ed-fav" ${it.fav ? 'checked' : ''}><span>Favorita</span></label>
        <div class="hint bad" id="ed-err"></div>`,
      foot: `<button class="btn ghost sm" data-close>Cancelar</button><button class="btn primary sm" id="ed-save">${ic('ok')}Guardar</button>`,
    });
    const el = m.el;
    let type = it.type || 'login';
    const setType = (t) => {
      type = t;
      $$('#ed-type button', el).forEach((b) => b.classList.toggle('on', b.dataset.t === t));
      $('.ed-login', el).hidden = t === 'note';
    };
    $$('#ed-type button', el).forEach((b) => (b.onclick = () => setType(b.dataset.t)));
    setType(type);

    const drawUrls = () => {
      $('#ed-urls', el).innerHTML = urls.map((u, i) => `<div class="r"><input class="inp mono" data-u="${i}" value="${esc(u)}" placeholder="https://ejemplo.com" spellcheck="false">
        ${urls.length > 1 ? `<button class="ibtn danger" data-ux="${i}" title="Quitar">${ic('x')}</button>` : ''}</div>`).join('');
      $$('[data-u]', el).forEach((inp) => inp.addEventListener('input', () => { urls[+inp.dataset.u] = inp.value; strength(); }));
      $$('[data-ux]', el).forEach((b) => (b.onclick = () => { urls.splice(+b.dataset.ux, 1); drawUrls(); }));
    };
    drawUrls();
    $('#ed-addurl', el).onclick = () => { urls.push(''); drawUrls(); $$('[data-u]', el).pop().focus(); };

    const drawFields = () => {
      $('#ed-fields', el).innerHTML = fields.map((f, i) => `<div class="cf"><input class="inp" data-fn="${i}" value="${esc(f.name)}" placeholder="Nombre">
        <input class="inp ${f.hidden ? 'mono' : ''}" data-fv="${i}" type="${f.hidden ? 'password' : 'text'}" value="${esc(f.value)}" placeholder="Valor" autocomplete="off">
        <button class="ibtn" data-fh="${i}" title="${f.hidden ? 'Es secreto' : 'Es visible'}">${ic(f.hidden ? 'eyeOff' : 'eye')}</button>
        <button class="ibtn danger" data-fx="${i}" title="Quitar">${ic('x')}</button></div>`).join('');
      $$('[data-fn]', el).forEach((inp) => inp.addEventListener('input', () => (fields[+inp.dataset.fn].name = inp.value)));
      $$('[data-fv]', el).forEach((inp) => inp.addEventListener('input', () => (fields[+inp.dataset.fv].value = inp.value)));
      $$('[data-fh]', el).forEach((b) => (b.onclick = () => { const f = fields[+b.dataset.fh]; f.hidden = !f.hidden; drawFields(); }));
      $$('[data-fx]', el).forEach((b) => (b.onclick = () => { fields.splice(+b.dataset.fx, 1); drawFields(); }));
    };
    drawFields();
    $('#ed-addfield', el).onclick = () => { fields.push({ name: '', value: '', hidden: true }); drawFields(); };

    const pw = $('#ed-pw', el);
    const strength = debounce(async () => {
      if (!pw.value) { meter('#ed', null); return; }
      try {
        const inputs = [$('#ed-user', el).value, ...urls.map(hostOf)];
        meter('#ed', await call('strength', { password: pw.value, inputs }));
      } catch { /* nada */ }
    }, 120);
    pw.addEventListener('input', strength);
    $('#ed-user', el).addEventListener('input', strength);
    strength();
    $('#ed-eye', el).onclick = () => { pw.type = pw.type === 'password' ? 'text' : 'password'; };
    $('#ed-gen', el).onclick = async () => {
      try { const g = await call('generate'); pw.value = g.password; pw.type = 'text'; strength(); } catch (e) { fail(e); }
    };
    if (preset.password) { pw.type = 'text'; }
    if (isNew && !it.title) setTimeout(() => $('#ed-title', el).focus(), 40);

    const save = async () => {
      const item = { ...it, type, title: $('#ed-title', el).value.trim(), fav: $('#ed-fav', el).checked };
      item.urls = type === 'note' ? [] : urls.map((u) => u.trim()).filter(Boolean);
      item.username = type === 'note' ? '' : $('#ed-user', el).value.trim();
      const secret = {
        password: type === 'note' ? '' : pw.value,
        totp: type === 'note' ? '' : $('#ed-totp', el).value.trim(),
        notes: $('#ed-notes', el).value,
        fields: fields.filter((f) => f.name.trim() || f.value),
      };
      if (!item.title && !item.urls.length) { $('#ed-err', el).textContent = 'Ponle un nombre.'; m.shake(); return; }
      const b = $('#ed-save', el);
      b.classList.add('busy');
      try {
        const saved = await call('save', { item, secret });
        m.close();
        toast(isNew ? 'Guardada' : 'Cambios guardados');
        if (S.view !== 'items') go('items', 'all');
        await loadItems();
        S.sel = null;
        select(saved.id);
        S.health = null;
      } catch (e) {
        $('#ed-err', el).textContent = e.message;
        m.shake();
      } finally { b.classList.remove('busy'); }
    };
    $('#ed-save', el).onclick = save;
    el.addEventListener('keydown', (e) => { if (e.key === 'Enter' && e.ctrlKey) save(); });
  }

  // =====================================================================
  // generador
  // =====================================================================
  async function renderGen() {
    const el = $('#gen');
    if (!S.settings) { try { S.settings = await call('settings'); } catch (e) { fail(e); return; } }
    const o = S.genOpts || { ...S.settings.gen };
    S.genOpts = o;
    el.innerHTML = `<h1>Generador</h1><p class="lead">Contraseñas aleatorias de verdad (el generador criptográfico de Windows). Una contraseña larga y al azar no hace falta recordarla: la guarda TurboVAULT.</p>
      <div class="gen-out"><div class="pwtext" id="g-out"></div>
        <button class="ibtn" id="g-new" title="Otra (Espacio)">${ic('refresh')}</button>
        <button class="ibtn" id="g-copy" title="Copiar">${ic('copy')}</button></div>
      <div class="strength"><i id="g-bar"></i></div>
      <div class="sinfo"><b id="g-label">&nbsp;</b><span id="g-crack"></span></div>
      <div class="card" style="margin-top:18px">
        <div class="seg" id="g-mode"><button data-m="random">Aleatoria</button><button data-m="words">Frase de palabras</button><button data-m="pin">PIN</button></div>
        <div class="gen-grid" id="g-opts"></div>
      </div>
      <div class="row-btns" style="justify-content:flex-start"><button class="btn primary sm" id="g-save">${ic('plus')}Guardar en una entrada nueva</button></div>`;
    const drawOpts = () => {
      $$('#g-mode button').forEach((b) => b.classList.toggle('on', b.dataset.m === o.mode));
      let h = '';
      if (o.mode === 'random') {
        h += `<div class="full"><div class="opt"><b>Longitud</b></div><div class="slider-row"><input type="range" id="g-len" min="8" max="64" value="${o.length}"><output id="g-lenv">${o.length}</output></div></div>`;
        for (const [k, l] of [['lower', 'Minúsculas (a-z)'], ['upper', 'Mayúsculas (A-Z)'], ['digits', 'Números (0-9)'], ['symbols', 'Símbolos (!#$%…)'], ['noAmbiguous', 'Evitar parecidos (l, 1, O, 0)']]) {
          h += `<label class="opt"><b>${l}</b><span class="toggle"><input type="checkbox" data-o="${k}" ${o[k] ? 'checked' : ''}><i></i></span></label>`;
        }
      } else if (o.mode === 'words') {
        h += `<div class="full"><div class="opt"><b>Palabras</b></div><div class="slider-row"><input type="range" id="g-words" min="3" max="10" value="${o.words}"><output id="g-wordsv">${o.words}</output></div></div>`;
        h += `<label class="opt"><b>Mayúscula inicial</b><span class="toggle"><input type="checkbox" data-o="capitalize" ${o.capitalize ? 'checked' : ''}><i></i></span></label>`;
        h += `<label class="opt"><b>Añadir un número</b><span class="toggle"><input type="checkbox" data-o="number" ${o.number ? 'checked' : ''}><i></i></span></label>`;
        h += `<div class="opt"><b>Separador</b><div class="seg" id="g-sep">${['-', '.', '_', ' ', ''].map((s) => `<button data-s="${esc(s)}" class="${o.sep === s ? 'on' : ''}">${s === ' ' ? 'espacio' : s === '' ? 'nada' : esc(s)}</button>`).join('')}</div></div>`;
      } else {
        h += `<div class="full"><div class="opt"><b>Cifras</b></div><div class="slider-row"><input type="range" id="g-len" min="4" max="12" value="${Math.min(12, Math.max(4, o.length))}"><output id="g-lenv">${Math.min(12, Math.max(4, o.length))}</output></div></div>`;
      }
      $('#g-opts').innerHTML = h;
      const len = $('#g-len');
      if (len) len.oninput = () => { o.length = +len.value; $('#g-lenv').textContent = len.value; regen(); };
      const w = $('#g-words');
      if (w) w.oninput = () => { o.words = +w.value; $('#g-wordsv').textContent = w.value; regen(); };
      $$('[data-o]').forEach((c) => (c.onchange = () => { o[c.dataset.o] = c.checked; regen(); }));
      $$('#g-sep button').forEach((b) => (b.onclick = () => { o.sep = b.dataset.s; drawOpts(); regen(); }));
    };
    $$('#g-mode button').forEach((b) => (b.onclick = () => {
      o.mode = b.dataset.m;
      if (o.mode === 'pin') o.length = 6; else if (o.mode === 'random' && o.length < 8) o.length = 20;
      drawOpts(); regen();
    }));
    let cur = '';
    const regen = debounce(async () => {
      try {
        const g = await call('generate', o);
        cur = g.password;
        $('#g-out').innerHTML = colorPw(cur);
        meter('#g', g.strength);
      } catch (e) { $('#g-out').textContent = e.message; }
    }, 60);
    $('#g-new').onclick = regen;
    $('#g-copy').onclick = async () => {
      try { const r = await call('copyText', { text: cur, secret: true }); toast(`Copiada · se borra del portapapeles en ${r.clearIn} s`, 'secret', { bar: r.clearIn }); } catch (e) { fail(e); }
    };
    $('#g-save').onclick = () => openEditor(null, { password: cur, type: 'login' });
    drawOpts();
    regen();
  }

  // =====================================================================
  // salud
  // =====================================================================
  async function renderHealth(force) {
    const el = $('#health');
    if (!S.health || force) {
      el.innerHTML = '<div class="spinner"></div>';
      try { S.health = await call('health'); } catch (e) { fail(e); return; }
      renderSide();
    }
    const H = S.health;
    const reusedN = H.reused.reduce((n, g) => n + g.length, 0);
    const sc = H.score;
    const color = sc >= 80 ? '#34d399' : sc >= 50 ? '#fbbf24' : '#f87171';
    const C = 2 * Math.PI * 70;
    const tabs = [
      ['weak', 'Débiles', H.weak.length], ['reused', 'Repetidas', reusedN], ['breached', 'En filtraciones', H.breached.length],
      ['old', 'Más de un año', H.old.length], ['insecure', 'Webs sin https', H.insecure.length],
    ];
    el.innerHTML = `<h1>Salud de tus contraseñas</h1><p class="lead">Lo que conviene cambiar primero. Todo se calcula en tu PC.</p>
      <div class="health-top">
        <div class="gauge"><svg viewBox="0 0 170 170"><circle class="bgc" cx="85" cy="85" r="70"/><circle class="fg" cx="85" cy="85" r="70" stroke="${color}" stroke-dasharray="${C}" stroke-dashoffset="${C * (1 - sc / 100)}" stroke-linecap="round"/></svg>
          <div class="num"><div><b style="color:${color}">${sc}</b><span>de 100</span></div></div></div>
        <div><div class="hstats">${tabs.map(([k, l, n]) => `<button class="hstat ${n ? 'bad' : 'good'} ${S.healthTab === k ? 'on' : ''}" data-tab="${k}"><b>${n}</b><span>${l}</span></button>`).join('')}</div>
          <p class="hint" style="margin-top:10px">${H.total} contraseñas analizadas · ${H.noTotp} sin verificación en dos pasos guardada.</p></div>
      </div>
      <div class="card" id="breach-card"><h3>${ic('shield')}Filtraciones conocidas</h3>
        <p>Comprueba si alguna de tus contraseñas aparece en filtraciones públicas (servicio <b>Have I Been Pwned</b>). De cada contraseña solo se envían los 5 primeros caracteres de su huella SHA-1 (de 40): ni la contraseña ni su huella completa salen de tu PC.</p>
        <div style="display:flex;align-items:center;gap:12px"><button class="btn ghost sm" id="h-breach">${ic('refresh')}Comprobar ahora</button>
        <span class="hint">${H.breachAt ? 'Última comprobación: ' + esc(ago(H.breachAt)) : 'No se ha comprobado en esta sesión.'}</span></div></div>
      <div id="h-list"></div>`;
    $$('[data-tab]').forEach((b) => (b.onclick = () => { S.healthTab = b.dataset.tab; renderHealth(); }));
    $('#h-breach').onclick = async (e) => {
      const b = e.currentTarget;
      b.classList.add('busy');
      try { S.health = await call('breach'); S.healthTab = 'breached'; renderSide(); renderHealth(); toast(S.health.breached.length ? 'Hay contraseñas filtradas: cámbialas cuanto antes' : 'Ninguna contraseña aparece en filtraciones', S.health.breached.length ? 'warn' : 'ok'); }
      catch (err) { fail(err); } finally { b.classList.remove('busy'); }
    };
    const row = (x) => `<div class="row-i" data-hid="${esc(x.id)}">${avatar({ title: x.title, urls: x.host ? [x.host] : [] })}
      <div class="t"><b><span>${esc(x.title)}</span></b><small>${esc(x.note || x.username || x.host || '')}</small></div>
      <div class="meta"><span class="chip s${x.score}">${esc(SCORE_LABEL[x.score] || '')}</span><button class="btn xs ghost" data-fix="${esc(x.id)}">Cambiar</button></div></div>`;
    let lh = '';
    const tab = S.healthTab;
    const desc = {
      weak: 'Se pueden adivinar con facilidad. Cámbialas por una generada.',
      reused: 'La misma contraseña en varias webs: si una se filtra, caen todas.',
      breached: 'Han aparecido en filtraciones: los atacantes las prueban las primeras.',
      old: 'No se cambian desde hace más de un año.',
      insecure: 'Webs guardadas con http://: la contraseña viaja sin cifrar.',
    }[tab];
    lh += `<p class="hint" style="margin:6px 0 10px">${esc(desc)}</p>`;
    if (tab === 'reused') {
      lh += H.reused.length ? H.reused.map((g) => `<div class="hgroup"><div class="group-h">${g.length} webs con la misma contraseña</div>${g.map(row).join('')}</div>`).join('')
        : `<div class="empty">${ic('shieldOk')}<b>Ninguna repetida</b></div>`;
    } else {
      const list = H[tab] || [];
      lh += list.length ? `<div class="hlist">${list.map(row).join('')}</div>` : `<div class="empty">${ic('shieldOk')}<b>Todo bien por aquí</b></div>`;
    }
    $('#h-list').innerHTML = lh;
    $$('[data-fix]').forEach((b) => (b.onclick = async (e) => {
      e.stopPropagation();
      go('items', 'all');
      await select(b.dataset.fix);
      editSelected();
    }));
    $$('[data-hid]').forEach((r) => (r.onclick = () => { go('items', 'all'); select(r.dataset.hid); }));
  }

  // =====================================================================
  // actividad
  // =====================================================================
  const EV = {
    unlock: ['unlock', 'unlock', 'Bóveda abierta'],
    lock: ['lock', 'lock', 'Bóveda cerrada'],
    fails: ['warn', 'bad', 'Intentos fallidos'],
    fill: ['browser', 'fill', 'Rellenada en el navegador'],
    totp: ['otp', 'fill', 'Código 2FA usado'],
    copy: ['copy', '', 'Copiada'],
    add: ['plus', '', 'Añadida'],
    edit: ['edit', '', 'Modificada'],
    create: ['key', 'unlock', 'Bóveda creada'],
    import: ['download', '', 'Importadas'],
    export: ['upload', 'bad', 'Exportada sin cifrar (CSV)'],
    master: ['key', 'lock', 'Contraseña maestra cambiada'],
    pair: ['browser', 'unlock', 'Navegador vinculado'],
    unpair: ['browser', 'lock', 'Navegador desvinculado'],
    backup: ['usb', '', 'Copia de seguridad'],
    restore: ['upload', 'lock', 'Copia restaurada'],
    abuse: ['warn', 'bad', 'Bloqueo por seguridad'],
    breach: ['shield', '', 'Comprobación de filtraciones'],
    rollback: ['warn', 'bad', 'La bóveda es más antigua de lo esperado'],
    tampered: ['warn', 'bad', 'Extensión modificada'],
  };
  async function renderActivity() {
    const el = $('#activity');
    let evs;
    try { evs = await call('events'); } catch (e) { fail(e); return; }
    let h = `<div style="display:flex;align-items:flex-start;gap:12px"><div class="grow"><h1>Actividad</h1>
      <p class="lead">Qué se ha hecho con tu bóveda y desde dónde. Si ves algo que no has hecho tú, cambia la contraseña maestra.</p></div>
      ${evs.length ? `<button class="btn ghost sm" id="ev-clear">${ic('trash')}Borrar registro</button>` : ''}</div>`;
    if (!evs.length) h += `<div class="empty">${ic('act')}<b>Sin actividad todavía</b></div>`;
    let day = '';
    for (const e of evs) {
      const d = new Date(e.t * 1000);
      const dk = d.toLocaleDateString('es-ES', { weekday: 'long', day: 'numeric', month: 'long' });
      if (dk !== day) { day = dk; h += `<div class="day-h">${esc(dk)}</div>`; }
      const [icon, cls, label] = EV[e.kind] || ['act', '', e.kind];
      let title = label, sub = '';
      if (['fill', 'totp', 'copy', 'add', 'edit'].includes(e.kind)) { title = `${label}: ${e.title || ''}`; }
      if (['lock', 'fails', 'import', 'abuse', 'pair', 'breach', 'backup', 'restore', 'export', 'rollback', 'tampered'].includes(e.kind) && e.title) sub = e.title;
      const from = e.src === 'app' ? 'Esta ventana' : e.src === 'agente' ? 'TurboVAULT' : e.src || '';
      if (e.origin) sub = [hostOf(e.origin), from].filter(Boolean).join(' · ');
      else if (!sub) sub = from;
      h += `<div class="ev"><div class="ic ${cls}">${ic(icon)}</div><div class="t"><b>${esc(title)}</b><span>${esc(sub)}</span></div>
        <time>${esc(d.toLocaleTimeString('es-ES', { hour: '2-digit', minute: '2-digit' }))}</time></div>`;
    }
    el.innerHTML = h;
    const c = $('#ev-clear');
    if (c) c.onclick = async () => {
      if (!(await confirmBox('Borrar registro', 'Se borrará toda la actividad guardada.', 'Borrar', true))) return;
      try { await call('clearEvents'); renderActivity(); } catch (e) { fail(e); }
    };
  }

  // =====================================================================
  // copias, importar y exportar
  // =====================================================================
  const KIND = { usb: ['usb', 'USB'], disco: ['drive', 'Otro disco'], sistema: ['cpu', 'Disco del sistema'], red: ['globe', 'Red'], otro: ['folder', 'Carpeta'] };
  async function renderBackup() {
    const el = $('#backup');
    let targets = [];
    try { targets = await call('backupTargets'); S.settings = await call('settings'); } catch (e) { fail(e); return; }
    let tl = targets.map((t) => {
      const [kic, kname] = KIND[t.kind] || KIND.otro;
      const status = !t.connected ? '<span class="dot"></span> No conectado'
        : t.lastError ? `<span class="dot bad"></span> ${esc(t.lastError)}`
        : t.lastCopy ? `<span class="dot ok"></span> Última copia ${esc(ago(t.lastCopy))} · ${t.copies} guardadas` : '<span class="dot warn"></span> Pendiente';
      return `<div class="titem"><div class="tile" style="--tile:${t.kind === 'usb' ? 'linear-gradient(135deg,#10b981,#06b6d4)' : t.kind === 'sistema' ? 'linear-gradient(135deg,#f59e0b,#ef4444)' : 'var(--grad)'}">${ic(kic)}</div>
        <div class="txt"><b>${esc(t.label ? `${t.label} · ` : '')}${esc(t.current || t.path)}</b><span>${kname} · guarda las ${t.keep} últimas · ${status}</span></div>
        ${t.connected ? `<button class="ibtn" data-open="${esc(t.current)}" title="Abrir carpeta">${ic('folder')}</button>` : ''}
        <button class="ibtn danger" data-rm="${esc(t.id)}" title="Quitar">${ic('x')}</button></div>`;
    }).join('');
    if (!targets.length) tl = `<div class="warn-box">Todavía no hay ninguna. Elige una carpeta en un <b>disco externo o USB</b>: si pierdes o se rompe el PC, ahí tendrás tus contraseñas.</div>`;
    el.innerHTML = `<h1>Copias e importar</h1><p class="lead">El archivo de la bóveda va cifrado: puedes copiarlo donde quieras. Sin tu contraseña maestra no sirve de nada.</p>
      <div class="card"><h3>${ic('usb')}Copias automáticas</h3>
        <p>Cada vez que cambias algo se guarda una copia nueva en estas carpetas (y en cuanto conectes el disco si no estaba). Se reconocen aunque el USB cambie de letra.</p>
        <div class="tlist">${tl}</div>
        <div class="row-btns" style="justify-content:flex-start"><button class="btn primary sm" id="bk-add">${ic('plus')}Añadir carpeta</button>
        ${targets.length ? `<button class="btn ghost sm" id="bk-now">${ic('refresh')}Copiar ahora</button>` : ''}</div></div>
      <div class="card"><h3>${ic('download')}Copia puntual</h3><p>Guarda una copia cifrada ahora en la carpeta que elijas (por ejemplo, para llevarla a otro PC).</p>
        <button class="btn ghost sm" id="bk-once">${ic('folder')}Guardar una copia en…</button></div>
      <div class="card"><h3>${ic('upload')}Restaurar una copia</h3><p>Abre un archivo <b>.tvault</b> con su contraseña maestra. La bóveda actual no se borra: se guarda al lado.</p>
        <button class="btn ghost sm" id="bk-restore">${ic('upload')}Restaurar…</button></div>
      <div class="card"><h3>${ic('key')}Importar contraseñas</h3>
        <p>Desde Chrome: abre <span class="mono">chrome://password-manager/settings</span>, pulsa <b>Exportar contraseñas</b> y elige aquí el archivo CSV. También vale el CSV de Edge, Firefox, Bitwarden, KeePass, 1Password, LastPass o Proton Pass.</p>
        <button class="btn primary sm" id="bk-import">${ic('download')}Importar CSV…</button></div>
      <div class="card"><h3>${ic('warn')}Exportar sin cifrar</h3><p>Crea un CSV con todas tus contraseñas <b>en texto normal</b>. Solo para pasarte a otro gestor; bórralo justo después.</p>
        <button class="btn ghost sm danger-txt" id="bk-export">${ic('upload')}Exportar CSV…</button></div>`;
    $('#bk-add').onclick = addTarget;
    const now = $('#bk-now');
    if (now) now.onclick = async () => { now.classList.add('busy'); try { await call('backupNow', {}); toast('Copia hecha'); renderBackup(); } catch (e) { fail(e); } finally { now.classList.remove('busy'); } };
    $('#bk-once').onclick = async () => {
      const dir = await api.PickFolder('Carpeta para la copia');
      if (!dir) return;
      try { const f = await call('backupNow', { path: dir }); toast('Copia guardada', 'ok', { action: 'Ver', onAction: () => api.ShowInFolder(f) }); } catch (e) { fail(e); }
    };
    $('#bk-restore').onclick = restoreFlow;
    $('#bk-import').onclick = importFlow;
    $('#bk-export').onclick = exportFlow;
    $$('[data-rm]').forEach((b) => (b.onclick = async () => {
      if (!(await confirmBox('Quitar destino', 'Dejarán de hacerse copias ahí. Las que ya hay no se borran.', 'Quitar'))) return;
      try { await call('removeBackupTarget', { id: b.dataset.rm }); renderBackup(); } catch (e) { fail(e); }
    }));
    $$('[data-open]').forEach((b) => (b.onclick = () => api.ShowInFolder(b.dataset.open)));
  }

  async function addTarget() {
    const dir = await api.PickFolder('Carpeta para las copias automáticas (mejor en un disco externo o USB)');
    if (!dir) return;
    if (/^c:/i.test(dir) && !(await confirmBox('¿En el mismo disco?', 'Esa carpeta está en el disco del sistema: si se rompe o pierdes el PC, perderías también la copia. Mejor un disco externo o USB.', 'Usarla igualmente'))) return;
    try { await call('addBackupTarget', { path: dir, keep: 10 }); toast('Copia automática configurada'); renderBackup(); } catch (e) { fail(e); }
  }

  async function restoreFlow() {
    const file = await api.PickFile('Elige una copia de TurboVAULT', 'Copias de TurboVAULT', '*.tvault');
    if (!file) return;
    try { await call('checkBackup', { path: file }); } catch (e) { fail(e); return; }
    let info = null;
    const master = await askMaster('Restaurar copia', `Escribe la contraseña maestra <b>de esa copia</b> (la que tenías cuando se hizo).`, 'Comprobar', async (pw) => {
      info = await call('checkBackup', { path: file, master: pw });
      return pw;
    });
    if (!master) return;
    const exists = S.state && S.state.exists;
    const ok = await confirmBox('Restaurar copia', `La copia tiene <b>${info.items}</b> entradas (creada el ${esc(fmtDate(info.created))}).${exists ? ' La bóveda actual se guardará al lado por si acaso.' : ''} Desde ahora, tu contraseña maestra será la de esa copia.`, 'Restaurar');
    if (!ok) return;
    try {
      const r = await call('restore', { path: file, master });
      toast('Copia restaurada');
      if (r && r.saved) toast('La bóveda anterior está guardada en la carpeta de datos', 'ok', { ttl: 6000 });
      S.lockStep = 'welcome';
    } catch (e) { fail(e); }
  }

  async function importFlow() {
    const file = await api.PickFile('Elige el CSV exportado', 'CSV', '*.csv');
    if (!file) return;
    const m = modal({
      title: 'Importar contraseñas',
      body: `<p class="mtext">Se añadirán a tu bóveda las entradas de <b>${esc(file.split(/[\\/]/).pop())}</b>. Las que ya tengas iguales se saltan.</p>
        <label class="chk"><input type="checkbox" id="im-wipe" checked><span><b>Borrar el CSV al terminar</b> (recomendado: lleva tus contraseñas sin cifrar).</span></label>`,
      foot: `<button class="btn ghost sm" data-close>Cancelar</button><button class="btn primary sm" id="im-go">${ic('download')}Importar</button>`,
    });
    $('#im-go', m.el).onclick = async (e) => {
      const b = e.currentTarget;
      b.classList.add('busy');
      try {
        const r = await call('import', { path: file, wipe: $('#im-wipe', m.el).checked });
        m.close();
        await loadItems();
        S.health = null;
        const done = modal({
          title: 'Importación hecha',
          body: `<div style="text-align:center"><div class="ok-big" style="margin:0 auto 12px">${ic('ok')}</div>
            <p class="mtext"><b>${r.added}</b> añadidas desde ${esc(r.source)}${r.dupes ? ` · ${r.dupes} ya estaban` : ''}${r.skipped ? ` · ${r.skipped} vacías` : ''}.${r.wiped ? ' El CSV se ha borrado.' : ''}</p></div>
            <div class="info-box"><b>Ahora quítalas de Chrome</b>, para que un virus no pueda robarlas de ahí:<br>
            1. Abre <span class="mono">chrome://password-manager/settings</span> y desactiva «Ofrecer guardar contraseñas».<br>
            2. En <span class="mono">chrome://settings/clearBrowserData</span> → Avanzado, marca solo «Contraseñas y otros datos de inicio de sesión» → «Desde siempre» → Borrar.</div>`,
          foot: '<button class="btn primary sm" data-close>Hecho</button>',
        });
        void done;
        if (S.view === 'items') renderList();
      } catch (err) { fail(err); } finally { b.classList.remove('busy'); }
    };
  }

  async function exportFlow() {
    const master = await askMaster('Exportar sin cifrar', 'El archivo llevará <b>todas tus contraseñas en texto normal</b>. Escribe tu contraseña maestra para confirmar.', 'Continuar', async (pw) => { await call('verifyMaster', { master: pw }); return pw; });
    if (!master) return;
    const path = await api.PickSaveFile('Guardar exportación', 'turbovault-exportacion.csv', 'CSV', '*.csv');
    if (!path) return;
    try {
      const n = await call('export', { path, master });
      toast(`${n} entradas exportadas. Borra el archivo cuando acabes.`, 'warn', { ttl: 7000, action: 'Ver', onAction: () => api.ShowInFolder(path) });
    } catch (e) { fail(e); }
  }

  // =====================================================================
  // ajustes
  // =====================================================================
  async function renderSettings() {
    const el = $('#settings');
    let st, pairs, never, ext;
    try {
      [st, pairs, never] = await Promise.all([call('settings'), call('pairs').catch(() => []), call('never').catch(() => [])]);
      ext = await api.ExtensionInfo().catch(() => ({}));
    } catch (e) { fail(e); el.innerHTML = `<h1>Ajustes</h1><p class="lead">No se han podido leer: ${esc(e.message)}</p>`; return; }
    pairs = pairs || []; never = never || []; ext = ext || {};
    S.settings = st;
    const seg = (id, vals, cur, fmt) => `<div class="seg" id="${id}">${vals.map((v) => `<button data-v="${v}" class="${v === cur ? 'on' : ''}">${fmt(v)}</button>`).join('')}</div>`;
    const tog = (id, on) => `<label class="toggle"><input type="checkbox" id="${id}" ${on ? 'checked' : ''}><i></i></label>`;
    const extLoaded = ext.loaded && ext.loaded.length > 0;
    el.innerHTML = `<h1>Ajustes</h1><p class="lead">Versión ${esc(await api.Version())}</p>
      <div class="card"><h3>${ic('clock')}Sesión</h3>
        <div class="srow" id="st-session-row"><div><b>Tiempo que se queda abierta</b><span>Cuenta desde que la abres y no se alarga al usarla. Al cambiarlo, la sesión de ahora vuelve a contar desde cero con el tiempo nuevo. Se cierra antes si bloqueas Windows, suspendes el PC o cierras sesión.</span></div>
          <select class="fsel" id="st-session">${[1, 2, 5, 10, 15, 30, 60, 120, 240, 480].map((v) => `<option value="${v}" ${v === st.sessionMin ? 'selected' : ''}>${fmtMin(v)}</option>`).join('')}</select></div>
        <div class="srow"><div><b>Borrar el portapapeles a los</b><span>Las contraseñas copiadas no van al historial de Windows (Win+V) ni a la nube.</span></div>
          ${seg('st-clip', [10, 20, 30, 60], st.clipSeconds, (v) => v + ' s')}</div>
        <div class="srow"><div><b>Volver a ocultar lo que muestras a los</b><span>Contraseñas, campos secretos y notas que destapas con el ojo.</span></div>
          ${seg('st-reveal', [10, 30, 60], st.revealSec || 30, (v) => v + ' s')}</div>
        <div class="srow"><div><b>Cerrar la bóveda al cerrar la ventana</b><span>Si está apagado, sigue abierta en la bandeja hasta que acabe la sesión (para que la extensión pueda rellenar).</span></div>${tog('st-lockclose', st.lockOnClose)}</div>
        <div class="srow"><div><b>Cerrar la bóveda al minimizar</b><span>Para que nadie la vea si dejas el PC un momento.</span></div>${tog('st-lockmin', st.lockOnMin)}</div></div>
      <div class="card"><h3>${ic('shield')}Privacidad</h3>
        <div class="srow"><div><b>Ocultar la ventana en capturas</b><span>Sale en negro en capturas de pantalla, grabaciones y al compartir pantalla.</span></div>${tog('st-capture', st.hideCapture)}</div>
        <div class="srow"><div><b>Avisar al rellenar</b><span>Un aviso junto al reloj cada vez que el navegador usa una contraseña.</span></div>${tog('st-notify', st.notifyFill)}</div>
        <div class="srow"><div><b>Buscar filtraciones al abrir la bóveda</b><span>Comprueba en segundo plano si alguna contraseña sale en filtraciones conocidas (Have I Been Pwned). Solo se mandan los 5 primeros caracteres de su huella, nunca la contraseña.</span></div>${tog('st-breach', st.autoBreach)}</div>
        <div class="srow"><div><b>Arrancar con Windows</b><span>Deja TurboVAULT en la bandeja (junto a la flechita) para que la extensión funcione siempre.</span></div>${tog('st-auto', st.autostart)}</div></div>
      <div class="card"><h3>${ic('key')}Contraseña maestra</h3>
        <div class="srow"><div><b>Cambiar la contraseña maestra</b><span>También cambia la clave interna de cifrado: las copias viejas siguen abriéndose solo con la contraseña de entonces.</span></div>
          <button class="btn ghost sm" id="st-master">Cambiar…</button></div></div>
      <div class="card"><h3>${ic('browser')}Navegador</h3>
        <div class="srow"><div><b>Extensión</b><span>${extLoaded ? 'Cargada en el navegador.' : 'Aún no está cargada en el navegador.'} ${ext.host ? '' : 'El puente con el navegador no está registrado.'}</span>
          <span class="mono">${esc(ext.dir || '')}</span></div>
          <button class="btn ${extLoaded ? 'ghost' : 'primary'} sm" id="st-ext">${extLoaded ? 'Reinstalar' : 'Preparar'}</button></div>
        ${pairs.length ? pairs.map((p) => `<div class="srow"><div><b>${esc(p.name || 'Navegador')}</b><span>Vinculado ${esc(fmtDate(p.created))}</span></div><button class="btn ghost xs danger-txt" data-unpair="${esc(p.hash)}">Desvincular</button></div>`).join('')
          : '<div class="srow"><div><b>Ningún navegador vinculado</b><span>Al usar la extensión por primera vez te pedirá vincularla con un código.</span></div></div>'}</div>
      <div class="card"><h3>${ic('globe')}Webs donde no ofrecer guardar</h3>
        ${never.length ? never.map((h) => `<div class="srow"><div><b>${esc(h)}</b></div><button class="btn ghost xs" data-never="${esc(h)}">Quitar</button></div>`).join('') : '<p style="margin:0">Ninguna.</p>'}</div>
      <div class="card"><h3>${ic('folder')}Datos</h3>
        <div class="srow"><div><b>Carpeta de la bóveda</b><span class="mono">${esc(await api.DataFolder())}</span><span>Solo tu usuario de Windows puede abrirla.</span></div>
          <button class="btn ghost sm" id="st-folder">Abrir</button></div></div>`;
    const set = async (patch) => { try { S.settings = await call('setSettings', patch); toast('Guardado'); } catch (e) { fail(e); } };
    $('#st-session').onchange = async (e) => {
      const v = +e.target.value;
      try {
        S.settings = await call('setSettings', { sessionMin: v });
        await refreshState();
        toast(`Se cerrará dentro de ${fmtMin(v)}${v >= 120 ? '. Recuerda bloquear Windows (Win+L) si te alejas del PC' : ''}`, v >= 120 ? 'warn' : 'ok', { ttl: 5000 });
      } catch (err) { fail(err); }
    };
    $$('#st-clip button').forEach((b) => (b.onclick = () => { set({ clipSeconds: +b.dataset.v }); $$('#st-clip button').forEach((x) => x.classList.toggle('on', x === b)); }));
    $('#st-capture').onchange = (e) => { set({ hideCapture: e.target.checked }); api.Protect(e.target.checked); };
    $('#st-notify').onchange = (e) => set({ notifyFill: e.target.checked });
    $('#st-auto').onchange = (e) => set({ autostart: e.target.checked });
    $$('#st-reveal button').forEach((b) => (b.onclick = () => { set({ revealSec: +b.dataset.v }); $$('#st-reveal button').forEach((x) => x.classList.toggle('on', x === b)); }));
    $('#st-lockclose').onchange = (e) => set({ lockOnClose: e.target.checked });
    $('#st-lockmin').onchange = (e) => set({ lockOnMin: e.target.checked });
    $('#st-breach').onchange = (e) => set({ autoBreach: e.target.checked });
    $('#st-master').onclick = changeMaster;
    $('#st-ext').onclick = setupExtension;
    $('#st-folder').onclick = async () => api.ShowInFolder(await api.DataFolder());
    $$('[data-unpair]').forEach((b) => (b.onclick = async () => {
      if (!(await confirmBox('Desvincular', 'Ese navegador dejará de poder pedir contraseñas hasta que lo vuelvas a vincular.', 'Desvincular', true))) return;
      try { await call('unpair', { hash: b.dataset.unpair }); renderSettings(); } catch (e) { fail(e); }
    }));
    $$('[data-never]').forEach((b) => (b.onclick = async () => { try { await call('setNever', { host: b.dataset.never, on: false }); renderSettings(); } catch (e) { fail(e); } }));
  }

  async function setupExtension() {
    try {
      const info = (await api.SetupExtension()) || {};
      const b = info.browser || { name: 'Chrome', page: 'chrome://extensions/', load: 'Cargar descomprimida', toggle: 'arriba a la derecha' };
      const where = info.opened
        ? `Se ha abierto <span class="mono">${esc(b.page)}</span> en ${esc(b.name)} y la ruta de la extensión ya está copiada.`
        : 'La ruta de la extensión ya está copiada. Abre tu navegador y escribe <span class="mono">chrome://extensions</span> (<span class="mono">edge://extensions</span> en Edge) en la barra de direcciones.';
      modal({
        title: `Cargar la extensión en ${esc(info.opened ? b.name : 'el navegador')}`,
        body: `<p class="mtext">${where}</p>
          <ol class="mtext" style="padding-left:18px;margin:0 0 10px"><li>Activa <b>Modo de desarrollador</b> (${esc(b.toggle)}).</li>
          <li>Pulsa <b>${esc(b.load)}</b>.</li><li>Pega la ruta (Ctrl+V) en la barra de la ventana y pulsa <b>Seleccionar carpeta</b>.</li></ol>
          <p class="mtext">El navegador no deja que un programa instale extensiones por su cuenta; este paso se hace una sola vez. Después, fija la extensión en la barra (icono del puzle).</p>`,
        foot: '<button class="btn primary sm" data-close>Entendido</button>',
      });
      if (S.view === 'settings') renderSettings();
    } catch (e) { fail(e); }
  }

  function changeMaster() {
    const m = modal({
      title: 'Cambiar contraseña maestra',
      body: `<label class="field"><span>Contraseña maestra actual</span><input type="password" id="cm-old" autocomplete="off"></label>
        <label class="field"><span>Nueva <em>· mínimo 12 caracteres</em></span><div class="pw"><input type="password" id="cm-new" autocomplete="off"><button type="button" class="ibtn in-btn" id="cm-sug" title="Sugerir una frase">${ic('dice')}</button></div>
          <div class="strength"><i id="cm-bar"></i></div><div class="sinfo"><b id="cm-label">&nbsp;</b><span id="cm-crack"></span></div><div class="hint" id="cm-warn"></div></label>
        <label class="field"><span>Repite la nueva</span><input type="password" id="cm-new2" autocomplete="off"></label>
        <div class="hint bad" id="cm-err"></div>`,
      foot: `<button class="btn ghost sm" data-close>Cancelar</button><button class="btn primary sm" id="cm-go" disabled>Cambiar</button>`,
    });
    const el = m.el, nw = $('#cm-new', el), nw2 = $('#cm-new2', el), go2 = $('#cm-go', el);
    let score = 0;
    const upd = () => { go2.disabled = !(score >= 3 && nw.value === nw2.value && $('#cm-old', el).value); };
    const chk = debounce(async () => {
      if (!nw.value) { meter('#cm', null); score = 0; upd(); return; }
      const s = await call('strength', { password: nw.value, master: true });
      score = nw.value.length >= 12 ? s.score : 1;
      meter('#cm', s, nw.value.length < 12 ? 'Mínimo 12 caracteres.' : '');
      upd();
    }, 120);
    nw.addEventListener('input', chk);
    nw2.addEventListener('input', upd);
    $('#cm-old', el).addEventListener('input', upd);
    $('#cm-sug', el).onclick = async () => { const g = await call('suggest'); nw.value = g.password; nw.type = 'text'; nw2.type = 'text'; chk(); nw2.focus(); };
    go2.onclick = async () => {
      go2.classList.add('busy');
      try {
        await call('changeMaster', { old: $('#cm-old', el).value, new: nw.value });
        m.close();
        toast('Contraseña maestra cambiada');
      } catch (e) {
        $('#cm-err', el).textContent = e.code === 'wrong' ? 'La contraseña actual no es correcta.' : e.message;
        m.shake();
      } finally { go2.classList.remove('busy'); }
    };
  }

  // =====================================================================
  // teclado
  // =====================================================================
  document.addEventListener('keydown', (e) => {
    if (!S.state || !S.state.unlocked || S.mode !== '' || $('.modal')) return;
    const inField = e.target.closest('input, textarea, select');
    if (e.ctrlKey && e.key.toLowerCase() === 'f') { e.preventDefault(); if (S.view !== 'items') go('items'); qIn.focus(); qIn.select(); return; }
    if (e.ctrlKey && e.key.toLowerCase() === 'n') { e.preventDefault(); openEditor(null); return; }
    if (e.ctrlKey && e.key.toLowerCase() === 'l') { e.preventDefault(); lockNow(); return; }
    if (inField) {
      if (e.key === 'Escape') { if (e.target === qIn && qIn.value) $('#q-clear').click(); else e.target.blur(); }
      return;
    }
    if (S.view !== 'items') return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const list = visibleItems();
      if (!list.length) return;
      let i = list.findIndex((x) => x.id === S.sel);
      i = e.key === 'ArrowDown' ? Math.min(list.length - 1, i + 1) : Math.max(0, i - 1);
      select(list[i].id);
      const row = $(`.row-i[data-id="${CSS.escape(list[i].id)}"]`);
      if (row) row.scrollIntoView({ block: 'nearest' });
      return;
    }
    if (S.sel && e.ctrlKey && e.shiftKey && e.key.toLowerCase() === 'c') { const it = findItem(S.sel); if (it && it.hasTotp) copyField(it.id, 'totp'); return; }
    if (S.sel && e.ctrlKey && e.key.toLowerCase() === 'c') { const it = findItem(S.sel); if (it && it.hasPassword) copyField(it.id, 'password'); return; }
    if (S.sel && e.ctrlKey && e.key.toLowerCase() === 'b') { const it = findItem(S.sel); if (it && it.username) copyField(it.id, 'username'); return; }
    if (S.sel && e.key === 'Enter') { editSelected(); return; }
    if (S.sel && e.key === 'Delete') { del(S.sel); }
  });

  // ---------- arranque ----------
  (async () => {
    try { S.mode = (await api.Mode()) || ''; } catch { S.mode = ''; }
    document.body.classList.toggle('compact', S.mode !== '');
    renderSide();
    await refreshState();
    try { S.settings = await call('settings'); } catch { /* cerrada */ }
  })();
})();
