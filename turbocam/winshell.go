package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

// ---------- cierre pedido por el desinstalador ----------

const quitEventName = `Local\TurboCAM.Quit`

func quitEvent() (windows.Handle, error) {
	name, _ := windows.UTF16PtrFromString(quitEventName)
	return windows.CreateEvent(nil, 1, 0, name)
}

// waitEvent espera al evento; devuelve false si antes se cierra stop.
func waitEvent(ev windows.Handle, stop <-chan struct{}) bool {
	defer windows.CloseHandle(ev)
	for {
		r, _ := windows.WaitForSingleObject(ev, 500)
		if r == windows.WAIT_OBJECT_0 {
			return true
		}
		select {
		case <-stop:
			return false
		default:
		}
	}
}

// signalQuit pide a un TurboCAM abierto que se cierre y espera un poco a que lo haga.
func signalQuit() {
	name, _ := windows.UTF16PtrFromString(quitEventName)
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return // no hay ninguno abierto
	}
	windows.SetEvent(ev)
	windows.CloseHandle(ev)
	for i := 0; i < 40 && findWindow(windowClass) != 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

// ---------- acceso directo ----------

func startMenuLink() string {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		dir = filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	}
	return filepath.Join(dir, "TurboCAM.lnk")
}

// createShortcut crea un .lnk con WScript.Shell (COM), que viene con Windows.
func createShortcut(lnk, target, description string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oe, ok := err.(*ole.OleError); !ok || oe.Code() != 1 { // 1 = S_FALSE: ya estaba iniciado
			return err
		}
	}
	defer ole.CoUninitialize()
	unk, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unk.Release()
	shell, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	v, err := oleutil.CallMethod(shell, "CreateShortcut", lnk)
	if err != nil {
		return err
	}
	sc := v.ToIDispatch()
	defer sc.Release()
	oleutil.PutProperty(sc, "TargetPath", target)
	oleutil.PutProperty(sc, "WorkingDirectory", filepath.Dir(target))
	oleutil.PutProperty(sc, "Description", description)
	oleutil.PutProperty(sc, "IconLocation", target+",0")
	_, err = oleutil.CallMethod(sc, "Save")
	return err
}
