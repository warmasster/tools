# TurboCUT

Analiza un disco o una carpeta a toda velocidad, te enseña qué ocupa más y te ayuda a borrar lo que sobra. Es como WinDirStat o WizTree, con un buscador instantáneo como Everything y un buscador de duplicados, todo sobre el mismo análisis y con la misma interfaz que el resto de herramientas.

- **`TurboCUT.exe`**: un único ejecutable (~12 MB). Se instala con [TurboSETUP](../installer/) o se descarga suelto de la [Release](https://github.com/warmasster/tools/releases/tag/apps); no necesita instalar nada.
- **Dos formas de escanear**:
  - **Modo normal** (sin permisos): recorre las carpetas en paralelo. Un disco de 600 GB con 1,3 millones de archivos tarda unos 25 s la primera vez y unos 5 s las siguientes, porque Windows ya tiene las carpetas en caché. Un archivo con varios nombres (enlaces duros, como los de `C:\Windows\WinSxS`) cuenta una sola vez, igual que en modo turbo; sus otros nombres salen con la etiqueta «enlace duro» y no suman (no ocupan más). Solo en discos locales NTFS o ReFS: en unidades de red cada nombre cuenta, como antes.
  - **⚡ Modo turbo** (como administrador): lee la MFT, el índice interno de NTFS, de un tirón, igual que WizTree. El mismo disco de 600 GB tarda unos 3-4 s, incluso la primera vez, y además ve las carpetas protegidas (`System Volume Information`, la papelera de otros usuarios…). Se activa con el botón «Reiniciar en modo turbo» o en Ajustes → «Abrir siempre en modo turbo».
- **Qué enseña**:
  - **Carpetas**: lista ordenada por tamaño, con barras, número de archivos y fecha. Arriba se ve siempre la ruta entera, con un botón para copiarla.
  - **Mapa** (botón «Mapa»): rectángulos (treemap) donde cada color es un tipo de archivo. Al pasar el ratón por encima dice qué es; con doble clic entras en esa carpeta. Sale oculto por defecto.
  - **Archivos más grandes**: los 300 mayores de la carpeta actual y sus subcarpetas. Puedes filtrarlos por tipo pulsando en el panel «Por tipo».
  - **Buscar**: busca por nombre en todo lo analizado mientras escribes (unos 20 ms con 1,3 millones de archivos). Filtros por tipo, tamaño y fecha, solo archivos o solo carpetas, y en todo el análisis o solo en la carpeta actual. Varias palabras se buscan a la vez; `"entre comillas"` busca la frase; `.mp4` o `ext:mp4,mkv` filtran por extensión.
  - **Duplicados**: encuentra archivos idénticos. Primero agrupa por tamaño sin leer nada; luego descarta los enlaces duros (son el mismo archivo), compara el principio y el final, y a los que siguen empatados les lee el contenido entero (SHA-256). «Marcar copias» marca todas menos una por grupo (la más antigua, la más reciente o la de ruta más corta) y nunca deja marcar todas. Por defecto no mira Windows, Archivos de programa, AppData, `node_modules` ni `.git`.
  - **Limpieza**: busca sola temporales, cachés de navegadores, de shaders, de apps y de programación, volcados de errores, la papelera, instaladores y ZIP viejos en Descargas, `node_modules` sin tocar, carpetas `target` de Rust y modelos de IA. Lo separa en «Seguro de borrar», «Revisa antes» y «Para que lo sepas» (`hiberfil.sys`, `pagefile.sys`).
  - **Qué ha crecido**: compara con el análisis anterior de la misma unidad o carpeta y te dice qué carpetas han crecido o bajado. En Ajustes se puede activar una flecha en la lista para cada carpeta que haya cambiado, y elegir a partir de cuánto se avisa (100 MB por defecto). Si el análisis anterior se hizo en el otro modo (o con una versión que contaba distinto los enlaces duros), lo avisa.
- **Borrar con seguridad**:
  - Por defecto todo va a la papelera. Si algo no cabe en ella, Windows avisa antes de borrarlo para siempre.
  - «Borrar para siempre» pide confirmación aparte.
  - Si intentas borrar algo de Windows, de Archivos de programa o tu carpeta de usuario, TurboCUT avisa y no deja seguir hasta que marques «Sé lo que hago».
  - Si un archivo está en uso, dice qué programa lo tiene abierto.
  - Las cachés y los temporales de la pestaña Limpieza se borran directamente, porque mandarlos a la papelera no liberaría espacio. Lo que esté en uso se salta solo.

## Uso

- **Analizar**: pulsa una unidad, usa «Elegir una carpeta…» o arrastra una carpeta a la ventana.
- **Moverse**: un clic en una carpeta para entrar (o Intro), Retroceso para subir, Alt+← para volver atrás, Ctrl+F para buscar (Esc vuelve a donde estabas). En los archivos, doble clic los abre (en modo turbo los abre el Explorador, sin permisos de administrador).
- **Borrar uno**: el icono de la papelera de cada fila, o Supr (Mayús+Supr lo borra para siempre).
- **Seleccionar varios**: el botón «Seleccionar» enseña las casillas. También vale Ctrl+clic o Espacio.
- **Botón derecho**: abrir, mostrar en el Explorador, copiar ruta, volver a leer la carpeta o borrar. «Volver a leer» no bloquea la ventana (puedes seguir mirando) y se puede cancelar.
- **«En disco» o «Tamaño real»** (Ajustes): «En disco» es lo que de verdad ocupa. No cuenta los archivos de OneDrive que solo están en la nube y sí tiene en cuenta la compresión.

Los ajustes y el historial de análisis se guardan en `%APPDATA%\TurboCUT`. El historial solo guarda el tamaño de las carpetas de más de 32 MB (unos 100 KB por disco) y como mucho los últimos 30 análisis.

## Compilar

Necesita Go y Wails v2:

```powershell
.\build.ps1
```

Las pruebas incluyen un disco NTFS falso en memoria para probar el lector de la MFT sin ser administrador. Para medir un disco de verdad:

```powershell
$env:TURBOCUT_SCAN = "C:\"; go test -run Real -v .
$env:TURBOCUT_DUPES = "1"   # además, busca duplicados de 10 MB o más
```

Para probar la interfaz con `wails dev` sin que se relance como administrador (si está activado «Abrir siempre en modo turbo»), pon antes `$env:TURBOCUT_NO_ELEVATE = "1"`.

Estructura:

| Archivo | Qué hace |
|---|---|
| `walk.go` | Modo normal: lee carpetas en paralelo con `GetFileInformationByHandleEx` (tamaños y fechas de golpe). |
| `mft.go` | Modo turbo: abre `\\.\C:`, lee la MFT y reconstruye el árbol en memoria. |
| `tree.go` | El árbol en arrays planos (48 bytes por archivo) y sus operaciones. |
| `cleanup.go` | Reglas de la pestaña Limpieza. |
| `history.go` | Guarda las carpetas grandes de cada análisis y calcula qué ha crecido. |
| `delete.go`, `winapi.go` | Papelera, borrado definitivo, vaciar papelera y quién tiene un archivo abierto (Restart Manager). |
| `find.go` | Pestaña Buscar: búsqueda por nombre con filtros sobre el árbol en memoria. |
| `dupes.go` | Pestaña Duplicados: tamaños, enlaces duros, principio y final, contenido entero. |
| `app.go` | Lo que llama la interfaz: listados, mapa, archivos más grandes, tipos… |
| `frontend/` | Interfaz (HTML, CSS y JS sin dependencias). |
