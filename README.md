# tools

Herramientas propias: programas para Windows (un único `.exe` sin dependencias) y extensiones de Chrome, con interfaz cuidada.

| Herramienta | Qué hace | Descarga |
|---|---|---|
| [Enviador](enviador-online/) | Envía archivos de cualquier tamaño por Cloudflare o directo por tu IP (UPnP). El que recibe solo necesita el navegador. | [Releases](../../releases) |
| [Enviador v2](enviador-online/v2/) | Lo mismo, convertido en chats: mensajes y archivos en los dos sentidos, para una persona o un grupo. | [Releases](../../releases) |
| [TurboDL](turbodl/) | Extensión de Chrome: gestor de descargas multiconexión que pausa, reanuda y sobrevive a cerrar el navegador. Con su [motor nativo en Go](turbodl/native/) opcional, hasta 64 conexiones. | Cargar descomprimida + `turbodl-native.exe` |

## Compilar

Cada programa trae su `build.ps1`. Necesitan [Go](https://go.dev) y [Wails v2](https://wails.io). La extensión TurboDL no se compila: se carga tal cual en `chrome://extensions`; su motor nativo se compila con `turbodl/native/build.ps1` (solo Go).
