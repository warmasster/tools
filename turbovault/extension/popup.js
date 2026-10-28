// Menú de la extensión: lo guardado para la web abierta, búsqueda, generador.
(() => {
  const $ = (s) => document.querySelector(s);
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const COLORS = ['#7c3aed', '#4f46e5', '#0891b2', '#059669', '#d97706', '#db2777', '#2563eb', '#9333ea', '#0d9488', '#dc2626', '#65a30d', '#c026d3'];
  const colorOf = (s) => { let h = 0; for (const c of s || '?') h = (h * 31 + c.charCodeAt(0)) >>> 0; return COLORS[h % COLORS.length]; };
  const I = {
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    key: '<circle cx="7.5" cy="15.5" r="4.5"/><path d="m10.7 12.3 8.8-8.8"/><path d="m16 7 3 3"/><path d="m18.5 4.5 2 2"/>',
    otp: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M12 8v4l2.5 1.5"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2.5"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    unlock: '<rect x="4" y="11" width="16" height="10" rx="2.5"/><path d="M8 11V7a4 4 0 0 1 7.7-1.5"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${I[n]}</svg>`;

  const bg = (msg) => new Promise((resolve, reject) => {
    chrome.runtime.sendMessage(msg, (r) => {
      if (chrome.runtime.lastError) { reject(new Error(chrome.runtime.lastError.message)); return; }
      if (r && r.ok) resolve(r.data); else reject(Object.assign(new Error((r && r.error) || 'Error'), { code: r && r.code }));
    });
  });

  let info = null;
  let toastT = 0;
  function toast(t) {
    const el = $('#toast');
    el.textContent = t;
    el.hidden = false;
    clearTimeout(toastT);
    toastT = setTimeout(() => (el.hidden = true), 2600);
  }

  async function load() {
    try { info = await bg({ type: 'popup:info' }); } catch (e) { info = { state: { agent: false, error: e.message } }; }
    render();
  }

  function pill() {
    const st = info.state;
    const p = $('#pill');
    p.className = 'pill';
    if (!st.agent) { p.textContent = 'Sin conexión'; return; }
    if (!st.unlocked) { p.textContent = 'Cerrada'; return; }
    // TurboVAULT y Chrome van en el mismo PC: misma hora.
    const left = Math.max(0, st.expires - Date.now());
    const h = Math.floor(left / 3600000), m = Math.floor((left % 3600000) / 60000), s = Math.floor((left % 60000) / 1000);
    const t = h ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`;
    p.textContent = st.expires ? `Abierta · ${t}` : 'Abierta';
    p.classList.add(left < 60000 ? 'soon' : 'open');
  }
  setInterval(() => info && pill(), 1000);

  function render() {
    const st = info.state || {};
    pill();
    $('#lock').hidden = !(st.agent && st.unlocked);
    const main = $('#main');
    if (!st.agent) {
      main.innerHTML = `<div class="center"><img class="big" src="icons/logo.svg" alt=""><b>TurboVAULT no responde</b><p>${esc(st.error || 'Comprueba que TurboVAULT está instalado en este PC.')}</p><button class="btn ghost" id="retry">Reintentar</button></div>`;
      $('#retry').onclick = load;
      return;
    }
    if (!st.exists) {
      main.innerHTML = `<div class="center"><img class="big" src="icons/logo.svg" alt=""><b>Aún no tienes bóveda</b><p>Abre TurboVAULT para crearla con tu contraseña maestra.</p><button class="btn" id="open">Abrir TurboVAULT</button></div>`;
      $('#open').onclick = openApp;
      return;
    }
    if (!st.unlocked) {
      main.innerHTML = `<div class="center"><img class="big" src="icons/logo.svg" alt=""><b>La bóveda está cerrada</b><p>Se abrirá una ventanita de TurboVAULT para escribir tu contraseña maestra. Nunca se escribe en el navegador.</p><button class="btn" id="unlock">${ic('unlock')}Abrir la bóveda</button></div>`;
      $('#unlock').onclick = async () => { try { await bg({ type: 'unlock' }); window.close(); } catch (e) { toast(e.message); } };
      return;
    }
    if (!st.paired) {
      main.innerHTML = `<div class="center"><img class="big" src="icons/logo.svg" alt=""><b>Vincula este navegador</b>
        <p>Por seguridad, TurboVAULT solo da contraseñas a navegadores que tú apruebas. Pulsa «Vincular» y confirma en TurboVAULT que ves este código:</p>
        <div class="code">${esc(info.code)}</div><button class="btn" id="pair">Vincular</button></div>`;
      $('#pair').onclick = async (e) => {
        e.currentTarget.disabled = true;
        try { await bg({ type: 'pair' }); toast('Confirma en la ventana de TurboVAULT'); } catch (err) { toast(err.message); e.currentTarget.disabled = false; }
      };
      return;
    }
    let h = `<div class="sec"><span>En ${esc(info.host || 'esta página')}</span><span>${info.matches.length || ''}</span></div>`;
    if (!info.matches.length) h += `<div class="empty">${info.host ? 'Nada guardado para esta web. Al iniciar sesión te ofrecerá guardarla.' : 'Esta página no admite rellenar.'}</div>`;
    for (const m of info.matches) h += row(m, true);
    h += `<div class="search">${ic('search')}<input id="q" placeholder="Buscar en la bóveda" spellcheck="false" autocomplete="off"></div><div id="results"></div>`;
    h += `<div class="sec"><span>Contraseña nueva</span></div><div class="gen"><div class="out"><div class="pw" id="gpw"></div>
      <button class="ib" id="gnew" title="Otra">${ic('refresh')}</button><button class="ib" id="gcopy" title="Copiar">${ic('copy')}</button></div></div>`;
    main.innerHTML = h;
    bindRows(main);
    const q = $('#q');
    let qt = 0;
    q.addEventListener('input', () => {
      clearTimeout(qt);
      qt = setTimeout(async () => {
        if (q.value.trim().length < 2) { $('#results').innerHTML = ''; return; }
        try {
          const r = await bg({ type: 'popup:search', q: q.value });
          $('#results').innerHTML = r.length ? r.map((m) => row(m, false)).join('') : '<div class="empty">Nada coincide.</div>';
          bindRows($('#results'));
        } catch (e) { toast(e.message); }
      }, 180);
    });
    $('#gnew').onclick = gen;
    $('#gcopy').onclick = async () => {
      try { const r = await bg({ type: 'popup:copyText', text: cur }); toast(`Copiada · se borra en ${r.clearIn} s`); } catch (e) { toast(e.message); }
    };
    gen();
  }

  function row(m, canFill) {
    return `<div class="item"><div class="av" style="--c2:${colorOf(m.host || m.title)}">${esc((m.title || '?').charAt(0))}</div>
      <div class="tx"><b>${esc(m.title)}</b><span>${esc(m.username || m.host || '')}</span></div>
      <div class="acts">
        ${m.username ? `<button class="ib" data-c="username" data-id="${esc(m.id)}" title="Copiar usuario">${ic('user')}</button>` : ''}
        ${m.hasPassword ? `<button class="ib" data-c="password" data-id="${esc(m.id)}" title="Copiar contraseña">${ic('key')}</button>` : ''}
        ${m.hasTotp ? `<button class="ib" data-c="totp" data-id="${esc(m.id)}" title="Copiar código 2FA">${ic('otp')}</button>` : ''}
        ${canFill ? `<button class="fillb" data-fill="${esc(m.id)}">Rellenar</button>` : ''}
      </div></div>`;
  }

  function bindRows(el) {
    el.querySelectorAll('[data-c]').forEach((b) => (b.onclick = async () => {
      try {
        const r = await bg({ type: 'popup:copy', id: b.dataset.id, field: b.dataset.c });
        b.classList.add('done');
        toast(r && r.clearIn ? `Copiado · se borra en ${r.clearIn} s` : 'Copiado');
      } catch (e) { toast(e.message); }
    }));
    el.querySelectorAll('[data-fill]').forEach((b) => (b.onclick = async () => {
      try { await bg({ type: 'popup:fill', id: b.dataset.fill }); window.close(); } catch (e) { toast(e.message); }
    }));
  }

  let cur = '';
  async function gen() {
    try {
      const g = await bg({ type: 'popup:generate' });
      cur = g.password;
      $('#gpw').innerHTML = [...cur].map((c) => (/\d/.test(c) ? `<span class="d">${esc(c)}</span>` : /[^\p{L}\d]/u.test(c) ? `<span class="s">${esc(c)}</span>` : esc(c))).join('');
    } catch (e) { toast(e.message); }
  }

  async function openApp() { try { await bg({ type: 'openApp' }); window.close(); } catch (e) { toast(e.message); } }
  $('#app').onclick = openApp;
  $('#lock').onclick = async () => { try { await bg({ type: 'lock' }); } catch (e) { toast(e.message); } };

  chrome.runtime.onMessage.addListener((msg) => {
    if (msg.type === 'state') load();
  });
  load();
})();
