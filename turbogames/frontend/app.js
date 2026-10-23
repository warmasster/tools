(() => {
  "use strict";

  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const nf0 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 0 });
  const nf1 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 1 });
  const nf2 = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 2 });
  const dtf = new Intl.DateTimeFormat("es-ES", { day: "numeric", month: "short", year: "numeric" });
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };
  const store = {
    get(k, d) { try { const v = localStorage.getItem("tg:" + k); return v == null ? d : JSON.parse(v); } catch { return d; } },
    set(k, v) { try { localStorage.setItem("tg:" + k, JSON.stringify(v)); } catch { } },
  };

  function fmtBytes(n) {
    if (!isFinite(n) || n <= 0) return "0 B";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    const f = i === 0 ? nf0 : (n >= 100 ? nf0 : (i >= 3 && n < 10 ? nf2 : nf1));
    return f.format(n) + " " + u[i];
  }
  function fmtHours(min) {
    if (!min) return "";
    if (min < 60) return nf0.format(min) + " min";
    const h = min / 60;
    return (h >= 100 ? nf0 : nf1).format(h) + " h";
  }
  function ago(unix) {
    if (!unix) return "";
    const s = Math.max(0, Date.now() / 1000 - unix);
    if (s < 90) return "hace un momento";
    if (s < 3600) return "hace " + Math.round(s / 60) + " min";
    if (s < 86400) return "hace " + Math.round(s / 3600) + " h";
    const d = Math.round(s / 86400);
    if (d === 1) return "ayer";
    if (d < 45) return "hace " + d + " días";
    if (d < 365) return "hace " + Math.round(d / 30.4) + " meses";
    const y = Math.round(d / 365);
    return y === 1 ? "hace un año" : "hace " + y + " años";
  }
  const fmtDate = (u) => u ? dtf.format(new Date(u * 1000)) : "—";
  const norm = (s) => String(s || "").toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");

  // ---------- iconos ----------
  const P = {
    users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
    user: '<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>',
    login: '<path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/><path d="m10 17 5-5-5-5"/><path d="M15 12H3"/>',
    plug: '<path d="M12 22v-5"/><path d="M9 8V2"/><path d="M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z"/>',
    copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    alert: '<circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/>',
    eye: '<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>',
    eyeoff: '<path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.53 13.53 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><path d="m2 2 20 20"/>',
    lock: '<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
    unlock: '<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/>',
    shield: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>',
    shieldplus: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="M9 12h6M12 9v6"/>',
    key: '<path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4"/><path d="m21 2-9.6 9.6"/><circle cx="7.5" cy="15.5" r="5.5"/>',
    edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    more: '<circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/>',
    star: '<path d="M11.48 3.5a.56.56 0 0 1 1.04 0l2.13 5.11a.56.56 0 0 0 .48.35l5.52.44a.56.56 0 0 1 .32.99l-4.2 3.6a.56.56 0 0 0-.18.56l1.28 5.39a.56.56 0 0 1-.84.61l-4.73-2.89a.56.56 0 0 0-.59 0l-4.73 2.89a.56.56 0 0 1-.84-.61l1.28-5.39a.56.56 0 0 0-.18-.56l-4.2-3.6a.56.56 0 0 1 .32-.99l5.52-.44a.56.56 0 0 0 .48-.35z"/>',
    hide: '<path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.53 13.53 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><path d="m2 2 20 20"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
    globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    folder: '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/>',
    play: '<path d="m6 3 14 9-14 9V3z"/>',
    game: '<path d="M6 11h4M8 9v4M15 12h.01M18 10h.01"/><path d="M17.32 5H6.68a4 4 0 0 0-3.98 3.59l-.7 6.94A3 3 0 0 0 5 19c1 0 1.5-.5 2-1l1.4-1.4A2 2 0 0 1 9.8 16h4.4a2 2 0 0 1 1.4.6L17 18c.5.5 1 1 2 1a3 3 0 0 0 3-3.47l-.7-6.94A4 4 0 0 0 17.32 5z"/>',
    store: '<path d="m2 7 4.41-4.41A2 2 0 0 1 7.83 2h8.34a2 2 0 0 1 1.42.59L22 7"/><path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8"/><path d="M15 22v-4a2 2 0 0 0-2-2h-2a2 2 0 0 0-2 2v4"/><path d="M2 7h20"/><path d="M22 7v3a2 2 0 0 1-2 2a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 16 12a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 12 12a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 8 12a2.7 2.7 0 0 1-1.59-.63.7.7 0 0 0-.82 0A2.7 2.7 0 0 1 4 12a2 2 0 0 1-2-2V7"/>',
    verify: '<path d="M21.8 10A10 10 0 1 1 17 3.34"/><path d="m9 11 3 3L22 4"/>',
    open: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    download: '<path d="M12 15V3"/><path d="m7 10 5 5 5-5"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>',
    plus: '<path d="M12 5v14"/><path d="M5 12h14"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
    wifioff: '<path d="M12 20h.01"/><path d="M8.5 16.43a5 5 0 0 1 7 0"/><path d="M2 8.82a15 15 0 0 1 4.17-2.65"/><path d="M10.66 5c4.01-.36 8.14.9 11.34 3.76"/><path d="M16.85 11.25a10 10 0 0 1 2.22 1.68"/><path d="M5 13a10 10 0 0 1 5.24-2.76"/><path d="m2 2 20 20"/>',
    id: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="M15 8h2M15 12h2M7 16h10"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${P[n] || ""}</svg>`;

  // ---------- puente con Go (o datos de muestra en el navegador) ----------
  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const rt = window.runtime || { EventsOn() { }, WindowMinimise() { }, WindowToggleMaximise() { }, Quit() { }, BrowserOpenURL(u) { window.open(u); } };
  const api = hasGo ? window.go.main.App : mockApi();

  // ---------- estado ----------
  const S = {
    view: store.get("view", "accounts"),
    steam: { found: true }, settings: {}, vault: {}, version: "",
    accounts: [], accSel: store.get("accSel", ""),
    accFilter: store.get("accFilter", "all"), accTag: "", accSort: store.get("accSort", "recent"), accQ: "",
    games: null, gamesAt: 0, gameSel: 0,
    gameFilter: store.get("gameFilter", "all"), gameAcc: "", gameSort: store.get("gameSort", "recent"), gameQ: "",
    secrets: new Map(), reveal: new Set(),
    switching: null,
  };

  const display = (a) => a.alias || a.persona || a.name;
  const AVC = ["#8b5cf6", "#6366f1", "#0ea5e9", "#14b8a6", "#f59e0b", "#ec4899", "#22c55e", "#ef4444", "#a855f7", "#06b6d4"];
  function hash(s) { let h = 2166136261; for (const c of String(s)) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; }
  function initials(s) {
    const w = String(s || "?").replace(/[_\-.]+/g, " ").trim().split(/\s+/);
    const t = (w.length > 1 ? w[0][0] + w[1][0] : (w[0] || "?").slice(0, 2));
    return t.toUpperCase();
  }
  function avatarHTML(a, cls = "", status = true) {
    const img = a.avatar ? `<img src="${esc(a.avatar)}" alt="" draggable="false" onerror="this.remove()">` : "";
    return `<div class="av ${cls}" style="--avc:${AVC[hash(a.key) % AVC.length]}"><span>${esc(initials(display(a)))}</span>${img}${status && a.active ? '<i class="st"></i>' : ""}</div>`;
  }
  const accByKey = (k) => S.accounts.find((a) => a.key === k);
  const activeAcc = () => S.accounts.find((a) => a.active);

  async function call(fn, ...args) {
    try { return await api[fn](...args); }
    catch (e) { toast(errText(e), "error"); throw e; }
  }
  const errText = (e) => String(e && e.message ? e.message : e).replace(/^Error: /, "");

  // ---------- barra de título ----------
  function renderPills() {
    const s = S.steam, el = $("#pills");
    const parts = [];
    if (!s.found) {
      parts.push(`<span class="pill"><i class="dot bad"></i><b>Steam no encontrado</b></span>`);
    } else if (s.switching) {
      parts.push(`<span class="pill"><i class="dot wait"></i><b>Cambiando de cuenta…</b></span>`);
    } else if (!s.running) {
      parts.push(`<span class="pill clickable" id="pill-steam" title="Abrir Steam"><i class="dot"></i>Steam cerrado</span>`);
    } else {
      const a = S.accounts.find((x) => x.steamId === s.activeId);
      if (s.activeId) parts.push(`<span class="pill clickable" id="pill-steam" title="Ver la cuenta"><i class="dot ok"></i>Steam · <b>${esc(a ? display(a) : "cuenta desconocida")}</b></span>`);
      else parts.push(`<span class="pill"><i class="dot idle"></i>Steam abierto · <b>sin cuenta</b></span>`);
    }
    if (s.runningApp) {
      const g = S.games && S.games.games.find((x) => x.appid === s.runningApp);
      parts.push(`<span class="pill play">${ic("play")}<b>${esc(g ? g.name : (S.runningName || "Jugando"))}</b></span>`);
    }
    el.innerHTML = parts.join("");
    const p = $("#pill-steam");
    if (p) p.addEventListener("click", () => {
      if (!s.running) { openSteam(); return; }
      const a = activeAcc();
      if (a) { setView("accounts"); selectAccount(a.key, true); }
    });
  }

  function renderVaultBtn() {
    const v = S.vault, b = $("#wb-vault");
    b.classList.toggle("open", !!v.unlocked);
    b.classList.toggle("none", !v.exists);
    b.innerHTML = ic(v.unlocked ? "unlock" : v.exists ? "lock" : "shieldplus");
    b.title = v.unlocked ? "Bóveda abierta" : v.exists ? "Abrir la bóveda" : "Crear la bóveda de contraseñas";
  }

  function renderCounts() {
    const vis = S.accounts.filter((a) => !a.hidden).length;
    $("#cnt-accounts").textContent = vis || "";
    $("#cnt-games").textContent = S.games ? (S.games.games.length || "") : "";
  }

  function setView(v) {
    S.view = v;
    store.set("view", v);
    $$("#nav button").forEach((b) => b.classList.toggle("on", b.dataset.view === v));
    const found = S.steam.found;
    $("#v-nosteam").classList.toggle("on", !found);
    $("#v-accounts").classList.toggle("on", found && v === "accounts");
    $("#v-games").classList.toggle("on", found && v === "games");
    if (found && v === "games") ensureGames();
  }

  // ---------- cuentas ----------
  function accMatches(a, q) {
    if (!q) return true;
    const hay = norm([a.name, a.persona, a.alias, a.notes, (a.tags || []).join(" "), a.steamId].join(" "));
    return q.split(/\s+/).every((w) => hay.includes(w));
  }
  function accCounts() {
    const c = { all: 0, token: 0, ask: 0, hidden: 0 };
    for (const a of S.accounts) {
      if (a.hidden) { c.hidden++; continue; }
      c.all++;
      if (a.hasToken || a.active) c.token++; else c.ask++;
    }
    return c;
  }
  function visibleAccounts() {
    const q = norm(S.accQ.trim());
    let list = S.accounts.filter((a) => {
      if (S.accFilter === "hidden") { if (!a.hidden) return false; }
      else if (a.hidden) return false;
      if (S.accFilter === "token" && !(a.hasToken || a.active)) return false;
      if (S.accFilter === "ask" && (a.hasToken || a.active)) return false;
      if (S.accTag && !(a.tags || []).some((t) => t.toLowerCase() === S.accTag.toLowerCase())) return false;
      return accMatches(a, q);
    });
    const by = {
      recent: (x, y) => (y.lastUsed || 0) - (x.lastUsed || 0),
      name: (x, y) => display(x).localeCompare(display(y), "es", { sensitivity: "base" }),
      hours: (x, y) => (y.minutes || 0) - (x.minutes || 0),
    }[S.accSort] || ((x, y) => 0);
    list.sort((x, y) => (y.pinned - x.pinned) || (y.active - x.active) || by(x, y));
    return list;
  }

  function renderAccFilters() {
    const c = accCounts();
    const items = [["all", "Todas", c.all, ""], ["token", "Con sesión", c.token, "Steam entra sin pedir la contraseña"], ["ask", "Sin sesión", c.ask, "Steam pedirá la contraseña"]];
    if (c.hidden) items.push(["hidden", "Ocultas", c.hidden, "Cuentas que has ocultado"]);
    if (S.accFilter === "hidden" && !c.hidden) S.accFilter = "all";
    $("#acc-filter").innerHTML = items.map(([v, l, n, t]) => `<button data-v="${v}" class="${S.accFilter === v ? "on" : ""}"${t ? ` title="${t}"` : ""}>${l}<small>${n}</small></button>`).join("");
    const tags = [...new Set(S.accounts.flatMap((a) => a.tags || []).map((t) => t))].sort((x, y) => x.localeCompare(y, "es"));
    const sel = $("#acc-tag");
    sel.hidden = !tags.length;
    if (S.accTag && !tags.some((t) => t.toLowerCase() === S.accTag.toLowerCase())) S.accTag = "";
    sel.innerHTML = `<option value="">Todas las etiquetas</option>` + tags.map((t) => `<option value="${esc(t)}"${t === S.accTag ? " selected" : ""}>#${esc(t)}</option>`).join("");
    sel.classList.toggle("on", !!S.accTag);
    $("#acc-sort").value = S.accSort;
  }

  function accChips(a, full = false) {
    const out = [];
    if (a.active) out.push(`<span class="chip live">En uso</span>`);
    else if (a.hasToken) out.push(`<span class="chip token" title="Steam tiene la sesión guardada: entra sin contraseña">${ic("check")}Sesión guardada</span>`);
    else if (a.inSteam) out.push(`<span class="chip ask" title="La sesión caducó o no se guardó: Steam pedirá la contraseña">${ic("key")}Pide contraseña</span>`);
    else out.push(`<span class="chip off" title="Steam no la recuerda en este PC">${ic("user")}Solo aquí</span>`);
    if (a.offline) out.push(`<span class="chip off">${ic("wifioff")}Sin conexión</span>`);
    if (a.hasSecret) out.push(`<span class="chip key" title="Tiene datos guardados en la bóveda">${ic("lock")}En la bóveda</span>`);
    if (full && a.autoLogin && !a.active) out.push(`<span class="chip off" title="Steam entrará con ella al abrirse">${ic("login")}Inicio automático</span>`);
    for (const t of a.tags || []) out.push(`<span class="chip tag">#${esc(t)}</span>`);
    return out.join("");
  }

  function renderAccounts() {
    renderAccFilters();
    renderCounts();
    const list = visibleAccounts();
    const grid = $("#acc-grid");
    if (!S.accounts.length) {
      grid.innerHTML = `<div class="empty">${ic("users")}<b>Todavía no hay cuentas</b><p>Entra en Steam con una cuenta (marcando «Recordarme») y aparecerá aquí sola, o añádela a mano.</p><div class="row-btns"><button class="btn primary sm" data-act="add">${ic("plus")}Añadir cuenta</button></div></div>`;
      renderAccDetail();
      return;
    }
    if (!list.length) {
      grid.innerHTML = `<div class="empty">${ic("search")}<b>Ninguna cuenta coincide</b><p>Prueba con otro nombre, etiqueta o filtro.</p></div>`;
      renderAccDetail();
      return;
    }
    if (!S.accSel || !list.some((a) => a.key === S.accSel)) S.accSel = (list.find((a) => a.active) || list[0]).key;
    grid.innerHTML = list.map((a, i) => {
      const bg = a.recent ? `<div class="bg" style="background-image:url('/img/app/${a.recent.appid}/hero')"></div>` : "";
      const line1 = a.active ? (S.steam.runningApp ? "Jugando ahora" : "Dentro ahora mismo") : (a.lastUsed ? "Usada " + ago(a.lastUsed) : "Sin usar en este PC");
      const line2 = a.games ? `<b>${a.games}</b> ${a.games === 1 ? "juego" : "juegos"}${a.minutes ? " · " + fmtHours(a.minutes) : ""}${a.recent ? " · " + esc(a.recent.name) : ""}` : (a.notes ? esc(a.notes.split("\n")[0]) : "&nbsp;");
      return `<div class="acc${a.key === S.accSel ? " sel" : ""}${a.active ? " active" : ""}${a.hidden ? " hidden-acc" : ""}" data-key="${esc(a.key)}" tabindex="-1" style="animation-delay:${Math.min(i, 12) * 0.03}s">${bg}
        <div class="acc-top">${avatarHTML(a)}
          <div class="acc-txt"><div class="acc-name"><span>${esc(display(a))}</span>${a.pinned ? `<svg viewBox="0 0 24 24">${P.star}</svg>` : ""}</div><span class="acc-user">${esc(a.name)}</span></div>
          <button class="ibtn more" data-act="menu" title="Más opciones">${ic("more")}</button>
        </div>
        <div class="acc-chips">${accChips(a)}</div>
        <div class="acc-foot"><div class="meta"><span>${esc(line1)}</span><span>${line2}</span></div>
          <button class="btn primary xs" data-act="switch">${ic("login")}Entrar</button></div>
      </div>`;
    }).join("") + `<button class="acc add-card" data-act="add"><div class="tile">${ic("plus")}</div><b>Añadir cuenta</b><span>Entra con ella en Steam o guárdala aquí con su contraseña</span></button>`;
    renderAccDetail();
  }

  function selectAccount(key, scroll) {
    if (S.accSel === key && !scroll) return;
    S.accSel = key;
    store.set("accSel", key);
    $$("#acc-grid .acc[data-key]").forEach((el) => el.classList.toggle("sel", el.dataset.key === key));
    renderAccDetail();
    if (scroll) {
      const el = $(`#acc-grid .acc[data-key="${CSS.escape(key)}"]`);
      if (el) el.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
  }

  // ---------- detalle de la cuenta ----------
  let detailToken = 0;
  function renderAccDetail() {
    const box = $("#acc-detail");
    const a = accByKey(S.accSel);
    if (!a) {
      box.innerHTML = `<div class="empty-d">${ic("user")}<b>Elige una cuenta</b><span>Aquí verás sus datos, su contraseña guardada y sus juegos.</span></div>`;
      return;
    }
    const tok = ++detailToken;
    const heroBg = a.recent ? `<div class="bg" style="background-image:url('/img/app/${a.recent.appid}/hero')"></div>` : "";
    const actions = a.active
      ? `<button class="btn ghost sm wide" data-act="open-steam">${ic("open")}Abrir Steam</button><button class="ibtn" data-act="menu" title="Más opciones">${ic("more")}</button>`
      : `<button class="btn primary sm" data-act="switch">${ic("login")}Entrar con esta cuenta</button><button class="btn ghost sm" data-act="offline" title="${a.hasToken ? "Entrar en modo sin conexión" : "Sin conexión solo funciona si la cuenta tiene la sesión guardada en este PC"}"${a.hasToken ? "" : " disabled"}>${ic("wifioff")}</button><button class="ibtn" data-act="menu" title="Más opciones">${ic("more")}</button>`;
    box.innerHTML = `
      <div class="d-hero">${heroBg}
        <div class="d-id">${avatarHTML(a, "lg")}
          <div class="txt"><h2>${esc(display(a))}</h2>
            <div class="sub"><span>${esc(a.name)}</span><button class="ibtn" data-copy-field="name" title="Copiar el usuario">${ic("copy")}</button></div>
            ${a.alias && a.persona && a.alias !== a.persona ? `<div class="sub"><span>En Steam: ${esc(a.persona)}</span></div>` : ""}
          </div>
        </div>
        <div class="d-chips">${accChips(a, true)}</div>
        <div class="d-actions">${actions}</div>
      </div>
      <div class="d-sec" id="d-access">${accessHTML(a)}</div>
      <div class="d-sec" id="d-card">
        <div class="sec-h"><span class="label">Ficha</span><span class="saved" id="d-saved">${ic("check")} Guardado</span></div>
        <div class="alias-row"><input class="inp" id="d-alias" maxlength="40" placeholder="Apodo (opcional, solo para ti)" value="${esc(a.alias)}" spellcheck="false"></div>
        <div class="tags" id="d-tags">${(a.tags || []).map((t) => `<span class="chip tag">#${esc(t)}<button data-untag="${esc(t)}" title="Quitar">${ic("x")}</button></span>`).join("")}<input class="tag-add" id="d-tag" maxlength="24" placeholder="+ etiqueta" spellcheck="false" title="Escribe y pulsa Intro"><span class="tag-err" id="d-tag-err"></span></div>
        <textarea class="inp note-area" id="d-notes" maxlength="2000" placeholder="Notas: para qué la usas, con quién la compartes… (no es secreto: para eso está la bóveda)" spellcheck="false">${esc(a.notes)}</textarea>
      </div>
      <div class="d-sec" id="d-games"></div>
      <div class="dfoot">
        ${a.steamId ? `<button class="btn ghost xs" data-act="profile">${ic("globe")}Perfil de Steam</button><button class="btn ghost xs" data-act="copy-id">${ic("id")}Copiar SteamID</button>` : ""}
        <button class="btn ghost xs" data-act="${a.hidden ? "unhide" : "hide"}">${ic(a.hidden ? "eye" : "hide")}${a.hidden ? "Volver a mostrar" : "Ocultar"}</button>
        <button class="btn ghost xs danger-txt" data-act="forget">${ic("trash")}Olvidar…</button>
      </div>`;
    loadSecret(a, tok);
    renderAccGames(a);
  }

  function accessHTML(a, entry) {
    const v = S.vault;
    const head = `<div class="sec-h"><span class="label">Acceso</span>${v.unlocked ? `<span class="mini">${ic("unlock")}</span>` : ""}</div>`;
    const user = `<div class="frow"><span class="k">Usuario</span><span class="v mono">${esc(a.name)}</span><span class="acts"><button class="ibtn" data-copy-field="name" title="Copiar">${ic("copy")}</button></span></div>`;
    if (v.damaged) return head + user + `<div class="warn-box red">El archivo de la bóveda está dañado: ${esc(v.damaged)}</div>`;
    if (!v.exists) {
      return head + user + `<div class="vault-cta"><div class="tile">${ic("shield")}</div><div class="txt"><b>Guarda su contraseña cifrada</b>Crea la bóveda con una contraseña maestra y tendrás todas a un clic.</div><button class="btn primary xs" data-act="vault-create">Crear</button></div>`;
    }
    if (!v.unlocked) {
      const pw = a.hasSecret ? `<span class="v secret">••••••••••</span>` : `<span class="v none">sin guardar</span>`;
      return head + user + `<div class="frow"><span class="k">Contraseña</span>${pw}<span class="acts"><button class="btn ghost xs" data-act="vault-unlock">${ic("lock")}Abrir bóveda</button></span></div>`;
    }
    if (!entry) return head + user + `<div class="frow"><span class="k">Contraseña</span><span class="v none">…</span><span></span></div>`;
    const row = (field, label, value, secret, mono) => {
      if (!value) return "";
      const shown = !secret || S.reveal.has(a.key + ":" + field);
      return `<div class="frow"><span class="k">${label}</span><span class="v${mono ? " mono" : ""}${shown ? "" : " secret"}">${shown ? esc(value) : "••••••••••"}</span><span class="acts">${secret ? `<button class="ibtn" data-reveal="${field}" title="${shown ? "Ocultar" : "Mostrar"}">${ic(shown ? "eyeoff" : "eye")}</button>` : ""}<button class="ibtn" data-copy-field="${field}" title="Copiar">${ic("copy")}</button></span></div>`;
    };
    let rows = entry.password ? row("password", "Contraseña", entry.password, true, true)
      : `<div class="frow"><span class="k">Contraseña</span><span class="v none">sin guardar</span><span class="acts"><button class="btn ghost xs" data-act="secret">${ic("plus")}Añadir</button></span></div>`;
    rows += row("email", "Correo", entry.email, false, false);
    rows += row("emailPassword", "Contr. del correo", entry.emailPassword, true, true);
    rows += row("recovery", "Recuperación", entry.recovery, true, true);
    if (entry.notes) rows += `<div class="frow"><span class="k">Notas privadas</span><span class="v" title="${esc(entry.notes)}">${esc(entry.notes.split("\n")[0])}</span><span></span></div>`;
    const any = entry.password || entry.email || entry.emailPassword || entry.recovery || entry.notes;
    return head + user + rows + (any ? `<div class="sec-foot"><button class="btn ghost xs" data-act="secret">${ic("edit")}Editar datos de acceso</button></div>` : "");
  }

  async function loadSecret(a, tok) {
    if (!S.vault.unlocked) return;
    let e = S.secrets.get(a.key);
    if (!e) {
      try { e = await api.VaultGet(a.name); } catch (err) { await refreshVault(); return; }
      S.secrets.set(a.key, e || {});
    }
    if (tok !== detailToken) return;
    const box = $("#d-access");
    if (box) box.innerHTML = accessHTML(a, e || {});
  }

  function renderAccGames(a) {
    const box = $("#d-games");
    if (!box) return;
    if (!a.steamId || !a.games) {
      box.innerHTML = `<div class="sec-h"><span class="label">Juegos</span></div><div class="note">${a.steamId ? "Esta cuenta aún no ha jugado a nada en este PC." : "Cuando entres con ella, aquí saldrán sus juegos."}</div>`;
      return;
    }
    if (!S.games) {
      box.innerHTML = `<div class="sec-h"><span class="label">Juegos</span><span class="mini">${a.games}</span></div><div class="note">Cargando…</div>`;
      ensureGames().then(() => { if (S.accSel === a.key) renderAccGames(a); });
      return;
    }
    const mine = S.games.games.map((g) => ({ g, p: g.players.find((p) => p.steamId === a.steamId) })).filter((x) => x.p)
      .sort((x, y) => (y.p.lastPlayed - x.p.lastPlayed) || (y.p.minutes - x.p.minutes));
    const top = mine.slice(0, 6);
    box.innerHTML = `<div class="sec-h"><span class="label">Juegos</span><span class="mini">${mine.length}${a.minutes ? " · " + fmtHours(a.minutes) : ""}</span></div>
      <div class="glist">${top.map(({ g, p }) => `<div class="gitem" data-game="${g.appid}">${thumbHTML(g)}<div style="min-width:0"><div class="gn">${esc(g.name)}</div><div class="gs">${[fmtHours(p.minutes), ago(p.lastPlayed)].filter(Boolean).join(" · ") || "—"}</div></div><button class="ibtn" data-play="${g.appid}" title="Jugar con esta cuenta">${ic("play")}</button></div>`).join("")}</div>
      ${mine.length > top.length ? `<div class="sec-foot"><button class="btn ghost xs" data-act="all-games">Ver sus ${mine.length} juegos</button></div>` : ""}`;
  }

  function thumbHTML(g) {
    return `<div class="gthumb" style="background-image:url('/img/app/${g.appid}/header')"></div>`;
  }

  // guarda la ficha al salir del campo
  const savedFlash = debounce(() => { const s = $("#d-saved"); if (s) s.classList.remove("on"); }, 1400);
  async function patchAccount(a, patch) {
    try {
      await api.UpdateAccount(a.name, patch);
      Object.assign(a, Object.fromEntries(Object.entries(patch).map(([k, v]) => [k, v])));
      const s = $("#d-saved"); if (s) { s.classList.add("on"); savedFlash(); }
      return true;
    } catch (e) { toast(errText(e), "error"); return false; }
  }

  // ---------- acciones de cuenta ----------
  // opts: offline, appid (juego a abrir), forceAsk (preguntar aunque esté
  // desactivado en Ajustes), note (texto extra para la pregunta), alt y onAlt
  // (otro botón en la pregunta)
  async function switchTo(a, opts = {}) {
    if (!a) return;
    if (S.switching) { toast("Ya se está cambiando de cuenta", "error"); return; }
    const st = S.steam;
    if (!st.found) { toast("No encuentro Steam en este PC", "error"); return; }
    if (a.active && !opts.offline && !opts.appid) { toast("Ya estás dentro con " + display(a)); return; }
    if (opts.offline && !a.hasToken) {
      toast("Para entrar sin conexión, la cuenta tiene que tener la sesión guardada en este PC (entrar antes marcando «Recordarme»)", "error");
      return;
    }
    const willClose = st.running && !(a.active && !opts.offline);
    const game = st.runningApp && willClose ? (S.games && S.games.games.find((x) => x.appid === st.runningApp)) : null;
    const target = opts.appid && S.games && S.games.games.find((x) => x.appid === opts.appid);
    const asked = (st.runningApp && willClose) || opts.forceAsk || (S.settings.confirmSwitch !== false && (willClose || !a.hasToken));
    if (asked) {
      let html = opts.note ? `<p>${opts.note}</p>` : "";
      html += `<p>${willClose ? "Steam se cerrará un momento y volverá a abrirse" : "Steam se abrirá"} con <b>${esc(display(a))}</b>${opts.offline ? " en modo sin conexión" : ""}.${target ? ` Después se abrirá <b>${esc(target.name)}</b>.` : ""}</p>`;
      if (st.runningApp && willClose) html += `<div class="warn-box red">También se cerrará <b>${esc(game ? game.name : S.runningName || "el juego que tienes abierto")}</b>. Guarda la partida antes.</div>`;
      if (!a.hasToken) html += `<div class="warn-box">Esta cuenta no tiene la sesión guardada: Steam te pedirá la contraseña.${a.hasSecret ? " La tienes en la bóveda y podrás copiarla desde aquí." : " No la tienes guardada en la bóveda."}</div>`;
      const r = await ask({
        title: target ? `¿Jugar con ${display(a)}?` : `¿Cambiar a ${display(a)}?`,
        html,
        ok: target ? "Cambiar y jugar" : opts.offline ? "Entrar sin conexión" : "Cambiar de cuenta",
        alt: opts.alt,
        check: S.settings.confirmSwitch !== false && !opts.forceAsk ? "No volver a preguntar" : "",
      });
      if (r.checked) {
        S.settings.confirmSwitch = false;
        api.SaveSettings({ ...S.settings }).then(() => toast("Ya no te preguntaré. Puedes volver a activarlo en Ajustes.")).catch(() => { });
      }
      if (r.alt) { if (opts.onAlt) opts.onAlt(); return; }
      if (!r.ok) return;
    }
    showSwitch(a, opts);
    try {
      const r = await api.SwitchTo({ name: a.name, offline: !!opts.offline, appid: opts.appid || 0 });
      finishSwitch(a, r, opts);
    } catch (e) {
      hideSwitch();
      toast(errText(e), "error");
    }
    loadAccounts();
  }

  async function copyField(a, field) {
    try {
      const secs = await api.CopyField(a.name, field);
      const labels = { name: "Usuario copiado", password: "Contraseña copiada", email: "Correo copiado", emailPassword: "Contraseña del correo copiada", recovery: "Código de recuperación copiado" };
      if (secs) toast(`${labels[field]} · se borrará en ${secs} s`, "secret", secs);
      else toast(labels[field] || "Copiado");
      return true;
    } catch (e) {
      const m = errText(e);
      if (/cerrada/i.test(m)) { openVault("unlock", () => copyField(a, field)); return false; }
      toast(m, "error");
      return false;
    }
  }

  function openProfile(a) { if (a.steamId) rt.BrowserOpenURL("https://steamcommunity.com/profiles/" + a.steamId); }

  function accMenu(a, x, y) {
    const items = [{ head: display(a) }];
    if (!a.active) {
      items.push({ icon: "login", label: "Entrar con esta cuenta", fn: () => switchTo(a) });
      items.push({ icon: "wifioff", label: "Entrar sin conexión", fn: () => switchTo(a, { offline: true }), disabled: !a.hasToken });
    } else items.push({ icon: "open", label: "Abrir Steam", fn: openSteam });
    items.push("-");
    items.push({ icon: "copy", label: "Copiar usuario", fn: () => copyField(a, "name") });
    if (a.hasSecret) items.push({ icon: "key", label: "Copiar contraseña", fn: () => copyField(a, "password") });
    items.push({ icon: "edit", label: S.vault.exists ? "Datos de acceso…" : "Guardar su contraseña…", fn: () => editSecret(a) });
    items.push("-");
    items.push({ icon: "star", label: a.pinned ? "Quitar de fijadas" : "Fijar arriba", fn: async () => { await patchAccount(a, { pinned: !a.pinned }); renderAccounts(); } });
    if (a.steamId) items.push({ icon: "globe", label: "Abrir su perfil de Steam", fn: () => openProfile(a) });
    items.push({ icon: a.hidden ? "eye" : "hide", label: a.hidden ? "Volver a mostrar" : "Ocultar", fn: () => setHidden(a, !a.hidden) });
    items.push({ icon: "trash", label: "Olvidar…", danger: true, fn: () => openForget(a) });
    showMenu(items, x, y);
  }

  async function setHidden(a, hidden) {
    if (await patchAccount(a, { hidden })) {
      toast(hidden ? `${display(a)} oculta. La tienes en el filtro «Ocultas».` : `${display(a)} vuelve a la lista`);
      await loadAccounts();
    }
  }

  // ---------- cambio de cuenta (pantalla) ----------
  const STEPS = [["close", "Cerrar Steam"], ["prepare", "Preparar la cuenta"], ["open", "Abrir Steam"], ["login", "Entrar"]];
  function showSwitch(a, opts) {
    S.switching = { a, opts, step: "", done: false };
    const sw = $("#switch");
    sw.classList.remove("done", "warn");
    $("#sw-av").innerHTML = avatarHTML({ ...a, active: false });
    $("#sw-title").textContent = (opts.offline ? "Entrando sin conexión como " : "Entrando como ") + display(a);
    const g = opts.appid && S.games && S.games.games.find((x) => x.appid === opts.appid);
    $("#sw-sub").textContent = g ? "Y después se abrirá " + g.name : (S.steam.running ? "Steam se cerrará un momento" : "");
    $("#sw-steps").innerHTML = STEPS.map(([k, l]) => `<li data-step="${k}"><i>${ic("check")}</i>${l}</li>`).join("");
    $("#sw-help").hidden = true;
    $("#sw-btns").innerHTML = "";
    sw.hidden = false;
    S.steam.switching = true;
    renderPills();
  }
  function setStep(step) {
    if (!S.switching) return;
    const order = STEPS.map((s) => s[0]);
    const idx = order.indexOf(step);
    if (idx >= 0) {
      $$("#sw-steps li").forEach((li, i) => {
        li.classList.toggle("ok", i < idx);
        li.classList.toggle("doing", i === idx);
      });
    }
    const g = S.switching.opts.appid && S.games && S.games.games.find((x) => x.appid === S.switching.opts.appid);
    const subs = { close: "Cerrando Steam con cuidado…", prepare: "Dejando la cuenta lista para entrar sola", open: "Abriendo Steam…", login: g ? "Esperando a que entre. Después se abrirá " + g.name : "Esperando a que Steam entre…" };
    if (subs[step]) $("#sw-sub").textContent = subs[step];
    if (step === "login") {
      $("#sw-btns").innerHTML = `<button class="btn ghost sm" id="sw-stop">Dejar de esperar</button>`;
      $("#sw-stop").onclick = () => api.CancelWait();
    }
    if (step === "password") showPwHelp();
    if (step === "game") $("#sw-sub").textContent = "Abriendo el juego…";
  }
  function showPwHelp() {
    const a = S.switching && S.switching.a;
    if (!a) return;
    const h = $("#sw-help");
    const canPw = S.vault.exists && a.hasSecret;
    h.innerHTML = `<b>${ic("key")}Steam te pide la contraseña</b>
      <p>${a.hasToken ? "La sesión guardada ha caducado." : "Esta cuenta no tiene la sesión guardada."} Escribe el usuario y la contraseña en Steam y marca «Recordarme».</p>
      <div class="row-btns"><button class="btn ghost xs" data-sw-copy="name">${ic("copy")}Copiar usuario</button>
      ${canPw ? `<button class="btn primary xs" data-sw-copy="password">${ic("key")}Copiar contraseña</button>` : `<span class="note">Si guardas su contraseña en la bóveda, la tendrás aquí a un clic.</span>`}</div>`;
    h.hidden = false;
    $$("[data-sw-copy]", h).forEach((b) => b.onclick = async () => {
      if (await copyField(a, b.dataset.swCopy)) { b.classList.add("done"); b.innerHTML = ic("check") + "Copiado"; }
    });
  }
  function finishSwitch(a, r, opts) {
    const sw = $("#switch");
    $$("#sw-steps li").forEach((li) => { li.classList.remove("doing"); });
    if (r.already) { hideSwitch(); toast(opts.appid ? "Abriendo el juego…" : "Ya estabas dentro con " + display(a)); return; }
    if (r.canceled) { hideSwitch(); toast("Steam sigue entrando por su cuenta"); return; }
    if (r.ok) {
      $$("#sw-steps li").forEach((li) => li.classList.add("ok"));
      sw.classList.add("done");
      $("#sw-title").textContent = "Dentro como " + display(a);
      $("#sw-sub").textContent = opts.appid ? "El juego se está abriendo" : "Listo";
      $("#sw-help").hidden = true;
      $("#sw-btns").innerHTML = "";
      setTimeout(hideSwitch, 1300);
      return;
    }
    sw.classList.add("done", "warn");
    $("#sw-ok").innerHTML = ic("alert");
    if (r.other) {
      $("#sw-title").textContent = "Steam ha entrado con " + r.other;
      $("#sw-sub").textContent = "Se eligió otra cuenta en la ventana de Steam.";
    } else {
      $("#sw-title").textContent = "Steam no ha terminado de entrar";
      $("#sw-sub").textContent = r.needLogin ? "Sigue esperando la contraseña en su ventana." : "Mira la ventana de Steam por si te pide algo.";
    }
    $("#sw-btns").innerHTML = `<button class="btn primary sm" id="sw-close">Entendido</button>`;
    $("#sw-close").onclick = hideSwitch;
  }
  function hideSwitch() {
    $("#switch").hidden = true;
    S.steam.switching = false;
    $("#sw-ok").innerHTML = ic("check");
    S.switching = null;
    refreshStatus();
  }

  // ---------- juegos ----------
  let gamesLoading = null;
  function ensureGames(force) {
    if (gamesLoading) return gamesLoading;
    if (S.games && !force && Date.now() - S.gamesAt < 30000) { renderGames(); return Promise.resolve(); }
    if (!S.games) $("#game-grid").innerHTML = `<div class="empty"><div class="spinner"></div><b>Leyendo tu biblioteca…</b></div>`;
    gamesLoading = api.Games().then((d) => {
      S.games = d || { games: [], libraries: [] };
      S.gamesAt = Date.now();
      renderGames();
      renderPills();
      renderCounts();
    }).catch((e) => {
      $("#game-grid").innerHTML = `<div class="empty">${ic("alert")}<b>No se pudo leer la biblioteca</b><p>${esc(errText(e))}</p></div>`;
    }).finally(() => { gamesLoading = null; });
    return gamesLoading;
  }

  function visibleGames() {
    const q = norm(S.gameQ.trim());
    const acc = S.gameAcc && accByKey(S.gameAcc);
    let list = S.games.games.filter((g) => {
      if (S.gameFilter === "installed" && !g.installed) return false;
      if (acc && !g.players.some((p) => p.steamId === acc.steamId) && g.owner !== acc.steamId) return false;
      if (q && !q.split(/\s+/).every((w) => norm(g.name).includes(w) || String(g.appid) === w)) return false;
      return true;
    });
    const minutesOf = (g) => acc ? ((g.players.find((p) => p.steamId === acc.steamId) || {}).minutes || 0) : g.minutes;
    const lastOf = (g) => acc ? ((g.players.find((p) => p.steamId === acc.steamId) || {}).lastPlayed || 0) : g.lastPlayed;
    const by = {
      recent: (x, y) => (lastOf(y) - lastOf(x)) || x.name.localeCompare(y.name, "es"),
      name: (x, y) => x.name.localeCompare(y.name, "es", { sensitivity: "base" }),
      hours: (x, y) => (minutesOf(y) - minutesOf(x)) || x.name.localeCompare(y.name, "es"),
      size: (x, y) => (y.size - x.size) || x.name.localeCompare(y.name, "es"),
    }[S.gameSort];
    list.sort((x, y) => (y.running - x.running) || by(x, y));
    return { list, minutesOf, lastOf };
  }

  function renderGameAccSelect() {
    const sel = $("#game-acc");
    const withGames = S.accounts.filter((a) => a.steamId && (a.games || S.games && S.games.games.some((g) => g.owner === a.steamId)));
    if (S.gameAcc && !withGames.some((a) => a.key === S.gameAcc)) S.gameAcc = "";
    sel.innerHTML = `<option value="">Todas las cuentas</option>` + withGames.map((a) => `<option value="${esc(a.key)}"${a.key === S.gameAcc ? " selected" : ""}>${esc(display(a))}</option>`).join("");
    sel.classList.toggle("on", !!S.gameAcc);
    sel.hidden = withGames.length < 2;
  }

  function renderGames() {
    if (!S.games) return;
    renderGameAccSelect();
    $$("#game-filter button").forEach((b) => b.classList.toggle("on", b.dataset.v === S.gameFilter));
    $("#game-sort").value = S.gameSort;
    const { list, minutesOf, lastOf } = visibleGames();
    const inst = S.games.games.filter((g) => g.installed);
    const size = inst.reduce((s, g) => s + (g.size || 0), 0);
    $("#game-stat").innerHTML = `<b>${list.length}</b> de ${S.games.games.length} · ${inst.length} instalados · ${fmtBytes(size)}`;
    const grid = $("#game-grid");
    if (!S.games.games.length) {
      grid.innerHTML = `<div class="empty">${ic("game")}<b>No hay juegos todavía</b><p>Aquí salen los juegos instalados y los que ha jugado cada cuenta en este PC.</p></div>`;
      renderGameDetail();
      return;
    }
    if (!list.length) {
      grid.innerHTML = `<div class="empty">${ic("search")}<b>Ningún juego coincide</b><p>Prueba con otro nombre o quita los filtros.</p></div>`;
      renderGameDetail();
      return;
    }
    if (!S.gameSel || !list.some((g) => g.appid === S.gameSel)) S.gameSel = list[0].appid;
    grid.innerHTML = list.map((g, i) => {
      const m = minutesOf(g), l = lastOf(g);
      const meta = [fmtHours(m), l ? ago(l) : (g.installed ? "Sin jugar" : "")].filter(Boolean).join(" · ") || (g.installed ? fmtBytes(g.size) : "—");
      const hue = hash(g.appid) % 360;
      return `<div class="game${g.appid === S.gameSel ? " sel" : ""}${g.installed ? "" : " notinst"}" data-appid="${g.appid}" style="animation-delay:${Math.min(i, 24) * 0.015}s">
        <div class="cover"><div class="ph" style="--ph1:hsl(${hue} 55% 38%);--ph2:hsl(${(hue + 50) % 360} 60% 24%)">${esc(g.name)}</div>
          <img loading="lazy" src="/img/app/${g.appid}/cover" alt="" draggable="false" onload="this.classList.add('ok')" onerror="this.remove()">
          <div class="shade"></div>
          <div class="badges">${g.running ? `<span class="run">En marcha</span>` : ""}${g.type === "demo" ? "<span>Demo</span>" : ""}${STATE[g.state] ? `<span class="st-${g.state}">${STATE[g.state].short}</span>` : ""}</div>
          <button class="playb" data-play="${g.appid}" title="${g.installed ? "Jugar" : "Instalar o jugar"}">${ic("play")}</button>
        </div>
        <div class="gname" title="${esc(g.name)}">${esc(g.name)}</div>
        <div class="gmeta">${g.installed ? "<i></i>" : ""}<span>${esc(meta)}</span></div>
      </div>`;
    }).join("");
    renderGameDetail();
  }

  // estados de instalación que hay que avisar (los da Go en g.state)
  const STATE = {
    update: { short: "Actualizar", long: "Necesita actualizarse", cls: "ask", tip: "Steam lo actualizará antes de abrirlo" },
    updating: { short: "Actualizando", long: "Actualizándose", cls: "token", tip: "Steam lo está descargando o verificando" },
    incomplete: { short: "Incompleto", long: "Instalación incompleta", cls: "ask", tip: "Le faltan archivos o no terminó de instalarse" },
    missing: { short: "Falta la carpeta", long: "Falta la carpeta", cls: "bad", tip: "Steam lo da por instalado pero su carpeta ya no está" },
  };

  function selectGame(appid) {
    S.gameSel = appid;
    $$("#game-grid .game").forEach((el) => el.classList.toggle("sel", +el.dataset.appid === appid));
    renderGameDetail();
  }

  // a qué cuenta jugar por defecto: la que está dentro si lo ha jugado (o lo
  // instaló), si no la que más recientemente lo jugó
  function bestAccountFor(g) {
    const active = activeAcc();
    const known = (sid) => S.accounts.find((a) => a.steamId === sid && !a.hidden) || S.accounts.find((a) => a.steamId === sid);
    if (active && (g.players.some((p) => p.steamId === active.steamId) || g.owner === active.steamId)) return active;
    for (const p of g.players) { const a = known(p.steamId); if (a) return a; }
    if (g.owner) { const a = known(g.owner); if (a) return a; }
    return active || null;
  }

  // playGame abre un juego comprobando antes que se puede: que no esté ya
  // abierto, que esté instalado y bien, y con qué cuenta. Si hay que cambiar
  // de cuenta, siempre pregunta.
  async function playGame(g, a) {
    if (!S.steam.found) { toast("No encuentro Steam en este PC", "error"); return; }
    if (g.running) { toast(`${g.name} ya está abierto`); return; }
    const chosen = !!a;
    a = a || bestAccountFor(g);
    const active = activeAcc();
    const fail = (e) => toast(errText(e), "error");
    const needSwitch = a && !a.active;

    if (!g.installed) {
      if (!needSwitch) {
        const r = await ask({ title: `${g.name} no está instalado`, html: "<p>Steam te ofrecerá instalarlo. Si la cuenta abierta no lo tiene, te llevará a la tienda.</p>", ok: "Instalar" });
        if (r.ok) api.SteamURL("steam://install/" + g.appid).then(() => toast("Steam te pedirá dónde instalarlo")).catch(fail);
        return;
      }
      switchTo(a, { appid: g.appid, forceAsk: true, note: `<b>${esc(g.name)}</b> no está instalado: al entrar con la cuenta que lo ha jugado, Steam te ofrecerá instalarlo.` });
      return;
    }
    if (g.state === "missing") {
      const r = await ask({ title: "Falta la carpeta del juego", html: `<p>Steam cree que <b>${esc(g.name)}</b> está instalado en</p><div class="warn-box"><span class="mono">${esc(g.dir)}</span></div><p style="margin-top:8px">pero esa carpeta ya no existe (quizá la borraste a mano). Al verificar los archivos, Steam lo vuelve a descargar.</p>`, ok: "Verificar archivos" });
      if (r.ok) api.SteamURL("steam://validate/" + g.appid).then(() => toast("Steam está verificando los archivos")).catch(fail);
      return;
    }
    if (g.state === "incomplete") {
      const r = await ask({ title: "La instalación no está completa", html: `<p>A <b>${esc(g.name)}</b> le faltan archivos o no terminó de instalarse. Steam lo terminará antes de abrirlo.</p>`, ok: "Seguir" });
      if (!r.ok) return;
    }
    if (S.steam.runningApp && !needSwitch) {
      const other = S.games && S.games.games.find((x) => x.appid === S.steam.runningApp);
      const r = await ask({ title: "Ya hay un juego abierto", html: `<p>Tienes abierto <b>${esc(other ? other.name : S.runningName || "otro juego")}</b>. Steam suele abrir solo uno a la vez.</p>`, ok: "Abrir igualmente" });
      if (!r.ok) return;
    }
    const pre = g.state === "updating" ? "Steam lo está actualizando: se abrirá cuando termine" : g.state === "update" ? "Steam lo actualizará antes de abrirlo" : "";
    const launch = async () => {
      try { await api.PlayGame(g.appid, ""); toast(pre || `Abriendo ${g.name}…`); }
      catch (e) { fail(e); }
    };
    if (!needSwitch) { launch(); return; }

    // hay que cambiar de cuenta: preguntar siempre y ofrecer jugar con la actual
    let note = "";
    if (!chosen && active && !g.players.some((p) => p.steamId === active.steamId)) {
      note = `<b>${esc(display(active))}</b>, la cuenta abierta ahora, no lo ha jugado en este PC; <b>${esc(display(a))}</b> sí.`;
    }
    switchTo(a, {
      appid: g.appid, forceAsk: true, note,
      alt: active && S.steam.running ? `Jugar con ${display(active)}` : "",
      onAlt: launch,
    });
  }

  function renderGameDetail() {
    const box = $("#game-detail");
    const g = S.games && S.games.games.find((x) => x.appid === S.gameSel);
    if (!g) {
      box.innerHTML = `<div class="empty-d">${ic("game")}<b>Elige un juego</b><span>Verás quién lo ha jugado, cuánto ocupa y podrás abrirlo con la cuenta que quieras.</span></div>`;
      return;
    }
    const best = bestAccountFor(g);
    const playLabel = best && !best.active ? `Jugar con ${esc(display(best))}` : (g.installed ? "Jugar" : "Instalar");
    const players = g.players.map((p) => {
      const a = S.accounts.find((x) => x.steamId === p.steamId);
      const av = a ? avatarHTML(a, "sm", false) : `<div class="av sm" style="--avc:#475569"><span>${esc(initials(p.persona || "?"))}</span></div>`;
      const name = a ? display(a) : (p.persona || "Cuenta sin guardar");
      return `<div class="pitem">${av}<div style="min-width:0"><div class="pn">${esc(name)}${a && a.active ? ' <span class="chip live" style="margin-left:4px">En uso</span>' : ""}</div><div class="ps">${esc(ago(p.lastPlayed) || "—")}</div></div><span class="ph">${esc(fmtHours(p.minutes) || "—")}</span>${a ? `<button class="btn ghost xs" data-play-as="${esc(a.key)}" title="${a.active ? "Jugar" : "Cambiar a esta cuenta y jugar"}">${ic("play")}</button>` : "<span></span>"}</div>`;
    }).join("");
    box.innerHTML = `
      <div class="g-hero" style="background-image:url('/img/app/${g.appid}/hero'),url('/img/app/${g.appid}/header')"><div class="logo" style="background-image:url('/img/app/${g.appid}/logo')"></div></div>
      <div class="g-head">
        <h2>${esc(g.name)}</h2>
        <div class="d-chips">${g.running ? '<span class="chip live">En marcha</span>' : ""}${g.installed ? (STATE[g.state] ? `<span class="chip ${STATE[g.state].cls}" title="${esc(STATE[g.state].tip)}">${ic("alert")}${STATE[g.state].long}</span>` : `<span class="chip token">${ic("check")}Instalado</span>`) : `<span class="chip off">No instalado</span>`}${g.type === "demo" ? '<span class="chip">Demo</span>' : ""}</div>
        <div class="g-play"><button class="btn go sm" data-act="play">${ic("play")}${playLabel}</button>${S.accounts.filter((a) => a.steamId).length > 1 ? `<button class="ibtn" data-act="play-menu" title="Jugar con otra cuenta">${ic("users")}</button>` : ""}<button class="ibtn" data-act="game-menu" title="Más opciones">${ic("more")}</button></div>
      </div>
      <div class="d-sec">
        <div class="facts">
          <div class="fact"><b>${g.minutes ? fmtHours(g.minutes) : "—"}</b><span>jugado en este PC</span></div>
          <div class="fact"><b>${g.lastPlayed ? esc(ago(g.lastPlayed)) : "—"}</b><span>última partida</span></div>
          <div class="fact"><b>${g.installed ? fmtBytes(g.size) : "—"}</b><span>en disco</span></div>
          <div class="fact"><b>${g.updated ? fmtDate(g.updated) : "—"}</b><span>actualizado</span></div>
        </div>
        ${g.installed && g.dir ? `<div class="path"><span title="${esc(g.dir)}">&lrm;${esc(g.dir)}&lrm;</span><button class="ibtn" data-act="folder" title="Abrir la carpeta">${ic("folder")}</button></div>` : ""}
      </div>
      <div class="d-sec">
        <div class="sec-h"><span class="label">Quién lo ha jugado</span><span class="mini">${g.players.length || ""}</span></div>
        ${players ? `<div class="plist">${players}</div>` : `<div class="note">Ninguna cuenta lo ha jugado todavía en este PC${g.owner ? "" : ""}.</div>`}
      </div>
      <div class="dfoot">
        <button class="btn ghost xs" data-act="steam-lib">${ic("open")}Ver en Steam</button>
        <button class="btn ghost xs" data-act="store">${ic("store")}Tienda</button>
        ${g.installed ? `<button class="btn ghost xs" data-act="verify">${ic("verify")}Verificar archivos</button>` : `<button class="btn ghost xs" data-act="install">${ic("download")}Instalar</button>`}
      </div>`;
  }

  function gameMenu(g, x, y) {
    showMenu([
      { head: g.name },
      { icon: "open", label: "Ver en la biblioteca de Steam", fn: () => api.SteamURL("steam://nav/games/details/" + g.appid).catch((e) => toast(errText(e), "error")) },
      { icon: "store", label: "Página de la tienda", fn: () => rt.BrowserOpenURL("https://store.steampowered.com/app/" + g.appid) },
      g.installed ? { icon: "folder", label: "Abrir la carpeta", fn: () => api.OpenFolder(g.dir).catch((e) => toast(errText(e), "error")) } : null,
      g.installed ? { icon: "verify", label: "Verificar archivos", fn: () => api.SteamURL("steam://validate/" + g.appid).catch((e) => toast(errText(e), "error")) } : { icon: "download", label: "Instalar", fn: () => playGame(g) },
      g.installed ? { icon: "copy", label: "Copiar la ruta", fn: () => api.CopyText(g.dir).then(() => toast("Ruta copiada")) } : null,
      { icon: "id", label: "Copiar el número del juego (" + g.appid + ")", fn: () => api.CopyText(String(g.appid)).then(() => toast("Copiado")) },
    ].filter(Boolean), x, y);
  }

  function playAsMenu(g, x, y) {
    const accs = S.accounts.filter((a) => a.steamId && !a.hidden);
    const played = new Set(g.players.map((p) => p.steamId));
    accs.sort((p, q) => (played.has(q.steamId) - played.has(p.steamId)) || (q.lastUsed - p.lastUsed));
    showMenu([{ head: "Jugar con…" }, ...accs.map((a) => ({
      html: `${avatarHTML(a, "sm", false)}<span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis">${esc(display(a))}</span>${a.active ? '<span class="chip live">En uso</span>' : played.has(a.steamId) ? `<span class="chip">${esc(fmtHours((g.players.find((p) => p.steamId === a.steamId) || {}).minutes) || "jugado")}</span>` : ""}`,
      fn: () => playGame(g, a),
    }))], x, y);
  }

  // ---------- bóveda ----------
  async function refreshVault() {
    try { S.vault = await api.VaultState(); } catch { }
    if (!S.vault.unlocked) { S.secrets.clear(); S.reveal.clear(); }
    renderVaultBtn();
  }

  function strength(pw) {
    let s = 0;
    if (pw.length >= 8) s++;
    if (pw.length >= 12) s++;
    if (pw.length >= 16) s++;
    if (/[a-z]/.test(pw) && /[A-Z]/.test(pw)) s++;
    if (/\d/.test(pw)) s++;
    if (/[^\w\s]/.test(pw)) s++;
    const lvl = pw.length < 8 ? 0 : Math.min(4, Math.max(1, Math.round(s * 4 / 6)));
    return { lvl, label: ["Muy corta", "Débil", "Aceptable", "Buena", "Muy buena"][lvl], color: ["#f87171", "#f97316", "#fbbf24", "#34d399", "#22d3ee"][lvl] };
  }

  let vaultThen = null;
  function openVault(mode, then) {
    vaultThen = then || null;
    const m = $("#m-vault"), f = $("#mv-form"), go = $("#mv-go");
    m.dataset.mode = mode;
    go.className = "btn primary sm";
    const pwField = (id, label, auto) => `<label class="field"><span>${label}</span><div class="pw"><input id="${id}" type="password" spellcheck="false" autocomplete="${auto || "off"}"><button type="button" class="ibtn" data-eye="${id}" title="Mostrar">${ic("eye")}</button></div></label>`;
    if (mode === "create") {
      $("#mv-title").textContent = "Crear la bóveda";
      f.innerHTML = `<div class="v-hero"><div class="tile">${ic("shield")}</div><p>Guarda las contraseñas de tus cuentas cifradas en este PC. Elige una contraseña maestra: no se guarda en ningún sitio y, si la olvidas, no hay forma de recuperar lo que haya dentro.</p></div>
        ${pwField("mv-a", "Contraseña maestra", "new-password")}<div class="strength"><i id="mv-str"></i></div><div class="hint" id="mv-strt">Al menos 8 caracteres. Mejor una frase larga que no uses en otro sitio.</div><div style="height:12px"></div>
        ${pwField("mv-b", "Repítela", "new-password")}<div class="hint" id="mv-err"></div>`;
      go.textContent = "Crear bóveda";
    } else if (mode === "unlock") {
      $("#mv-title").textContent = "Abrir la bóveda";
      f.innerHTML = `<div class="v-hero"><div class="tile">${ic("lock")}</div><p>Escribe la contraseña maestra. Se cerrará sola tras ${S.settings.lockMinutes || 5} min sin usarla.</p></div>${pwField("mv-a", "Contraseña maestra", "current-password")}<div class="hint" id="mv-err"></div>`;
      go.textContent = "Abrir";
    } else if (mode === "change") {
      $("#mv-title").textContent = "Cambiar la contraseña maestra";
      f.innerHTML = `${pwField("mv-o", "Contraseña maestra actual", "current-password")}${pwField("mv-a", "Nueva contraseña maestra", "new-password")}<div class="strength"><i id="mv-str"></i></div><div class="hint" id="mv-strt">Al menos 8 caracteres.</div><div style="height:12px"></div>${pwField("mv-b", "Repite la nueva", "new-password")}<div class="hint" id="mv-err"></div>`;
      go.textContent = "Cambiar";
    } else if (mode === "destroy") {
      $("#mv-title").textContent = "Borrar la bóveda";
      f.innerHTML = `<div class="warn-box red">Se borrarán para siempre todas las contraseñas, correos y notas privadas guardadas. Las cuentas siguen en la lista.</div>${pwField("mv-a", "Contraseña maestra para confirmar", "current-password")}<div class="hint" id="mv-err"></div>`;
      go.textContent = "Borrar la bóveda";
      go.className = "btn danger-solid sm";
    }
    wireEyes(f);
    const a = $("#mv-a");
    if (a && $("#mv-str")) a.addEventListener("input", () => {
      const s = strength(a.value);
      $("#mv-str").style.width = (a.value ? (s.lvl + 1) * 20 : 0) + "%";
      $("#mv-str").style.background = s.color;
      $("#mv-strt").textContent = a.value ? s.label + (a.value.length < 8 ? ` · faltan ${8 - a.value.length} caracteres` : "") : "Al menos 8 caracteres. Mejor una frase larga que no uses en otro sitio.";
    });
    m.hidden = false;
    setTimeout(() => ($("#mv-o") || a).focus(), 50);
  }

  async function submitVault(e) {
    e.preventDefault();
    const m = $("#m-vault"), mode = m.dataset.mode, go = $("#mv-go");
    const val = (id) => ($(id) ? $(id).value : "");
    const err = (t) => { $("#mv-err").textContent = t; $("#mv-err").classList.add("bad"); const c = $(".modal-card", m); c.classList.remove("shake"); void c.offsetWidth; c.classList.add("shake"); };
    const a = val("#mv-a"), b = val("#mv-b");
    if ((mode === "create" || mode === "change") && a.length < 8) return err("La contraseña maestra debe tener al menos 8 caracteres.");
    if ((mode === "create" || mode === "change") && a !== b) return err("Las dos contraseñas no coinciden.");
    if (mode === "change" && !val("#mv-o")) return err("Escribe la contraseña maestra actual.");
    if (mode === "change" && val("#mv-o") === a) return err("La nueva es igual que la actual.");
    if ((mode === "create" || mode === "change") && a.trim() !== a) return err("La contraseña maestra no puede empezar ni acabar con un espacio.");
    if (!a) return err("Escribe la contraseña maestra.");
    go.classList.add("busy");
    try {
      if (mode === "create") S.vault = await api.VaultCreate(a);
      else if (mode === "unlock") S.vault = await api.VaultUnlock(a);
      else if (mode === "change") S.vault = await api.VaultChangeMaster(val("#mv-o"), a);
      else if (mode === "destroy") S.vault = await api.VaultDestroy(a);
      m.hidden = true;
      S.secrets.clear(); S.reveal.clear();
      renderVaultBtn();
      toast({ create: "Bóveda creada", unlock: "Bóveda abierta", change: "Contraseña maestra cambiada", destroy: "Bóveda borrada" }[mode]);
      await loadAccounts();
      const then = vaultThen; vaultThen = null;
      if (then && S.vault.unlocked) then();
    } catch (ex) {
      err(errText(ex));
      const i = $("#mv-o") || $("#mv-a"); if (i) { i.select(); i.focus(); }
    } finally { go.classList.remove("busy"); }
  }

  let popTimer = null;
  function openVaultPop() {
    const pop = $("#vault-pop");
    if (!pop.hidden) { closePop(); return; }
    const draw = () => {
      const left = Math.max(0, S.vault.locksIn | 0);
      const mm = Math.floor(left / 60), ss = String(left % 60).padStart(2, "0");
      pop.innerHTML = `<div class="pop-head"><div class="tile" style="--tile:linear-gradient(135deg,#22d3ee,#6366f1)">${ic("unlock")}</div><div><b>Bóveda abierta</b><span>Se cierra sola en ${mm}:${ss}</span></div></div>
        <button class="mi" data-v="lock">${ic("lock")}Cerrar ahora</button>
        <button class="mi" data-v="change">${ic("key")}Cambiar la contraseña maestra</button>
        <hr><button class="mi danger" data-v="destroy">${ic("trash")}Borrar la bóveda…</button>`;
    };
    draw();
    const r = $("#wb-vault").getBoundingClientRect();
    pop.style.top = r.bottom + 6 + "px";
    pop.style.left = Math.max(8, Math.min(window.innerWidth - 292, r.right - 280)) + "px";
    pop.hidden = false;
    clearInterval(popTimer);
    popTimer = setInterval(() => { S.vault.locksIn = Math.max(0, (S.vault.locksIn || 0) - 1); draw(); }, 1000);
    pop.onclick = async (e) => {
      const b = e.target.closest("[data-v]");
      if (!b) return;
      closePop();
      if (b.dataset.v === "lock") { S.vault = await api.VaultLock(); S.secrets.clear(); S.reveal.clear(); renderVaultBtn(); renderAccDetail(); toast("Bóveda cerrada"); }
      else openVault(b.dataset.v);
    };
  }
  function closePop() { $("#vault-pop").hidden = true; clearInterval(popTimer); }

  async function vaultClick() {
    await refreshVault();
    if (S.vault.damaged) { toast("El archivo de la bóveda está dañado: " + S.vault.damaged, "error"); return; }
    if (!S.vault.exists) openVault("create");
    else if (!S.vault.unlocked) openVault("unlock");
    else openVaultPop();
  }

  // datos de acceso de una cuenta
  let secretFor = null;
  async function editSecret(a) {
    await refreshVault();
    if (!S.vault.exists) { openVault("create", () => editSecret(a)); return; }
    if (!S.vault.unlocked) { openVault("unlock", () => editSecret(a)); return; }
    let e;
    try { e = await api.VaultGet(a.name); } catch (err) { toast(errText(err), "error"); return; }
    secretFor = a;
    $("#ms-title").textContent = "Datos de acceso de " + display(a);
    $("#ms-pass").value = e.password || "";
    $("#ms-email").value = e.email || "";
    $("#ms-epass").value = e.emailPassword || "";
    $("#ms-rec").value = e.recovery || "";
    $("#ms-notes").value = e.notes || "";
    wireEyes($("#m-secret"));
    clearErrs($("#m-secret"));
    secretOrig = secretValues();
    $("#ms-clear").hidden = !(e.password || e.email || e.emailPassword || e.recovery || e.notes);
    $("#m-secret").hidden = false;
    setTimeout(() => $("#ms-pass").focus(), 50);
  }
  let secretOrig = "";
  const secretValues = () => JSON.stringify(["#ms-pass", "#ms-email", "#ms-epass", "#ms-rec", "#ms-notes"].map((i) => $(i).value));
  function secretCheck() {
    const email = $("#ms-email").value.trim(), rec = $("#ms-rec").value.trim(), epass = $("#ms-epass").value;
    let ok = fieldErr($("#ms-email"), emailError(email));
    ok = fieldErr($("#ms-rec"), recoveryError(rec)) && ok;
    ok = fieldErr($("#ms-epass"), epass && !email ? "Falta el correo de esta contraseña" : "") && ok;
    fieldErr($("#ms-pass"), spaceHint($("#ms-pass").value), true);
    return ok;
  }
  async function saveSecret(ev, clear) {
    if (ev) ev.preventDefault();
    const a = secretFor;
    if (!a) return;
    if (clear && !(await confirmBox("¿Borrar sus datos de acceso?", `<p>Se borrarán la contraseña, el correo, el código de recuperación y las notas privadas de <b>${esc(display(a))}</b>.</p>`, "Borrar", true))) return;
    if (!clear && !secretCheck()) { shake($("#m-secret")); return; }
    if (!clear && secretValues() === secretOrig) { secretOrig = ""; $("#m-secret").hidden = true; return; }
    const e = clear ? {} : { password: $("#ms-pass").value, email: $("#ms-email").value.trim(), emailPassword: $("#ms-epass").value, recovery: $("#ms-rec").value.trim().toUpperCase(), notes: $("#ms-notes").value };
    if (!clear && !e.password && !e.email && !e.emailPassword && !e.recovery && !e.notes && secretOrig.replace(/[\[\]",]/g, "") !== "") {
      if (!(await confirmBox("¿Dejarla sin datos?", `<p>Has vaciado todos los campos: se borrará lo que había guardado de <b>${esc(display(a))}</b>.</p>`, "Borrar", true))) return;
    }
    try {
      await api.VaultSet(a.name, e);
      S.secrets.delete(a.key);
      secretOrig = "";
      $("#m-secret").hidden = true;
      toast(clear || (!e.password && !e.email && !e.recovery && !e.notes && !e.emailPassword) ? "Datos de acceso borrados" : "Guardado en la bóveda");
      await loadAccounts();
    } catch (err) { toast(errText(err), "error"); }
  }

  // ---------- añadir / olvidar ----------
  function openAdd() {
    $("#ma-name").value = "";
    $("#ma-pass").value = "";
    clearErrs($("#m-add"));
    const v = S.vault;
    $("#ma-pass").disabled = false;
    $("#ma-note").textContent = !v.exists ? "La contraseña se guardará cifrada: primero te pediré crear la bóveda." : !v.unlocked ? "Para guardar la contraseña te pediré abrir la bóveda." : "La contraseña se guarda cifrada en la bóveda.";
    wireEyes($("#m-add"));
    $("#m-add").hidden = false;
    setTimeout(() => $("#ma-name").focus(), 50);
  }
  // addCheck comprueba el formulario de añadir y dice qué cuenta sería
  function addCheck() {
    const inp = $("#ma-name"), name = inp.value.trim();
    let msg = accountNameError(name);
    const dup = !msg && S.accounts.find((x) => x.key === name.toLowerCase());
    if (dup && !dup.hidden) msg = `Ya la tienes en la lista${dup.persona && dup.persona !== dup.name ? " (" + dup.persona + ")" : ""}`;
    fieldErr(inp, msg);
    if (!msg && dup && dup.hidden) fieldErr(inp, "Estaba oculta: volverá a la lista", true);
    return { name, ok: !msg, dup };
  }
  async function addManual(andEnter) {
    const { name, ok, dup } = addCheck();
    const pass = $("#ma-pass").value;
    if (!ok) { shake($("#m-add")); $("#ma-name").focus(); return; }
    if (pass && pass.trim() === "") { fieldErr($("#ma-pass"), "La contraseña no puede ser solo espacios"); shake($("#m-add")); return; }
    if (dup && dup.hasSecret && pass) {
      if (!(await confirmBox("¿Cambiar su contraseña guardada?", `<p><b>${esc(display(dup))}</b> ya tiene una contraseña en la bóveda. Se cambiará por la nueva.</p>`, "Cambiarla"))) return;
    }
    const doIt = async () => {
      try {
        const res = await api.AddManualAccount(name);
        if (pass) {
          const prev = dup && S.vault.unlocked ? await api.VaultGet(name).catch(() => ({})) : {};
          await api.VaultSet(name, { ...prev, password: pass });
        }
        $("#m-add").hidden = true;
        await loadAccounts();
        const a = S.accounts.find((x) => x.key === name.toLowerCase());
        if (a) {
          S.accFilter = "all";
          selectAccount(a.key, true);
          renderAccounts();
          if (andEnter) switchTo(a);
          else toast(res && res.restored ? `${display(a)} vuelve a la lista` : `${name} guardada`);
        }
      } catch (e) { fieldErr($("#ma-name"), errText(e)); $("#m-add").hidden = false; shake($("#m-add")); }
    };
    if (pass && !S.vault.unlocked) {
      $("#m-add").hidden = true;
      openVault(S.vault.exists ? "unlock" : "create", doIt);
      return;
    }
    doIt();
  }
  async function addViaSteam() {
    if (!S.steam.found) { toast("No encuentro Steam en este PC", "error"); return; }
    if (S.switching || S.steam.switching) { toast("Ya se está cambiando de cuenta", "error"); return; }
    $("#m-add").hidden = true;
    if (S.steam.running && (S.steam.runningApp || S.settings.confirmSwitch !== false)) {
      let html = "<p>Steam se cerrará y volverá a abrirse en su pantalla de inicio de sesión, sin entrar en ninguna cuenta.</p>";
      if (S.steam.runningApp) html += `<div class="warn-box red">También se cerrará <b>${esc(S.runningName || "el juego que tienes abierto")}</b>. Guarda la partida antes.</div>`;
      if (!(await confirmBox("¿Abrir el inicio de sesión de Steam?", html, "Cerrar Steam y seguir"))) return;
    }
    S.steam.switching = true; renderPills();
    try {
      await api.AddAccount();
      toast("Entra en Steam con la cuenta nueva. Aparecerá aquí sola.");
    } catch (e) { toast(errText(e), "error"); }
    refreshStatus();
  }

  let forgetFor = null;
  function openForget(a) {
    forgetFor = a;
    $("#mf-title").textContent = "¿Olvidar " + display(a) + "?";
    $("#mf-text").innerHTML = a.inSteam
      ? `Se quitará de la lista de TurboGAMES. Como Steam la sigue recordando, si no marcas la primera opción quedará en «Ocultas».`
      : `Se quitará de la lista de TurboGAMES.`;
    $("#mf-steam-row").hidden = !a.inSteam;
    $("#mf-steam").checked = false;
    $("#mf-vault-row").hidden = !(a.hasSecret && S.vault.exists);
    $("#mf-vault").checked = a.hasSecret && S.vault.exists;
    const note = () => {
      const on = $("#mf-steam").checked && S.steam.running;
      $("#mf-note").hidden = !on;
      $("#mf-note").innerHTML = on ? `Steam se cerrará un momento para olvidarla y volverá a abrirse${a.active ? ", sin entrar en ninguna cuenta (es la que está abierta ahora)" : ""}.${S.steam.runningApp ? " <b>También se cerrará el juego que tienes abierto.</b>" : ""}` : "";
    };
    $("#mf-steam").onchange = note;
    note();
    $("#m-forget").hidden = false;
  }
  async function doForget() {
    const a = forgetFor;
    if (!a) return;
    const o = { steam: a.inSteam && $("#mf-steam").checked, vault: a.hasSecret && S.vault.exists && $("#mf-vault").checked };
    if (o.vault && !S.vault.unlocked) { $("#m-forget").hidden = true; openVault("unlock", () => { openForget(a); }); return; }
    if (o.steam && (S.switching || S.steam.switching)) { toast("Espera a que termine el cambio de cuenta", "error"); return; }
    const btn = $("#mf-go");
    btn.classList.add("busy");
    try {
      await api.ForgetAccount(a.name, o);
      $("#m-forget").hidden = true;
      toast(o.steam ? `Steam ha olvidado ${display(a)}` : a.inSteam ? `${display(a)} está ahora en «Ocultas»` : `${display(a)} olvidada`);
      S.secrets.delete(a.key);
      await loadAccounts();
    } catch (e) { toast(errText(e), "error"); }
    finally { btn.classList.remove("busy"); }
  }

  // ---------- confirmación ----------
  // ask enseña una pregunta y devuelve { ok, alt, checked }.
  function ask({ title, html, ok, danger, alt, check }) {
    return new Promise((resolve) => {
      const m = $("#m-confirm");
      $("#mc-title").textContent = title;
      $("#mc-text").innerHTML = html;
      const go = $("#mc-go"), altB = $("#mc-alt");
      go.textContent = ok;
      go.className = "btn sm " + (danger ? "danger-solid" : "primary");
      altB.hidden = !alt;
      altB.textContent = alt || "";
      $("#mc-check-row").hidden = !check;
      $("#mc-check").checked = false;
      $("#mc-check-t").textContent = check || "";
      let done = false;
      const finish = (v) => {
        if (done) return;
        done = true;
        obs.disconnect();
        m.hidden = true;
        resolve({ ok: v === "ok", alt: v === "alt", checked: v !== "no" && $("#mc-check").checked });
      };
      go.onclick = () => finish("ok");
      altB.onclick = () => finish("alt");
      const obs = new MutationObserver(() => { if (m.hidden) finish("no"); });
      obs.observe(m, { attributes: true, attributeFilter: ["hidden"] });
      m.hidden = false;
      setTimeout(() => go.focus(), 50);
    });
  }
  async function confirmBox(title, html, okLabel, danger) {
    return (await ask({ title, html, ok: okLabel, danger })).ok;
  }

  // ---------- errores en los formularios ----------
  function fieldErr(input, msg, soft) {
    const field = input.closest(".field");
    let h = field && field.querySelector(".hint.ferr");
    if (field && !h) { h = document.createElement("div"); h.className = "hint ferr"; field.appendChild(h); }
    if (h) { h.textContent = msg || ""; h.hidden = !msg; h.classList.toggle("bad", !soft); h.classList.toggle("caps", !!soft); }
    input.classList.toggle("bad", !!msg && !soft);
    return !msg || soft;
  }
  function clearErrs(root) {
    $$(".hint.ferr, .hint.caps", root).forEach((h) => { h.textContent = h.classList.contains("ferr") ? "" : h.textContent; h.hidden = true; });
    $$(".bad", root).forEach((i) => { if (!i.classList.contains("hint")) i.classList.remove("bad"); });
  }
  function shake(m) { const c = $(".modal-card", m); c.classList.remove("shake"); void c.offsetWidth; c.classList.add("shake"); }

  // las mismas reglas que comprueba Go (accountNameError)
  function accountNameError(n) {
    if (!n) return "Escribe el usuario de Steam";
    if (n.includes("@")) return "Es el usuario con el que entras en Steam, no el correo";
    if (/\s/.test(n)) return "El usuario de Steam no lleva espacios";
    if (n.length < 3) return "El usuario de Steam tiene al menos 3 caracteres";
    if (n.length > 64) return "El usuario de Steam tiene como mucho 64 caracteres";
    if (!/^[A-Za-z0-9_]+$/.test(n)) return "El usuario de Steam solo lleva letras sin acentos, números y _";
    return "";
  }
  function emailError(v) {
    if (!v) return "";
    const at = v.lastIndexOf("@");
    if (at < 1 || !v.slice(at + 1).includes(".") || /[\s,;]/.test(v) || v.endsWith(".")) return "Ese correo no parece válido";
    return "";
  }
  function recoveryError(v) {
    if (!v) return "";
    return /^R\d{5}$/i.test(v) ? "" : "Es una R y 5 números, como R12345";
  }
  const spaceHint = (v) => v && v !== v.trim() ? "Ojo: empieza o acaba con un espacio" : "";

  // aviso de Bloq Mayús en los campos de contraseña
  function capsHint(input, on) {
    const field = input.closest(".field");
    if (!field) return;
    let h = field.querySelector(".hint.capslock");
    if (!h && on) { h = document.createElement("div"); h.className = "hint caps capslock"; h.textContent = "Bloq Mayús está activado"; field.appendChild(h); }
    if (h) h.hidden = !on;
  }
  for (const ev of ["keydown", "keyup"]) {
    document.addEventListener(ev, (e) => { if (e.target.matches && e.target.matches(".pw input") && e.getModifierState) capsHint(e.target, e.getModifierState("CapsLock")); });
  }
  document.addEventListener("focusout", (e) => { if (e.target.matches && e.target.matches(".pw input")) capsHint(e.target, false); });

  // ---------- ajustes ----------
  let pendingDir = null;
  function openSettings() {
    const s = S.settings;
    pendingDir = s.steamDir || "";
    const drawDir = () => {
      $("#s-dir").textContent = pendingDir || (S.steam.found ? S.steam.dir + " (automática)" : "No encontrada");
      $("#s-dir-auto").hidden = !pendingDir;
    };
    drawDir();
    $("#s-dir-pick").onclick = async () => { try { const d = await api.PickSteamDir(); if (d) { pendingDir = d; drawDir(); } } catch (e) { toast(errText(e), "error"); } };
    $("#s-dir-auto").onclick = () => { pendingDir = ""; drawDir(); };
    $("#s-silent").checked = !!s.startSilent;
    $("#s-confirm").checked = s.confirmSwitch !== false;
    setSeg("#s-after", s.afterSwitch || "nada");
    setSeg("#s-lock", String(s.lockMinutes || 5));
    setSeg("#s-clip", String(s.clipSeconds || 20));
    $("#s-ver").textContent = "TurboGAMES " + (S.version || "");
    $("#m-settings").hidden = false;
  }
  function setSeg(sel, v) { $$(sel + " button").forEach((b) => b.classList.toggle("on", b.dataset.v === v)); }
  const segVal = (sel) => { const b = $(sel + " button.on"); return b ? b.dataset.v : ""; };
  async function saveSettings() {
    const s = { steamDir: pendingDir || "", startSilent: $("#s-silent").checked, confirmSwitch: $("#s-confirm").checked, afterSwitch: segVal("#s-after"), lockMinutes: +segVal("#s-lock"), clipSeconds: +segVal("#s-clip") };
    try {
      S.steam = await api.SaveSettings(s);
      S.settings = s;
      $("#m-settings").hidden = true;
      toast("Ajustes guardados");
      setView(S.view);
      await refreshVault();
      await loadAccounts();
      if (S.games) ensureGames(true);
    } catch (e) { toast(errText(e), "error"); }
  }

  // ---------- menú contextual ----------
  function showMenu(items, x, y) {
    const m = $("#ctx");
    m.innerHTML = items.map((it, i) => {
      if (it === "-") return "<hr>";
      if (it.head) return `<div class="ch">${esc(it.head)}</div>`;
      return `<button data-i="${i}" class="${it.danger ? "danger" : ""}"${it.disabled ? " disabled" : ""}>${it.html || ic(it.icon) + esc(it.label)}</button>`;
    }).join("");
    m.hidden = false;
    const r = m.getBoundingClientRect();
    m.style.left = Math.max(8, Math.min(x, window.innerWidth - r.width - 8)) + "px";
    m.style.top = Math.max(56, Math.min(y, window.innerHeight - r.height - 8)) + "px";
    m.onclick = (e) => {
      const b = e.target.closest("button[data-i]");
      if (!b) return;
      m.hidden = true;
      const it = items[+b.dataset.i];
      if (it && it.fn) it.fn();
    };
  }
  const closeMenu = () => { $("#ctx").hidden = true; };

  // ---------- avisos ----------
  function toast(text, kind = "ok", secs) {
    const el = document.createElement("div");
    el.className = "toast " + kind;
    el.innerHTML = ic(kind === "error" ? "alert" : kind === "secret" ? "lock" : "check") + `<span>${esc(text)}</span>`;
    let dur = kind === "error" ? 6000 : 2800;
    if (kind === "secret" && secs) {
      dur = Math.min(secs, 8) * 1000;
      const bar = document.createElement("i");
      bar.className = "bar";
      bar.animate([{ transform: "scaleX(1)" }, { transform: "scaleX(0)" }], { duration: dur, easing: "linear", fill: "forwards" });
      el.appendChild(bar);
    }
    $("#toasts").appendChild(el);
    setTimeout(() => { el.classList.add("out"); setTimeout(() => el.remove(), 260); }, dur);
  }

  function wireEyes(root) {
    $$("[data-eye]", root).forEach((b) => {
      const inp = $("#" + b.dataset.eye);
      inp.type = "password";
      b.innerHTML = ic("eye");
      b.onclick = () => { const show = inp.type === "password"; inp.type = show ? "text" : "password"; b.innerHTML = ic(show ? "eyeoff" : "eye"); b.title = show ? "Ocultar" : "Mostrar"; };
    });
  }

  // ---------- carga ----------
  async function loadAccounts() {
    try { S.accounts = await api.Accounts() || []; }
    catch (e) { toast("No se pudieron leer las cuentas: " + errText(e), "error"); }
    renderAccounts();
    renderPills();
    if (S.view === "games" && S.games) { renderGameAccSelect(); renderGameDetail(); }
  }
  const loadAccountsSoon = debounce(loadAccounts, 300);

  async function refreshStatus() {
    try { S.steam = await api.Status(); } catch { }
    renderPills();
  }

  async function openSteam() {
    try { await api.OpenSteam(); } catch (e) { toast(errText(e), "error"); }
  }

  // ---------- eventos de la interfaz ----------
  $("#wb-min").addEventListener("click", () => rt.WindowMinimise());
  $("#wb-max").addEventListener("click", () => rt.WindowToggleMaximise());
  $("#wb-close").addEventListener("click", () => rt.Quit());
  $(".titlebar").addEventListener("dblclick", (e) => { if (!e.target.closest("button,.pill,.nav")) rt.WindowToggleMaximise(); });
  $("#wb-settings").addEventListener("click", openSettings);
  $("#wb-steam").addEventListener("click", openSteam);
  $("#wb-vault").addEventListener("click", (e) => { e.stopPropagation(); vaultClick(); });
  $$("#nav button").forEach((b) => b.addEventListener("click", () => setView(b.dataset.view)));

  // buscadores
  for (const [id, key, render] of [["acc-q", "accQ", () => renderAccounts()], ["game-q", "gameQ", () => renderGames()]]) {
    const inp = $("#" + id);
    const r = debounce(render, 60);
    inp.addEventListener("input", () => { S[key] = inp.value; inp.parentElement.classList.toggle("has", !!inp.value); r(); });
    inp.addEventListener("keydown", (e) => { if (e.key === "Escape" && inp.value) { e.stopPropagation(); inp.value = ""; S[key] = ""; inp.parentElement.classList.remove("has"); render(); } });
  }
  $$("[data-clear]").forEach((b) => b.addEventListener("click", () => { const i = $("#" + b.dataset.clear); i.value = ""; i.dispatchEvent(new Event("input")); i.focus(); }));

  $("#acc-filter").addEventListener("click", (e) => { const b = e.target.closest("button"); if (!b) return; S.accFilter = b.dataset.v; store.set("accFilter", S.accFilter); renderAccounts(); });
  $("#acc-tag").addEventListener("change", (e) => { S.accTag = e.target.value; renderAccounts(); });
  $("#acc-sort").addEventListener("change", (e) => { S.accSort = e.target.value; store.set("accSort", S.accSort); renderAccounts(); });
  $("#acc-add").addEventListener("click", openAdd);
  $("#game-filter").addEventListener("click", (e) => { const b = e.target.closest("button"); if (!b) return; S.gameFilter = b.dataset.v; store.set("gameFilter", S.gameFilter); renderGames(); });
  $("#game-acc").addEventListener("change", (e) => { S.gameAcc = e.target.value; renderGames(); });
  $("#game-sort").addEventListener("change", (e) => { S.gameSort = e.target.value; store.set("gameSort", S.gameSort); renderGames(); });

  // tarjetas de cuenta
  const grid = $("#acc-grid");
  grid.addEventListener("click", (e) => {
    const add = e.target.closest("[data-act=add]");
    if (add) { openAdd(); return; }
    const card = e.target.closest(".acc[data-key]");
    if (!card) return;
    const a = accByKey(card.dataset.key);
    const act = e.target.closest("[data-act]");
    if (act && act.dataset.act === "switch") { selectAccount(a.key); switchTo(a); return; }
    if (act && act.dataset.act === "menu") { const r = act.getBoundingClientRect(); selectAccount(a.key); accMenu(a, r.left - 200, r.bottom + 4); return; }
    selectAccount(a.key);
  });
  grid.addEventListener("dblclick", (e) => {
    const card = e.target.closest(".acc[data-key]");
    if (!card || e.target.closest("button")) return;
    const a = accByKey(card.dataset.key);
    if (a && !a.active) switchTo(a);
  });
  grid.addEventListener("contextmenu", (e) => {
    const card = e.target.closest(".acc[data-key]");
    if (!card) return;
    e.preventDefault();
    const a = accByKey(card.dataset.key);
    selectAccount(a.key);
    accMenu(a, e.clientX, e.clientY);
  });

  // panel de la cuenta
  const det = $("#acc-detail");
  det.addEventListener("click", async (e) => {
    const a = accByKey(S.accSel);
    if (!a) return;
    const cf = e.target.closest("[data-copy-field]");
    if (cf) {
      if (await copyField(a, cf.dataset.copyField)) { cf.classList.add("done"); cf.innerHTML = ic("check"); setTimeout(() => { cf.classList.remove("done"); cf.innerHTML = ic("copy"); }, 1400); }
      return;
    }
    const rv = e.target.closest("[data-reveal]");
    if (rv) {
      const k = a.key + ":" + rv.dataset.reveal;
      if (S.reveal.has(k)) S.reveal.delete(k); else S.reveal.add(k);
      api.VaultTouch().then((v) => { S.vault = v; });
      $("#d-access").innerHTML = accessHTML(a, S.secrets.get(a.key) || {});
      return;
    }
    const un = e.target.closest("[data-untag]");
    if (un) {
      const tags = (a.tags || []).filter((t) => t !== un.dataset.untag);
      if (await patchAccount(a, { tags })) { renderAccDetail(); renderAccounts(); }
      return;
    }
    const pl = e.target.closest("[data-play]");
    if (pl) { const g = S.games && S.games.games.find((x) => x.appid === +pl.dataset.play); if (g) playGame(g, a); return; }
    const gi = e.target.closest("[data-game]");
    if (gi) { S.gameSel = +gi.dataset.game; S.gameAcc = a.key; setView("games"); renderGames(); return; }
    const act = e.target.closest("[data-act]");
    if (!act) return;
    switch (act.dataset.act) {
      case "switch": switchTo(a); break;
      case "offline": switchTo(a, { offline: true }); break;
      case "open-steam": openSteam(); break;
      case "menu": { const r = act.getBoundingClientRect(); accMenu(a, r.left - 200, r.bottom + 4); break; }
      case "vault-create": openVault("create", () => editSecret(a)); break;
      case "vault-unlock": openVault("unlock"); break;
      case "secret": editSecret(a); break;
      case "profile": openProfile(a); break;
      case "copy-id": api.CopyText(a.steamId).then(() => toast("SteamID copiado")); break;
      case "hide": setHidden(a, true); break;
      case "unhide": setHidden(a, false); break;
      case "forget": openForget(a); break;
      case "all-games": S.gameAcc = a.key; setView("games"); renderGames(); break;
    }
  });
  det.addEventListener("change", async (e) => {
    const a = accByKey(S.accSel);
    if (!a) return;
    if (e.target.id === "d-alias") { if (await patchAccount(a, { alias: e.target.value.trim() })) { $(".d-id h2").textContent = display(a); renderAccounts(); } }
    if (e.target.id === "d-notes") { if (await patchAccount(a, { notes: e.target.value })) renderAccountsSoon(); }
  });
  det.addEventListener("keydown", async (e) => {
    const a = accByKey(S.accSel);
    if (!a) return;
    if (e.target.id === "d-alias" && e.key === "Enter") { e.target.blur(); }
    if (e.target.id === "d-tag" && (e.key === "Enter" || e.key === ",")) {
      e.preventDefault();
      const t = e.target.value.replace(/^#+/, "").trim();
      const errEl = $("#d-tag-err");
      const say = (m) => { if (errEl) errEl.textContent = m; };
      if (!t) { say(""); return; }
      if ((a.tags || []).some((x) => x.toLowerCase() === t.toLowerCase())) { say("Ya tiene esa etiqueta"); return; }
      if ((a.tags || []).length >= 8) { say("Como mucho 8 etiquetas por cuenta"); return; }
      if (/[\s,#]/.test(t)) { say("Sin espacios, comas ni #: usa una palabra o guiones"); return; }
      say("");
      const tags = [...(a.tags || []), t];
      if (await patchAccount(a, { tags })) {
        await loadAccounts();
        const i = $("#d-tag"); if (i) i.focus();
      }
    }
    if (e.target.id === "d-tag" && e.key === "Backspace" && !e.target.value && (a.tags || []).length) {
      const tags = a.tags.slice(0, -1);
      if (await patchAccount(a, { tags })) { await loadAccounts(); const i = $("#d-tag"); if (i) i.focus(); }
    }
  });
  const renderAccountsSoon = debounce(() => { const keep = document.activeElement && document.activeElement.id; if (!keep || !keep.startsWith("d-")) renderAccounts(); }, 400);

  // juegos
  const ggrid = $("#game-grid");
  ggrid.addEventListener("click", (e) => {
    const card = e.target.closest(".game[data-appid]");
    if (!card) return;
    const g = S.games.games.find((x) => x.appid === +card.dataset.appid);
    if (e.target.closest("[data-play]")) { selectGame(g.appid); playGame(g); return; }
    selectGame(g.appid);
  });
  ggrid.addEventListener("dblclick", (e) => {
    const card = e.target.closest(".game[data-appid]");
    if (!card) return;
    const g = S.games.games.find((x) => x.appid === +card.dataset.appid);
    if (g) playGame(g);
  });
  ggrid.addEventListener("contextmenu", (e) => {
    const card = e.target.closest(".game[data-appid]");
    if (!card) return;
    e.preventDefault();
    const g = S.games.games.find((x) => x.appid === +card.dataset.appid);
    selectGame(g.appid);
    gameMenu(g, e.clientX, e.clientY);
  });
  $("#game-detail").addEventListener("click", (e) => {
    const g = S.games && S.games.games.find((x) => x.appid === S.gameSel);
    if (!g) return;
    const pa = e.target.closest("[data-play-as]");
    if (pa) { playGame(g, accByKey(pa.dataset.playAs)); return; }
    const act = e.target.closest("[data-act]");
    if (!act) return;
    const r = act.getBoundingClientRect();
    const fail = (err) => toast(errText(err), "error");
    switch (act.dataset.act) {
      case "play": playGame(g); break;
      case "play-menu": playAsMenu(g, r.left - 210, r.bottom + 4); break;
      case "game-menu": gameMenu(g, r.left - 210, r.bottom + 4); break;
      case "folder": api.OpenFolder(g.dir).catch(fail); break;
      case "steam-lib": api.SteamURL("steam://nav/games/details/" + g.appid).catch(fail); break;
      case "store": rt.BrowserOpenURL("https://store.steampowered.com/app/" + g.appid); break;
      case "verify": api.SteamURL("steam://validate/" + g.appid).then(() => toast("Steam está verificando los archivos")).catch(fail); break;
      case "install": playGame(g); break;
    }
  });

  // modales
  // closeModal cierra un modal; si es el de datos de acceso con cambios sin
  // guardar, pregunta antes
  async function closeModal(m) {
    if (m.id === "m-secret" && secretOrig && secretValues() !== secretOrig) {
      if (!(await confirmBox("¿Descartar los cambios?", "<p>Has cambiado los datos de acceso y no los has guardado.</p>", "Descartar", true))) return;
    }
    if (m.id === "m-secret") secretOrig = "";
    m.hidden = true;
  }
  const topModal = () => $$(".modal").filter((m) => !m.hidden).pop();
  $$(".modal").forEach((m) => {
    m.addEventListener("mousedown", (e) => { if (e.target === m) closeModal(m); });
    $$("[data-close]", m).forEach((b) => b.addEventListener("click", () => closeModal(m)));
  });
  $("#mv-form").addEventListener("submit", submitVault);
  $("#ms-form").addEventListener("submit", (e) => saveSecret(e, false));
  $("#ms-clear").addEventListener("click", () => saveSecret(null, true));
  $("#ma-steam").addEventListener("click", addViaSteam);
  $("#ma-form").addEventListener("submit", (e) => { e.preventDefault(); addManual(true); });
  $("#ma-save").addEventListener("click", () => addManual(false));
  $("#ma-name").addEventListener("input", debounce(() => { if ($("#ma-name").value) addCheck(); else clearErrs($("#m-add")); }, 250));
  $("#ma-name").addEventListener("blur", () => { if ($("#ma-name").value) addCheck(); });
  $("#ma-pass").addEventListener("input", () => fieldErr($("#ma-pass"), spaceHint($("#ma-pass").value), true));
  for (const id of ["#ms-email", "#ms-rec", "#ms-epass", "#ms-pass"]) $(id).addEventListener("input", debounce(secretCheck, 300));
  $("#ms-rec").addEventListener("blur", (e) => { e.target.value = e.target.value.trim().toUpperCase(); secretCheck(); });
  $("#mf-go").addEventListener("click", doForget);
  $("#s-save").addEventListener("click", saveSettings);
  $$("#m-settings .seg").forEach((sg) => sg.addEventListener("click", (e) => { const b = e.target.closest("button"); if (b) setSeg("#" + sg.id, b.dataset.v); }));
  $("#ns-pick").addEventListener("click", async () => {
    try {
      const d = await api.PickSteamDir();
      if (!d) return;
      S.steam = await api.SaveSettings({ ...S.settings, steamDir: d });
      S.settings.steamDir = d;
      setView(S.view);
      await loadAccounts();
    } catch (e) { toast(errText(e), "error"); }
  });
  $("#ns-retry").addEventListener("click", async () => { const d = await api.Init(); applyInit(d); });

  document.addEventListener("mousedown", (e) => {
    if (!e.target.closest("#ctx")) closeMenu();
    if (!e.target.closest("#vault-pop") && !e.target.closest("#wb-vault")) closePop();
  });
  window.addEventListener("blur", () => { closeMenu(); });
  window.addEventListener("resize", () => { closeMenu(); closePop(); });

  // teclado
  document.addEventListener("keydown", (e) => {
    const inField = e.target.matches("input, textarea, select");
    if (e.key === "Escape") {
      if (!$("#ctx").hidden) { closeMenu(); return; }
      if (!$("#vault-pop").hidden) { closePop(); return; }
      const open = topModal();
      if (open) { e.preventDefault(); closeModal(open); return; }
      if (inField) { e.target.blur(); return; }
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "f") {
      e.preventDefault();
      const i = $(S.view === "games" ? "#game-q" : "#acc-q");
      i.focus(); i.select();
      return;
    }
    if (e.ctrlKey && (e.key === "1" || e.key === "2")) { e.preventDefault(); setView(e.key === "1" ? "accounts" : "games"); return; }
    if (inField || $$(".modal").some((m) => !m.hidden) || !$("#switch").hidden) return;
    if (S.view === "accounts") {
      const list = visibleAccounts();
      const i = list.findIndex((a) => a.key === S.accSel);
      if (e.key === "Enter" && i >= 0) { e.preventDefault(); const a = list[i]; if (!a.active) switchTo(a); return; }
      const cols = Math.max(1, getComputedStyle(grid).gridTemplateColumns.split(" ").length);
      const mv = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols }[e.key];
      if (mv && list.length) { e.preventDefault(); const n = Math.max(0, Math.min(list.length - 1, (i < 0 ? 0 : i + mv))); selectAccount(list[n].key, true); }
    } else if (S.view === "games" && S.games) {
      const { list } = visibleGames();
      const i = list.findIndex((g) => g.appid === S.gameSel);
      if (e.key === "Enter" && i >= 0) { e.preventDefault(); playGame(list[i]); return; }
      const cols = Math.max(1, getComputedStyle(ggrid).gridTemplateColumns.split(" ").length);
      const mv = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols }[e.key];
      if (mv && list.length) {
        e.preventDefault();
        const n = Math.max(0, Math.min(list.length - 1, (i < 0 ? 0 : i + mv)));
        selectGame(list[n].appid);
        const el = $(`#game-grid .game[data-appid="${list[n].appid}"]`);
        if (el) el.scrollIntoView({ block: "nearest", behavior: "smooth" });
      }
    }
  });

  // eventos de Go
  rt.EventsOn("steam", (s) => {
    const prev = S.steam;
    S.steam = s;
    if (S.switching) S.steam.switching = true;
    renderPills();
    if (prev.found !== s.found) setView(S.view);
    if (prev.activeId !== s.activeId || prev.running !== s.running) loadAccountsSoon();
    if (prev.runningApp !== s.runningApp && S.games) {
      for (const g of S.games.games) g.running = !!s.runningApp && g.appid === s.runningApp;
      if (S.view === "games") renderGames();
      if (!s.runningApp) S.gamesAt = 0; // al cerrar un juego cambian las horas
      if (s.runningApp) api.GameName(s.runningApp).then((n) => { S.runningName = n; renderPills(); }).catch(() => { });
    }
  });
  rt.EventsOn("accounts", () => { loadAccountsSoon(); S.gamesAt = 0; });
  rt.EventsOn("vault", (v) => {
    const was = S.vault.unlocked;
    S.vault = v;
    if (!v.unlocked) { S.secrets.clear(); S.reveal.clear(); closePop(); }
    renderVaultBtn();
    renderAccDetail();
    if (was && !v.unlocked) toast("La bóveda se ha cerrado sola", "secret");
  });
  rt.EventsOn("switch", (p) => setStep(p.step));
  rt.EventsOn("clipCleared", () => toast("Portapapeles borrado", "secret"));

  // ---------- arranque ----------
  function applyInit(d) {
    S.version = d.version;
    S.steam = d.steam || { found: false };
    S.settings = d.settings || {};
    S.vault = d.vault || {};
    S.accounts = d.accounts || [];
    if (d.error) toast(d.error, "error");
    renderVaultBtn();
    renderPills();
    setView(S.view);
    renderAccounts();
    if (S.steam.found) ensureGames().then(() => renderAccDetail());
  }
  api.Init().then(applyInit).catch((e) => toast(errText(e), "error"));

  // ---------- datos de muestra (solo al abrir el HTML en un navegador) ----------
  function mockApi() {
    const now = Math.floor(Date.now() / 1000);
    const accounts = [
      { key: "nightowl_99", name: "nightowl_99", steamId: "76561198000000001", persona: "NightOwl", alias: "", notes: "La principal. Tiene CS2 Prime.", tags: ["principal"], pinned: true, hidden: false, inSteam: true, hasToken: true, hasSecret: true, active: true, autoLogin: true, offline: false, lastUsed: now - 600, added: now - 9e6, avatar: "", games: 14, minutes: 32140, recent: { appid: 294100, name: "RimWorld", lastPlayed: now - 3600 } },
      { key: "smurfito", name: "smurfito", steamId: "76561198000000002", persona: "Smurfito", alias: "Smurf CS", notes: "", tags: ["smurf", "cs2"], pinned: false, hidden: false, inSteam: true, hasToken: true, hasSecret: true, active: false, autoLogin: false, offline: false, lastUsed: now - 86400 * 3, added: now - 5e6, avatar: "", games: 3, minutes: 4200, recent: { appid: 730, name: "Counter-Strike 2", lastPlayed: now - 86400 * 3 } },
      { key: "familia_lopez", name: "familia_lopez", steamId: "76561198000000003", persona: "Lopez Family", alias: "", notes: "Compartida con mi hermano", tags: ["familia"], pinned: false, hidden: false, inSteam: true, hasToken: false, hasSecret: false, active: false, autoLogin: false, offline: false, lastUsed: now - 86400 * 40, added: now - 4e6, avatar: "", games: 6, minutes: 900, recent: { appid: 945360, name: "Among Us", lastPlayed: now - 86400 * 40 } },
      { key: "tradebot_eu", name: "tradebot_eu", steamId: "", persona: "", alias: "Intercambios", notes: "", tags: ["trade"], pinned: false, hidden: false, inSteam: false, hasToken: false, hasSecret: true, active: false, autoLogin: false, offline: false, lastUsed: 0, added: now - 1e5, avatar: "", games: 0, minutes: 0 },
    ];
    const games = [
      { appid: 294100, name: "RimWorld", type: "game", installed: true, dir: "C:\\Program Files (x86)\\Steam\\steamapps\\common\\RimWorld", library: "C:\\Program Files (x86)\\Steam", size: 577e6, updated: now - 86400 * 20, lastPlayed: now - 3600, minutes: 19376, owner: "76561198000000001", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 19376, lastPlayed: now - 3600 }], running: false },
      { appid: 730, name: "Counter-Strike 2", type: "game", installed: true, dir: "D:\\SteamLibrary\\steamapps\\common\\Counter-Strike Global Offensive", library: "D:\\SteamLibrary", size: 36e9, updated: now - 86400 * 2, lastPlayed: now - 86400 * 3, minutes: 9100, owner: "76561198000000002", players: [{ key: "smurfito", name: "smurfito", persona: "Smurfito", steamId: "76561198000000002", minutes: 4200, lastPlayed: now - 86400 * 3 }, { key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 4900, lastPlayed: now - 86400 * 9 }], running: false },
      { appid: 108600, name: "Project Zomboid", type: "game", installed: true, dir: "C:\\Program Files (x86)\\Steam\\steamapps\\common\\ProjectZomboid", library: "C:\\", size: 10.3e9, state: "update", updated: now - 86400 * 25, lastPlayed: now - 86400 * 15, minutes: 6200, owner: "76561198000000001", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 6200, lastPlayed: now - 86400 * 15 }], running: false },
      { appid: 945360, name: "Among Us", type: "game", installed: true, dir: "C:\\Games\\Among Us", library: "C:\\", size: 1.1e9, updated: now - 86400 * 60, lastPlayed: now - 86400 * 40, minutes: 900, owner: "76561198000000003", players: [{ key: "familia_lopez", name: "familia_lopez", persona: "Lopez Family", steamId: "76561198000000003", minutes: 900, lastPlayed: now - 86400 * 40 }], running: false },
      { appid: 239820, name: "Game Dev Tycoon", type: "game", installed: true, state: "missing", dir: "C:\\Games\\GDT", library: "C:\\", size: 168e6, updated: now - 86400 * 200, lastPlayed: 0, minutes: 0, owner: "76561198000000001", players: [], running: false },
      { appid: 1091500, name: "Cyberpunk 2077", type: "game", installed: false, dir: "", library: "", size: 0, updated: 0, lastPlayed: now - 86400 * 120, minutes: 5400, owner: "", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 5400, lastPlayed: now - 86400 * 120 }], running: false },
      { appid: 4900640, name: "Shreddead Demo", type: "demo", installed: true, dir: "C:\\Games\\Shreddead", library: "C:\\", size: 420e6, updated: now - 86400 * 5, lastPlayed: now - 86400 * 6, minutes: 75, owner: "76561198000000001", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 75, lastPlayed: now - 86400 * 6 }], running: false },
    ];
    let vault = { exists: true, unlocked: false, lockMinutes: 5, locksIn: 0 };
    const secrets = { nightowl_99: { password: "correct-horse-battery", email: "night@example.com", recovery: "R12345" }, smurfito: { password: "smurf-pass-123" }, tradebot_eu: { password: "trade$$2026" } };
    const wait = (ms) => new Promise((r) => setTimeout(r, ms));
    const status = { found: true, dir: "C:\\Program Files (x86)\\Steam", running: true, activeId: "76561198000000001", runningApp: 0, autoLogin: "nightowl_99", switching: false };
    const emit = (n, d) => (window.__mockHandlers[n] || []).forEach((f) => f(d));
    window.__mockHandlers = {};
    rt.EventsOn = (n, f) => { (window.__mockHandlers[n] = window.__mockHandlers[n] || []).push(f); };
    return {
      async Init() { return { version: "1.0 (muestra)", steam: status, settings: { lockMinutes: 5, clipSeconds: 20, afterSwitch: "nada", confirmSwitch: true }, vault, accounts }; },
      async Accounts() { return accounts.map((a) => ({ ...a, active: a.steamId && a.steamId === status.activeId })); },
      async Status() { return status; },
      async Games() { await wait(300); return { games, libraries: [] }; },
      async GameName() { return ""; },
      async UpdateAccount(name, p) { const a = accounts.find((x) => x.name === name); Object.assign(a, p); },
      async AddManualAccount(name) { const old = accounts.find((x) => x.key === name.toLowerCase()); if (old) { if (!old.hidden) throw "Esa cuenta ya está en la lista"; old.hidden = false; return { restored: true }; } accounts.push({ key: name.toLowerCase(), name, steamId: "", persona: "", alias: "", notes: "", tags: [], inSteam: false, lastUsed: 0, games: 0, minutes: 0 }); return {}; },
      async ForgetAccount(name) { const i = accounts.findIndex((x) => x.name === name); accounts.splice(i, 1); },
      async SwitchTo(r) {
        for (const s of ["close", "prepare", "open", "login"]) { emit("switch", { step: s }); await wait(700); }
        const a = accounts.find((x) => x.name === r.name);
        if (!a.hasToken) { emit("switch", { step: "password" }); await wait(4000); }
        status.activeId = a.steamId || "x"; a.hasToken = true; a.lastUsed = Math.floor(Date.now() / 1000);
        return { ok: true };
      },
      async CancelWait() { },
      async AddAccount() { await wait(800); },
      async PlayGame() { return { ok: true, already: true }; },
      async CopyField(name, f) { if (f === "name") return 0; if (!vault.unlocked) throw "La bóveda está cerrada"; return 20; },
      async CopyText() { },
      async VaultState() { return vault; },
      async VaultCreate(m) { await wait(400); vault = { ...vault, exists: true, unlocked: true, locksIn: 300 }; return vault; },
      async VaultUnlock(m) { await wait(400); if (m !== "maestra123") throw "Contraseña maestra incorrecta"; vault = { ...vault, unlocked: true, locksIn: 300 }; return vault; },
      async VaultLock() { vault = { ...vault, unlocked: false }; return vault; },
      async VaultTouch() { return vault; },
      async VaultGet(name) { if (!vault.unlocked) throw "La bóveda está cerrada"; return secrets[name.toLowerCase()] || {}; },
      async VaultSet(name, e) { secrets[name.toLowerCase()] = e; const a = accounts.find((x) => x.name === name); if (a) a.hasSecret = !!(e.password || e.email); },
      async VaultChangeMaster() { return vault; },
      async VaultDestroy() { vault = { exists: false, unlocked: false }; return vault; },
      async SaveSettings() { return status; },
      async PickSteamDir() { return ""; },
      async OpenFolder() { }, async SteamURL() { }, async OpenSteam() { },
    };
  }
})();
