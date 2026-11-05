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
    minus: '<circle cx="12" cy="12" r="10"/><path d="M8 12h8"/>',
    puzzle: '<path d="M19.44 13.11a2 2 0 0 0 0-2.22l-.88-1.32a2 2 0 0 1 .25-2.52l.36-.36a2 2 0 0 0 0-2.83l-.06-.06a2 2 0 0 0-2.83 0l-.36.36a2 2 0 0 1-2.52.25l-1.32-.88a2 2 0 0 0-2.22 0l-1.32.88a2 2 0 0 1-2.52-.25l-.36-.36a2 2 0 0 0-2.83 0"/><path d="M15 9h.01M9 15h.01"/><rect x="3" y="9" width="12" height="12" rx="2"/>',
    share: '<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><path d="m8.59 13.51 6.83 3.98M15.41 6.51l-6.82 3.98"/>',
    paste: '<rect x="8" y="2" width="8" height="4" rx="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/>',
    file: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>',
    layers: '<path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5"/><path d="m2 12 10 5 10-5"/>',
    save: '<path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><path d="M17 21v-8H7v8M7 3v5h8"/>',
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
    gameFilter: store.get("gameFilter", "all"), gameAcc: "", gameSort: store.get("gameSort", "recent"), gameQ: "", gameTag: store.get("gameTag", ""),
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
    $("#v-mods").classList.toggle("on", found && v === "mods");
    if (found && v === "games") ensureGames();
    if (found && v === "mods") ensureMods();
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
    }).join("") + `<button class="acc add-card" data-act="add"><div class="tile">${ic("plus")}</div><b>Añadir cuenta</b><span>Guárdala con su usuario y, si quieres, su contraseña</span></button>`;
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
    // los que ha jugado y, detrás, los de su biblioteca que aún no ha jugado aquí
    const mine = S.games.games.map((g) => ({ g, p: g.players.find((p) => p.steamId === a.steamId) || ((g.inLibrary || []).includes(a.steamId) ? { minutes: 0, lastPlayed: 0, unplayed: true } : null) })).filter((x) => x.p)
      .sort((x, y) => (y.p.lastPlayed - x.p.lastPlayed) || (y.p.minutes - x.p.minutes) || x.g.name.localeCompare(y.g.name, "es"));
    const top = mine.slice(0, 6);
    box.innerHTML = `<div class="sec-h"><span class="label">Juegos</span><span class="mini">${mine.length}${a.minutes ? " · " + fmtHours(a.minutes) : ""}</span></div>
      <div class="glist">${top.map(({ g, p }) => `<div class="gitem" data-game="${g.appid}">${thumbHTML(g)}<div style="min-width:0"><div class="gn">${esc(g.name)}</div><div class="gs">${[fmtHours(p.minutes), ago(p.lastPlayed)].filter(Boolean).join(" · ") || (p.unplayed ? "Sin jugar" : "—")}</div></div><button class="ibtn" data-play="${g.appid}" title="Jugar con esta cuenta">${ic("play")}</button></div>`).join("")}</div>
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

  // cuentas que tienen un juego en su biblioteca sin haberlo jugado en este PC
  // (recién comprado, prestado por la familia…)
  const inLib = (g, sid) => !!sid && (g.inLibrary || []).includes(sid);
  const hasPlayed = (g, sid) => !!sid && g.players.some((p) => p.steamId === sid);
  // si una cuenta tiene que ver con el juego: lo ha jugado, lo tiene o lo instaló
  const hasGame = (g, sid) => hasPlayed(g, sid) || inLib(g, sid) || (!!sid && g.owner === sid);

  function visibleGames() {
    const q = norm(S.gameQ.trim());
    const acc = S.gameAcc && accByKey(S.gameAcc);
    const minutesOf = (g) => acc ? ((g.players.find((p) => p.steamId === acc.steamId) || {}).minutes || 0) : g.minutes;
    const lastOf = (g) => acc ? ((g.players.find((p) => p.steamId === acc.steamId) || {}).lastPlayed || 0) : g.lastPlayed;
    const m2wOf = (g) => acc ? ((g.players.find((p) => p.steamId === acc.steamId) || {}).minutes2w || 0) : (g.minutes2w || 0);
    const tg = S.gameTag;
    // con #algo se busca en las etiquetas y colecciones
    const tagHay = (g) => norm([...(g.tags || []), ...(g.collections || [])].join(" "));
    let list = S.games.games.filter((g) => {
      if (S.gameFilter === "installed" && !g.installed) return false;
      if (acc && !hasGame(g, acc.steamId)) return false;
      if (tg === "2w" && !(m2wOf(g) > 0)) return false;
      if (tg === "mods" && !g.mods) return false;
      if (tg.startsWith("t:") && !(g.tags || []).some((t) => t.toLowerCase() === tg.slice(2).toLowerCase())) return false;
      if (tg.startsWith("c:") && !(g.collections || []).includes(tg.slice(2))) return false;
      if (q && !q.split(/\s+/).every((w) => w.startsWith("#") ? tagHay(g).includes(w.slice(1)) : (norm(g.name).includes(w) || String(g.appid) === w || tagHay(g).includes(w)))) return false;
      return true;
    });
    const by = {
      recent: (x, y) => (lastOf(y) - lastOf(x)) || x.name.localeCompare(y.name, "es"),
      name: (x, y) => x.name.localeCompare(y.name, "es", { sensitivity: "base" }),
      hours: (x, y) => (minutesOf(y) - minutesOf(x)) || x.name.localeCompare(y.name, "es"),
      "2w": (x, y) => (m2wOf(y) - m2wOf(x)) || (lastOf(y) - lastOf(x)) || x.name.localeCompare(y.name, "es"),
      size: (x, y) => (y.size - x.size) || x.name.localeCompare(y.name, "es"),
    }[S.gameSort] || ((x, y) => 0);
    if (tg === "top") {
      // los más jugados: los 20 con más horas (de la cuenta elegida, si hay)
      list = list.filter((g) => minutesOf(g) > 0).sort((x, y) => (minutesOf(y) - minutesOf(x)) || x.name.localeCompare(y.name, "es")).slice(0, 20);
    } else list.sort((x, y) => (y.running - x.running) || by(x, y));
    return { list, minutesOf, lastOf, m2wOf };
  }

  function renderGameAccSelect() {
    const sel = $("#game-acc");
    const withGames = S.accounts.filter((a) => a.steamId && (a.games || S.games && S.games.games.some((g) => hasGame(g, a.steamId))));
    if (S.gameAcc && !withGames.some((a) => a.key === S.gameAcc)) S.gameAcc = "";
    sel.innerHTML = `<option value="">Todas las cuentas</option>` + withGames.map((a) => `<option value="${esc(a.key)}"${a.key === S.gameAcc ? " selected" : ""}>${esc(display(a))}</option>`).join("");
    sel.classList.toggle("on", !!S.gameAcc);
    sel.hidden = withGames.length < 2;
  }

  function renderGameTagSelect() {
    const sel = $("#game-tag");
    const gs = S.games.games;
    const tags = [...new Set(gs.flatMap((g) => g.tags || []))].sort((a, b) => a.localeCompare(b, "es"));
    const cols = [...new Set(gs.flatMap((g) => g.collections || []))].sort((a, b) => a.localeCompare(b, "es"));
    const anyMods = gs.some((g) => g.mods);
    const ok = new Set(["", "top", "2w", ...(anyMods ? ["mods"] : []), ...tags.map((t) => "t:" + t), ...cols.map((c) => "c:" + c)]);
    if (!ok.has(S.gameTag)) S.gameTag = "";
    const o = (v, l) => `<option value="${esc(v)}"${v === S.gameTag ? " selected" : ""}>${esc(l)}</option>`;
    sel.innerHTML = o("", "Etiquetas y colecciones") + o("top", "★ Más jugados") + o("2w", "Jugados estas 2 semanas") + (anyMods ? o("mods", "Con mods") : "") +
      (tags.length ? `<optgroup label="Tus etiquetas">${tags.map((t) => o("t:" + t, "#" + t)).join("")}</optgroup>` : "") +
      (cols.length ? `<optgroup label="Colecciones de Steam">${cols.map((c) => o("c:" + c, c)).join("")}</optgroup>` : "");
    sel.classList.toggle("on", !!S.gameTag);
  }

  function renderGames() {
    if (!S.games) return;
    renderGameAccSelect();
    renderGameTagSelect();
    $$("#game-filter button").forEach((b) => b.classList.toggle("on", b.dataset.v === S.gameFilter));
    $("#game-sort").value = S.gameSort;
    const { list, minutesOf, lastOf, m2wOf } = visibleGames();
    const inst = S.games.games.filter((g) => g.installed);
    const size = inst.reduce((s, g) => s + (g.size || 0), 0);
    $("#game-stat").innerHTML = `<b>${list.length}</b> de ${S.games.games.length} · ${inst.length} instalados · ${fmtBytes(size)}`;
    const grid = $("#game-grid");
    if (!S.games.games.length) {
      grid.innerHTML = `<div class="empty">${ic("game")}<b>No hay juegos todavía</b><p>Aquí salen los juegos instalados y los que tiene o ha jugado cada cuenta en este PC.</p></div>`;
      renderGameDetail();
      return;
    }
    if (!list.length) {
      grid.innerHTML = `<div class="empty">${ic("search")}<b>Ningún juego coincide</b><p>Prueba con otro nombre o quita los filtros.</p></div>`;
      renderGameDetail();
      return;
    }
    if (!S.gameSel || !list.some((g) => g.appid === S.gameSel)) S.gameSel = list[0].appid;
    // las tarjetas se reutilizan: al buscar o filtrar solo se recolocan, y la
    // portada que ya estaba cargada no vuelve a parpadear
    let fresh = 0;
    grid.replaceChildren(...list.map((g) => {
      let el = gameEls.get(g.appid);
      if (!el) { el = gameCard(g, fresh++); gameEls.set(g.appid, el); }
      const m = minutesOf(g), l = lastOf(g);
      const meta = S.gameSort === "2w" && m2wOf(g) ? fmtHours(m2wOf(g)) + " en 2 semanas" : [fmtHours(m), l ? ago(l) : (g.installed || (g.inLibrary || []).length ? "Sin jugar" : "")].filter(Boolean).join(" · ") || (g.installed ? fmtBytes(g.size) : "—");
      el.classList.toggle("sel", g.appid === S.gameSel);
      el.classList.toggle("notinst", !g.installed);
      setHTML(el.querySelector(".badges"), (g.running ? `<span class="run">En marcha</span>` : "") + (g.type === "demo" ? "<span>Demo</span>" : "") + (STATE[g.state] ? `<span class="st-${g.state}">${STATE[g.state].short}</span>` : ""));
      el.querySelector(".playb").title = g.installed ? "Jugar" : "Instalar o jugar";
      const gn = el.querySelector(".gname");
      if (gn.textContent !== g.name) { gn.textContent = g.name; gn.title = g.name; el.querySelector(".ph span").textContent = g.name; }
      setHTML(el.querySelector(".gmeta"), (g.installed ? "<i></i>" : "") + `<span>${esc(meta)}</span>`);
      return el;
    }));
    renderGameDetail();
  }

  const gameEls = new Map();
  function gameCard(g, n) {
    const el = document.createElement("div");
    el.className = "game";
    el.dataset.appid = g.appid;
    el.style.animationDelay = Math.min(n, 24) * 0.015 + "s";
    el.innerHTML = `<div class="cover"><div class="ph"><span></span></div>
      <img loading="lazy" decoding="async" alt="" draggable="false">
      <div class="shade"></div><div class="badges"></div>
      <button class="playb" data-play="${g.appid}">${ic("play")}</button></div>
      <div class="gname"></div><div class="gmeta"></div>`;
    // la animación de entrada solo la primera vez; si no, se repetiría al recolocarla
    el.addEventListener("animationend", () => el.classList.add("seen"), { once: true });
    setTimeout(() => el.classList.add("seen"), 1500); // por si la ventana estaba oculta y no llegó a animarse
    const cover = el.firstElementChild, img = cover.querySelector("img");
    img.onload = () => cover.classList.add("loaded");
    img.onerror = () => { img.remove(); cover.classList.add("noimg"); };
    img.src = `/img/app/${g.appid}/cover`;
    return el;
  }
  // setHTML solo toca el DOM si cambia algo
  function setHTML(el, html) { if (el.innerHTML !== html) el.innerHTML = html; }

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

  // a qué cuenta jugar por defecto: la que está dentro si lo ha jugado, lo
  // tiene o lo instaló; si no, la que más recientemente lo jugó, una que lo
  // tenga o la que lo instaló
  function bestAccountFor(g) {
    const active = activeAcc();
    const known = (sid) => S.accounts.find((a) => a.steamId === sid && !a.hidden) || S.accounts.find((a) => a.steamId === sid);
    if (active && hasGame(g, active.steamId)) return active;
    for (const p of g.players) { const a = known(p.steamId); if (a) return a; }
    for (const sid of g.inLibrary || []) { const a = known(sid); if (a) return a; }
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
      switchTo(a, { appid: g.appid, forceAsk: true, note: `<b>${esc(g.name)}</b> no está instalado: al entrar con la cuenta que ${hasPlayed(g, a.steamId) ? "lo ha jugado" : "lo tiene"}, Steam te ofrecerá instalarlo.` });
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
    if (!chosen && active && !hasPlayed(g, active.steamId)) {
      note = `<b>${esc(display(active))}</b>, la cuenta abierta ahora, no lo ha jugado en este PC; <b>${esc(display(a))}</b> ${hasPlayed(g, a.steamId) ? "sí" : "lo tiene en su biblioteca"}.`;
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
    const row = (sid, persona, sub, hours) => {
      const a = S.accounts.find((x) => x.steamId === sid);
      const av = a ? avatarHTML(a, "sm", false) : `<div class="av sm" style="--avc:#475569"><span>${esc(initials(persona || "?"))}</span></div>`;
      const name = a ? display(a) : (persona || "Cuenta sin guardar");
      return `<div class="pitem">${av}<div style="min-width:0"><div class="pn">${esc(name)}${a && a.active ? ' <span class="chip live" style="margin-left:4px">En uso</span>' : ""}</div><div class="ps">${esc(sub)}</div></div><span class="ph">${esc(hours || "—")}</span>${a ? `<button class="btn ghost xs" data-play-as="${esc(a.key)}" title="${a.active ? "Jugar" : "Cambiar a esta cuenta y jugar"}">${ic("play")}</button>` : "<span></span>"}</div>`;
    };
    const lib = g.inLibrary || [];
    const players = g.players.map((p) => row(p.steamId, p.persona, ago(p.lastPlayed) || "—", fmtHours(p.minutes)))
      .concat(lib.map((sid) => row(sid, "", "En su biblioteca · sin jugar", ""))).join("");
    box.innerHTML = `
      <div class="g-hero" style="background-image:url('/img/app/${g.appid}/hero'),url('/img/app/${g.appid}/header')"><div class="logo" style="background-image:url('/img/app/${g.appid}/logo')"></div></div>
      <div class="g-head">
        <h2>${esc(g.name)}</h2>
        <div class="d-chips">${g.running ? '<span class="chip live">En marcha</span>' : ""}${g.installed ? (STATE[g.state] ? `<span class="chip ${STATE[g.state].cls}" title="${esc(STATE[g.state].tip)}">${ic("alert")}${STATE[g.state].long}</span>` : `<span class="chip token">${ic("check")}Instalado</span>`) : `<span class="chip off">No instalado</span>`}${g.type === "demo" ? '<span class="chip">Demo</span>' : ""}</div>
        <div class="g-play"><button class="btn go sm" data-act="play">${ic("play")}${playLabel}</button>${S.accounts.filter((a) => a.steamId).length > 1 ? `<button class="ibtn" data-act="play-menu" title="Jugar con otra cuenta">${ic("users")}</button>` : ""}<button class="ibtn" data-act="game-menu" title="Más opciones">${ic("more")}</button></div>
      </div>
      <div class="d-sec">
        <div class="facts">
          <div class="fact"><b>${g.minutes ? fmtHours(g.minutes) : "—"}</b><span>${g.minutes2w ? esc(fmtHours(g.minutes2w)) + " estas 2 semanas" : "jugado en este PC"}</span></div>
          <div class="fact"><b>${g.lastPlayed ? esc(ago(g.lastPlayed)) : "—"}</b><span>última partida</span></div>
          <div class="fact"><b>${g.installed ? fmtBytes(g.size) : "—"}</b><span>en disco</span></div>
          <div class="fact"><b>${g.updated ? fmtDate(g.updated) : "—"}</b><span>actualizado</span></div>
        </div>
        ${g.installed && g.dir ? `<div class="path"><span title="${esc(g.dir)}">&lrm;${esc(g.dir)}&lrm;</span><button class="ibtn" data-act="folder" title="Abrir la carpeta">${ic("folder")}</button></div>` : ""}
      </div>
      <div class="d-sec">
        <div class="sec-h"><span class="label">Etiquetas</span><span class="saved" id="g-saved">${ic("check")} Guardado</span></div>
        <div class="tags" style="margin-bottom:${(g.collections || []).length ? 8 : 0}px">${(g.tags || []).map((t) => `<span class="chip tag">#${esc(t)}<button data-guntag="${esc(t)}" title="Quitar">${ic("x")}</button></span>`).join("")}<input class="tag-add" id="g-tag" maxlength="24" placeholder="+ etiqueta" spellcheck="false" title="Escribe y pulsa Intro: coop, pendiente, con-amigos…"><span class="tag-err" id="g-tag-err"></span></div>
        ${(g.collections || []).length ? `<div class="g-tags">${g.collections.map((c) => `<span class="chip coll" data-gcoll="${esc(c)}" title="Colección de tu biblioteca de Steam: ver sus juegos">${ic("layers")}${esc(c)}</span>`).join("")}</div>` : ""}
      </div>
      <div class="d-sec">
        <div class="sec-h"><span class="label">${lib.length ? "Cuentas con este juego" : "Quién lo ha jugado"}</span><span class="mini">${g.players.length + lib.length || ""}</span></div>
        ${players ? `<div class="plist">${players}</div>` : `<div class="note">Ninguna cuenta lo ha jugado todavía en este PC.</div>`}
      </div>
      <div class="dfoot">
        <button class="btn ghost xs" data-act="steam-lib">${ic("open")}Ver en Steam</button>
        <button class="btn ghost xs" data-act="store">${ic("store")}Tienda</button>
        ${g.workshop || g.mods ? `<button class="btn ghost xs" data-act="mods">${ic("puzzle")}Mods${g.mods ? " · " + g.mods : ""}</button>` : ""}
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
    // primero las que lo han jugado, luego las que lo tienen
    const rank = (a) => hasPlayed(g, a.steamId) ? 2 : inLib(g, a.steamId) ? 1 : 0;
    accs.sort((p, q) => (rank(q) - rank(p)) || (q.lastUsed - p.lastUsed));
    showMenu([{ head: "Jugar con…" }, ...accs.map((a) => ({
      html: `${avatarHTML(a, "sm", false)}<span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis">${esc(display(a))}</span>${a.active ? '<span class="chip live">En uso</span>' : hasPlayed(g, a.steamId) ? `<span class="chip">${esc(fmtHours((g.players.find((p) => p.steamId === a.steamId) || {}).minutes) || "jugado")}</span>` : inLib(g, a.steamId) ? '<span class="chip">Lo tiene</span>' : ""}`,
      fn: () => playGame(g, a),
    }))], x, y);
  }

  // ---------- mods del Workshop ----------
  const MS = {
    home: true, // portada con los juegos
    app: store.get("modApp", 0), tab: store.get("modTab", "subs"), q: "",
    list: null, loading: false, loadTok: 0,
    sel: null, selMod: null, // sel: { kind: "mod" | "preset", id }
    checked: new Set(), pchecked: new Set(),
    presets: null, info: new Map(), pmods: new Map(),
    search: { q: "", sort: store.get("modSort", "trend"), days: store.get("modDays", 7), userSort: false, tags: [], exclude: [], hideMine: false, mods: [], cursor: "", total: 0, loading: false, done: false, err: "", app: 0 },
    f: { state: "all", wtags: [], mytags: [], size: "", upd: "", rating: "", sort: store.get("modListSort", "name") }, // filtros de la lista
    h: { filter: store.get("modHomeFilter", "mods"), tags: [], sort: store.get("modHomeSort", "recent") }, // filtros de la portada
    p: { kind: "", from: "", tags: [], sort: store.get("modPresetSort", "recent") }, // filtros de los presets
    fopen: store.get("modFiltersOpen", false), allTags: new Set(), // panel de filtros abierto, grupos con todas sus etiquetas
    connect: new Set(), busy: false,
    view: store.get("modView", "list"), // list, details o grid
    picked: new Map(), pickOpen: false, pickQ: "", // lo elegido con las casillas (se guarda aunque cambie la búsqueda)
  };

  // juegos de la pestaña: los que tienen Workshop o mods en este PC, y los de
  // presets que te han pasado aunque no tengas el juego
  function modGames() {
    const list = S.games ? S.games.games.filter((g) => g.workshop || g.mods) : [];
    for (const p of MS.presets || []) {
      if (!list.some((g) => g.appid === p.appid)) list.push({ appid: p.appid, name: p.game || ("Juego " + p.appid), mods: 0, workshop: true, players: [], tags: [], collections: [], notOwned: true });
    }
    return list.sort((x, y) => ((y.mods || 0) - (x.mods || 0)) || ((y.lastPlayed || 0) - (x.lastPlayed || 0)) || x.name.localeCompare(y.name, "es"));
  }
  const modGame = () => modGames().find((g) => g.appid === MS.app);
  const wantLive = (app) => !S.settings.workshopManual || MS.connect.has(app);
  const modById = (id) => (MS.list && (MS.list.mods.find((m) => m.id === id) || (MS.list.history || []).find((m) => m.id === id))) || MS.search.mods.find((m) => m.id === id) || (MS.selMod && MS.selMod.id === id ? MS.selMod : null) || MS.pmods.get(id) || MS.picked.get(id) || null;
  // elegir o soltar un mod con su casilla (se guarda una copia por si deja de verse)
  function pick(id, on) {
    if (on) { MS.checked.add(id); const m = modById(id); if (m) MS.picked.set(id, m); }
    else { MS.checked.delete(id); MS.picked.delete(id); }
  }
  function clearPicks() { MS.checked.clear(); MS.picked.clear(); MS.pickOpen = false; }
  const subscribedIds = () => MS.list ? MS.list.mods.filter((m) => m.subscribed).map((m) => m.id) : [];
  const presetsOf = (app) => (MS.presets || []).filter((p) => p.appid === app);
  const gamePresets = () => presetsOf(MS.app);
  const presetById = (id) => (MS.presets || []).find((p) => p.id === id);
  const plural = (n, one, many) => n + " " + (n === 1 ? one : many);
  const spinnerBox = (t) => `<div class="empty"><div class="spinner"></div><b>${esc(t)}</b></div>`;
  const fail = (e) => toast(errText(e), "error");
  const notOwnedErr = (msg) => /no tiene este juego/i.test(String(msg || ""));

  // cuentas tuyas (que no son la abierta) que tienen el juego
  function ownersOf(appid) {
    const g = S.games && S.games.games.find((x) => x.appid === appid);
    if (!g) return [];
    const act = activeAcc();
    return S.accounts.filter((a) => a.steamId && !a.hidden && (!act || a.key !== act.key) && hasGame(g, a.steamId))
      .sort((x, y) => (hasPlayed(g, y.steamId) - hasPlayed(g, x.steamId)) || ((y.lastUsed || 0) - (x.lastUsed || 0)));
  }

  async function ensureMods() {
    await ensureGames();
    if (!MS.presets) await loadPresets(true);
    if (MS.home) { renderMods(); return; }
    const games = modGames();
    if (!games.some((g) => g.appid === MS.app)) { MS.home = true; renderMods(); return; }
    if (!MS.list || MS.list.appid !== MS.app) loadMods();
    else renderMods();
  }
  async function loadPresets(quiet) {
    try { MS.presets = (await api.Presets()) || []; } catch { MS.presets = MS.presets || []; }
    if (!quiet) renderMods();
  }
  async function loadMods(opts = {}) {
    const app = MS.app;
    if (!app) return;
    const tok = ++MS.loadTok;
    MS.loading = true;
    if (!MS.list || MS.list.appid !== app) { MS.list = null; clearPicks(); }
    renderMods();
    const g = modGame();
    if (g && g.notOwned) {
      MS.list = { appid: app, mods: [], history: [], live: false, liveError: "", account: "", diskSize: 0, notOwned: true, noGame: true };
      MS.loading = false;
      renderMods();
      return;
    }
    const live = opts.live !== undefined ? opts.live : wantLive(app);
    try {
      // lo que hay en el PC sale al momento; preguntar a Steam tarda un poco más
      if (live && (!MS.list || MS.list.appid !== app)) {
        const quick = await api.Mods(app, false).catch(() => null);
        if (tok !== MS.loadTok) return;
        if (quick) { MS.list = { ...quick, liveError: "", connecting: true }; renderMods(); }
      }
      const l = await api.Mods(app, live);
      if (tok !== MS.loadTok) return;
      MS.list = l;
    } catch (e) {
      if (tok === MS.loadTok) MS.list = { appid: app, mods: [], history: [], live: false, liveError: errText(e), account: "", diskSize: 0 };
    } finally {
      if (tok === MS.loadTok) MS.loading = false;
    }
    if (tok === MS.loadTok) renderMods();
  }

  // entra en los mods de un juego (desde la portada o desde Juegos)
  function openModGame(appid, tab) {
    const changed = MS.app !== appid;
    MS.home = false;
    MS.app = appid;
    store.set("modApp", appid);
    if (changed) {
      MS.sel = null; MS.selMod = null; clearPicks(); MS.pchecked.clear(); MS.list = null;
      MS.f = { ...MS.f, state: "all", wtags: [], mytags: [], size: "", upd: "", rating: "" };
      MS.p = { ...MS.p, kind: "", from: "", tags: [] };
      MS.search = { ...MS.search, tags: [], exclude: [], hideMine: false, mods: [], cursor: "", total: 0, done: false, err: "", app: 0 };
    }
    clearModQ();
    if (tab) { MS.tab = tab; store.set("modTab", tab); }
    if (S.view !== "mods") { setView("mods"); return; }
    ensureMods();
    if (MS.tab === "search" && MS.search.app !== MS.app) runSearch();
  }
  function goModHome() {
    MS.home = true;
    MS.sel = null;
    clearModQ();
    renderMods();
  }
  function clearModQ() {
    MS.q = "";
    const inp = $("#mod-q");
    inp.value = ""; inp.parentElement.classList.remove("has");
  }

  function setModTab(t) {
    MS.tab = t;
    store.set("modTab", t);
    clearModQ();
    if (t === "presets" && MS.sel && MS.sel.kind !== "preset") MS.sel = null;
    if (t !== "presets" && MS.sel && MS.sel.kind === "preset") MS.sel = null;
    renderMods();
    if (t === "search" && MS.search.app !== MS.app && !MS.search.loading) runSearch();
  }

  function renderMods() {
    if (S.view !== "mods") return;
    renderModTop();
    renderModBar();
    renderModList();
    renderModDetail();
  }

  // ---------- barra de arriba ----------
  function renderModTop() {
    const home = MS.home;
    $("#mod-back").hidden = home;
    $("#mod-game").hidden = home;
    $("#mod-tab").hidden = home;
    $("#mod-home-title").hidden = !home;
    const games = modGames();
    if (!home) {
      $("#mod-game").innerHTML = games.map((g) => `<option value="${g.appid}"${g.appid === MS.app ? " selected" : ""}>${esc(g.name)}${g.mods ? " · " + g.mods : ""}${g.notOwned ? " (no lo tienes)" : ""}</option>`).join("");
      $$("#mod-tab button").forEach((b) => b.classList.toggle("on", b.dataset.v === MS.tab));
      $("#mod-n-subs").textContent = MS.list && !MS.list.noGame ? MS.list.mods.filter((m) => m.subscribed).length : "";
      $("#mod-n-presets").textContent = gamePresets().filter((p) => !p.auto).length || "";
    }
    $("#mod-q").placeholder = home ? "Buscar entre tus juegos" : MS.tab === "search" ? "Buscar en el Workshop y pulsar Intro" : MS.tab === "presets" ? "Buscar presets: nombre, quién lo pasó, notas…" : "Buscar por nombre, autor o etiqueta (#etiqueta)";
    // estado de Steam, corto: la cuenta ya sale arriba, en la barra de título
    const lv = $("#mod-live"), l = MS.list;
    let h = "";
    if (!home && MS.app && l && !l.noGame) {
      if (l.connecting) h = `<span class="livep" title="Conectando con Steam…"><span class="spinner xs"></span>Conectando…</span>`;
      else if (l.live) {
        const a = S.accounts.find((x) => x.steamId === l.account);
        h = `<span class="livep" title="Conectado con Steam${a ? " como " + esc(display(a)) : ""}: los cambios se hacen con esa cuenta. Mientras está conectado, tus amigos te ven «jugando» a este juego.">${MS.loading ? '<span class="spinner xs"></span>' : '<i class="dot ok"></i>'}Conectado</span>`;
      } else if (l.liveError) {
        const own = notOwnedErr(l.liveError) ? ownersOf(MS.app)[0] : null;
        h = own
          ? `<span class="livep warn" data-mact="switch-owner" title="La cuenta abierta en Steam no tiene este juego; ${esc(display(own))} sí. Pulsa para cambiar.">${ic("login")}Cambiar de cuenta</span>`
          : `<span class="livep warn" data-mact="connect" title="${esc(l.liveError)}. Pulsa para volver a intentarlo.">${ic("alert")}Sin conexión</span>`;
      } else h = `<span class="livep off" data-mact="connect" title="Conectar con Steam para ver todas tus suscripciones (aunque el juego no esté instalado) y poder cambiarlas">${ic("plug")}Conectar</span>`;
    }
    lv.innerHTML = h;
  }

  // ---------- portada: los juegos con mods ----------
  function homeGames() {
    const q = norm(MS.q.trim());
    const h = MS.h;
    let list = modGames().filter((g) => {
      if (h.filter === "mods" && !g.mods && !presetsOf(g.appid).length) return false;
      if (h.filter === "presets" && !presetsOf(g.appid).length) return false;
      if (h.filter === "installed" && !g.installed) return false;
      for (const t of h.tags) {
        if (t.startsWith("t:") && !(g.tags || []).some((x) => x.toLowerCase() === t.slice(2).toLowerCase())) return false;
        if (t.startsWith("c:") && !(g.collections || []).includes(t.slice(2))) return false;
      }
      const hay = norm([g.name, ...(g.tags || []), ...(g.collections || []), g.appid].join(" "));
      return !q || q.split(/\s+/).every((w) => hay.includes(w.replace(/^#/, "")));
    });
    const by = {
      recent: (x, y) => ((y.lastPlayed || 0) - (x.lastPlayed || 0)),
      mods: (x, y) => ((y.mods || 0) - (x.mods || 0)),
      size: (x, y) => ((y.modsSize || 0) - (x.modsSize || 0)),
      hours: (x, y) => ((y.minutes || 0) - (x.minutes || 0)),
      name: () => 0,
    }[h.sort] || (() => 0);
    return list.sort((x, y) => by(x, y) || x.name.localeCompare(y.name, "es", { sensitivity: "base" }));
  }

  function renderHomeList(box) {
    if (!S.games) { box.innerHTML = spinnerBox("Leyendo tu biblioteca…"); return; }
    const list = homeGames();
    if (!modGames().length) {
      box.innerHTML = `<div class="empty">${ic("puzzle")}<b>Ningún juego con Workshop</b><p>Aquí salen los juegos de tu biblioteca que tienen mods en el Workshop de Steam. También puedes pegar un preset que te hayan pasado.</p><div class="row-btns"><button class="btn primary sm" data-mact="paste">${ic("paste")}Pegar preset</button></div></div>`;
      return;
    }
    if (!list.length) { box.innerHTML = `<div class="empty">${ic("search")}<b>Ningún juego coincide</b><p>Prueba con otro nombre o quita los filtros (arriba, «Con Workshop» los enseña todos).</p></div>`; return; }
    box.innerHTML = `<div class="game-grid">${list.map((g) => {
      const np = presetsOf(g.appid).filter((p) => !p.auto).length;
      const badges = (g.mods ? `<span>${plural(g.mods, "mod", "mods")}</span>` : "") + (np ? `<span>${plural(np, "preset", "presets")}</span>` : "") + (g.notOwned ? `<span class="st-update">No lo tienes</span>` : "");
      const meta = [g.modsSize ? fmtBytes(g.modsSize) + " en mods" : "", !g.mods && !np ? "Sin mods" : "", g.lastPlayed ? ago(g.lastPlayed) : ""].filter(Boolean).join(" · ");
      return `<div class="game seen${g.installed || g.notOwned ? "" : " notinst"}" data-mgame="${g.appid}" title="Ver sus mods">
        <div class="cover"><div class="ph"><span>${esc(g.name)}</span></div><img loading="lazy" decoding="async" alt="" draggable="false" src="/img/app/${g.appid}/cover" onload="this.parentElement.classList.add('loaded')" onerror="this.parentElement.classList.add('noimg');this.remove()"><div class="shade"></div><div class="badges">${badges}</div></div>
        <div class="gname" title="${esc(g.name)}">${esc(g.name)}</div><div class="gmeta">${g.installed ? "<i></i>" : ""}<span>${esc(meta || "—")}</span></div></div>`;
    }).join("")}</div>`;
  }
  function renderHomeDetail(box) {
    const all = modGames();
    const mods = all.reduce((s, g) => s + (g.mods || 0), 0), size = all.reduce((s, g) => s + (g.modsSize || 0), 0);
    const ps = (MS.presets || []).filter((p) => !p.auto);
    const froms = [...new Set(ps.map((p) => p.from).filter(Boolean))];
    const recent = [...ps].sort((a, b) => b.updated - a.updated).slice(0, 6);
    box.innerHTML = `<div class="d-title" style="padding-top:20px"><h2>Tus mods</h2><div class="by">Elige un juego para ver sus mods, sus presets y buscar en su Workshop.</div></div>
      <div class="d-sec"><div class="facts">
        <div class="fact"><b>${nf0.format(mods)}</b><span>mods en este PC</span></div>
        <div class="fact"><b>${size ? fmtBytes(size) : "—"}</b><span>ocupan</span></div>
        <div class="fact"><b>${ps.length}</b><span>presets</span></div>
        <div class="fact"><b>${froms.length}</b><span>${froms.length === 1 ? "persona te ha pasado presets" : "personas te han pasado presets"}</span></div>
      </div></div>
      ${recent.length ? `<div class="d-sec"><div class="sec-h"><span class="label">Presets recientes</span></div>${recent.map((p) => `<div class="ritem" data-home-preset="${esc(p.id)}"><div class="mthumb" style="background-image:url('/img/app/${p.appid}/header');background-size:cover;background-position:center"></div><div style="min-width:0"><div class="rn">${esc(p.name)}</div><div class="rs">${esc(p.game || "")} · ${plural(p.mods.length, "mod", "mods")}${p.from ? " · de " + esc(p.from) : ""}</div></div><span></span></div>`).join("")}</div>` : ""}
      <div class="dfoot"><button class="btn ghost xs" data-mact="paste">${ic("paste")}Pegar un preset</button><button class="btn ghost xs" data-mact="import">${ic("file")}Abrir un archivo…</button></div>`;
  }

  // ---------- filtros ----------
  const MSTATES = [["all", "Todos"], ["subs", "Suscritos"], ["installed", "En el disco"], ["notdl", "Sin bajar"], ["update", "Por actualizar"], ["downloading", "Bajando o en cola"], ["nosub", "Sin suscripción"], ["missing", "Ya no existen"], ["history", "Los que tuve antes"]];
  const MSIZES = [["", "Cualquiera"], ["s", "Menos de 100 MB"], ["m", "De 100 MB a 1 GB"], ["l", "Más de 1 GB"]];
  const MUPDS = [["", "Cuando sea"], ["1m", "Último mes"], ["6m", "Últimos 6 meses"], ["1y", "Último año"], ["old", "Hace más de un año"]];
  const MRATES = [["", "Cualquiera"], ["90", "90 % o más"], ["75", "75 % o más"], ["50", "50 % o más"], ["nov", "Sin votos"]];
  const SSORTS = [["relevance", "Más parecidos"], ["trend", "Populares"], ["votes", "Mejor valorados"], ["subs", "Más suscriptores"], ["recent", "Más nuevos"], ["updated", "Actualizados"]];
  const SPERIODS = [[1, "de hoy"], [7, "de esta semana"], [30, "de este mes"], [90, "de 3 meses"], [180, "de 6 meses"], [365, "del año"]];
  const LSORTS = [["name", "Nombre"], ["size", "Más grandes"], ["updated", "Actualizados hace poco"], ["votes", "Mejor valorados"], ["subs", "Más suscriptores"], ["since", "Suscritos hace poco"]];
  const HSORTS = [["recent", "Jugados hace poco"], ["mods", "Más mods"], ["size", "Más espacio en mods"], ["hours", "Más horas"], ["name", "Nombre"]];
  const PSORTS = [["recent", "Más recientes"], ["name", "Nombre"], ["mods", "Más mods"], ["have", "Los que más tengo"]];
  const lbl = (list, v) => (list.find((x) => String(x[0]) === String(v)) || [v, v])[1];

  function stateMatch(m, st) {
    switch (st) {
      case "subs": return m.subscribed;
      case "installed": return m.installed;
      case "notdl": return m.subscribed && !m.installed && !m.downloading && !m.pending;
      case "update": return m.needsUpdate;
      case "downloading": return m.downloading || m.pending;
      case "nosub": return !m.subscribed;
      case "missing": return m.missing;
    }
    return true;
  }
  const modMatch = (m, q) => !q || q.split(/\s+/).every((w) => w.startsWith("#") ? norm([...(m.tags || []), ...(m.myTags || [])].join(" ")).includes(w.slice(1)) : norm([m.title, m.author, (m.tags || []).join(" "), (m.myTags || []).join(" "), m.id].join(" ")).includes(w));
  const modSize = (m) => m.diskSize || m.size || 0;
  const score = (m) => { const t = (m.votesUp || 0) + (m.votesDown || 0); return t ? m.votesUp / t : -1; };
  function sizeOk(m, v) { const s = modSize(m); return !v || (v === "s" ? s < 100e6 : v === "m" ? s >= 100e6 && s < 1e9 : s >= 1e9); }
  function updOk(m, v) {
    if (!v) return true;
    const age = Date.now() / 1000 - (m.updated || 0);
    return v === "old" ? age > 365 * 86400 : age <= ({ "1m": 30, "6m": 182, "1y": 365 }[v]) * 86400;
  }
  function rateOk(m, v) { if (!v) return true; const s = score(m); return v === "nov" ? s < 0 : s >= +v / 100; }

  function sourceMods() {
    if (!MS.list) return [];
    return MS.f.state === "history" ? (MS.list.history || []) : MS.list.mods;
  }
  // los mods que se ven ahora en la lista (con todos los filtros)
  function visibleMods() {
    if (MS.tab === "search") return MS.search.hideMine ? MS.search.mods.filter((m) => !m.subscribed) : MS.search.mods;
    const q = norm(MS.q.trim()), f = MS.f;
    const list = sourceMods().filter((m) => (f.state === "history" || stateMatch(m, f.state))
      && f.wtags.every((t) => (m.tags || []).includes(t)) && f.mytags.every((t) => (m.myTags || []).includes(t))
      && sizeOk(m, f.size) && updOk(m, f.upd) && rateOk(m, f.rating) && modMatch(m, q));
    const by = {
      name: (x, y) => x.title.localeCompare(y.title, "es", { sensitivity: "base" }),
      size: (x, y) => modSize(y) - modSize(x),
      updated: (x, y) => ((y.updated || 0) - (x.updated || 0)),
      votes: (x, y) => (score(y) - score(x)) || ((y.votesUp || 0) - (x.votesUp || 0)),
      subs: (x, y) => ((y.subscriptions || 0) - (x.subscriptions || 0)),
      since: (x, y) => (f.state === "history" ? (y.removed || 0) - (x.removed || 0) : (y.since || 0) - (x.since || 0)),
    }[f.sort] || (() => 0);
    return list.sort((x, y) => by(x, y) || x.title.localeCompare(y.title, "es", { sensitivity: "base" }));
  }
  function tagCounts(mods, key) {
    const c = new Map();
    for (const m of mods) for (const t of m[key] || []) c.set(t, (c.get(t) || 0) + 1);
    return [...c.entries()].sort((a, b) => (b[1] - a[1]) || a[0].localeCompare(b[0], "es"));
  }

  // filtros puestos ahora, como fichas que se quitan con la X: [etiqueta, clave para quitarla]
  function activeFilters() {
    const out = [];
    if (MS.home) {
      if (MS.h.filter !== "mods") out.push([lbl([["presets", "Con presets"], ["installed", "Instalados"], ["all", "Todos con Workshop"]], MS.h.filter), "h.filter"]);
      for (const t of MS.h.tags) out.push([t.startsWith("t:") ? "#" + t.slice(2) : t.slice(2), "h.tags|" + t]);
      return out;
    }
    if (MS.tab === "subs") {
      const f = MS.f;
      if (f.state !== "all") out.push([lbl(MSTATES, f.state), "f.state"]);
      for (const t of f.wtags) out.push([t, "f.wtags|" + t]);
      for (const t of f.mytags) out.push(["#" + t, "f.mytags|" + t]);
      if (f.size) out.push([lbl(MSIZES, f.size), "f.size"]);
      if (f.upd) out.push(["Actualizado: " + lbl(MUPDS, f.upd).toLowerCase(), "f.upd"]);
      if (f.rating) out.push(["Votos: " + lbl(MRATES, f.rating), "f.rating"]);
    } else if (MS.tab === "presets") {
      const p = MS.p;
      if (p.kind) out.push([lbl([["mine", "Solo los míos"], ["received", "Los que me han pasado"], ["auto", "Copias automáticas"]], p.kind), "p.kind"]);
      if (p.from) out.push(["De " + p.from, "p.from"]);
      for (const t of p.tags) out.push(["#" + t, "p.tags|" + t]);
    } else {
      const s = MS.search;
      for (const t of s.tags) out.push([t, "s.tags|" + t]);
      for (const t of s.exclude) out.push(["Sin " + t, "s.exclude|" + t]);
      if (s.hideMine) out.push(["Sin los que ya tengo", "s.hideMine"]);
    }
    return out;
  }
  function removeFilter(key) {
    const [k, v] = key.split("|");
    const [obj, field] = k.split(".");
    const o = { h: MS.h, f: MS.f, p: MS.p, s: MS.search }[obj];
    if (!o) return;
    if (Array.isArray(o[field])) o[field] = o[field].filter((x) => x !== v);
    else o[field] = field === "filter" ? "mods" : field === "state" ? "all" : field === "hideMine" ? false : "";
    if (obj === "s" && field !== "hideMine") runSearch(); else { renderModBar(); renderModList(); }
  }
  function clearFilters() {
    if (MS.home) MS.h = { ...MS.h, filter: "mods", tags: [] };
    else if (MS.tab === "subs") MS.f = { ...MS.f, state: "all", wtags: [], mytags: [], size: "", upd: "", rating: "" };
    else if (MS.tab === "presets") MS.p = { ...MS.p, kind: "", from: "", tags: [] };
    else { const had = MS.search.tags.length || MS.search.exclude.length; MS.search = { ...MS.search, tags: [], exclude: [], hideMine: false }; if (had) { runSearch(); return; } }
    renderModBar(); renderModList();
  }

  // un grupo de fichas del panel de filtros. multi: varias a la vez (todas
  // tienen que cumplirse)
  function chipGroup(title, key, opts, cur, multi, note) {
    const on = (v) => multi ? cur.includes(v) : String(cur) === String(v);
    return `<div class="fgroup"><div class="fg-h">${esc(title)}${note ? `<span>${esc(note)}</span>` : ""}</div><div class="fg-c">${opts.map(([v, l, n]) => `<button class="fchip${on(v) ? " on" : ""}" data-fk="${key}" data-fv="${esc(v)}">${esc(l)}${n != null ? `<small>${n}</small>` : ""}</button>`).join("")}</div></div>`;
  }
  function tagGroup(title, key, counts, cur, note, prefix) {
    if (!counts.length) return "";
    const all = MS.allTags.has(key);
    const shown = all ? counts : counts.slice(0, 24);
    const opts = shown.map(([t, n]) => [t, (prefix || "") + t, n]);
    // las elegidas siempre se ven aunque estén más abajo
    for (const t of cur) if (!shown.some(([x]) => x === t)) opts.unshift([t, (prefix || "") + t, null]);
    return chipGroup(title, key, opts, cur, true, note) .replace(/<\/div><\/div>$/, (counts.length > 24 ? `<button class="fchip more" data-mact="all-tags" data-k="${key}">${all ? "Ver menos" : `Ver todas (${counts.length})`}</button>` : "") + "</div></div>");
  }

  function renderFilterPanel(box) {
    let h = "";
    if (MS.home) {
      const all = modGames();
      const n = { mods: all.filter((g) => g.mods || presetsOf(g.appid).length).length, presets: all.filter((g) => presetsOf(g.appid).length).length, installed: all.filter((g) => g.installed).length, all: all.length };
      h += chipGroup("Mostrar", "h.filter", [["mods", "Con mods o presets", n.mods], ["presets", "Con presets", n.presets], ["installed", "Instalados", n.installed], ["all", "Todos con Workshop", n.all]], MS.h.filter, false);
      const tags = tagCounts(all, "tags").map(([t, c]) => ["t:" + t, "#" + t, c]);
      const cols = tagCounts(all, "collections").map(([t, c]) => ["c:" + t, t, c]);
      if (tags.length) h += chipGroup("Tus etiquetas", "h.tags", tags, MS.h.tags, true, "las pones en la ficha de cada juego");
      if (cols.length) h += chipGroup("Colecciones de Steam", "h.tags", cols, MS.h.tags, true);
    } else if (MS.tab === "subs") {
      const l = MS.list, f = MS.f, mods = l ? l.mods : [];
      const cnt = (st) => mods.filter((m) => stateMatch(m, st)).length;
      h += chipGroup("Estado", "f.state", MSTATES.map(([v, t]) => [v, t, v === "history" ? ((l && l.history) || []).length : cnt(v)]), f.state, false);
      const src = sourceMods();
      h += tagGroup("Etiquetas del Workshop", "f.wtags", tagCounts(src, "tags"), f.wtags, "las que les puso su autor · si eliges varias, tienen que tenerlas todas");
      h += tagGroup("Tus etiquetas", "f.mytags", tagCounts([...mods, ...((l && l.history) || [])], "myTags"), f.mytags, "se ponen en la ficha del mod o a varios a la vez con las casillas", "#");
      h += `<div class="frow3">${chipGroup("Tamaño", "f.size", MSIZES, f.size, false)}${chipGroup("Actualizado", "f.upd", MUPDS, f.upd, false)}${chipGroup("Votos a favor", "f.rating", MRATES, f.rating, false)}</div>`;
    } else if (MS.tab === "presets") {
      const ps = gamePresets(), p = MS.p;
      h += chipGroup("Cuáles", "p.kind", [["", "Todos", ps.length], ["mine", "Los míos", ps.filter((x) => !x.from && !x.auto).length], ["received", "Los que me han pasado", ps.filter((x) => x.from).length], ["auto", "Copias automáticas", ps.filter((x) => x.auto).length]], p.kind, false);
      const froms = tagCounts(ps.map((x) => ({ f: x.from ? [x.from] : [] })), "f");
      if (froms.length) h += chipGroup("Quién te lo pasó", "p.from", [["", "Cualquiera", null], ...froms.map(([t, c]) => [t, t, c])], p.from, false);
      const tags = tagCounts(ps, "tags");
      if (tags.length) h += tagGroup("Etiquetas", "p.tags", tags, p.tags, "", "#");
    } else {
      const s = MS.search;
      const known = tagCounts([...s.mods, ...(MS.list ? MS.list.mods : [])], "tags");
      for (const t of [...s.tags, ...s.exclude]) if (!known.some(([x]) => x === t)) known.unshift([t, 0]);
      h += `<div class="fgroup"><div class="fg-h">Etiquetas del Workshop<span>pulsa una vez para pedirla, otra para quitar los que la tengan y otra para olvidarla</span></div><div class="fg-c">${known.slice(0, MS.allTags.has("s") ? 400 : 40).map(([t, n]) => { const inc = s.tags.includes(t), exc = s.exclude.includes(t); return `<button class="fchip${inc ? " on" : exc ? " ex" : ""}" data-fk="stag" data-fv="${esc(t)}" title="${inc ? "Solo los que la tienen" : exc ? "Sin los que la tienen" : "Pulsa para filtrar"}">${inc ? "+ " : exc ? "− " : ""}${esc(t)}${n ? `<small>${n}</small>` : ""}</button>`; }).join("")}${known.length > 40 ? `<button class="fchip more" data-mact="all-tags" data-k="s">${MS.allTags.has("s") ? "Ver menos" : `Ver todas (${known.length})`}</button>` : ""}<input class="tag-add" id="ms-tagin" maxlength="40" placeholder="+ otra etiqueta" spellcheck="false" title="Escribe una etiqueta del Workshop y pulsa Intro"></div></div>`;
      h += chipGroup("Mostrar", "s.hideMine", [["", "Todos"], ["1", "Sin los que ya tengo"]], s.hideMine ? "1" : "", false);
    }
    h += `<div class="fpanel-foot"><button class="btn ghost xs" data-mact="clear-filters">Quitar los filtros</button><button class="btn primary xs" data-mact="close-filters">Listo</button></div>`;
    box.innerHTML = h;
  }

  function onFilterChip(key, v) {
    const [obj, field] = key.split(".");
    if (key === "stag") {
      const s = MS.search;
      if (s.tags.includes(v)) { s.tags = s.tags.filter((x) => x !== v); s.exclude.push(v); }
      else if (s.exclude.includes(v)) s.exclude = s.exclude.filter((x) => x !== v);
      else s.tags.push(v);
      runSearch();
      return;
    }
    const o = { h: MS.h, f: MS.f, p: MS.p, s: MS.search }[obj];
    if (Array.isArray(o[field])) o[field] = o[field].includes(v) ? o[field].filter((x) => x !== v) : [...o[field], v];
    else if (field === "hideMine") o.hideMine = v === "1";
    else o[field] = o[field] === v && field !== "state" && field !== "filter" && field !== "kind" ? "" : v;
    if (key === "f.state" && v === "history" && MS.f.sort === "name") MS.f.sort = "since";
    if (key === "h.filter") store.set("modHomeFilter", v);
    renderModBar(); renderModList();
  }

  // ---------- fila del buscador, panel, fichas y barra de elegidos ----------
  function renderModBar() {
    if (S.view !== "mods") return;
    const sel = (id, list, cur, title) => `<select class="fsel" id="${id}" title="${esc(title)}">${list.map(([v, l]) => `<option value="${esc(v)}"${String(v) === String(cur) ? " selected" : ""}>${esc(l)}</option>`).join("")}</select>`;
    const sb = $("#mod-sortbox"), st = $("#mod-stat"), acts = $("#mod-actions");
    const l = MS.list;
    if (MS.home) {
      sb.innerHTML = `<span class="slabel">Ordenar</span>` + sel("mh-sort", HSORTS, MS.h.sort, "Orden");
      st.innerHTML = `<b>${homeGames().length}</b> juegos`;
      acts.innerHTML = `<button class="btn ghost sm" data-mact="paste">${ic("paste")}Pegar preset</button>`;
    } else if (MS.tab === "subs") {
      sb.innerHTML = `<span class="slabel">Ordenar</span>` + sel("mf-sort", LSORTS.map(([v, t]) => [v, v === "since" && MS.f.state === "history" ? "Quitados hace poco" : t]), MS.f.sort, "Orden");
      const vis = visibleMods(), src = sourceMods();
      st.innerHTML = `<b>${vis.length}</b>${vis.length !== src.length ? " de " + src.length : ""} mods${MS.f.state !== "history" && l && l.diskSize ? " · " + fmtBytes(l.diskSize) : ""}${l && l.pending ? ` · <span class="spinner xs"></span>trayendo datos de ${l.pending}` : ""}`;
      acts.innerHTML = `<button class="btn ghost sm" data-mact="subs-menu">${ic("more")}Acciones</button>`;
    } else if (MS.tab === "presets") {
      sb.innerHTML = `<span class="slabel">Ordenar</span>` + sel("mp-sort", PSORTS, MS.p.sort, "Orden");
      st.innerHTML = `<b>${filteredPresets().length}</b> presets`;
      acts.innerHTML = (MS.pchecked.size > 1 ? `<button class="btn go sm" data-mact="activate-checked">${ic("layers")}Activar los ${MS.pchecked.size} juntos</button>` : "") + `<button class="btn primary sm" data-mact="new-menu">${ic("plus")}Nuevo preset</button>`;
    } else {
      const s = MS.search;
      const sorts = SSORTS.filter(([v]) => v !== "relevance" || s.q);
      sb.innerHTML = `<span class="slabel">Ordenar</span>` + sel("ms-sort", sorts, s.sort, "Orden") + (s.sort === "trend" ? sel("ms-days", SPERIODS, s.days, "De qué periodo") : "");
      st.innerHTML = s.loading ? `<span class="spinner xs"></span>buscando…` : s.total ? `<b>${nf0.format(s.total)}</b> mods` : "";
      acts.innerHTML = `<button class="ibtn" data-mact="search-menu" title="Más opciones">${ic("more")}</button>`;
    }
    const vbox = $("#mod-views");
    vbox.hidden = MS.home || MS.tab === "presets";
    $$("#mod-views button").forEach((b) => b.classList.toggle("on", b.dataset.v === MS.view));
    // filtros
    const af = activeFilters();
    const fb = $("#mod-fbtn");
    fb.classList.toggle("on", MS.fopen);
    $("#mod-fcount").textContent = af.length || "";
    const panel = $("#mod-filters");
    panel.hidden = !MS.fopen;
    if (MS.fopen) renderFilterPanel(panel);
    $("#mod-active").innerHTML = af.length && !MS.fopen ? af.map(([t, k]) => `<span class="chip afil">${esc(t)}<button data-rmf="${esc(k)}" title="Quitar este filtro">${ic("x")}</button></span>`).join("") + `<button class="btn ghost xs" data-mact="clear-filters">Quitar todos</button>` : "";
    $("#mod-active").hidden = !af.length || MS.fopen;
    renderBulk();
  }

  // barra de abajo con lo que se puede hacer con los mods elegidos
  function renderBulk() {
    const box = $("#mod-bulk");
    const ok = !MS.home && (MS.tab === "subs" || MS.tab === "search") && MS.checked.size > 0;
    box.hidden = !ok;
    if (!ok) return;
    const ids = [...MS.checked];
    const ms = ids.map(modById).filter(Boolean);
    const toSub = ms.filter((m) => !m.subscribed).length, toUnsub = ms.filter((m) => m.subscribed).length;
    const size = ms.reduce((s, m) => s + modSize(m), 0);
    box.innerHTML = (MS.pickOpen ? `<div class="picklist"><div class="pl-h"><div class="search"><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg><input id="pick-q" placeholder="Buscar entre los elegidos" spellcheck="false" autocomplete="off" value="${esc(MS.pickQ)}"></div><button class="ibtn" data-mact="pick-list" title="Cerrar la lista">${ic("x")}</button></div><div class="pl-items" id="pick-items">${pickItems()}</div></div>` : "") +
      `<button class="btn ghost sm pickbtn${MS.pickOpen ? " on" : ""}" data-mact="pick-list" title="Ver y repasar los elegidos"><b>${ids.length}</b> ${ids.length === 1 ? "elegido" : "elegidos"}${size ? " · " + fmtBytes(size) : ""}<svg viewBox="0 0 24 24"><path d="${MS.pickOpen ? "m6 9 6 6 6-6" : "m18 15-6-6-6 6"}"/></svg></button>
      ${toSub ? `<button class="btn go sm" data-mact="sub-checked">${ic("plus")}Suscribirse${toSub !== ids.length ? " a " + toSub : ""}</button>` : ""}
      ${toUnsub ? `<button class="btn ghost sm danger-txt" data-mact="unsub-checked">${ic("minus")}Quitar${toUnsub !== ids.length ? " " + toUnsub : ""}</button>` : ""}
      <button class="btn ghost sm" data-mact="save-checked">${ic("save")}Hacer un preset</button>
      <span class="sep"></span>
      <input class="tag-add" id="mod-bulk-tag" maxlength="24" placeholder="+ etiqueta tuya" spellcheck="false" title="Escribe y pulsa Intro para ponérsela a todos los elegidos">
      <button class="btn ghost sm" data-mact="bulk-untag">Quitar etiqueta…</button>
      <span class="grow"></span>
      <button class="btn ghost sm" data-mact="uncheck">Ninguno</button>`;
  }

  // la lista de elegidos (con su buscador)
  function pickItems() {
    const q = norm(MS.pickQ.trim());
    const list = [...MS.checked].map((id) => modById(id) || { id, title: "Mod " + id }).filter((m) => !q || norm(m.title + " " + (m.author || "")).includes(q));
    if (!list.length) return `<div class="note" style="padding:10px">${MS.checked.size ? "Ninguno coincide." : "No hay ninguno elegido."}</div>`;
    return list.map((m) => `<div class="pl-it">${thumbMod(m)}<div style="min-width:0"><div class="rn">${esc(m.title)}</div><div class="rs">${[m.subscribed ? "Suscrito" : "Sin suscripción", modSize(m) ? fmtBytes(modSize(m)) : ""].filter(Boolean).join(" · ")}</div></div><button class="ibtn" data-unpick="${m.id}" title="Quitarlo de los elegidos">${ic("x")}</button></div>`).join("");
  }

  // encabezado de la lista: elegir todos los que se ven
  function listHead(vis) {
    if (!vis.length) return "";
    const all = vis.every((m) => MS.checked.has(m.id));
    return `<label class="lhead"><input type="checkbox" data-mact="check-all"${all ? " checked" : ""}><span>${all ? "Quitar la marca a todos" : `Elegir los ${vis.length} que se ven`}</span></label>`;
  }

  function renderModList() {
    const box = $("#mod-list");
    if (S.view !== "mods") return;
    if (MS.home) { renderHomeList(box); return; }
    if (!S.games) { box.innerHTML = spinnerBox("Leyendo tu biblioteca…"); return; }
    if (MS.tab === "subs") renderModSubs(box);
    else if (MS.tab === "presets") renderPresetList(box);
    else renderSearch(box);
  }

  function modChips(m, mode) {
    const out = [];
    const acc = m.subscribedBy && S.accounts.find((a) => a.steamId === m.subscribedBy);
    if (m.missing) out.push(`<span class="chip bad" title="Lo ha borrado su autor o lo ha hecho privado">Ya no existe</span>`);
    if (m.removed) out.push(`<span class="chip off" title="Quitado ${esc(fmtDate(m.removed))}">Quitado ${esc(ago(m.removed))}</span>`);
    else if (mode === "search") { if (m.subscribed) out.push(`<span class="chip token">${ic("check")}Suscrito</span>`); }
    else if (!m.subscribed && m.installed) {
      const other = m.subscribedBy && MS.list && m.subscribedBy !== MS.list.account;
      out.push(`<span class="chip off" title="Steam lo tiene en el disco pero la cuenta abierta no está suscrita">${other ? "De " + esc(acc ? display(acc) : "otra cuenta") : "Sin suscripción"}</span>`);
    } else if (m.subscribed && !m.installed && !m.downloading && !m.pending) out.push(`<span class="chip ask" title="Steam lo bajará cuando el juego esté instalado o lo abras">Sin bajar</span>`);
    if (m.downloading) out.push(`<span class="chip token">Bajando ${Math.round((m.progress || 0) * 100)}%</span>`);
    else if (m.pending) out.push(`<span class="chip token">En cola</span>`);
    else if (m.needsUpdate) out.push(`<span class="chip ask" title="Steam lo actualizará">Actualizar</span>`);
    for (const t of (m.myTags || []).slice(0, 3)) out.push(`<span class="chip from">#${esc(t)}</span>`);
    return out.join("");
  }
  function votesHTML(m) {
    const t = (m.votesUp || 0) + (m.votesDown || 0);
    if (!m.hasVotes || !t) return "";
    const pct = Math.round(m.votesUp / t * 100);
    return `<span class="votes" title="${nf0.format(m.votesUp)} a favor · ${nf0.format(m.votesDown)} en contra"><span class="vbar"><i style="width:${pct}%"></i></span>${pct}%</span>`;
  }
  function thumbMod(m) {
    return `<div class="mthumb">${m.preview ? `<img src="/img/mod/${m.id}" alt="" loading="lazy" draggable="false" onload="this.classList.add('on')" onerror="this.remove()">` : ""}${m.downloading ? `<i class="bar" style="width:${Math.round((m.progress || 0) * 100)}%"></i>` : ""}</div>`;
  }
  function modRow(m, mode) {
    const sub = [];
    if (m.author) sub.push(esc(m.author));
    const size = m.diskSize || m.size;
    if (size) sub.push(fmtBytes(size));
    if (m.updated) sub.push("act. " + esc(ago(m.updated)));
    if (mode === "search" && m.subscriptions) sub.push(nf0.format(m.subscriptions) + " suscr.");
    const v = votesHTML(m);
    if (v) sub.push(v);
    const act = !m.subscribed
      ? `<button class="ibtn sub" data-sub="${m.id}" title="${m.removed ? "Volver a suscribirse" : "Suscribirse"}">${ic("plus")}</button>`
      : `<button class="ibtn danger" data-unsub="${m.id}" title="Quitar la suscripción">${ic("minus")}</button>`;
    const sel = MS.sel && MS.sel.kind === "mod" && MS.sel.id === m.id;
    return `<div class="mrow${sel ? " sel" : ""}${!m.subscribed && mode !== "search" ? " dim" : ""}${MS.checked.has(m.id) ? " checked" : ""}" data-mod="${m.id}">
      <input type="checkbox" class="ck" data-check="${m.id}"${MS.checked.has(m.id) ? " checked" : ""} title="Elegir (Mayús para elegir varios seguidos)">
      ${thumbMod(m)}
      <div class="mtxt"><div class="mn" title="${esc(m.title)}">${esc(m.title)}</div><div class="ms">${sub.join(" · ") || "&nbsp;"}</div></div>
      <div class="mchips">${modChips(m, mode)}</div>
      <div class="macts">${act}<button class="ibtn" data-mmenu="${m.id}" title="Más opciones">${ic("more")}</button></div>
    </div>`;
  }
  // valoración: % de votos a favor y cuántos votos
  function ratingHTML(m) {
    const t = (m.votesUp || 0) + (m.votesDown || 0);
    if (!m.hasVotes || !t) return `<span class="novotes">Sin votos</span>`;
    const pct = Math.round(m.votesUp / t * 100);
    return `<span class="votes" title="${nf0.format(m.votesUp)} a favor · ${nf0.format(m.votesDown)} en contra"><span class="vbar"><i style="width:${pct}%"></i></span><b>${pct} %</b><small>${nf0.format(t)} ${t === 1 ? "voto" : "votos"}</small></span>`;
  }
  const subsHTML = (m) => m.subscriptions ? `<span title="Suscriptores">${ic("users")}${nf0.format(m.subscriptions)}</span>` : "";
  const modAct = (m) => !m.subscribed
    ? `<button class="ibtn sub" data-sub="${m.id}" title="${m.removed ? "Volver a suscribirse" : "Suscribirse"}">${ic("plus")}</button>`
    : `<button class="ibtn danger" data-unsub="${m.id}" title="Quitar la suscripción">${ic("minus")}</button>`;
  const modImg = (m, cls) => `<div class="mthumb ${cls || ""}">${m.preview ? `<img src="/img/mod/${m.id}" alt="" loading="lazy" draggable="false" onload="this.classList.add('on')" onerror="this.remove()">` : ""}${m.downloading ? `<i class="bar" style="width:${Math.round((m.progress || 0) * 100)}%"></i>` : ""}</div>`;

  // vista de detalles: imagen grande y todo lo que se sabe
  function modDetailRow(m, mode) {
    const sel = MS.sel && MS.sel.kind === "mod" && MS.sel.id === m.id;
    const tags = [...(m.myTags || []).map((t) => `<span class="chip from">#${esc(t)}</span>`), ...(m.tags || []).slice(0, 6).map((t) => `<span class="chip tag clickable" data-wtag="${esc(t)}">${esc(t)}</span>`)].join("");
    return `<div class="mrow drow${sel ? " sel" : ""}${!m.subscribed && mode !== "search" ? " dim" : ""}${MS.checked.has(m.id) ? " checked" : ""}" data-mod="${m.id}">
      <input type="checkbox" class="ck" data-check="${m.id}"${MS.checked.has(m.id) ? " checked" : ""} title="Elegir (Mayús para elegir varios seguidos)">
      ${modImg(m, "big")}
      <div class="mtxt">
        <div class="mn" title="${esc(m.title)}">${esc(m.title)}</div>
        <div class="ms">${m.author ? "de <b>" + esc(m.author) + "</b>" : "&nbsp;"}</div>
        ${m.summary ? `<div class="msum">${esc(m.summary)}</div>` : ""}
        ${tags ? `<div class="mtags">${tags}</div>` : ""}
      </div>
      <div class="mstats">
        ${ratingHTML(m)}
        <div class="mst">${subsHTML(m)}${modSize(m) ? `<span>${fmtBytes(modSize(m))}</span>` : ""}</div>
        <div class="mst">${m.updated ? `<span>act. ${esc(ago(m.updated))}</span>` : ""}</div>
        <div class="mchips">${modChips({ ...m, myTags: [] }, mode)}</div>
      </div>
      <div class="macts">${modAct(m)}<button class="ibtn" data-mmenu="${m.id}" title="Más opciones">${ic("more")}</button></div>
    </div>`;
  }
  // vista de cuadrícula: imágenes grandes
  function modCard(m, mode) {
    const sel = MS.sel && MS.sel.kind === "mod" && MS.sel.id === m.id;
    return `<div class="mcard${sel ? " sel" : ""}${!m.subscribed && mode !== "search" ? " dim" : ""}${MS.checked.has(m.id) ? " checked" : ""}" data-mod="${m.id}">
      <div class="mc-img">${modImg(m, "card")}
        <input type="checkbox" class="ck" data-check="${m.id}"${MS.checked.has(m.id) ? " checked" : ""} title="Elegir">
        <div class="mc-chips">${modChips({ ...m, myTags: [] }, mode)}</div>
        <div class="mc-act">${modAct(m)}</div>
      </div>
      <div class="mc-t" title="${esc(m.title)}">${esc(m.title)}</div>
      <div class="mc-r">${ratingHTML(m)}</div>
      <div class="mc-m">${[subsHTML(m), modSize(m) ? `<span>${fmtBytes(modSize(m))}</span>` : "", m.author ? `<span>${esc(m.author)}</span>` : ""].filter(Boolean).join("")}</div>
    </div>`;
  }
  // los mods con la vista elegida
  function rowsHTML(list, mode) {
    if (MS.view === "grid") return `<div class="mgrid">${list.map((m) => modCard(m, mode)).join("")}</div>`;
    if (MS.view === "details") return `<div class="mlist details">${list.map((m) => modDetailRow(m, mode)).join("")}</div>`;
    return `<div class="mlist">${list.map((m) => modRow(m, mode)).join("")}</div>`;
  }

  const groupHead = (label, n) => `<div class="mgroup"><span class="label">${esc(label)}</span><span class="mini fstat">${n}</span><span class="line"></span></div>`;

  function switchOwnerHTML() {
    const own = ownersOf(MS.app);
    if (!own.length) return `<p>Ninguna de tus cuentas guardadas tiene este juego.</p>`;
    return `<div class="row-btns" style="justify-content:flex-start"><button class="btn primary sm" data-mact="switch-owner">${ic("login")}Cambiar a ${esc(display(own[0]))}</button>${own.length > 1 ? `<button class="btn ghost sm" data-mact="switch-owner-menu">Otra cuenta…</button>` : ""}</div>`;
  }

  function renderModSubs(box) {
    const l = MS.list;
    if (!l) { box.innerHTML = spinnerBox("Leyendo los mods…"); return; }
    const g = modGame();
    if (l.noGame) {
      box.innerHTML = `<div class="empty">${ic("puzzle")}<b>${esc(g ? g.name : "Este juego")} no está en tu biblioteca</b><p>Tienes presets de este juego que te han pasado. Para suscribirte a sus mods, entra en Steam con una cuenta que lo tenga.</p><div class="row-btns"><button class="btn ghost sm" data-mact="tab-presets">Ver sus presets</button></div></div>`;
      return;
    }
    const vis = visibleMods();
    let h = "";
    if (!l.live && notOwnedErr(l.liveError)) h += `<div class="warn-box"><b>La cuenta abierta en Steam no tiene ${esc(g ? g.name : "este juego")}.</b> Para ver y cambiar sus suscripciones, entra con una cuenta que lo tenga. Abajo sale lo que hay en este PC.${switchOwnerHTML()}</div>`;
    else if (!l.live) h += `<div class="warn-box">${l.liveError ? esc(l.liveError) + ". " : ""}Esta lista sale de lo que Steam tiene en este PC: las suscripciones que aún no se han bajado no salen hasta que conectes con Steam.</div>`;
    if (!sourceMods().length) {
      if (MS.f.state === "history") { box.innerHTML = h + `<div class="empty">${ic("clock")}<b>Todavía no hay historial</b><p>Aquí saldrán los mods que quites de este juego (desde aquí o desde Steam), para poder volver a ponerlos. TurboGAMES lo apunta cada vez que conecta con Steam.</p></div>`; return; }
      const why = l.live ? "La cuenta abierta en Steam no está suscrita a ningún mod de este juego." : "En este PC no hay mods de este juego.";
      box.innerHTML = h + `<div class="empty">${ic("puzzle")}<b>Sin mods de ${esc(g ? g.name : "este juego")}</b><p>${why}</p><div class="row-btns"><button class="btn primary sm" data-mact="tab-search">${ic("search")}Buscar en el Workshop</button><button class="btn ghost sm" data-mact="paste">${ic("paste")}Pegar preset</button></div></div>`;
      return;
    }
    if (!vis.length) { box.innerHTML = h + `<div class="empty">${ic("search")}<b>Ningún mod coincide</b><p>Prueba con otro nombre o quita los filtros.</p><div class="row-btns"><button class="btn ghost sm" data-mact="clear-filters">Quitar los filtros</button></div></div>`; return; }
    h += listHead(vis);
    if (MS.f.state === "all" && MS.f.sort === "name") {
      const subs = vis.filter((m) => m.subscribed), rest = vis.filter((m) => !m.subscribed);
      if (subs.length) h += (rest.length ? groupHead("Suscritos", subs.length) : "") + rowsHTML(subs);
      if (rest.length) h += groupHead("En este PC, sin suscripción de esta cuenta", rest.length) + rowsHTML(rest);
    } else h += rowsHTML(vis);
    box.innerHTML = h;
  }

  function presetMatch(p, q) {
    const f = MS.p;
    if (f.kind === "mine" && (p.from || p.auto)) return false;
    if (f.kind === "received" && !p.from) return false;
    if (f.kind === "auto" && !p.auto) return false;
    if (f.from && p.from !== f.from) return false;
    if (!f.tags.every((t) => (p.tags || []).includes(t))) return false;
    return !q || q.split(/\s+/).every((w) => norm([p.name, p.author, p.from, p.notes, (p.tags || []).join(" ")].join(" ")).includes(w.replace(/^#/, "")));
  }
  function filteredPresets() {
    const q = norm(MS.q.trim());
    const subs = new Set(subscribedIds());
    const have = (p) => p.mods.length ? p.mods.filter((m) => subs.has(m.id)).length / p.mods.length : 0;
    const by = { recent: (a, b) => b.updated - a.updated, name: (a, b) => a.name.localeCompare(b.name, "es", { sensitivity: "base" }), mods: (a, b) => b.mods.length - a.mods.length, have: (a, b) => have(b) - have(a) }[MS.p.sort] || (() => 0);
    return gamePresets().filter((p) => presetMatch(p, q)).sort(by);
  }
  function presetCard(p, subs) {
    const have = p.mods.filter((m) => subs.has(m.id)).length;
    const pct = p.mods.length ? Math.round(have / p.mods.length * 100) : 0;
    const chips = [];
    if (p.auto) chips.push(`<span class="chip off">${ic("clock")}Copia automática</span>`);
    else if (p.from) chips.push(`<span class="chip from" title="Te lo pasó ${esc(p.from)}">${ic("user")}De ${esc(p.from)}</span>`);
    else chips.push(`<span class="chip">${ic("star")}Tuyo</span>`);
    for (const t of p.tags || []) chips.push(`<span class="chip tag">#${esc(t)}</span>`);
    const sel = MS.sel && MS.sel.kind === "preset" && MS.sel.id === p.id;
    return `<div class="pcard${sel ? " sel" : ""}${p.auto ? " auto" : ""}" data-preset="${esc(p.id)}">
      <div class="ph"><input type="checkbox" class="ck" data-pcheck="${esc(p.id)}"${MS.pchecked.has(p.id) ? " checked" : ""} title="Elegir para activar varios juntos">
        <div class="pt"><div class="pn" title="${esc(p.name)}">${esc(p.name)}</div><div class="pg">${plural(p.mods.length, "mod", "mods")}${p.author && p.author !== p.from ? " · hecho por " + esc(p.author) : ""} · ${esc(ago(p.updated) || fmtDate(p.updated))}</div></div>
        <button class="ibtn" data-pmenu="${esc(p.id)}" title="Más opciones">${ic("more")}</button></div>
      <div class="pchips">${chips.join("")}</div>
      <div class="pcov" title="${have} de ${p.mods.length} ya suscritos"><i style="width:${pct}%"></i></div>
      <div class="pf"><span>${have === p.mods.length ? "Los tienes todos" : `${have} de ${p.mods.length} suscritos`}</span>
        <button class="ibtn" data-pcopy="${esc(p.id)}" title="Copiar para mandarlo por un chat">${ic("copy")}</button>
        <button class="btn go xs" data-pact="${esc(p.id)}" title="Suscribirte a estos mods y quitar los demás">${ic("play")}Activar</button></div>
    </div>`;
  }
  function renderPresetList(box) {
    if (!MS.presets) { box.innerHTML = spinnerBox("Leyendo los presets…"); return; }
    const all = gamePresets();
    const ps = filteredPresets();
    if (!all.length) {
      box.innerHTML = `<div class="empty">${ic("layers")}<b>Todavía no hay presets de este juego</b><p>Un preset es una lista de mods que activas de golpe: te suscribe a los suyos y quita los demás. Guarda lo que tienes ahora, o pega uno que te hayan pasado por un chat.</p><div class="row-btns"><button class="btn primary sm" data-mact="new-preset"${subscribedIds().length ? "" : " disabled"}>${ic("save")}Guardar lo suscrito</button><button class="btn ghost sm" data-mact="paste">${ic("paste")}Pegar preset</button><button class="btn ghost sm" data-mact="import">${ic("file")}Abrir archivo…</button></div></div>`;
      return;
    }
    if (!ps.length) { box.innerHTML = `<div class="empty">${ic("search")}<b>Ningún preset coincide</b><p>Prueba con otro nombre o quita los filtros.</p></div>`; return; }
    const subs = new Set(subscribedIds());
    const mine = ps.filter((p) => !p.auto), autos = ps.filter((p) => p.auto);
    let h = "";
    if (mine.length) h += `<div class="pgrid">${mine.map((p) => presetCard(p, subs)).join("")}</div>`;
    if (autos.length) h += groupHead("Copias automáticas (antes de quitar mods)", autos.length) + `<div class="pgrid">${autos.map((p) => presetCard(p, subs)).join("")}</div>`;
    box.innerHTML = h;
  }

  function renderSearch(box) {
    const s = MS.search;
    if (MS.list && MS.list.noGame) { box.innerHTML = `<div class="empty">${ic("search")}<b>Necesitas tener el juego</b><p>Para buscar en su Workshop, entra en Steam con una cuenta que lo tenga.</p></div>`; return; }
    if (s.err && !s.mods.length) {
      const own = notOwnedErr(s.err);
      box.innerHTML = `<div class="empty">${ic("plug")}<b>${own ? "La cuenta abierta no tiene este juego" : "No se puede buscar ahora"}</b><p>${esc(s.err)}. Para buscar en el Workshop desde aquí, Steam tiene que estar abierto con una cuenta que tenga el juego.</p>${own ? switchOwnerHTML() : ""}<div class="row-btns"><button class="btn ${own ? "ghost" : "primary"} sm" data-mact="search-retry">${ic("refresh")}Reintentar</button><button class="btn ghost sm" data-mact="workshop-web">${ic("globe")}Abrir el Workshop en el navegador</button></div></div>`;
      return;
    }
    const vis = visibleMods();
    if (!vis.length) { box.innerHTML = s.loading ? spinnerBox("Buscando en el Workshop…") : (s.app === MS.app ? `<div class="empty">${ic("search")}<b>Nada por aquí</b><p>${s.mods.length ? "Ya tienes todos los de esta página: carga más o quita «Sin los que ya tengo»." : "Prueba con otras palabras o quita algún filtro."}</p>${s.mods.length && !s.done ? `<div class="row-btns"><button class="btn ghost sm" data-mact="search-more">Cargar más</button></div>` : ""}</div>` : ""); return; }
    box.innerHTML = listHead(vis) + rowsHTML(vis, "search") +
      (s.loading ? spinnerBox("Buscando…") : !s.done ? `<div class="more-row"><button class="btn ghost sm" data-mact="search-more">Cargar más</button></div>` : "");
  }

  async function runSearch(more) {
    const s = MS.search, app = MS.app;
    if (!app || s.loading) return;
    if (!more) {
      const q = MS.tab === "search" ? MS.q.trim() : "";
      // con texto, lo normal es ordenar por parecido (si no se ha elegido otro orden)
      if (q && !s.q && !s.userSort) s.sort = "relevance";
      if (!q && s.sort === "relevance") s.sort = "trend";
      s.mods = []; s.cursor = ""; s.done = false; s.err = ""; s.total = 0; s.q = q;
    }
    s.app = app;
    s.loading = true;
    renderModBar(); renderModList();
    try {
      const r = await api.ModsSearch(app, { text: s.q, sort: s.sort, days: +s.days || 7, cursor: more ? s.cursor : "", tags: s.tags, exclude: s.exclude });
      if (MS.app !== app) return;
      const seen = new Set(s.mods.map((m) => m.id));
      s.mods = s.mods.concat((r.mods || []).filter((m) => !seen.has(m.id)));
      s.total = r.total; s.cursor = r.cursor; s.done = !r.cursor || !(r.mods || []).length;
    } catch (e) { s.err = errText(e); }
    finally { s.loading = false; }
    renderMods();
  }

  // ---------- ficha de un mod ----------
  function bbcode(src) {
    let s = esc(String(src || "").slice(0, 12000));
    const link = (u, t) => /^https?:\/\//i.test(u) ? `<a data-url="${u}">${t}</a>` : t;
    s = s.replace(/\[url=([^\]\s]+)\]([\s\S]*?)\[\/url\]/gi, (_, u, t) => link(u.replace(/&quot;|&#39;/g, ""), t))
      .replace(/\[url\]([\s\S]*?)\[\/url\]/gi, (_, u) => link(u, u))
      .replace(/\[img\][\s\S]*?\[\/img\]/gi, "")
      .replace(/\[previewyoutube=[^\]]*\][\s\S]*?\[\/previewyoutube\]/gi, "")
      .replace(/\[(b|h1|h2|h3)\]/gi, "<b>").replace(/\[\/(b|h1|h2|h3)\]/gi, "</b>")
      .replace(/\s*\[\*\]\s*/g, "\n• ")
      .replace(/\[\/?[a-z0-9]+(=[^\]]*)?\]/gi, "")
      .replace(/\r/g, "").replace(/\n{3,}/g, "\n\n").trim()
      .replace(/\n/g, "<br>");
    return s;
  }
  function starsHTML(n) {
    if (n == null || n < 0) return "";
    return `<span class="stars" title="${n} de 5 estrellas">${[1, 2, 3, 4, 5].map((i) => `<svg viewBox="0 0 24 24" class="${i <= n ? "" : "o"}">${P.star}</svg>`).join("")}</span>`;
  }
  async function loadInfo(m) {
    if (MS.info.has(m.id)) return MS.info.get(m.id);
    const live = !!(MS.list && MS.list.live);
    const p = api.ModInfo(MS.app, m.id, live).catch((e) => ({ error: errText(e), gallery: [], required: [], commentList: [] }));
    MS.info.set(m.id, p);
    const d = await p;
    MS.info.set(m.id, d);
    if (d && d.mod && d.mod.known) {
      // trae datos más nuevos (votos, etiquetas…): se pasan al mod de la lista
      const cur = modById(m.id);
      if (cur) for (const k of ["votesUp", "votesDown", "hasVotes", "subscriptions", "tags", "children", "author"]) if (d.mod[k] !== undefined && d.mod[k] !== "") cur[k] = d.mod[k];
    }
    if (MS.sel && MS.sel.kind === "mod" && MS.sel.id === m.id) renderModDetail();
    return d;
  }
  function presetsWith(id) { return gamePresets().filter((p) => !p.auto && p.mods.some((x) => x.id === id)); }
  const ytThumb = (v) => `https://i.ytimg.com/vi/${encodeURIComponent(v)}/mqdefault.jpg`;
  function galleryHTML(d) {
    const g = (d && d.gallery) || [];
    if (!g.length) return "";
    return `<div class="d-sec"><div class="sec-h"><span class="label">Imágenes y vídeos</span><span class="mini">${g.length}</span></div>
      <div class="gal">${g.map((x, i) => x.video
        ? `<button class="gal-it vid" data-gal="${i}" title="Ver el vídeo"><img src="${esc(ytThumb(x.video))}" alt="" loading="lazy" draggable="false" onerror="this.remove()"><i>${ic("play")}</i></button>`
        : `<button class="gal-it" data-gal="${i}" title="Ver en grande"><img src="${esc(x.url)}?imw=400&imh=225&ima=fit&impolicy=Letterbox" alt="" loading="lazy" draggable="false" onerror="this.remove()"></button>`).join("")}</div></div>`;
  }

  function renderModDetail() {
    const box = $("#mod-detail");
    if (MS.home) { renderHomeDetail(box); return; }
    if (!MS.sel) {
      const tips = { subs: ["Elige un mod", "Verás sus imágenes y vídeos, su descripción, valoración, los mods que necesita y los últimos comentarios. Con las casillas eliges varios para suscribirte, quitarlos o etiquetarlos de golpe."], presets: ["Elige un preset", "Verás sus mods, cuáles te faltan, de quién viene, y podrás activarlo o mandarlo por un chat."], search: ["Busca mods nuevos", "Escribe arriba y pulsa Intro, ordena o filtra por etiquetas. Con + te suscribes, o elige varios con las casillas."] }[MS.tab];
      box.innerHTML = `<div class="empty-d">${ic(MS.tab === "presets" ? "layers" : "puzzle")}<b>${tips[0]}</b><span>${tips[1]}</span></div>`;
      return;
    }
    if (MS.sel.kind === "preset") { renderPresetDetail(box); return; }
    const m = modById(MS.sel.id);
    if (!m) { MS.sel = null; renderModDetail(); return; }
    const info = MS.info.get(m.id);
    if (!info) loadInfo(m);
    const d = info && !(info instanceof Promise) ? info : null;
    const chips = modChips({ ...m, myTags: [] }, m.subscribed ? "search" : "") + (m.collection ? `<span class="chip">Colección</span>` : "") + (m.banned ? `<span class="chip bad">Bloqueado por Steam</span>` : "");
    const size = m.diskSize || m.size;
    const rating = d && d.stars >= 0 ? `${starsHTML(d.stars)}` : (votesHTML(m) || "—");
    const ratingSub = d && d.ratings ? `${nf0.format(d.ratings)} valoraciones` : (m.hasVotes ? `${nf0.format((m.votesUp || 0) + (m.votesDown || 0))} votos` : "valoración");
    const inP = presetsWith(m.id);
    let req = "";
    if (d && d.required && d.required.length) {
      const subs = new Set(subscribedIds());
      const missing = d.required.filter((r) => !subs.has(r.id));
      req = `<div class="d-sec"><div class="sec-h"><span class="label">Necesita</span><span class="mini">${d.required.length}</span>${missing.length && m.subscribed ? `<button class="btn ghost xs" data-mact="sub-req">${ic("plus")}Suscribirse a ${missing.length === 1 ? "el que falta" : "los " + missing.length + " que faltan"}</button>` : ""}</div>
        ${d.required.map((r) => `<div class="ritem" data-rmod="${r.id}">${thumbMod(r)}<div style="min-width:0"><div class="rn">${esc(r.title || "Mod " + r.id)}</div><div class="rs${subs.has(r.id) ? " ok" : ""}">${subs.has(r.id) ? "Suscrito" : r.missing ? "Ya no existe" : "No suscrito"}${r.size ? " · " + fmtBytes(r.size) : ""}</div></div>${subs.has(r.id) ? "<span></span>" : `<button class="ibtn sub" data-sub="${r.id}" title="Suscribirse">${ic("plus")}</button>`}</div>`).join("")}</div>`;
    }
    const desc = d ? (d.description || "") : "";
    const comments = d && d.commentList && d.commentList.length
      ? d.commentList.map((c) => `<div class="comment"><div class="ca">${esc(c.author)}<span>${esc(ago(c.time))}</span></div><div class="ct">${esc(c.text)}</div></div>`).join("")
      : d ? `<div class="note">${d.commentsN ? "Steam solo enseña sus comentarios dentro de su página (el perfil del autor es privado)." : "Todavía no tiene comentarios."}</div>` : `<div class="note">Cargando…</div>`;
    box.innerHTML = `
      <div class="m-hero">${m.preview ? `<img src="/img/mod/${m.id}" alt="" draggable="false" onerror="this.remove()">` : ""}${d && d.gallery && d.gallery.length ? `<button class="hero-gal" data-gal="0" title="Ver las imágenes y vídeos">${ic("play")}${d.gallery.length}</button>` : ""}</div>
      <div class="d-title">
        <h2>${esc(m.title)}</h2>
        <div class="by">${m.author ? "de <b>" + esc(m.author) + "</b> · " : ""}<span class="mono">${m.id}</span>${m.since && m.subscribed ? " · suscrito desde " + esc(fmtDate(m.since)) : ""}</div>
        <div class="d-chips">${chips}</div>
        <div class="d-actions">${m.subscribed ? `<button class="btn ghost sm danger-txt" data-unsub="${m.id}">${ic("minus")}Quitar suscripción</button>` : `<button class="btn go sm" data-sub="${m.id}">${ic("plus")}${m.removed ? "Volver a suscribirse" : "Suscribirse"}</button>`}
          <button class="ibtn" data-mact="mod-steam" title="Abrir en Steam">${ic("open")}</button><button class="ibtn" data-mact="mod-web" title="Abrir en el navegador">${ic("globe")}</button><button class="ibtn" data-mmenu="${m.id}" title="Más opciones">${ic("more")}</button></div>
      </div>
      <div class="d-sec">
        <div class="facts">
          <div class="fact"><b>${size ? fmtBytes(size) : "—"}</b><span>${m.installed ? "en el disco" : "tamaño"}</span></div>
          <div class="fact"><b>${m.updated ? esc(ago(m.updated)) : "—"}</b><span>actualizado</span></div>
          <div class="fact"><b>${m.subscriptions ? nf0.format(m.subscriptions) : "—"}</b><span>suscriptores</span></div>
          <div class="fact"><b>${rating}</b><span>${ratingSub}</span></div>
        </div>
        ${m.installed && m.dir ? `<div class="path"><span title="${esc(m.dir)}">&lrm;${esc(m.dir)}&lrm;</span><button class="ibtn" data-mact="mod-folder" title="Abrir la carpeta">${ic("folder")}</button></div>` : ""}
      </div>
      ${d ? galleryHTML(d) : `<div class="d-sec"><div class="sec-h"><span class="label">Imágenes y vídeos</span></div><div class="note">Cargando…</div></div>`}
      <div class="d-sec"><div class="sec-h"><span class="label">Tus etiquetas</span></div>
        <div class="tags" style="margin:0">${(m.myTags || []).map((t) => `<span class="chip from">#${esc(t)}<button data-muntag="${esc(t)}" title="Quitar">${ic("x")}</button></span>`).join("")}<input class="tag-add" id="md-tag" maxlength="24" placeholder="+ etiqueta" spellcheck="false" title="Para ordenarlos a tu manera: imprescindible, gráficos, probar… (Intro)"></div></div>
      ${req}
      ${inP.length ? `<div class="d-sec"><div class="sec-h"><span class="label">En tus presets</span></div><div class="g-tags">${inP.map((p) => `<span class="chip coll" data-goto-preset="${esc(p.id)}">${ic("layers")}${esc(p.name)}</span>`).join("")}</div></div>` : ""}
      <div class="d-sec"><div class="sec-h"><span class="label">Descripción</span></div>${d ? (desc ? `<div class="desc${desc.length > 900 ? " long" : ""}" id="md-desc">${bbcode(desc)}</div>${desc.length > 900 ? `<div class="sec-foot"><button class="btn ghost xs" data-mact="desc-more">Leer todo</button></div>` : ""}` : `<div class="note">Sin descripción.</div>`) : `<div class="note">Cargando…</div>`}</div>
      <div class="d-sec"><div class="sec-h"><span class="label">Comentarios</span><span class="mini">${d && d.commentsN ? nf0.format(d.commentsN) : ""}</span></div>${comments}${d && d.commentsN ? `<div class="sec-foot"><button class="btn ghost xs" data-mact="mod-web">${ic("globe")}Ver todos en su página</button></div>` : ""}</div>
      ${m.tags && m.tags.length ? `<div class="d-sec"><div class="sec-h"><span class="label">Etiquetas del Workshop</span></div><div class="g-tags">${m.tags.map((t) => `<span class="chip tag clickable" data-wtag="${esc(t)}" title="Ver solo los mods con esta etiqueta">${esc(t)}</span>`).join("")}</div></div>` : ""}`;
  }

  // ---------- visor de imágenes y vídeos ----------
  let lb = null; // { items, i }
  function openLightbox(items, i) {
    lb = { items, i };
    drawLightbox();
    $("#lightbox").hidden = false;
  }
  function drawLightbox() {
    if (!lb) return;
    const it = lb.items[lb.i], n = lb.items.length;
    $("#lb-stage").innerHTML = it.video
      ? `<iframe src="https://www.youtube-nocookie.com/embed/${encodeURIComponent(it.video)}?autoplay=1&rel=0" title="Vídeo" allow="autoplay; encrypted-media; picture-in-picture; fullscreen" allowfullscreen></iframe>`
      : `<img src="${esc(it.url)}" alt="" draggable="false">`;
    $("#lb-count").textContent = `${lb.i + 1} de ${n}`;
    $("#lb-open").textContent = it.video ? "Abrir en YouTube" : "Abrir la imagen";
    $("#lb-prev").hidden = $("#lb-next").hidden = n < 2;
  }
  function closeLightbox() { $("#lightbox").hidden = true; $("#lb-stage").innerHTML = ""; lb = null; }
  const lbMove = (d) => { if (!lb) return; lb.i = (lb.i + d + lb.items.length) % lb.items.length; drawLightbox(); };
  $("#lb-prev").addEventListener("click", () => lbMove(-1));
  $("#lb-next").addEventListener("click", () => lbMove(1));
  $("#lb-close").addEventListener("click", closeLightbox);
  $("#lightbox").addEventListener("mousedown", (e) => { if (e.target.id === "lightbox" || e.target.id === "lb-stage") closeLightbox(); });
  $("#lb-open").addEventListener("click", () => { if (!lb) return; const it = lb.items[lb.i]; rt.BrowserOpenURL(it.video ? "https://www.youtube.com/watch?v=" + encodeURIComponent(it.video) : it.url); });
  document.addEventListener("keydown", (e) => {
    if (!lb) return;
    if (e.key === "Escape") { e.preventDefault(); e.stopImmediatePropagation(); closeLightbox(); }
    else if (e.key === "ArrowLeft") { e.preventDefault(); e.stopImmediatePropagation(); lbMove(-1); }
    else if (e.key === "ArrowRight") { e.preventDefault(); e.stopImmediatePropagation(); lbMove(1); }
  }, true);

  // ---------- ficha de un preset ----------
  async function loadPresetMods(p) {
    const ids = p.mods.map((m) => m.id).filter((id) => !MS.pmods.has(id));
    if (!ids.length) return;
    try {
      const list = await api.ModDetails(p.appid, ids);
      for (const m of list || []) MS.pmods.set(m.id, m);
    } catch { }
    if (MS.sel && MS.sel.kind === "preset" && MS.sel.id === p.id) renderModDetail();
  }
  function renderPresetDetail(box) {
    const p = presetById(MS.sel.id);
    if (!p) { MS.sel = null; renderModDetail(); return; }
    if (p.mods.some((m) => !MS.pmods.has(m.id))) loadPresetMods(p);
    const subs = new Set(subscribedIds());
    const have = p.mods.filter((m) => subs.has(m.id)).length;
    const size = p.mods.reduce((s, m) => s + ((MS.pmods.get(m.id) || {}).size || 0), 0);
    const owned = !(MS.list && MS.list.noGame);
    const missingMods = p.mods.filter((m) => (MS.pmods.get(m.id) || {}).missing).length;
    box.innerHTML = `
      <div class="d-title" style="padding-top:20px">
        <h2>${esc(p.name)}</h2>
        <div class="by">${esc(p.game || "")} · ${plural(p.mods.length, "mod", "mods")}${p.author ? " · hecho por <b>" + esc(p.author) + "</b>" : ""}</div>
        <div class="d-chips">${p.auto ? `<span class="chip off">${ic("clock")}Copia automática</span>` : p.from ? `<span class="chip from">${ic("user")}Te lo pasó ${esc(p.from)}</span>` : `<span class="chip">${ic("star")}Tuyo</span>`}${(p.tags || []).map((t) => `<span class="chip tag">#${esc(t)}</span>`).join("")}</div>
        <div class="d-actions"><button class="btn go sm" data-pact="${esc(p.id)}"${owned ? "" : " disabled"}>${ic("play")}Activar</button><button class="btn ghost sm" data-padd="${esc(p.id)}"${owned ? "" : " disabled"} title="Suscribirte a los que faltan sin quitar nada">${ic("plus")}Añadir</button><button class="ibtn" data-pcopy="${esc(p.id)}" title="Copiar para mandarlo por un chat">${ic("copy")}</button><button class="ibtn" data-pmenu="${esc(p.id)}" title="Más opciones">${ic("more")}</button></div>
      </div>
      <div class="d-sec"><div class="facts">
        <div class="fact"><b>${have} / ${p.mods.length}</b><span>ya suscritos</span></div>
        <div class="fact"><b>${size ? fmtBytes(size) : "—"}</b><span>tamaño de todos</span></div>
        <div class="fact"><b>${esc(fmtDate(p.created))}</b><span>${p.from ? "lo recibiste" : "creado"}</span></div>
        <div class="fact"><b>${missingMods || "0"}</b><span>ya no existen</span></div>
      </div></div>
      <div class="d-sec" id="pd-card">
        <div class="sec-h"><span class="label">Ficha</span><span class="saved" id="pd-saved">${ic("check")} Guardado</span></div>
        <div class="alias-row"><input class="inp" id="pd-name" maxlength="80" value="${esc(p.name)}" placeholder="Nombre" spellcheck="false"></div>
        <div class="field-row"><label class="field"><span>Lo hizo</span><input id="pd-author" maxlength="64" value="${esc(p.author)}" placeholder="quien lo montó" spellcheck="false"></label><label class="field"><span>Te lo pasó</span><input id="pd-from" maxlength="64" value="${esc(p.from)}" placeholder="vacío si es tuyo" spellcheck="false"></label></div>
        <div class="tags">${(p.tags || []).map((t) => `<span class="chip tag">#${esc(t)}<button data-puntag="${esc(t)}" title="Quitar">${ic("x")}</button></span>`).join("")}<input class="tag-add" id="pd-tag" maxlength="24" placeholder="+ etiqueta" spellcheck="false" title="Escribe y pulsa Intro"></div>
        <textarea class="inp note-area" id="pd-notes" maxlength="4000" placeholder="Notas: para qué es, en qué servidor se usa…" spellcheck="false">${esc(p.notes)}</textarea>
      </div>
      <div class="d-sec"><div class="sec-h"><span class="label">Mods</span><span class="mini">${p.mods.length}</span></div>
        ${p.mods.map((x) => { const m = MS.pmods.get(x.id) || { id: x.id, title: x.title }; const ok = subs.has(x.id); return `<div class="ritem" data-rmod="${x.id}">${thumbMod(m)}<div style="min-width:0"><div class="rn">${esc(m.title || x.title || "Mod " + x.id)}</div><div class="rs${ok ? " ok" : ""}">${ok ? "Suscrito" : m.missing ? "Ya no existe" : "Te falta"}${m.size ? " · " + fmtBytes(m.size) : ""}</div></div><button class="ibtn danger" data-prm="${x.id}" title="Quitar del preset">${ic("x")}</button></div>`; }).join("")}
      </div>
      <div class="dfoot"><button class="btn ghost xs" data-pcopy="${esc(p.id)}">${ic("copy")}Copiar texto</button><button class="btn ghost xs" data-pcode="${esc(p.id)}">${ic("share")}Copiar solo el código</button><button class="btn ghost xs" data-pexport="${esc(p.id)}">${ic("file")}Guardar como archivo</button><button class="btn ghost xs danger-txt" data-pdel="${esc(p.id)}">${ic("trash")}Borrar</button></div>`;
  }

  const pSavedFlash = debounce(() => { const s = $("#pd-saved"); if (s) s.classList.remove("on"); }, 1400);
  async function patchPreset(p, patch, rerender) {
    try {
      const saved = await api.SavePreset({ ...p, ...patch });
      const i = MS.presets.findIndex((x) => x.id === p.id);
      if (i >= 0) MS.presets[i] = saved;
      const s = $("#pd-saved"); if (s) { s.classList.add("on"); pSavedFlash(); }
      if (rerender) renderMods(); else { renderModList(); renderModTop(); }
      return saved;
    } catch (e) { fail(e); return null; }
  }

  // ---------- etiquetas tuyas en los mods ----------
  async function tagMods(ids, add, remove) {
    if (!ids.length) return;
    try {
      const r = await api.SetModTags(ids, add || [], remove || []);
      for (const [k, tags] of Object.entries(r || {})) {
        const id = +k;
        for (const m of [...(MS.list ? MS.list.mods : []), ...((MS.list && MS.list.history) || []), ...MS.search.mods]) if (m.id === id) m.myTags = tags || [];
        if (MS.selMod && MS.selMod.id === id) MS.selMod.myTags = tags || [];
      }
      renderMods();
      return true;
    } catch (e) { fail(e); return false; }
  }

  // ---------- cambiar a una cuenta que tenga el juego ----------
  // Devuelve true si al final está dentro una cuenta que lo tiene.
  async function switchToOwner(a, then) {
    const g = modGame();
    if (!a) { toast("Ninguna de tus cuentas guardadas tiene este juego", "error"); return false; }
    await switchTo(a, { forceAsk: true, note: `La cuenta abierta en Steam no tiene <b>${esc(g ? g.name : "este juego")}</b>; <b>${esc(display(a))}</b> sí.${then ? " Cuando entre, se seguirá con los mods." : ""}` });
    await loadAccounts();
    const now = activeAcc();
    if (!now || now.key !== a.key) return false;
    MS.connect.add(MS.app);
    MS.info.clear();
    await loadMods({ live: true });
    if (then) await then();
    return true;
  }
  function ownerMenu(x, y, then) {
    const own = ownersOf(MS.app);
    showMenu([{ head: "Cambiar a…" }, ...own.map((a) => ({ html: `${avatarHTML(a, "sm", false)}<span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis">${esc(display(a))}</span>`, fn: () => switchToOwner(a, then) }))], x, y);
  }

  // ---------- suscribirse, quitar y activar ----------
  function showProgress(total, title) {
    $("#mg-title").textContent = title || "Cambiando mods…";
    $("#mg-text").textContent = "Preparando…";
    $("#mg-bar").style.width = "0%";
    $("#mg-count").textContent = `0 de ${total}`;
    $("#mg-stop").disabled = false;
    $("#mg-stop").textContent = "Parar";
    $("#m-progress").hidden = false;
  }
  rt.EventsOn("mods-progress", (p) => {
    if ($("#m-progress").hidden || !p) return;
    $("#mg-bar").style.width = Math.round((p.done / Math.max(1, p.total)) * 100) + "%";
    $("#mg-count").textContent = `${p.done} de ${p.total}`;
    if (p.title) $("#mg-text").innerHTML = `${p.op === "subscribe" ? "Suscribiendo a" : "Quitando"} <b>${esc(p.title)}</b>`;
  });
  $("#mg-stop").addEventListener("click", () => { api.ModsCancel(); $("#mg-stop").disabled = true; $("#mg-stop").textContent = "Parando…"; });

  async function changeMods(sub, unsub, title) {
    const app = MS.app;
    if (MS.busy) { toast("Ya se están cambiando mods", "error"); return; }
    MS.busy = true;
    const total = sub.length + unsub.length;
    if (total > 2) showProgress(total, title);
    let notOwned = false;
    try {
      const r = await api.ModsChange(app, sub, unsub);
      const parts = [];
      if (r.subscribed) parts.push(plural(r.subscribed, "mod suscrito", "mods suscritos"));
      if (r.unsubscribed) parts.push(plural(r.unsubscribed, "suscripción quitada", "suscripciones quitadas"));
      if (r.canceled) parts.push("parado a medias");
      if (parts.length) toast(parts.join(" · ") + (r.backup ? " · copia guardada en Presets" : ""));
      if (r.failed && r.failed.length) toast(`${plural(r.failed.length, "mod no se ha", "mods no se han")} podido cambiar. ${r.failed[0].title}: ${r.failed[0].error}`, "error");
      if (r.backup) loadPresets(true);
      for (const m of MS.search.mods) { if (sub.includes(m.id)) m.subscribed = true; if (unsub.includes(m.id)) m.subscribed = false; }
    } catch (e) {
      const msg = errText(e);
      MS.busy = false;
      $("#m-progress").hidden = true;
      if (notOwnedErr(msg)) {
        // la cuenta abierta no tiene el juego: se ofrece cambiar a una que sí y seguir
        notOwned = true;
        const g = modGame(), own = ownersOf(app);
        if (!own.length) toast(`Ninguna de tus cuentas guardadas tiene ${g ? g.name : "este juego"}`, "error");
        else {
          const r = await ask({ title: "Hay que cambiar de cuenta", html: `<p>La cuenta abierta en Steam no tiene <b>${esc(g ? g.name : "este juego")}</b>. <b>${esc(display(own[0]))}</b> sí lo tiene.</p><p>Steam se cerrará un momento, entrará con ella y después se hará lo que has pedido.</p>`, ok: `Cambiar a ${display(own[0])} y seguir` });
          if (r.ok) await switchToOwner(own[0], () => changeMods(sub, unsub, title));
        }
      } else if (total === 1) {
        const r = await ask({ title: "No se ha podido hacer desde aquí", html: `<p>${esc(msg)}.</p><p>Puedes hacerlo en la página del mod dentro de Steam.</p>`, ok: "Abrir en Steam" });
        if (r.ok) api.OpenModInSteam(sub[0] || unsub[0]).catch(fail);
      } else toast(msg, "error");
    } finally {
      MS.busy = false;
      $("#m-progress").hidden = true;
    }
    if (notOwned) return;
    MS.connect.add(app);
    MS.info.clear();
    await loadMods({ live: true });
  }

  async function subscribeIds(ids) {
    ids = ids.filter((id) => !(modById(id) || {}).subscribed);
    if (!ids.length) { toast("Ya estás suscrito"); return; }
    if (ids.length > 3) {
      const size = ids.reduce((s, id) => s + ((modById(id) || {}).size || 0), 0);
      const r = await ask({ title: `¿Suscribirse a ${ids.length} mods?`, html: `<p>Steam los bajará${size ? ` (unos <b>${fmtBytes(size)}</b>)` : ""} ${modGame() && modGame().installed ? "en cuanto pueda" : "cuando instales el juego"}. Van varios a la vez.</p>`, ok: "Suscribirse" });
      if (!r.ok) return;
    }
    clearPicks();
    changeMods(ids, [], "Suscribiendo a " + plural(ids.length, "mod", "mods"));
  }
  async function unsubscribeIds(ids, all) {
    ids = ids.filter((id) => (modById(id) || { subscribed: true }).subscribed);
    if (!ids.length) return;
    const one = ids.length === 1 && modById(ids[0]);
    const r = await ask({
      title: all ? "¿Quitar todas las suscripciones?" : one ? `¿Quitar ${one.title}?` : `¿Quitar ${ids.length} suscripciones?`,
      html: `<p>${all ? `Te quitarás de los <b>${ids.length}</b> mods de este juego.` : one ? "Te quitarás de este mod." : `Te quitarás de <b>${ids.length}</b> mods.`} Steam borrará sus archivos del disco.</p><p>Antes se guarda una <b>copia automática</b> en Presets con lo que tienes ahora, y quedan en «Los que tuve antes» para volver a ponerlos.</p>`,
      ok: all ? "Quitar todo" : "Quitar", danger: true,
    });
    if (!r.ok) return;
    clearPicks();
    changeMods([], ids, all ? "Quitando todo…" : "Quitando suscripciones…");
  }

  async function activatePresets(list, exclusive) {
    if (!MS.list || MS.list.noGame) { toast("Necesitas tener el juego para activar sus mods", "error"); return; }
    if (!MS.list.live) {
      MS.connect.add(MS.app);
      await loadMods({ live: true });
      if (!MS.list.live) {
        if (notOwnedErr(MS.list.liveError) && ownersOf(MS.app).length) {
          const own = ownersOf(MS.app)[0];
          const r = await ask({ title: "Hay que cambiar de cuenta", html: `<p>La cuenta abierta en Steam no tiene este juego. <b>${esc(display(own))}</b> sí.</p><p>Steam se cerrará un momento, entrará con ella y después se activará el preset.</p>`, ok: `Cambiar a ${display(own)} y seguir` });
          if (r.ok) await switchToOwner(own, () => activatePresets(list, exclusive));
          return;
        }
        toast((MS.list.liveError || "No se puede conectar con Steam") + ". Sin eso no se sabe a qué estás suscrito.", "error");
        return;
      }
    }
    const want = new Set(list.flatMap((p) => p.mods.map((m) => m.id)));
    const subs = subscribedIds();
    const toSub = [...want].filter((id) => !subs.includes(id));
    const toUnsub = exclusive ? subs.filter((id) => !want.has(id)) : [];
    if (!toSub.length && !toUnsub.length) { toast(exclusive ? "Ya tienes justo esos mods" : "Ya tienes todos sus mods"); return; }
    const g = modGame();
    const name = list.length === 1 ? `«${list[0].name}»` : `${list.length} presets juntos`;
    const html = `<div class="plan"><div class="fact"><b class="up">+${toSub.length}</b><span>suscribirse</span></div><div class="fact"><b class="down">−${toUnsub.length}</b><span>quitar</span></div><div class="fact"><b>${want.size - toSub.length}</b><span>ya los tienes</span></div></div>
      ${toUnsub.length ? "<p>Antes de quitar nada se guarda una <b>copia automática</b> en Presets con lo que tienes ahora.</p>" : ""}
      <p>Steam bajará los nuevos ${g && g.installed ? "en cuanto pueda" : "cuando instales el juego"}${toUnsub.length ? " y borrará del disco los que quites" : ""}. Van varios a la vez: con cientos de mods tarda poco.</p>`;
    const r = await ask({ title: `¿Activar ${name}?`, html, ok: exclusive ? "Activar" : "Añadir", alt: exclusive && toUnsub.length && toSub.length ? "Solo añadir, sin quitar nada" : "" });
    if (r.alt) { changeMods(toSub, [], "Activando " + name); return; }
    if (r.ok) changeMods(toSub, toUnsub, "Activando " + name);
  }

  // ---------- crear, pegar y guardar presets ----------
  let presetDraft = null;
  function openPresetModal(draft, mode) {
    presetDraft = { ...draft, mode, preset: { ...draft.preset, mods: [...draft.preset.mods] } };
    const p = presetDraft.preset;
    $("#mpr-title").textContent = mode === "import" ? "Preset recibido" : "Nuevo preset";
    $("#mpr-name").value = p.name || "";
    $("#mpr-author").value = p.author || "";
    $("#mpr-from").value = mode === "import" ? (p.from || p.author || "") : (p.from || "");
    $("#mpr-tags").value = (p.tags || []).join(", ");
    $("#mpr-notes").value = p.notes || "";
    drawPresetDraft();
    $("#m-preset").hidden = false;
    setTimeout(() => $("#mpr-name").focus(), 50);
  }
  function drawPresetDraft() {
    const d = presetDraft, p = d.preset;
    const known = S.games && S.games.games.some((g) => g.appid === p.appid);
    $("#mpr-sum").innerHTML = `<div class="sum-box"><div class="tile">${ic("layers")}</div><div><b>${esc(p.game || "Juego " + p.appid)}</b>${plural(p.mods.length, "mod", "mods")}${d.size ? " · " + fmtBytes(d.size) : ""}${known ? "" : " · no tienes este juego en este PC"}</div></div>` +
      (d.warnings || []).map((w) => `<div class="warn-box">${esc(w)}</div>`).join("") +
      (d.mode === "import" ? `<p class="lead" style="margin:0 0 12px">Guárdalo para tenerlo a mano. Puedes poner quién te lo ha pasado y etiquetas para encontrarlo luego. Activarlo es otro paso: no se cambia nada todavía.</p>` : "");
    $("#mpr-mods").innerHTML = `<div class="field"><span>Mods</span><div class="prev-list">${p.mods.slice(0, 400).map((m) => `<div><span>${esc(m.title || "Mod " + m.id)}</span><small class="mono">${m.id}</small><button type="button" class="ibtn danger" data-drm="${m.id}" title="Quitar">${ic("x")}</button></div>`).join("")}${p.mods.length > 400 ? `<div><span>… y ${p.mods.length - 400} más</span></div>` : ""}</div></div>`;
  }
  $("#mpr-mods").addEventListener("click", (e) => {
    const b = e.target.closest("[data-drm]");
    if (!b || !presetDraft) return;
    presetDraft.preset.mods = presetDraft.preset.mods.filter((m) => m.id !== +b.dataset.drm);
    drawPresetDraft();
  });
  $("#mpr-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const d = presetDraft;
    if (!d) return;
    const p = { ...d.preset, name: $("#mpr-name").value.trim(), author: $("#mpr-author").value.trim(), from: $("#mpr-from").value.trim(), notes: $("#mpr-notes").value.trim(), tags: $("#mpr-tags").value.split(/[,\s]+/).map((t) => t.replace(/^#+/, "").trim()).filter(Boolean) };
    if (!p.name) { fieldErr($("#mpr-name"), "Ponle un nombre"); shake($("#m-preset")); return; }
    const btn = $("#mpr-go");
    btn.classList.add("busy");
    try {
      const saved = await api.SavePreset(p);
      $("#m-preset").hidden = true;
      presetDraft = null;
      await loadPresets(true);
      toast(d.mode === "import" ? `Preset «${saved.name}» guardado` : "Preset guardado");
      openModGame(saved.appid, "presets");
      MS.sel = { kind: "preset", id: saved.id };
      setModTab("presets");
    } catch (err) { fieldErr($("#mpr-name"), errText(err)); shake($("#m-preset")); }
    finally { btn.classList.remove("busy"); }
  });

  function newPresetFrom(ids, name) {
    if (!ids.length) return;
    const g = modGame();
    const mods = ids.map((id) => ({ id, title: (modById(id) || {}).title || "" }));
    const size = ids.reduce((s, id) => s + ((modById(id) || {}).size || 0), 0);
    const act = activeAcc();
    openPresetModal({ preset: { appid: MS.app, game: g ? g.name : "", name: name || "", author: act ? display(act) : "", from: "", tags: [], notes: "", mods, source: "propio" }, warnings: [], size }, "new");
  }

  function openPaste() {
    $("#mp-text").value = "";
    $("#mp-err").textContent = "";
    $("#m-paste").hidden = false;
    setTimeout(() => $("#mp-text").focus(), 50);
  }
  async function readPaste(fromFile) {
    const btn = fromFile ? $("#mp-file") : $("#mp-go");
    btn.classList.add("busy");
    try {
      const d = fromFile ? await api.ImportPresetFile() : await api.ParsePreset($("#mp-text").value);
      if (!d || !d.preset) return; // archivo no elegido
      $("#m-paste").hidden = true;
      openPresetModal(d, "import");
    } catch (e) {
      $("#mp-err").textContent = errText(e);
      $("#mp-err").classList.add("bad");
      $("#m-paste").hidden = false;
      shake($("#m-paste"));
    } finally { btn.classList.remove("busy"); }
  }
  $("#mp-go").addEventListener("click", () => readPaste(false));
  $("#mp-file").addEventListener("click", () => readPaste(true));
  $("#mp-text").addEventListener("keydown", (e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); readPaste(false); } });

  async function copyPreset(p, full) {
    try {
      const t = await api.PresetText(p.id, full);
      await api.CopyText(t);
      if (full && t.length > 1900) toast("Preset copiado. Es largo: si el chat no lo deja, usa «Copiar solo el código» o «Guardar como archivo»");
      else toast(full ? "Preset copiado: pégalo en el chat" : "Código copiado: pégalo en el chat");
    } catch (e) { fail(e); }
  }

  function presetMenu(p, x, y) {
    const noGame = MS.list && MS.list.noGame;
    showMenu([
      { head: p.name },
      { icon: "play", label: "Activar (suscribirse y quitar los demás)", fn: () => activatePresets([p], true), disabled: noGame },
      { icon: "plus", label: "Añadir (solo suscribirse a los que faltan)", fn: () => activatePresets([p], false), disabled: noGame },
      "-",
      { icon: "copy", label: "Copiar el texto para un chat", fn: () => copyPreset(p, true) },
      { icon: "share", label: "Copiar solo el código", fn: () => copyPreset(p, false) },
      { icon: "file", label: "Guardar como archivo…", fn: () => api.ExportPresetFile(p.id).then((ok) => { if (ok) toast("Preset guardado"); }).catch(fail) },
      "-",
      { icon: "edit", label: "Ver y editar", fn: () => { MS.sel = { kind: "preset", id: p.id }; renderMods(); } },
      { icon: "layers", label: "Duplicar", fn: async () => { try { const s = await api.SavePreset({ ...p, id: "", auto: false, name: p.name + " (copia)" }); await loadPresets(true); MS.sel = { kind: "preset", id: s.id }; renderMods(); } catch (e) { fail(e); } } },
      { icon: "trash", label: "Borrar…", danger: true, fn: () => deletePreset(p) },
    ], x, y);
  }
  async function deletePreset(p) {
    if (!(await confirmBox("¿Borrar el preset?", `<p>Se borrará <b>${esc(p.name)}</b>. Tus suscripciones no cambian.</p>`, "Borrar", true))) return;
    try {
      await api.DeletePreset(p.id);
      if (MS.sel && MS.sel.id === p.id) MS.sel = null;
      MS.pchecked.delete(p.id);
      await loadPresets();
      toast("Preset borrado");
    } catch (e) { fail(e); }
  }

  function modMenu(m, x, y) {
    const ps = gamePresets().filter((p) => !p.auto);
    showMenu([
      { head: m.title },
      m.subscribed ? { icon: "minus", label: "Quitar la suscripción", danger: true, fn: () => unsubscribeIds([m.id]) } : { icon: "plus", label: m.removed ? "Volver a suscribirse" : "Suscribirse", fn: () => subscribeIds([m.id]) },
      { icon: "open", label: "Abrir en Steam", fn: () => api.OpenModInSteam(m.id).catch(fail) },
      { icon: "globe", label: "Abrir en el navegador", fn: () => rt.BrowserOpenURL("https://steamcommunity.com/sharedfiles/filedetails/?id=" + m.id) },
      m.installed && m.dir ? { icon: "folder", label: "Abrir su carpeta", fn: () => api.OpenFolder(m.dir).catch(fail) } : null,
      { icon: "copy", label: "Copiar el enlace", fn: () => api.CopyText("https://steamcommunity.com/sharedfiles/filedetails/?id=" + m.id).then(() => toast("Enlace copiado")) },
      "-",
      ps.length ? { icon: "layers", label: "Añadir a un preset…", fn: () => setTimeout(() => showMenu([{ head: "Añadir a…" }, ...ps.map((p) => ({ icon: p.mods.some((x) => x.id === m.id) ? "check" : "layers", label: p.name, disabled: p.mods.some((x) => x.id === m.id), fn: () => patchPreset(p, { mods: [...p.mods, { id: m.id, title: m.title }] }).then((s) => { if (s) toast(`Añadido a «${s.name}»`); }) }))], x, y), 0) } : null,
      { icon: "save", label: "Nuevo preset con este mod", fn: () => newPresetFrom([m.id], "") },
    ].filter(Boolean), x, y);
  }

  // ---------- eventos de la pestaña ----------
  $("#mod-game").addEventListener("change", (e) => openModGame(+e.target.value));
  $("#mod-back").addEventListener("click", goModHome);
  $("#mod-tab").addEventListener("click", (e) => { const b = e.target.closest("button"); if (b) setModTab(b.dataset.v); });
  {
    const inp = $("#mod-q");
    const r = debounce(() => { if (MS.home || MS.tab !== "search") { renderModList(); renderModBar(); } }, 60);
    inp.addEventListener("input", () => { MS.q = inp.value; inp.parentElement.classList.toggle("has", !!inp.value); r(); });
    inp.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !MS.home && MS.tab === "search") { e.preventDefault(); runSearch(); }
      if (e.key === "Escape" && inp.value) { e.stopPropagation(); clearModQ(); if (!MS.home && MS.tab === "search") { if (MS.search.q) runSearch(); } else { renderModList(); renderModBar(); } }
    });
  }
  // selects del orden
  $("#mod-sortbox").addEventListener("change", (e) => {
    const id = e.target.id, v = e.target.value;
    if (id === "ms-sort") { MS.search.sort = v; MS.search.userSort = true; store.set("modSort", v); runSearch(); }
    else if (id === "ms-days") { MS.search.days = +v; store.set("modDays", +v); runSearch(); }
    else if (id === "mf-sort") { MS.f.sort = v; store.set("modListSort", v); renderModBar(); renderModList(); }
    else if (id === "mh-sort") { MS.h.sort = v; store.set("modHomeSort", v); renderModBar(); renderModList(); }
    else if (id === "mp-sort") { MS.p.sort = v; store.set("modPresetSort", v); renderModBar(); renderModList(); }
  });
  $("#mod-fbtn").addEventListener("click", () => { MS.fopen = !MS.fopen; store.set("modFiltersOpen", MS.fopen); renderModBar(); });
  // etiqueta a los elegidos, y etiqueta del Workshop escrita a mano en la búsqueda
  document.addEventListener("keydown", (e) => {
    const id = e.target && e.target.id;
    if ((id !== "mod-bulk-tag" && id !== "ms-tagin") || (e.key !== "Enter" && e.key !== ",")) return;
    e.preventDefault();
    const t = e.target.value.replace(/^#+/, "").trim();
    if (!t) return;
    if (id === "ms-tagin") { if (!MS.search.tags.includes(t)) MS.search.tags.push(t); runSearch(); return; }
    if (/[\s,#]/.test(t)) { toast("Sin espacios, comas ni #: usa una palabra o guiones", "error"); return; }
    const n = MS.checked.size;
    tagMods([...MS.checked], [t], []).then((ok) => { if (ok) toast(`#${t} puesta a ${plural(n, "mod", "mods")}`); });
  });

  async function modAction(act, el) {
    const m = MS.sel && MS.sel.kind === "mod" ? modById(MS.sel.id) : null;
    switch (act) {
      case "connect": MS.connect.add(MS.app); loadMods({ live: true }); break;
      case "switch-owner": switchToOwner(ownersOf(MS.app)[0]); break;
      case "switch-owner-menu": { const r = el.getBoundingClientRect(); ownerMenu(r.left, r.bottom + 4); break; }
      case "reload": MS.info.clear(); loadMods(); break;
      case "paste": openPaste(); break;
      case "import": readPaste(true); break;
      case "save-all": newPresetFrom(subscribedIds(), ""); break;
      case "save-checked": newPresetFrom([...MS.checked], ""); break;
      case "new-preset": newPresetFrom(subscribedIds(), ""); break;
      case "sub-checked": subscribeIds([...MS.checked]); break;
      case "unsub-checked": unsubscribeIds([...MS.checked]); break;
      case "uncheck": clearPicks(); renderModBar(); renderModList(); break;
      case "pick-list": MS.pickOpen = !MS.pickOpen; renderBulk(); if (MS.pickOpen) setTimeout(() => { const i = $("#pick-q"); if (i) i.focus(); }, 30); break;
      case "check-all": {
        const vis = visibleMods();
        const all = vis.length && vis.every((x) => MS.checked.has(x.id));
        for (const x of vis) pick(x.id, !all);
        renderModBar(); renderModList();
        break;
      }
      case "bulk-untag": {
        const ids = [...MS.checked];
        const tags = [...new Set(ids.flatMap((id) => (modById(id) || {}).myTags || []))];
        if (!tags.length) { toast("Los elegidos no tienen etiquetas tuyas"); break; }
        const r = el.getBoundingClientRect();
        showMenu([{ head: "Quitar a los elegidos…" }, ...tags.map((t) => ({ icon: "x", label: "#" + t, fn: () => tagMods(ids, [], [t]) }))], r.left - 160, r.bottom + 4);
        break;
      }
      case "clear-filters": clearFilters(); break;
      case "close-filters": MS.fopen = false; store.set("modFiltersOpen", false); renderModBar(); break;
      case "all-tags": { const k = el.dataset.k; if (MS.allTags.has(k)) MS.allTags.delete(k); else MS.allTags.add(k); renderModBar(); break; }
      case "subs-menu": {
        const r = el.getBoundingClientRect(), subs = subscribedIds();
        showMenu([{ head: "Mods de este juego" },
          { icon: "save", label: "Guardar lo suscrito como preset", fn: () => newPresetFrom(subscribedIds(), ""), disabled: !subs.length },
          { icon: "paste", label: "Pegar un preset…", fn: openPaste },
          { icon: "file", label: "Abrir un archivo de preset…", fn: () => readPaste(true) },
          { icon: "refresh", label: "Volver a leer", fn: () => { MS.info.clear(); loadMods(); } },
          "-",
          { icon: "trash", label: `Quitar todas las suscripciones (${subs.length})…`, danger: true, fn: () => unsubscribeIds(subscribedIds(), true), disabled: !subs.length },
        ], r.right - 260, r.bottom + 4);
        break;
      }
      case "search-menu": {
        const r = el.getBoundingClientRect();
        showMenu([{ head: "Workshop" },
          { icon: "globe", label: "Abrir el Workshop en el navegador", fn: () => rt.BrowserOpenURL("https://steamcommunity.com/app/" + MS.app + "/workshop/") },
          { icon: "refresh", label: "Volver a buscar", fn: () => runSearch() },
        ], r.right - 270, r.bottom + 4);
        break;
      }
      case "new-menu": {
        const r = el.getBoundingClientRect();
        showMenu([{ head: "Nuevo preset" },
          { icon: "save", label: "Con lo que tengo suscrito", fn: () => newPresetFrom(subscribedIds(), ""), disabled: !subscribedIds().length },
          { icon: "paste", label: "Pegar uno que me han pasado…", fn: openPaste },
          { icon: "file", label: "Abrir un archivo (.txt o del lanzador de Arma 3)…", fn: () => readPaste(true) },
        ], r.right - 300, r.bottom + 4);
        break;
      }
      case "unsub-all": unsubscribeIds(subscribedIds(), true); break;
      case "activate-checked": activatePresets([...MS.pchecked].map(presetById).filter(Boolean), true); break;
      case "puncheck": MS.pchecked.clear(); renderModBar(); renderModList(); break;
      case "tab-search": setModTab("search"); break;
      case "tab-presets": setModTab("presets"); break;
      case "search-more": runSearch(true); break;
      case "search-retry": runSearch(); break;
      case "search-clear": clearModQ(); runSearch(); break;
      case "workshop-web": rt.BrowserOpenURL("https://steamcommunity.com/app/" + MS.app + "/workshop/"); break;
      case "mod-steam": if (m) api.OpenModInSteam(m.id).catch(fail); break;
      case "mod-web": if (m) rt.BrowserOpenURL("https://steamcommunity.com/sharedfiles/filedetails/?id=" + m.id); break;
      case "mod-folder": if (m && m.dir) api.OpenFolder(m.dir).catch(fail); break;
      case "desc-more": { const d = $("#md-desc"); if (d) { d.classList.add("open"); el.remove(); } break; }
      case "sub-req": {
        const d = m && MS.info.get(m.id);
        if (d && d.required) { const subs = new Set(subscribedIds()); subscribeIds(d.required.filter((r) => !subs.has(r.id) && !r.missing).map((r) => r.id)); }
        break;
      }
    }
  }

  let lastCheck = null; // para elegir varios seguidos con Mayús
  function modClick(e) {
    const t = e.target;
    const url = t.closest("[data-url]");
    if (url) { e.preventDefault(); rt.BrowserOpenURL(url.dataset.url); return; }
    const hg = t.closest("[data-mgame]");
    if (hg) { openModGame(+hg.dataset.mgame); return; }
    const hp = t.closest("[data-home-preset]");
    if (hp) { const p = presetById(hp.dataset.homePreset); if (p) { openModGame(p.appid, "presets"); MS.sel = { kind: "preset", id: p.id }; setModTab("presets"); } return; }
    const fc = t.closest("[data-fk]");
    if (fc) { onFilterChip(fc.dataset.fk, fc.dataset.fv); return; }
    const rf = t.closest("[data-rmf]");
    if (rf) { removeFilter(rf.dataset.rmf); return; }
    const gal = t.closest("[data-gal]");
    if (gal) { const d = MS.sel && MS.info.get(MS.sel.id); if (d && d.gallery && d.gallery.length) openLightbox(d.gallery, +gal.dataset.gal); return; }
    const ck = t.closest("[data-check]");
    if (ck) {
      const id = +ck.dataset.check;
      const vis = visibleMods().map((m) => m.id);
      if (e.shiftKey && lastCheck != null && vis.includes(lastCheck)) {
        const [a, b] = [vis.indexOf(lastCheck), vis.indexOf(id)].sort((x, y) => x - y);
        for (const x of vis.slice(a, b + 1)) pick(x, ck.checked);
      } else pick(id, ck.checked);
      lastCheck = id;
      renderModBar();
      if (e.shiftKey) renderModList(); else { const el = ck.closest("[data-mod]"); if (el) el.classList.toggle("checked", ck.checked); }
      return;
    }
    const unp = t.closest("[data-unpick]");
    if (unp) { pick(+unp.dataset.unpick, false); renderBulk(); renderModList(); if (MS.checked.size) { renderModBar(); } else renderModBar(); return; }
    const vw = t.closest("#mod-views [data-v]");
    if (vw) { MS.view = vw.dataset.v; store.set("modView", MS.view); renderModBar(); renderModList(); return; }
    const pck = t.closest("[data-pcheck]");
    if (pck) { if (pck.checked) MS.pchecked.add(pck.dataset.pcheck); else MS.pchecked.delete(pck.dataset.pcheck); renderModBar(); return; }
    const stag = t.closest("[data-stag]");
    if (stag) { MS.search.tags = MS.search.tags.filter((x) => x !== stag.dataset.stag); runSearch(); return; }
    const wtag = t.closest("[data-wtag]");
    if (wtag) {
      const tg = wtag.dataset.wtag;
      if (MS.tab === "search") { if (!MS.search.tags.includes(tg)) { MS.search.tags.push(tg); runSearch(); } }
      else { if (!MS.f.wtags.includes(tg)) MS.f.wtags.push(tg); if (MS.f.state === "history") MS.f.state = "all"; renderModBar(); renderModList(); }
      return;
    }
    const mun = t.closest("[data-muntag]");
    if (mun) { const m = MS.sel && modById(MS.sel.id); if (m) tagMods([m.id], [], [mun.dataset.muntag]); return; }
    const sub = t.closest("[data-sub]");
    if (sub) { subscribeIds([+sub.dataset.sub]); return; }
    const uns = t.closest("[data-unsub]");
    if (uns) { unsubscribeIds([+uns.dataset.unsub]); return; }
    const mm = t.closest("[data-mmenu]");
    if (mm) { const m = modById(+mm.dataset.mmenu); const r = mm.getBoundingClientRect(); if (m) modMenu(m, r.left - 210, r.bottom + 4); return; }
    const pm = t.closest("[data-pmenu]");
    if (pm) { const p = presetById(pm.dataset.pmenu); const r = pm.getBoundingClientRect(); if (p) presetMenu(p, r.left - 230, r.bottom + 4); return; }
    const pa = t.closest("[data-pact]");
    if (pa) { const p = presetById(pa.dataset.pact); if (p) activatePresets([p], true); return; }
    const pad = t.closest("[data-padd]");
    if (pad) { const p = presetById(pad.dataset.padd); if (p) activatePresets([p], false); return; }
    const pc = t.closest("[data-pcopy]");
    if (pc) { const p = presetById(pc.dataset.pcopy); if (p) copyPreset(p, true); return; }
    const pcode = t.closest("[data-pcode]");
    if (pcode) { const p = presetById(pcode.dataset.pcode); if (p) copyPreset(p, false); return; }
    const pex = t.closest("[data-pexport]");
    if (pex) { api.ExportPresetFile(pex.dataset.pexport).then((ok) => { if (ok) toast("Preset guardado"); }).catch(fail); return; }
    const pdel = t.closest("[data-pdel]");
    if (pdel) { const p = presetById(pdel.dataset.pdel); if (p) deletePreset(p); return; }
    const prm = t.closest("[data-prm]");
    if (prm) {
      const p = presetById(MS.sel && MS.sel.id);
      if (!p) return;
      if (p.mods.length <= 1) { toast("Un preset necesita al menos un mod: bórralo si ya no lo quieres", "error"); return; }
      patchPreset(p, { mods: p.mods.filter((x) => x.id !== +prm.dataset.prm) }, true);
      return;
    }
    const pun = t.closest("[data-puntag]");
    if (pun) { const p = presetById(MS.sel && MS.sel.id); if (p) patchPreset(p, { tags: (p.tags || []).filter((x) => x !== pun.dataset.puntag) }, true); return; }
    const gp = t.closest("[data-goto-preset]");
    if (gp) { MS.sel = { kind: "preset", id: gp.dataset.gotoPreset }; setModTab("presets"); return; }
    const rm = t.closest("[data-rmod]");
    if (rm) {
      const id = +rm.dataset.rmod;
      const prev = MS.sel && MS.info.get(MS.sel.id);
      const found = modById(id) || (prev && prev.required ? prev.required.find((x) => x.id === id) : null);
      MS.selMod = found || { id, title: "Mod " + id, tags: [], children: [] };
      MS.sel = { kind: "mod", id };
      renderModList(); renderModDetail();
      $("#mod-detail").scrollTop = 0;
      return;
    }
    const act = t.closest("[data-mact]");
    if (act) { modAction(act.dataset.mact, act); return; }
    const row = t.closest("[data-mod]");
    if (row) { MS.sel = { kind: "mod", id: +row.dataset.mod }; MS.selMod = null; $$("#mod-list [data-mod]").forEach((el) => el.classList.toggle("sel", el === row)); renderModDetail(); $("#mod-detail").scrollTop = 0; return; }
    const card = t.closest("[data-preset]");
    if (card) { MS.sel = { kind: "preset", id: card.dataset.preset }; $$("#mod-list .pcard").forEach((el) => el.classList.toggle("sel", el === card)); renderModDetail(); $("#mod-detail").scrollTop = 0; }
  }
  for (const id of ["#mod-list", "#mod-detail", "#mod-live", "#mod-filters", "#mod-active", "#mod-actions", "#mod-bulk", "#mod-sortbox", "#mod-views"]) $(id).addEventListener("click", modClick);
  // buscador de la lista de elegidos
  document.addEventListener("input", (e) => {
    if (e.target.id !== "pick-q") return;
    MS.pickQ = e.target.value;
    const box = $("#pick-items");
    if (box) box.innerHTML = pickItems();
  });
  $("#mod-list").addEventListener("contextmenu", (e) => {
    const row = e.target.closest("[data-mod]"), card = e.target.closest("[data-preset]");
    if (row) { e.preventDefault(); const m = modById(+row.dataset.mod); if (m) modMenu(m, e.clientX, e.clientY); }
    else if (card) { e.preventDefault(); const p = presetById(card.dataset.preset); if (p) presetMenu(p, e.clientX, e.clientY); }
  });
  $("#mod-list").addEventListener("dblclick", (e) => {
    const card = e.target.closest("[data-preset]");
    if (card && !e.target.closest("button,input")) { const p = presetById(card.dataset.preset); if (p) activatePresets([p], true); }
  });
  // fichas: se guardan al salir de cada campo
  $("#mod-detail").addEventListener("change", (e) => {
    const p = MS.sel && MS.sel.kind === "preset" && presetById(MS.sel.id);
    if (!p) return;
    const v = e.target.value.trim();
    if (e.target.id === "pd-name") { if (!v) { e.target.value = p.name; toast("El preset necesita un nombre", "error"); return; } patchPreset(p, { name: v }).then(() => { const h = $("#mod-detail .d-title h2"); if (h) h.textContent = v; }); }
    if (e.target.id === "pd-author") patchPreset(p, { author: v });
    if (e.target.id === "pd-from") patchPreset(p, { from: v });
    if (e.target.id === "pd-notes") patchPreset(p, { notes: v });
  });
  $("#mod-detail").addEventListener("keydown", (e) => {
    if (["pd-name", "pd-author", "pd-from"].includes(e.target.id) && e.key === "Enter") e.target.blur();
    if ((e.target.id === "pd-tag" || e.target.id === "md-tag") && (e.key === "Enter" || e.key === ",")) {
      e.preventDefault();
      const t = e.target.value.replace(/^#+/, "").trim();
      if (!t) return;
      if (/[\s,#]/.test(t)) { toast("Sin espacios, comas ni #: usa una palabra o guiones", "error"); return; }
      if (e.target.id === "md-tag") {
        const m = MS.sel && modById(MS.sel.id);
        if (!m) return;
        if ((m.myTags || []).length >= 8) { toast("Como mucho 8 etiquetas por mod", "error"); return; }
        tagMods([m.id], [t], []).then(() => { const i = $("#md-tag"); if (i) i.focus(); });
        return;
      }
      const p = MS.sel && presetById(MS.sel.id);
      if (!p) return;
      if ((p.tags || []).some((x) => x.toLowerCase() === t.toLowerCase())) { e.target.value = ""; return; }
      if ((p.tags || []).length >= 8) { toast("Como mucho 8 etiquetas", "error"); return; }
      patchPreset(p, { tags: [...(p.tags || []), t] }, true).then(() => { const i = $("#pd-tag"); if (i) i.focus(); });
    }
  });
  // al salir de la ventana (para ir a Steam a jugar, por ejemplo) se suelta la
  // conexión con Steam: mientras está, Steam cree que el juego está abierto
  let dropTimer = null;
  function dropSteamSoon() {
    clearTimeout(dropTimer);
    dropTimer = setTimeout(async () => {
      if (document.hasFocus() || MS.busy || !MS.list || !MS.list.live) return;
      let ok = false;
      try { ok = await api.WorkshopDisconnect(); } catch { }
      if (ok && MS.list && MS.list.live) { MS.list.live = false; MS.list.liveError = ""; if (S.view === "mods") renderModTop(); }
    }, 1500);
  }
  window.addEventListener("blur", dropSteamSoon);
  document.addEventListener("visibilitychange", () => { if (document.hidden) dropSteamSoon(); });
  window.addEventListener("focus", () => clearTimeout(dropTimer));

  // los datos de los mods (y los nombres de los autores) llegan después, por
  // tandas: se juntan con lo que hay y se vuelve a dibujar, sin pedir nada más
  const metaQueue = new Set();
  const applyMeta = debounce(async () => {
    if (!MS.list || !MS.app) { metaQueue.clear(); return; }
    const all = [...MS.list.mods, ...(MS.list.history || []), ...MS.search.mods];
    const ids = metaQueue.size ? [...metaQueue].filter((id) => all.some((m) => m.id === id) || MS.pmods.has(id)) : all.map((m) => m.id);
    metaQueue.clear();
    if (!ids.length) return;
    let d;
    try { d = await api.ModsCached(ids); } catch { return; }
    const by = new Map((d || []).map((m) => [m.id, m]));
    const keys = ["title", "author", "owner", "preview", "size", "updated", "tags", "votesUp", "votesDown", "score", "hasVotes", "subscriptions", "comments", "children", "missing", "banned", "collection", "known"];
    for (const m of all) {
      const x = by.get(m.id);
      if (!x || !x.known) continue;
      for (const k of keys) if (x[k] !== undefined && !(k === "author" && !x[k])) m[k] = x[k];
    }
    for (const x of d || []) if (MS.pmods.has(x.id) && x.known) MS.pmods.set(x.id, x);
    if (S.view === "mods" && !MS.home) { renderModBar(); renderModList(); renderModDetail(); }
  }, 250);
  rt.EventsOn("mods-meta", (p) => {
    if (p && p.ids) for (const id of p.ids) metaQueue.add(id);
    if (p && MS.list && p.appid === MS.app && typeof p.left === "number") MS.list.pending = p.left;
    applyMeta();
  });

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
    const lvl = pw.length < 12 ? 0 : Math.min(4, Math.max(1, Math.round(s * 4 / 6)));
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
        ${pwField("mv-a", "Contraseña maestra", "new-password")}<div class="strength"><i id="mv-str"></i></div><div class="hint" id="mv-strt">Al menos 12 caracteres. Mejor una frase larga que no uses en otro sitio.</div><div style="height:12px"></div>
        ${pwField("mv-b", "Repítela", "new-password")}<div class="hint" id="mv-err"></div>`;
      go.textContent = "Crear bóveda";
    } else if (mode === "unlock") {
      $("#mv-title").textContent = "Abrir la bóveda";
      f.innerHTML = `<div class="v-hero"><div class="tile">${ic("lock")}</div><p>Escribe la contraseña maestra. Se cerrará sola tras ${S.settings.lockMinutes || 5} min sin usarla.</p></div>${pwField("mv-a", "Contraseña maestra", "current-password")}<div class="hint" id="mv-err"></div>`;
      go.textContent = "Abrir";
    } else if (mode === "change") {
      $("#mv-title").textContent = "Cambiar la contraseña maestra";
      f.innerHTML = `${pwField("mv-o", "Contraseña maestra actual", "current-password")}${pwField("mv-a", "Nueva contraseña maestra", "new-password")}<div class="strength"><i id="mv-str"></i></div><div class="hint" id="mv-strt">Al menos 12 caracteres.</div><div style="height:12px"></div>${pwField("mv-b", "Repite la nueva", "new-password")}<div class="hint" id="mv-err"></div>`;
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
      $("#mv-strt").textContent = a.value ? s.label + (a.value.length < 12 ? ` · faltan ${12 - a.value.length} caracteres` : "") : "Al menos 12 caracteres. Mejor una frase larga que no uses en otro sitio.";
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
    if ((mode === "create" || mode === "change") && a.length < 12) return err("La contraseña maestra debe tener al menos 12 caracteres.");
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
    $("#s-wsauto").checked = !s.workshopManual;
    setSeg("#s-after", s.afterSwitch || "nada");
    setSeg("#s-lock", String(s.lockMinutes || 5));
    setSeg("#s-clip", String(s.clipSeconds || 20));
    $("#s-ver").textContent = "TurboGAMES " + (S.version || "");
    $("#m-settings").hidden = false;
  }
  function setSeg(sel, v) { $$(sel + " button").forEach((b) => b.classList.toggle("on", b.dataset.v === v)); }
  const segVal = (sel) => { const b = $(sel + " button.on"); return b ? b.dataset.v : ""; };
  async function saveSettings() {
    const s = { steamDir: pendingDir || "", startSilent: $("#s-silent").checked, confirmSwitch: $("#s-confirm").checked, workshopManual: !$("#s-wsauto").checked, afterSwitch: segVal("#s-after"), lockMinutes: +segVal("#s-lock"), clipSeconds: +segVal("#s-clip") };
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
  $("#game-tag").addEventListener("change", (e) => { S.gameTag = e.target.value; store.set("gameTag", S.gameTag); renderGames(); });

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
      case "mods": openModGame(g.appid, "subs"); break;
    }
  });
  $("#game-detail").addEventListener("click", async (e) => {
    const g = S.games && S.games.games.find((x) => x.appid === S.gameSel);
    if (!g) return;
    const un = e.target.closest("[data-guntag]");
    if (un) { await setGameTags(g, (g.tags || []).filter((t) => t !== un.dataset.guntag)); return; }
    const co = e.target.closest("[data-gcoll]");
    if (co) { S.gameTag = "c:" + co.dataset.gcoll; store.set("gameTag", S.gameTag); renderGames(); }
  });
  $("#game-detail").addEventListener("keydown", async (e) => {
    if (e.target.id !== "g-tag") return;
    const g = S.games && S.games.games.find((x) => x.appid === S.gameSel);
    if (!g) return;
    const say = (m) => { const el = $("#g-tag-err"); if (el) el.textContent = m; };
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      const t = e.target.value.replace(/^#+/, "").trim();
      if (!t) { say(""); return; }
      if ((g.tags || []).some((x) => x.toLowerCase() === t.toLowerCase())) { say("Ya tiene esa etiqueta"); return; }
      if ((g.tags || []).length >= 8) { say("Como mucho 8 etiquetas por juego"); return; }
      if (/[\s,#]/.test(t)) { say("Sin espacios, comas ni #: usa una palabra o guiones"); return; }
      await setGameTags(g, [...(g.tags || []), t]);
    }
    if (e.key === "Backspace" && !e.target.value && (g.tags || []).length) await setGameTags(g, g.tags.slice(0, -1));
  });
  async function setGameTags(g, tags) {
    try {
      g.tags = await api.SetGameTags(g.appid, tags);
      renderGames();
      const s = $("#g-saved"); if (s) { s.classList.add("on"); setTimeout(() => s.classList.remove("on"), 1400); }
      const i = $("#g-tag"); if (i) i.focus();
    } catch (err) { toast(errText(err), "error"); }
  }

  // modales
  // closeModal cierra un modal; si es el de datos de acceso con cambios sin
  // guardar, pregunta antes
  async function closeModal(m) {
    if (m.id === "m-progress") return; // se cierra solo al terminar (o con «Parar»)
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
  $("#ma-form").addEventListener("submit", (e) => { e.preventDefault(); addManual(false); });
  $("#ma-go").addEventListener("click", () => addManual(true));
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
      const i = $(S.view === "games" ? "#game-q" : S.view === "mods" ? "#mod-q" : "#acc-q");
      i.focus(); i.select();
      return;
    }
    if (e.ctrlKey && (e.key === "1" || e.key === "2" || e.key === "3")) { e.preventDefault(); setView({ 1: "accounts", 2: "games", 3: "mods" }[e.key]); return; }
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
    if (prev.activeId !== s.activeId && MS.list) { MS.list = null; MS.info.clear(); MS.search.app = 0; if (S.view === "mods") loadMods(); }
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
  function bootStep(t) { const el = $("#boot-step"); if (el) el.textContent = t; }
  function bootDone() {
    const b = $("#boot");
    if (!b || b.classList.contains("gone")) return;
    b.classList.add("gone");
    setTimeout(() => b.remove(), 300);
  }
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
    // la primera pantalla ya está dibujada: fuera la de carga
    requestAnimationFrame(() => requestAnimationFrame(bootDone));
    setTimeout(bootDone, 120); // por si la ventana no se está dibujando (minimizada…)
  }
  bootStep("Leyendo tus cuentas…");
  api.Init().then(applyInit).catch((e) => { bootDone(); toast(errText(e), "error"); });
  setTimeout(bootDone, 8000); // por si algo se queda colgado, que no tape la ventana

  // ---------- datos de muestra (solo al abrir el HTML en un navegador) ----------
  function mockApi() {
    const now = Math.floor(Date.now() / 1000);
    const accounts = [
      { key: "nightowl_99", name: "nightowl_99", steamId: "76561198000000001", persona: "NightOwl", alias: "", notes: "La principal. Tiene CS2 Prime.", tags: ["principal"], pinned: true, hidden: false, inSteam: true, hasToken: true, hasSecret: true, active: true, autoLogin: true, offline: false, lastUsed: now - 600, added: now - 9e6, avatar: "", games: 14, minutes: 32140, recent: { appid: 294100, name: "RimWorld", lastPlayed: now - 3600 } },
      { key: "smurfito", name: "smurfito", steamId: "76561198000000002", persona: "Smurfito", alias: "Smurf CS", notes: "", tags: ["smurf", "cs2"], pinned: false, hidden: false, inSteam: true, hasToken: true, hasSecret: true, active: false, autoLogin: false, offline: false, lastUsed: now - 86400 * 3, added: now - 5e6, avatar: "", games: 4, minutes: 4200, recent: { appid: 730, name: "Counter-Strike 2", lastPlayed: now - 86400 * 3 } },
      { key: "familia_lopez", name: "familia_lopez", steamId: "76561198000000003", persona: "Lopez Family", alias: "", notes: "Compartida con mi hermano", tags: ["familia"], pinned: false, hidden: false, inSteam: true, hasToken: false, hasSecret: false, active: false, autoLogin: false, offline: false, lastUsed: now - 86400 * 40, added: now - 4e6, avatar: "", games: 6, minutes: 900, recent: { appid: 945360, name: "Among Us", lastPlayed: now - 86400 * 40 } },
      { key: "tradebot_eu", name: "tradebot_eu", steamId: "", persona: "", alias: "Intercambios", notes: "", tags: ["trade"], pinned: false, hidden: false, inSteam: false, hasToken: false, hasSecret: true, active: false, autoLogin: false, offline: false, lastUsed: 0, added: now - 1e5, avatar: "", games: 0, minutes: 0 },
    ];
    const games = [
      { appid: 294100, name: "RimWorld", type: "game", workshop: true, mods: 6, modsSize: 1.4e9, tags: ["colonias", "pendiente-dlc"], collections: ["Favoritos"], minutes2w: 640, installed: true, dir: "C:\\Program Files (x86)\\Steam\\steamapps\\common\\RimWorld", library: "C:\\Program Files (x86)\\Steam", size: 577e6, updated: now - 86400 * 20, lastPlayed: now - 3600, minutes: 19376, owner: "76561198000000001", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 19376, lastPlayed: now - 3600 }], running: false },
      { appid: 730, name: "Counter-Strike 2", type: "game", workshop: true, tags: ["competitivo"], collections: ["Favoritos", "Con amigos"], minutes2w: 180, installed: true, dir: "D:\\SteamLibrary\\steamapps\\common\\Counter-Strike Global Offensive", library: "D:\\SteamLibrary", size: 36e9, updated: now - 86400 * 2, lastPlayed: now - 86400 * 3, minutes: 9100, owner: "76561198000000002", players: [{ key: "smurfito", name: "smurfito", persona: "Smurfito", steamId: "76561198000000002", minutes: 4200, lastPlayed: now - 86400 * 3 }, { key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 4900, lastPlayed: now - 86400 * 9 }], running: false },
      { appid: 108600, name: "Project Zomboid", type: "game", workshop: true, mods: 3, tags: ["coop"], collections: ["Con amigos"], installed: true, dir: "C:\\Program Files (x86)\\Steam\\steamapps\\common\\ProjectZomboid", library: "C:\\", size: 10.3e9, state: "update", updated: now - 86400 * 25, lastPlayed: now - 86400 * 15, minutes: 6200, owner: "76561198000000001", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 6200, lastPlayed: now - 86400 * 15 }], running: false },
      { appid: 945360, name: "Among Us", type: "game", collections: ["Con amigos"], installed: true, dir: "C:\\Games\\Among Us", library: "C:\\", size: 1.1e9, updated: now - 86400 * 60, lastPlayed: now - 86400 * 40, minutes: 900, owner: "76561198000000003", players: [{ key: "familia_lopez", name: "familia_lopez", persona: "Lopez Family", steamId: "76561198000000003", minutes: 900, lastPlayed: now - 86400 * 40 }], running: false },
      { appid: 239820, name: "Game Dev Tycoon", type: "game", installed: true, state: "missing", dir: "C:\\Games\\GDT", library: "C:\\", size: 168e6, updated: now - 86400 * 200, lastPlayed: 0, minutes: 0, owner: "76561198000000001", players: [], running: false },
      { appid: 107410, name: "Arma 3", type: "game", workshop: true, mods: 0, tags: ["milsim", "coop"], installed: false, dir: "", library: "", size: 0, updated: 0, lastPlayed: now - 86400 * 9, minutes: 41000, minutes2w: 0, owner: "", players: [{ key: "smurfito", name: "smurfito", persona: "Smurfito", steamId: "76561198000000002", minutes: 41000, lastPlayed: now - 86400 * 9 }], running: false },
      { appid: 1091500, name: "Cyberpunk 2077", type: "game", installed: false, dir: "", library: "", size: 0, updated: 0, lastPlayed: now - 86400 * 120, minutes: 5400, owner: "", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 5400, lastPlayed: now - 86400 * 120 }], running: false },
      { appid: 1304930, name: "The Outlast Trials", type: "game", installed: false, dir: "", library: "", size: 0, updated: 0, lastPlayed: 0, minutes: 0, owner: "", players: [], inLibrary: ["76561198000000002"], running: false },
      { appid: 4900640, name: "Shreddead Demo", type: "demo", installed: true, dir: "C:\\Games\\Shreddead", library: "C:\\", size: 420e6, updated: now - 86400 * 5, lastPlayed: now - 86400 * 6, minutes: 75, owner: "76561198000000001", players: [{ key: "nightowl_99", name: "nightowl_99", persona: "NightOwl", steamId: "76561198000000001", minutes: 75, lastPlayed: now - 86400 * 6 }], running: false },
    ];
    let vault = { exists: true, unlocked: false, lockMinutes: 5, locksIn: 0 };
    // mods de muestra
    const myTags = { 2009463077: ["imprescindible"], 1561769193: ["imprescindible", "base"] };
    const mk = (id, title, author, size, o = {}) => ({ id, appid: o.appid || 294100, title, author, myTags: myTags[id] || [], since: now - 86400 * (id % 40), summary: "Añade " + title.toLowerCase() + " al juego con nuevas mecánicas, objetos y opciones. Compatible con la última versión y con los mods más usados.", owner: "7656119800" + id, preview: false, size, updated: now - 86400 * (o.days || 12), tags: o.tags || [], votesUp: o.up || 900, votesDown: o.down || 40, score: .9, hasVotes: true, subscriptions: o.subs || 120000, comments: 40, children: o.children || [], missing: !!o.missing, banned: false, collection: false, known: true, subscribed: o.sub !== false, subscribedBy: o.sub === false ? "" : "76561198000000001", installed: o.inst !== false, dir: o.inst === false ? "" : "C:\\Program Files (x86)\\Steam\\steamapps\\workshop\\content\\294100\\" + id, diskSize: o.inst === false ? 0 : size, needsUpdate: !!o.upd, downloading: !!o.dl, pending: false, progress: o.dl ? .42 : 0 });
    const mods = {
      294100: [
        mk(2009463077, "Harmony", "Brrainz", 1.2e6, { subs: 3.1e6, up: 24000, down: 300, tags: ["Library"] }),
        mk(818773962, "HugsLib", "UnlimitedHugs", 2.9e6, { subs: 1.9e6, up: 15000, down: 400, children: [2009463077] }),
        mk(1561769193, "Vanilla Expanded Framework", "Oskar Potocki", 48e6, { subs: 1.2e6, up: 9800, down: 210, upd: true }),
        mk(2023507013, "Vanilla Factions Expanded - Medieval", "Oskar Potocki", 410e6, { subs: 520000, children: [1561769193, 2009463077] }),
        mk(1631756268, "Dubs Bad Hygiene", "Dubwise", 95e6, { inst: false, subs: 880000 }),
        mk(735106432, "Prepare Carefully", "edbmods", 12e6, { sub: false, subs: 1.4e6 }),
        mk(3000000001, "Medieval Overhaul", "Sihv", 820e6, { dl: true, subs: 230000, children: [2009463077] }),
      ],
      108600: [
        mk(1299328280, "More Traits", "Dr_Cox1911", 11e6, { appid: 108600, subs: 1.9e6 }),
        mk(3807334881, "pz3d - Atmospheric Vanilla PZ in 3D", "kolpaque", 279e6, { appid: 108600, subs: 73000 }),
        mk(2169435993, "ModOptions", "star", 31000, { appid: 108600, missing: true }),
      ],
      107410: [],
    };
    const allMods = () => Object.values(mods).flat();
    let presets = [
      { id: "p1", appid: 294100, game: "RimWorld", name: "Medieval tranquilo", author: "NightOwl", from: "", tags: ["medieval"], notes: "Partida larga sin prisas", mods: [2009463077, 818773962, 1561769193, 2023507013, 3000000001].map((id) => ({ id, title: (allMods().find((m) => m.id === id) || {}).title || "" })), created: now - 86400 * 30, updated: now - 86400 * 2, auto: false, source: "propio" },
      { id: "p2", appid: 294100, game: "RimWorld", name: "Lo de Ana", author: "Ana", from: "Ana", tags: ["coop", "ligero"], notes: "", mods: [2009463077, 735106432, 1631756268].map((id) => ({ id, title: (allMods().find((m) => m.id === id) || {}).title || "" })), created: now - 86400 * 4, updated: now - 86400 * 4, auto: false, source: "pegado" },
      { id: "p3", appid: 107410, game: "Arma 3", name: "Antistasi", author: "Clan Lobo", from: "Pablo", tags: ["milsim", "coop"], notes: "Servidor de los viernes", mods: [{ id: 450814997, title: "CBA_A3" }, { id: 463939057, title: "ace" }, { id: 843577117, title: "RHSUSAF" }], created: now - 86400 * 10, updated: now - 86400 * 10, auto: false, source: "arma3" },
      { id: "p4", appid: 294100, game: "RimWorld", name: "Copia automática · 01/11/2026 18:20", author: "", from: "", tags: [], notes: "Lo que tenías suscrito antes de quitar mods.", mods: [{ id: 2009463077, title: "Harmony" }], created: now - 86400 * 4, updated: now - 86400 * 4, auto: true, source: "copia" },
    ];
    const secrets = { nightowl_99: { password: "correct-horse-battery", email: "night@example.com", recovery: "R12345" }, smurfito: { password: "smurf-pass-123" }, tradebot_eu: { password: "trade$$2026" } };
    const wait = (ms) => new Promise((r) => setTimeout(r, ms));
    const status = { found: true, dir: "C:\\Program Files (x86)\\Steam", running: true, activeId: "76561198000000001", runningApp: 0, autoLogin: "nightowl_99", switching: false };
    const emit = (n, d) => (window.__mockHandlers[n] || []).forEach((f) => f(d));
    window.__mockHandlers = {};
    rt.EventsOn = (n, f) => { (window.__mockHandlers[n] = window.__mockHandlers[n] || []).push(f); };
    return {
      async Init() { return { version: "1.0 (muestra)", steam: status, settings: { lockMinutes: 5, clipSeconds: 20, afterSwitch: "nada", confirmSwitch: true, workshopManual: false }, vault, accounts }; },
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
      async SetGameTags(appid, tags) { const g = games.find((x) => x.appid === appid); if (g) g.tags = tags; return tags; },
      async Mods(app, live) {
        await wait(250);
        if (app === 107410 && status.activeId !== "76561198000000002") return { appid: app, mods: [], history: [], live: false, liveError: "La cuenta abierta en Steam no tiene este juego", notOwned: true, account: status.activeId, diskSize: 0 };
        const l = (mods[app] || []).map((m) => ({ ...m, myTags: myTags[m.id] || [] }));
        const history = app === 294100 ? [mk(1541460369, "Rimfeller", "Oskar Potocki", 6e6, { sub: false, inst: false }), mk(1414302321, "Combat Extended", "N7Huntsman", 230e6, { sub: false, inst: false })].map((m, i) => ({ ...m, removed: now - 86400 * (3 + i * 20) })) : [];
        return { appid: app, mods: l, history, live: !!live, liveError: live ? "" : "", account: status.activeId, diskSize: l.reduce((s, m) => s + (m.diskSize || 0), 0) };
      },
      async SetModTags(ids, add, remove) { const out = {}; for (const id of ids) { let t = (myTags[id] || []).filter((x) => !remove.some((r) => r.toLowerCase() === x.toLowerCase())); for (const a of add) if (!t.some((x) => x.toLowerCase() === a.toLowerCase())) t.push(a); myTags[id] = t; out[id] = t; } return out; },
      async ModsCached(ids) { return ids.map((id) => ({ ...(allMods().find((m) => m.id === id) || { id, title: "", known: false }) })); },
      async ModDetails(app, ids) { return ids.map((id) => ({ ...(allMods().find((m) => m.id === id) || { id, title: "", known: false }) })); },
      async ModInfo(app, id) {
        await wait(300);
        const m = allMods().find((x) => x.id === id) || { id, title: "Mod " + id, tags: [], children: [] };
        return { gallery: [{ video: "yHBGkPQ6tVk" }, { url: "https://images.steamusercontent.com/ugc/1/A/" }, { url: "https://images.steamusercontent.com/ugc/2/B/" }], mod: m, previewUrl: "", description: "[h1]" + m.title + "[/h1]\nUn mod de muestra con [b]negritas[/b] y un [url=https://example.com]enlace[/url].\n\n[list][*]Cosa uno[*]Cosa dos[/list]\n" + "Texto largo de relleno. ".repeat(60), required: (m.children || []).map((c) => ({ ...(allMods().find((x) => x.id === c) || { id: c, title: "Mod " + c }) })), stars: 4, ratings: 1234, commentsN: 56, commentList: [{ author: "Ana", text: "Va genial con la 1.5", time: now - 3600 * 5 }, { author: "Pablo", text: "¿Es compatible con Medieval Overhaul?\nGracias", time: now - 86400 * 2 }], pageError: "" };
      },
      async ModsSearch(app, sq) {
        const { text: q, cursor, tags, exclude } = sq;
        await wait(400);
        if (app === 107410 && status.activeId !== "76561198000000002") throw "La cuenta abierta en Steam no tiene este juego";
        const page = cursor ? +cursor : 0;
        const TG = ["Interfaz", "Animales", "Armas", "Clima", "Ropa", "Mapas"];
        let list = Array.from({ length: 12 }, (_, i) => mk(4000000000 + page * 12 + i, (q ? q + " " : "") + ["Mejor interfaz", "Más animales", "Armas medievales", "Clima realista", "Ropa nueva", "Mapas grandes"][i % 6] + " " + (page * 12 + i + 1), "autor" + i, (i + 1) * 3e6, { appid: app, sub: false, inst: false, subs: 90000 - i * 5000, tags: [TG[i % 6], i % 2 ? "1.5" : "1.4"] }));
        if (tags && tags.length) list = list.filter((m) => tags.every((t) => m.tags.includes(t)));
        if (exclude && exclude.length) list = list.filter((m) => !exclude.some((t) => m.tags.includes(t)));
        for (const m of list) m.subscribed = (mods[app] || []).some((x) => x.id === m.id);
        return { mods: list, total: 340, cursor: page < 2 ? String(page + 1) : "" };
      },
      async ModsChange(app, sub, unsub) {
        const total = sub.length + unsub.length;
        let done = 0, backup = "";
        if (unsub.length) { backup = "Copia automática"; presets.push({ id: "a" + Date.now(), appid: app, game: (games.find((g) => g.appid === app) || {}).name, name: "Copia automática · ahora", author: "", from: "", tags: [], notes: "", mods: (mods[app] || []).filter((m) => m.subscribed).map((m) => ({ id: m.id, title: m.title })), created: Math.floor(Date.now() / 1000), updated: Math.floor(Date.now() / 1000), auto: true, source: "copia" }); }
        for (const [op, ids] of [["unsubscribe", unsub], ["subscribe", sub]]) for (const id of ids) {
          const m = (mods[app] || []).find((x) => x.id === id);
          emit("mods-progress", { appid: app, done, total, title: m ? m.title : "Mod " + id, op });
          await wait(160);
          if (op === "unsubscribe") { if (m) { mods[app] = mods[app].filter((x) => x !== m); } }
          else if (m) m.subscribed = true;
          else (mods[app] = mods[app] || []).push(mk(id, "Mod nuevo " + id, "alguien", 5e6, { appid: app, inst: false }));
          done++;
        }
        return { subscribed: sub.length, unsubscribed: unsub.length, failed: [], canceled: false, backup };
      },
      async ModsCancel() { },
      async WorkshopDisconnect() { return true; },
      async OpenModInSteam() { },
      async Presets() { return presets.map((p) => ({ ...p, mods: [...p.mods], tags: [...p.tags] })).sort((a, b) => b.updated - a.updated); },
      async SavePreset(p) { if (!p.name) throw "Ponle un nombre al preset"; const n = { ...p, updated: Math.floor(Date.now() / 1000) }; if (!n.id) { n.id = "p" + Date.now(); n.created = n.updated; presets.push(n); } else presets = presets.map((x) => x.id === n.id ? n : x); return n; },
      async DeletePreset(id) { presets = presets.filter((p) => p.id !== id); },
      async PresetText(id, full) { const p = presets.find((x) => x.id === id); return `Preset de TurboGAMES: «${p.name}» · ${p.game} · ${p.mods.length} mods\n` + (full ? p.mods.map((m, i) => `${i + 1}. ${m.title} — https://steamcommunity.com/sharedfiles/filedetails/?id=${m.id}`).join("\n") + "\n" : "") + "TGP1:muestra"; },
      async ParsePreset(text) {
        await wait(300);
        const ids = [...new Set([...String(text).matchAll(/id=(\d{3,})|^\s*(\d{6,})\s*$/gm)].map((m) => +(m[1] || m[2])))];
        if (!ids.length) throw "No he encontrado ningún mod en ese texto. Pega un preset de TurboGAMES, enlaces del Workshop o sus números.";
        const name = (String(text).match(/«([^»]+)»/) || [])[1] || "Preset de RimWorld";
        const from = (String(text).match(/^De:\s*(.+)$/m) || [])[1] || "";
        return { preset: { id: "", appid: 294100, game: "RimWorld", name, author: from, from: "", tags: [], notes: "", mods: ids.map((id) => ({ id, title: (allMods().find((m) => m.id === id) || {}).title || "" })), source: "pegado" }, missing: 0, others: 0, local: 0, warnings: ids.length > 2 ? ["1 mod ya no existe, es privado o no se ha podido comprobar."] : [], size: ids.length * 4e6 };
      },
      async ImportPresetFile() { return this.ParsePreset("Preset de TurboGAMES: «Desde archivo»\nhttps://steamcommunity.com/sharedfiles/filedetails/?id=2009463077\n818773962"); },
      async ExportPresetFile() { return true; },
    };
  }
})();
