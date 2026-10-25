// La regla de malware redirige aquí con la URL original detrás de "#".
const url = location.hash.slice(1);
let host = '';
try { host = new URL(url).hostname; } catch {}
document.getElementById('url').textContent = url || '—';
// Cualquier web puede meter esta página en un marco (tiene que ser accesible para la
// redirección): ahí no se deja entrar, para que no te hagan pulsar el botón a ciegas.
let framed = true;
try { framed = window.top !== window; } catch {}
if (framed) document.getElementById('go').hidden = true;
document.getElementById('back').onclick = () => {
  if (history.length > 1) history.back();
  else window.close();
};
document.getElementById('go').onclick = async () => {
  if (framed || !/^https?:\/\//.test(url)) return;
  await chrome.runtime.sendMessage({ type: 'allowOnce', host, url });
  location.replace(url);
};
