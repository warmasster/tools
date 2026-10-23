// Genera icons.html: una hoja con cada icono del menú a varios tamaños (celdas de 64 px).
// Uso: node dev/icons/make-sheet.js  → luego Chrome headless saca sheet.png y gen.go hace los .ico.
const fs = require("fs");
const path = require("path");
const root = path.join(__dirname, "..", "..");
const src = fs.readFileSync(path.join(root, "frontend/app.js"), "utf8");
const P = eval("({" + src.match(/const P = \{([\s\S]*?)\n  \};/)[1] + "})");
const icons = [["reduce", "shrink"], ["convert", "convert"], ["send", "send"], ["qr", "phone"], ["safe", "shieldOk"], ["lock", "lock"], ["origin", "globe"], ["folder", "folderPlus"], ["paste", "paste"], ["receive", "phoneDown"]];
const sizes = [16, 20, 24, 32, 40, 48, 64];
const logo = fs.readFileSync(path.join(root, "frontend/logo.svg"), "utf8").replace(/^<svg /, "<svg width=\"100%\" height=\"100%\" ");
let cells = "";
const ids = ["turbo", ...icons.map((i) => i[0])];
ids.forEach((id, r) => {
  sizes.forEach((s, c) => {
    const sw = s <= 20 ? 2.6 : s <= 32 ? 2.25 : 2;
    const inner = id === "turbo" ? logo
      : `<svg width="100%" height="100%" viewBox="0 0 24 24" fill="none" stroke="url(#g)" stroke-width="${sw}" stroke-linecap="round" stroke-linejoin="round">${P[icons[r - 1][1]]}</svg>`;
    cells += `<div style="position:absolute;left:${c * 64}px;top:${r * 64}px;width:${s}px;height:${s}px">${inner}</div>`;
  });
});
fs.writeFileSync(path.join(__dirname, "icons.html"), `<!doctype html><html><head><style>html,body{margin:0;background:transparent}</style></head><body>
<svg width="0" height="0" style="position:absolute"><defs><linearGradient id="g" x1="0" y1="0" x2="24" y2="24" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#8b5cf6"/><stop offset=".5" stop-color="#6366f1"/><stop offset="1" stop-color="#06b6d4"/></linearGradient></defs></svg>${cells}</body></html>`);
fs.writeFileSync(path.join(__dirname, "layout.json"), JSON.stringify({ ids, sizes, cell: 64 }));
console.log(`${ids.length * 64}x${sizes.length * 64}`);
