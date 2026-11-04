package main

// Instalar, arrancar con Windows y hablar con el agente.
//
// Para medir los FPS el agente tiene que ser administrador. Al instalar se
// pide permiso una vez y se crea una tarea programada que lo arranca así al
// iniciar sesión (sin volver a preguntar). Si se dice que no, arranca sin
// permiso desde la clave Run del usuario: todo funciona menos los FPS.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	turboKey = `Software\Turbo\TurboMON`
	runValue = "TurboMON"
	noWindow = 0x08000000 // CREATE_NO_WINDOW
)

func taskName() string {
	if t := os.Getenv("TURBOMON_TASK"); t != "" { // pruebas
		return t
	}
	if u, err := user.Current(); err == nil {
		n := u.Username
		if i := strings.LastIndex(n, `\`); i >= 0 {
			n = n[i+1:]
		}
		return "TurboMON (" + n + ")"
	}
	return "TurboMON"
}

// ---------- llamar al agente ----------

var httpClient = &http.Client{}

func agentCall(method, path string, body, out any, timeout time.Duration) error {
	info, ok := readAgentInfo()
	if !ok {
		return errors.New("TurboMON no está en marcha")
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", info.Port, path), rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-TurboMON", info.Token)
	cl := *httpClient
	cl.Timeout = timeout
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return errors.New(strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func agentAlive() bool {
	var v map[string]any
	return agentCall("GET", "/settings", nil, &v, 1500*time.Millisecond) == nil
}

// ensureAgent arranca el agente si no está y espera a que conteste.
func ensureAgent() error {
	if agentAlive() {
		return nil
	}
	startAgent()
	for i := 0; i < 60; i++ {
		time.Sleep(100 * time.Millisecond)
		if agentAlive() {
			return nil
		}
	}
	return errors.New("no se pudo arrancar el agente de TurboMON")
}

// startAgent arranca el agente: por la tarea (como administrador, sin preguntar) si existe.
func startAgent() {
	if taskInstalled() && startAgentTask() == nil {
		return
	}
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "--agent")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}

func startAgentTask() error {
	return runHidden("schtasks.exe", "/Run", "/TN", taskName())
}

func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: noWindow}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func stopAgent() {
	info, ok := readAgentInfo()
	if !ok {
		return
	}
	agentCall("POST", "/quit", nil, nil, 2*time.Second)
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(info.PID)); err == nil {
		windows.WaitForSingleObject(h, 5000)
		windows.CloseHandle(h)
	}
}

// ---------- arranque con Windows ----------

func taskInstalled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, turboKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Task")
	return err == nil && v != ""
}

func setTaskFlag(name string) {
	if name == "" {
		registry.DeleteKey(registry.CURRENT_USER, turboKey)
		return
	}
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, turboKey, registry.SET_VALUE); err == nil {
		k.SetStringValue("Task", name)
		k.Close()
	}
}

// setRunKey pone (o quita) el arranque sin permiso de administrador.
func setRunKey(on bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if on {
		exe, _ := os.Executable()
		k.SetStringValue(runValue, fmt.Sprintf(`"%s" --agent`, exe))
	} else {
		k.DeleteValue(runValue)
	}
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// createTask crea la tarea que arranca el agente como administrador al iniciar sesión.
// Se ejecuta ya con permiso de administrador («TurboMON.exe setup-admin»).
func createTask() error {
	exe, _ := os.Executable()
	u, err := user.Current()
	if err != nil {
		return err
	}
	x := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>TurboMON: FPS, sensores y overlay (lo instala TurboSETUP)</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + xmlEscape(u.Username) + `</UserId><Delay>PT5S</Delay></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>` + xmlEscape(u.Uid) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>6</Priority>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + xmlEscape(exe) + `</Command><Arguments>--agent</Arguments></Exec></Actions>
</Task>`
	// schtasks quiere el XML en UTF-16 con BOM.
	u16 := utf16.Encode([]rune(x))
	b := make([]byte, 2+2*len(u16))
	b[0], b[1] = 0xFF, 0xFE
	for i, c := range u16 {
		b[2+2*i], b[3+2*i] = byte(c), byte(c>>8)
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("turbomon-task-%d.xml", os.Getpid()))
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := runHidden("schtasks.exe", "/Create", "/TN", taskName(), "/XML", tmp, "/F"); err != nil {
		return err
	}
	setTaskFlag(taskName())
	return nil
}

func deleteTask() error {
	err := runHidden("schtasks.exe", "/Delete", "/TN", taskName(), "/F")
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "no existe") || strings.Contains(strings.ToLower(err.Error()), "cannot find") {
		setTaskFlag("")
		return nil
	}
	return err
}

// ---------- permiso de administrador ----------

var errUACCancelled = errors.New("no se ha dado permiso de administrador")

// runElevated lanza «TurboMON.exe <op> <archivo de resultado>» como administrador
// (Windows lo pregunta) y espera a que acabe.
func runElevated(op string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	res := filepath.Join(os.TempDir(), fmt.Sprintf("turbomon-%s-%d.txt", op, os.Getpid()))
	os.Remove(res)
	defer os.Remove(res)
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(fmt.Sprintf(`%s "%s"`, op, res))
	info := shellExecuteInfo{fMask: 0x40 | 0x100, lpVerb: verb, lpFile: file, lpParameters: params, nShow: 0}
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, e := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if e == windows.ERROR_CANCELLED {
			return errUACCancelled
		}
		return fmt.Errorf("no se pudo pedir permiso de administrador: %v", e)
	}
	if info.hProcess != 0 {
		windows.WaitForSingleObject(info.hProcess, 60000)
		windows.CloseHandle(info.hProcess)
	}
	b, err := os.ReadFile(res)
	if err != nil {
		return errors.New("no ha contestado (¿se ha cancelado?)")
	}
	if msg := strings.TrimSpace(string(b)); msg != "ok" {
		return errors.New(msg)
	}
	return nil
}

// cliAdmin: lo que se hace ya con permiso («setup-admin» y «uninstall-admin»).
func cliAdmin(op, resFile string) int {
	var err error
	switch op {
	case "setup-admin":
		err = createTask()
	case "uninstall-admin":
		err = deleteTask()
	}
	msg := "ok"
	if err != nil {
		msg = err.Error()
	}
	if resFile != "" {
		os.WriteFile(resFile, []byte(msg), 0o644)
	}
	if err != nil {
		return 1
	}
	return 0
}

// ---------- órdenes de TurboSETUP ----------

func cliSetup() int {
	if exe, err := os.Executable(); err == nil {
		createShortcut(startMenuLink(), exe, "FPS, sensores y overlay para jugar")
	}
	stopAgent()
	if err := runElevated("setup-admin"); err != nil {
		// Sin permiso: arranca igual, pero sin FPS (se puede activar luego).
		fmt.Fprintln(os.Stderr, "sin permiso de administrador:", err)
		setRunKey(true)
	} else {
		setRunKey(false)
	}
	startAgent()
	fmt.Println("TurboMON instalado: está junto a la flechita de la barra de tareas.")
	return 0
}

func cliUninstall() int {
	stopAgent()
	setRunKey(false)
	os.Remove(startMenuLink())
	if taskInstalled() {
		if deleteTask() != nil {
			if err := runElevated("uninstall-admin"); err != nil {
				fmt.Fprintln(os.Stderr, "no se pudo quitar la tarea de inicio:", err)
			}
		}
	}
	setTaskFlag("")
	os.RemoveAll(filepath.Join(dataDir(), "webview"))
	os.Remove(agentInfoPath())
	fmt.Println("TurboMON desinstalado.")
	return 0
}

func startMenuLink() string {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		dir = filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	}
	return filepath.Join(dir, "TurboMON.lnk")
}

// createShortcut crea un .lnk con WScript.Shell (COM), que viene con Windows.
func createShortcut(lnk, target, description string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oe, ok := err.(*ole.OleError); !ok || oe.Code() != 1 {
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
