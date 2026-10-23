// La regla de malware redirige aquí con la URL original detrás de "#".
const url = location.hash.slice(1);
let host = '';
try { host = new URL(url).hostname; } catch {}
document.getElementById('url').textContent = url || '—';
document.getElementById('back').onclick = () => {
  if (history.length > 1) history.back();
  else window.close();
};
document.getElementById('go').onclick = async () => {
  if (!/^https?:\/\//.test(url)) return;
  await chrome.runtime.sendMessage({ type: 'allowOnce', host, url });
  location.replace(url);
};
