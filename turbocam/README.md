# TurboCAM

Usa el móvil (o varios) como **webcam del PC**, sin instalar nada en el móvil: escaneas un QR, se abre una página y ya está emitiendo. El PC lo ve como una cámara más, **«TurboCAM»**, en Discord, Meet, Zoom, Teams, OBS, Chrome o cualquier programa que use la webcam.

## Cómo se usa

1. Abre TurboCAM en el PC y escanea el código QR con la cámara del móvil.
2. En el móvil, pulsa **Empezar** y da permiso a la cámara.
3. En tu app de videollamadas, elige **TurboCAM** como cámara.

La imagen sale en directo en la ventana del PC, que también dice qué apps están usando la cámara («En uso por Discord, Chrome»).

Las apps buscan cámaras al arrancar: si una ya estaba abierta cuando se instaló TurboCAM (Discord, por ejemplo), hay que cerrarla del todo y volver a abrirla para que la vea.

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

La cámara del móvil se abre a la resolución justa para lo que se manda (a 640×480 si se mandan 360p, por ejemplo): moverla a 1080p sin necesidad gasta y en muchos móviles no pasa de 15 fps. Cada fotograma se coge tal cual sale de la cámara (`MediaStreamTrackProcessor`, si el navegador lo tiene), aunque la pantalla esté en modo negro.

**Si va a pocos fotogramas**, mira «Cámara» en la hoja del móvil (o la línea del móvil en el PC): son los que da la cámara. Con poca luz, los móviles alargan la exposición y bajan solos a 10-15 fps, y ni bajar la resolución ni la red lo arreglan; el móvil lo avisa con «Cámara a N fps: ¿poca luz?». Más luz (o la cámara trasera, que suele captar más) lo arregla.

**El vídeo va en H.264, comprimido por el chip de vídeo del móvil** (el mismo que usa para grabar). Casi no gasta CPU, así que el móvil no se calienta ni baja el ritmo al cabo de un rato, y a 720p y 30 fps necesita **unos 2 Mb/s, seis veces menos que mandar fotos JPEG** (unos 12 Mb/s). Si el navegador del móvil no sabe comprimir H.264 o el PC no puede decodificarlo, se usa JPEG como antes.

**Si se puede, el vídeo va directo al PC por WebRTC** (UDP, como en una videollamada), sin pasar por Cloudflare aunque la página se abriera por su enlace: menos retraso, se reenvía lo que se pierda y la tasa de bits se ajusta sola a lo que dé la red. La primera vez tarda un segundo en conectar; mientras tanto, o si la red no deja conexiones directas, el vídeo va por la conexión de siempre (también en H.264) y se nota igual. Si WebRTC se cae (el móvil pasa de wifi a datos, por ejemplo), en un instante sigue por la conexión de siempre y vuelve a probar solo. En la hoja del móvil y en la ventana del PC se ve por dónde va («WebRTC» o la conexión) y si la compresión es por hardware.

Por la conexión de siempre, el PC confirma cada fotograma nada más recibirlo y en camino solo va lo que cabe en el tiempo de ida y vuelta: si la red no da abasto o empieza a acumularse retraso (sube la latencia), baja la tasa de bits; si sobra, la sube poco a poco. Si la conexión se queda colgada sin cerrarse (pasa con los túneles), el móvil lo nota en unos segundos y se vuelve a conectar solo.

## En el PC

- **Escenas**: el móvil, una **región de la pantalla** (con o sin el ratón) o **imagen sobre imagen**: un móvil en una esquina sobre la pantalla, sobre otro móvil, etc. La esquina y el tamaño se eligen con un clic.
- **Varios móviles a la vez**: se elige cuál sale con «Poner en cámara». Los que no salen (en espera) mandan solo una vista previa pequeña a pocos fotogramas por segundo; la imagen de la cámara va a ritmo completo.
- **Foto** (`Ctrl+Alt+P`): se la pide al móvil a la resolución de su cámara, con el mismo encuadre, y la guarda en `Imágenes\TurboCAM`. Con **cuenta atrás** opcional (3, 5 o 10 s, en Ajustes), que se ve en grande sobre la imagen; otro clic en «Foto» o Esc la cancela.
- **Congelar** (`Ctrl+Alt+F`): la cámara se queda con la última imagen.
- **Tapar** (`Ctrl+Alt+T`): en vez de la imagen sale una tarjeta «Cámara tapada».
- Los atajos funcionan desde cualquier programa y se pueden cambiar en Ajustes.
- **Perfiles por app**: un perfil guarda la escena y los retoques. Si le asignas apps (`discord.exe`, `ms-teams.exe`…), se pone solo cuando esa app abre la cámara y, al cerrarla, todo vuelve a como estaba.
- **Modo ventana** (sin cámara virtual): la ventana se convierte en un visor siempre encima, para compartirlo en una llamada. Doble clic en la imagen para entrar o salir; lo de «siempre encima» se puede quitar en Ajustes.
- **Resolución automática**: manda la que pide la app que usa la cámara (hasta 1080p). También se puede fijar de 1080p a 360p, y a 15, 24 o 30 fotogramas por segundo. Además, cada móvil puede mandar menos (en «Imagen» → «Envío»): el PC lo escala.
- **Ahorro**: si ninguna app usa la cámara y la ventana no se ve, el móvil baja a 1 imagen por segundo.
- Al cerrar con la cámara en uso, pregunta antes (se puede minimizar, o marcar «No volver a preguntar»; se vuelve a activar en Ajustes).
- En «Conectar», un clic en el enlace lo copia y un clic en el QR lo enseña en grande, para escanearlo desde lejos.

Sin imagen, la cámara enseña una tarjeta que explica qué pasa: «TurboCAM está cerrado», «Esperando al móvil», «El móvil está en pausa» o «Cámara tapada».

## Conexión

Funciona desde cualquier sitio, no solo en la misma wifi. El QR se puede pedir de tres formas:

| | Cómo va | Cuándo usarlo |
|---|---|---|
| **Internet** | Enlace HTTPS de Cloudflare (un túnel que se abre solo) | Siempre funciona y sin avisos. Algo más de retraso. |
| **Directa** | HTTPS directo a tu IP pública; abre el puerto en el router por UPnP y lo cierra al salir | Fuera de casa y con menos retraso, si el router lo permite (no con CG-NAT). |
| **Wifi** | HTTPS a la IP del PC en la red local | Lo más rápido, con el móvil en la misma red. |

Sea cual sea el enlace, el vídeo intenta ir **directo por WebRTC** (ver «En el móvil»): por el mismo número de puerto, en UDP. El modo directo abre también ese puerto UDP en el router, así que llega directo incluso con el móvil en datos; sin UPnP, WebRTC suele abrirse paso igual (con la ayuda de los servidores STUN de Google y Cloudflare, que solo dicen a cada lado su dirección pública: el vídeo no pasa por ellos).

El navegador del móvil solo deja usar la cámara en páginas seguras (HTTPS). Por Cloudflare el certificado es de verdad; directo, TurboCAM usa uno propio, así que la primera vez el móvil avisa de que «la conexión no es privada» y hay que tocar *Configuración avanzada → Continuar*. En iPhone, mejor «Internet» (Safari no deja abrir el canal de vídeo con un certificado propio; aun así TurboCAM se pasa solo a un plan B por HTTP).

El enlace lleva un código secreto. «Cambiar» en la pestaña Conectar crea otro y desconecta a todos. Con un **túnel de Cloudflare con dominio propio** (Ajustes) el enlace no cambia nunca.

## Instalar

Con [TurboSETUP](../installer/). `TurboCAM.exe setup` registra la cámara y crea el acceso directo; `TurboCAM.exe uninstall` lo quita todo. Si se abre `TurboCAM.exe` suelto, también registra la cámara.

**No necesita administrador**: la cámara se registra en `HKCU`, en 64 y 32 bits (para que la vean también las apps de 32 bits). Ajustes y certificado en `%APPDATA%\TurboCAM`; la cámara, el navegador interno y `cloudflared` en `%LOCALAPPDATA%\TurboCAM` (si TurboSEND o TurboMENU ya tienen `cloudflared`, se usa ese).

## Cómo va por dentro

```
móvil (página web)  ──H.264 por WebRTC (o WebSocket)──▶  TurboCAM.exe  ──NV12 en memoria compartida──▶  cámara virtual (DLL)  ──▶  Discord, Meet…
```

- **Móvil** (`web/`): cada fotograma sale de la cámara como `VideoFrame` (`MediaStreamTrackProcessor`). Sin retoques, se recorta la parte que se ve sin copiar nada (`visibleRect`) y el codificador lo escala; con giro, espejo o filtros se pinta en un `OffscreenCanvas` (`paint.js`). Luego:
  - **WebRTC**: el fotograma va a un `MediaStreamTrackGenerator` (o a un lienzo con `captureStream` en Safari) que se manda por un `RTCPeerConnection` solo con H.264. La oferta, la respuesta y los candidatos ICE van por el WebSocket. El PC le dice a Chrome a qué tasa empezar (`x-google-start-bitrate`, la ya medida), para que no arranque a 320×180.
  - **WebSocket**: `VideoEncoder` (WebCodecs, por hardware si lo hay, `latencyMode: "realtime"`, Annex B) y cada fotograma va como `[3][número][clave][H.264]`. La tasa de bits sube y baja con `configure()` sin rehacer el codificador. Si el codificador se cuelga, se rehace.
  - **JPEG** (si no hay H.264): varios workers (`encoder.js`) lo pasan a JPEG a la vez. También para el plan B por HTTP (POST + EventSource), donde los envíos pueden llegar desordenados.
  - Un service worker guarda la página para que la app instalada abra sin conexión.
- **PC** (Go): un solo puerto atiende HTTPS (certificado propio) y, solo desde el propio PC, HTTP (lo usa `cloudflared`); el mismo número en UDP es para WebRTC ([pion](https://github.com/pion/webrtc), con un solo socket para todos los móviles; NACK, informes RTCP y transport-cc para que el móvil ajuste la tasa). El H.264 lo decodifica el decodificador de Media Foundation que trae Windows (`h264dec.go`, por COM sin librerías, en modo baja latencia: unos 2 ms por fotograma a 720p), que da NV12 directamente. Cada móvil tiene su hilo de decodificación: el H.264 se decodifica entero y en orden, y si falta algo (un salto en la numeración, un paquete perdido por WebRTC o un error) espera al siguiente clave y lo pide (PLI por WebRTC). Al pasar de una conexión a otra, cambia de flujo en un fotograma clave. El JPEG, con la librería estándar. Si hay que componer (pantalla, imagen sobre imagen), escala por planos en YUV sin pasar por RGB.
- **Memoria compartida** `Local\TurboCAM.Video.1` (formato en [`camera/protocol.h`](camera/protocol.h)): tres búferes NV12 hasta 1080p y un número de fotograma; la app escribe en el que nadie está leyendo. La cámara apunta ahí qué apps la usan y con qué formato, y la app lo usa para la resolución automática, los perfiles y el ahorro.
- **Cámara virtual** (`camera/turbocam.c`): un filtro de captura DirectShow en C, sin librerías (unos 45 KB, sin redistribuibles de Visual C++). Ofrece NV12, YUY2, I420 y RGB24 en 1080p, 720p, 540p, 480p y 360p; adapta cada fotograma al formato que haya pedido cada app (recortando para no deformar) y lo entrega a ritmo fijo con un temporizador de alta resolución. Las tarjetas sin imagen las dibuja con GDI. Va dentro del `.exe` y se saca a una carpeta por versión, así una actualización nunca choca con una app que tenga la anterior cargada.

## Compilar

```powershell
.\build.ps1
```

Compila la cámara (`camera\build.ps1`, con las herramientas de C++ de Visual Studio; en GitHub Actions ya están), pasa `go vet` y las pruebas, y deja `TurboCAM.exe` en esta carpeta.

Pruebas de verdad (registran la cámara en tu usuario y usan sus ajustes, que dejan como estaban). No corren con TurboCAM abierto: escribirían en la misma cámara que esté usando Discord.

```powershell
$env:TURBOCAM_REAL = "1"
go test -run Real -v .   # la cámara con ffmpeg, y la cadena entera con un Chrome haciendo de móvil (node dev/phone.mjs)
```

Solo la página del móvil (Chrome con cámara falsa → servidor → motor), con ajustes y memoria de prueba; se puede pasar con TurboCAM abierto. `$env:TURBOCAM_THROTTLE = "6"` hace la CPU del «móvil» 6 veces más lenta:

```powershell
$env:TURBOCAM_PHONE = "1"
go test -run PhonePage -v .
```

`$env:TURBOCAM_SHOTS = "carpeta"` guarda capturas de lo que ve `ffmpeg` y de la página del móvil (en `PhonePage`, también `pc.jpg`: lo que sale por la cámara). Usan el `ffmpeg` de TurboMENU si no hay otro.

Más opciones de `PhonePage`:

| Variable | Qué hace |
|---|---|
| `TURBOCAM_FAKEVIDEO=cam.mjpeg` | La cámara falsa pone ese vídeo. La de Chrome es demasiado simple: con grano de verdad JPEG pesa seis veces más. Uno con grano: `ffmpeg -f lavfi -i "testsrc2=size=1280x720:rate=30,noise=alls=10:allf=t+u" -t 6 -q:v 4 -f mjpeg cam.mjpeg` |
| `TURBOCAM_NET=80,3000` | 80 ms más de latencia y 3000 kb/s de subida (no afecta a WebRTC) |
| `TURBOCAM_JPEG=1` / `TURBOCAM_NORTC=1` | Como antes, con JPEG / H.264 sin WebRTC (en el enlace, `?jpeg` y `?nortc`) |
| `TURBOCAM_RTCDROP=1` | Corta WebRTC a mitad: el vídeo debe seguir por el WebSocket |
| `TURBOCAM_RETOUCH=1` | Con espejo y brillo (el fotograma pasa por el lienzo) |
| `TURBOCAM_CF=1` | Por un túnel de Cloudflare de verdad |
| `TURBOCAM_SECS=60` | Más rato (por defecto, 16 s) |

La interfaz con datos de muestra se puede abrir en un navegador: servidor `turbocam-ui` (y la página del móvil, sin PC, en `turbocam-phone`). Los iconos salen de `dev/genicons.py`.
