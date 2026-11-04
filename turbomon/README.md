# TurboMON 🧪

> 🧪 **Experimental.** Funciona, pero algunas herramientas son nuevas (el Laboratorio, el ahorro inteligente, la precarga y el mapa de ruido) y pueden cambiar. Si algo no va como esperas, cuéntalo.

FPS, sensores y overlay para jugar, el historial de tus partidas, una revisión de que tu PC está configurado para rendir lo que debe y herramientas que te dicen **por qué** te va mal un juego (tirones, cierres, lag) en lenguaje normal. Se adapta a tu PC: solo enseña lo que aplica a tus piezas. En segundo plano va al 0 % de CPU y ocupa unos 25 MB.

## Qué hace

- **Overlay en el juego**: FPS, 1 % bajo, CPU, gráfica (uso y temperatura), RAM, VRAM y ping. Se pone y se quita con **un clic en el icono de la bandeja** o con **Ctrl+Alt+O**. Hay un modo «solo FPS» y otro que solo lo enseña con un juego delante.
- **Inicio**: la nota de tu PC (de 0 a 100), el interruptor del overlay, cuatro cifras grandes, lo que hay que arreglar y, si «Solucionar» ha encontrado algo grave, un aviso que te lleva directo.
- **Mi PC**: el modelo exacto de cada pieza (procesador, placa y BIOS, cada módulo de RAM, gráfica con su fabricante y su enlace PCIe, cada disco con su salud y temperatura, pantallas, red, periféricos y batería), una prueba de velocidad de cada disco y otra de los hercios del ratón. En directo: uso de cada hilo, frecuencias, temperaturas, VRAM, ventilador, memoria, disco y red.
- **Revisión** (más de 20 comprobaciones, solo las que tocan en tu PC), cada una con qué hay, por qué importa y cómo se arregla: RAM (XMP/EXPO, doble canal), pantallas a sus hercios máximos y conectadas a la gráfica, driver de la gráfica, enlace PCIe y Resizable BAR, microcódigo de los Intel de 13.ª y 14.ª generación, plan de energía, ajustes de Windows para jugar, discos y red. Lo que se puede arreglar sin riesgo tiene botón.
- **Juegos**: una ficha por juego con sus partidas (FPS medios, 1 % y 0,1 % bajos, tirones, CPU, gráfica, RAM, temperaturas y ping, con la gráfica de toda la partida), cómo evoluciona partida a partida y si **un driver o una actualización de Windows te ha bajado los FPS**. Ahí mismo, los ajustes de ese juego, que se ponen solos al abrirlo.

### Solucionar

Una tarjeta por problema. TurboMON lo vigila de fondo y te dice la causa:

- **Se me cierra el juego**: lee los registros de Windows (cierres, cuelgues de la gráfica, pantallazos azules, reinicios sin aviso, errores de hardware) y te dice qué pasó y qué hacer.
- **Me va a tirones**: apunta la hora exacta de cada tirón de la partida y quién estaba haciendo algo justo en ese segundo (otro programa, el disco, un driver, el procesador frenándose, un vídeo en otra ventana). Si los tirones van con reloj («cada 60 s exactos»), lo detecta: es la firma de una tarea de fondo. Incluye el **mapa de ruido de tus núcleos**: mide cuánto tiempo le quitan al juego los drivers y Windows en cada hilo y, con permiso de administrador, **qué driver es** y cuánto tarda en el peor caso.
- **Tirones con dos pantallas** (solo si tienes dos): compara tus tirones con un vídeo puesto en la otra pantalla y sin él, y te dice cómo quitarlos.
- **¿Es estable mi PC?**: busca núcleos que se equivocan y errores en la RAM (overclock o undervolt justos, XMP inestable, procesadores que se degradan).
- **Me saca del juego**: quién te minimiza el juego (ventanas que roban el foco) y, si arranca con Windows, botón para quitarlo.
- **Ratón y ventana del juego**: ratón encerrado en la pantalla del juego y «ventana sin bordes» forzada para juegos que no la tienen.
- **Me va con lag**: el ping de verdad al servidor del juego, qué programa usa internet mientras juegas, la prueba de si tu conexión aguanta una descarga («bufferbloat») y, por wifi, apagar la búsqueda de redes durante la partida.

### Optimizar

- **Laboratorio**: ¿ese ajuste de los foros te da FPS de verdad? TurboMON lo enciende y lo apaga **al azar cada 20 segundos mientras juegas, sin decirte cuándo**, y compara. En unos 11 minutos sabes si mejora, empeora o da igual, con estadística que no se deja engañar por los cambios de escena. Prueba solo lo que tiene sentido en tu PC: solo núcleos de rendimiento, un hilo por núcleo, el primer chip de los Ryzen con dos, lejos del núcleo 0, prioridad alta y prioridad alta en la gráfica. Lo que gane se puede dejar puesto en ese juego.
- **Ahorro inteligente** (experimental, apagado de serie): si tus FPS van limitados o la gráfica va al 100 %, baja poco a poco la frecuencia máxima del procesador mientras los fotogramas sigan saliendo a tiempo, y la sube al instante si hace falta. Menos calor, ruido y consumo con los mismos FPS. Trabaja sobre una copia de tu plan de energía que se borra al acabar.
- **Modo juego Turbo**: durante la partida, el plan de energía de más rendimiento, prioridad para el juego y, si se acaba la memoria libre, soltar la memoria en espera. Al cerrar el juego lo deja todo como estaba.
- **Liberar la memoria en espera** a mano y **lo que más gasta ahora**, con botón para cerrarlo.
- **Precarga aprendida** (en la ficha de cada juego, solo si está en un disco duro o SSD SATA): aprende qué trozos de qué archivos lee el juego al cargar y, la próxima vez, se los adelanta con la prioridad de disco más baja. En un NVMe no hace falta y no se ofrece.
- Lo que *no* hace a propósito: «limpiar la RAM» vaciando la memoria de los programas, que solo provoca tirones.

### De dónde salen las ideas

Varias herramientas aplican, por primera vez en un programa para jugar, técnicas de estudios de investigación: el mapa de ruido es la prueba FTQ con la que se mide el «ruido del sistema operativo» en superordenadores; el Laboratorio usa experimentos «switchback» (como los de las empresas con sus servidores) con una prueba de rangos con signo; el ahorro inteligente sigue la idea de PEGASUS y Rubik (no ir al máximo si se va a esperar igual); la precarga es la predictiva de Kroeger y Long, aprendida juego a juego; y la prueba de estabilidad busca los «núcleos que se equivocan» de los que avisaron Google y Meta.

## Permiso de administrador

Windows solo deja ver los fotogramas, el tráfico de red por programa, los archivos que lee un juego y el tiempo de cada driver a un programa con permiso de administrador (igual que a MSI Afterburner, PresentMon o LatencyMon). TurboMON lo pide **una vez** al instalarlo y crea una tarea que lo arranca así al iniciar sesión. Si dices que no, funciona todo menos eso, y se puede activar luego.

No se mete en el juego: escucha los avisos que Windows ya da (DXGI, Direct3D 9 y, para Vulkan y OpenGL, el núcleo gráfico). Por eso no da problemas con los antitrampas. Los ajustes del Laboratorio (núcleos y prioridad) se le ponen al proceso desde fuera; si un antitrampas no lo deja, TurboMON lo dice. El overlay es una ventana por encima de todo: se ve en juegos **en ventana o en pantalla completa sin bordes**, pero no en «pantalla completa exclusiva».

La **temperatura del procesador** no la da Windows sin un driver especial. Si tienes HWiNFO abierto con «Shared Memory Support», TurboMON la lee de ahí (y los vatios, para el ahorro inteligente).

## Lo que ocupa y gasta

- El agente de la bandeja lee los sensores **una vez por segundo solo si alguien mira** (overlay, ventana o una partida). Si no, lee procesador y memoria cada 5 s. Durante una partida, la lectura completa y el análisis de tirones suman unos 7 ms de CPU por segundo.
- El tráfico por programa y el ping solo se miran durante la partida o con «Me va con lag» abierto. Los archivos del juego, solo los primeros minutos (y solo para la precarga). El mapa de ruido y las pruebas, solo cuando los pides.
- No usa la librería de NVIDIA (NVML), que ocupa 20 MB solo con cargarla.
- Cada partida guardada ocupa entre 3 y 20 KB.

## Para desarrollar

Un solo `TurboMON.exe` con varios papeles:

| Orden | Qué es |
|---|---|
| `TurboMON.exe` | La ventana (Wails). Arranca el agente si no está. |
| `TurboMON.exe --agent` | El agente: bandeja, overlay (Win32 + GDI), atajo, sensores, eventos de Windows (ETW), partidas, laboratorio y ahorro. Atiende a la ventana por HTTP en `127.0.0.1` con un token que guarda en `agente.json`. |
| `TurboMON.exe --noise-probe ms hilos` | La medida del mapa de ruido (la lanza el agente, con `GODEBUG=asyncpreemptoff=1`). |
| `setup` / `uninstall` | Lo que usa TurboSETUP. `setup` pide permiso (`setup-admin`) para crear la tarea programada `TurboMON (<usuario>)`; si no se da, usa la clave `Run` del usuario. |

Carpetas de `internal/`:

- `sensors`: CPU, memoria, PDH, gráficas (DXGI y D3DKMT), tiempo en DPC por núcleo, qué proceso dibuja en qué gráfica y quién decodifica vídeo; HWiNFO opcional.
- `etw`: sesiones ETW en tiempo real con varios proveedores y registrador del núcleo.
- `fps`: fotogramas por proceso (FPS, 1 % bajo, histograma de la partida).
- `parts`: el modelo de cada pieza (SMBIOS, SetupAPI, IOCTL de discos, EDID, wlanapi…).
- `checks`: las comprobaciones y sus arreglos.
- `events`: los registros de Windows para «Se me cierra el juego».
- `stability`: la prueba de núcleos y RAM.
- `netmon`: tráfico por programa (Kernel-Network), servidor del juego, ping y bufferbloat.
- `procstat`: lo que gasta cada proceso en un segundo (una sola llamada a Windows).
- `stutter`: el análisis de tirones (culpables y ritmo).
- `noise`: el mapa de ruido (bucle en ensamblador, DPC e interrupciones por driver).
- `lab`: los experimentos A/B y su estadística.
- `governor`: el controlador del ahorro inteligente.
- `prefetch`: aprender y adelantar las lecturas de cada juego (Kernel-File y Kernel-Process).

Datos en `%LOCALAPPDATA%\TurboMON`: `ajustes.json`, `partidas.jsonl`, `juegos.json` (ajustes por juego), `laboratorio.json`, `ruido.json`, `estabilidad.json`, `robos-de-foco.jsonl`, `precarga\`, `turbomon.log` y `agente.json`. Si TurboMON se cierra en mitad de una partida, al volver a arrancar deshace lo que tuviera puesto (plan de energía, copia del ahorro, búsqueda de wifi). `TURBOMON_DIR` cambia la carpeta (pruebas).

Pruebas:

```powershell
.\build.ps1                                            # vet, pruebas y TurboMON.exe
go test -run TestRun -v ./internal/checks/             # la revisión de este PC (solo lee)
# Las que necesitan administrador (solo leen eventos; sale el aviso de Windows):
Start-Process powershell -Verb RunAs -ArgumentList '-ExecutionPolicy Bypass -File dev\pruebas-admin.ps1 -Log C:\ruta\pruebas.log'
```

Interfaz en el navegador con datos de muestra: servidor `turbomon-ui` de `.claude/launch.json` (`?mal` para un PC con problemas, `?nofps` sin permiso, `?nogame` sin juego).
