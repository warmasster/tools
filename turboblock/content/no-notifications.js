/* TurboBLOCK · sin peticiones de notificaciones (mundo MAIN, document_start).
 * Las webs que preguntan «¿Permitir notificaciones?» reciben un «no» sin que salga
 * la pregunta. Las webs a las que ya diste permiso no se tocan.
 */
(() => {
  'use strict';
  const N = window.Notification;
  if (!N || N.permission === 'granted') return;
  try {
    Object.defineProperty(N, 'permission', { configurable: true, get: () => 'denied' });
    N.requestPermission = function (cb) {
      if (typeof cb === 'function') setTimeout(() => cb('denied'), 0);
      return Promise.resolve('denied');
    };
  } catch {}
  const PM = window.PushManager && window.PushManager.prototype;
  if (PM) {
    try {
      PM.subscribe = function () { return Promise.reject(new DOMException('Permission denied', 'NotAllowedError')); };
      PM.permissionState = function () { return Promise.resolve('denied'); };
    } catch {}
  }
  const perm = navigator.permissions;
  if (perm && perm.query) {
    const q = perm.query.bind(perm);
    perm.query = function (desc) {
      if (desc && (desc.name === 'notifications' || desc.name === 'push')) {
        return q(desc).then((st) => {
          try { Object.defineProperty(st, 'state', { configurable: true, get: () => 'denied' }); } catch {}
          return st;
        });
      }
      return q(desc);
    };
  }
})();
