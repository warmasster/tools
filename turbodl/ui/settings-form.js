// Formulario de ajustes compartido por el popup y el gestor. Los campos se marcan con
// name="<ajuste>" y se guardan solos al cambiar.

import { getSettings, setSettings } from '../lib/settings.js';

const FIELDS = `
  <div class="field">
    <label for="f-min">Tamaño mínimo para interceptar</label>
    <div class="range-row"><input id="f-min" name="minSizeMB" type="number" min="0" step="1"><span class="unit">MB</span></div>
  </div>
  <div class="field" data-engine="browser">
    <label for="f-total">Conexiones totales</label>
    <div class="range-row"><input id="f-total" name="maxTotalConnections" type="number" min="1" max="64" step="1"><span class="unit">entre todas</span></div>
  </div>
  <div class="field span-2" data-engine="browser">
    <label for="f-conns">Conexiones por descarga</label>
    <div class="range-row"><input id="f-conns" name="connections" type="range" min="1" max="6" step="1"><output for="connections"></output></div>
    <span class="hint">Chrome no abre más de 6 por servidor.</span>
  </div>
  <div class="field">
    <label for="f-engine">Motor</label>
    <select id="f-engine" name="engine">
      <option value="auto">Nativo (Go)</option>
      <option value="browser">Navegador</option>
    </select>
  </div>
  <div class="field" data-engine="native">
    <label for="f-nconns">Conexiones</label>
    <div class="range-row"><input id="f-nconns" name="nativeConnections" type="number" min="1" max="64" step="1"><span class="unit">máx. 64</span></div>
  </div>
  <p class="hint engine-status span-2"></p>
  <div class="field span-2">
    <label for="f-excl">Dominios excluidos</label>
    <textarea id="f-excl" name="excludedDomains" rows="2" spellcheck="false" placeholder="ejemplo.com&#10;drive.google.com"></textarea>
    <span class="hint">Uno por línea. Incluye sus subdominios.</span>
  </div>`;

/** Pinta los campos dentro de `root` (si no los tiene ya) y los conecta. */
export async function mountSettingsForm(root, { saved = root.querySelector('.saved') } = {}) {
  if (!root.querySelector('[name="minSizeMB"]')) root.insertAdjacentHTML('beforeend', FIELDS);
  const $ = (n) => root.querySelector(`[name="${n}"]`);
  const conns = $('connections');
  const connsOut = root.querySelector('output[for="connections"]');

  function fill(s) {
    $('minSizeMB').value = s.minSizeMB;
    conns.value = s.connections;
    connsOut.textContent = s.connections;
    $('maxTotalConnections').value = s.maxTotalConnections;
    $('engine').value = s.engine;
    $('nativeConnections').value = s.nativeConnections;
    $('excludedDomains').value = s.excludedDomains.join('\n');
  }

  // Estado del motor nativo (lo detecta el service worker y lo deja en storage). Solo
  // se muestran los ajustes del motor que se está usando.
  const status = root.querySelector('.engine-status');
  async function showEngine() {
    const { engine } = await chrome.storage.local.get('engine');
    const native = !!engine?.native && $('engine').value !== 'browser';
    if (!engine?.native) {
      status.textContent = 'Motor nativo no instalado: se usa el del navegador (máx. 6 conexiones). Para instalarlo, abre turbodl-native.exe.';
    } else if (native) {
      status.textContent = `Motor nativo ${engine.version}: hasta 64 conexiones, guarda directamente en ${engine.downloadsDir}.`;
    } else {
      status.textContent = 'Motor del navegador (máx. 6 conexiones por servidor).';
    }
    status.classList.toggle('ok', native);
    for (const el of root.querySelectorAll('[data-engine]')) el.hidden = el.dataset.engine !== (native ? 'native' : 'browser');
  }
  showEngine();
  $('engine').addEventListener('change', showEngine);
  chrome.storage.onChanged.addListener((c, area) => { if (area === 'local' && (c.engine || c.settings)) showEngine(); });

  fill(await getSettings());

  let timer;
  async function save() {
    await setSettings({
      minSizeMB: $('minSizeMB').value,
      connections: conns.value,
      maxTotalConnections: $('maxTotalConnections').value,
      engine: $('engine').value,
      nativeConnections: $('nativeConnections').value,
      excludedDomains: $('excludedDomains').value,
    });
    if (saved) {
      saved.classList.add('show');
      clearTimeout(saved.t);
      saved.t = setTimeout(() => saved.classList.remove('show'), 1200);
    }
  }

  conns.addEventListener('input', () => { connsOut.textContent = conns.value; });
  root.addEventListener('input', (e) => {
    if (!e.target.name) return;
    clearTimeout(timer);
    // El deslizador se guarda al momento; el texto, al dejar de escribir.
    timer = setTimeout(save, e.target.type === 'range' || e.target.tagName === 'SELECT' ? 0 : 400);
  });
  // Al salir de un campo se normaliza lo escrito (p. ej. 9 conexiones → 6).
  root.addEventListener('change', async () => { clearTimeout(timer); await save(); fill(await getSettings()); });

  // Cambios desde otro sitio (atajo de teclado, la otra página).
  chrome.storage.onChanged.addListener(async (changes, area) => {
    if (area !== 'local' || !changes.settings) return;
    const active = document.activeElement;
    if (active && root.contains(active)) return; // no pisar lo que se está escribiendo
    fill(await getSettings());
  });
}

export async function shortcutText() {
  try {
    const cmds = await chrome.commands.getAll();
    return cmds.find((c) => c.name === 'toggle-interception')?.shortcut || '';
  } catch { return ''; }
}

/** Conecta un interruptor con el ajuste "enabled" (interceptación activada). */
export async function bindEnabledToggle(input, onChange = () => {}) {
  const sync = async () => {
    const s = await getSettings();
    input.checked = s.enabled;
    onChange(s.enabled);
  };
  input.addEventListener('change', async () => {
    await setSettings({ enabled: input.checked });
    onChange(input.checked);
  });
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === 'local' && changes.settings) sync();
  });
  await sync();
}
