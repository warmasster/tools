/* TurboBLOCK · ocultación de elementos (mundo aislado, document_start, todos los marcos).
 *
 * 1. Lee de chrome.storage.local solo las porciones de datos de este dominio
 *    (dominio, dominios padre y entidades tipo "google.*") en una sola llamada.
 * 2. Ocultación genérica sin hoja gigante: mira las clases e ids que van
 *    apareciendo en la página y, si un filtro de Bloom dice que hay selectores
 *    para alguna, pide solo esos.
 * 3. Selectores procedimentales (:has-text, :upward, :remove()…) con un
 *    MutationObserver agrupado por fotograma.
 * Todo va a una hoja de estilos adoptada que la página no ve en el DOM.
 */
(() => {
  if (globalThis.__turboblockCosmetic) return;
  globalThis.__turboblockCosmetic = true;

  let host = location.hostname;
  if (!host) { try { host = new URL(origin).hostname; } catch {} }
  if (!host) return;

  // ---------- utilidades (iguales que lib/util.js) ----------
  const fnv = (s) => {
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 0x01000193); }
    return h >>> 0;
  };
  const hash2 = (s) => {
    let h = 5381;
    for (let i = 0; i < s.length; i++) h = Math.imul(h, 33) ^ s.charCodeAt(i);
    return (h >>> 0) | 1;
  };
  const SPEC_SHARDS = 512, GEN_SHARDS = 512;

  // Claves que aplican aquí: dominio, padres y entidades.
  const labels = host.split('.');
  const keys = [];
  for (let i = 0; i < labels.length; i++) keys.push(labels.slice(i).join('.'));
  if (!/^\d+\.\d+\.\d+\.\d+$/.test(host)) {
    for (let i = 0; i < labels.length - 1; i++) for (let j = i; j < labels.length - 1; j++) keys.push(labels.slice(i, j + 1).join('.') + '.*');
  }
  const keySet = new Set(keys);
  const shardKeys = [...new Set(keys.map((k) => 'c' + (fnv(k) % SPEC_SHARDS)))];

  // ---------- hoja de estilos ----------
  const sheet = new CSSStyleSheet();
  const added = new Set();
  let attached = false;
  const attach = () => {
    const list = document.adoptedStyleSheets;
    if (!list.includes(sheet)) {
      try { document.adoptedStyleSheets = [...list, sheet]; } catch {}
    }
    attached = true;
  };
  const hideRule = (sel) => {
    if (added.has(sel)) return;
    added.add(sel);
    try { sheet.insertRule(`${sel}{display:none!important}`, sheet.cssRules.length); } catch {}
  };
  const cssRule = (rule) => {
    if (added.has(rule)) return;
    added.add(rule);
    try { sheet.insertRule(rule, sheet.cssRules.length); } catch {}
  };

  let unhide = new Set();
  let genericOn = true;

  chrome.storage.local.get([...shardKeys, 'gb', 'cg'], (data) => {
    if (chrome.runtime.lastError || !data) return;
    const hide = [], css = [], proc = [], sla = [];
    let noSpec = false;
    for (const k of keys) {
      const e = data['c' + (fnv(k) % SPEC_SHARDS)]?.[k];
      if (!e) continue;
      if (e.s) noSpec = true;
      if (e.g) genericOn = false;
      if (e.h) hide.push(...e.h);
      if (e.u) for (const u of e.u) unhide.add(u);
      if (e.c) css.push(...e.c);
      if (e.p) proc.push(...e.p);
      if (e.j) sla.push(...e.j);
    }
    if (sla.length) chrome.runtime.sendMessage({ type: 'sla', host, list: sla }).catch(() => {});
    if (!noSpec) {
      for (const s of hide) if (!unhide.has(s)) hideRule(s);
      for (const r of css) cssRule(r);
    }
    for (const [sel, except] of data.cg || []) {
      if (unhide.has(sel) || except.some((d) => keySet.has(d) || host.endsWith('.' + d))) continue;
      hideRule(sel);
    }
    attach();
    if (genericOn && data.gb) startGeneric(data.gb);
    if (!noSpec && proc.length) startProcedural(proc.filter((p) => !unhide.has(p)));
  });

  // ---------- genéricos por clase e id ----------
  function startGeneric(bloom) {
    const bin = atob(bloom.b);
    const mask = bloom.m - 1;
    const maybe = (t) => {
      const h1 = fnv(t), h2 = hash2(t);
      for (let i = 0; i < bloom.k; i++) {
        const idx = (h1 + Math.imul(i, h2)) & mask;
        if (!((bin.charCodeAt(idx >> 3) >> (idx & 7)) & 1)) return false;
      }
      return true;
    };
    const seen = new Set();
    const wanted = new Map(); // shard -> [tokens]
    const queue = [];
    let scheduled = false;

    const collect = (el) => {
      const id = el.id;
      if (id && typeof id === 'string') { const t = '#' + id; if (!seen.has(t)) { seen.add(t); if (maybe(t)) want(t); } }
      const cl = el.classList;
      if (cl) for (let i = 0; i < cl.length; i++) { const t = '.' + cl[i]; if (!seen.has(t)) { seen.add(t); if (maybe(t)) want(t); } }
    };
    const want = (t) => {
      const sk = 'g' + (fnv(t) % GEN_SHARDS);
      const l = wanted.get(sk);
      if (l) l.push(t); else wanted.set(sk, [t]);
    };
    const flush = () => {
      if (!wanted.size) return;
      const req = new Map(wanted);
      wanted.clear();
      chrome.storage.local.get([...req.keys()], (data) => {
        if (chrome.runtime.lastError || !data) return;
        for (const [sk, toks] of req) {
          const shard = data[sk];
          if (!shard) continue;
          for (const t of toks) for (const sel of shard[t] || []) if (!unhide.has(sel)) hideRule(sel);
        }
        if (!document.adoptedStyleSheets.includes(sheet)) attach();
      });
    };
    const process = () => {
      scheduled = false;
      const t0 = performance.now();
      while (queue.length) {
        const n = queue.pop();
        if (n.nodeType !== 1) continue;
        collect(n);
        if (n.firstElementChild) {
          const all = n.querySelectorAll('[id],[class]');
          for (let i = 0; i < all.length; i++) collect(all[i]);
        }
        // No bloquear la página con inserciones enormes: el resto, en el siguiente.
        if (performance.now() - t0 > 8) { schedule(); break; }
      }
      flush();
    };
    const schedule = () => {
      if (scheduled) return;
      scheduled = true;
      (document.readyState === 'loading' ? setTimeout : requestAnimationFrame)(process, 0);
    };
    if (document.documentElement) { queue.push(document.documentElement); schedule(); }
    new MutationObserver((muts) => {
      for (const m of muts) for (const n of m.addedNodes) if (n.nodeType === 1) queue.push(n);
      if (queue.length) schedule();
    }).observe(document, { childList: true, subtree: true });
  }

  // ---------- procedimentales ----------
  const mark = 'tb' + Math.random().toString(36).slice(2, 8);

  function startProcedural(list) {
    const tasks = [];
    for (const src of list) {
      try {
        const t = compile(src);
        if (t) tasks.push(t);
      } catch {}
    }
    if (!tasks.length) return;
    hideRule(`[${mark}]`);
    const styles = new Map();
    let pending = false;
    const run = () => {
      pending = false;
      for (const t of tasks) {
        if (t.path && !t.path.test(location.pathname + location.search)) continue;
        let nodes;
        try { nodes = exec(t.steps, null); } catch { continue; }
        for (const el of nodes) {
          switch (t.action) {
            case 'remove': el.remove(); break;
            case 'style': {
              let a = styles.get(t.arg);
              if (!a) { a = `${mark}s${styles.size}`; styles.set(t.arg, a); cssRule(`[${a}]{${t.arg}}`); }
              el.setAttribute(a, '');
              break;
            }
            case 'remove-attr': for (const x of t.arg.split('|')) el.removeAttribute(x.trim()); break;
            case 'remove-class': for (const x of t.arg.split('|')) el.classList.remove(x.trim()); break;
            default: if (!el.hasAttribute(mark)) el.setAttribute(mark, '');
          }
        }
      }
    };
    const schedule = () => {
      if (pending) return;
      pending = true;
      setTimeout(() => requestAnimationFrame(run), 50);
    };
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', run, { once: true });
    else run();
    new MutationObserver(schedule).observe(document, { childList: true, subtree: true, characterData: true });
  }

  // Parte un selector en pasos: CSS normal y operadores (:has-text(...), …).
  const OPS = ['has-text', 'upward', 'matches-css', 'matches-css-before', 'matches-css-after', 'matches-attr', 'matches-path',
    'min-text-length', 'xpath', 'watch-attr', 'has', 'not', 'remove', 'style', 'remove-attr', 'remove-class'];
  const PROC = /:(has-text|upward|matches-css|matches-css-before|matches-css-after|matches-attr|matches-path|min-text-length|xpath|watch-attr|remove|style|remove-attr|remove-class)\(/;

  function compile(src) {
    const steps = [];
    let action = null, arg = '', path = null;
    let css = '';
    let i = 0;
    while (i < src.length) {
      const c = src[i];
      if (c === '[' || c === '"' || c === "'") { const j = skip(src, i); css += src.slice(i, j); i = j; continue; }
      if (c === ':') {
        const m = /^:([a-z-]+)\(/.exec(src.slice(i));
        if (m && OPS.includes(m[1])) {
          const open = i + m[0].length - 1;
          const close = matchParen(src, open);
          const inner = src.slice(open + 1, close);
          const name = m[1];
          // :has()/:not() sin nada procedimental dentro: es CSS normal.
          if ((name === 'has' || name === 'not') && !PROC.test(inner)) { css += src.slice(i, close + 1); i = close + 1; continue; }
          if (css.trim()) { steps.push({ css: css.trim(), rel: steps.length > 0 }); css = ''; }
          if (['remove', 'style', 'remove-attr', 'remove-class'].includes(name)) { action = name; arg = inner; i = close + 1; continue; }
          if (name === 'matches-path') { path = toRe(inner); i = close + 1; continue; }
          if (name === 'watch-attr') { i = close + 1; continue; }
          steps.push({ op: name, arg: name === 'has' || name === 'not' ? compile(inner) : inner, re: name === 'has-text' || name === 'matches-attr' || name.startsWith('matches-css') ? null : undefined });
          i = close + 1;
          continue;
        }
      }
      css += c;
      i++;
    }
    if (css.trim()) steps.push({ css: css.trim(), rel: steps.length > 0 });
    if (!steps.length && !action) return null;
    for (const s of steps) prepare(s);
    return { steps, action, arg, path };
  }
  function skip(s, i) {
    const c = s[i];
    const end = c === '[' ? ']' : c;
    for (let j = i + 1; j < s.length; j++) {
      if (s[j] === '\\') { j++; continue; }
      if (s[j] === end) return j + 1;
    }
    return s.length;
  }
  function matchParen(s, open) {
    let depth = 0;
    for (let j = open; j < s.length; j++) {
      const c = s[j];
      if (c === '\\') { j++; continue; }
      if (c === '"' || c === "'") { j = skip(s, j) - 1; continue; }
      if (c === '(') depth++;
      else if (c === ')' && --depth === 0) return j;
    }
    throw new Error('paréntesis');
  }
  function toRe(a) {
    const m = /^\/(.+)\/([imsu]*)$/.exec(a);
    if (m) return new RegExp(m[1], m[2]);
    return new RegExp(a.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  }
  function prepare(s) {
    if (!s.op) return;
    switch (s.op) {
      case 'has-text': s.re = toRe(s.arg); break;
      case 'min-text-length': s.n = parseInt(s.arg, 10) || 0; break;
      case 'upward': s.n = /^\d+$/.test(s.arg.trim()) ? parseInt(s.arg, 10) : 0; break;
      case 'matches-attr': {
        const m = /^\s*"?([^"=]+)"?\s*(?:=\s*"?(.*?)"?)?\s*$/.exec(s.arg);
        s.name = toRe(m ? m[1] : s.arg);
        s.value = m && m[2] !== undefined ? toRe(m[2]) : null;
        break;
      }
      case 'matches-css': case 'matches-css-before': case 'matches-css-after': {
        const k = s.arg.indexOf(':');
        s.prop = s.arg.slice(0, k).trim();
        s.value = toRe(s.arg.slice(k + 1).trim().replace(/^"|"$/g, ''));
        s.pseudo = s.op === 'matches-css' ? null : s.op === 'matches-css-before' ? '::before' : '::after';
        break;
      }
    }
  }
  function exec(steps, root) {
    let nodes = null;
    for (const s of steps) {
      if (s.css !== undefined) {
        if (nodes === null && !s.rel) nodes = [...(root || document).querySelectorAll(root ? ':scope ' + s.css : s.css)];
        else {
          const base = nodes || [root || document.documentElement];
          const out = new Set();
          const relative = /^[\s>+~]/.test(s.css);
          for (const n of base) {
            if (relative) for (const x of n.querySelectorAll(':scope' + s.css)) out.add(x);
            else if (n.matches(s.css)) out.add(n);
          }
          nodes = [...out];
        }
        continue;
      }
      if (nodes === null) nodes = s.op === 'xpath' ? [] : [...(root || document).querySelectorAll('*')];
      nodes = applyOp(s, nodes, root);
    }
    return nodes || [];
  }
  function applyOp(s, nodes, root) {
    const out = [];
    switch (s.op) {
      case 'has-text': for (const n of nodes) if (s.re.test(n.textContent)) out.push(n); break;
      case 'min-text-length': for (const n of nodes) if (n.textContent.length >= s.n) out.push(n); break;
      case 'upward':
        for (const n of nodes) {
          let p = n;
          if (s.n > 0) { for (let i = 0; i < s.n && p; i++) p = p.parentElement; }
          else p = n.parentElement?.closest(s.arg);
          if (p && p !== document.documentElement && p !== document.body) out.push(p);
        }
        break;
      case 'matches-attr':
        for (const n of nodes) for (const a of n.attributes) {
          if (s.name.test(a.name) && (!s.value || s.value.test(a.value))) { out.push(n); break; }
        }
        break;
      case 'matches-css': case 'matches-css-before': case 'matches-css-after':
        for (const n of nodes) {
          const v = getComputedStyle(n, s.pseudo)[s.prop];
          if (v !== undefined && s.value.test(v)) out.push(n);
        }
        break;
      case 'has': for (const n of nodes) if (s.arg && exec(s.arg.steps, n).length) out.push(n); break;
      case 'not': for (const n of nodes) if (s.arg && !exec(s.arg.steps, n).length) out.push(n); break;
      case 'xpath': {
        const ctx = nodes.length ? nodes : [root || document];
        for (const c of ctx) {
          const r = document.evaluate(s.arg, c, null, XPathResult.ORDERED_NODE_SNAPSHOT_TYPE, null);
          for (let i = 0; i < r.snapshotLength; i++) { const x = r.snapshotItem(i); if (x.nodeType === 1) out.push(x); }
        }
        break;
      }
      default: return nodes;
    }
    return [...new Set(out)];
  }
})();
