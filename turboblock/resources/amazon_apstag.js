/* TurboBLOCK: sustituto de apstag (anuncios de Amazon): las pujas vuelven vacías. */
(function () {
  'use strict';
  window.apstag = {
    _Q: [], init() {}, setDisplayBids() {}, targetingKeys() { return []; }, renderImp() {}, dpa() {}, rpa() {}, upa() {},
    fetchBids(cfg, cb) { if (typeof cb === 'function') setTimeout(() => cb([]), 1); },
  };
})();
