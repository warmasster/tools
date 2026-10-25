// TurboBLOCK · service worker.
//
// No filtra nada por sí mismo: prepara a Chrome (rulesets, reglas, scripts
// registrados) y despierta solo para cambios de ajustes, ventanas emergentes,
// mensajes de la interfaz y la actualización de listas.

import { getSettings, saveSettings } from './lib/settings.js';
import { apply, learnEntityHost, getMeta, enabledCats } from './lib/engine.js';
import * as popups from './lib/popups.js';
import * as updater from './lib/updater.js';
import * as stats from './lib/stats.js';
import { hostOf, normalizeHost, hostIn } from './lib/util.js';

popups.init();
updater.init();
stats.init();

chrome.runtime.onInstalled.addListener((d) => {
  apply('instalado: ' + d.reason);
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'pick', title: 'Ocultar un elemento de esta página', contexts: ['page', 'image', 'link', 'frame', 'video'] });
    chrome.contextMenus.create({ id: 'toggle-site', title: 'Activar o desactivar en esta web', contexts: ['action'] });
  });
});
chrome.runtime.onStartup.addListener(() => apply('inicio'));

chrome.storage.onChanged.addListener((changes, area) => {
  if (area !== 'local') return;
  if (changes.settings) apply('ajustes');
  if (changes.pop) popups.invalidatePopupHosts();
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (!tab?.id) return;
  if (info.menuItemId === 'pick') startPicker(tab.id);
  if (info.menuItemId === 'toggle-site') toggleSite(tab);
});

chrome.commands.onCommand.addListener(async (cmd, tab) => {
  tab ||= (await chrome.tabs.query({ active: true, currentWindow: true }))[0];
  if (!tab?.id) return;
  if (cmd === 'toggle-site') toggleSite(tab);
  if (cmd === 'pick-element') startPicker(tab.id);
});

async function toggleSite(tab) {
  const host = normalizeHost(hostOf(tab.url));
  if (!host) return;
  const s = await getSettings();
  const allow = hostIn(host, s.allow) ? s.allow.filter((h) => !(host === h || host.endsWith('.' + h))) : [...s.allow, host];
  await saveSettings({ allow });
  await apply('web');
  chrome.tabs.reload(tab.id);
}

async function startPicker(tabId) {
  await chrome.scripting.executeScript({ target: { tabId }, files: ['content/picker.js'] }).catch(() => {});
}

// ---------- mensajes ----------

const handlers = {
  // content/popups.js
  click(msg, sender) { if (sender.tab) popups.recordClick(sender.tab.id, msg); },
  async openPopup(msg, sender) {
    if (!/^https?:\/\//.test(msg.url)) return;
    await chrome.tabs.create({ url: msg.url, openerTabId: sender.tab?.id, index: sender.tab ? sender.tab.index + 1 : undefined });
  },
  async allowPopups(msg) {
    const h = normalizeHost(msg.host);
    const s = await getSettings();
    if (h && !s.popupAllow.includes(h)) await saveSettings({ popupAllow: [...s.popupAllow, h] });
  },
  // content/cosmetic.js: una web con comodín que tiene scriptlets.
  async sla(msg, sender) {
    if (sender.tab && msg.host) await learnEntityHost(msg.host, msg.list, sender.tab.id, sender.frameId || 0);
  },
  // content/picker.js
  async addFilter(msg) {
    const s = await getSettings();
    const f = String(msg.filter || '').trim();
    if (!f) return;
    const lines = s.userFilters.split(/\r?\n/);
    if (!lines.includes(f)) await saveSettings({ userFilters: (s.userFilters.trim() ? s.userFilters.trimEnd() + '\n' : '') + f });
    await apply('filtro');
  },
  // blocked.html
  async allowOnce(msg, sender) {
    const h = normalizeHost(msg.host) || hostOf(msg.url);
    if (!h) return;
    const rules = await chrome.declarativeNetRequest.getSessionRules();
    const id = 1 + rules.reduce((m, r) => Math.max(m, r.id), 0);
    // Solo en la pestaña donde se pulsó: el resto de pestañas siguen protegidas.
    const condition = { requestDomains: [h], resourceTypes: ['main_frame'] };
    if (sender.tab?.id >= 0) condition.tabIds = [sender.tab.id];
    await chrome.declarativeNetRequest.updateSessionRules({
      addRules: [{ id, priority: 200, action: { type: 'allowAllRequests' }, condition }],
    });
  },
  // interfaz
  async tabInfo(msg) {
    const tab = await chrome.tabs.get(msg.tabId);
    const s = await getSettings();
    const host = hostOf(tab.url);
    const out = { host, url: tab.url, web: /^https?:/.test(tab.url || ''), allowed: hostIn(host, s.allow), popupsAllowed: hostIn(host, s.popupAllow), byCat: {}, total: 0 };
    try {
      const { rulesMatchedInfo } = await chrome.declarativeNetRequest.getMatchedRules({ tabId: msg.tabId });
      for (const r of rulesMatchedInfo) {
        if (!(await stats.isBlock(r.rule))) continue;
        const c = r.rule.rulesetId === '_dynamic' ? 'user' : r.rule.rulesetId;
        out.byCat[c] = (out.byCat[c] || 0) + 1;
        out.total++;
      }
    } catch {}
    const { tabPopups = {} } = await chrome.storage.session.get('tabPopups');
    out.popups = tabPopups[msg.tabId] || 0;
    return out;
  },
  async toggleSite(msg) {
    const tab = await chrome.tabs.get(msg.tabId);
    await toggleSite(tab);
  },
  async pick(msg) { await startPicker(msg.tabId); },
  async update(msg) { return updater.runUpdate(!!msg.force); },
  async overview() {
    const meta = await getMeta();
    const s = await getSettings();
    const { status, updateState } = await chrome.storage.local.get(['status', 'updateState']);
    const st = await stats.getStats();
    const cats = enabledCats(s);
    let rules = 0;
    for (const c of cats) rules += meta.categories[c]?.network || 0;
    return { meta: { build: meta.build, built: meta.built, lists: meta.lists, categories: meta.categories }, status, updateState, stats: st, rules, cats, version: chrome.runtime.getManifest().version };
  },
  async reapply() { await apply('manual'); },
};

// Lo que pueden pedir los scripts de contenido (corren dentro de las webs). El resto
// solo desde las páginas de la extensión (ventanita, opciones, página bloqueada).
const FROM_PAGES = new Set(['click', 'openPopup', 'allowPopups', 'sla', 'addFilter']);
const EXT_URL = chrome.runtime.getURL('');

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  const h = handlers[msg?.type];
  if (!h) return false;
  if (!FROM_PAGES.has(msg.type) && !(sender.id === chrome.runtime.id && sender.url?.startsWith(EXT_URL))) return false;
  Promise.resolve(h(msg, sender)).then((r) => reply(r ?? null), (e) => reply({ error: e.message }));
  return true;
});

// Para depurar desde la consola del service worker (y para dev/run-tests.js).
globalThis.turboblock = { apply, getSettings, saveSettings, getMeta, runUpdate: updater.runUpdate, collectStats: stats.collect };
