(() => {
  "use strict";
  const $ = (s) => document.querySelector(s);
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  // ---------- puente con Go (con datos de muestra si se abre en un navegador) ----------
  const hasGo = !!(window.go && window.go.main && window.go.main.App);
  const api = hasGo ? window.go.main.App : demoApi();
  const rt = window.runtime || demoRuntime();

  const ICONS = { turbosend: "icons/turbosend.svg", turbocut: "icons/turbocut.svg", turbodl: "icons/turbodl.png", turboblock: "icons/turboblock.svg", turbomenu: "icons/turbomenu.svg", turbogames: "icons/turbogames.svg", turbovault: "icons/turbovault.svg" };
  const I = {
    down: '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>',
    up: '<path d="M12 19V5"/><path d="m5 12 7-7 7 7"/>',
    open: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
    gh: '<path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.4 5.4 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65S8.93 17.38 9 18v4"/><path d="M9 18c-4.51 2-5-2-7-2"/>',
    redo: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
    ok: '<path d="M20 6 9 17l-5-5"/>',
    err: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5"/><path d="M12 16.5v.01"/>',
  };
  const ic = (n) => `<svg viewBox="0 0 24 24">${I[n]}</svg>`;

  const S = { cat: null, busy: new Map(), loading: false };

  function fmtSize(b) {
    if (!b) return "";
    const u = ["B", "KB", "MB", "GB"];
    let i = 0;
    while (b >= 1024 && i < u.length - 1) { b /= 1024; i++; }
    return (i >= 2 ? b.toFixed(1) : Math.round(b)).toString().replace(".", ",") + " " + u[i];
  }

  function toast(msg, kind = "ok") {
    const t = document.createElement("div");
    t.className = "toast " + kind;
    t.innerHTML = ic(kind === "ok" ? "ok" : "err") + `<span>${esc(msg)}</span>`;
    $("#toasts").appendChild(t);
    setTimeout(() => { t.classList.add("out"); setTimeout(() => t.remove(), 300); }, kind === "ok" ? 4200 : 7000);
  }

  // ---------- catálogo ----------
  async function load() {
    if (S.loading) return;
    S.loading = true;
    $("#wb-refresh").classList.add("spin");
    if (!S.cat) $("#tools").innerHTML = '<div class="skel"></div><div class="skel"></div><div class="skel"></div>';
    try {
      S.cat = await api.Catalog();
    } catch (e) {
      S.cat = { tools: [], error: String(e) };
    }
    S.loading = false;
    $("#wb-refresh").classList.remove("spin");
    render();
  }

  function render() {
    const c = S.cat || { tools: [] };
    $("#ver").textContent = c.version ? "v" + c.version : "";
    $("#where").textContent = c.root ? "Se instala en " + c.root : "";

    $("#self-banner").hidden = !c.setupUpdate;
    $("#self-text").textContent = c.setupUpdate ? `Tienes la ${c.version} y está la ${c.setupLatest}.` : "";

    $("#err-banner").hidden = !c.error;
    $("#err-text").textContent = c.error ? c.error + (c.tools.length ? " Se muestra lo que tienes instalado." : "") : "";

    const box = $("#tools");
    if (!c.tools.length) {
      box.innerHTML = c.error ? "" : '<div class="empty">Todavía no hay herramientas publicadas.</div>';
    } else {
      if (!S.shown) { S.shown = true; box.classList.add("first"); setTimeout(() => box.classList.remove("first"), 1200); }
      box.innerHTML = c.tools.map(card).join("");
      c.tools.forEach((t, i) => { box.children[i].style.animationDelay = i * 50 + "ms"; });
      for (const [id, p] of S.busy) paintProgress(id, p);
    }

    const pending = c.tools.filter((t) => t.available && (!t.installed || t.update));
    const all = $("#all");
    all.hidden = pending.length < 2;
    const ups = pending.filter((t) => t.update).length;
    all.innerHTML = ic(ups === pending.length ? "up" : "down") +
      (ups === pending.length ? "Actualizar todo" : ups ? "Instalar y actualizar todo" : "Instalar todo");
  }

  function card(t) {
    const icon = ICONS[t.id] ? `<img class="tool-ic" src="${ICONS[t.id]}" alt="">` : '<div class="tool-ic blank"></div>';
    let chip, actions = "";
    if (t.update) {
      chip = `<span class="chip up">${esc(t.installedVersion)} → ${esc(t.version)}</span>`;
    } else if (t.installed) {
      chip = `<span class="chip ok">${ic("ok")}Instalado · ${esc(t.installedVersion)}</span>`;
    } else {
      chip = `<span class="chip">${esc(t.version)}${t.size ? " · " + fmtSize(t.size) : ""}</span>`;
    }

    const isApp = t.kind !== "setup";
    if (t.installed) {
      actions += `<button class="ibtn" data-act="gh" title="Ver en GitHub">${ic("gh")}</button>`;
      actions += `<button class="ibtn danger" data-act="uninstall" title="Desinstalar">${ic("trash")}</button>`;
    } else if (t.url) {
      actions += `<button class="ibtn" data-act="gh" title="Ver en GitHub">${ic("gh")}</button>`;
    }
    if (t.update) {
      if (isApp) actions += `<button class="btn ghost sm" data-act="open">Abrir</button>`;
      actions += `<button class="btn amber sm" data-act="install">${ic("up")}Actualizar</button>`;
    } else if (t.installed) {
      if (isApp) actions += `<button class="btn primary sm" data-act="open">${ic("open")}Abrir</button>`;
      else if (t.available) actions += `<button class="btn ghost sm" data-act="install" title="Vuelve a instalar el motor y te recuerda cómo cargar la extensión">${ic("redo")}Reinstalar</button>`;
    } else if (t.available) {
      actions += `<button class="btn primary sm" data-act="install">${ic("down")}Instalar</button>`;
    }

    return `<div class="tool${t.update ? " has-update" : ""}" data-id="${esc(t.id)}">
      <div class="tool-top">
        ${icon}
        <div class="tool-txt">
          <div class="tool-name"><h3>${esc(t.name)}</h3>${chip}</div>
          <div class="tool-sum">${esc(t.summary || "")}</div>
        </div>
        <div class="tool-act">${actions}</div>
      </div>
      <div class="prog">
        <div class="prog-bar indet"><i></i></div>
        <div class="prog-row"><span class="prog-t">Conectando con GitHub…</span><span class="grow"></span><a data-act="cancel">Cancelar</a></div>
      </div>
    </div>`;
  }

  // ---------- progreso ----------
  rt.EventsOn("progress", (p) => {
    const b = S.busy.get(p.id);
    if (!b) return;
    const now = performance.now();
    if (b.t0 === undefined) { b.t0 = now; b.d0 = p.done; }
    const secs = (now - b.t0) / 1000;
    b.speed = secs > 0.5 ? (p.done - b.d0) / secs : 0;
    b.done = p.done; b.total = p.total;
    paintProgress(p.id, b);
  });

  function paintProgress(id, b) {
    const el = document.querySelector(`.tool[data-id="${CSS.escape(id)}"]`);
    if (!el) return;
    el.classList.add("busy");
    el.querySelectorAll(".tool-act button").forEach((x) => (x.disabled = true));
    const bar = el.querySelector(".prog-bar");
    const txt = el.querySelector(".prog-t");
    if (b.phase === "setup") {
      bar.classList.add("indet");
      txt.textContent = "Instalando… sigue los pasos de la ventana que se abre";
      el.querySelector("[data-act=cancel]").hidden = true;
      return;
    }
    if (b.total > 0) {
      const pct = Math.min(100, (b.done / b.total) * 100);
      bar.classList.remove("indet");
      bar.querySelector("i").style.width = pct.toFixed(1) + "%";
      if (b.done >= b.total) {
        txt.textContent = "Instalando…";
        el.querySelector("[data-act=cancel]").hidden = true;
      } else {
        txt.textContent = `${fmtSize(b.done)} de ${fmtSize(b.total)}` + (b.speed ? ` · ${fmtSize(b.speed)}/s` : "");
      }
    }
  }

  async function install(id) {
    if (S.busy.has(id)) return;
    const t = S.cat.tools.find((x) => x.id === id);
    const b = {};
    S.busy.set(id, b);
    paintProgress(id, b);
    if (t && t.kind === "setup") {
      // Tras descargar, TurboDL abre su propio asistente.
      const timer = setInterval(() => { if (b.total && b.done >= b.total) { b.phase = "setup"; paintProgress(id, b); clearInterval(timer); } }, 200);
      b.timer = timer;
    }
    try {
      const r = await api.Install(id, $("#desktop").checked);
      toast(r.message, "ok");
    } catch (e) {
      if (String(e) !== "cancelado") toast(`${t ? t.name : id}: ${e}`, "error");
    } finally {
      clearInterval(b.timer);
      S.busy.delete(id);
      await load();
    }
  }

  async function installAll() {
    const pending = S.cat.tools.filter((t) => t.available && (!t.installed || t.update));
    $("#all").disabled = true;
    for (const t of pending) await install(t.id);
    $("#all").disabled = false;
  }

  // ---------- desinstalar ----------
  let unId = null;
  function askUninstall(id) {
    const t = S.cat.tools.find((x) => x.id === id);
    unId = id;
    $("#un-title").textContent = `¿Desinstalar ${t.name}?`;
    $("#un-text").textContent = t.kind === "setup"
      ? "Se quita el motor nativo y la copia de la extensión. Después, quita también TurboDL en chrome://extensions."
      : "Se borra el programa y sus accesos directos. Tus ajustes y archivos no se tocan.";
    $("#m-uninstall").hidden = false;
  }
  $("#un-go").addEventListener("click", async () => {
    const id = unId;
    $("#m-uninstall").hidden = true;
    try {
      toast(await api.Uninstall(id), "ok");
    } catch (e) {
      toast(String(e), "error");
    }
    load();
  });
  document.querySelectorAll("[data-close]").forEach((b) => b.addEventListener("click", () => ($("#m-uninstall").hidden = true)));

  // ---------- eventos ----------
  $("#tools").addEventListener("click", async (e) => {
    const btn = e.target.closest("[data-act]");
    if (!btn) return;
    const id = btn.closest(".tool").dataset.id;
    const t = S.cat.tools.find((x) => x.id === id);
    switch (btn.dataset.act) {
      case "install": install(id); break;
      case "cancel": api.Cancel(id); break;
      case "uninstall": askUninstall(id); break;
      case "gh": api.OpenURL(t.url); break;
      case "open":
        try { await api.Open(id); } catch (err) { toast(String(err), "error"); }
        break;
    }
  });
  $("#all").addEventListener("click", installAll);
  $("#err-retry").addEventListener("click", load);
  $("#wb-refresh").addEventListener("click", load);
  $("#wb-folder").addEventListener("click", () => api.OpenFolder());
  $("#where").addEventListener("click", () => api.OpenFolder());
  $("#repo").addEventListener("click", (e) => { e.preventDefault(); api.OpenURL((S.cat && S.cat.repo) || "https://github.com/warmasster/tools"); });
  $("#wb-min").addEventListener("click", () => rt.WindowMinimise());
  $("#wb-close").addEventListener("click", () => rt.Quit());
  $("#self-go").addEventListener("click", async () => {
    const b = $("#self-go");
    b.disabled = true; b.textContent = "Descargando…";
    try { await api.UpdateSelf(); } catch (e) { toast(String(e), "error"); b.disabled = false; b.textContent = "Actualizar"; }
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") $("#m-uninstall").hidden = true;
    if (e.key === "F5") { e.preventDefault(); load(); }
  });
  document.addEventListener("contextmenu", (e) => e.preventDefault());

  load();

  // ---------- datos de muestra para ver la interfaz en un navegador ----------
  function demoApi() {
    const tools = [
      { id: "turbosend", name: "TurboSEND", kind: "app", version: "2.0.0", size: 13.4e6, available: true, installed: true, installedVersion: "1.9.0", update: true, summary: "Envía y recibe archivos de cualquier tamaño en chats. El otro lado solo necesita el navegador.", url: "#" },
      { id: "turbocut", name: "TurboCUT", kind: "app", version: "1.0.0", size: 12.1e6, available: true, installed: true, installedVersion: "1.0.0", summary: "Analiza el disco a toda velocidad y te ayuda a liberar espacio.", url: "#" },
      { id: "turbodl", name: "TurboDL", kind: "setup", version: "1.1.0", size: 7.4e6, available: true, summary: "Gestor de descargas multiconexión para Chrome, con motor nativo de hasta 64 conexiones.", url: "#" },
      { id: "turbogames", name: "TurboGAMES", kind: "app", version: "1.0.0", size: 11.2e6, available: true, summary: "Todas tus cuentas de Steam a un clic, con sus contraseñas en una bóveda cifrada, y tus juegos con sus portadas y quién los ha jugado.", url: "#" },
    ];
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    return {
      async Catalog() { await sleep(500); return { tools, version: "1.0.0", root: "C:\\Users\\tú\\AppData\\Local\\Programs\\Turbo", repo: "#" }; },
      async Install(id) {
        const t = tools.find((x) => x.id === id);
        for (let d = 0; d <= t.size; d += t.size / 25) { await sleep(60); window.runtime_emit && window.runtime_emit({ id, done: d, total: t.size }); }
        t.installed = true; t.installedVersion = t.version; t.update = false;
        return { message: t.name + " " + t.version + " instalado" };
      },
      async Uninstall(id) { const t = tools.find((x) => x.id === id); t.installed = false; t.update = false; return t.name + " desinstalado"; },
      Cancel() { }, Open() { }, OpenURL() { }, OpenFolder() { }, UpdateSelf() { },
    };
  }
  function demoRuntime() {
    const subs = [];
    window.runtime_emit = (p) => subs.forEach((f) => f(p));
    return { EventsOn(n, f) { subs.push(f); }, WindowMinimise() { }, Quit() { } };
  }
})();
