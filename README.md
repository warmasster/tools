# tools

Herramientas propias: programas para Windows (un único `.exe` sin dependencias) y extensiones de Chrome, con interfaz cuidada.

| Herramienta | Qué hace | Descarga |
|---|---|---|
| [TurboSEND](enviador-online/v2/) | Envía y recibe archivos de cualquier tamaño en chats, por Cloudflare o directo por tu IP (UPnP). El otro lado solo necesita el navegador. La [v1](enviador-online/) era solo para envíos sueltos. | [Releases](../../releases) (`TurboSEND.exe`) |
| [TurboDL](turbodl/) | Extensión de Chrome: gestor de descargas multiconexión que pausa, reanuda y sobrevive a cerrar el navegador. Con su [motor nativo en Go](turbodl/native/) opcional, hasta 64 conexiones. | [Releases](../../releases) (`TurboDL.exe`) |

## Compilar

Cada programa trae su `build.ps1`. Necesitan [Go](https://go.dev) y [Wails v2](https://wails.io). La extensión TurboDL no se compila: se carga tal cual en `chrome://extensions`; su motor nativo se compila con `turbodl/native/build.ps1` (solo Go).
