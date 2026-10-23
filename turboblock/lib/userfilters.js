// "Mis filtros": la misma sintaxis que las listas (la parte más usada), compilada
// aquí a reglas dinámicas de declarativeNetRequest y a datos cosméticos.
//
//   ||dominio.com^              bloquear un dominio
//   ||dominio.com/ruta$script   con opciones: script, image, xhr, third-party, domain=…
//   @@||dominio.com^            excepción
//   ##.selector                 ocultar en todas las webs
//   dominio.com##.selector      ocultar en una web (también :has-text(), :upward()…)
//   dominio.com#@#.selector     no ocultar ahí

export const USER_PRIORITY = { block: 40, allow: 45 };
export const USER_RULE_BASE = 1000; // ids de reglas dinámicas de "Mis filtros"
const MAX_USER_RULES = 5000;

const TYPES = {
  script: 'script', image: 'image', stylesheet: 'stylesheet', css: 'stylesheet',
  xmlhttprequest: 'xmlhttprequest', xhr: 'xmlhttprequest', subdocument: 'sub_frame', frame: 'sub_frame',
  media: 'media', font: 'font', websocket: 'websocket', ping: 'ping', other: 'other', object: 'object',
  document: 'main_frame', doc: 'main_frame',
};

const PROC_RE = /:(has-text|-abp-contains|contains|upward|nth-ancestor|remove|style|matches-css|matches-css-before|matches-css-after|matches-attr|matches-path|min-text-length|xpath|remove-attr|remove-class)\(/;
const HOST_RE = /^[a-z0-9]([a-z0-9_-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9_-]*[a-z0-9])?)+$/;

function domains(list, sep) {
  const pos = [], neg = [];
  for (let d of list.split(sep)) {
    d = d.trim().toLowerCase();
    if (!d) continue;
    const n = d.startsWith('~');
    if (n) d = d.slice(1);
    if (!HOST_RE.test(d) && !/^([a-z0-9-]+\.)+\*$/.test(d) && !/^[a-z0-9-]+$/.test(d)) continue;
    (n ? neg : pos).push(d);
  }
  return { pos, neg };
}

/** Devuelve { rules, spec, generic, popups, errors } */
export function compileUserFilters(text) {
  const rules = [];
  const spec = {}; // dominio -> { h, u, p, c }
  const generic = []; // selectores para todas las webs
  const popups = [];
  const errors = [];
  const entry = (k) => (spec[k] ||= { h: [], u: [], p: [], c: [] });

  const lines = String(text || '').split(/\r?\n/);
  lines.forEach((raw, i) => {
    const line = raw.trim();
    if (!line || line.startsWith('!') || line.startsWith('[')) return;
    const fail = (why) => errors.push({ line: i + 1, text: line, why });

    // Cosméticos
    const m = /^([^/|$#\s]*)(#@#|##)(.+)$/.exec(line);
    if (m) {
      let [, doms, sep, sel] = m;
      sel = sel.trim().replace(/:-abp-has\(/g, ':has(').replace(/:-abp-contains\(|:contains\(/g, ':has-text(').replace(/:nth-ancestor\(/g, ':upward(');
      if (sel.startsWith('+js(') || sel.startsWith('^')) return fail('Los scriptlets y el filtrado de HTML solo funcionan en las listas');
      const { pos, neg } = domains(doms, ',');
      const proc = PROC_RE.test(sel);
      if (sep === '#@#') {
        if (!pos.length) return fail('Una excepción necesita dominio (dominio.com#@#…)');
        for (const d of pos) entry(d).u.push(sel);
        return;
      }
      if (!pos.length) {
        if (proc) return fail('Los selectores especiales necesitan dominio (dominio.com##…)');
        if (neg.length) {
          generic.push([sel, neg]);
        } else generic.push([sel, []]);
        return;
      }
      for (const d of pos) {
        const st = /^(.*):style\((.+)\)$/.exec(sel);
        if (st && !PROC_RE.test(st[1])) entry(d).c.push(`${st[1]}{${st[2]}}`);
        else if (proc) entry(d).p.push(sel);
        else entry(d).h.push(sel);
      }
      for (const d of neg) entry(d).u.push(sel);
      return;
    }

    // Red
    let allow = false, pat = line, opts = '';
    if (pat.startsWith('@@')) { allow = true; pat = pat.slice(2); }
    const dollar = pat.lastIndexOf('$');
    if (dollar >= 0 && /^~?[a-z0-9_-]+/.test(pat.slice(dollar + 1))) { opts = pat.slice(dollar + 1); pat = pat.slice(0, dollar); }
    if (/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(pat) && !opts) pat = `||${pat.toLowerCase()}^`;
    const cond = {};
    const types = [], notTypes = [];
    let important = false, popup = false, matchCase = false;
    for (const o of opts ? opts.split(',') : []) {
      const [name, val] = o.trim().split('=');
      const neg = name.startsWith('~');
      const n = neg ? name.slice(1) : name;
      if (TYPES[n]) { (neg ? notTypes : types).push(TYPES[n]); continue; }
      switch (n) {
        case 'third-party': case '3p': cond.domainType = neg ? 'firstParty' : 'thirdParty'; break;
        case 'first-party': case '1p': cond.domainType = neg ? 'thirdParty' : 'firstParty'; break;
        case 'domain': case 'from': {
          const { pos, neg: ng } = domains(val || '', '|');
          if (pos.length) cond.initiatorDomains = pos.filter((d) => !d.endsWith('*'));
          if (ng.length) cond.excludedInitiatorDomains = ng.filter((d) => !d.endsWith('*'));
          break;
        }
        case 'important': important = true; break;
        case 'popup': popup = true; break;
        case 'match-case': matchCase = true; break;
        case 'all': types.push('main_frame', 'sub_frame', 'stylesheet', 'script', 'image', 'font', 'object', 'xmlhttprequest', 'ping', 'media', 'websocket', 'other'); break;
        default: return fail(`Opción no admitida: ${n}`);
      }
    }
    if (errors.length && errors[errors.length - 1].line === i + 1) return;
    if (popup && !types.length) {
      const h = /^\|\|([a-z0-9.-]+)\^?$/.exec(pat);
      if (h && !allow) popups.push(h[1]);
      else fail('$popup solo admite ||dominio^');
      return;
    }
    if (/^\/.*\/$/.test(pat)) {
      cond.regexFilter = pat.slice(1, -1);
    } else {
      pat = pat.replace(/^\*+/, '').replace(/\*+$/, '');
      if (!/^[\x21-\x7e]*$/.test(pat)) return fail('El patrón tiene caracteres no válidos');
      if (pat && pat !== '|' && pat !== '||') cond.urlFilter = pat;
    }
    if (!cond.urlFilter && !cond.regexFilter && !cond.initiatorDomains) return fail('Filtro demasiado general: bloquearía todo');
    if (matchCase) cond.isUrlFilterCaseSensitive = true;
    if (types.length) cond.resourceTypes = [...new Set(types)];
    if (notTypes.length) cond.excludedResourceTypes = [...new Set(notTypes)];
    let action = { type: allow ? 'allow' : 'block' };
    if (allow && types.includes('main_frame')) { action = { type: 'allowAllRequests' }; cond.resourceTypes = ['main_frame', 'sub_frame']; }
    if (rules.length >= MAX_USER_RULES) return fail('Demasiados filtros de red');
    rules.push({
      id: USER_RULE_BASE + rules.length,
      priority: allow ? USER_PRIORITY.allow : USER_PRIORITY.block + (important ? 2 : 0),
      action,
      condition: cond,
      _line: i + 1,
    });
  });
  for (const k of Object.keys(spec)) {
    const e = spec[k];
    for (const f of ['h', 'u', 'p', 'c']) if (!e[f].length) delete e[f];
  }
  return { rules, spec, generic, popups, errors };
}
