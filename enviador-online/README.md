# Enviador

Envía archivos y carpetas de cualquier tamaño desde tu PC. Quien los recibe solo abre un enlace en el navegador.

- **`Enviador.exe`**: un único ejecutable (~13 MB). No necesita instalar nada y se puede pasar tal cual a otras personas.
- **Tres enlaces por envío**:
  - **⚡ Directo**: `http://TU-IP:47821/...`. Abre el puerto en el router por UPnP y lo cierra al salir. Es el más rápido, pero no funciona si la operadora usa CG-NAT.
  - **☁️ Cloudflare**: `https://xxxx.trycloudflare.com/...`. Va por HTTPS y funciona siempre, sin abrir puertos. La dirección cambia cada vez que se abre la app.
  - **🏠 Red local**: para quien esté en tu misma red.
- **Página de descarga** (en el navegador de quien recibe):
  - **Turbo en Chrome/Edge**: varias conexiones a la vez y cada bloque de 8 MB verificado con CRC32. Si se corta, reintenta; si falla del todo, se puede reanudar.
  - **Carpetas**: se guardan con su estructura (en modo turbo) o se descargan como un ZIP generado al vuelo.
  - **Otros navegadores**: descarga normal, que se puede reanudar.
- **Sin ventanas de terminal**: `cloudflared` se ejecuta oculto y se cierra con la app.

## Uso

- **Crear un envío**:
  - arrastra archivos o carpetas a la ventana, o usa los botones;
  - o arrastra archivos sobre el icono de `Enviador.exe`.
- **Compartir**: «Copiar mensaje para enviar» copia un texto con los enlaces, listo para pegar en WhatsApp o Discord.
- **Primera vez en otro PC**:
  - Windows SmartScreen avisará porque el ejecutable no está firmado: *Más información → Ejecutar de todas formas*.
  - El firewall pedirá permiso: pulsa *Permitir*, es necesario para el enlace directo.
  - Si ese PC no tiene `cloudflared`, Enviador lo descarga de GitHub (oficial, se verifica que la firma sea de Cloudflare).

Los datos (envíos y ajustes) se guardan en `%APPDATA%\Enviador`.

## Compilar

Necesita Go (instalado en `%LOCALAPPDATA%\Programs\go`) y Wails v2:

```powershell
.\build.ps1
```

Estructura:

| Archivo | Qué hace |
|---|---|
| `main.go`, `app.go` | Ventana (Wails), estado y acciones de la interfaz |
| `server.go` | Servidor público: página, descargas con Range, turbo (`/c`, `/m`) y ZIP (`/z`) |
| `zipstream.go` | ZIP sin compresión al vuelo, con tamaño exacto conocido de antemano (ZIP64) |
| `tunnel.go` | Gestión de `cloudflared` (quick tunnel) y comprobación de que el enlace responde |
| `upnp.go`, `direct.go` | UPnP IGD, detección de CG-NAT y enlace directo |
| `transfers.go` | Seguimiento de descargas y velocidades |
| `frontend/` | Interfaz de la app de escritorio |
| `web/` | Página que ve quien descarga |
