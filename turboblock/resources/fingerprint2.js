/* TurboBLOCK: sustituto de fingerprintjs2. No toma ninguna huella del navegador. */
(function () {
  'use strict';
  const hash = '74656c6c2069732061206c696172';
  const later = (fn, ...a) => setTimeout(() => { if (typeof fn === 'function') fn(...a); }, 1);
  const Fp = function () {};
  Fp.get = function (opts, cb) { later(cb || opts, []); };
  Fp.getPromise = function () { return Promise.resolve([]); };
  Fp.getV18 = function (opts, cb) { later(cb || opts, hash, []); };
  Fp.x64hash128 = function () { return hash; };
  Fp.prototype = { get(cb) { later(cb, hash, []); } };
  window.Fingerprint2 = Fp;
  window.Fingerprint = Fp;
})();
