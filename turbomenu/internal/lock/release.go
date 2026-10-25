//go:build windows

package lock

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Status es el resultado de intentar liberar.
type Status string

const (
	StatusClosed     Status = "closed"     // el programa se cerró
	StatusReleased   Status = "released"   // se soltaron los archivos y el programa sigue abierto
	StatusNeedsForce Status = "needsForce" // no se cerró (cambios sin guardar, sin ventana…): ofrecer «Forzar»
	StatusDenied     Status = "denied"     // sin permiso: es de administrador o del sistema
	StatusGone       Status = "gone"       // el programa ya no está abierto
	StatusFailed     Status = "failed"     // no se pudo
)

// Result cuenta qué pasó al liberar.
type Result struct {
	PID         int    `json:"pid"`
	Status      Status `json:"status"`
	Message     string `json:"message"`           // frase para el usuario
	Handles     int    `json:"handles,omitempty"` // archivos soltados sin cerrar el programa
	StillLocked bool   `json:"stillLocked"`       // todavía bloquea alguna de las rutas
}

// Tiempos de espera.
const (
	gentleWait = 8 * time.Second // tras pedir que se cierre
	forceWait  = 3 * time.Second // tras forzar
)

var procIsWindowEnabled = user32.NewProc("IsWindowEnabled")

// Release libera las rutas que bloquea el proceso pid. Sin force le pide que
// se cierre como al pulsar la X; si no lo hace devuelve StatusNeedsForce.
// Con force lo termina. El Explorador y los procesos de Windows nunca se
// cierran: solo se sueltan los archivos concretos (como Unlock).
func Release(ctx context.Context, pid int, paths []string, force bool) Result {
	r := Result{PID: pid}
	e, ok := sameProcess(uint32(pid))
	if !ok {
		return gone(r)
	}
	svcs := servicesByPID()[uint32(pid)]
	switch kindOf(uint32(pid), e.exeName, svcs) {
	case KindExplorer, KindSystem, KindService:
		return Unlock(ctx, pid, paths)
	}
	if force {
		return terminate(ctx, r)
	}
	return closeGently(ctx, r, paths)
}

// closeGently manda WM_CLOSE a sus ventanas y espera a que se cierre.
func closeGently(ctx context.Context, r Result, paths []string) Result {
	pid := uint32(r.PID)
	wins := topWindows()[pid]
	if len(wins) == 0 {
		r.Status = StatusNeedsForce
		r.StillLocked = true
		r.Message = "Este programa no tiene ventana que cerrar. Puedes forzar su cierre o soltar el archivo sin cerrarlo."
		return r
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return denied(r, KindApp)
	}
	defer windows.CloseHandle(h)
	sent := 0
	for _, w := range wins {
		if p, _, _ := syscall.SyscallN(procPostMessageW.Addr(), uintptr(w.hwnd), wmClose, 0, 0); p != 0 {
			sent++
		}
	}
	if sent == 0 {
		return denied(r, KindApp) // UIPI: ventana de un programa elevado
	}
	ctx, cancel := context.WithTimeout(ctx, gentleWait)
	defer cancel()
	for ctx.Err() == nil {
		ev, _ := windows.WaitForSingleObject(h, 150)
		if ev == windows.WAIT_OBJECT_0 {
			r.Status = StatusClosed
			r.Message = "Programa cerrado."
			return r
		}
		// ¿Ha sacado un diálogo modal («¿Guardar cambios?»)? Su ventana se desactiva.
		for _, w := range wins {
			if en, _, _ := syscall.SyscallN(procIsWindowEnabled.Addr(), uintptr(w.hwnd)); en == 0 && windows.IsWindowVisible(w.hwnd) {
				r.Status = StatusNeedsForce
				r.StillLocked = true
				r.Message = "Está preguntando algo, quizá si quieres guardar los cambios. Contesta en su ventana o fuerza el cierre."
				return r
			}
		}
	}
	if !locksAny(context.Background(), pid, paths) {
		r.Status = StatusReleased
		r.Message = "Ya no lo bloquea."
		return r
	}
	r.Status = StatusNeedsForce
	r.StillLocked = true
	r.Message = "No se ha cerrado. Puede que tenga cambios sin guardar: revisa su ventana o fuerza el cierre."
	return r
}

// terminate cierra el proceso a la fuerza.
func terminate(ctx context.Context, r Result) Result {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(r.PID))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return gone(r)
		}
		return denied(r, KindApp)
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		if err == windows.ERROR_ACCESS_DENIED {
			return denied(r, KindApp)
		}
		r.Status = StatusFailed
		r.Message = "No se ha podido cerrar el programa."
		r.StillLocked = true
		return r
	}
	wait := uint32(forceWait / time.Millisecond)
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d < forceWait && d > 0 {
			wait = uint32(d / time.Millisecond)
		}
	}
	windows.WaitForSingleObject(h, wait)
	r.Status = StatusClosed
	r.Message = "Programa cerrado a la fuerza."
	return r
}

// Unlock suelta los archivos de esas rutas que el proceso tiene abiertos,
// sin cerrarlo (lo que hacen LockHunter o Unlocker). Ojo: el programa puede
// fallar o perder lo que estaba guardando en esos archivos; no suelta
// programas o DLL cargados.
func Unlock(ctx context.Context, pid int, paths []string) Result {
	r := Result{PID: pid}
	e, ok := sameProcess(uint32(pid))
	if !ok {
		return gone(r)
	}
	kind := kindOf(uint32(pid), e.exeName, servicesByPID()[uint32(pid)])
	targets, _, err := resolveTargets(paths)
	if err != nil {
		r.Status = StatusFailed
		r.Message = err.Error()
		return r
	}
	proc, err := windows.OpenProcess(windows.PROCESS_DUP_HANDLE, false, uint32(pid))
	if err != nil {
		return denied(r, kind)
	}
	defer windows.CloseHandle(proc)

	nts := make([]ntTarget, 0, len(targets))
	for _, t := range targets {
		if t.nt != "" {
			nts = append(nts, ntTarget{t.nt, t.dir})
		}
	}
	match := ntMatcher(nts)
	res, err := scanHandles(ctx, scanOpts{pid: uint32(pid), match: match, budget: 2 * time.Second})
	if err != nil {
		r.Status = StatusFailed
		r.Message = "No se ha podido revisar el programa."
		return r
	}
	cur := windows.CurrentProcess()
	for _, h := range res.handles {
		// Se vuelve a comprobar justo antes de cerrar: el número de handle
		// podría haberse reutilizado para otra cosa.
		var dup windows.Handle
		if windows.DuplicateHandle(proc, windows.Handle(h.value), cur, &dup, 0, false, 0) != nil {
			continue
		}
		// Si el número ya es de otra cosa (una tubería), pedir su nombre podría
		// quedarse colgado: solo archivos de disco, como en la búsqueda.
		if t, _ := windows.GetFileType(dup); t != windows.FILE_TYPE_DISK {
			windows.CloseHandle(dup)
			continue
		}
		nt, _ := finalPathNT(dup, nil)
		windows.CloseHandle(dup)
		if nt == "" || !match(nt) {
			continue
		}
		if windows.DuplicateHandle(proc, windows.Handle(h.value), cur, &dup, 0, false, windows.DUPLICATE_CLOSE_SOURCE) == nil {
			windows.CloseHandle(dup)
			r.Handles++
		}
	}
	r.StillLocked = locksAny(ctx, uint32(pid), paths)
	switch {
	case r.Handles > 0 && !r.StillLocked:
		r.Status = StatusReleased
		r.Message = "Liberado sin cerrar el programa."
	case r.Handles > 0:
		r.Status = StatusReleased
		r.Message = "Se han soltado sus archivos, pero aún tiene algo cargado de aquí. Hay que cerrar el programa para liberarlo del todo."
	case !r.StillLocked:
		r.Status = StatusReleased
		r.Message = "Ya no lo bloquea."
	default:
		r.Status = StatusFailed
		r.Message = "No tiene archivos abiertos que se puedan soltar: lo tiene cargado como programa. Hay que cerrarlo."
		if kind == KindExplorer || kind == KindSystem {
			r.Message = "Windows lo tiene cargado y no se puede soltar. Prueba a reiniciar el equipo."
		}
	}
	return r
}

// locksAny dice si el proceso sigue bloqueando alguna de las rutas.
func locksAny(ctx context.Context, pid uint32, paths []string) bool {
	targets, _, err := resolveTargets(paths)
	if err != nil {
		return false // ya no existen
	}
	nts := make([]ntTarget, 0, len(targets))
	for _, t := range targets {
		if h, err := openForQuery(t.dos); err == nil {
			pids, _ := pidsUsingFile(h)
			windows.CloseHandle(h)
			for _, p := range pids {
				if p == pid {
					return true
				}
			}
		}
		if t.dir {
			if exe, _, ok := imageInfo(pid); ok && exe != "" && dosInside(exe, t.dos) {
				return true
			}
		}
		if t.nt != "" {
			nts = append(nts, ntTarget{t.nt, t.dir})
		}
	}
	if len(nts) == 0 {
		return false
	}
	res, err := scanHandles(ctx, scanOpts{pid: pid, match: ntMatcher(nts), budget: time.Second})
	return err == nil && len(res.handles) > 0
}

// sameProcess comprueba que el PID sigue siendo el programa que vio Find
// (y no otro que haya heredado el número).
func sameProcess(pid uint32) (procEntry, bool) {
	if pid <= 4 {
		return procEntry{pid: pid, exeName: "System"}, true
	}
	e, ok := processSnapshot()[pid]
	if !ok {
		return e, false
	}
	startMu.Lock()
	want, known := startTimes[pid]
	startMu.Unlock()
	if known {
		if _, got, ok := imageInfo(pid); ok && got != 0 && got != want {
			return e, false
		}
	}
	return e, true
}

func gone(r Result) Result {
	r.Status = StatusGone
	r.Message = "Ese programa ya no está abierto."
	return r
}

func denied(r Result, k Kind) Result {
	r.Status = StatusDenied
	r.StillLocked = true
	switch k {
	case KindService:
		r.Message = "Es un servicio de Windows: hace falta ser administrador para liberarlo."
	case KindSystem:
		r.Message = "Es parte de Windows: no se puede tocar sin ser administrador."
	default:
		r.Message = "Es un programa de administrador. Ciérralo desde su ventana o abre TurboMENU como administrador."
	}
	return r
}

// String resume el resultado (útil en registros).
func (r Result) String() string {
	return fmt.Sprintf("%d %s: %s", r.PID, r.Status, strings.TrimSpace(r.Message))
}
