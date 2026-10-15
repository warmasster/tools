# tools

Herramientas propias: programas para Windows (un único `.exe` sin dependencias) y extensiones de Chrome, con interfaz cuidada.

| Herramienta | Qué hace | Descarga |
|---|---|---|
| [Enviador](enviador-online/) | Envía archivos de cualquier tamaño por Cloudflare o directo por tu IP (UPnP). El que recibe solo necesita el navegador. | [Releases](../../releases) |
| [Enviador v2](enviador-online/v2/) | Lo mismo, convertido en chats: mensajes y archivos en los dos sentidos, para una persona o un grupo. | [Releases](../../releases) |
| [TurboDL](turbodl/) | Extensión de Chrome: gestor de descargas multiconexión que pausa, reanuda y sobrevive a cerrar el navegador. | Cargar descomprimida |

## Compilar

Cada programa trae su `build.ps1`. Necesitan [Go](https://go.dev) y [Wails v2](https://wails.io). TurboDL no se compila: se carga tal cual en `chrome://extensions`.
