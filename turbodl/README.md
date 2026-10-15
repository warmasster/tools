# TurboDL

Gestor de descargas multiconexión que funciona **entero dentro de Chrome**, sin programas externos. Cuando haces clic en un enlace de descarga, TurboDL la intercepta y la baja por trozos en paralelo (HTTP Range). Guarda los trozos en el almacenamiento privado de la extensión (OPFS) y, al terminar, deja el archivo en tu carpeta de Descargas.

- **Hasta 6 conexiones por descarga**, con reparto dinámico: cuando una conexión acaba su trozo, parte por la mitad el trozo pendiente más grande y se pone con esa mitad.
- **Pausar y reanudar**, también tras cerrar Chrome (o si se cuelga): al volver a abrirlo sigue donde iba.
- **Reintentos por trozo** (hasta 5, con espera creciente) sin afectar al resto.
- **Detecta si el archivo cambia en el servidor** (ETag / Last-Modified con If-Range) y en ese caso vuelve a empezar.
- **Cookies y Referer** de la página original, para descargas que piden sesión.
- **Gestor de descargas** en una pestaña, con barra de segmentos al estilo IDM, velocidad, tiempo restante y conexiones activas.
- Interfaz en español, tema oscuro y sin librerías externas.

## Instalar (modo desarrollador)

1. Abre `chrome://extensions`.
2. Activa **Modo de desarrollador** (arriba a la derecha).
3. Pulsa **Cargar descomprimida** y elige esta carpeta (`turbodl/`, la que contiene `manifest.json`).
4. Fija el icono de TurboDL en la barra para tener a mano el popup.

Necesita Chrome 116 o posterior. No hay que compilar nada.

## Uso

- **Automático**: haz clic en cualquier enlace de descarga. Si el archivo pasa del tamaño mínimo (10 MB por defecto) y el servidor permite descargar por partes, se encarga TurboDL. Si no, la descarga sigue con Chrome como siempre.
- **A la fuerza**: clic derecho en un enlace → **Descargar con TurboDL**. Así se ignora el tamaño mínimo y, si el servidor no admite Range, se descarga con una sola conexión. También puedes pegar un enlace en el gestor.
- **Popup** (icono de la barra):
  - activar o desactivar la interceptación;
  - tamaño mínimo;
  - conexiones por descarga (1-6) y conexiones totales;
  - dominios excluidos;
  - botón para abrir el gestor.
- **Atajo**: `Alt+Mayús+T` activa o desactiva la interceptación. Se cambia en `chrome://extensions/shortcuts`. Con la interceptación desactivada, el icono muestra `OFF`.
- **Gestor**: pausar, reanudar, cancelar, reintentar, abrir el archivo y mostrarlo en la carpeta. Si una descarga falla, el botón **Descargar con Chrome normal** sirve de plan B.

## Cómo funciona

```
background.js (service worker)       ← intercepta, sondea, orquesta, entrega, badge
   │  chrome.runtime.sendMessage
   ▼
offscreen.html / offscreen.js        ← motor persistente: planificador de conexiones, blob URL final
   │  new Worker()
   ▼
download-worker.js (uno por descarga) ← fetch por rangos → OPFS (FileSystemSyncAccessHandle)
   │
IndexedDB  ← estado compartido (downloads · segments · history)
BroadcastChannel('turbodl') ← progreso en vivo → manager.html / popup.html
```

- **El service worker no descarga nada**, porque Chrome lo duerme a los 30 s. Todo el trabajo pesado se hace en el documento offscreen y en sus workers.
- **Un worker por descarga**: solo se puede abrir un `SyncAccessHandle` por archivo, así que todas las conexiones de una descarga escriben desde el mismo worker, cada una en su posición (`write(buf, {at})`). Los datos se leen en streaming y no se acumulan en RAM.
- **Guardado del progreso** cada ~1 s, en este orden: se copian las posiciones, se hace `flush()` y luego se escribe en IndexedDB. Así lo guardado nunca va por delante de lo que está realmente en disco, y sobrevive a un cierre brusco.
- **Interceptación** (`onDeterminingFilename`):
  - se ignoran las descargas creadas por extensiones, incluida la entrega final de TurboDL, para que no haya bucles;
  - se hace un `GET` con `Range: bytes=0-0` (3 s como máximo) y solo se toma el control si responde 206, el tamaño coincide y no es HTML;
  - siempre se llama a `suggest()`.
- **Cookies y Referer**: `fetch` no deja fijarlos, así que se ponen con reglas de sesión de `declarativeNetRequest`. Se limitan a `tabIds: [-1]` y al dominio de la descarga, y se borran al pausar o terminar. Las cookies solo se fuerzan si con `credentials: 'include'` el servidor responde como si no hubieran llegado.
- **Entrega**: se crea un blob URL sobre el archivo de OPFS (sin copiarlo a memoria) y se pasa a `chrome.downloads.download`. Cuando se completa, se borra el temporal y el estado.
- **Capa de backend** (`lib/backend.js`): `InBrowserBackend` es la implementación actual. `NativeBackend` es el hueco para un futuro programa nativo (`com.turbodl.native`) y activarlo es cambiar una línea de `createBackend()`.

## Servidor de pruebas

`dev/test-server.js` sirve un archivo grande generado al vuelo (no ocupa disco). Admite Range, ETag/If-Range y un límite de velocidad **por conexión**, para que se note la mejora.

```bash
node dev/test-server.js --size 2GB --rate 2MB
```

Abre `http://localhost:8077/`: hay enlaces para cada caso (archivo grande, archivo pequeño, descarga HTML, `blob:`, `data:`, descarga con sesión).

| Opción | Qué hace |
|---|---|
| `--port 8077` | Puerto |
| `--size 2GB` | Tamaño de `/file.bin` |
| `--rate 2MB` | Límite por conexión en bytes/s (`0` = sin límite) |
| `--no-range` | Ignora Range (siempre 200 con el archivo entero) |
| `--fail-rate 0.01` | Probabilidad de cortar la conexión en cada bloque de 64 KB |
| `--change-etag-after 30` | A los N segundos el archivo cambia (nuevo ETag y contenido) |
| `--verbose` | Una línea por petición |

Otras rutas: `/__hash` (SHA-256 esperado), `/__stats` (conexiones simultáneas, cortes, Referer y cookies recibidos) y `/__bump` (cambiar la versión del archivo).

## Pruebas automáticas

`dev/run-tests.js` arranca un Chrome real sin ventana, con un perfil temporal, y carga la extensión por el protocolo de DevTools. Después levanta los servidores de prueba y comprueba el SHA-256 de cada archivo final.

```bash
node dev/run-tests.js
```

Opciones:

- `--size 120MB` y `--rate 2MB`: tamaño del archivo y límite por conexión;
- `--big`: añade la descarga de 2 GB;
- `--only speed,pause`: solo esas pruebas;
- `--headed`: con ventana visible;
- `--keep`: no borra el perfil temporal.

| Prueba | Qué comprueba |
|---|---|
| `speed` | 6 conexiones contra 1, con el mismo límite por conexión: al menos 4 veces más rápido. Hash correcto. |
| `pause` | Pausa al 30 %: no se descarga nada mientras está en pausa; al reanudar termina con el hash correcto. |
| `restart` | Cerrar Chrome a mitad y volver a abrirlo: continúa desde donde iba y el hash es correcto. |
| `crash` | Igual, pero matando el proceso (sin cierre ordenado). |
| `concurrent` | Dos descargas del mismo servidor: nunca más de 6 conexiones entre las dos y nombres distintos. |
| `norange` | Servidor sin Range: al interceptar la descarga se queda en Chrome; forzada, va con una conexión. |
| `failures` | 1 % de cortes por bloque: termina bien gracias a los reintentos. |
| `etag` | El archivo cambia durante una pausa: se detecta, se reinicia y queda la versión nueva. |
| `cookies` | Descarga que necesita cookie de sesión y Referer. |
| `skip` | Un archivo pequeño, una página HTML, `blob:` y `data:` se quedan en Chrome. |
| `noloop` | Las entregas finales no se vuelven a interceptar. |
| `cleanup` | Al final no quedan reglas de red ni archivos temporales. |

`dev/make-icons.js` regenera los iconos.

## Limitaciones conocidas

- **Máximo 6 conexiones por servidor.** Es el límite de Chrome para HTTP/1.1 por origen, y lo comparten todas las descargas de ese servidor. TurboDL las reparte por orden de llegada y respeta además el tope global que pongas.
- **Poca o ninguna mejora en servidores HTTP/2 o HTTP/3.** Chrome mete todas las peticiones a un mismo servidor en una sola conexión multiplexada. Si ese servidor limita la velocidad por conexión, abrir varias no ayuda. La mejora se nota en servidores HTTP/1.1 que limitan por conexión.
- **Hace falta ~2 veces el tamaño del archivo en disco**: uno para el temporal de OPFS y otro para la copia en Descargas. TurboDL lo comprueba antes de empezar y, si no hay sitio, deja la descarga a Chrome.
- **La copia final no es instantánea**: con archivos de varios GB, pasar de OPFS a Descargas tarda unos segundos (es una copia en disco).
- **Sin soporte de Range no hay reanudación**: con una sola conexión, pausar o reiniciar Chrome hace que vuelva a empezar.
- **Descargas que dependen de un POST** (formularios) no se pueden repetir con GET. TurboDL las detecta (el servidor devuelve HTML o un tamaño distinto) y las deja a Chrome.
- **Archivos comprimidos al vuelo** (`Content-Encoding: gzip`, etc.) no se pueden partir por rangos y se dejan a Chrome.
- Los enlaces firmados que caducan pueden fallar al reanudar mucho después. En ese caso, el plan B es **Descargar con Chrome normal**.
