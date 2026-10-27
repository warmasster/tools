# TurboCAM

Usa el móvil (o varios) como **webcam del PC**, sin instalar nada en el móvil: escaneas un QR, se abre una página y ya está emitiendo. El PC lo ve como una cámara más, **«TurboCAM»**, en Discord, Meet, Zoom, Teams, OBS, Chrome o cualquier programa que use la webcam.

## Cómo se usa

1. Abre TurboCAM en el PC y escanea el código QR con la cámara del móvil.
2. En el móvil, pulsa **Empezar** y da permiso a la cámara.
3. En tu app de videollamadas, elige **TurboCAM** como cámara.

La imagen sale en directo en la ventana del PC, que también dice qué apps están usando la cámara («En uso por Discord, Chrome»).

> Es una cámara de DirectShow, como la de OBS: la ven Discord, Chrome, Edge, Meet, Zoom, Teams, OBS, Skype… Las pocas apps que solo usan Media Foundation (la app Cámara de Windows) no ven este tipo de cámaras; para esas está el **modo ventana**.

## En el móvil

- **Cambiar de cámara** (delantera, trasera o, si el móvil las tiene, cada lente), **zoom** (el óptico del móvil si lo deja, si no uno digital), **linterna**, **espejo**, **girar** y **encuadre** («Rellenar» o «Completo» con bandas).
- **Brillo, contraste y color**.
- Un recuadro marca lo que se ve en el PC.
- **Modo negro**: la pantalla se queda negra con un puntito de estado (verde emitiendo, azul en reposo, rojo sin conexión). Parece apagado y gasta poco, pero sigue emitiendo. Se vuelve con dos toques.
- **La pantalla no se bloquea** mientras emite. Un navegador no puede grabar con el móvil bloqueado de verdad: si lo bloqueas o cambias de app, se pausa (la cámara enseña «El móvil está en pausa») y sigue sola al volver.
- **Pantalla completa** y, por el enlace de Cloudflare, **instalar en la pantalla de inicio** como una app.
- Todo lo que cambias en el móvil se ve también en el PC (y al revés), y se recuerda para la próxima vez.

El móvil hace el trabajo caro: dibuja cada fotograma con los retoques al tamaño exacto que necesita el PC y lo manda en JPEG. La calidad se adapta sola a la red: si los fotogramas tardan, baja; si sobra, sube. Nunca hay más de dos fotogramas en camino, así que el retraso no se acumula.

## En el PC

- **Escenas**: el móvil, una **región de la pantalla** (con o sin el ratón) o **imagen sobre imagen**: un móvil en una esquina sobre la pantalla, sobre otro móvil, etc. La esquina y el tamaño se eligen con un clic.
- **Varios móviles a la vez**: se elige cuál sale con «Poner en cámara». Los que no salen mandan solo una miniatura.
- **Foto** (`Ctrl+Alt+P`): se la pide al móvil a la resolución de su cámara, con el mismo encuadre, y la guarda en `Imágenes\TurboCAM`.
- **Congelar** (`Ctrl+Alt+F`): la cámara se queda con la última imagen.
- **Tapar** (`Ctrl+Alt+T`): en vez de la imagen sale una tarjeta «Cámara tapada».
- Los atajos funcionan desde cualquier programa y se pueden cambiar en Ajustes.
- **Perfiles por app**: un perfil guarda la escena y los retoques. Si le asignas apps (`discord.exe`, `ms-teams.exe`…), se pone solo cuando esa app abre la cámara y, al cerrarla, todo vuelve a como estaba.
- **Modo ventana** (sin cámara virtual): la ventana se convierte en un visor siempre encima, para compartirlo en una llamada.
- **Resolución automática**: manda la que pide la app que usa la cámara (hasta 1080p). También se puede fijar a 1080p, 720p o 540p, y a 15, 24 o 30 fotogramas por segundo.
- **Ahorro**: si ninguna app usa la cámara y la ventana no se ve, el móvil baja a 1 imagen por segundo.
- Al cerrar con la cámara en uso, pregunta antes (se puede minimizar).

Sin imagen, la cámara enseña una tarjeta que explica qué pasa: «TurboCAM está cerrado», «Esperando al móvil», «El móvil está en pausa» o «Cámara tapada».

## Conexión

Funciona desde cualquier sitio, no solo en la misma wifi. El QR se puede pedir de tres formas:

| | Cómo va | Cuándo usarlo |
|---|---|---|
| **Internet** | Enlace HTTPS de Cloudflare (un túnel que se abre solo) | Siempre funciona y sin avisos. Algo más de retraso. |
| **Directa** | HTTPS directo a tu IP pública; abre el puerto en el router por UPnP y lo cierra al salir | Fuera de casa y con menos retraso, si el router lo permite (no con CG-NAT). |
| **Wifi** | HTTPS a la IP del PC en la red local | Lo más rápido, con el móvil en la misma red. |

El navegador del móvil solo deja usar la cámara en páginas seguras (HTTPS). Por Cloudflare el certificado es de verdad; directo, TurboCAM usa uno propio, así que la primera vez el móvil avisa de que «la conexión no es privada» y hay que tocar *Configuración avanzada → Continuar*. En iPhone, mejor «Internet» (Safari no deja abrir el canal de vídeo con un certificado propio; aun así TurboCAM se pasa solo a un plan B por HTTP).

El enlace lleva un código secreto. «Cambiar» en la pestaña Conectar crea otro y desconecta a todos. Con un **túnel de Cloudflare con dominio propio** (Ajustes) el enlace no cambia nunca.

## Instalar

Con [TurboSETUP](../installer/). `TurboCAM.exe setup` registra la cámara y crea el acceso directo; `TurboCAM.exe uninstall` lo quita todo. Si se abre `TurboCAM.exe` suelto, también registra la cámara.

**No necesita administrador**: la cámara se registra en `HKCU`, en 64 y 32 bits (para que la vean también las apps de 32 bits). Ajustes y certificado en `%APPDATA%\TurboCAM`; la cámara, el navegador interno y `cloudflared` en `%LOCALAPPDATA%\TurboCAM` (si TurboSEND o TurboMENU ya tienen `cloudflared`, se usa ese).

## Cómo va por dentro

```
móvil (página web)  ──JPEG por WebSocket──▶  TurboCAM.exe  ──NV12 en memoria compartida──▶  cámara virtual (DLL)  ──▶  Discord, Meet…
```

- **Móvil** (`web/`): pide la cámara, pinta cada fotograma en un lienzo con los retoques, al tamaño que pide el PC, y lo manda como JPEG con un número. El PC confirma cada uno; con dos sin confirmar, se salta fotogramas en vez de acumular retraso. Si el WebSocket no abre, usa POST + EventSource.
- **PC** (Go): un solo puerto atiende HTTPS (certificado propio) y, solo desde el propio PC, HTTP (lo usa `cloudflared`). Decodifica el JPEG con la librería estándar y, como ya llega al tamaño justo, solo lo copia como NV12 (unos 0,2 ms por fotograma a 720p). Si hay que componer (pantalla, imagen sobre imagen), escala por planos en YUV sin pasar por RGB.
- **Memoria compartida** `Local\TurboCAM.Video.1` (formato en [`camera/protocol.h`](camera/protocol.h)): tres búferes NV12 hasta 1080p y un número de fotograma; la app escribe en el que nadie está leyendo. La cámara apunta ahí qué apps la usan y con qué formato, y la app lo usa para la resolución automática, los perfiles y el ahorro.
- **Cámara virtual** (`camera/turbocam.c`): un filtro de captura DirectShow en C, sin librerías (unos 45 KB, sin redistribuibles de Visual C++). Ofrece NV12, YUY2, I420 y RGB24 en 1080p, 720p, 540p, 480p y 360p; adapta cada fotograma al formato que haya pedido cada app (recortando para no deformar) y lo entrega a ritmo fijo con un temporizador de alta resolución. Las tarjetas sin imagen las dibuja con GDI. Va dentro del `.exe` y se saca a una carpeta por versión, así una actualización nunca choca con una app que tenga la anterior cargada.

## Compilar

```powershell
.\build.ps1
```

Compila la cámara (`camera\build.ps1`, con las herramientas de C++ de Visual Studio; en GitHub Actions ya están), pasa `go vet` y las pruebas, y deja `TurboCAM.exe` en esta carpeta.

Pruebas de verdad (registran la cámara en tu usuario):

```powershell
$env:TURBOCAM_REAL = "1"
go test -run Real -v .   # la cámara con ffmpeg, y la cadena entera con un Chrome haciendo de móvil (node dev/phone.mjs)
```

`$env:TURBOCAM_SHOTS = "carpeta"` guarda capturas de lo que ve `ffmpeg` y de la página del móvil. Usan el `ffmpeg` de TurboMENU si no hay otro.

La interfaz con datos de muestra se puede abrir en un navegador: servidor `turbocam-ui` (y la página del móvil, sin PC, en `turbocam-phone`). Los iconos salen de `dev/genicons.py`.
