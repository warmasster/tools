# TurboSEND

Envía y recibe archivos y carpetas de cualquier tamaño desde tu PC, en chats. Quien está al otro lado solo abre un enlace en el navegador: no instala nada.

- **Cada enlace es un chat**, para una persona o un grupo. Quien entra pone su nombre y podéis escribiros y mandaros archivos en los dos sentidos.
- **Tres formas de llegar a tu PC**:
  - **⚡ Directo** (`http://TU-IP:47831/...`): abre el puerto en el router por UPnP y lo cierra al salir. Es lo más rápido, pero no funciona si tu operadora usa CG-NAT.
  - **☁️ Cloudflare** (`https://xxxx.trycloudflare.com/...`): va por HTTPS y funciona siempre, sin abrir puertos. La dirección cambia cada vez que abres la app, salvo que pongas un dominio propio.
  - **🏠 Red local**: para quien esté en tu misma red.
- **Lo que tú envías**:
  - En Chrome/Edge va en **modo turbo**: varias conexiones a la vez y cada bloque verificado con CRC32. Si se corta, reintenta; si falla del todo, se puede reanudar.
  - Las carpetas se guardan con su estructura (en turbo) o se descargan como un ZIP generado al vuelo.
- **Lo que te envían**:
  - Aparece en el chat y tú pulsas **Descargar**. La autodescarga se activa en Ajustes (con límite de tamaño opcional) o en cada chat desde el menú ⋯.
  - La subida va en bloques de 16 MB con CRC32, con varias conexiones a la vez, y se puede reanudar.
  - Se guarda en `Descargas\TurboSEND\<chat>\`, marcada como descargada de Internet.
  - Quien envía tiene que tener el chat abierto. Si recarga la página, se le pide volver a elegir los archivos y continúa donde se quedó.
- **Dominio propio (opcional)**: en Ajustes pones la dirección (`enviar.tudominio.com`) y el token del túnel. Dentro de la app están los pasos. Si falla o lo dejas vacío, se usa el enlace temporal.
- **Sin ventanas de terminal**: `cloudflared` se ejecuta oculto y se cierra con la app. Si no lo tienes, TurboSEND lo descarga de GitHub (el oficial; comprueba que la firma sea de Cloudflare).

Velocidad medida por Cloudflare: unos 40–60 MB/s de bajada y unos 8–20 MB/s de subida. Para subidas grandes conviene el enlace directo ⚡.

## Instalar

Con [TurboSETUP](../installer/), o descargando `TurboSEND.exe` de la [Release](https://github.com/warmasster/tools/releases/tag/apps): es un único ejecutable y no necesita instalar nada.

La primera vez, el firewall de Windows pedirá permiso: pulsa *Permitir*, hace falta para el enlace directo.

Los chats y ajustes se guardan en `%APPDATA%\EnviadorV2` (nombre heredado de cuando la app se llamaba Enviador).

## Compilar

Necesita [Go](https://go.dev) y [Wails v2](https://wails.io):

```powershell
.\build.ps1
```

| Archivo | Qué hace |
|---|---|
| `main.go`, `app.go` | Ventana (Wails), estado y acciones de la interfaz |
| `rooms.go` | Chats, mensajes, miembros y envíos entrantes (en `rooms/<id>.json`) |
| `hub.go` | Tiempo real por long-polling (Cloudflare retiene SSE), presencia y «escribiendo…» |
| `roomapi.go` | API pública del chat: unirse, mensajes, ofertas, subida de bloques |
| `upload.go` | Aceptar, recibir, verificar y guardar lo que te envían |
| `server.go` | Servidor público: páginas, descargas con Range, turbo y ZIP |
| `zipstream.go` | ZIP sin compresión al vuelo, con el tamaño exacto conocido de antemano (ZIP64) |
| `tunnel.go` | `cloudflared` (túnel temporal o con dominio propio) |
| `upnp.go`, `direct.go` | UPnP, detección de CG-NAT y enlace directo |
| `transfers.go`, `shares.go` | Seguimiento de envíos y velocidades |
| `frontend/` | Ventana de escritorio |
| `web/` | Página del chat en el navegador (turbo, subidas, CRC32) |
