/* Datos de muestra para ver la interfaz en un navegador normal (sin Wails).
   En la app de verdad no se usa: allí está window.go. Contraseña maestra de
   muestra: "maestra-de-prueba". */
window.__tvMock = function () {
  const handlers = {};
  const emit = (n, d) => (handlers[n] || []).forEach((f) => f(d));
  const now = () => Math.floor(Date.now() / 1000);
  let unlocked = false, exists = !new URLSearchParams(location.search).has('nueva'), expires = 0, fails = 0;
  let pending = null;
  const settings = { sessionMin: 10, clipSeconds: 20, notifyFill: false, autostart: true, hideCapture: true, gen: { mode: 'random', length: 20, lower: true, upper: true, digits: true, symbols: true, noAmbiguous: false, words: 5, sep: '-', capitalize: true, number: true }, backups: [] };
  let n = 0;
  const id = () => 'm' + (++n);
  const items = [];
  const secrets = {};
  const add = (it, sec) => {
    const x = Object.assign({ id: id(), type: 'login', urls: [], username: '', fav: false, created: now() - 86400 * 400, updated: now() - 86400 * 20, used: now() - 3600, uses: 3, score: 4 }, it);
    x.hasPassword = !!sec.password; x.hasTotp = !!sec.totp; x.hasNotes = !!sec.notes;
    if (!x.hasPassword) x.score = -1;
    x.pwChanged = x.updated;
    items.push(x);
    secrets[x.id] = Object.assign({ password: '', notes: '', totp: '', fields: [], history: [] }, sec);
  };
  add({ title: 'Google', urls: ['https://accounts.google.com'], username: 'fernando@gmail.com', fav: true }, { password: 'tH7$-qPz!8vK-2mW#9xR', totp: 'JBSWY3DPEHPK3PXP', history: [{ password: 'viejaClave2021', until: now() - 86400 * 90 }] });
  add({ title: 'Steam', urls: ['https://store.steampowered.com'], username: 'fer_gamer', fav: true }, { password: 'Caracol-Bisonte-faro7', fields: [{ name: 'Código de recuperación', value: 'R12345', hidden: true }] });
  add({ title: 'Banco', urls: ['https://www.mibanco.es'], username: '12345678Z', score: 1 }, { password: 'Fernando1999!' });
  add({ title: 'Netflix', urls: ['https://netflix.com'], username: 'fernando@gmail.com', score: 1 }, { password: 'Fernando1999!' });
  add({ title: 'Router de casa', urls: ['http://192.168.1.1'], username: 'admin', score: 0 }, { password: 'admin1234' });
  add({ title: 'GitHub', urls: ['https://github.com'], username: 'warmasster' }, { password: 'kX9#mQ2$vL7&pR4!zN5^', totp: 'otpauth://totp/GitHub:warmasster?secret=JBSWY3DPEHPK3PXP&issuer=GitHub' });
  add({ title: 'Wifi de casa', type: 'note' }, { notes: 'Red: MiCasa_5G\nClave: alcachofa-Tambor-92' });
  add({ title: 'Amazon', urls: ['https://amazon.es'], username: 'fernando@gmail.com', score: 3 }, { password: 'Pelicano.Vereda.Lince.4' });
  const events = [
    { t: now() - 60, kind: 'fill', title: 'GitHub', origin: 'https://github.com', src: 'Chrome' },
    { t: now() - 300, kind: 'unlock', src: 'agente' },
    { t: now() - 86400, kind: 'lock', title: 'Se acabó el tiempo de la sesión', src: 'agente' },
    { t: now() - 86400 - 600, kind: 'add', title: 'Amazon', src: 'app' },
  ];
  const state = () => ({ exists, unlocked, expires: unlocked ? expires : 0, now: Date.now(), sessionMin: settings.sessionMin, count: items.filter((i) => !i.deleted).length, version: 'muestra', fails, pair: unlocked ? pending : null });
  const push = () => setTimeout(() => emit('state', state()), 10);
  const ok = (data) => ({ ok: true, data });
  const err = (error, code = 'error') => ({ ok: false, error, code });
  const strength = (pw) => {
    const len = (pw || '').length;
    let score = len < 8 ? 0 : len < 11 ? 1 : len < 14 ? 2 : len < 18 ? 3 : 4;
    if (/^(fernando|password|123456|contrase)/i.test(pw)) score = 0;
    return { score, label: ['Muy débil', 'Débil', 'Aceptable', 'Fuerte', 'Muy fuerte'][score], bits: len * 5, crack: ['al instante', '3 horas', '20 años', 'siglos', 'millones de años'][score], warning: score < 2 ? 'Es demasiado corta.' : '' };
  };
  const gen = (o) => {
    const sets = { lower: 'abcdefghijkmnopqrstuvwxyz', upper: 'ABCDEFGHJKLMNPQRSTUVWXYZ', digits: '23456789', symbols: '!#$%&*+-=?@' };
    if (o.mode === 'words') {
      const w = ['caracol', 'bisonte', 'faro', 'ciruela', 'tambor', 'lince', 'vereda', 'pelicano', 'alcachofa', 'girasol'];
      const out = []; for (let i = 0; i < o.words; i++) { let x = w[Math.floor(Math.random() * w.length)]; if (o.capitalize) x = x[0].toUpperCase() + x.slice(1); out.push(x); }
      if (o.number) out[0] += Math.floor(Math.random() * 10);
      return out.join(o.sep);
    }
    if (o.mode === 'pin') { let s = ''; for (let i = 0; i < o.length; i++) s += Math.floor(Math.random() * 10); return s; }
    let all = ''; for (const k of ['lower', 'upper', 'digits', 'symbols']) if (o[k]) all += sets[k];
    let s = ''; for (let i = 0; i < o.length; i++) s += all[Math.floor(Math.random() * all.length)];
    return s;
  };
  const totp = () => ({ code: String(100000 + Math.floor(Math.random() * 899999)), left: 30 - (Math.floor(Date.now() / 1000) % 30), period: 30 });

  const api = {
    __rt: {
      EventsOn: (n, f) => { (handlers[n] = handlers[n] || []).push(f); },
      WindowMinimise() {}, WindowToggleMaximise() {}, Quit() { console.log('Quit'); },
    },
    Mode: async () => new URLSearchParams(location.search).get('mode') || '',
    SetMode: async () => {},
    Version: async () => 'muestra',
    PickFile: async () => 'C:\\Users\\yo\\Descargas\\Contraseñas de Chrome.csv',
    PickSaveFile: async () => 'C:\\Users\\yo\\Desktop\\exportacion.csv',
    PickFolder: async () => 'E:\\Copias TurboVAULT',
    ShowInFolder: async () => {},
    DataFolder: async () => 'C:\\Users\\yo\\AppData\\Roaming\\TurboVAULT',
    OpenURL: async () => {},
    Protect: async () => {},
    KillOtherAgent: async () => {},
    ExtensionInfo: async () => ({ dir: 'C:\\Users\\yo\\AppData\\Local\\TurboVAULT\\extension', installed: true, loaded: [], host: true, id: 'x' }),
    SetupExtension: async () => ({}),
    Call: async (cmd, a) => {
      a = a || {};
      await new Promise((r) => setTimeout(r, cmd === 'unlock' ? 400 : 30));
      const need = !['state', 'unlock', 'create', 'lock', 'settings', 'strength', 'suggest', 'generate', 'setSettings', 'checkBackup', 'restore', 'backupTargets', 'copyText'].includes(cmd);
      if (need && !unlocked) return err('La bóveda está cerrada.', 'locked');
      switch (cmd) {
        case 'state': return ok(state());
        case 'unlock':
          if (a.master !== 'maestra-de-prueba') { fails++; return err('contraseña maestra incorrecta', 'wrong'); }
          unlocked = true; fails = 0; expires = Date.now() + settings.sessionMin * 60000; push(); return ok(state());
        case 'create': exists = true; unlocked = true; expires = Date.now() + settings.sessionMin * 60000; push(); return ok(state());
        case 'lock': unlocked = false; push(); return ok(null);
        case 'items': return ok(items.filter((i) => !!i.deleted === !!a.trash).map((i) => Object.assign({}, i)));
        case 'get': { const it = items.find((i) => i.id === a.id); const s = secrets[a.id]; return ok({ item: it, secret: s, totp: s.totp ? totp() : undefined }); }
        case 'totp': return ok(totp());
        case 'save': {
          let it = items.find((i) => i.id === a.item.id);
          if (!it) { add(Object.assign({}, a.item, { id: undefined, created: now(), updated: now(), uses: 0, used: 0 }), a.secret); it = items[items.length - 1]; } else { Object.assign(it, a.item, { updated: now() }); secrets[it.id] = Object.assign(secrets[it.id], a.secret); it.hasPassword = !!a.secret.password; it.hasTotp = !!a.secret.totp; }
          it.score = strength(a.secret.password).score;
          push(); return ok(it);
        }
        case 'delete': items.find((i) => i.id === a.id).deleted = now(); push(); return ok(null);
        case 'untrash': items.find((i) => i.id === a.id).deleted = 0; push(); return ok(null);
        case 'purge': items.splice(items.findIndex((i) => i.id === a.id), 1); push(); return ok(null);
        case 'emptyTrash': for (let i = items.length - 1; i >= 0; i--) if (items[i].deleted) items.splice(i, 1); return ok(0);
        case 'fav': items.find((i) => i.id === a.id).fav = a.on; return ok(null);
        case 'copy': return ok({ clearIn: a.field === 'username' ? 0 : settings.clipSeconds });
        case 'copyText': return ok({ clearIn: settings.clipSeconds });
        case 'generate': { const o = Object.assign({}, settings.gen, a || {}); if (a && a.mode) settings.gen = o; const p = gen(o); return ok({ password: p, bits: 100, strength: strength(p) }); }
        case 'suggest': { const p = gen({ mode: 'words', words: 5, sep: '-', capitalize: true, number: true }); return ok({ password: p, bits: 55, strength: strength(p) }); }
        case 'strength': return ok(strength(a.password));
        case 'health': {
          const live = items.filter((i) => !i.deleted && i.type === 'login');
          const hi = (i) => ({ id: i.id, title: i.title, username: i.username, host: (i.urls[0] || '').replace(/^https?:\/\//, ''), score: i.score });
          return ok({ total: live.length, score: 64, weak: live.filter((i) => i.score <= 1).map(hi), reused: [[hi(live[2]), hi(live[3])]], old: [Object.assign(hi(live[0]), { note: 'Sin cambiar desde 08/2025' })], insecure: [hi(live[4])], breached: [], noTotp: 5, breachAt: 0 });
        }
        case 'breach': { const h = (await api.Call('health')).data; h.breached = [Object.assign({}, h.weak[0], { note: 'Aparece 1.234 veces en filtraciones' })]; h.breachAt = Date.now(); return ok(h); }
        case 'events': return ok(events);
        case 'clearEvents': events.length = 0; return ok(null);
        case 'never': return ok(['facebook.com']);
        case 'setNever': return ok(null);
        case 'pairs': return ok([{ hash: 'h', name: 'Chrome', created: now() - 86400 * 3 }]);
        case 'unpair': return ok(null);
        case 'approvePair': case 'rejectPair': pending = null; push(); return ok(null);
        case 'settings': return ok(settings);
        case 'setSettings': Object.assign(settings, a); return ok(settings);
        case 'verifyMaster': return a.master === 'maestra-de-prueba' ? ok(null) : err('Contraseña maestra incorrecta.', 'wrong');
        case 'changeMaster': return ok(null);
        case 'import': return ok({ source: 'Chrome', added: 42, dupes: 3, skipped: 1, wiped: true });
        case 'export': return ok(items.length);
        case 'backupTargets': return ok(settings.backups);
        case 'addBackupTarget': settings.backups.push({ id: 'b1', path: a.path, current: a.path, label: 'USB KINGSTON', keep: 10, connected: true, kind: 'usb', lastCopy: Date.now(), copies: 1 }); return ok(settings.backups);
        case 'removeBackupTarget': settings.backups = settings.backups.filter((b) => b.id !== a.id); return ok(settings.backups);
        case 'backupNow': return ok(a.path ? a.path + '\\TurboVAULT-2026-10-24.tvault' : settings.backups);
        case 'checkBackup': return a.master && a.master !== 'maestra-de-prueba' ? err('La contraseña maestra de esa copia no es esa.', 'wrong') : ok(a.master ? { items: 8, created: now() - 86400 * 30, rev: 3 } : null);
        case 'restore': return ok({ saved: '' });
      }
      return err('Orden desconocida: ' + cmd);
    },
  };
  window.__mockPair = () => { pending = { code: '482 913', browser: 'Chrome' }; push(); };
  setTimeout(() => emit('agent', { ok: true }), 0);
  return api;
};
