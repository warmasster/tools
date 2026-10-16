# TurboDL · motor nativo (Go)

El motor nativo hace las descargas fuera de Chrome. Es un único ejecutable sin dependencias: `TurboDL.exe`, que además lleva la extensión dentro e instala ambas cosas con un doble clic. Al instalarse se copia como `%LOCALAPPDATA%\TurboDL\turbodl-native.exe`, que es el que arranca Chrome. La extensión lo usa automáticamente si está instalado. También funciona solo desde la terminal.

## Por qué es más rápido que el motor del navegador

Medido en local con un servidor que limita cada conexión a 2 MB/s: 16 conexiones nativas descargan 48 MB unas 9 veces más rápido que 1 conexión. La extensión con el motor del navegador, con 6 conexiones, va unas 6 veces más rápido.

En Internet depende del servidor:

- **Servidores que limitan por conexión** (la mayoría de webs de descargas): se nota muchísimo.
- **Servidores que ya llenan tu línea con una conexión:** el motor lo detecta y no abre más.
- **Servidores que limitan las conexiones por IP** (por ejemplo, el de pruebas de Hetzner deja 1 o 2): se queda en las que acepten, sin fallar.

| | Navegador | Nativo |
|---|---|---|
| Conexiones por descarga | 6 como máximo (límite de Chrome por servidor) | 16 por defecto, hasta 64 |
| Servidores HTTP/2 | Chrome mete todo en 1 conexión | Una conexión TCP propia por trabajador |
| Servidores con varias IPs (CDN) | Una IP | Reparte las conexiones entre todas |
| Disco | Temporal en OPFS + copia final (2x) | Escribe directamente en la carpeta final (archivo disperso, sin copia) |
| Redes con IPv6 roto | Lo que haga Chrome | Prefiere IPv4 si el servidor lo tiene |

Además, el motor nativo:

- **Arranca de forma gradual:** abre 4 conexiones y las duplica cada medio segundo mientras la velocidad suba más de un 10 %. Si una conexión ya llena tu línea, no abre más, y así no salta el antiabuso de los servidores que bloquean la IP cuando llegan 16 conexiones de golpe.
- **Reutiliza las conexiones:** pide el archivo en tramos de 8 a 256 MB (unos 30 s cada uno) y solo reparte lo que nadie ha pedido todavía. Así ninguna respuesta se corta a medias y cada conexión TCP sirve para muchos tramos (keep-alive). En las pruebas, 8 conexiones simultáneas abren exactamente 8 conexiones TCP.
- **Distingue los dos tipos de límite del servidor:**
  - **Límite de peticiones** (429 o 503 con `Retry-After`, como megadl, donde el `Retry-After` es la cuenta atrás hasta la ventana siguiente): no quita conexiones. Las que descargan siguen, nadie pide tramos nuevos hasta que pase la espera, y los tramos se hacen más grandes para gastar menos peticiones.
  - **Límite de conexiones** (403 o 509, o la misma conexión rechazada 3 veces seguidas): baja a las que acepta y, cada 30 s sin quejas, prueba con una más.
  - Si solo queda una conexión y recibe 429 o 503, espera y reintenta: no se rinde.
- **Reparte el trabajo:** cuando una conexión termina, parte por la mitad el trozo pendiente más grande. Al final, si no queda nada libre, parte el tramo en curso de otra conexión.
- **Sustituye las conexiones lentas:** la que va a menos del 20 % de la mediana se reconecta, si puede a otra IP. No lo hace con conexiones de menos de 10 s (todavía están arrancando) ni si el servidor está limitando (sería otra petición rechazada).
- **Reintenta los cortes:** cada trozo se reintenta por separado, con espera creciente, y la última conexión aguanta unos 2,5 minutos de cortes de red.
- **Reanuda de forma segura:** guarda el estado (`archivo.turbodl.json`) después de `fsync`, y al reanudar comprueba con If-Range que el archivo del servidor no ha cambiado.
- **No pierde viajes de ida y vuelta:** la primera petición ya es `Range: bytes=0-`, así que sirve de sondeo y empieza a descargar a la vez.

## Instalar

1. Compila con `build.ps1` (necesita [Go](https://go.dev)): genera `..\TurboDL.exe` con la extensión dentro, icono y versión.
2. **Doble clic en `TurboDL.exe`.** Se copia a `%LOCALAPPDATA%\TurboDL` y se registra para Chrome, Edge, Brave y Chromium, sin permisos de administrador.
3. Deja la extensión en `%LOCALAPPDATA%\TurboDL\extension`. Si aún no la tienes, abre `chrome://extensions` con la ruta copiada; si ya la tienes, te dice que la recargues.

Cuando está instalado, el popup muestra «Motor nativo (Go)».

Para comprobar o quitar la instalación:

```bash
TurboDL.exe status
```

```bash
TurboDL.exe uninstall
```

En el popup o en los ajustes del gestor puedes elegir el motor: **Nativo si está instalado** o **Solo el del navegador**. Ahí también se eligen las conexiones del nativo (1-64).

## Desde la terminal

```bash
TurboDL.exe get "https://servidor/archivo.zip"
```

| Opción | Qué hace |
|---|---|
| `-c 16` | Conexiones máximas (1-64) |
| `-o CARPETA` | Carpeta de destino (por defecto Descargas) |
| `-n NOMBRE` | Nombre del archivo |
| `-H "Cookie: a=b"` | Cabecera extra (se puede repetir) |
| `--referer URL` | Cabecera Referer |
| `--ua TEXTO` | User-Agent (por defecto `turbodl-native/1.0`; desde Chrome se usa el del navegador) |
| `--http2` | Permitir HTTP/2 (por defecto, una conexión TCP por trabajador) |
| `--no-spread` | No repartir las conexiones entre IPs |

`Ctrl+C` pausa y guarda el progreso. El mismo comando otra vez continúa donde lo dejó.

Muchas webs de descargas atan el enlace a tu sesión. En ese caso hay que pasar la cookie (`-H "Cookie: ..."`) o descargar desde Chrome, donde la extensión se la pasa sola al motor.

## Cómo habla con la extensión

Native Messaging (`com.turbodl.native`): mensajes JSON por stdin y stdout, precedidos de su longitud.

- **Órdenes:** `hello`, `start`, `pause`, `cancel`, `setConnections`, `open`, `show` y `stat`.
- **Eventos:** `info`, `progress`, `notice` y `status`.

La extensión mantiene el puerto abierto solo mientras hay descargas. Si Chrome se cierra, el motor pausa todo y guarda el estado; al volver a abrir Chrome, la extensión lo reanuda.

El log está en `%LOCALAPPDATA%\TurboDL\native.log`.

## Pruebas

```bash
go test ./...
```

Las pruebas levantan un servidor local con límite de velocidad por conexión, cortes aleatorios, cambio de ETag, servidor sin Range, límite de conexiones simultáneas y varias IPs (127.0.0.1-3). En todas se comprueba el SHA-256 del archivo final.

`node ../dev/run-tests.js` incluye además las pruebas `native-*` de extremo a extremo con la extensión.
