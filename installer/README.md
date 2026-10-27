# TurboSETUP

Instalador de las herramientas Turbo. Enseña la lista, instalas las que quieras con un clic y, cada vez que lo abres, te avisa si hay versiones nuevas.

- **Descarga de GitHub**: todo sale de la Release [`apps`](https://github.com/warmasster/tools/releases/tag/apps) de este repo. Antes de instalar comprueba el tamaño y el SHA-256 de cada archivo.
- **Sin permisos de administrador**: instala en `%LOCALAPPDATA%\Programs\Turbo\<Herramienta>`.
- **Accesos directos** en el menú Inicio y, si quieres, en el escritorio.
- **Aparece en Configuración → Aplicaciones**, desde donde también se puede desinstalar.
- **Actualiza aunque el programa esté abierto**: el nuevo se usa la próxima vez que lo abras.
- **Se actualiza a sí mismo** cuando sale una versión nueva de TurboSETUP.
- **TurboDL** es una extensión de Chrome: TurboSETUP baja `TurboDL.exe` y lo lanza, y TurboDL te guía para cargarla en tu navegador (Chrome, Edge o Brave: abre la página de extensiones del que uses).
- Las que se instalan solas **y** tienen ventana (TurboVAULT, TurboMENU, TurboCAM) se pueden **abrir** desde aquí; el botón pequeño de al lado las reinstala.
- **Si falta el programa** de una herramienta (lo borraste a mano o se lo llevó un antivirus), sale como «Falta el programa» con **Reparar**. Al desinstalarla, se vuelve a bajar un momento para que quite todo lo que dejó puesto (clic derecho, arranque con Windows, puente de la extensión, cámara virtual); sin conexión, quita lo que deja en el registro.

Al instalar la primera herramienta deja una copia de sí mismo en la carpeta de instalación, con su acceso directo en el menú Inicio. Al quitar la última, borra la carpeta entera.

## Cómo se publica una versión

1. Sube la versión del programa: `productVersion` en su `wails.json` (en TurboDL, `version` en `manifest.json`).
2. Haz push a `main`.

GitHub Actions ([`release.yml`](../.github/workflows/release.yml)) compila solo lo que ha cambiado de versión, lo sube a la Release `apps` y actualiza `manifest.json`, que es lo que lee TurboSETUP. Cada `.exe` se sube también con la versión en el nombre (`TurboSEND-2.1.2.exe`): TurboSETUP 1.0.7 o posterior baja ese, así que mientras se publica nunca ve un archivo a medio cambiar. El de nombre fijo se sigue subiendo (para el enlace de esta página y los TurboSETUP de antes); los dos van antes que el catálogo, y las copias con versión de hace más de una publicación se borran. Es la única Release del repo: si encuentra otras, las borra. La lista de herramientas está en [`tools.json`](../tools.json): para añadir una nueva basta con añadirla ahí (`kind: "app"` si se abre con acceso directo, `"setup"` si su `.exe` es su propio instalador, y `"open": true` si además tiene ventana).

Para probar sin subir nada:

```powershell
..\.github\scripts\release.ps1 -OutDir dist        # compila todo en dist\
python -m http.server 8799 --directory dist
$env:TURBOSETUP_BASE = "http://localhost:8799"; .\TurboSETUP.exe
```

## Compilar

Necesita [Go](https://go.dev) y [Wails v2](https://wails.io):

```powershell
.\build.ps1
```

| Archivo | Qué hace |
|---|---|
| `catalog.go` | Lee `manifest.json` de la Release y lo cruza con lo instalado |
| `install.go` | Descargar, comprobar, instalar, registrar y desinstalar |
| `winutil.go` | Accesos directos, avisos de Windows y procesos sin consola |
| `app.go` | Lo que llama la ventana |
| `frontend/` | Interfaz (HTML, CSS y JS sin dependencias). Abierta en un navegador enseña datos de muestra. |
| `e2e_test.go` | Prueba completa contra un servidor local (`$env:TURBOSETUP_E2E`) |
