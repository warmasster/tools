/* TurboBLOCK · biblioteca de scriptlets (mundo MAIN, document_start).
 *
 * No se carga tal cual: el compilador (native/compile.go) la mete dentro de cada
 * rules/<cat>.slN.js, junto con los datos S (scriptlets), K (dominio → índices) y
 * X (excepciones), y copia solo los bloques que ese archivo necesita:
 *
 *   //# nombre [needs=a,b]   ...   //# end
 *
 * "core" y "run" van siempre. Los nombres que empiezan por "_" son ayudantes.
 * Todo queda dentro de una función: la página no ve nada de esto.
 * La semántica sigue la de uBlock Origin para que sus filtros funcionen igual.
 */

//# core
const host = location.hostname;
const R = Object.create(null); // scriptlets por nombre
const W = window;
const $apply = Reflect.apply;
const $defineProperty = Object.defineProperty;
const $getOwnPropertyDescriptor = Object.getOwnPropertyDescriptor;
const $toString = Function.prototype.toString;
const magic = String.fromCharCode(97 + Date.now() % 26) + Math.floor(Math.random() * 982451653 + 982451653).toString(36);

// Los errores que lanzamos para abortar scripts no deben llenar la consola.
W.addEventListener('error', (ev) => {
  if (typeof ev.message === 'string' && ev.message.includes(magic)) ev.preventDefault();
}, true);
const abort = () => { throw new ReferenceError(magic); };

// "texto" literal, "/regex/flags" o "" (todo). Con "!" delante, al revés.
function toRegex(pattern, flags) {
  if (pattern === undefined || pattern === '') return /^/;
  const m = /^\/(.+)\/([gimsu]*)$/.exec(pattern);
  if (m) {
    try { return new RegExp(m[1], flags !== undefined ? flags : m[2]); } catch { return /^\b$/; }
  }
  return new RegExp(pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), flags);
}
function needleTest(needle) {
  let not = false;
  if (typeof needle === 'string' && needle.startsWith('!')) { not = true; needle = needle.slice(1); }
  const re = toRegex(needle);
  return (s) => re.test(s) !== not;
}
function fnText(fn) {
  if (typeof fn === 'function') { try { return $apply($toString, fn, []); } catch { return ''; } }
  return String(fn);
}
// Cuándo ejecutar algo que necesita el DOM.
function whenReady(fn, at) {
  const want = at === 'complete' ? 3 : at === 'interactive' || at === 'end' ? 2 : 1;
  const state = () => ({ loading: 1, interactive: 2, complete: 3 })[document.readyState];
  if (state() >= want) { fn(); return; }
  const ev = want === 3 ? 'load' : 'DOMContentLoaded';
  (want === 3 ? W : document).addEventListener(ev, () => fn(), { once: true, capture: true });
}
// Valores de set-constant y compañía.
function constValue(raw) {
  switch (raw) {
    case 'undefined': return undefined;
    case 'false': return false;
    case 'true': return true;
    case 'null': return null;
    case "''": case '': return '';
    case '[]': case 'emptyArr': return [];
    case '{}': case 'emptyObj': return {};
    case 'noopFunc': return function () {};
    case 'noopCallbackFunc': return function () { const a = arguments[0]; if (typeof a === 'function') try { a(); } catch {} };
    case 'trueFunc': return function () { return true; };
    case 'falseFunc': return function () { return false; };
    case 'throwFunc': return function () { throw new Error(magic); };
    case 'noopThrow': return function () { throw new Error(magic); };
    case 'emptyStr': return '';
    case 'yes': case 'no': case 'on': case 'off': case 'ok': case 'accept': case 'reject': case 'allow': case 'deny': return raw;
    case '-1': return -1;
  }
  if (/^\d+$/.test(raw)) { const n = parseInt(raw, 10); return n > 0x7fff ? undefined : n; }
  return Symbol.for('turboblock-nope');
}
const NOPE = Symbol.for('turboblock-nope');

// Recorre "a.b.c" poniendo trampas en los objetos que aún no existen: cuando la
// página asigne "a", seguimos con "b".
function trapChain(owner, chain, onFinal) {
  const pos = chain.indexOf('.');
  if (pos === -1) { onFinal(owner, chain); return; }
  const prop = chain.slice(0, pos);
  const rest = chain.slice(pos + 1);
  let v;
  try { v = owner[prop]; } catch { return; }
  if (v instanceof Object || (typeof v === 'object' && v !== null)) { trapChain(v, rest, onFinal); return; }
  const d = $getOwnPropertyDescriptor(owner, prop);
  if (d && d.configurable === false) return;
  try {
    $defineProperty(owner, prop, {
      configurable: true,
      get() { return v; },
      set(a) { v = a; if (a instanceof Object) trapChain(a, rest, onFinal); },
    });
  } catch {}
}
function resolveChain(chain) {
  let owner = W;
  if (chain.startsWith('window.')) chain = chain.slice(7);
  for (;;) {
    const pos = chain.indexOf('.');
    if (pos === -1) return [owner, chain];
    owner = owner[chain.slice(0, pos)];
    if (owner === undefined || owner === null) return [];
    chain = chain.slice(pos + 1);
  }
}

// "url:/x/ method:POST" o solo un patrón de URL. Vacío = todo.
function propsMatcher(props) {
  const tests = [];
  if (props) {
    for (const part of props.trim().split(/\s+/)) {
      const i = part.indexOf(':');
      if (i > 0 && /^[a-zA-Z]+$/.test(part.slice(0, i))) tests.push([part.slice(0, i), needleTest(part.slice(i + 1))]);
      else tests.push(['url', needleTest(part)]);
    }
  }
  return (details) => tests.every(([k, t]) => t(String(details[k] ?? '')));
}
function fetchDetails(args) {
  const d = { url: '', method: 'GET', body: '' };
  const [a, b] = args;
  if (a instanceof Request) { d.url = a.url; d.method = a.method; }
  else d.url = String(a);
  if (b && typeof b === 'object') {
    if (b.method) d.method = String(b.method);
    if (typeof b.body === 'string') d.body = b.body;
  }
  try { d.url = new URL(d.url, location.href).href; } catch {}
  return d;
}
// Lee los pares opcionales ("propsToMatch", "url:/x/") del final de los argumentos.
function extraArgs(args) {
  const o = {};
  for (let i = 0; i + 1 < args.length; i += 2) o[args[i]] = args[i + 1];
  return o;
}
//# end

//# set-constant
R['set-constant'] = function (chain, raw, ...rest) {
  if (!chain) return;
  let cValue = constValue(raw);
  if (cValue === NOPE) return;
  const opts = extraArgs(rest);
  if (opts.as === 'function') { const v = cValue; cValue = () => v; }
  else if (opts.as === 'callback') { const v = cValue; cValue = () => () => v; }
  else if (opts.as === 'resolved') cValue = Promise.resolve(cValue);
  else if (opts.as === 'rejected') cValue = Promise.reject(cValue);
  setConstant(chain.replace(/^window\./, ''), cValue);
};
function setConstant(chain, cValue, trusted) {
  let aborted = false;
  const mustAbort = (v) => {
    if (trusted) return false;
    if (aborted) return true;
    aborted = v !== undefined && v !== null && cValue !== undefined && cValue !== null && typeof v !== typeof cValue;
    return aborted;
  };
  trapChain(W, chain, (owner, prop) => {
    let current;
    try { current = owner[prop]; } catch {}
    if (mustAbort(current)) return;
    const d = $getOwnPropertyDescriptor(owner, prop);
    if (d && d.configurable === false) {
      try { if (d.writable) owner[prop] = cValue; } catch {}
      return;
    }
    let pageValue = cValue;
    try {
      $defineProperty(owner, prop, {
        configurable: false,
        get() { return aborted ? pageValue : cValue; },
        set(a) { if (mustAbort(a)) pageValue = a; },
      });
    } catch {}
  });
}
//# end

//# trusted-set-constant needs=set-constant
R['trusted-set-constant'] = function (chain, raw) {
  if (!chain) return;
  let v = constValue(raw);
  if (v === NOPE) { try { v = JSON.parse(raw); } catch { v = raw; } }
  setConstant(chain.replace(/^window\./, ''), v, true);
};
//# end

//# abort-on-property-read
R['abort-on-property-read'] = function (chain) {
  if (!chain) return;
  trapChain(W, chain.replace(/^window\./, ''), (owner, prop) => {
    const d = $getOwnPropertyDescriptor(owner, prop);
    if (d && d.configurable === false) return;
    try { $defineProperty(owner, prop, { configurable: true, get: abort, set() {} }); } catch {}
  });
};
//# end

//# abort-on-property-write
R['abort-on-property-write'] = function (chain) {
  if (!chain) return;
  const [owner, prop] = resolveChain(chain);
  if (!owner) return;
  try { delete owner[prop]; $defineProperty(owner, prop, { configurable: true, set: abort }); } catch {}
};
//# end

//# abort-current-script
R['abort-current-script'] = function (target, needle, context) {
  if (!target) return;
  const [owner, prop] = resolveChain(target);
  if (!owner) return;
  const reNeedle = toRegex(needle);
  const reContext = toRegex(context);
  const thisScript = document.currentScript;
  const scriptText = (e) => {
    const t = e.textContent;
    if (t.trim() !== '') return t;
    const m = /^data:([^,]*),(.+)$/.exec(e.src.trim());
    if (!m) return '';
    try { return m[1].endsWith(';base64') ? atob(m[2]) : decodeURIComponent(m[2]); } catch { return ''; }
  };
  const validate = () => {
    const e = document.currentScript;
    if (!(e instanceof HTMLScriptElement) || e === thisScript) return;
    if (context && !reContext.test(e.src)) return;
    if (needle && !reNeedle.test(scriptText(e))) return;
    abort();
  };
  let d = $getOwnPropertyDescriptor(owner, prop);
  if (d && d.configurable === false) return;
  let value = owner[prop];
  if (d && (d.get || d.set)) {
    const { get, set } = d;
    try {
      $defineProperty(owner, prop, {
        configurable: true,
        get() { validate(); return get ? get.call(owner) : undefined; },
        set(a) { validate(); if (set) set.call(owner, a); },
      });
    } catch {}
    return;
  }
  try {
    $defineProperty(owner, prop, {
      configurable: true,
      get() { validate(); return value; },
      set(a) { validate(); value = a; },
    });
  } catch {}
};
//# end

//# abort-on-stack-trace
R['abort-on-stack-trace'] = function (chain, needle) {
  if (!chain) return;
  const re = toRegex(needle);
  const check = () => {
    let stack = '';
    try { stack = new Error().stack || ''; } catch {}
    // uBO llama "inlineScript" a la propia página.
    const page = location.href.split('#')[0];
    stack = stack.split('\n').slice(2).map((l) => l.includes(page) ? l.replace(page, 'inlineScript') : l).join('\n');
    if (re.test(stack)) abort();
  };
  trapChain(W, chain.replace(/^window\./, ''), (owner, prop) => {
    const d = $getOwnPropertyDescriptor(owner, prop);
    if (d && d.configurable === false) return;
    let value = owner[prop];
    try {
      $defineProperty(owner, prop, {
        configurable: true,
        get() { check(); return value; },
        set(a) { check(); value = a; },
      });
    } catch {}
  });
};
//# end

//# addEventListener-defuser
R['addEventListener-defuser'] = function (type, pattern) {
  if (!type && !pattern) return;
  const tType = needleTest(type || '');
  const tPattern = needleTest(pattern || '');
  const proto = EventTarget.prototype;
  proto.addEventListener = new Proxy(proto.addEventListener, {
    apply(target, self, args) {
      let t = '', h = '';
      try { t = String(args[0]); h = fnText(args[1]); } catch {}
      if (tType(t) && tPattern(h)) return;
      return $apply(target, self, args);
    },
  });
};
//# end

//# _timer
// no-setTimeout-if / no-setInterval-if / nano-*-booster comparten esto.
function timerMatch(needle, delay) {
  const tNeedle = needleTest(needle || '');
  let notDelay = false, d = delay;
  if (typeof d === 'string' && d.startsWith('!')) { notDelay = true; d = d.slice(1); }
  const nDelay = d === undefined || d === '' ? undefined : parseInt(d, 10);
  return (fn, ms) => {
    if (!tNeedle(fnText(fn))) return false;
    if (nDelay === undefined || isNaN(nDelay)) return true;
    return (Number(ms) === nDelay) !== notDelay;
  };
}
//# end

//# no-setTimeout-if needs=_timer
R['no-setTimeout-if'] = function (needle, delay) {
  const m = timerMatch(needle, delay);
  W.setTimeout = new Proxy(W.setTimeout, {
    apply(target, self, args) {
      if (m(args[0], args[1])) args[0] = function () {};
      return $apply(target, self, args);
    },
  });
};
//# end

//# no-setInterval-if needs=_timer
R['no-setInterval-if'] = function (needle, delay) {
  const m = timerMatch(needle, delay);
  W.setInterval = new Proxy(W.setInterval, {
    apply(target, self, args) {
      if (m(args[0], args[1])) args[0] = function () {};
      return $apply(target, self, args);
    },
  });
};
//# end

//# no-requestAnimationFrame-if needs=_timer
R['no-requestAnimationFrame-if'] = function (needle) {
  const m = timerMatch(needle);
  W.requestAnimationFrame = new Proxy(W.requestAnimationFrame, {
    apply(target, self, args) {
      if (m(args[0])) args[0] = function () {};
      return $apply(target, self, args);
    },
  });
};
//# end

//# nano-setInterval-booster
R['nano-setInterval-booster'] = function (needle, delay, boost) {
  nanoBoost('setInterval', needle, delay, boost);
};
function nanoBoost(name, needle, delay, boost) {
  const t = needleTest(needle === '*' ? '' : needle || '');
  const target = parseInt(delay || '1000', 10);
  let b = parseFloat(boost || '0.05');
  if (isNaN(b) || !isFinite(b)) b = 0.05;
  b = Math.min(Math.max(b, 0.001), 50);
  W[name] = new Proxy(W[name], {
    apply(fn, self, args) {
      if (t(fnText(args[0])) && (delay === '*' || Number(args[1]) === target)) args[1] = Number(args[1]) * b;
      return $apply(fn, self, args);
    },
  });
}
//# end

//# nano-setTimeout-booster needs=nano-setInterval-booster
R['nano-setTimeout-booster'] = function (needle, delay, boost) {
  nanoBoost('setTimeout', needle, delay, boost);
};
//# end

//# no-window-open-if
R['no-window-open-if'] = function (pattern) {
  const t = needleTest(pattern || '');
  // Una ventana falsa: los scripts que hacen w.location = ... no fallan.
  const decoy = () => {
    const fake = new Proxy(function () {}, {
      get(_, p) {
        if (p === 'closed') return false;
        if (p === 'window' || p === 'self' || p === 'top' || p === 'opener' || p === 'document' || p === 'location') return fake;
        if (p === Symbol.toPrimitive) return () => '';
        return function () { return fake; };
      },
      set() { return true; },
      apply() { return fake; },
    });
    return fake;
  };
  W.open = new Proxy(W.open, {
    apply(target, self, args) {
      const url = args.length ? String(args[0]) : '';
      if (!t(url)) return $apply(target, self, args);
      return decoy();
    },
  });
};
//# end

//# _dom
// Aplica fn ahora y cada vez que cambie el DOM (agrupado por fotograma) hasta que
// la página termine de cargar; con "stay", para siempre.
function onDomChanges(fn, behavior) {
  let pending = false;
  const run = () => { pending = false; try { fn(); } catch {} };
  const mo = new MutationObserver(() => {
    if (pending) return;
    pending = true;
    requestAnimationFrame(run);
  });
  const start = () => {
    run();
    mo.observe(document, { childList: true, subtree: true, attributes: true });
    if (!/\bstay\b/.test(behavior || '')) W.addEventListener('load', () => { run(); setTimeout(() => mo.disconnect(), 2000); }, { once: true });
  };
  if (/\bcomplete\b/.test(behavior || '')) whenReady(start, 'complete');
  else if (document.documentElement) start();
  else whenReady(start, 'interactive');
}
//# end

//# remove-attr needs=_dom
R['remove-attr'] = function (attrs, selector, behavior) {
  if (!attrs) return;
  const list = attrs.split(/\s*\|\s*/).filter(Boolean);
  const sel = selector || list.map((a) => `[${CSS.escape(a)}]`).join(',');
  onDomChanges(() => {
    for (const el of document.querySelectorAll(sel)) for (const a of list) el.removeAttribute(a);
  }, behavior);
};
//# end

//# remove-class needs=_dom
R['remove-class'] = function (classes, selector, behavior) {
  if (!classes) return;
  const list = classes.split(/\s*\|\s*/).filter(Boolean);
  const sel = selector || list.map((c) => '.' + CSS.escape(c)).join(',');
  onDomChanges(() => {
    for (const el of document.querySelectorAll(sel)) el.classList.remove(...list);
  }, behavior);
};
//# end

//# remove-node-text
R['remove-node-text'] = function (nodeName, needle) {
  if (!nodeName || !needle) return;
  const reName = toRegex(nodeName);
  const reText = toRegex(needle);
  const check = (node) => {
    if (!reName.test(node.nodeName.toLowerCase())) return;
    if (!reText.test(node.textContent)) return;
    if (node.nodeType === 3) node.textContent = '';
    else node.remove();
  };
  const mo = new MutationObserver((muts) => {
    for (const m of muts) for (const n of m.addedNodes) check(n);
  });
  mo.observe(document, { childList: true, subtree: true });
  W.addEventListener('load', () => mo.disconnect(), { once: true });
};
//# end

//# _fakeResponse
function fakeBody(kind) {
  switch (kind) {
    case 'emptyObj': return '{}';
    case 'emptyArr': return '[]';
    case 'emptyStr': case '': case undefined: return '';
    default: return '';
  }
}
//# end

//# no-fetch-if needs=_fakeResponse
R['no-fetch-if'] = function (props, responseBody) {
  if (props === undefined) return;
  const match = propsMatcher(props);
  W.fetch = new Proxy(W.fetch, {
    apply(target, self, args) {
      let d;
      try { d = fetchDetails(args); } catch { return $apply(target, self, args); }
      if (!match(d)) return $apply(target, self, args);
      const r = new Response(fakeBody(responseBody), { status: 200, statusText: 'OK', headers: { 'Content-Type': 'text/plain' } });
      try { $defineProperty(r, 'url', { value: d.url }); } catch {}
      return Promise.resolve(r);
    },
  });
};
//# end

//# _xhr
// Engancha XMLHttpRequest una sola vez. Los scriptlets añaden:
//   send(xhr, d)       → false = la petición no sale (no-xhr-if)
//   transform(text, d) → texto nuevo de la respuesta (json-prune, replace, xml/m3u-prune)
// La respuesta se transforma en los getters del prototipo: así la web la lee ya
// limpia aunque sus manejadores se registraran antes que los nuestros.
const xhrHooks = [];
let xhrHooked = false;
function hookXHR(hook) {
  xhrHooks.push(hook);
  if (xhrHooked) return;
  xhrHooked = true;
  const P = XMLHttpRequest.prototype;
  const reqs = new WeakMap();
  const cache = new WeakMap();
  P.open = new Proxy(P.open, {
    apply(target, xhr, args) {
      let url = String(args[1]);
      try { url = new URL(url, location.href).href; } catch {}
      reqs.set(xhr, { method: String(args[0] || 'GET').toUpperCase(), url });
      cache.delete(xhr);
      return $apply(target, xhr, args);
    },
  });
  P.send = new Proxy(P.send, {
    apply(target, xhr, args) {
      const d = reqs.get(xhr);
      if (d) for (const h of xhrHooks) if (h.send && h.send(xhr, d) === false) return;
      return $apply(target, xhr, args);
    },
  });
  const rDesc = $getOwnPropertyDescriptor(P, 'response');
  const tDesc = $getOwnPropertyDescriptor(P, 'responseText');
  const transformed = (xhr) => {
    if (xhr.readyState !== 4) return null;
    if (cache.has(xhr)) return cache.get(xhr);
    const d = reqs.get(xhr);
    let out = null;
    if (d) {
      const type = xhr.responseType;
      let text = null;
      try {
        if (type === '' || type === 'text') text = tDesc.get.call(xhr);
        else if (type === 'json') text = JSON.stringify(rDesc.get.call(xhr));
      } catch {}
      if (typeof text === 'string') {
        let t = text;
        for (const h of xhrHooks) {
          if (!h.transform) continue;
          try { const r = h.transform(t, d); if (typeof r === 'string') t = r; } catch {}
        }
        if (t !== text) {
          let json;
          if (type === 'json') { try { json = JSON.parse(t); } catch { json = null; } }
          out = { text: t, json };
        }
      }
    }
    cache.set(xhr, out);
    return out;
  };
  try {
    $defineProperty(P, 'responseText', {
      configurable: true, enumerable: true,
      get() { const c = transformed(this); return c ? c.text : tDesc.get.call(this); },
    });
    $defineProperty(P, 'response', {
      configurable: true, enumerable: true,
      get() {
        const c = transformed(this);
        if (!c) return rDesc.get.call(this);
        return this.responseType === 'json' ? c.json : c.text;
      },
    });
  } catch {}
}
// fetch: devuelve una Response nueva con el texto cambiado (mismas cabeceras y estado).
function hookFetchText(match, transform) {
  W.fetch = new Proxy(W.fetch, {
    apply(target, self, args) {
      const p = $apply(target, self, args);
      let d;
      try { d = fetchDetails(args); } catch { return p; }
      if (!match(d)) return p;
      return p.then((resp) => resp.clone().text().then((text) => {
        let out = text;
        try { out = transform(text, d); } catch {}
        if (typeof out !== 'string' || out === text) return resp;
        const r = new Response(out, { status: resp.status, statusText: resp.statusText, headers: resp.headers });
        try { $defineProperty(r, 'url', { value: resp.url }); $defineProperty(r, 'type', { value: resp.type }); } catch {}
        return r;
      }).catch(() => resp));
    },
  });
}
//# end

//# no-xhr-if needs=_xhr,_fakeResponse
R['no-xhr-if'] = function (props, directive) {
  if (props === undefined) return;
  fakeXHR(propsMatcher(props), fakeBody(directive));
};
function fakeXHR(match, body) {
  hookXHR({
    send(xhr, d) {
      if (!match(d)) return true;
      const set = (k, v) => { try { $defineProperty(xhr, k, { configurable: true, get: () => v }); } catch {} };
      set('readyState', 4); set('status', 200); set('statusText', 'OK'); set('responseURL', d.url);
      let json = null;
      try { json = JSON.parse(body); } catch {}
      set('response', xhr.responseType === 'json' ? json : body); set('responseText', body);
      setTimeout(() => {
        for (const t of ['readystatechange', 'load', 'loadend']) xhr.dispatchEvent(new Event(t));
      }, 1);
      return false;
    },
  });
}
//# end

//# _prune
// Rutas al estilo uBO: "a.b.c", "[]" = cada elemento, "*" = cada clave,
// "[-]" = quitar del array el elemento en el que existe el resto de la ruta.
function pruneObject(obj, prunePaths, requiredPaths) {
  if (obj === null || typeof obj !== 'object') return false;
  if (requiredPaths.length && !requiredPaths.every((p) => pathExists(obj, p.split('.')))) return false;
  let changed = false;
  for (const p of prunePaths) if (prunePath(obj, p.split('.'))) changed = true;
  return changed;
}
function pathExists(o, segs) {
  if (segs.length === 0) return true;
  if (o === null || typeof o !== 'object') return false;
  const [s, ...rest] = segs;
  if (s === '[]' || s === '[-]' || s === '*') {
    const vals = Array.isArray(o) ? o : Object.values(o);
    return vals.some((v) => pathExists(v, rest));
  }
  return Object.prototype.hasOwnProperty.call(o, s) && pathExists(o[s], rest);
}
function prunePath(o, segs) {
  if (o === null || typeof o !== 'object' || segs.length === 0) return false;
  const [s, ...rest] = segs;
  if (s === '[-]' && Array.isArray(o)) {
    let changed = false;
    for (let i = o.length - 1; i >= 0; i--) if (pathExists(o[i], rest)) { o.splice(i, 1); changed = true; }
    return changed;
  }
  if (s === '[]' || s === '*' || s === '[-]') {
    let changed = false;
    for (const k of Object.keys(o)) {
      if (rest.length === 0) { delete o[k]; changed = true; } else if (prunePath(o[k], rest)) changed = true;
    }
    return changed;
  }
  if (!Object.prototype.hasOwnProperty.call(o, s)) return false;
  if (rest.length === 0) { delete o[s]; return true; }
  return prunePath(o[s], rest);
}
function splitPaths(s) { return (s || '').trim().split(/\s+/).filter(Boolean); }
//# end

//# json-prune needs=_prune
R['json-prune'] = function (rawPrune, rawRequired, stackNeedle) {
  const prune = splitPaths(rawPrune);
  const required = splitPaths(rawRequired);
  if (prune.length === 0) return;
  const reStack = stackNeedle ? toRegex(stackNeedle) : null;
  const stackOK = () => {
    if (!reStack) return true;
    try { return reStack.test(new Error().stack || ''); } catch { return false; }
  };
  JSON.parse = new Proxy(JSON.parse, {
    apply(target, self, args) {
      const obj = $apply(target, self, args);
      if (stackOK()) pruneObject(obj, prune, required);
      return obj;
    },
  });
  Response.prototype.json = new Proxy(Response.prototype.json, {
    apply(target, self, args) {
      return $apply(target, self, args).then((obj) => { pruneObject(obj, prune, required); return obj; });
    },
  });
};
//# end

//# json-prune-fetch-response needs=_prune
R['json-prune-fetch-response'] = function (rawPrune, rawRequired, ...rest) {
  const prune = splitPaths(rawPrune);
  const required = splitPaths(rawRequired);
  if (prune.length === 0) return;
  const match = propsMatcher(extraArgs(rest).propsToMatch);
  W.fetch = new Proxy(W.fetch, {
    apply(target, self, args) {
      const p = $apply(target, self, args);
      let d;
      try { d = fetchDetails(args); } catch { return p; }
      if (!match(d)) return p;
      return p.then((resp) => resp.clone().json().then((obj) => {
        if (!pruneObject(obj, prune, required)) return resp;
        const r = new Response(JSON.stringify(obj), { status: resp.status, statusText: resp.statusText, headers: resp.headers });
        try { $defineProperty(r, 'url', { value: resp.url }); $defineProperty(r, 'type', { value: resp.type }); } catch {}
        return r;
      }).catch(() => resp));
    },
  });
};
//# end

//# json-prune-xhr-response needs=_prune,_xhr
R['json-prune-xhr-response'] = function (rawPrune, rawRequired, ...rest) {
  const prune = splitPaths(rawPrune);
  const required = splitPaths(rawRequired);
  if (prune.length === 0) return;
  const match = propsMatcher(extraArgs(rest).propsToMatch);
  hookXHR({
    transform(text, d) {
      if (!match(d)) return;
      const obj = JSON.parse(text);
      if (pruneObject(obj, prune, required)) return JSON.stringify(obj);
    },
  });
};
//# end

//# _replace
function replacer(pattern, replacement) {
  if (!pattern) return null;
  const m = /^\/(.+)\/([gimsu]*)$/.exec(pattern);
  let re;
  try { re = m ? new RegExp(m[1], m[2].includes('g') ? m[2] : m[2] + 'g') : new RegExp(pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g'); } catch { return null; }
  return (text) => text.replace(re, replacement || '');
}
//# end

//# trusted-replace-fetch-response needs=_replace,_xhr
R['trusted-replace-fetch-response'] = function (pattern, replacement, props) {
  const rep = replacer(pattern, replacement);
  if (!rep) return;
  hookFetchText(propsMatcher(props), (text) => rep(text));
};
//# end

//# trusted-replace-xhr-response needs=_replace,_xhr
R['trusted-replace-xhr-response'] = function (pattern, replacement, props) {
  const rep = replacer(pattern, replacement);
  if (!rep) return;
  const match = propsMatcher(props);
  hookXHR({ transform(text, d) { if (match(d)) return rep(text); } });
};
//# end

//# _storage
function storageValue(raw, trusted) {
  if (raw === '$remove$') return null;
  if (trusted) {
    if (raw === '$now$') return String(Date.now());
    if (raw === '$currentDate$') return String(new Date());
    if (raw === '$currentISODate$') return new Date().toISOString();
    return raw;
  }
  const safe = ['undefined', 'false', 'true', 'null', '', 'yes', 'no', 'on', 'off', 'ok', 'accept', 'accepted', 'reject', 'rejected', 'allow', 'deny', 'emptyArr', 'emptyObj', '[]', '{}', '$now$'];
  if (raw === '$now$') return String(Date.now());
  if (raw === 'emptyArr') return '[]';
  if (raw === 'emptyObj') return '{}';
  if (safe.includes(raw) || /^\d+$/.test(raw) && raw.length <= 8) return raw;
  return undefined;
}
function setStorage(which, key, raw, trusted) {
  if (!key) return;
  const v = storageValue(raw ?? '', trusted);
  if (v === undefined) return;
  try {
    const st = W[which];
    if (v === null) st.removeItem(key);
    else st.setItem(key, v);
  } catch {}
}
//# end

//# set-local-storage-item needs=_storage
R['set-local-storage-item'] = (k, v) => setStorage('localStorage', k, v, false);
//# end

//# set-session-storage-item needs=_storage
R['set-session-storage-item'] = (k, v) => setStorage('sessionStorage', k, v, false);
//# end

//# trusted-set-local-storage-item needs=_storage
R['trusted-set-local-storage-item'] = (k, v) => setStorage('localStorage', k, v, true);
//# end

//# remove-cookie
R['remove-cookie'] = function (needle) {
  const t = needleTest(needle || '');
  const remove = () => {
    for (const c of document.cookie.split(/\s*;\s*/)) {
      const name = c.split('=')[0];
      if (!name || !t(name)) continue;
      const parts = location.hostname.split('.');
      const expire = '=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
      for (let i = 0; i < parts.length - 1; i++) {
        const dom = parts.slice(i).join('.');
        document.cookie = `${name}${expire}; path=/; domain=${dom}`;
        document.cookie = `${name}${expire}; path=/; domain=.${dom}`;
      }
      document.cookie = `${name}${expire}; path=/`;
      document.cookie = `${name}${expire}`;
    }
  };
  remove();
  W.addEventListener('beforeunload', remove);
  document.addEventListener('visibilitychange', remove);
};
//# end

//# set-cookie
R['set-cookie'] = function (name, value, path) {
  if (!name) return;
  const ok = ['true', 'false', 'yes', 'no', 'y', 'n', 'ok', 'on', 'off', 'accept', 'accepted', 'reject', 'rejected', 'allow', 'allowed', 'deny', 'denied', 'necessary', 'required', 'hide', 'hidden', 'essential', 'nonessential', 'checked', 'unchecked', '', 'emptyStr'];
  if (!ok.includes((value || '').toLowerCase()) && !/^\d{1,8}$/.test(value || '')) return;
  setCookie(name, value === 'emptyStr' ? '' : value, path);
};
function setCookie(name, value, path, expiresSec) {
  const cur = document.cookie.split(/\s*;\s*/).find((c) => c.split('=')[0] === name);
  if (cur && cur.slice(name.length + 1) === encodeURIComponent(value)) return;
  let s = `${encodeURIComponent(name)}=${encodeURIComponent(value)}; path=${path === 'none' ? '' : path || '/'}`;
  if (expiresSec) s += `; max-age=${expiresSec}`;
  try { document.cookie = s; } catch {}
}
//# end

//# trusted-set-cookie needs=set-cookie
R['trusted-set-cookie'] = function (name, value, offset, path) {
  if (!name) return;
  let v = value || '';
  if (v === '$now$') v = String(Date.now());
  else if (v === '$currentDate$') v = new Date().toUTCString();
  const days = /^(\d+)\s*(d|day|days)?$/.exec(offset || '');
  setCookie(name, v, path, days ? parseInt(days[1], 10) * (days[2] ? 86400 : 1) : undefined);
};
//# end

//# nowebrtc
R['nowebrtc'] = function () {
  const names = ['RTCPeerConnection', 'webkitRTCPeerConnection'];
  for (const n of names) {
    if (typeof W[n] !== 'function') continue;
    const Fake = function () {};
    Fake.prototype = {
      close() {}, createDataChannel() {}, createOffer() { return Promise.resolve(); },
      setRemoteDescription() { return Promise.resolve(); }, addEventListener() {}, removeEventListener() {},
      toString() { return '[object RTCPeerConnection]'; },
    };
    W[n] = Fake;
  }
};
//# end

//# noeval
R['noeval'] = function () {
  W.eval = new Proxy(W.eval, { apply() { return undefined; } });
};
//# end

//# noeval-if
R['noeval-if'] = function (needle) {
  const t = needleTest(needle || '');
  W.eval = new Proxy(W.eval, {
    apply(target, self, args) {
      if (t(String(args[0]))) return undefined;
      return $apply(target, self, args);
    },
  });
};
//# end

//# popads-dummy
R['popads-dummy'] = function () {
  try {
    delete W.PopAds; delete W.popns;
    $defineProperty(W, 'PopAds', { value: {} });
    $defineProperty(W, 'popns', { value: {} });
  } catch {}
};
//# end

//# _fab
function fakeAdblockDetector(names) {
  const Fake = function () {};
  Fake.prototype = {
    check() { return this; }, clearEvent() { return this; }, emitEvent() { return this; },
    on(detected, fn) { if (!detected && typeof fn === 'function') setTimeout(fn, 1); return this; },
    onDetected() { return this; },
    onNotDetected(fn) { if (typeof fn === 'function') setTimeout(fn, 1); return this; },
    setOption() { return this; },
  };
  const inst = new Fake();
  for (const [cls, obj] of names) {
    try { $defineProperty(W, cls, { value: Fake }); $defineProperty(W, obj, { value: inst }); } catch {}
  }
}
//# end

//# fuckadblock-defuser needs=_fab
R['fuckadblock-defuser'] = () => fakeAdblockDetector([['FuckAdBlock', 'fuckAdBlock']]);
//# end

//# bab-defuser needs=_fab
R['bab-defuser'] = () => fakeAdblockDetector([['BlockAdBlock', 'blockAdBlock']]);
//# end

//# href-sanitizer needs=_dom
R['href-sanitizer'] = function (selector, source) {
  if (!selector) return;
  const src = source || 'text';
  const valid = (u) => { try { const x = new URL(u, location.href); return /^https?:$/.test(x.protocol) ? x.href : ''; } catch { return ''; } };
  onDomChanges(() => {
    for (const a of document.querySelectorAll(selector)) {
      if (!(a instanceof HTMLAnchorElement)) continue;
      let v = '';
      if (src === 'text') v = a.textContent.trim();
      else if (src.startsWith('[') && src.endsWith(']')) v = a.getAttribute(src.slice(1, -1)) || '';
      else if (src.startsWith('?')) { try { v = new URL(a.href).searchParams.get(src.slice(1)) || ''; } catch {} }
      v = valid(v);
      if (v && a.href !== v) a.setAttribute('href', v);
    }
  });
};
//# end

//# prevent-refresh
R['prevent-refresh'] = function () {
  const defuse = () => {
    for (const m of document.querySelectorAll('meta[http-equiv="refresh" i]')) m.remove();
  };
  whenReady(defuse, 'interactive');
  defuse();
};
//# end

//# disable-newtab-links
R['disable-newtab-links'] = function () {
  document.addEventListener('click', (ev) => {
    for (let t = ev.target; t; t = t.parentNode) {
      if (t.localName === 'a' && t.hasAttribute('target')) { ev.stopPropagation(); ev.preventDefault(); return; }
    }
  }, { capture: true });
};
//# end

//# window-close-if
R['window-close-if'] = function (needle) {
  if (!needleTest(needle || '')(location.href)) return;
  W.close();
  // Si no se puede cerrar (no la abrió un script), al menos que quede vacía.
  setTimeout(() => { if (!W.closed) location.replace('about:blank'); }, 50);
};
//# end

//# trusted-click-element
R['trusted-click-element'] = function (selectors, extraMatch, delay) {
  if (!selectors) return;
  const list = selectors.split(/\s*,\s*(?![^[(]*[\])])/).filter(Boolean);
  const wait = Math.max(parseInt(delay || '1', 10) || 1, 1);
  const deadline = Date.now() + 10000;
  let i = 0;
  const step = () => {
    if (i >= list.length || Date.now() > deadline) { mo.disconnect(); return; }
    let el = null;
    try { el = document.querySelector(list[i]); } catch { i = list.length; }
    if (!el) return;
    i++;
    setTimeout(() => { try { el.click(); } catch {} step(); }, wait);
  };
  const mo = new MutationObserver(step);
  whenReady(() => {
    mo.observe(document, { childList: true, subtree: true });
    step();
    setTimeout(() => mo.disconnect(), 10000);
  }, 'interactive');
};
//# end

//# trusted-prevent-xhr needs=no-xhr-if
R['trusted-prevent-xhr'] = function (props, body) {
  if (props === undefined) return;
  fakeXHR(propsMatcher(props), body || '');
};
//# end

//# trusted-prevent-fetch
R['trusted-prevent-fetch'] = function (props, body) {
  if (props === undefined) return;
  const match = propsMatcher(props);
  W.fetch = new Proxy(W.fetch, {
    apply(target, self, args) {
      let d;
      try { d = fetchDetails(args); } catch { return $apply(target, self, args); }
      if (!match(d)) return $apply(target, self, args);
      return Promise.resolve(new Response(body || '', { status: 200, statusText: 'OK' }));
    },
  });
};
//# end

//# xml-prune needs=_xhr
// Quita nodos de respuestas XML (VAST, DASH .mpd): los anuncios de vídeo que van
// metidos en el propio manifiesto del vídeo.
R['xml-prune'] = function (selector, selectorCheck, urlPattern) {
  if (!selector) return;
  const urlTest = urlPattern ? needleTest(urlPattern) : () => true;
  const prune = (text) => {
    if (!/^\s*</.test(text)) return;
    const doc = new DOMParser().parseFromString(text, 'text/xml');
    if (doc.querySelector('parsererror')) return;
    if (selectorCheck && !doc.querySelector(selectorCheck)) return;
    let nodes = [];
    const xp = /^xpath\((.+)\)$/.exec(selector);
    if (xp) {
      const r = doc.evaluate(xp[1], doc, null, XPathResult.UNORDERED_NODE_SNAPSHOT_TYPE, null);
      for (let i = 0; i < r.snapshotLength; i++) nodes.push(r.snapshotItem(i));
    } else nodes = [...doc.querySelectorAll(selector)];
    if (!nodes.length) return;
    for (const n of nodes) {
      if (n.nodeType === 2) n.ownerElement?.removeAttribute(n.nodeName);
      else n.remove();
    }
    return new XMLSerializer().serializeToString(doc);
  };
  hookXHR({ transform(text, d) { if (urlTest(d.url)) return prune(text); } });
  hookFetchText((d) => urlTest(d.url), prune);
};
//# end

//# m3u-prune needs=_xhr
// Quita los segmentos de anuncio de las listas HLS (.m3u8).
R['m3u-prune'] = function (pattern, urlPattern) {
  if (!pattern) return;
  const urlTest = urlPattern ? needleTest(urlPattern) : () => true;
  const m = /^\/(.+)\/([gimsu]*)$/.exec(pattern);
  const multi = m && /[gm]/.test(m[2]);
  let re;
  try { re = m ? new RegExp(m[1], multi ? (m[2].includes('g') ? m[2] : m[2] + 'g') : m[2]) : toRegex(pattern); } catch { return; }
  const prune = (text) => {
    if (!text.trimStart().startsWith('#EXTM3U')) return;
    if (multi) return text.replace(re, '\n');
    const lines = text.split(/\n/);
    const out = [];
    for (const line of lines) {
      if (!re.test(line)) { out.push(line); continue; }
      // El #EXTINF que describe el segmento quitado también sobra.
      if (out.length && /^#EXTINF/.test(out[out.length - 1])) out.pop();
    }
    return out.join('\n');
  };
  hookXHR({ transform(text, d) { if (urlTest(d.url)) return prune(text); } });
  hookFetchText((d) => urlTest(d.url), prune);
};
//# end

//# replace-node-text
// Cambia texto dentro de nodos (normalmente <script> en línea) antes de que se ejecuten.
R['replace-node-text'] = function (nodeName, pattern, replacement, ...rest) {
  if (!nodeName || !pattern) return;
  const reName = toRegex(nodeName);
  const rep = (() => {
    const m = /^\/(.+)\/([gimsu]*)$/.exec(pattern);
    try { return m ? new RegExp(m[1], m[2]) : new RegExp(pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g'); } catch { return null; }
  })();
  if (!rep) return;
  const cond = extraArgs(rest).condition;
  const reCond = cond ? toRegex(cond) : null;
  const check = (node) => {
    if (!reName.test(node.nodeName.toLowerCase())) return;
    const t = node.textContent;
    if (reCond && !reCond.test(t)) return;
    const out = t.replace(rep, replacement || '');
    if (out !== t) node.textContent = out;
  };
  const mo = new MutationObserver((muts) => {
    for (const m of muts) for (const n of m.addedNodes) check(n);
  });
  mo.observe(document, { childList: true, subtree: true });
  W.addEventListener('load', () => mo.disconnect(), { once: true });
};
//# end

//# trusted-prevent-dom-bypass
// Las webs crean un iframe para sacar un fetch/JSON.parse "limpio". Le ponemos el nuestro.
R['trusted-prevent-dom-bypass'] = function (methodChain, propChain) {
  if (!methodChain) return;
  const [owner, prop] = resolveChain(methodChain);
  if (!owner || typeof owner[prop] !== 'function') return;
  const fix = (node) => {
    const frames = node instanceof HTMLIFrameElement ? [node] : node.querySelectorAll ? [...node.querySelectorAll('iframe')] : [];
    for (const f of frames) {
      let w;
      try { w = f.contentWindow; } catch { continue; }
      if (!w) continue;
      try {
        if (!propChain) continue;
        const [src, sp] = resolveChain(propChain);
        let dst = w;
        const parts = propChain.replace(/^window\./, '').split('.');
        for (let i = 0; i < parts.length - 1; i++) dst = dst[parts[i]];
        if (src && dst) dst[parts[parts.length - 1]] = src[sp];
      } catch {}
    }
  };
  owner[prop] = new Proxy(owner[prop], {
    apply(target, self, args) {
      const r = $apply(target, self, args);
      try { if (args[0] instanceof Node) fix(args[0]); } catch {}
      return r;
    },
  });
};
//# end

//# trusted-replace-argument
// Cambia un argumento de una función de la página (opcionalmente solo si su texto coincide).
R['trusted-replace-argument'] = function (chain, argpos, argraw, ...rest) {
  if (!chain || argpos === undefined) return;
  const [owner, prop] = resolveChain(chain);
  if (!owner || typeof owner[prop] !== 'function') return;
  const pos = parseInt(argpos, 10);
  if (isNaN(pos)) return;
  const cond = extraArgs(rest).condition;
  const reCond = cond ? toRegex(cond) : null;
  let make;
  const repl = /^repl\((.+)\)$/.exec(argraw || '');
  if (repl) {
    let o;
    try { o = JSON.parse(repl[1]); } catch { return; }
    let re;
    try { re = new RegExp(o.regex, 'g'); } catch { return; }
    make = (old) => typeof old === 'string' ? old.replace(re, o.replacement ?? '') : old;
  } else {
    let v = constValue(argraw);
    if (v === NOPE) { try { v = JSON.parse(argraw); } catch { v = argraw; } }
    make = () => v;
  }
  owner[prop] = new Proxy(owner[prop], {
    apply(target, self, args) {
      const i = pos < 0 ? args.length + pos : pos;
      if (i >= 0 && i < args.length && (!reCond || reCond.test(String(args[i])))) args[i] = make(args[i]);
      return $apply(target, self, args);
    },
  });
};
//# end

//# set-attr needs=_dom
R['set-attr'] = function (selector, attr, value) {
  if (!selector || !attr) return;
  const v = value ?? '';
  // Valores seguros: vacío, true/false, números o copiar otro atributo ("[otro]").
  if (!/^(|true|false|-?\d+|\[[\w-]+\])$/.test(v)) return;
  setAttr(selector, attr, v);
};
function setAttr(selector, attr, v) {
  const copy = /^\[([\w-]+)\]$/.exec(v);
  onDomChanges(() => {
    for (const el of document.querySelectorAll(selector)) {
      const nv = copy ? el.getAttribute(copy[1]) : v;
      if (nv !== null && el.getAttribute(attr) !== nv) el.setAttribute(attr, nv);
    }
  });
}
//# end

//# trusted-set-attr needs=set-attr
R['trusted-set-attr'] = function (selector, attr, value) {
  if (!selector || !attr) return;
  setAttr(selector, attr, value ?? '');
};
//# end

//# trusted-set-session-storage-item needs=_storage
R['trusted-set-session-storage-item'] = (k, v) => setStorage('sessionStorage', k, v, true);
//# end

//# set-cookie-reload needs=set-cookie
R['set-cookie-reload'] = function (name, value, path) {
  if (!name) return;
  const before = document.cookie;
  R['set-cookie'](name, value, path);
  if (document.cookie !== before) location.reload();
};
//# end

//# tb-video-ads
// Propio de TurboBLOCK: si un anuncio de vídeo llega a reproducirse (YouTube cambia a
// menudo y las listas tardan en ponerse al día), se silencia, se salta al final y se
// pulsa «Saltar». En Twitch, donde el anuncio va dentro del propio directo, se
// silencia y se tapa hasta que acaba.
R['tb-video-ads'] = function (site) {
  const SKIP = '.ytp-ad-skip-button,.ytp-ad-skip-button-modern,.ytp-skip-ad-button,.ytp-ad-skip-button-slot button,.ytp-ad-overlay-close-button,.ytp-ad-skip-button-container button';
  const state = new WeakMap();
  const restore = (v) => {
    const s = state.get(v);
    if (!s) return;
    state.delete(v);
    v.muted = s.muted;
    if (v.playbackRate !== s.rate) v.playbackRate = s.rate;
  };
  const remember = (v) => { if (!state.has(v)) state.set(v, { muted: v.muted, rate: v.playbackRate }); };
  // Saltar al final si se puede; si no (sin rango que buscar), a toda velocidad.
  const jumpToEnd = (v) => {
    const sk = v.seekable;
    const end = sk && sk.length ? sk.end(sk.length - 1) : 0;
    if (isFinite(v.duration) && v.duration > 0 && end > v.currentTime + 0.3) {
      try { v.currentTime = Math.min(v.duration, end) - 0.05; return; } catch {}
    }
    if (v.playbackRate < 16) { try { v.playbackRate = 16; } catch {} }
  };

  const youtube = () => {
    const player = document.querySelector('#movie_player.ad-showing, .html5-video-player.ad-showing, #movie_player.ad-interrupting');
    const v = (player || document).querySelector('video.html5-main-video, video');
    if (!player) { if (v) restore(v); return; }
    if (!v) return;
    remember(v);
    v.muted = true;
    jumpToEnd(v);
    for (const b of document.querySelectorAll(SKIP)) { try { b.click(); } catch {} }
  };

  let cover = null;
  const twitch = () => {
    const ad = document.querySelector('[data-a-target="video-ad-label"],[data-a-target="video-ad-countdown"],[data-test-selector="ad-banner-default-text"]');
    const v = document.querySelector('video');
    if (!v) return;
    const box = v.closest('.video-player__container, .video-player, [data-a-target="video-player"]') || v.parentElement;
    if (ad) {
      remember(v);
      v.muted = true;
      if (!cover && box) {
        cover = document.createElement('div');
        cover.textContent = 'Anuncio de Twitch · silenciado por TurboBLOCK';
        cover.style.cssText = 'position:absolute;inset:0;z-index:9;display:grid;place-items:center;background:#0a0b12;color:#9196ae;' +
          'font:600 15px "Segoe UI",system-ui,sans-serif;letter-spacing:.2px;pointer-events:none';
        box.appendChild(cover);
      }
    } else {
      restore(v);
      if (cover) { cover.remove(); cover = null; }
    }
  };

  // Reproductores genéricos (JW Player, Video.js con IMA).
  const generic = () => {
    for (const p of document.querySelectorAll('.jw-flag-ads, .vjs-ad-playing')) {
      const v = p.querySelector('video');
      if (!v) continue;
      remember(v);
      v.muted = true;
      jumpToEnd(v);
      for (const b of p.querySelectorAll('.jw-skip, .vjs-ad-skip, [class*="skip-ad"], [class*="ad-skip"]')) { try { b.click(); } catch {} }
    }
  };

  const tick = site === 'youtube' ? youtube : site === 'twitch' ? twitch : generic;
  // Un vistazo cada 250 ms solo mientras hay un <video>: barato y sin observar todo el DOM.
  setInterval(() => { if (document.querySelector('video')) { try { tick(); } catch {} } }, 250);
};
//# end

//# run
// Qué scriptlets tocan aquí: el dominio, sus dominios padre y sus entidades ("google.*").
(function () {
  if (!host) return;
  const labels = host.split('.');
  const keys = [];
  for (let i = 0; i < labels.length; i++) keys.push(labels.slice(i).join('.'));
  if (!/^\d+\.\d+\.\d+\.\d+$/.test(host)) {
    for (let i = 0; i < labels.length - 1; i++) {
      for (let j = i; j < labels.length - 1; j++) keys.push(labels.slice(i, j + 1).join('.') + '.*');
    }
  }
  const off = new Set();
  for (const k of keys) {
    const x = X[k];
    if (!x) continue;
    if (x.includes(-1)) return;
    for (const i of x) off.add(i);
  }
  const todo = new Set();
  for (const k of keys) {
    const l = K[k];
    if (l) for (const i of l) if (!off.has(i)) todo.add(i);
  }
  for (const i of todo) {
    const [name, ...args] = S[i];
    const fn = R[name];
    if (fn) { try { fn(...args); } catch {} }
  }
})();
//# end
