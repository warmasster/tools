# Turbo tools

Programas para Windows hechos a medida: rápidos, en un solo `.exe` y con una interfaz cuidada.

## Descargar

**[⬇ TurboSETUP.exe](https://github.com/warmasster/tools/releases/download/apps/TurboSETUP.exe)**: ábrelo, elige qué herramientas quieres y las instala. También se encarga de las actualizaciones.

> Windows puede avisar de que el programa no está firmado: *Más información → Ejecutar de todas formas*.

## Herramientas

| | Qué hace |
|---|---|
| [**TurboSEND**](turbosend/README.md) | Envía y recibe archivos de cualquier tamaño en chats. El otro lado solo necesita el navegador; va por Cloudflare o directo por tu IP. |
| [**TurboCUT**](turbocut/README.md) | Analiza el disco en segundos (lee la MFT de NTFS) y te enseña qué ocupa más. Busca cualquier archivo al instante mientras escribes, encuentra duplicados y ayuda a liberar espacio: temporales, cachés, papelera, descargas viejas. |
| [**TurboDL**](turbodl/README.md) | Extensión de Chrome que descarga por partes en paralelo, pausa, reanuda y sobrevive a cerrar el navegador. Con motor nativo, hasta 64 conexiones. |
| [**TurboBLOCK**](turboblock/README.md) | Bloqueador de anuncios, rastreadores, malware y ventanas emergentes para Chrome. El filtrado lo hace Chrome por dentro y las listas se actualizan solas. |
| [**TurboMENU**](turbomenu/README.md) | Añade «Turbo» al clic derecho: reducir tamaño (vídeo para Discord o WhatsApp, imágenes, PDF), convertir a cualquier formato, pasar al móvil con un QR, ver si un archivo es seguro, qué programa lo bloquea, de dónde se descargó y más. Todo en ventanitas, sin abrir otros programas. |
| [**TurboGAMES**](turbogames/README.md) | Todas tus cuentas de Steam a un clic, sin el límite del selector de Steam: cambia de cuenta solo, avisa de cuáles piden contraseña y guarda las contraseñas en una bóveda cifrada con contraseña maestra. También enseña tus juegos con sus portadas, quién ha jugado a cada uno y cuánto, y los abre con la cuenta que toque. |
| [**TurboSETUP**](installer/README.md) | El instalador: descarga las herramientas de este repo y las mantiene al día. |

## Compilar

Cada carpeta tiene su `build.ps1`. Hace falta [Go](https://go.dev) y, para las apps con ventana, [Wails v2](https://wails.io).

Las versiones se publican solas: al subir el número de versión de un programa y hacer push, GitHub Actions lo compila y lo sube a la Release. Más detalles en [TurboSETUP](installer/README.md#cómo-se-publica-una-versión).
