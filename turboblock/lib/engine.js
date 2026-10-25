// Aplica los ajustes al navegador: rulesets de declarativeNetRequest, reglas
// dinámicas (webs permitidas y "Mis filtros"), scripts de contenido registrados y
// los datos cosméticos repartidos en chrome.storage.local.
//
// Todo lo pesado lo hace Chrome: los rulesets se comprueban en el proceso de red,
// las hojas de estilo y los scriptlets los inyecta él solo en las webs que tocan.
// El service worker solo trabaja cuando cambia algo.

import { getSettings } from './settings.js';
import { compileUserFilters, USER_RULE_BASE } from './userfilters.js';
import { SPEC_SHARDS, GEN_SHARDS, specShard, genShard, makeBloom, matchPattern, fnv } from './util.js';
import { invalidateDynamic } from './stats.js';

export const ALL_CATS = ['base', 'ads', 'privacy', 'malware', 'cookies', 'annoyances'];
const ALLOW_RULE_ID = 1; // regla dinámica de las webs permitidas
const ALLOW_PRIORITY = 100;

let metaCache = null;
export async function getMeta() {
  if (!metaCache) metaCache = await (await fetch(chrome.runtime.getURL('rules/meta.json'))).json();
  return metaCache;
}

export function enabledCats(s) {
  if (!s.enabled) return [];
  return ALL_CATS.filter((c) => c === 'base' || s.cats[c]);
}

// Un solo apply a la vez; si llegan varios cambios seguidos, se juntan.
let running = null, again = false;
export function apply(reason = '') {
  if (running) { again = true; return running; }
  running = (async () => {
    try {
      do {
        again = false;
        await applyNow(reason);
      } while (again);
    } finally {
      running = null;
    }
  })();
  return running;
}

async function applyNow(reason) {
  const t0 = performance.now();
  const s = await getSettings();
  const meta = await getMeta();
  const cats = enabledCats(s);
  const user = compileUserFilters(s.enabled ? s.userFilters : '');
  const errors = [];

  // 1. Rulesets estáticos.
  const current = await chrome.declarativeNetRequest.getEnabledRulesets();
  const enable = cats.filter((c) => !current.includes(c));
  const disable = current.filter((c) => !cats.includes(c));
  if (enable.length || disable.length) {
    try {
      await chrome.declarativeNetRequest.updateEnabledRulesets({ enableRulesetIds: enable, disableRulesetIds: disable });
    } catch (e) {
      errors.push('No se pudieron activar todas las listas: ' + e.message);
      // Por partes: que al menos entren las que caben.
      for (const c of enable) {
        try { await chrome.declarativeNetRequest.updateEnabledRulesets({ enableRulesetIds: [c] }); } catch {}
      }
    }
  }

  // 2. Reglas dinámicas.
  const allowHosts = s.allow.filter(Boolean);
  const dyn = [];
  if (allowHosts.length) {
    dyn.push({
      id: ALLOW_RULE_ID, priority: ALLOW_PRIORITY, action: { type: 'allowAllRequests' },
      condition: { requestDomains: allowHosts, resourceTypes: ['main_frame'] },
    });
  }
  const userRules = [];
  for (const r of user.rules) {
    if (r.condition.regexFilter) {
      const ok = await chrome.declarativeNetRequest.isRegexSupported({ regex: r.condition.regexFilter });
      if (!ok.isSupported) { user.errors.push({ line: r._line, text: r.condition.regexFilter, why: 'Expresión regular no admitida por Chrome' }); continue; }
    }
    const { _line, ...rule } = r;
    userRules.push({ rule, line: _line });
  }
  dyn.push(...userRules.map((x) => x.rule));
  const old = await chrome.declarativeNetRequest.getDynamicRules();
  try {
    await chrome.declarativeNetRequest.updateDynamicRules({ removeRuleIds: old.map((r) => r.id), addRules: dyn });
  } catch (e) {
    // Alguna regla no vale: se añaden una a una para saber cuál.
    await chrome.declarativeNetRequest.updateDynamicRules({ removeRuleIds: (await chrome.declarativeNetRequest.getDynamicRules()).map((r) => r.id) });
    for (const r of dyn) {
      try { await chrome.declarativeNetRequest.updateDynamicRules({ addRules: [r] }); } catch (err) {
        const u = userRules.find((x) => x.rule === r);
        if (u) user.errors.push({ line: u.line, text: '', why: err.message });
        else errors.push(err.message);
      }
    }
  }

  invalidateDynamic();

  // 3. Datos cosméticos (solo si ha cambiado algo que les afecte).
  await writeCosmetics(meta, cats, user);

  // 4. Scripts de contenido.
  await registerScripts(s, meta, cats);

  // 5. Contador del icono.
  await updateBadge(s);

  await chrome.storage.local.set({
    status: { applied: Date.now(), build: meta.build, cats, errors, userErrors: user.errors, ms: Math.round(performance.now() - t0), reason },
  });
}

// ---------- cosméticos ----------

async function loadCosmetic(cat) {
  return (await fetch(chrome.runtime.getURL(`rules/${cat}.cosmetic.json`))).json();
}

function mergeEntry(dst, src) {
  for (const f of ['h', 'u', 'p', 'c']) {
    if (src[f]?.length) dst[f] = dst[f] ? dst[f].concat(src[f]) : src[f].slice();
  }
  if (src.s) dst.s = 1;
  if (src.g) dst.g = 1;
}

async function writeCosmetics(meta, cats, user) {
  const sig = String(fnv(JSON.stringify([meta.build, cats, user.spec, user.generic, user.popups])));
  const { cosSig, cosKeys = [] } = await chrome.storage.local.get(['cosSig', 'cosKeys']);
  if (cosSig === sig) return;

  const spec = {}; // clave -> entrada
  const gen = {}; // token -> selectores
  const cg = []; // [selector, [dominios donde no]]
  const pop = new Set(), popx = new Set();
  for (const cat of cats) {
    const cf = await loadCosmetic(cat);
    for (const [k, e] of Object.entries(cf.spec || {})) mergeEntry(spec[k] ||= {}, e);
    for (const [t, sels] of Object.entries(cf.gen || {})) gen[t] = gen[t] ? gen[t].concat(sels) : sels;
    for (const g of cf.gx || []) cg.push(g);
    for (const h of cf.pop || []) pop.add(h);
    for (const h of cf.popx || []) popx.add(h);
    // Entidades ("google.*") con scriptlets: se marcan para que cosmetic.js avise.
    for (const [ent, bucket] of Object.entries(cf.sla || {})) {
      const e = spec[ent] ||= {};
      (e.j ||= []).push([cat, bucket]);
    }
  }
  for (const [k, e] of Object.entries(user.spec)) mergeEntry(spec[k] ||= {}, e);
  for (const g of user.generic) cg.push(g);
  for (const h of user.popups) pop.add(h);

  const out = {};
  for (const [k, e] of Object.entries(spec)) (out[specShard(k)] ||= {})[k] = e;
  for (const [t, sels] of Object.entries(gen)) (out[genShard(t)] ||= {})[t] = sels;
  out.gb = makeBloom(Object.keys(gen));
  out.cg = cg;
  out.pop = [...pop].filter((h) => !popx.has(h));
  const keys = Object.keys(out);
  const stale = cosKeys.filter((k) => !(k in out));
  if (stale.length) await chrome.storage.local.remove(stale);
  await chrome.storage.local.set({ ...out, cosKeys: keys, cosSig: sig });
}

// ---------- scripts de contenido ----------

async function registerScripts(s, meta, cats) {
  const allowPatterns = s.allow.map(matchPattern);
  const ghide = meta.ghide.map(matchPattern);
  const want = [];
  const base = { allFrames: true, runAt: 'document_start', persistAcrossSessions: true };
  if (cats.length) {
    // Cosmética específica, genérica por clases/ids y procedimental.
    want.push({ ...base, id: 'cosmetic', js: ['content/cosmetic.js'], matches: ['<all_urls>'], matchOriginAsFallback: true, excludeMatches: allowPatterns });
    for (const cat of cats) {
      const cm = meta.categories[cat];
      if (cm.genericCss > 0) {
        want.push({ ...base, id: `css-${cat}`, css: [`rules/${cat}.css`], matches: ['<all_urls>'], matchOriginAsFallback: true, excludeMatches: [...allowPatterns, ...ghide] });
      }
      cm.slBuckets.forEach((hosts, i) => {
        if (!hosts.length) return;
        want.push({ ...base, id: `sl-${cat}-${i}`, js: [`rules/${cat}.sl${i}.js`], world: 'MAIN', matches: hosts.map(matchPattern), excludeMatches: allowPatterns });
      });
    }
    // Scriptlets de entidades ya visitadas (los aprende learnEntityHost).
    const { learned = {} } = await chrome.storage.local.get('learned');
    for (const [key, hosts] of Object.entries(learned)) {
      const [cat, i] = key.split(':');
      if (!cats.includes(cat) || i >= meta.categories[cat].slaBuckets || !hosts.length) continue;
      want.push({ ...base, id: `sla-${cat}-${i}`, js: [`rules/${cat}.sla${i}.js`], world: 'MAIN', matches: hosts.map(matchPattern), excludeMatches: allowPatterns });
    }
  }
  if (s.enabled && s.noNotifications) {
    want.push({ ...base, allFrames: false, id: 'no-notifications', js: ['content/no-notifications.js'], world: 'MAIN', matches: ['<all_urls>'], excludeMatches: allowPatterns });
  }
  if (s.enabled && s.popups) {
    want.push({ ...base, id: 'popups', js: ['content/popups.js'], matches: ['<all_urls>'], excludeMatches: [...allowPatterns, ...s.popupAllow.map(matchPattern)] });
  }

  const sig = JSON.stringify(want);
  const { scriptsSig } = await chrome.storage.local.get('scriptsSig');
  const registered = await chrome.scripting.getRegisteredContentScripts();
  if (scriptsSig === sig && registered.length === want.length) return;
  if (registered.length) await chrome.scripting.unregisterContentScripts();
  try {
    await chrome.scripting.registerContentScripts(want);
  } catch (e) {
    // Uno roto no debe dejar la web sin protección: se registran uno a uno.
    console.warn('TurboBLOCK: registro conjunto fallido', e);
    for (const w of want) {
      try { await chrome.scripting.registerContentScripts([w]); } catch (err) { console.warn('TurboBLOCK: script', w.id, err.message); }
    }
  }
  await chrome.storage.local.set({ scriptsSig: sig });
}

// La primera visita a una web con comodín ("1337x.*"): se inyectan ya sus
// scriptlets y se registran para ese dominio, así las siguientes visitas los
// tienen desde el primer instante.
export async function learnEntityHost(host, list, tabId, frameId) {
  const meta = await getMeta();
  const s = await getSettings();
  const cats = enabledCats(s);
  const { learned = {} } = await chrome.storage.local.get('learned');
  let changed = false;
  if (typeof host !== 'string' || !/^[a-z0-9.-]{1,253}$/i.test(host) || !Array.isArray(list)) return;
  for (const [cat, bucket] of list) {
    // bucket forma parte del nombre del archivo: solo un número.
    if (!cats.includes(cat) || !Number.isInteger(bucket) || bucket < 0 || bucket > 4096) continue;
    const key = `${cat}:${bucket}`;
    const hosts = learned[key] ||= [];
    if (hosts.includes(host)) continue;
    hosts.push(host);
    if (hosts.length > 400) hosts.shift();
    changed = true;
    chrome.scripting.executeScript({
      target: { tabId, frameIds: [frameId] }, world: 'MAIN', injectImmediately: true,
      files: [`rules/${cat}.sla${bucket}.js`],
    }).catch(() => {});
  }
  if (!changed) return;
  await chrome.storage.local.set({ learned });
  await registerScripts(s, meta, cats);
}

// ---------- icono ----------

export async function updateBadge(s) {
  s ||= await getSettings();
  await chrome.declarativeNetRequest.setExtensionActionOptions({ displayActionCountAsBadgeText: s.enabled }).catch(() => {});
  await chrome.action.setBadgeBackgroundColor({ color: s.enabled ? '#6d4aff' : '#5f6580' });
  if (chrome.action.setBadgeTextColor) await chrome.action.setBadgeTextColor({ color: '#ffffff' });
  await chrome.action.setBadgeText({ text: s.enabled ? '' : 'OFF' });
}

export { USER_RULE_BASE, SPEC_SHARDS, GEN_SHARDS };
