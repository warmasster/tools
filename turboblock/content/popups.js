/* TurboBLOCK · ventanas emergentes (mundo aislado, document_start).
 * - Cuenta al service worker cada clic real: en qué enlace o botón, y si pedías
 *   abrir en otra pestaña (target, Ctrl, Mayús, botón central). Así se distingue
 *   una ventana que pediste de una "popunder".
 * - Enseña el aviso de «ventana bloqueada» con Abrir / Permitir en esta web.
 */
(() => {
  if (globalThis.__turboblockPopups) return;
  globalThis.__turboblockPopups = true;

  const CONTROL = 'button,input[type=button],input[type=submit],input[type=image],[role=button],[role=link],[role=menuitem],[role=tab],select,summary,label,[onclick]';

  // Un enlace que no lleva a ningún sitio ("#", "javascript:") es en realidad un botón.
  const navigates = (a) => {
    const raw = (a.getAttribute('href') || '').trim();
    if (!raw || raw.startsWith('#') || /^javascript:/i.test(raw)) return false;
    try {
      const u = new URL(a.href), here = new URL(location.href);
      return !(u.origin === here.origin && u.pathname === here.pathname && u.search === here.search);
    } catch { return false; }
  };

  const report = (ev) => {
    if (!ev.isTrusted) return; // los clics simulados por scripts no cuentan
    const t = ev.target instanceof Element ? ev.target : null;
    if (!t) return;
    const a = t.closest('a[href], area[href]');
    const form = t.closest('form');
    const link = a && navigates(a);
    const target = (a || form)?.getAttribute('target') || '';
    const newTab = ev.button === 1 || ev.ctrlKey || ev.metaKey || ev.shiftKey || (target !== '' && target !== '_self' && target !== '_top' && target !== '_parent');
    chrome.runtime.sendMessage({
      type: 'click',
      href: link ? a.href : (form && t.closest('button,input[type=submit]') ? form.action : ''),
      newTab,
      control: !link && (!!a || !!t.closest(CONTROL)),
    }).catch(() => {});
  };
  addEventListener('pointerdown', report, true);
  addEventListener('auxclick', report, true);
  addEventListener('keydown', (ev) => { if (ev.isTrusted && (ev.key === 'Enter' || ev.key === ' ')) report(ev); }, true);

  if (window !== top) return;

  // ---------- aviso ----------
  let box = null, count = 0, timer = 0;
  chrome.runtime.onMessage.addListener((msg) => {
    if (msg?.type === 'popupBlocked') show(msg);
  });

  const ICON = '<svg viewBox="0 0 24 24" width="18" height="18"><path d="M12 3l7 2.6V11c0 4.4-3 8.3-7 9.6C8 19.3 5 15.4 5 11V5.6z" fill="#fff"/><rect x="8.6" y="10.2" width="6.8" height="2.4" rx=".8" fill="#6366f1"/></svg>';

  function show(msg) {
    count++;
    if (!box) {
      const hostEl = document.createElement('turboblock-toast');
      const root = hostEl.attachShadow({ mode: 'closed' });
      root.innerHTML = `<style>
        :host{all:initial;position:fixed;z-index:2147483647;right:18px;bottom:18px}
        .t{font:13px/1.4 "Segoe UI Variable Text","Segoe UI",system-ui,sans-serif;color:#eceef8;width:330px;
          background:linear-gradient(#11131df2,#0d0f18f2) padding-box,linear-gradient(135deg,#8b5cf6,#6366f1 45%,#06b6d4) border-box;
          border:1px solid transparent;border-radius:14px;box-shadow:0 18px 50px rgba(0,0,0,.5),0 0 0 1px rgba(255,255,255,.03);
          padding:12px 14px 12px;display:grid;gap:9px;backdrop-filter:blur(10px);animation:in .28s cubic-bezier(.2,.9,.3,1.2)}
        @keyframes in{from{transform:translateY(14px) scale(.96);opacity:0}}
        .h{display:flex;align-items:center;gap:10px}
        .ic{width:30px;height:30px;border-radius:9px;display:grid;place-items:center;flex:none;
          background:linear-gradient(135deg,#8b5cf6,#6366f1 45%,#06b6d4);box-shadow:0 6px 18px rgba(99,102,241,.45)}
        .tx{display:grid;min-width:0;flex:1}
        .m{font-weight:650}
        .u{color:#9196ae;font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
        .n{display:none;font:700 11px/18px system-ui;min-width:18px;padding:0 5px;border-radius:9px;background:#22d3ee;color:#04121a;text-align:center}
        .n.on{display:inline-block}
        .b{display:flex;gap:6px;justify-content:flex-end}
        button{font:inherit;font-size:12px;color:#eceef8;background:rgba(255,255,255,.06);border:1px solid rgba(255,255,255,.12);
          border-radius:8px;padding:5px 11px;cursor:pointer;transition:background .12s}
        button:hover{background:rgba(255,255,255,.12)}
        button.p{background:linear-gradient(135deg,#8b5cf6,#6366f1 45%,#06b6d4);border-color:transparent;font-weight:600;color:#fff}
        button.x{background:none;border:none;color:#9196ae;padding:2px 6px;font-size:14px;align-self:flex-start}
        .bar{height:2px;border-radius:2px;background:linear-gradient(90deg,#8b5cf6,#06b6d4);transform-origin:left;animation:bar 9s linear forwards}
        @keyframes bar{to{transform:scaleX(0)}}
      </style>
      <div class="t"><div class="h"><div class="ic">${ICON}</div><div class="tx"><span class="m"></span><span class="u"></span></div>
      <span class="n"></span><button class="x" title="Cerrar">✕</button></div>
      <div class="b"><button class="allow">Permitir en esta web</button><button class="p open">Abrir</button></div><div class="bar"></div></div>`;
      box = { el: hostEl, root, m: root.querySelector('.m'), u: root.querySelector('.u'), n: root.querySelector('.n'), url: '' };
      root.querySelector('.x').onclick = hide;
      root.querySelector('.open').onclick = () => { chrome.runtime.sendMessage({ type: 'openPopup', url: box.url }); hide(); };
      root.querySelector('.allow').onclick = () => { chrome.runtime.sendMessage({ type: 'allowPopups', host: location.hostname }); hide(); };
      (document.body || document.documentElement).appendChild(hostEl);
    }
    box.url = msg.url;
    box.m.textContent = 'Ventana emergente bloqueada';
    box.u.textContent = msg.host;
    box.u.title = msg.url;
    box.n.textContent = count;
    box.n.classList.toggle('on', count > 1);
    const bar = box.root.querySelector('.bar');
    bar.style.animation = 'none'; void bar.offsetWidth; bar.style.animation = '';
    clearTimeout(timer);
    timer = setTimeout(hide, 9000);
  }
  function hide() {
    clearTimeout(timer);
    box?.el.remove();
    box = null;
    count = 0;
  }
})();
