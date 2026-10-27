/*
 * Contrato entre TurboCAM.exe (escribe) y la cámara virtual (lee).
 *
 * Es una memoria compartida con nombre. Cualquiera de los dos lados la crea si no
 * existe, así que la cámara puede abrirse antes que TurboCAM y al revés. La lee y
 * escribe código en Go (shm.go) y en C (turbocam.c): si cambia algo de aquí hay que
 * cambiarlo también allí. La prueba TestProtocolMatchesHeader lo comprueba.
 *
 * Todos los campos son de 32 bits y alineados, para que las lecturas y escrituras
 * sean atómicas también en la DLL de 32 bits.
 *
 * Cabecera (TC_OFF_*): la escribe la app.
 *   magic, version  identifican el formato.
 *   state           TC_STATE_*: qué enseñar si no hay imagen.
 *   latest          número del último fotograma completo (0 = ninguno). Va en la
 *                   ranura latest % TC_SLOTS.
 *   heartbeat       GetTickCount() de la app; se renueva cada ~250 ms. Si se queda
 *                   viejo, la app se ha cerrado o colgado.
 *
 * Consumidores (TC_CONS_*): los escribe la cámara, uno por cada app que la usa.
 *   pid             proceso (0 = libre). Se reclama con una operación atómica.
 *   seen            GetTickCount() de la última señal de vida.
 *   width, height   tamaño que ha pedido esa app.
 *   fps             fotogramas por segundo que ha pedido.
 *   exe             nombre del ejecutable en UTF-16 (sin ruta, cortado).
 *
 * Ranuras (TC_SLOT_*): tres búferes NV12. La app escribe en la que no es la última:
 *   seq = 0, datos, seq = número, latest = número. La cámara copia la última y
 *   vuelve a mirar seq: si ha cambiado mientras copiaba, descarta la copia.
 */
#ifndef TURBOCAM_PROTOCOL_H
#define TURBOCAM_PROTOCOL_H

#define TC_MAP_NAME L"Local\\TurboCAM.Video.1"
/* Permite el acceso a procesos con menos privilegios (navegadores, apps de la tienda). */
#define TC_MAP_SDDL L"D:(A;;GA;;;WD)(A;;GA;;;AC)(A;;GA;;;S-1-15-2-2)S:(ML;;NW;;;LW)"

#define TC_MAGIC 0x4D414354 /* "TCAM" */
#define TC_VERSION 1

#define TC_MAX_W 1920
#define TC_MAX_H 1080
#define TC_SLOTS 3
#define TC_MAX_CONSUMERS 16

#define TC_STATE_OFF 0     /* TurboCAM cerrado */
#define TC_STATE_WAITING 1 /* abierto, sin ningún móvil */
#define TC_STATE_LIVE 2    /* hay imagen */
#define TC_STATE_PAUSED 3  /* el móvil está en segundo plano o bloqueado */
#define TC_STATE_COVER 4   /* imagen tapada a propósito desde el PC */

/* Tras este tiempo sin latido, la cámara da la app por cerrada. */
#define TC_HEARTBEAT_TIMEOUT_MS 3000
/* Tras este tiempo sin señal, un consumidor se da por muerto y su sitio se reutiliza. */
#define TC_CONSUMER_TIMEOUT_MS 5000

#define TC_OFF_MAGIC 0
#define TC_OFF_VERSION 4
#define TC_OFF_STATE 8
#define TC_OFF_LATEST 12
#define TC_OFF_HEARTBEAT 16
#define TC_OFF_FPS 20
#define TC_OFF_WIDTH 24
#define TC_OFF_HEIGHT 28

#define TC_CONS_BASE 64
#define TC_CONS_SIZE 64
#define TC_CONS_PID 0
#define TC_CONS_SEEN 4
#define TC_CONS_WIDTH 8
#define TC_CONS_HEIGHT 12
#define TC_CONS_FPS 16
#define TC_CONS_EXE 20
#define TC_CONS_EXE_CHARS 22

#define TC_SLOT_BASE 4096
#define TC_SLOT_SEQ 0
#define TC_SLOT_WIDTH 4
#define TC_SLOT_HEIGHT 8
#define TC_SLOT_DATA 64
#define TC_FRAME_BYTES (TC_MAX_W * TC_MAX_H * 3 / 2)
#define TC_SLOT_STRIDE 3112960 /* TC_SLOT_DATA + TC_FRAME_BYTES, redondeado a 4 KB */

#define TC_MAP_SIZE (TC_SLOT_BASE + TC_SLOTS * TC_SLOT_STRIDE)

#endif
