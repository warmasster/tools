# TurboGAMES

Todas tus cuentas de Steam a un clic, sin el límite del selector de Steam, con sus contraseñas guardadas cifradas si quieres. También enseña tus juegos con sus portadas, quién ha jugado a cada uno y cuánto, y los abre con la cuenta que toque.

- **`TurboGAMES.exe`**: un único ejecutable. Se instala con [TurboSETUP](../installer/) o se descarga suelto de la [Release](https://github.com/warmasster/tools/releases/tag/apps); no necesita instalar nada.
- No usa internet para las cuentas: lee y cambia lo que Steam guarda en tu PC. Solo baja de los servidores de imágenes de Steam las portadas que Steam no tenga ya guardadas.

## Cuentas

- **Salen solas**: todas las cuentas que Steam recuerda en este PC, con su foto de perfil, su nombre visible y su usuario. Puedes añadir también cuentas que Steam no conoce todavía.
- **Cambiar de cuenta**: pulsa «Entrar» (o doble clic, o Intro). TurboGAMES cierra Steam con cuidado (`steam.exe -shutdown`, y a la fuerza solo si no responde), deja la cuenta elegida como la de inicio automático y vuelve a abrir Steam, que entra directamente. Una pantalla va enseñando cada paso.
  - Antes de cerrar Steam pregunta (se puede desactivar en Ajustes o con «No volver a preguntar»), y avisa si la cuenta no tiene la sesión guardada y si tienes su contraseña en la bóveda. Si hay un juego abierto, pregunta siempre.
  - **Entrar sin conexión** (el botón del wifi tachado): el modo sin conexión de Steam. Solo se puede si la cuenta tiene la sesión guardada en este PC.
- **Sesión guardada o no**: cada cuenta dice si Steam tiene su sesión guardada («Sesión guardada»: entra sin contraseña) o no («Pide contraseña»). Si Steam pide la contraseña al cambiar, la pantalla de cambio ofrece copiar el usuario y la contraseña.
- **Ficha de cada cuenta**: apodo, etiquetas (para filtrar: `#smurf`, `#familia`…), notas, fijar arriba y ocultar. Esto no es secreto y se guarda en `cuentas.json`.
- **Sus juegos**: a qué ha jugado cada cuenta en este PC y cuántas horas, con un botón para jugar con ella.
- **Añadir una cuenta**: se guarda con su usuario y, si quieres, su contraseña (en la bóveda); «Guardar y entrar» cambia a ella al momento. La primera vez Steam pide la contraseña: marcando «Recordarme» ya no vuelve a pedirla. Las cuentas con las que entres directamente en Steam salen solas. Comprueba el usuario mientras lo escribes (que no esté vacío, que no sea el correo, solo letras sin acentos, números y `_`, de 3 a 64) y que no la tengas ya; si estaba oculta, vuelve a la lista.
- **Olvidar una cuenta**: la quita de la lista. Si además marcas «Que Steam también la olvide», TurboGAMES la quita de `loginusers.vdf` y borra su sesión guardada en este PC (en los servidores de Steam no se toca nada).
- Antes de tocar `loginusers.vdf` o `local.vdf`, TurboGAMES guarda una copia en `%APPDATA%\TurboGAMES\copias` (las 10 últimas de cada uno).

## Bóveda (opcional)

- Guarda de cada cuenta su contraseña, el correo y su contraseña, el código de recuperación de Steam Guard y notas privadas.
- Va cifrada con una **contraseña maestra** que no se guarda en ningún sitio: la clave se saca con Argon2id (64 MB, 3 pasadas) y los datos van con AES-256-GCM, que además detecta cualquier cambio en el archivo. **Si olvidas la contraseña maestra, no hay forma de recuperar lo que haya dentro.**
- **Comprueba lo que guardas**: el correo, que el código de recuperación sea una R y 5 números, que la contraseña del correo tenga su correo y avisa si la contraseña empieza o acaba con un espacio o si tienes Bloq Mayús activado. Si cierras con cambios sin guardar, pregunta.
- **Copiar sin dejar rastro**: lo que copias de la bóveda no entra en el historial del portapapeles de Windows (Win+V) ni en el portapapeles en la nube, y se borra solo a los 20 s (configurable) si no has copiado otra cosa después.
- **Se cierra sola** tras 5 minutos sin usarla (configurable) y siempre al cerrar TurboGAMES.
- Desde el candado de la barra de título: cerrarla, cambiar la contraseña maestra o borrarla entera.
- El archivo es `%APPDATA%\TurboGAMES\boveda.dat` (y `boveda.dat.bak`, la versión anterior por si el disco falla a mitad de guardar).

## Juegos

- Junta los juegos instalados de todas las bibliotecas de Steam y los que ha jugado cada cuenta en este PC, con su portada. Las portadas salen de la caché de Steam; las que no estén ahí se bajan una vez y se guardan en `%APPDATA%\TurboGAMES\cache\img`.
- Filtros por instalados y por cuenta, búsqueda (Ctrl+F) y orden por recientes, nombre, horas o espacio: la lista se recoloca mientras escribes sin volver a cargar las portadas.
- **Jugar**: lo abre con la cuenta que está dentro si ya lo ha jugado; si no, con la última que lo jugó. El botón de las personas deja elegir otra. Antes de abrirlo comprueba:
  - si ya está abierto, o si hay otro juego abierto;
  - si no está instalado (te ofrece instalarlo, con la cuenta que lo ha jugado);
  - si Steam lo da por instalado pero falta su carpeta (te ofrece verificar los archivos) o la instalación está a medias;
  - si hay que cambiar de cuenta: siempre pregunta, y te deja jugar con la cuenta abierta si lo prefieres.
- Cada juego dice si necesita actualizarse, se está actualizando, está incompleto o le falta la carpeta.
- Por cada juego: horas, última partida, espacio en disco, carpeta, quién lo ha jugado, verlo en Steam, la tienda y verificar archivos.

## Ajustes

Carpeta de Steam (se busca sola), preguntar antes de cambiar de cuenta, abrir Steam minimizado al cambiar de cuenta, minimizar TurboGAMES al terminar, tiempo para cerrar la bóveda y tiempo para borrar el portapapeles. Todo en `%APPDATA%\TurboGAMES\ajustes.json`.

## Teclado

Ctrl+F buscar · Ctrl+1 cuentas · Ctrl+2 juegos · flechas para moverte · Intro entrar o jugar · Esc cerrar.

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
| Nombres y portadas | `appcache\appinfo.vdf` (binario, versiones 27 a 29) y `appcache\librarycache\<id>\` |

## Compilar

Necesita Go y Wails v2:

```powershell
.\build.ps1
```

Para leer el Steam de este PC sin cambiar nada:

```powershell
$env:TURBOGAMES_REAL = "1"; go test -run Real -v ./internal/steam/
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
| `games.go`, `images.go` | La biblioteca y las imágenes (`/img/app/<id>/<cover\|header\|hero\|logo>`, `/img/avatar/<SteamID64>`). |
| `frontend/` | Interfaz (HTML, CSS y JS sin dependencias). |
