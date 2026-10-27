# TurboBLOCK

Bloqueador de anuncios, rastreadores, malware y ventanas emergentes para Chrome (y Edge o Brave). Está hecho para gastar lo mínimo: el bloqueo de peticiones lo hace **Chrome por dentro** (`declarativeNetRequest`, Manifest V3), sin ejecutar JavaScript en cada petición, y la extensión solo trabaja cuando cambia algo.

- **Anuncios**: EasyList, EasyList Spanish, uBlock filters (con *quick fixes*), la lista de Peter Lowe y una lista propia de vídeo (`filters/turboblock.txt`).
- **Anuncios de vídeo**:
  - **YouTube**: se quitan los anuncios de la respuesta del reproductor (`json-prune`, `trusted-replace-*` de las listas de uBlock) y, si alguno llega a reproducirse, se silencia, se salta al final y se pulsa «Saltar».
  - **Twitch**: los anuncios van dentro del propio directo; se silencian y se tapan hasta que acaban.
  - **Cualquier web con Google IMA** (el SDK de anuncios de vídeo más usado): se cambia por uno que responde «no hay anuncios» y el reproductor pasa directo al vídeo. Las peticiones VAST/VMAP reciben una respuesta vacía.
  - **Streams HLS y DASH**: `m3u-prune` y `xml-prune` quitan los anuncios metidos en el propio manifiesto (Paramount+, Fox, Discovery+…, según las listas).
- **Rastreadores**: EasyPrivacy y uBlock Privacy.
- **Malware y estafas**: uBlock Badware y URLhaus. Si entras en una web peligrosa sale un aviso con «Volver atrás» o «Entrar de todas formas» (solo para esa pestaña).
- **Avisos de cookies** y **Molestias** (botones sociales, carteles de suscripción, avisos de notificaciones…): desactivadas al instalar, se activan en el popup.
- **Ventanas emergentes**: cierra las *popunder* (las que una web abre aprovechando tu clic en cualquier parte) y enseña un aviso en la página con **Abrir** y **Permitir en esta web**. Lo que abres tú (enlaces con Ctrl o botón central, `target=_blank`, botones como «Compartir») no se toca.
- **Peticiones de notificaciones** (opcional): las webs que preguntan «¿Permitir notificaciones?» reciben un no sin que salga la pregunta.
- **Estadísticas**: bloqueos de hoy y en total, por categoría, gráfica de los últimos 30 días y una estimación de los datos ahorrados.
- **Copia de seguridad**: guardar y recuperar ajustes, webs, filtros y estadísticas en un archivo.
- **Ocultación de elementos** por web, genérica y con selectores especiales de uBlock Origin (`:has-text()`, `:upward()`, `:remove()`, `:style()`…).
- **Scriptlets** de uBlock Origin (más de 50, los más usados): quitan anuncios de YouTube, defienden de los antiadblock y paran `window.open` en las webs que lo necesitan.
- **Sustitutos** de scripts de anuncios y analítica (Google Analytics, Tag Manager, GPT, AdSense, IMA, apstag, FuckAdBlock, fingerprintjs…): cuando se bloquean, la web recibe uno que no hace nada en vez de romperse.
- **Ocultar elemento**: eliges con el ratón lo que quieres quitar y se guarda para siempre.
- **Mis filtros**: tus propios filtros, con la misma sintaxis que las listas.
- **Listas al día**: `TurboBLOCK.exe` las descarga y recompila cada 6 horas (solo baja lo que ha cambiado).
- Contador de bloqueos en el icono, interfaz en español, tema oscuro y sin librerías externas.

## Instalar

Con [TurboSETUP](../installer/) o bajando `TurboBLOCK.exe` de la [Release](https://github.com/warmasster/tools/releases/tag/apps):

1. **Doble clic en `TurboBLOCK.exe`.** Sin permisos de administrador:
   - instala el actualizador de listas;
   - deja la extensión en `%LOCALAPPDATA%\TurboBLOCK\extension`;
   - copia esa ruta al portapapeles y abre `chrome://extensions`.
2. Activa **Modo de desarrollador** (arriba a la derecha).
3. Pulsa **Cargar descomprimida**, pega la ruta (`Ctrl+V`) y pulsa **Seleccionar carpeta**.

Solo hay que hacerlo una vez: Chrome no deja que un programa instale extensiones que no vengan de la Chrome Web Store. Las listas se actualizan solas; para una versión nueva de TurboBLOCK, **Actualizar** en TurboSETUP y ↻ en `chrome://extensions`.

```bash
TurboBLOCK.exe status
```

```bash
TurboBLOCK.exe update --force
```

```bash
TurboBLOCK.exe uninstall
```

(y quitar TurboBLOCK en `chrome://extensions`).

El ID de la extensión es siempre `bdgliednephldclmchggpdnahalgdcif` (lo fija `key` en `manifest.json`); el actualizador solo acepta a esa extensión. Necesita Chrome 121 o posterior.

## Uso

- **Popup** (icono de la barra): anillo con lo bloqueado en la página por categoría, activar o desactivar en esa web, **Ocultar elemento**, bloqueos de hoy y en total con la gráfica de 14 días, las categorías, las ventanas emergentes (con **modo estricto**) y cuándo se actualizaron las listas.
- **Ajustes** (panel): estadísticas de 30 días, protección (categorías, ventanas, notificaciones, actualización), webs sin bloqueo y que pueden abrir ventanas, **Mis filtros** (con los errores línea a línea), listas, copia de seguridad y «acerca de».
- **Clic derecho** en una página → *Ocultar un elemento de esta página*.
- **Atajos**: `Alt+Mayús+B` activa o desactiva en la web actual y `Alt+Mayús+H` oculta un elemento (se cambian en `chrome://extensions/shortcuts`).

### Ventanas emergentes

Chrome ya para las ventanas que una web abre sin que hagas clic. Las que se cuelan se abren aprovechando tu clic. TurboBLOCK cierra una ventana nueva:

- siempre, si su destino está en las listas (`$popup`);
- en **modo estricto** (activado al instalar), si va a otra web y no la pediste tú. Cuenta como pedida:
  - la que va a donde apunta el enlace en el que hiciste clic (con `target=_blank`, Ctrl, Mayús o botón central);
  - la primera que abre un botón, o un enlace que no lleva a ningún sitio (`#`, `javascript:`), aunque vaya a otra web («Compartir», «Iniciar sesión con…»). Cuentan también los botones hechos con un bloque cualquiera (con cursor de mano, del tamaño de un botón), y vale aunque la ventana tarde hasta 5 s en abrirse (pagos e inicios de sesión que esperan al servidor);
  - la que abres desde el clic derecho con «Abrir enlace en una pestaña nueva»;
  - inicios de sesión y pagos conocidos (Google, Microsoft, Apple, PayPal, Stripe, Adyen, Klarna, Redsys…).

  Si haces clic en un enlace normal (que se abre en la misma pestaña) y además sale una ventana a otra web, esa ventana es una popunder y se cierra.

Si alguna web legítima necesita abrir ventanas, **Permitir en esta web** en el aviso (o en Ajustes).

## Cómo funciona

```
TurboBLOCK.exe (Go)
   ├─ compila las listas → rules/   (al compilar el .exe y cada 6 h con el actualizador)
   └─ host de Native Messaging com.turboblock.native

rules/<cat>.json          → rulesets de declarativeNetRequest (los aplica Chrome)
rules/<cat>.css           → ocultación genérica que no depende de clases (pocos KB)
rules/<cat>.cosmetic.json → ocultación por web, genérica por clase/id, ventanas emergentes
rules/<cat>.slN.js        → scriptlets en grupos por dominio (mundo MAIN, document_start)
rules/<cat>.slaN.js       → scriptlets de webs con comodín ("1337x.*")

background.js  ← activa rulesets, registra scripts, reparte los datos en chrome.storage.local
content/cosmetic.js       ← en cada marco: oculta lo de ese dominio
content/popups.js         ← clics reales y aviso de ventana bloqueada
content/picker.js         ← «Ocultar elemento»
```

Las decisiones que lo hacen ligero:

- **Dominios agrupados.** Todos los `||dominio^` con las mismas opciones van en **una sola regla** con `requestDomains`, que Chrome comprueba con una tabla hash. Los 70.000 filtros de anuncios quedan en ~8.400 reglas y todas las listas activas juntas en ~21.000, dentro del mínimo garantizado de Chrome (30.000).
- **Nada de hoja de estilos gigante.** Los selectores genéricos que dependen de una clase o id (casi todos) no se inyectan en todas las páginas: `cosmetic.js` mira las clases e ids que aparecen, los comprueba en un filtro de Bloom y pide solo los selectores de las que coinciden. La hoja fija de anuncios pesa 18 KB en vez de 240 KB por marco.
- **Datos repartidos.** Lo cosmético se guarda en `chrome.storage.local` en 512 porciones por dominio: cada marco lee solo las de su dominio en una llamada.
- **Scriptlets por grupos.** Cada grupo (~30 KB) lleva solo los scriptlets que usa y se registra solo en sus dominios: una web normal no carga ninguno. Los de webs con comodín se aprenden en la primera visita y desde entonces se registran para ese dominio.
- **El service worker no filtra nada**: solo despierta para cambios de ajustes, ventanas emergentes y la actualización de listas.

### Compilador de listas (`native/`)

Entiende la sintaxis de Adblock Plus y uBlock Origin: `!#if`/`!#include`, opciones de red (`third-party`, tipos, `domain=`, `to=`, `denyallow=`, `method=`, `important`, `badfilter`, `redirect=`, `removeparam=`, `document`, `popup`, `generichide`…), ocultación normal y procedimental, `+js()` y excepciones `#@#`. Lo que Chrome no puede hacer (`$csp`, `$replace`, filtrado de HTML…) se descarta en vez de aplicarlo mal. Compila todo en ~1 s.

Las listas son de sus autores y se usan con sus licencias (EasyList: GPLv3 / CC BY-SA 3.0; uBlock filters: GPLv3; URLhaus: CC0; Peter Lowe: uso libre con atribución).

## Compilar

```powershell
.\native\build.ps1
```

Necesita [Go](https://go.dev). Hace `go vet` y `go test -short`, descarga las listas que hayan caducado (caché en `native\.cache`), las compila en `rules\` y deja `TurboBLOCK.exe` con la extensión comprimida dentro. Con `-Offline` usa solo la caché.

Para desarrollar, después de `build.ps1` se puede cargar la carpeta `turboblock\` directamente en `chrome://extensions`.

`node dev/make-assets.js` regenera los iconos y las imágenes de `resources/`.

## Pruebas

```bash
go test ./...
```

(desde `native/`; sin `-short` también compila las listas reales de la caché.)

```bash
node dev/run-tests.js
```

Arranca un Chrome de verdad con un perfil temporal, compila las listas reales más `dev/test-filters.txt`, hace que cualquier dominio apunte a un servidor local y comprueba:

| Prueba | Qué comprueba |
|---|---|
| `carga` | ID fijo, rulesets activos, scripts registrados y sin errores. |
| `red` | Un script de un dominio bloqueado no llega ni a pedirse. |
| `cosmetica` | Ocultación específica, genérica por clase (también en elementos añadidos después), hoja fija, `:has-text()`, `:style()` y excepciones `#@#`. |
| `scriptlets` | `set-constant` por dominio y por entidad (`entidad.*`), en la primera visita y en las siguientes. |
| `popups` | Se cierra la ventana de un clic en una zona cualquiera y la de un dominio de la lista; un enlace con `target=_blank` se abre; `nowoif`; aviso en la página. |
| `enlaces` | Un enlace `#` que abre otra web («Compartir») y Ctrl+clic se permiten; la popunder pegada a un enlace normal se cierra. |
| `video` | Google IMA cambiado por el sustituto, VAST vacío, `json-prune` en XHR (aunque la web registre su manejador antes), `m3u-prune`, `xml-prune` y saltar un anuncio en un reproductor tipo YouTube (silencia, salta, pulsa «Saltar» y devuelve el sonido). |
| `estadisticas` | Los bloqueos se cuentan por categoría y día. |
| `malware` | Página de aviso y «Entrar de todas formas». |
| `web-permitida` | En una web permitida no se bloquea ni oculta nada. |
| `mis-filtros` | Reglas de red y cosméticas propias, y errores por línea. |
| `desactivar` | Al desactivar no queda ninguna regla ni script; al activar vuelve todo. |
| `interfaz` | El popup y los ajustes cargan. |

Opciones: `--headed` (con ventana), `--only red,popups`, `--keep` (no borra el perfil temporal).

La prueba `video` usa además el puerto 80, porque los filtros `||dominio/ruta` no admiten puerto en la URL; si está ocupado, esa prueba falla.

## Limitaciones

- **No hay filtrado de HTML ni `$replace`/`$csp`**: Chrome no deja modificar respuestas desde una extensión Manifest V3. En YouTube se cubre con scriptlets (`json-prune`, `trusted-replace-*`) y, si aun así se cuela un anuncio, se salta solo en menos de un segundo.
- **Twitch**: los anuncios se silencian y se tapan, pero no se pueden quitar del directo (van mezclados en el vídeo).
- **Estadísticas**: Chrome bloquea sin avisar a la extensión; los contadores se ponen al día cada pocos minutos (y al abrir el popup). Los «datos ahorrados» son una estimación.
- **Scriptlets en webs con comodín** (`1337x.*`): en la primera visita se inyectan un instante más tarde; desde la segunda, desde el principio.
- **Modo estricto de ventanas**: un botón que abre otra web con JavaScript (p. ej. «Compartir en X») puede cerrarse; el aviso tiene **Abrir** y **Permitir en esta web**.
- **Límites de Chrome**: con todas las categorías activadas puede que alguna no quepa (Chrome admite 1000 expresiones regulares por extensión). Entran por importancia (malware antes que anuncios) y la que se queda fuera sale marcada «No cabe» en la ventanita.
- **Lo que pueden hacer las listas**: las excepciones de otras categorías ya no quitan el aviso de malware (solo las de las propias listas de malware y las tuyas); las que no son de uBlock Origin no usan scriptlets que escriben cookies, almacenamiento o enlaces; y ninguna puede quitar de las direcciones parámetros de inicio de sesión, pagos o enlaces firmados (`code`, `state`, `token`…).
- Si un filtro de **Mis filtros** no vale, se buscan los malos sin quitar los que ya funcionan.
- Sin `TurboBLOCK.exe` instalado (extensión cargada a mano) funciona igual, pero las listas no se actualizan solas.
