// «Móvil» de pruebas: un Chrome sin ventana, con la cámara falsa de Chrome (una
// imagen de prueba que se mueve), abre la página del móvil de TurboCAM y pulsa
// «Empezar». Lo usa TestRealPhoneE2E (camera_real_test.go).
//
//   node dev/phone.mjs <url> [segundos] [--headed]
//
// Escribe en la salida líneas JSON con lo que pasa en la página.
import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const [url, secs = '20', ...rest] = process.argv.slice(2);
if (!url) { console.error('uso: node dev/phone.mjs <url> [segundos]'); process.exit(2); }
const headed = rest.includes('--headed');
const CHROME = process.env.CHROME || [
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
].find(existsSync);
if (!CHROME) { console.log(JSON.stringify({ error: 'no hay Chrome' })); process.exit(3); }

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const profile = mkdtempSync(join(tmpdir(), 'turbocam-phone-'));
const args = [
  `--user-data-dir=${profile}`, '--remote-debugging-pipe', '--no-first-run', '--no-default-browser-check',
  '--use-fake-device-for-media-stream=fps=30', '--use-fake-ui-for-media-stream', '--ignore-certificate-errors',
  '--autoplay-policy=no-user-gesture-required', '--window-size=420,860',
];
// TURBOCAM_FAKEVIDEO=cam.mjpeg: la cámara falsa pone ese vídeo (con grano, como una de verdad).
if (process.env.TURBOCAM_FAKEVIDEO) args.push(`--use-file-for-fake-video-capture=${process.env.TURBOCAM_FAKEVIDEO}`);
if (!headed) args.push('--headless=new');
const proc = spawn(CHROME, args, { stdio: ['ignore', 'ignore', 'pipe', 'pipe', 'pipe'] });
proc.stderr.on('data', () => {});
let seq = 0, buf = '';
const pending = new Map(), listeners = [];
proc.stdio[4].on('data', (d) => {
  buf += d.toString('utf8');
  let i;
  while ((i = buf.indexOf('\0')) >= 0) {
    const m = JSON.parse(buf.slice(0, i));
    buf = buf.slice(i + 1);
    if (m.id && pending.has(m.id)) { const p = pending.get(m.id); pending.delete(m.id); m.error ? p.reject(new Error(m.error.message)) : p.resolve(m.result); }
    else if (m.method) listeners.forEach((l) => l(m));
  }
});
const send = (method, params = {}, sessionId) => {
  const id = ++seq;
  proc.stdio[3].write(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }) + '\0');
  return new Promise((resolve, reject) => { pending.set(id, { resolve, reject }); setTimeout(() => pending.delete(id) && reject(new Error('timeout ' + method)), 30000); });
};
const out = (o) => console.log(JSON.stringify(o));

try {
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
  await send('Runtime.enable', {}, sessionId);
  await send('Page.enable', {}, sessionId);
  listeners.push((m) => {
    if (m.method === 'Runtime.consoleAPICalled' && ['error', 'warning'].includes(m.params.type))
      out({ console: m.params.args.map((a) => a.value || a.description).join(' ') });
    if (m.method === 'Runtime.exceptionThrown') out({ exception: m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text });
  });
  await send('Emulation.setDeviceMetricsOverride', { width: 412, height: 860, deviceScaleFactor: 2, mobile: true }, sessionId);
  // TURBOCAM_THROTTLE=4: la CPU va 4 veces más lenta, como un móvil normal.
  if (process.env.TURBOCAM_THROTTLE) await send('Emulation.setCPUThrottlingRate', { rate: Number(process.env.TURBOCAM_THROTTLE) }, sessionId);
  // TURBOCAM_NET=80,3000: 80 ms más de latencia y 3000 kb/s de subida, como unos datos
  // móviles regulares.
  if (process.env.TURBOCAM_NET) {
    const [lat, up] = process.env.TURBOCAM_NET.split(',').map(Number);
    await send('Network.enable', {}, sessionId);
    await send('Network.emulateNetworkConditions', { offline: false, latency: lat || 0, downloadThroughput: -1, uploadThroughput: up ? up * 1000 / 8 : -1 }, sessionId);
  }
  await send('Page.navigate', { url }, sessionId);
  const ev = async (expr) => (await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true }, sessionId)).result.value;
  for (let i = 0; i < 50 && !(await ev('!!document.querySelector("#go")')); i++) await sleep(100);
  await ev('document.querySelector("#go").click(), true');
  const end = Date.now() + Number(secs) * 1000;
  let shot = 0;
  await sleep(1500);
  await ev('document.querySelector("#b-tune").click(), true'); // la hoja enseña cómo va el envío
  while (Date.now() < end) {
    await sleep(1000);
    out({ info: await ev('document.querySelector("#info").innerText.split(String.fromCharCode(10)).join(" | ")') });
    out({ status: await ev('document.querySelector("#status-text").textContent'), chip: await ev('document.querySelector("#chip-res").textContent'), screen: await ev('document.querySelector("#app").dataset.screen'), err: await ev('document.querySelector("#start-err").hidden ? "" : document.querySelector("#start-err").textContent') });
    if (process.env.TURBOCAM_SHOTS && shot++ === 3) {
      const { data } = await send('Page.captureScreenshot', { format: 'png' }, sessionId);
      const { writeFileSync } = await import('node:fs');
      writeFileSync(join(process.env.TURBOCAM_SHOTS, 'movil.png'), Buffer.from(data, 'base64'));
    }
  }
} catch (e) {
  out({ error: String(e) });
} finally {
  await send('Browser.close').catch(() => {});
  await sleep(500);
  proc.kill();
  try { rmSync(profile, { recursive: true, force: true }); } catch { }
}
