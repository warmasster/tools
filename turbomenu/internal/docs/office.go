package docs

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Conversión de documentos de Office a PDF con Microsoft Office (COM desde
// PowerShell) o con LibreOffice (soffice --headless).

// OfficeApps dice qué programas de oficina hay instalados.
type OfficeApps struct {
	Word        bool   `json:"word"`
	Excel       bool   `json:"excel"`
	PowerPoint  bool   `json:"powerPoint"`
	LibreOffice string `json:"libreOffice"` // ruta de soffice.exe ("" si no está)
}

var officeOnce struct {
	sync.Once
	apps OfficeApps
}

// DetectOffice mira en el registro y en disco (sin lanzar nada) qué hay instalado.
// El resultado se calcula una vez por proceso.
func DetectOffice() OfficeApps {
	officeOnce.Do(func() {
		a := &officeOnce.apps
		a.Word = hasCOM("Word.Application")
		a.Excel = hasCOM("Excel.Application")
		a.PowerPoint = hasCOM("PowerPoint.Application")
		a.LibreOffice = findSoffice()
	})
	return officeOnce.apps
}

func hasCOM(progID string) bool {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, progID+`\CLSID`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	return err == nil && v != ""
}

func findSoffice() string {
	try := func(p string) string {
		if p == "" {
			return ""
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		return ""
	}
	// Ruta de instalación registrada por LibreOffice.
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			k, err := registry.OpenKey(root, `SOFTWARE\LibreOffice\UNO\InstallPath`, registry.QUERY_VALUE|view)
			if err == nil {
				v, _, err := k.GetStringValue("")
				k.Close()
				if err == nil {
					if p := try(filepath.Join(v, "soffice.exe")); p != "" {
						return p
					}
				}
			}
		}
		k, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\soffice.exe`, registry.QUERY_VALUE)
		if err == nil {
			v, _, err := k.GetStringValue("")
			k.Close()
			if err == nil {
				if p := try(strings.Trim(v, `"`)); p != "" {
					return p
				}
			}
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		if base := os.Getenv(env); base != "" {
			if p := try(filepath.Join(base, "LibreOffice", "program", "soffice.exe")); p != "" {
				return p
			}
		}
	}
	return ""
}

// Tipos de documento de Office.
const (
	docWord  = "word"
	docExcel = "excel"
	docSlide = "powerpoint"
)

var officeKinds = map[string]string{
	"doc": docWord, "docx": docWord, "docm": docWord, "dot": docWord, "dotx": docWord, "odt": docWord, "rtf": docWord, "wpd": docWord,
	"xls": docExcel, "xlsx": docExcel, "xlsm": docExcel, "xlsb": docExcel, "ods": docExcel,
	"ppt": docSlide, "pptx": docSlide, "pptm": docSlide, "pps": docSlide, "ppsx": docSlide, "odp": docSlide,
}

// IsOfficeDoc dice si el archivo es un documento de Office que se puede pasar a PDF.
func IsOfficeDoc(path string) bool {
	_, ok := officeKinds[lowerExt(path)]
	return ok
}

// OfficeEngine dice qué motor de conversión hay ("Microsoft Office" o
// "LibreOffice") y si hay alguno.
func OfficeEngine() (string, bool) {
	a := DetectOffice()
	switch {
	case a.Word || a.Excel || a.PowerPoint:
		return "Microsoft Office", true
	case a.LibreOffice != "":
		return "LibreOffice", true
	}
	return "", false
}

// OfficeEngineFor dice con qué se convertiría ese archivo. Si no hay nada,
// devuelve false y un texto con lo que falta («Hace falta Microsoft Word o LibreOffice»).
func OfficeEngineFor(path string) (string, bool) {
	kind, ok := officeKinds[lowerExt(path)]
	if !ok {
		return "No es un documento de Office", false
	}
	eng := officeEngineFor(kind, lowerExt(path))
	if eng != "" {
		if eng == "lo" {
			return "LibreOffice", true
		}
		return "Microsoft " + officeAppName(kind), true
	}
	return "Hace falta Microsoft " + officeAppName(kind) + " o LibreOffice", false
}

func officeAppName(kind string) string {
	switch kind {
	case docExcel:
		return "Excel"
	case docSlide:
		return "PowerPoint"
	}
	return "Word"
}

// officeEngineFor devuelve "ms", "lo" o "".
func officeEngineFor(kind, ext string) string {
	a := DetectOffice()
	ms := (kind == docWord && a.Word) || (kind == docExcel && a.Excel) || (kind == docSlide && a.PowerPoint)
	lo := a.LibreOffice != ""
	// Para formatos abiertos (odt, ods, odp) es mejor LibreOffice.
	if strings.HasPrefix(ext, "od") && lo {
		return "lo"
	}
	if ms {
		return "ms"
	}
	if lo {
		return "lo"
	}
	return ""
}

// OfficeToPDF convierte un documento de Office a PDF. Con out vacío se guarda
// junto al original como «doc.pdf» (sin pisar nada).
func OfficeToPDF(ctx context.Context, in, out string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	kind, ok := officeKinds[lowerExt(in)]
	if !ok {
		return Result{}, uerr(nil, "No es un documento de Office")
	}
	if _, err := os.Stat(in); err != nil {
		return Result{}, cleanErr(ctx, err, "No se puede abrir el documento")
	}
	abs, err := filepath.Abs(in)
	if err == nil {
		in = abs
	}
	eng := officeEngineFor(kind, lowerExt(in))
	if eng == "" {
		msg, _ := OfficeEngineFor(in)
		return Result{}, uerr(nil, "%s para convertir este documento", msg)
	}
	var o *outFile
	if out == "" {
		o, err = createOut(in, "", ".pdf")
	} else {
		o, err = createOutAt(out)
	}
	if err != nil {
		return Result{}, err
	}
	o.f.Close()
	os.Remove(o.Tmp)
	tmpPDF := o.Tmp + ".pdf"
	if eng == "ms" {
		err = msOfficeToPDF(ctx, kind, in, tmpPDF)
	} else {
		err = sofficeToPDF(ctx, in, tmpPDF)
	}
	if err == nil {
		if st, e := os.Stat(tmpPDF); e != nil || st.Size() == 0 {
			err = uerr(e, "No se ha podido convertir el documento")
		}
	}
	if err != nil {
		os.Remove(tmpPDF)
		os.Remove(o.Final)
		return Result{}, cleanErr(ctx, err, "No se ha podido convertir el documento")
	}
	if err := os.Rename(tmpPDF, o.Final); err != nil {
		os.Remove(tmpPDF)
		os.Remove(o.Final)
		return Result{}, cleanErr(ctx, err, "No se ha podido guardar el PDF")
	}
	return Result{Out: o.Final, Before: fileSize(in), After: fileSize(o.Final)}, nil
}

const psWordToPDF = `
$app = $null; $doc = $null
try {
  $app = New-Object -ComObject Word.Application
  try { 'PID ' + (Get-Process WINWORD -ErrorAction Stop | Sort-Object StartTime -Descending | Select-Object -First 1).Id } catch {}
  $app.Visible = $false
  $app.DisplayAlerts = 0
  try { $app.AutomationSecurity = 3 } catch {}
  $m = [Type]::Missing
  $doc = $app.Documents.Open($in, $false, $true, $false, 'turbo-x', 'turbo-x', $false, 'turbo-x', 'turbo-x', 0, $m, $false, $false, $m, $true)
  $doc.ExportAsFixedFormat($out, 17)
  'OK'
} catch { 'ERR ' + $_.Exception.Message }
finally {
  if ($doc) { try { $doc.Close(0) } catch {}; [void][Runtime.InteropServices.Marshal]::ReleaseComObject($doc) }
  if ($app) { try { $app.Quit(0) } catch {}; [void][Runtime.InteropServices.Marshal]::ReleaseComObject($app) }
}
`

const psExcelToPDF = `
$app = $null; $wb = $null
try {
  $app = New-Object -ComObject Excel.Application
  try { 'PID ' + (Get-Process EXCEL -ErrorAction Stop | Sort-Object StartTime -Descending | Select-Object -First 1).Id } catch {}
  $app.Visible = $false
  $app.DisplayAlerts = $false
  $app.AskToUpdateLinks = $false
  try { $app.AutomationSecurity = 3 } catch {}
  $m = [Type]::Missing
  $wb = $app.Workbooks.Open($in, 0, $true, $m, 'turbo-x', 'turbo-x', $true)
  $wb.ExportAsFixedFormat(0, $out)
  'OK'
} catch { 'ERR ' + $_.Exception.Message }
finally {
  if ($wb) { try { $wb.Close($false) } catch {}; [void][Runtime.InteropServices.Marshal]::ReleaseComObject($wb) }
  if ($app) { try { $app.Quit() } catch {}; [void][Runtime.InteropServices.Marshal]::ReleaseComObject($app) }
}
`

const psPowerPointToPDF = `
$app = $null; $pres = $null
try {
  $app = New-Object -ComObject PowerPoint.Application
  try { 'PID ' + (Get-Process POWERPNT -ErrorAction Stop | Sort-Object StartTime -Descending | Select-Object -First 1).Id } catch {}
  try { $app.DisplayAlerts = 1 } catch {}
  try { $app.AutomationSecurity = 3 } catch {}
  $pres = $app.Presentations.Open($in, -1, 0, 0)
  $pres.SaveAs($out, 32)
  'OK'
} catch { 'ERR ' + $_.Exception.Message }
finally {
  if ($pres) { try { $pres.Close() } catch {}; [void][Runtime.InteropServices.Marshal]::ReleaseComObject($pres) }
  if ($app) { try { $app.Quit() } catch {}; [void][Runtime.InteropServices.Marshal]::ReleaseComObject($app) }
}
`

func msOfficeToPDF(ctx context.Context, kind, in, out string) error {
	script := psWordToPDF
	switch kind {
	case docExcel:
		script = psExcelToPDF
	case docSlide:
		script = psPowerPointToPDF
	}
	script = "$in = " + psQuote(in) + "\n$out = " + psQuote(out) + "\n" + script
	var pid int
	var mu sync.Mutex
	err := runPS(ctx, script, func(line string) {
		if strings.HasPrefix(line, "PID ") {
			mu.Lock()
			pid, _ = strconv.Atoi(strings.TrimPrefix(line, "PID "))
			mu.Unlock()
		}
	})
	if err != nil {
		// Si se canceló a medias, el programa de Office puede quedarse abierto.
		mu.Lock()
		p := pid
		mu.Unlock()
		if ctx.Err() != nil && p > 0 {
			if pr, e := os.FindProcess(p); e == nil {
				pr.Kill()
			}
		}
		if ctx.Err() != nil {
			return ErrCanceled
		}
		msg := strings.ToLower(psMessage(err))
		if strings.Contains(msg, "password") || strings.Contains(msg, "contraseña") {
			return uerr(err, "El documento está protegido con contraseña")
		}
		return uerr(err, "Microsoft %s no ha podido convertir el documento", officeAppName(kind))
	}
	return nil
}

// LibreOffice usa un perfil propio para no chocar con una ventana abierta.
var sofficeMu sync.Mutex

func sofficeProfile() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "TurboMENU", "libreoffice")
}

func sofficeToPDF(ctx context.Context, in, out string) error {
	exe := DetectOffice().LibreOffice
	if exe == "" {
		return uerr(nil, "Hace falta LibreOffice")
	}
	sofficeMu.Lock()
	defer sofficeMu.Unlock()
	outDir, err := os.MkdirTemp("", "turbo-office-*")
	if err != nil {
		return cleanErr(ctx, err, "No se ha podido convertir el documento")
	}
	defer os.RemoveAll(outDir)
	prof := sofficeProfile()
	os.MkdirAll(prof, 0o755)
	profURL := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(prof)}).String()
	cmd := hiddenCmd(ctx, exe, "--headless", "--norestore", "--nologo", "--nodefault", "--nolockcheck",
		"-env:UserInstallation="+profURL, "--convert-to", "pdf", "--outdir", outDir, in)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return uerr(err, "No se ha podido iniciar LibreOffice")
	}
	// Trabajo de Windows para cerrar también soffice.bin si se cancela.
	job := killOnCloseJob(cmd.Process.Pid)
	if job != 0 {
		defer windows.CloseHandle(job)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		if job != 0 {
			windows.TerminateJobObject(job, 1)
		}
		<-done
		return ErrCanceled
	}
	if ctx.Err() != nil {
		return ErrCanceled
	}
	produced := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))+".pdf")
	if _, e := os.Stat(produced); e != nil {
		// Busca cualquier PDF por si cambió el nombre.
		ents, _ := os.ReadDir(outDir)
		for _, e := range ents {
			if strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
				produced = filepath.Join(outDir, e.Name())
				break
			}
		}
	}
	if _, e := os.Stat(produced); e != nil {
		return uerr(err, "LibreOffice no ha podido convertir el documento")
	}
	// Mover (el temporal está en otra unidad quizá): copiar si hace falta.
	if err := os.Rename(produced, out); err != nil {
		data, e := os.ReadFile(produced)
		if e != nil {
			return cleanErr(ctx, e, "No se ha podido guardar el PDF")
		}
		if e := os.WriteFile(out, data, 0o644); e != nil {
			return cleanErr(ctx, e, "No se ha podido guardar el PDF")
		}
	}
	return nil
}

// killOnCloseJob mete el proceso en un trabajo que muere al cerrarse.
func killOnCloseJob(pid int) windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return 0
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return 0
	}
	return job
}
