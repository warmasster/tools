# TurboMON

FPS, sensores y overlay para jugar, el historial de tus partidas y una revisión de que tu PC está configurado para rendir lo que debe. Pensado para gastar lo mínimo: en segundo plano va al 0 % de CPU y ocupa unos 25 MB.

## Qué hace

- **Overlay en el juego**: FPS, 1 % bajo, CPU, gráfica (uso y temperatura), RAM y VRAM en una esquina de la pantalla. Se pone y se quita con **un clic en el icono de la bandeja** (junto a la flechita de la barra de tareas) o con **Ctrl+Alt+O**. Hay un modo «solo FPS» y otro que solo lo enseña con un juego delante.
- **Inicio sencillo**: la nota de tu PC (de 0 a 100), el interruptor del overlay, cuatro cifras grandes (FPS, procesador, gráfica y RAM) y lo que hay que arreglar, con su botón.
- **Revisión del equipo** (17 comprobaciones), cada una con qué hay, por qué importa y cómo se arregla:
  - Memoria: velocidad de la RAM y perfil **XMP/EXPO** (lee el modelo del módulo para saber a cuánto puede ir), **doble canal** y cantidad.
  - Pantalla: cada monitor a sus **hercios máximos** y a su resolución, y que esté **conectado a la gráfica** y no a la placa base.
  - Gráfica: antigüedad del driver y programación de GPU acelerada por hardware.
  - Energía: plan de energía, límite del procesador y, en portátiles, si está enchufado.
  - Windows: modo de juego, grabación de fondo de la Xbox Game Bar, memoria virtual, aceleración del ratón y programas que arrancan solos.
  - Discos: espacio libre y si los juegos (Steam y Epic) están en SSD.
  - Lo que se puede arreglar sin riesgo tiene botón: hercios, resolución, plan de energía, límite del procesador, modo de juego, grabación de fondo y aceleración del ratón. Lo demás (XMP, cables) trae los pasos.
- **Partidas**: detecta solo cuándo juegas y apunta FPS medios, 1 % y 0,1 % bajos, CPU, gráfica, RAM, VRAM y temperaturas, con una gráfica de toda la partida. Dice qué te limita (la gráfica, el procesador o un tope de FPS) y compara con la vez anterior del mismo juego.
- **Sensores** en directo: uso de cada hilo, frecuencias, temperaturas, VRAM, ventilador, memoria en uso, en espera y libre, disco y red.
- **Herramientas**:
  - **Modo juego Turbo**: durante la partida pone el plan de energía de más rendimiento, da prioridad al juego y, si se acaba la memoria libre, suelta la memoria en espera. Al cerrar el juego lo deja todo como estaba.
  - **Liberar la memoria en espera** a mano.
  - **Lo que más gasta ahora**, agrupado por programa, con botón para cerrarlo.
  - Lo que *no* hace a propósito: «limpiar la RAM» vaciando la memoria de los programas, que solo provoca tirones.

## Permiso para los FPS

Windows solo deja ver los fotogramas a un programa con permiso de administrador (igual que a MSI Afterburner o PresentMon). TurboMON lo pide **una vez** al instalarlo y crea una tarea que lo arranca así al iniciar sesión, sin volver a preguntar. Si dices que no, funciona todo menos los FPS, y se puede activar luego desde la ventana o el menú de la bandeja.

No se mete en el juego: escucha los avisos que Windows ya da con cada fotograma (DXGI, Direct3D 9 y, para Vulkan y OpenGL, el núcleo gráfico). Por eso no da problemas con los antitrampas. Como el overlay es una ventana normal por encima de todo, se ve en juegos **en ventana o en pantalla completa sin bordes**, pero no en «pantalla completa exclusiva».

La **temperatura del procesador** no la da Windows sin un driver especial. Si tienes HWiNFO abierto con «Shared Memory Support», TurboMON la lee de ahí. La de la gráfica sí la da Windows para cualquier marca.

## Lo que ocupa y gasta

- El agente de la bandeja lee los sensores **una vez por segundo solo si alguien mira** (overlay, ventana o una partida). Si no, lee procesador y memoria cada 5 s. Una lectura completa tarda unos 2 ms.
- No usa la librería de NVIDIA (NVML), que ocupa 20 MB solo con cargarla. Uso, temperatura, frecuencia, ventilador y VRAM salen de Windows y valen para cualquier marca.
- Cada partida guardada ocupa entre 3 y 15 KB: un resumen y un punto de la gráfica cada 5 s. Los tiempos de fotograma se guardan en un histograma de tamaño fijo, no uno a uno. Si una partida es muy larga, los puntos se juntan de dos en dos. A los 90 días se quita la gráfica y se queda el resumen.
- La ventana solo existe mientras está abierta. Al cerrarla, TurboMON sigue en la bandeja.

## Para desarrollar

Un solo `TurboMON.exe` con varios papeles:

| Orden | Qué es |
|---|---|
| `TurboMON.exe` | La ventana (Wails). Arranca el agente si no está. |
| `TurboMON.exe --agent` | El agente: bandeja, overlay (Win32 + GDI), atajo, sensores, FPS (ETW) y partidas. Atiende a la ventana por HTTP en `127.0.0.1` con un token que guarda en `agente.json`. |
| `setup` / `uninstall` | Lo que usa TurboSETUP. `setup` pide permiso (`setup-admin`) para crear la tarea programada `TurboMON (<usuario>)`; si no se da, usa la clave `Run` del usuario. |

Carpetas:

- `internal/sensors`: CPU (`GetSystemTimes`, `NtQuerySystemInformation`), memoria, contadores PDH, gráficas (DXGI para enumerarlas y D3DKMT para temperatura, ventilador y frecuencias) y lectura opcional de HWiNFO.
- `internal/fps`: sesión ETW en tiempo real y el contador por proceso (FPS, 1 % bajo, histograma de la partida).
- `internal/checks`: las comprobaciones (tabla SMBIOS para la RAM, `QueryDisplayConfig` para las pantallas, planes de energía, registro) y sus arreglos.
- `dev/icon`: genera los iconos (`go run ./dev/icon`).

Datos en `%LOCALAPPDATA%\TurboMON`: `ajustes.json`, `partidas.jsonl`, `turbomon.log` y `agente.json`. `TURBOMON_DIR` cambia esa carpeta (pruebas).

Pruebas:

```powershell
.\build.ps1                                                # vet, pruebas y TurboMON.exe
go test -run TestRun -v ./internal/checks/                 # la revisión de este PC (solo lee)
$env:TURBOMON_REAL="1"; go test -run RealETW -v ./internal/fps/   # FPS de verdad (como administrador)
```

Interfaz en el navegador con datos de muestra: servidor `turbomon-ui` de `.claude/launch.json` (`?mal` para un PC mal configurado, `?nofps` sin permiso, `?nogame` sin juego).
