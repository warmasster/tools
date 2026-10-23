/* TurboBLOCK: sustituto de adsbygoogle.js. Acepta los push() sin pedir anuncios. */
(function () {
  'use strict';
  const q = window.adsbygoogle;
  window.adsbygoogle = { loaded: true, push() {} };
  if (Array.isArray(q)) q.length = 0;
})();
