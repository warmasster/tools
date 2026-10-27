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
- **Envío**: resolución (automática, o de 1080p a 240p) y fotogramas por segundo de cada móvil. Menos es menos datos y menos batería.
- **Pantalla completa** y, por el enlace de Cloudflare, **instalar en la pantalla de inicio** como una app. La app abre aunque el PC esté apagado.
- **Escanear otro código**: para pasar a otro PC o al enlace nuevo del mismo (el de Cloudflare cambia cada vez que se abre TurboCAM). Si el móvil no encuentra el PC, dice si es que no responde o que el enlace ha cambiado, y ofrece escanear el nuevo. Sin lector de códigos en el navegador, se pega el enlace.
- **Olvidar este PC**: el móvil se desconecta, deja de entrar solo y el PC borra sus ajustes.
- Todo lo que cambias en el móvil se ve también en el PC (y al revés), y se recuerda para la próxima vez, aunque entre por otro enlace (se reconoce por el modelo).

El móvil hace el trabajo caro y lo reparte: la GPU recorta y escala solo la parte que se ve, y 2 o 3 hilos a la vez (uno por núcleo libre) le ponen los retoques y lo pasan a JPEG, así que comprimir no limita los fotogramas por segundo. El PC confirma cada fotograma nada más recibirlo (lo decodifica aparte, siempre el más nuevo), y en camino solo hay los que caben en el tiempo de ida y vuelta: no se frena por la latencia de internet ni se acumula retraso. Se adapta solo: si la red no da abasto, baja la calidad; si es el móvil el que no da abasto, baja la resolución (en automático); si sobra, vuelve a subir. Por Cloudflare, 720p a 30 fotogramas por segundo.

## En el PC

- **Escenas**: el móvil, una **región de la pantalla** (con o sin el ratón) o **imagen sobre imagen**: un móvil en una esquina sobre la pantalla, sobre otro móvil, etc. La esquina y el tamaño se eligen con un clic.
- **Varios móviles a la vez**: se elige cuál sale con «Poner en cámara». Los que no salen mandan solo una miniatura.
- **Foto** (`Ctrl+Alt+P`): se la pide al móvil a la resolución de su cámara, con el mismo encuadre, y la guarda en `Imágenes\TurboCAM`.
- **Congelar** (`Ctrl+Alt+F`): la cámara se queda con la última imagen.
- **Tapar** (`Ctrl+Alt+T`): en vez de la imagen sale una tarjeta «Cámara tapada».
- Los atajos funcionan desde cualquier programa y se pueden cambiar en Ajustes.
- **Perfiles por app**: un perfil guarda la escena y los retoques. Si le asignas apps (`discord.exe`, `ms-teams.exe`…), se pone solo cuando esa app abre la cámara y, al cerrarla, todo vuelve a como estaba.
- **Modo ventana** (sin cámara virtual): la ventana se convierte en un visor siempre encima, para compartirlo en una llamada.
- **Resolución automática**: manda la que pide la app que usa la cámara (hasta 1080p). También se puede fijar de 1080p a 360p, y a 15, 24 o 30 fotogramas por segundo. Además, cada móvil puede mandar menos (en «Imagen» → «Envío»): el PC lo escala.
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

- **Móvil** (`web/`): `createImageBitmap` recorta y escala en la GPU la parte del vídeo que se ve; varios workers (`encoder.js`) la pintan en un `OffscreenCanvas` con los retoques (`paint.js`, que también usa la página para las fotos y los navegadores sin workers) y la pasan a JPEG. Se manda con un número; el PC la confirma al recibirla y, si llega una más vieja que la última, la descarta. En camino van las que caben en el tiempo de ida y vuelta (de 2 a 6). Si el WebSocket no abre, usa POST + EventSource. Un service worker guarda la página para que la app instalada abra sin conexión.
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
