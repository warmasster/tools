"use strict";
// Service worker de TurboCAM. Guarda la página para que la app instalada abra aunque
// el PC esté apagado o el enlace haya caducado (y así se pueda escanear el nuevo).
// Siempre va primero a la red: lo guardado solo se usa si el PC no contesta.
const CACHE = "turbocam-1";
const SHELL = ["./", "phone.js", "phone.css", "paint.js", "encoder.js", "logo.svg", "icon-192.png", "icon-512.png", "manifest.webmanifest"];
const scope = new URL(self.registration.scope);
const shellURLs = new Set(SHELL.map((f) => new URL(f, scope).href));

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).catch(() => {}).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (e) => {
  e.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  // Solo la página y sus archivos; lo demás (ws, alive?…, ev?…) va directo.
  if (url.search || !shellURLs.has(url.origin + url.pathname)) return;
  e.respondWith((async () => {
    const cache = await caches.open(CACHE);
    try {
      const res = await fetch(req);
      if (res.ok) {
        cache.put(req, res.clone());
        return res;
      }
      // 404: el enlace ya no vale. Mejor abrir la app (que ofrece escanear el nuevo).
      return (await cache.match(req)) || res;
    } catch {
      const hit = await cache.match(req);
      if (hit) return hit;
      throw new Error("sin conexión");
    }
  })());
});
