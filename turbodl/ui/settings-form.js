// Formulario de ajustes compartido por el popup y el gestor. Los campos se marcan con
// name="<ajuste>" y se guardan solos al cambiar.

import { getSettings, setSettings } from '../lib/settings.js';

const FIELDS = `
  <div class="field">
    <label for="f-min">Tamaño mínimo para interceptar</label>
    <div class="range-row"><input id="f-min" name="minSizeMB" type="number" min="0" step="1"><span class="unit">MB</span></div>
  </div>
  <div class="field">
    <label for="f-conns">Conexiones por descarga</label>
    <div class="range-row"><input id="f-conns" name="connections" type="range" min="1" max="6" step="1"><output for="connections"></output></div>
    <span class="hint">Chrome no abre más de 6 por servidor.</span>
  </div>
  <div class="field">
    <label for="f-total">Conexiones totales</label>
    <div class="range-row"><input id="f-total" name="maxTotalConnections" type="number" min="1" max="64" step="1"><span class="unit">entre todas</span></div>
  </div>
  <div class="field">
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
    $('excludedDomains').value = s.excludedDomains.join('\n');
  }

  fill(await getSettings());

  let timer;
  async function save() {
    await setSettings({
      minSizeMB: $('minSizeMB').value,
      connections: conns.value,
      maxTotalConnections: $('maxTotalConnections').value,
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
    timer = setTimeout(save, e.target.type === 'range' ? 0 : 400);
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
