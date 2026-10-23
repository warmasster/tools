/* TurboBLOCK · ocultar un elemento a mano. Se inyecta al pulsar «Ocultar elemento».
 * Pasa el ratón por encima, clic para elegir, ↑/↓ (o la rueda) para subir o bajar
 * de nivel y «Ocultar» guarda el filtro "dominio##selector" en Mis filtros.
 */
(() => {
  if (globalThis.__turboblockPicker) { globalThis.__turboblockPicker.stop(); return; }

  const host = location.hostname.replace(/^www\./, '');
  const wrap = document.createElement('turboblock-picker');
  const root = wrap.attachShadow({ mode: 'closed' });
  root.innerHTML = `<style>
    :host{all:initial}
    .hl{position:fixed;pointer-events:none;z-index:2147483646;border:2px solid #8b5cf6;background:rgba(139,92,246,.18);border-radius:3px;transition:all .05s}
    .panel{position:fixed;left:50%;bottom:16px;transform:translateX(-50%);z-index:2147483647;font:13px/1.4 "Segoe UI",system-ui,sans-serif;
      color:#eceef8;background:#11131d;border:1px solid rgba(255,255,255,.12);border-radius:12px;box-shadow:0 12px 40px rgba(0,0,0,.5);
      padding:10px 12px;display:grid;gap:8px;width:min(560px,calc(100vw - 32px))}
    .row{display:flex;gap:6px;align-items:center}
    .tip{color:#9196ae;font-size:12px}
    input{flex:1;font:12px "Cascadia Mono",Consolas,monospace;color:#eceef8;background:#0a0b12;border:1px solid rgba(255,255,255,.12);border-radius:7px;padding:6px 8px;outline:none}
    input:focus{border-color:#8b5cf6}
    button{font:inherit;font-size:12px;color:#eceef8;background:rgba(255,255,255,.06);border:1px solid rgba(255,255,255,.12);border-radius:7px;padding:5px 10px;cursor:pointer}
    button:hover{background:rgba(255,255,255,.12)}
    button.p{background:linear-gradient(135deg,#8b5cf6,#6366f1 45%,#06b6d4);border-color:transparent;font-weight:600}
    .n{color:#22d3ee;font-variant-numeric:tabular-nums;min-width:70px;text-align:right;font-size:12px}
  </style>
  <div class="hl" hidden></div>
  <div class="panel"><div class="tip">Haz clic en lo que quieres ocultar · ↑ ↓ para ampliar o reducir · Esc para salir</div>
  <div class="row"><input class="sel" spellcheck="false" placeholder="Selector"><span class="n"></span></div>
  <div class="row" style="justify-content:flex-end"><button class="up">Más grande ↑</button><button class="down">Más pequeño ↓</button>
  <button class="cancel">Cancelar</button><button class="p ok">Ocultar</button></div></div>`;
  const hl = root.querySelector('.hl'), input = root.querySelector('.sel'), num = root.querySelector('.n');
  document.documentElement.appendChild(wrap);

  let picked = null, chain = [], depth = 0, locked = false;

  const good = (c) => /^[a-zA-Z_-][\w-]{1,40}$/.test(c) && !/\d{4,}|[a-f0-9]{8,}/i.test(c);
  function part(el) {
    if (el.id && good(el.id)) return '#' + CSS.escape(el.id);
    let s = el.localName;
    const cls = [...el.classList].filter(good).slice(0, 3);
    if (cls.length) s += cls.map((c) => '.' + CSS.escape(c)).join('');
    else if (el.parentElement) {
      const same = [...el.parentElement.children].filter((x) => x.localName === el.localName);
      if (same.length > 1) s += `:nth-of-type(${same.indexOf(el) + 1})`;
    }
    return s;
  }
  function selectorFor(el) {
    const parts = [];
    for (let e = el, i = 0; e && e !== document.documentElement && e !== document.body && i < 4; e = e.parentElement, i++) {
      parts.unshift(part(e));
      if (parts[0].startsWith('#')) break;
      try { if (document.querySelectorAll(parts.join(' > ')).length === 1) break; } catch {}
    }
    return parts.join(' > ');
  }
  function box(el) {
    if (!el) { hl.hidden = true; return; }
    const r = el.getBoundingClientRect();
    Object.assign(hl.style, { left: r.left + 'px', top: r.top + 'px', width: r.width + 'px', height: r.height + 'px' });
    hl.hidden = false;
  }
  function count() {
    try { const n = document.querySelectorAll(input.value).length; num.textContent = n === 1 ? '1 elemento' : `${n} elementos`; return n; }
    catch { num.textContent = 'no válido'; return 0; }
  }
  function choose(el) {
    picked = el;
    input.value = selectorFor(el);
    box(el);
    count();
  }
  const under = (x, y) => {
    wrap.style.display = 'none';
    const el = document.elementFromPoint(x, y);
    wrap.style.display = '';
    return el && el !== document.documentElement && el !== document.body ? el : null;
  };
  function onMove(ev) {
    if (locked) return;
    const el = under(ev.clientX, ev.clientY);
    if (el) { box(el); picked = el; input.value = selectorFor(el); count(); }
  }
  function onClick(ev) {
    if (ev.composedPath().includes(wrap)) return;
    ev.preventDefault(); ev.stopPropagation();
    const el = under(ev.clientX, ev.clientY);
    if (!el) return;
    locked = true; chain = [el]; depth = 0;
    choose(el);
  }
  function move(dir) {
    if (!picked) return;
    if (dir > 0) {
      const p = chain[depth].parentElement;
      if (p && p !== document.body && p !== document.documentElement) { if (depth === chain.length - 1) chain.push(p); depth++; }
    } else if (depth > 0) depth--;
    locked = true;
    choose(chain[depth] || picked);
  }
  function onKey(ev) {
    if (ev.key === 'Escape') { stop(); ev.preventDefault(); }
    else if (ev.key === 'ArrowUp' && ev.target !== input) { move(1); ev.preventDefault(); }
    else if (ev.key === 'ArrowDown' && ev.target !== input) { move(-1); ev.preventDefault(); }
  }
  function block(ev) { if (!ev.composedPath().includes(wrap)) { ev.preventDefault(); ev.stopPropagation(); } }

  input.addEventListener('input', () => { count(); try { box(document.querySelector(input.value)); } catch {} });
  root.querySelector('.up').onclick = () => move(1);
  root.querySelector('.down').onclick = () => move(-1);
  root.querySelector('.cancel').onclick = () => stop();
  root.querySelector('.ok').onclick = () => {
    const sel = input.value.trim();
    if (!sel || !count()) return;
    try { for (const el of document.querySelectorAll(sel)) el.style.setProperty('display', 'none', 'important'); } catch {}
    chrome.runtime.sendMessage({ type: 'addFilter', filter: `${host}##${sel}` });
    stop();
  };

  addEventListener('mousemove', onMove, true);
  addEventListener('click', onClick, true);
  addEventListener('mousedown', block, true);
  addEventListener('mouseup', block, true);
  addEventListener('keydown', onKey, true);

  function stop() {
    removeEventListener('mousemove', onMove, true);
    removeEventListener('click', onClick, true);
    removeEventListener('mousedown', block, true);
    removeEventListener('mouseup', block, true);
    removeEventListener('keydown', onKey, true);
    wrap.remove();
    delete globalThis.__turboblockPicker;
  }
  globalThis.__turboblockPicker = { stop };
})();
