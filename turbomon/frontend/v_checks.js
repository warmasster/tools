"use strict";
// ---------- revisión ----------
function renderChecks() {
  const r = state.report;
  if (!r) return `<div class="empty">Revisando tu PC…</div>`;
  const [t, p] = scoreText(r);
  const groups = {};
  r.results.forEach(x => (groups[x.group] = groups[x.group] || []).push(x));
  const stTxt = { ok: "Bien", warn: "Mejorable", bad: "Mal", info: "Info" };
  const html = Object.keys(groups).map(g => `<div class="group"><h3>${esc(g)}</h3>${groups[g].map(x => {
    const open = state.open.has(x.id);
    return `<div class="chk ${x.status} ${open ? "open" : ""}" data-key="${esc(x.id)}">
      <div class="chk-row" data-act="toggle" data-arg="${esc(x.id)}"><span class="dot ${x.status}"></span><div class="t"><b>${esc(x.title)}</b></div>
        <span class="val">${esc(x.value)}</span><span class="pill-st ${x.status}">${stTxt[x.status]}</span><span class="chev">${IC.chev}</span></div>
      <div class="chk-body"><p>${esc(x.detail)}</p>${x.howTo && x.howTo.length ? `<ol>${x.howTo.map(h => `<li>${esc(h)}</li>`).join("")}</ol>` : ""}
        ${x.fix ? `<button class="btn sm primary" data-act="fix" data-arg="${esc(x.fix)}" ${state.busy[x.fix] ? "disabled" : ""}>${IC.wrench}${esc(x.fixLabel || "Arreglar")}</button>` : ""}</div>
    </div>`;
  }).join("")}</div>`).join("");
  return `<div class="check-head">${ring(r.score, 104)}<div class="grow"><h1>${esc(t)}</h1><p class="sub">${esc(p)} Revisado ${when(r.time)}.</p></div>
      <button class="btn ghost" data-act="recheck" ${state.checking ? "disabled" : ""}>${IC.refresh}Revisar otra vez</button></div>
    <div class="groups">${html}</div>`;
}

views.checks = { render: renderChecks, enter: () => { if (!state.report || Date.now() / 1000 - state.report.time > 60) runChecks(); } };
