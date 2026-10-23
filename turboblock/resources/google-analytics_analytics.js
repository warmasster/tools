/* TurboBLOCK: sustituto de Google Analytics (analytics.js / ga.js). No envía nada. */
(function () {
  'use strict';
  const noop = function () {};
  const tracker = { get: noop, set: noop, send: noop };
  const ga = function () {
    const args = arguments;
    const last = args[args.length - 1];
    if (last && typeof last === 'object' && typeof last.hitCallback === 'function') { try { last.hitCallback(); } catch {} }
    if (typeof args[0] === 'function') { try { args[0](tracker); } catch {} }
  };
  ga.create = () => tracker; ga.getByName = () => tracker; ga.getAll = () => [tracker]; ga.remove = noop; ga.loaded = true;
  const name = window.GoogleAnalyticsObject || 'ga';
  const queue = window[name] && window[name].q;
  window[name] = ga;
  if (Array.isArray(queue)) for (const a of queue) ga.apply(null, a);
  window._gaq = window._gaq || { push: noop };
  window._gat = window._gat || { _getTracker: () => ({ _trackPageview: noop, _trackEvent: noop }), _createTracker: () => ({}) };
  // gtag.js / dataLayer: ejecutar los callbacks de eventos para que no se queden esperando.
  const dl = window.dataLayer;
  if (Array.isArray(dl)) {
    const push = (o) => { if (o && typeof o.eventCallback === 'function') { try { o.eventCallback(); } catch {} } };
    for (const o of dl) push(o);
    dl.push = function (...items) { items.forEach(push); return Array.prototype.push.apply(this, items); };
  }
})();
