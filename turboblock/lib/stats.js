// Estadísticas: cuántas cosas se bloquean, por categoría y por día.
//
// Chrome bloquea sin avisar a la extensión (así no gasta nada). Cada pocos minutos
// se le pregunta qué reglas se han usado desde la última vez (getMatchedRules) y se
// suman. Las excepciones (ids desde 500000, ver native/dnr.go) no cuentan.

const ALARM = 'stats';
const ALLOW_ID_BASE = 500000;
const KEEP_DAYS = 90;
export const KB_PER_BLOCK = 22; // tamaño medio aproximado de lo que no se descarga

export function init() {
  chrome.alarms.get(ALARM).then((a) => {
    if (!a) chrome.alarms.create(ALARM, { delayInMinutes: 1, periodInMinutes: 3 });
  });
  chrome.alarms.onAlarm.addListener((a) => { if (a.name === ALARM) collect().catch(() => {}); });
}

export const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

let userAllow = null; // ids de reglas dinámicas que son excepciones
export function invalidateDynamic() { userAllow = null; }
async function dynamicAllowIds() {
  if (!userAllow) {
    const rules = await chrome.declarativeNetRequest.getDynamicRules();
    userAllow = new Set(rules.filter((r) => r.action.type === 'allow' || r.action.type === 'allowAllRequests').map((r) => r.id));
  }
  return userAllow;
}

/** ¿Esta coincidencia es un bloqueo (o redirección), no una excepción? */
export async function isBlock(rule) {
  if (rule.rulesetId === '_dynamic') return !(await dynamicAllowIds()).has(rule.ruleId);
  if (rule.rulesetId === '_session') return false;
  return rule.ruleId < ALLOW_ID_BASE;
}

let busy = null;
export function collect() {
  busy ||= doCollect().finally(() => { busy = null; });
  return busy;
}

async function doCollect() {
  const { statsSeen = 0 } = await chrome.storage.local.get('statsSeen');
  let info;
  try {
    info = await chrome.declarativeNetRequest.getMatchedRules({ minTimeStamp: statsSeen + 1 });
  } catch { return; } // cuota de Chrome superada: ya se contará en la siguiente
  const list = info.rulesMatchedInfo;
  if (!list.length) return;
  const add = {};
  let last = statsSeen, n = 0;
  for (const m of list) {
    if (m.timeStamp > last) last = m.timeStamp;
    if (!(await isBlock(m.rule))) continue;
    const c = m.rule.rulesetId === '_dynamic' ? 'user' : m.rule.rulesetId;
    add[c] = (add[c] || 0) + 1;
    n++;
  }
  await addCounts(add, last);
  return n;
}

/** Suma al día de hoy (y al total). */
export async function addCounts(add, seen) {
  const { statsDays = {}, statsTotal = {} } = await chrome.storage.local.get(['statsDays', 'statsTotal']);
  const day = statsDays[today()] ||= {};
  for (const [k, v] of Object.entries(add)) {
    day[k] = (day[k] || 0) + v;
    statsTotal[k] = (statsTotal[k] || 0) + v;
  }
  const keys = Object.keys(statsDays).sort();
  for (const k of keys.slice(0, Math.max(0, keys.length - KEEP_DAYS))) delete statsDays[k];
  if (!statsTotal.since) statsTotal.since = Date.now();
  const patch = { statsDays, statsTotal };
  if (seen) patch.statsSeen = seen;
  await chrome.storage.local.set(patch);
}

export async function getStats() {
  await collect();
  const { statsDays = {}, statsTotal = {} } = await chrome.storage.local.get(['statsDays', 'statsTotal']);
  return { days: statsDays, total: statsTotal, kbPerBlock: KB_PER_BLOCK };
}
