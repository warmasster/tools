// Cabeceras Cookie y Referer para las peticiones de la extensión.
//
// fetch() no deja fijar Referer ni Cookie a mano (son cabeceras prohibidas), así que
// usamos reglas de sesión de declarativeNetRequest. Se limitan a tabIds [-1] (peticiones
// que no salen de una pestaña, como las de nuestro offscreen/worker) y al dominio
// concreto de la descarga, y se borran al pausar, terminar o cancelar.
// La regla tiene el mismo id numérico que la descarga.

import { hostOf } from './util.js';

export async function cookieHeaderFor(url) {
  try {
    const cookies = await chrome.cookies.getAll({ url });
    return cookies.map((c) => `${c.name}=${c.value}`).join('; ');
  } catch {
    return '';
  }
}

/**
 * Instala (o reemplaza) la regla de la descarga `id`.
 * @param {{url: string, referer?: string, useCookies?: boolean}} opts
 * @returns true si se ha creado una regla
 */
export async function installHeaderRule(id, { url, referer, useCookies }) {
  const requestHeaders = [];
  if (referer && /^https?:/i.test(referer)) requestHeaders.push({ header: 'referer', operation: 'set', value: referer });
  if (useCookies) {
    const cookie = await cookieHeaderFor(url);
    if (cookie) requestHeaders.push({ header: 'cookie', operation: 'set', value: cookie });
  }
  const host = hostOf(url);
  if (!requestHeaders.length || !host) {
    await removeHeaderRule(id);
    return false;
  }
  await chrome.declarativeNetRequest.updateSessionRules({
    removeRuleIds: [id],
    addRules: [{
      id,
      priority: 1,
      action: { type: 'modifyHeaders', requestHeaders },
      condition: { requestDomains: [host], tabIds: [-1], resourceTypes: ['xmlhttprequest', 'other'] },
    }],
  });
  return true;
}

export async function removeHeaderRule(id) {
  await chrome.declarativeNetRequest.updateSessionRules({ removeRuleIds: [id] }).catch(() => {});
}
