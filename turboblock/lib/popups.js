// Bloqueador de ventanas emergentes.
//
// Chrome ya para las ventanas que una web abre sin que hagas clic. Las que se
// cuelan son las "popunder": la web abre la ventana aprovechando tu clic en
// cualquier parte (un vídeo, un botón de reproducir…). Aquí se cierran:
//
//   - siempre, si el destino está en las listas ($popup);
//   - en modo estricto, si la ventana va a otro sitio y tu clic no fue en un
//     enlace a ese sitio ni en un botón de verdad.
//
// La web de origen enseña un aviso con «Abrir» y «Permitir en esta web».

import { getSettings } from './settings.js';
import { hostOf, siteOf, hostIn } from './util.js';
import { addCounts } from './stats.js';

// Destinos legítimos de ventanas abiertas por botones (inicios de sesión, pagos…).
const SAFE = [
  'accounts.google.com', 'login.microsoftonline.com', 'login.live.com', 'appleid.apple.com',
  'www.facebook.com', 'facebook.com', 'm.facebook.com', 'github.com', 'api.twitter.com', 'twitter.com', 'x.com',
  'www.paypal.com', 'paypal.com', 'checkout.stripe.com', 'js.stripe.com', 'discord.com', 'id.twitch.tv',
  'www.linkedin.com', 'auth0.com', 'okta.com', 'login.yahoo.com', 'steamcommunity.com', 'store.steampowered.com',
];

const clicks = new Map(); // tabId -> { href, control, t }
const pendingBlank = new Map(); // tabId -> datos de la ventana abierta en about:blank
let popHosts = null;

export function invalidatePopupHosts() { popHosts = null; }

async function listHosts() {
  if (!popHosts) {
    const { pop = [] } = await chrome.storage.local.get('pop');
    popHosts = new Set(pop);
  }
  return popHosts;
}

function inSet(host, set) {
  for (let h = host; h; ) {
    if (set.has(h)) return true;
    const i = h.indexOf('.');
    if (i < 0) break;
    h = h.slice(i + 1);
  }
  return false;
}

/** Mensaje de content/popups.js: el usuario ha hecho clic (de verdad) en algo. */
export function recordClick(tabId, msg) {
  clicks.set(tabId, { href: msg.href || '', control: !!msg.control, newTab: !!msg.newTab, t: Date.now(), used: false });
}

export function init() {
  chrome.webNavigation.onCreatedNavigationTarget.addListener((d) => { onCreated(d).catch(() => {}); });
  chrome.webNavigation.onBeforeNavigate.addListener((d) => {
    if (d.frameId !== 0) return;
    const p = pendingBlank.get(d.tabId);
    if (!p) return;
    pendingBlank.delete(d.tabId);
    if (Date.now() - p.t < 5000) decide({ ...p.d, url: d.url }, p.ctx).catch(() => {});
  });
  chrome.tabs.onRemoved.addListener((tabId) => { clicks.delete(tabId); pendingBlank.delete(tabId); });
}

async function onCreated(d) {
  const s = await getSettings();
  if (!s.enabled || !s.popups) return;
  let top, src;
  try {
    top = await chrome.webNavigation.getFrame({ tabId: d.sourceTabId, frameId: 0 });
    src = d.sourceFrameId ? await chrome.webNavigation.getFrame({ tabId: d.sourceTabId, frameId: d.sourceFrameId }) : top;
  } catch { return; }
  const topHost = hostOf(top?.url);
  if (!topHost || !/^https?:/.test(top.url)) return;
  if (hostIn(topHost, s.allow) || hostIn(topHost, s.popupAllow)) return;
  const ctx = { s, topHost, srcHost: hostOf(src?.url) || topHost };
  if (!d.url || d.url === 'about:blank') {
    // window.open() y luego location = …: se decide cuando navegue.
    pendingBlank.set(d.tabId, { d, ctx, t: Date.now() });
    return;
  }
  await decide(d, ctx);
}

// ¿El último clic real en la pestaña de origen pidió esta ventana?
// true = sí; false = no; null = no hay clic reciente (puede estar en camino).
function clickAllows(d, site) {
  const c = clicks.get(d.sourceTabId);
  if (!c || Date.now() - c.t > 2500) return null;
  // Clic en un enlace: solo vale la ventana que va a donde apunta el enlace (con
  // target=_blank, Ctrl o botón central). Lo demás que se abra a la vez es una
  // popunder pegada al clic, aunque el enlace sea legítimo.
  if (c.href) return siteOf(hostOf(c.href)) === site;
  // Botón (o enlace "#"/javascript:) que abre una ventana: la primera que se abra
  // es la tuya, aunque vaya a otro sitio o pase por redirecciones.
  if ((c.newTab || c.control) && !c.used) { c.used = true; return true; }
  // Clic en un enlace normal (se abre en la misma pestaña) o en cualquier otra
  // parte de la página, y además sale una ventana a otro sitio: popunder.
  return false;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function decide(d, { s, topHost, srcHost }) {
  const target = hostOf(d.url);
  if (!target || !/^https?:/.test(d.url)) return;
  let reason = '';
  if (inSet(target, await listHosts())) {
    reason = 'list';
  } else if (s.strictPopups) {
    const site = siteOf(target);
    const sameSite = site === siteOf(topHost) || site === siteOf(srcHost);
    if (!sameSite && !hostIn(target, SAFE)) {
      // El aviso del clic puede llegar un poco después que la ventana nueva.
      let ok = clickAllows(d, site);
      for (let i = 0; ok === null && i < 4; i++) { await sleep(60); ok = clickAllows(d, site); }
      if (!ok) reason = 'strict';
    }
  }
  if (!reason) return;

  chrome.tabs.remove(d.tabId).catch(() => {});
  await addCounts({ popups: 1 });
  const key = 'tabPopups';
  const { [key]: perTab = {} } = await chrome.storage.session.get(key);
  perTab[d.sourceTabId] = (perTab[d.sourceTabId] || 0) + 1;
  await chrome.storage.session.set({ [key]: perTab });
  if (s.toasts) chrome.tabs.sendMessage(d.sourceTabId, { type: 'popupBlocked', url: d.url, host: target, reason }, { frameId: 0 }).catch(() => {});
}
