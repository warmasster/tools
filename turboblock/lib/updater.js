// Actualización de las listas con TurboBLOCK.exe (host com.turboblock.native).
//
// Cada 6 h se le pide al programa que mire si hay listas nuevas. Él las descarga
// (con caché: casi siempre responde "sin cambios"), las compila y las escribe en
// la carpeta de la extensión. Si la huella cambia, la extensión se recarga para
// que Chrome lea las reglas nuevas. Sin el programa instalado, la extensión
// funciona igual con las listas que trae.

import { getSettings } from './settings.js';
import { getMeta } from './engine.js';

const HOST = 'com.turboblock.native';
const ALARM = 'update-lists';
const EVERY_MIN = 6 * 60;

export function init() {
  chrome.alarms.get(ALARM).then((a) => {
    if (!a) chrome.alarms.create(ALARM, { delayInMinutes: 2, periodInMinutes: EVERY_MIN });
  });
  chrome.alarms.onAlarm.addListener((a) => {
    if (a.name === ALARM) runUpdate(false).catch(() => {});
  });
}

let busy = null;

/** Devuelve el estado guardado en "updateState". */
export function runUpdate(force) {
  if (busy) return busy;
  busy = doUpdate(force).finally(() => { busy = null; });
  return busy;
}

async function doUpdate(force) {
  const s = await getSettings();
  if (!force && !s.autoUpdate) return (await chrome.storage.local.get('updateState')).updateState;
  const meta = await getMeta();
  const state = { lastCheck: Date.now(), native: false, ok: false, error: '', build: meta.build };
  await chrome.storage.local.set({ updateState: { ...state, running: true } });
  let res;
  try {
    res = await ask({ cmd: 'update', build: meta.build, force: !!force });
    state.native = true;
  } catch (e) {
    state.error = e.message;
    await chrome.storage.local.set({ updateState: state });
    return state;
  }
  state.ok = !!res.ok;
  state.error = res.ok ? '' : res.result?.error || res.error || 'Error desconocido';
  const newBuild = res.result?.build;
  if (newBuild && newBuild !== meta.build) {
    state.build = newBuild;
    state.reloading = true;
    await chrome.storage.local.set({ updateState: state, lastUpdate: Date.now() });
    // Las reglas nuevas ya están en disco: Chrome las lee al recargar la extensión.
    setTimeout(() => chrome.runtime.reload(), 800);
    return state;
  }
  await chrome.storage.local.set({ updateState: state });
  return state;
}

function ask(msg) {
  return new Promise((resolve, reject) => {
    let port;
    try {
      port = chrome.runtime.connectNative(HOST);
    } catch (e) {
      reject(new Error('TurboBLOCK.exe no está instalado'));
      return;
    }
    const id = Date.now();
    const timer = setTimeout(() => { port.disconnect(); reject(new Error('El programa no responde')); }, 5 * 60_000);
    port.onMessage.addListener((m) => {
      if (m.id !== id) return;
      clearTimeout(timer);
      port.disconnect();
      resolve(m);
    });
    port.onDisconnect.addListener(() => {
      clearTimeout(timer);
      const err = chrome.runtime.lastError?.message || '';
      reject(new Error(/not found|no se encontr/i.test(err) ? 'TurboBLOCK.exe no está instalado' : err || 'El programa se cerró'));
    });
    port.postMessage({ ...msg, id });
  });
}
