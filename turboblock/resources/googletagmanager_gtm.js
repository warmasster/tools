/* TurboBLOCK: sustituto de Google Tag Manager. No carga ni envía nada. */
(function () {
  'use strict';
  const noop = function () {};
  const run = (o) => { if (o && typeof o.eventCallback === 'function') { try { o.eventCallback(); } catch {} } };
  const dl = window.dataLayer = window.dataLayer || [];
  for (const o of dl) run(o);
  dl.push = function (...items) { items.forEach(run); return Array.prototype.push.apply(this, items); };
  window.ga = window.ga || noop;
  window.google_tag_manager = window.google_tag_manager || {};
})();
