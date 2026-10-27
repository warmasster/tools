// Service worker mínimo: solo hace falta para que el navegador ofrezca instalar
// TurboCAM en la pantalla de inicio. No guarda nada: sin el PC no hay nada que ver.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {});
