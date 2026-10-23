/* TurboBLOCK: sustituto de FuckAdBlock / BlockAdBlock: siempre dice "no hay bloqueador". */
(function () {
  'use strict';
  const Fake = function () {};
  Fake.prototype = {
    check() { return this; }, clearEvent() { return this; }, emitEvent() { return this; },
    on(detected, fn) { if (!detected && typeof fn === 'function') setTimeout(fn, 1); return this; },
    onDetected() { return this; },
    onNotDetected(fn) { if (typeof fn === 'function') setTimeout(fn, 1); return this; },
    setOption() { return this; },
  };
  const inst = new Fake();
  for (const [c, o] of [['FuckAdBlock', 'fuckAdBlock'], ['BlockAdBlock', 'blockAdBlock'], ['SniffAdBlock', 'sniffAdBlock']]) {
    window[c] = Fake;
    window[o] = inst;
  }
})();
