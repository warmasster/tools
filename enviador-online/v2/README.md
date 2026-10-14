# Enviador v2: chats

`EnviadorV2.exe` es independiente de la v1: usa su propio puerto (47831) y su propia carpeta de datos (`%APPDATA%\EnviadorV2`).

- **Cada enlace es un chat**, para una persona o un grupo. Quien entra pone su nombre y podéis escribiros y mandaros archivos en los dos sentidos.
- **Lo que tú envías:** igual que en la v1. Turbo en Chrome/Edge, carpetas como carpeta o como ZIP, y bloques verificados con CRC32.
- **Lo que te envían:**
  - Por defecto aparece en el chat y tú pulsas **Descargar**. La autodescarga se activa en Ajustes (con límite de tamaño opcional) o en cada chat desde el menú ⋯.
  - La subida va en bloques de 16 MB con CRC32, con varias conexiones a la vez, y se puede reanudar.
  - Se guarda en `Descargas\Enviador\<chat>\`, marcada como descargada de Internet.
  - Quien envía tiene que tener el chat abierto. Si recarga la página, se le pide volver a elegir los archivos y continúa donde se quedó.
- **Tiempo real por long-polling:** los túneles temporales de Cloudflare retienen SSE, así que se usa long-polling.
- **Dominio propio (opcional):** en Ajustes pones la dirección (`enviar.tudominio.com`) y el token del túnel. Dentro de la app están los pasos. Si falla o lo dejas vacío, se usa el enlace temporal.

Velocidad medida por Cloudflare (subiendo y bajando por la misma conexión): unos 40–60 MB/s de descarga y unos 8–20 MB/s de subida. Para subidas grandes conviene el enlace directo ⚡.

## Compilar

```powershell
.\build.ps1
```

| Archivo | Qué hace |
|---|---|
| `rooms.go` | Chats, mensajes, miembros y envíos entrantes (guardados en `rooms/<id>.json`) |
| `hub.go` | Eventos en tiempo real por long-polling, presencia y «escribiendo…» |
| `roomapi.go` | API pública del chat (unirse, mensajes, ofertas, subida de bloques) |
| `upload.go` | Aceptar, recibir, verificar y guardar lo que te envían |
| `app.go` | Estado y acciones de la ventana |
| `web/room.*`, `web/lib.js` | Página del chat en el navegador (turbo, subidas, CRC32) |
| `frontend/` | Ventana de escritorio |
