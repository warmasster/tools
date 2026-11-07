# TurboGAMES

Todas tus cuentas de Steam a un clic, sin el límite del selector de Steam, con sus contraseñas guardadas cifradas si quieres. También enseña tus juegos con sus portadas, quién ha jugado a cada uno y cuánto, y los abre con la cuenta que toque. Y gestiona los mods del Workshop: suscribirte y quitarlos desde aquí, presets que se activan de golpe y se mandan por un chat.

- **`TurboGAMES.exe`**: un único ejecutable. Se instala con [TurboSETUP](../installer/) o se descarga suelto de la [Release](https://github.com/warmasster/tools/releases/tag/apps); no necesita instalar nada.
- No usa internet para las cuentas: lee y cambia lo que Steam guarda en tu PC. Solo baja de los servidores de imágenes de Steam las portadas que Steam no tenga ya guardadas, y de la web pública de Steam los datos de los mods (nombre, imagen, autor, valoración y comentarios).

## Cuentas

- **Salen solas**: todas las cuentas que Steam recuerda en este PC, con su foto de perfil, su nombre visible y su usuario. Puedes añadir también cuentas que Steam no conoce todavía.
- **Cambiar de cuenta**: pulsa «Entrar» (o doble clic, o Intro). TurboGAMES cierra Steam con cuidado (`steam.exe -shutdown`, y a la fuerza solo si no responde), deja la cuenta elegida como la de inicio automático y vuelve a abrir Steam, que entra directamente. Una pantalla va enseñando cada paso.
  - Antes de cerrar Steam pregunta (se puede desactivar en Ajustes o con «No volver a preguntar»), y avisa si la cuenta no tiene la sesión guardada y si tienes su contraseña en la bóveda. Si hay un juego abierto, pregunta siempre.
  - Solo abre `steam.exe` si está firmado por Valve, y a la fuerza solo cierra procesos de la carpeta de Steam (no cualquiera que se llame `steam.exe`).
  - **Entrar sin conexión** (el botón del wifi tachado): el modo sin conexión de Steam. Solo se puede si la cuenta tiene la sesión guardada en este PC.
- **Sesión guardada o no**: cada cuenta dice si Steam tiene su sesión guardada («Sesión guardada»: entra sin contraseña) o no («Pide contraseña»). Si Steam pide la contraseña al cambiar, la pantalla de cambio ofrece copiar el usuario y la contraseña.
- **Ficha de cada cuenta**: apodo, etiquetas (para filtrar: `#smurf`, `#familia`…), notas, fijar arriba y ocultar. Esto no es secreto y se guarda en `cuentas.json`.
- **Sus juegos**: a qué ha jugado cada cuenta en este PC y cuántas horas, y los de su biblioteca que aún no ha jugado, con un botón para jugar con ella.
- **Añadir una cuenta**: se guarda con su usuario y, si quieres, su contraseña (en la bóveda); «Guardar y entrar» cambia a ella al momento. La primera vez Steam pide la contraseña: marcando «Recordarme» ya no vuelve a pedirla. Las cuentas con las que entres directamente en Steam salen solas. Comprueba el usuario mientras lo escribes (que no esté vacío, que no sea el correo, solo letras sin acentos, números y `_`, de 3 a 64) y que no la tengas ya; si estaba oculta, vuelve a la lista.
- **Olvidar una cuenta**: la quita de la lista. Si además marcas «Que Steam también la olvide», TurboGAMES la quita de `loginusers.vdf` y borra su sesión guardada en este PC, también de las copias de seguridad (en los servidores de Steam no se toca nada).
- Antes de tocar `loginusers.vdf` o `local.vdf`, TurboGAMES guarda una copia en `%APPDATA%\TurboGAMES\copias` (las 10 últimas de cada uno).

## Bóveda (opcional)

- Guarda de cada cuenta su contraseña, el correo y su contraseña, el código de recuperación de Steam Guard y notas privadas.
- Va cifrada con una **contraseña maestra** (de al menos 12 caracteres) que no se guarda en ningún sitio: la clave se saca con Argon2id (256 MB, 6 pasadas, como TurboVAULT) y los datos van con AES-256-GCM, que además detecta cualquier cambio en el archivo. **Si olvidas la contraseña maestra, no hay forma de recuperar lo que haya dentro.**
  - Las bóvedas de versiones anteriores (64 MB, 3 pasadas, maestra de 8) se siguen abriendo igual y se refuerzan solas la primera vez que las abres; la maestra no cambia.
- **Comprueba lo que guardas**: el correo, que el código de recuperación sea una R y 5 números, que la contraseña del correo tenga su correo y avisa si la contraseña empieza o acaba con un espacio o si tienes Bloq Mayús activado. Si cierras con cambios sin guardar, pregunta.
- **Copiar sin dejar rastro**: lo que copias de la bóveda no entra en el historial del portapapeles de Windows (Win+V) ni en el portapapeles en la nube, y se borra solo a los 20 s (configurable) si no has copiado otra cosa después.
- **Se cierra sola** tras 5 minutos sin usarla (configurable), al bloquear Windows (Win+L), al suspender el PC y siempre al cerrar TurboGAMES.
- Desde el candado de la barra de título: cerrarla, cambiar la contraseña maestra o borrarla entera.
- El archivo es `%APPDATA%\TurboGAMES\boveda.dat` (y `boveda.dat.bak`, la versión anterior por si el disco falla a mitad de guardar).

## Juegos

- Junta los juegos instalados de todas las bibliotecas de Steam, los que ha jugado cada cuenta en este PC y los que tiene en su biblioteca sin haberlos jugado ni instalado (un juego recién comprado, por ejemplo), con su portada. De estos últimos Steam solo guarda en el PC los que se han abierto alguna vez en su biblioteca: basta con entrar en su ficha en Steam para que salgan. También salen los prestados por la familia de Steam, porque están en la biblioteca de la cuenta. Las portadas salen de la caché de Steam; las que no estén ahí se bajan una vez y se guardan en `%APPDATA%\TurboGAMES\cache\img`.
- Filtros por instalados y por cuenta, búsqueda (Ctrl+F) y orden por recientes, nombre, horas o espacio: la lista se recoloca mientras escribes sin volver a cargar las portadas.
- **Jugar**: lo abre con la cuenta que está dentro si ya lo ha jugado o lo tiene; si no, con la última que lo jugó o con una que lo tenga. El botón de las personas deja elegir otra. Antes de abrirlo comprueba:
  - si ya está abierto, o si hay otro juego abierto;
  - si no está instalado (te ofrece instalarlo, con la cuenta que lo ha jugado o lo tiene);
  - si Steam lo da por instalado pero falta su carpeta (te ofrece verificar los archivos) o la instalación está a medias;
  - si hay que cambiar de cuenta: siempre pregunta, y te deja jugar con la cuenta abierta si lo prefieres.
- Cada juego dice si necesita actualizarse, se está actualizando, está incompleto o le falta la carpeta.
- Por cada juego: horas (y las de las últimas 2 semanas), última partida, espacio en disco, carpeta, quién lo ha jugado (y qué cuentas lo tienen sin jugar), verlo en Steam, la tienda, verificar archivos y sus mods.
- **Etiquetas**: ponle a cada juego las tuyas (`#coop`, `#pendiente`…), guardadas en `juegos.json`. También salen las **colecciones de tu biblioteca de Steam** (Favoritos y las que hayas creado; las dinámicas no). Se filtra por ellas en el desplegable o escribiendo `#coop` en el buscador.
- **Más jugados**: los 20 con más horas, los jugados en las últimas 2 semanas y el orden «Estas 2 semanas», todas las cuentas juntas o la que elijas. Salen de lo que Steam ya guarda de cada cuenta: no hace falta TurboMON.

## Mods

La pestaña **Mods** empieza con tus juegos con Workshop, con sus portadas, cuántos mods y presets tiene cada uno y cuánto ocupan sus mods. Se filtran por los que tienen mods, presets, instalados o todos; por tus etiquetas y colecciones de Steam; y se ordenan por jugados hace poco, más mods, más espacio, horas o nombre. También se llega desde la ficha de cada juego («Mods»). Dentro de un juego:

- **Suscritos**: los mods con el nombre del Workshop (no el número de la carpeta), su imagen, autor, tamaño, cuándo se actualizó y el porcentaje de votos a favor. Dice si está bajando (y cuánto lleva), en cola, por actualizar o sin bajar todavía. También salen los que hay en el disco de otra cuenta del PC.
- **Buscar y filtrar**: un buscador ancho (por nombre, autor o `#etiqueta`) y el botón **Filtros**, que abre un panel por grupos: estado (suscritos, en el disco, sin bajar, por actualizar, bajando, sin suscripción, ya no existen o **los que tuviste antes**), etiquetas del autor en el Workshop (varias a la vez), tus etiquetas, tamaño, cuándo se actualizó y porcentaje de votos a favor. Los filtros puestos se ven como fichas que se quitan con la X. Se ordena por nombre, tamaño, actualizados, mejor valorados, suscriptores o suscritos hace poco.
- **Tres vistas**: lista, **detalles** (imagen grande, el principio de la descripción, etiquetas, porcentaje de votos a favor y cuántos votos, suscriptores, tamaño y fecha) y **cuadrícula** con imágenes grandes, título, valoración y suscriptores.
- **En bloque**: con las casillas («Elegir los N que se ven» arriba de la lista; con Mayús se eligen varios seguidos) sale abajo una barra (lo elegido se conserva aunque busques otra cosa o cambies de filtro; «N elegidos» abre la lista para repasarlos, buscar entre ellos y quitar alguno) para suscribirte, quitarlos, hacer un preset o ponerles o quitarles una etiqueta tuya de golpe. Por ejemplo: filtrar por una etiqueta del autor, elegir todos y quitarlos.
- **Tus etiquetas** en cada mod («imprescindible», «gráficos», «probar»…), en `mods.json`.
- **Los que tuviste antes**: TurboGAMES apunta, cada vez que conecta con Steam, a qué estaba suscrita cada cuenta. Los que quites (desde aquí o desde Steam) quedan ahí con la fecha, para volver a ponerlos.
- **Ficha de cada mod**: todas sus **imágenes y vídeos** (se ven en grande, y los vídeos de YouTube se reproducen dentro; ← → para pasar), descripción, valoración en estrellas, suscriptores, los mods que necesita (con «Suscribirse a los que faltan»), los últimos comentarios (si el perfil del autor es público), tus etiquetas y en qué presets está. Se abre en Steam, en el navegador o su carpeta.
- **Suscribirse y quitar** desde aquí, uno o varios a la vez. Funciona aunque el juego no esté instalado: Steam los baja cuando lo instales. Si no se puede (Steam cerrado…), ofrece abrir la página del mod en Steam.
- **Si la cuenta abierta no tiene el juego**, lo dice y ofrece cambiar a una de tus cuentas que sí lo tiene: con un «Aceptar» cambia de cuenta y sigue con lo que habías pedido (suscribirte, activar un preset…).
- **Buscar en el Workshop**: por texto (ordenado por parecido) o por populares de hoy, de la semana, del mes, de 3 o 6 meses o del año; mejor valorados o con más suscriptores de siempre; más nuevos o actualizados hace poco. En Filtros, cada etiqueta del Workshop se pulsa una vez para pedirla y otra para excluirla, y se pueden ocultar los que ya tienes. También con casillas para suscribirte a varios.
- **Quitar todo**: para empezar de cero (antes de reinstalar un juego, por ejemplo).

Para saber a qué está suscrita la cuenta y cambiarlo, TurboGAMES habla con el Steam abierto como lo haría el juego desde su menú de mods: un proceso aparte (el propio `TurboGAMES.exe --workshop`) se presenta a Steam como ese juego. No abre el juego ni toca sus archivos ni su memoria, así que no tiene nada que ver con los antitrampas (VAC solo mira dentro del juego). **Solo se conecta para lo que lo necesita** (leer tus suscripciones, suscribirte o quitar mods, buscar en el Workshop) y **se desconecta solo a los 2 segundos de terminar**: mientras está conectado, Steam da el juego por abierto (tus amigos te ven «jugando» y Steam no deja abrirlo de verdad). Arriba a la derecha se ve «Conectado» solo durante ese momento; después pasa a «Al día». Las fichas de los mods y lo que se carga en segundo plano no conectan por su cuenta. Si tienes **el juego abierto** (u otro), no se conecta y lo dice claro: «Tienes RimWorld abierto: ciérralo para gestionar sus mods desde aquí»; al cerrarlo, el aviso cambia solo. Al pulsar «Jugar», si acababa de estar conectado, espera a que Steam suelte el juego (unos segundos como mucho) para que se abra a la primera. También se desconecta antes de cambiar de cuenta. En Ajustes se puede hacer que solo lea tus suscripciones al pulsar «Conectar con Steam». Los cambios van por tandas, con 10 a la vez en marcha (como hace el propio Steam al cancelar todas las suscripciones): cientos de mods se quitan en poco tiempo. Lo que falle por algo pasajero se vuelve a intentar una vez.

### Presets

Un preset es una lista de mods de un juego con su nombre, quién lo hizo, **quién te lo pasó**, etiquetas y notas (en `presets.json`).

- **Activar**: te suscribe a sus mods y quita los demás, así solo ocupa lo que vas a jugar. **Añadir** solo suscribe los que faltan. Se pueden activar varios juntos (con las casillas). Antes de hacer nada enseña cuántos se suscriben, cuántos se quitan y cuántos ya tienes.
- **Copia automática**: antes de quitar cualquier suscripción, TurboGAMES guarda lo que tenías como preset (las 5 últimas de cada juego). Así siempre puedes volver atrás.
- **Compartir por texto**: «Copiar» deja un texto para cualquier chat con la lista de mods (nombre y enlace) y, al final, un código `TGP1:…` con todo el preset. Si el chat tiene poco espacio, «Copiar solo el código»; si son muchos, «Guardar como archivo».
- **Pegar preset**: lee ese texto (aunque el chat haya partido el código en varias líneas), enlaces o números de mods del Workshop, colecciones del Workshop (se cambian por sus mods) y los presets exportados del lanzador de Arma 3 (`.html`). Detecta el juego, avisa de los mods que ya no existen o son de otro juego, y te deja poner quién te lo ha pasado y etiquetas antes de guardarlo. Guardarlo no cambia nada: activarlo es otro paso.
- Se filtran por etiqueta y por quién te los pasó.

## Ajustes

Carpeta de Steam (se busca sola), preguntar antes de cambiar de cuenta, abrir Steam minimizado al cambiar de cuenta, conectar con Steam al ver los mods, minimizar TurboGAMES al terminar, tiempo para cerrar la bóveda y tiempo para borrar el portapapeles. Todo en `%APPDATA%\TurboGAMES\ajustes.json`.

## Teclado

Ctrl+F buscar · Ctrl+1 cuentas · Ctrl+2 juegos · Ctrl+3 mods · flechas para moverte · Intro entrar o jugar · Esc cerrar · Ctrl+Intro leer el preset pegado.

## Cómo funciona por dentro

| Qué | Dónde lo guarda Steam |
|---|---|
| Cuentas que recuerda | `config\loginusers.vdf` (`AccountName`, `PersonaName`, `AutoLogin`…) |
| Cuenta de inicio automático | Registro: `HKCU\Software\Valve\Steam\AutoLoginUser` |
| Cuenta abierta ahora | Registro: `HKCU\Software\Valve\Steam\ActiveProcess\ActiveUser` |
| Juego abierto | Registro: `HKCU\Software\Valve\Steam\RunningAppID` |
| Sesiones guardadas | `%LOCALAPPDATA%\Steam\local.vdf`, en `ConnectCache`: la clave es el CRC32 del usuario en minúsculas más `1`. El valor va cifrado por Steam y TurboGAMES no lo lee: solo mira si está. |
| Fotos de perfil | `config\avatarcache\<SteamID64>.png` |
| Juegos instalados | `steamapps\appmanifest_<id>.acf` de cada biblioteca (`config\libraryfolders.vdf`) |
| Horas por cuenta | `userdata\<id>\config\localconfig.vdf` |
| Juegos de la biblioteca sin jugar | `userdata\<id>\config\librarycache\<appid>.json` (Steam lo crea al abrir la ficha del juego; la lista completa de licencias, `licensecache`, va cifrada) |
| Nombres y portadas | `appcache\appinfo.vdf` (binario, versiones 27 a 29) y `appcache\librarycache\<id>\` |
| Horas de las últimas 2 semanas | `Playtime2wks` en `localconfig.vdf` (Steam solo lo pone al día al jugar: si la última partida es de hace más de 2 semanas, cuenta 0) |
| Colecciones de la biblioteca | `userdata\<id>\config\cloudstorage\cloud-storage-namespace-1.json` (`user-collections.*`) |
| Si un juego tiene Workshop | `workshop_visible` en `appinfo.vdf` |
| Mods en el disco y quién se suscribió | `steamapps\workshop\appworkshop_<appid>.acf` de cada biblioteca (`subscribedby`) y `steamapps\workshop\content\<appid>\<id>\` |
| Suscripciones de la cuenta, votos, dependencias y búsqueda | El Steam abierto, por `steamclient64.dll` (la que trae Steam, firmada por Valve): `ISteamUGC` (versión 020). Las posiciones de cada función salen de las cabeceras del SDK contando los métodos privados, que no salen en `steam_api.json` |
| Nombre, imagen, tamaño y suscriptores de un mod | API pública `ISteamRemoteStorage/GetPublishedFileDetails` y `GetCollectionDetails` (sin clave) |
| Valoración, mods que necesita y comentarios | La página pública del mod y `comment/PublishedFile_Public/render` |
| Imágenes y vídeos de un mod | Si justo hay conexión con Steam, `GetQueryUGCAdditionalPreview`; si no, de la página pública del mod |

## Compilar

Necesita Go y Wails v2:

```powershell
.\build.ps1
```

Para leer el Steam de este PC sin cambiar nada:

```powershell
$env:TURBOGAMES_REAL = "1"; go test -run Real -v ./internal/steam/
```

El Workshop con el Steam abierto, solo leyendo (no suscribe ni quita nada; mientras dura, Steam te enseña «jugando» a ese juego):

```powershell
$env:TURBOGAMES_WORKSHOP = "<appid de un juego con mods>"; go test -run Real -v ./internal/steamworks/
$env:TURBOGAMES_WORKSHOP = "<appid>"; go test -run WorkshopReal -v .
$env:TURBOGAMES_WEB = "1"; go test -run WebReal -v .     # la web pública de Steam
```

La interfaz abierta en un navegador (`frontend/index.html`) enseña datos de muestra; la contraseña maestra de muestra es `maestra123`. El icono se rehace con `go run dev/icons/genapp.go` a partir de `frontend/logo.svg`.

| Archivo | Qué hace |
|---|---|
| `internal/vdf` | Lee y escribe los `.vdf` de texto de Steam sin cambiar nada que no se toque. |
| `internal/steam` | Busca Steam, lee cuentas, sesiones, bibliotecas, horas y `appinfo.vdf`; cierra, prepara y abre Steam. |
| `internal/vault` | La bóveda cifrada. |
| `accounts.go` | Junta lo de Steam con las fichas de `cuentas.json`. |
| `switching.go` | El cambio de cuenta paso a paso. |
| `vaultapi.go`, `clipboard.go` | La bóveda vista desde la interfaz y el portapapeles que no deja rastro. |
| `games.go`, `images.go` | La biblioteca y las imágenes (`/img/app/<id>/<cover\|header\|hero\|logo>`, `/img/avatar/<SteamID64>`, `/img/mod/<id>`). |
| `gamemeta.go` | Las etiquetas de cada juego (`juegos.json`). |
| `internal/steamworks` | Habla con el Steam abierto por `steamclient64.dll`: suscripciones, estado de cada mod, datos con votos y búsqueda en el Workshop. |
| `workshop_host.go` | El proceso aparte del Workshop (`--workshop`) y cómo se abre, se le pregunta y se cierra. |
| `workshop.go` | La pestaña Mods: lista, ficha de cada mod, búsqueda, cambios por lotes y la caché `cache\workshop.json`. |
| `modmeta.go` | Tus etiquetas de los mods y el historial de suscripciones de cada cuenta (`mods.json`). |
| `presets.go` | Los presets: guardarlos, el texto y el código `TGP1` para compartirlos, leer lo pegado (también del lanzador de Arma 3) y las copias automáticas. |
| `frontend/` | Interfaz (HTML, CSS y JS sin dependencias). |
