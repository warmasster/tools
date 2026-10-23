package media

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow    = 0x08000000
	belowNormalPrio   = 0x00004000
	maxStderrCaptured = 16 << 10
)

// Job compartido: al cerrarse TurboMENU, Windows mata todos los procesos hijos.
var (
	jobOnce   sync.Once
	jobHandle windows.Handle
)

func job() windows.Handle {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		jobHandle = h
	})
	return jobHandle
}

// command prepara un proceso hijo sin ventana, con prioridad baja (el PC sigue
// respondiendo) y que muere con el contexto.
func command(ctx context.Context, exe string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | belowNormalPrio}
	return cmd
}

// start arranca el proceso y lo mete en el Job.
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	if j := job(); j != 0 {
		if h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)); err == nil {
			windows.AssignProcessToJobObject(j, h)
			windows.CloseHandle(h)
		}
	}
	return nil
}

// output ejecuta y devuelve stdout; stderr queda en el error.
func output(ctx context.Context, exe string, args ...string) ([]byte, []byte, error) {
	cmd := command(ctx, exe, args...)
	var out bytes.Buffer
	errBuf := &tailBuffer{max: maxStderrCaptured}
	cmd.Stdout = &out
	cmd.Stderr = errBuf
	if err := start(cmd); err != nil {
		return nil, nil, err
	}
	err := cmd.Wait()
	return out.Bytes(), errBuf.Bytes(), err
}

// tailBuffer guarda solo los últimos bytes escritos (el log de error de ffmpeg).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.buf...)
}
