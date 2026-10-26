# TurboMENU

Añade una entrada **Turbo** al clic derecho del Explorador con acciones rápidas. Cada una se abre en una ventanita junto al cursor, con la misma estética que el resto de herramientas Turbo, y sin abrir otros programas delante.

Solo salen las opciones que tienen sentido para lo que has pulsado:

| Dónde pulsas | Opciones |
|---|---|
| Un archivo | Reducir tamaño · Convertir (si se puede) · Enviar con TurboSEND · Pasar al móvil · ¿Es seguro? · ¿Qué lo bloquea? · ¿De dónde salió? · Nueva carpeta con esto |
| Una carpeta | Reducir tamaño (ZIP) · Enviar con TurboSEND · Pasar al móvil · ¿Qué lo bloquea? · Nueva carpeta con esto |
| Un hueco de una carpeta o del escritorio | Pegar como archivo · Recibir del móvil |
| Una unidad (USB) | ¿Qué lo bloquea? (para poder expulsarla) |

Con varios archivos seleccionados, todos llegan a la misma ventana.

## Qué hace cada opción

- **Reducir tamaño**: detecta el tipo de archivo y enseña opciones adaptadas.
  - **Vídeo**: «Para Discord» (menos de 10 MB), «Para WhatsApp» (menos de 16 MB), calidad alta/media/baja con el peso estimado de cada una, o el peso exacto que escribas. Usa la tarjeta gráfica si la hay (NVIDIA, Intel o AMD). Siempre queda por debajo del peso pedido.
  - **Imagen**: barra de calidad con vista previa antes/después (se arrastra la línea del medio) y el peso exacto que tendrá; también se puede hacer más pequeña.
  - **PDF**: «Para leer en pantalla» o «Para imprimir» (recomprime las imágenes de dentro).
  - **Audio**: calidad alta, media o baja.
  - **Carpetas y lo que no se puede reducir**: lo empaqueta en un ZIP.
  - Al terminar: «45 MB → 8,7 MB (−81 %)», con Abrir, Mostrar en la carpeta, Enviar con TurboSEND y Pasar al móvil. El original no se toca, salvo que marques «Reemplazar el original» (entonces va a la papelera).
- **Convertir**: solo los formatos posibles, cada uno con una frase que dice para qué sirve; los que usas a menudo salen arriba. Vídeo → MP4, WEBM, MKV, MOV, AVI, GIF o solo el audio; audio entre MP3, M4A, OGG, Opus, FLAC y WAV; imágenes entre JPG, PNG, WEBP, AVIF, GIF, BMP, TIFF, ICO y PDF; varias imágenes → un PDF; PDF → imágenes; Word, Excel y PowerPoint → PDF.
- **Enviar con TurboSEND**: eliges el chat (o uno nuevo) en la propia ventanita y te da el enlace ya copiado. TurboSEND trabaja por detrás, minimizado. Si es muy grande, te ofrece reducirlo antes.
- **Pasar al móvil**: un QR grande; lo escaneas con la cámara y se descarga, sin instalar nada. Va por internet: por un enlace seguro de Cloudflare (funciona siempre) o directo por tu IP, con el puerto abierto en el router por UPnP. Al cerrar la ventana se apaga todo y se libera el puerto.
  - Al tocar «Descargar», la página del móvil enseña el progreso real («Descargando 45 % · 8 MB/s · quedan 20 s»), no deja empezar la misma descarga dos veces y al acabar dice dónde ha quedado el archivo. En el PC sale una fila por descarga.
- **Recibir del móvil**: lo mismo al revés; desde el móvil eliges fotos o archivos y se guardan en esa carpeta. Van por trozos: si se corta la conexión, reintenta solo, y si recargas la página y vuelves a elegir el mismo archivo, sigue por donde iba. Mientras llega, el archivo a medias está oculto.
- **¿Es seguro?**: veredicto claro (🟢 limpio, 🟡 sospechoso, 🔴 peligroso) con lo que lo justifica: Microsoft Defender, la firma digital (quién lo firma y si es válida), de dónde se descargó y, con una clave gratuita de VirusTotal, cuántos de unos 70 antivirus lo detectan (se consulta la huella, el archivo no se sube). Si nadie lo ha visto antes, pregunta si quieres enviarlo a analizar.
- **¿Qué lo bloquea?**: qué programas están usando el archivo, la carpeta o el USB. «Liberar» cierra el programa como al pulsar la X (si tiene cambios sin guardar, avisa y ofrece forzar). Al Explorador y a Windows nunca los cierra: solo les quita ese archivo. También «Liberar y borrar», «Liberar y mover» y, en unidades, «Liberar y expulsar».
- **¿De dónde salió?**: la web desde la que se descargó, el enlace exacto y la fecha, con botones para abrirla o copiar el enlace. Si no viene de internet, lo dice.
- **Nueva carpeta con esto**: mete la selección en una carpeta nueva con un nombre sugerido por lo que tienen en común («Vacaciones playa», «Fotos agosto 2026»…). Se puede deshacer.
- **Pegar como archivo**: lo copiado se guarda como archivo en esa carpeta (captura → PNG, texto → TXT, enlace → acceso directo) con un nombre con la fecha, y queda listo para cambiarle el nombre. No abre ventanas.

## Lo que usa por detrás

Si falta algo que hace falta para una opción, la ventana lo dice y no deja usarla hasta que esté:

- **FFmpeg** (vídeo y audio): se descarga una vez, con un clic, de su build oficial (unos 87 MB; se comprueba su SHA-256). Si ya lo tienes instalado, se usa ese.
- **TurboSEND**: para «Enviar con TurboSEND». Si no está, la opción no sale en el menú y la ventana ofrece instalarlo con TurboSETUP.
- **Microsoft Office o LibreOffice**: para pasar documentos a PDF.
- **Cloudflare Tunnel** (`cloudflared`): para el QR; reutiliza el de TurboSEND o se descarga solo (comprueba que esté firmado por Cloudflare).
- **Clave de VirusTotal** (opcional, gratuita): se pega una vez en «¿Es seguro?».

## Rapidez

- La primera ventana tarda menos de un segundo. Al cerrarla, TurboMENU se queda preparado (oculto) 15 minutos y las siguientes se abren en unos 40 ms.
- En la ventana de inicio (abrir `TurboMENU.exe` sin nada) se puede activar «Abrir al instante», que lo deja preparado desde que inicias sesión.

## Instalar

Con [TurboSETUP](../installer/). En Windows 11 la entrada sale en «Mostrar más opciones» (o con Mayús + clic derecho).

También se puede abrir `TurboMENU.exe` suelto y pulsar «Añadir». Todo va en `HKCU` (no necesita administrador). Ajustes en `%APPDATA%\TurboMENU`; motores descargados y caché en `%LOCALAPPDATA%\TurboMENU`.

## Compilar

```powershell
.\build.ps1
```

| Archivo | Qué hace |
|---|---|
| `main.go`, `launch.go` | Arranque, reparto de la selección múltiple entre procesos y reutilización de la ventana preparada (tuberías con nombre) |
| `app.go`, `winapi.go` | Ventana: tamaño según el contenido, junto al cursor, foco |
| `menu.go`, `menuicons/` | Entradas del registro e iconos del menú |
| `jobs.go`, `zip.go`, `docsbridge.go` | Reducir y convertir: qué opciones enseñar y ejecutar con el motor que toque |
| `qr.go`, `web/`, `tunnel.go`, `upnp.go`, `direct.go`, `zipstream.go` | Pasar al móvil / recibir (red copiada de TurboSEND) |
| `send.go` | Habla con TurboSEND por su vía de control local |
| `safeui.go`, `lockui.go`, `folder.go`, `paste.go`, `home.go` | El resto de acciones |
| `internal/media` | FFmpeg: descarga, análisis, reducir y convertir |
| `internal/docs` | Imágenes, PDF y documentos de Office |
| `internal/safe` | Firma, Defender, VirusTotal, origen |
| `internal/lock` | Quién bloquea un archivo, liberar, papelera, expulsar |
| `internal/shellx` | Miniaturas e iconos del Explorador |
| `frontend/` | Interfaz (HTML, CSS y JS a mano) |
| `dev/icons/` | Genera los iconos (`node dev/icons/make-sheet.js`, Chrome sin ventana y `go run dev/icons/gen.go`) |

Pruebas con archivos reales: `$env:TURBOMENU_VIDEO = "C:\...\video.mp4"; go test -run VideoReal -v .`. Con `$env:TURBOMENU_TEST = "1"` la ventana se pinta detrás de todo y sin quitar el foco, y apunta los tiempos de apertura en `%TEMP%\TurboMENU-test.log`.
