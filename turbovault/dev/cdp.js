// Control mínimo de Chrome por el protocolo de DevTools sobre pipe (sin dependencias).
// Permite cargar la extensión descomprimida con Extensions.loadUnpacked, que exige
// --remote-debugging-pipe y --enable-unsafe-extension-debugging.

import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const EXT_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '..');

export const CHROME = process.env.CHROME || [
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  '/usr/bin/google-chrome',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
].find(existsSync);

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export class Browser {
  constructor(profile, { headed = false, args = [] } = {}) { this.profile = profile; this.headed = headed; this.extraArgs = args; }

  async launch() {
    const a = [
      `--user-data-dir=${this.profile}`,
      '--remote-debugging-pipe',
      '--enable-unsafe-extension-debugging',
      '--no-first-run', '--no-default-browser-check', '--disable-search-engine-choice-screen',
      '--window-size=1200,800',
      ...(this.extraArgs || []),
    ];
    if (!this.headed) a.push('--headless=new');
    this.proc = spawn(CHROME, a, { stdio: ['ignore', 'ignore', 'pipe', 'pipe', 'pipe'] });
    this.proc.stderr.on('data', () => {}); // Chrome escribe mucho ruido
    this.exited = new Promise((r) => this.proc.once('exit', r));
    this.seq = 0;
    this.pending = new Map();
    this.listeners = [];
    let buf = '';
    this.proc.stdio[4].on('data', (d) => {
      buf += d.toString('utf8');
      let i;
      while ((i = buf.indexOf('\0')) >= 0) {
        const msg = JSON.parse(buf.slice(0, i));
        buf = buf.slice(i + 1);
        if (msg.id && this.pending.has(msg.id)) {
          const { resolve: ok, reject } = this.pending.get(msg.id);
          this.pending.delete(msg.id);
          msg.error ? reject(new Error(`${msg.error.message} ${msg.error.data || ''}`)) : ok(msg.result);
        } else if (msg.method) {
          for (const l of this.listeners) l(msg);
        }
      }
    });
    await this.send('Target.setDiscoverTargets', { discover: true });
  }

  send(method, params = {}, sessionId) {
    const id = ++this.seq;
    this.proc.stdio[3].write(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }) + '\0');
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      setTimeout(() => { if (this.pending.delete(id)) reject(new Error(`CDP timeout: ${method}`)); }, 60_000);
    });
  }

  async close() {
    await this.send('Browser.close').catch(() => {});
    await Promise.race([this.exited, sleep(10_000)]);
    this.proc.kill();
  }

  async loadExtension(path = EXT_DIR) {
    const { id } = await this.send('Extensions.loadUnpacked', { path });
    this.extId = id;
    return id;
  }

  /** Sesión con el service worker de la extensión (se vuelve a enganchar si se reinicia). */
  async sw() {
    if (this.swSession) return this.swSession;
    for (let i = 0; i < 100; i++) {
      const { targetInfos } = await this.send('Target.getTargets');
      const t = targetInfos.find((x) => x.type === 'service_worker' && x.url.startsWith(`chrome-extension://${this.extId}/`));
      if (t) {
        const { sessionId } = await this.send('Target.attachToTarget', { targetId: t.targetId, flatten: true });
        this.swSession = sessionId;
        if (this.onConsole) {
          this.listeners.push((m) => {
            if (m.sessionId === sessionId && m.method === 'Runtime.consoleAPICalled') {
              this.onConsole(m.params.type, m.params.args.map((a) => a.value ?? a.description ?? '').join(' '));
            }
          });
          await this.send('Runtime.enable', {}, sessionId);
        }
        this.listeners.push((m) => { if (m.method === 'Target.detachedFromTarget' && m.params.sessionId === sessionId) this.swSession = null; });
        return sessionId;
      }
      // Un SW dormido no aparece como target: despertarlo abriendo una página de la extensión.
      if (i === 10) await this.send('Target.createTarget', { url: `chrome-extension://${this.extId}/options.html` }).catch(() => {});
      await sleep(200);
    }
    throw new Error('No aparece el service worker de la extensión');
  }

  async evalSW(expression) {
    for (let attempt = 0; ; attempt++) {
      try {
        const r = await this.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true }, await this.sw());
        if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text);
        return r.result.value;
      } catch (e) {
        if (attempt < 3 && /session|target|context/i.test(e.message)) { this.swSession = null; await sleep(300); continue; }
        throw e;
      }
    }
  }

  /** Abre una pestaña y, si se indica, hace clic en un elemento (para tener Referer). */
  async openAndClick(url, selector) {
    const { targetId } = await this.send('Target.createTarget', { url });
    if (!selector) return targetId;
    const { sessionId } = await this.send('Target.attachToTarget', { targetId, flatten: true });
    // Ojo: al principio la pestaña está en about:blank, que ya cuenta como "complete".
    // Hay que esperar a que exista el elemento en la página de destino.
    const sel = JSON.stringify(selector);
    for (let i = 0; ; i++) {
      const r = await this.send('Runtime.evaluate', {
        expression: `location.href.startsWith(${JSON.stringify(url)}) && document.readyState === 'complete' && !!document.querySelector(${sel})`,
        returnByValue: true,
      }, sessionId).catch(() => null);
      if (r?.result?.value === true) break;
      if (i > 100) throw new Error(`No aparece ${selector} en ${url}`);
      await sleep(100);
    }
    const r = await this.send('Runtime.evaluate', { expression: `document.querySelector(${sel}).click()` }, sessionId);
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || 'Fallo al hacer clic');
    return targetId;
  }
}
