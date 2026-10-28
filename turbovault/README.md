# TurboVAULT

Gestor de contraseñas que guarda todo **cifrado en tu PC**, con extensión para Chrome que rellena usuario y contraseña al iniciar sesión y te ofrece guardar las nuevas. Nada se sube a ninguna nube.

- **`TurboVAULT.exe`**: un único ejecutable con la extensión dentro. Se instala con [TurboSETUP](../installer/).
- Queda en la **bandeja** (junto a la flechita de la barra de tareas) para que la extensión funcione siempre. El icono es de colores con la bóveda abierta y gris con la bóveda cerrada.

## Lo que hace

- **Entradas**: inicios de sesión (varias webs por entrada, usuario, contraseña, clave 2FA, notas, campos extra como PIN o preguntas de seguridad) y **notas seguras**. Favoritas, búsqueda, orden por nombre, uso, cambios o fuerza.
- **Códigos 2FA** (TOTP, los de Google Authenticator): pegas la clave o el enlace `otpauth://` y TurboVAULT da el código con su cuenta atrás. La extensión lo rellena en el campo del código.
- **Historial**: las 12 contraseñas anteriores de cada entrada, por si cambias una y la web no la acepta.
- **Papelera**: lo borrado se queda 30 días.
- **Generador**: contraseñas aleatorias (8-64 caracteres, tipos a elegir, sin caracteres que se confunden), **frases de palabras** en español (más fáciles de teclear) y PIN. Usa el generador criptográfico de Windows.
- **Medidor de fuerza** de verdad: no se fía de «tiene mayúscula y símbolo». Detecta contraseñas famosas, palabras, nombres, secuencias (`abc`, `123`), recorridos de teclado (`qwerty`), repeticiones, años y fechas, y calcula cuánto tardaría un atacante.
- **Salud**: puntuación de 0 a 100 y listas de contraseñas débiles, repetidas en varias webs, sin cambiar desde hace más de un año y de webs sin https. Con un botón, comprueba si alguna aparece en **filtraciones conocidas** (Have I Been Pwned): de cada contraseña solo se envían los 5 primeros caracteres de su huella SHA-1 (de 40), así que ni la contraseña ni su huella salen del PC.
- **Actividad**: qué se ha hecho y desde dónde (abrir, cerrar, rellenar en qué web y desde qué navegador, copiar, importar, exportar, intentos fallidos…). Va cifrada dentro de la bóveda.
- **Importar** desde Chrome, Edge, Firefox, Bitwarden, KeePass/KeePassXC, 1Password, LastPass o Proton Pass (su CSV). Salta las que ya tienes y, si quieres, **borra el CSV** al terminar (lleva tus contraseñas sin cifrar). Después explica cómo quitarlas de Chrome.
- **Exportar** a CSV sin cifrar, solo pidiendo la contraseña maestra y con aviso.

## Copias de seguridad

- El archivo de la bóveda ya va cifrado: **se puede copiar a cualquier sitio** y sin la contraseña maestra no sirve de nada.
- **Copias automáticas**: eliges carpetas (mejor en un disco externo o un USB) y cada vez que cambias algo se guarda una copia nueva con la fecha, quedándose las N últimas. Si el disco no está conectado, la copia se hace en cuanto lo conectes. Los discos se reconocen por su número de serie, así que da igual que Windows le cambie la letra al USB. Cada copia se vuelve a leer para comprobar que se escribió bien.
- **Copia puntual** en la carpeta que elijas.
- **Restaurar**: abres un `.tvault` con *su* contraseña maestra (la que tenías cuando se hizo); la bóveda que había se guarda al lado, no se borra. Así se recupera todo en otro PC: instalas TurboVAULT y restauras la copia.

## La extensión de Chrome

- Cada campo de usuario o contraseña lleva el icono de TurboVAULT dentro, sin tener que pulsar nada: **de color** con la bóveda abierta y **gris** con la bóveda cerrada. Un clic en el icono (o entrar en el campo) abre el menú con lo guardado **para esa web**; otro clic y rellena. También con **Ctrl+Mayús+L**, o con clic derecho → «Rellenar con TurboVAULT».
- Mientras escribes en un campo de usuario, Chrome no saca sus sugerencias de lo que escribiste otras veces, para que no tapen el menú (al salir del campo queda como estaba). El menú va por encima de la página, también en las ventanas de inicio de sesión que abren algunas webs. Si algo de la página tapa un campo (un aviso, un menú), su icono no se ve.
- En formularios de **registro** o de **cambio de contraseña** ofrece una contraseña segura y la pone en los dos campos.
- Al enviar un formulario ofrece **guardar** la contraseña (o **actualizarla** si ha cambiado), también en la página siguiente si la web cambia de página al entrar. «Nunca aquí» deja de preguntarlo en esa web.
- El menú del icono enseña lo guardado para la web abierta (rellenar, copiar usuario, contraseña o código 2FA), busca en toda la bóveda y genera contraseñas.
- Funciona también en Edge, Brave y Vivaldi (el asistente abre la página de extensiones del navegador que uses, con sus pasos).
- En una web **sin https** (salvo las de tu propio PC) avisa antes de rellenar: lo que se rellena ahí viaja sin cifrar.
- Nunca rellena campos escondidos: fuera de la página, transparentes o diminutos.
- **Se actualiza sola**: cuando TurboVAULT trae una versión nueva de la extensión, ella misma se recarga y se pone en las pestañas abiertas, sin pasar por `chrome://extensions`.
- Si TurboVAULT no contesta, la extensión espera cada vez más entre intentos (hasta un minuto) en lugar de reintentar sin parar.

## Seguridad

**Cifrado**

- De la contraseña maestra sale una clave con **Argon2id** (256 MB de memoria y 6 pasadas por intento): probar contraseñas a lo bruto cuesta lo mismo en un ordenador potente o en tarjetas gráficas. La contraseña maestra no se guarda en ningún sitio.
- Los datos van cifrados con una clave aleatoria de 256 bits y **AES-256-GCM**, que además detecta cualquier cambio en el archivo. La parte secreta de cada entrada va **cifrada otra vez por separado**: con la bóveda abierta, en memoria solo hay títulos, webs y usuarios, y cada contraseña se descifra en el momento en que se pide.
- La clave vive en memoria bloqueada (no va al archivo de paginación) y **cifrada por Windows** mientras no se usa; un volcado de memoria o el archivo de hibernación no la enseñan.
- **Cambiar la contraseña maestra** cambia también la clave interna y vuelve a cifrar todo: una copia vieja del archivo no ayuda a leer los datos nuevos.
- La contraseña maestra debe tener al menos 12 caracteres y pasar el medidor como «Fuerte» o mejor. Al crearla, TurboVAULT puede sugerir una frase de cinco palabras al azar.

**Sesión**

- La bóveda se abre durante un **tiempo fijo** (10 minutos por defecto; de 1 a 60) que **no se alarga** al usarla. Después hay que volver a escribir la contraseña maestra.
- Se cierra también al **bloquear Windows** (Win+L), al **suspender** el PC y al cerrar sesión.
- Con la bóveda cerrada la ventana no deja ver nada de lo que había detrás: la pantalla de bloqueo es opaca y la lista se borra.
- Tras 3 intentos fallidos hay que esperar (5 s, 10 s, 20 s… hasta 15 minutos), aunque se cierre y se vuelva a abrir TurboVAULT. Cuenta todo lo que pide la contraseña maestra (abrir, exportar, cambiarla, restaurar), no solo abrir. Los intentos fallidos salen en la actividad.
- Si bloqueas Windows o suspendes el PC justo mientras se está abriendo, se queda cerrada.
- Si alguien pone en su sitio una versión más antigua del archivo de la bóveda (por ejemplo, una copia vieja), avisa al abrirla.

**Frente a otros programas del PC**

- La carpeta de datos (`%APPDATA%\TurboVAULT`) solo la puede abrir tu usuario de Windows.
- Los procesos de TurboVAULT cambian sus permisos para que **ningún otro programa de tu usuario pueda leer su memoria, inyectarles código ni desviar sus hilos** (los que lanza TurboVAULT nacen ya protegidos), y el agente solo acepta DLL firmadas por Microsoft. (Un administrador con privilegio de depuración sí podría.)
- La única ventana que puede ver la bóveda es la que **abre el propio agente**: al abrir TurboVAULT desde el menú Inicio, la bandeja o la extensión, se le pide al agente que la abra. Así ningún otro programa ha tenido nunca acceso a ella.
- Nadie puede cerrar desde fuera la parte del agente que vigila el bloqueo de Windows y la suspensión.
- La versión normal no hace caso de variables de entorno para cambiar su carpeta o su tubería, ni de las que cambian el navegador interno de la ventana. Arrastrar un enlace a la ventana no la lleva a otra página, y el navegador interno no recuerda lo que escribes.
- La ventana sale en negro en **capturas de pantalla**, grabaciones y al compartir pantalla (se puede desactivar).
- El portapapeles: lo que copias **no va al historial** de Windows (Win+V) ni a la nube, y se borra solo a los 20 s (configurable) si no has copiado otra cosa.

**Frente a webs y extensiones**

- **Nada se rellena solo**: siempre hace falta un clic o el atajo. Solo cuentan clics y teclas **de verdad**: una página no puede pulsar por ti, y si tapa, vuelve transparente o deforma el menú de TurboVAULT para engañarte (*clickjacking*), el clic no hace nada. El menú tampoco se abre solo si es la página la que pone el foco en el campo.
- Solo se ofrecen las entradas de **la web exacta** (con o sin `www.`) y del mismo puerto: lo guardado para `github.com` no vale en `gist.github.com`, ni en `github.com.otra-web.net`, ni en `g1thub.com`. Para que una entrada valga en todos los subdominios hay que pedirlo escribiendo la web como `*.google.com` (no vale sobre sufijos públicos como `*.github.io` o `*.co.uk`). Lo guardado con `https` no se da a una página `http`. Los `iframe` de otras webs no reciben nada.
- La extensión **nunca ve la bóveda entera** ni la contraseña maestra: pide a TurboVAULT la contraseña de una entrada concreta para la web en la que está, y TurboVAULT comprueba él mismo que esa entrada es de esa web.
- La contraseña maestra **nunca se escribe en el navegador**: si la bóveda está cerrada, la extensión abre una ventanita de TurboVAULT para escribirla allí.
- **Vinculación**: un navegador nuevo no recibe nada hasta que lo apruebas en TurboVAULT comprobando un código de 6 cifras que enseñan los dos. La aprobación se guarda cifrada dentro de la bóveda, y se puede desvincular en Ajustes.
- TurboVAULT comprueba que quien le habla es **su propio programa** (el mismo archivo, no solo la misma ruta) lanzado por un navegador **firmado por su fabricante** (Google, Microsoft, Brave…), con el identificador fijo de su extensión. Además, el puente comprueba que **su entrada y su salida son del propio navegador**: un programa que lo lance haciéndose pasar por Chrome (falsificando el proceso padre) tendría él esas tuberías. Si algo de eso no cuadra, rechaza la conexión.
- Comprueba que **la extensión cargada en el navegador es exactamente la original** que lleva dentro: si alguien cambia uno de sus archivos, deja de darle contraseñas, lo apunta en la actividad y avisa para reinstalarla. Si no la encuentra en ningún perfil del navegador (y por tanto no puede comprobarla), tampoco.
- Una pestaña no puede preguntar «¿guardo esto?» más de 5 veces por minuto, y solo al enviar un formulario de verdad.
- Si el navegador pide **demasiadas contraseñas seguidas** (más de 15 en un minuto o 150 en una hora), TurboVAULT **se cierra** y avisa: es lo que haría una extensión maliciosa intentando sacarlas todas.

**Lo que no puede evitar** (ningún gestor de contraseñas local): un virus que se ejecute en tu PC con tu usuario puede leer lo que escribes, ver la pantalla o cambiar los programas instalados en tu carpeta de usuario (TurboSETUP instala sin pedir permisos de administrador). Las protecciones de arriba le cierran los caminos fáciles, pero no lo detienen del todo mientras la bóveda está abierta. Por eso la sesión dura poco, se cierra al bloquear el PC y la actividad registra todo. Tampoco protege las **cookies** de sesión del navegador: para eso, activa la verificación en dos pasos en tus cuentas importantes.

## Instalación y primer uso

1. Instálalo con TurboSETUP. Se registra el puente con el navegador, se deja la extensión en `%LOCALAPPDATA%\TurboVAULT\extension`, TurboVAULT arranca con Windows en la bandeja y aparece en el menú Inicio.
2. Chrome no deja que un programa instale extensiones por su cuenta: se abre `chrome://extensions` con la ruta ya copiada. Activa *Modo de desarrollador* → *Cargar descomprimida* → pega la ruta. Se hace una sola vez (también desde *Ajustes → Navegador → Preparar*).
3. Al abrir TurboVAULT por primera vez creas la contraseña maestra.
4. La primera vez que uses la extensión te pedirá **vincular** el navegador con un código.

Al desinstalar se quita todo menos la bóveda, que sigue en `%APPDATA%\TurboVAULT` (cifrada).

## Ajustes

Duración de la sesión (de 1 minuto a 8 horas; al cambiarla, la sesión abierta vuelve a contar desde cero con el tiempo nuevo, y un clic en el contador de arriba lleva a este ajuste), segundos hasta borrar el portapapeles, cuánto se ve lo que destapas con el ojo (10, 30 o 60 s), cerrar la bóveda al cerrar la ventana o al minimizarla, ocultar la ventana en capturas, aviso junto al reloj al rellenar, buscar filtraciones cada vez que se abre la bóveda, arrancar con Windows, cambiar la contraseña maestra, navegadores vinculados, webs donde no ofrecer guardar y la carpeta de datos. Se guardan en `%APPDATA%\TurboVAULT\ajustes.json` (nada secreto).

## Teclado

Ctrl+F buscar (Esc borra la búsqueda) · Ctrl+N nueva · Ctrl+L cerrar la bóveda · flechas para moverte · Ctrl+C copiar la contraseña · Ctrl+B copiar el usuario · Ctrl+Mayús+C copiar el código 2FA · Intro editar · Supr a la papelera · Ctrl+Intro guardar en el editor. En el navegador: Ctrl+Mayús+L.

## Cómo funciona por dentro

| Proceso | Qué hace |
|---|---|
| `TurboVAULT.exe --agent` | El agente de la bandeja. Es el único que abre la bóveda. Atiende por una tubería con nombre que solo puede abrir tu usuario. |
| `TurboVAULT.exe` | Pide al agente que abra la ventana (y lo arranca si no está). |
| `TurboVAULT.exe --window` | La ventana (Wails). Solo la abre el agente; le pide todo a él y al cerrarla no queda nada en memoria. |
| `TurboVAULT.exe chrome-extension://…` | Puente que lanza Chrome (Native Messaging, `com.turbovault.native`). Pasa los mensajes de la extensión al agente. |

Archivos: `%APPDATA%\TurboVAULT\boveda.tvault` (la bóveda; y `.bak`, la versión anterior por si el disco falla a mitad de guardar), `ajustes.json`, y en `%LOCALAPPDATA%\TurboVAULT` la extensión y un registro técnico sin datos personales.

## Compilar y probar

```powershell
.\build.ps1                        # go vet, pruebas y TurboVAULT.exe
go run dev/icons/genicons.go       # rehace los iconos a partir de los SVG
node dev/run-tests.js              # prueba completa con Chrome de verdad
```

`dev/run-tests.js` compila una versión de pruebas, registra el puente solo durante la prueba y abre un perfil temporal de Chrome, que es quien arranca TurboVAULT con una carpeta y una tubería propias (no toca tu bóveda ni mira los navegadores del PC). Comprueba: que la ventana se abre, vincular, los iconos en los campos (de color, grises, tapados, dentro de un diálogo), que un clic en el icono abre el menú, rellenar, que una web parecida no reciba nada, guardar al iniciar sesión, la cuenta atrás del menú de la extensión y que sin TurboVAULT la extensión no reintente sin parar. Con `--shots carpeta` guarda capturas de los iconos y el menú.

La interfaz se puede ver en un navegador normal con datos de muestra (`python -m http.server --directory frontend`); la contraseña maestra de muestra es `maestra-de-prueba`.
