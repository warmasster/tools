// turbodl-native: motor nativo de TurboDL.
//
//   - Lo arranca Chrome como host de Native Messaging (com.turbodl.native).
//   - También se usa solo desde la terminal:
//     turbodl-native get [opciones] URL
//     turbodl-native install [--extension-id ID]   (o doble clic en el .exe)
//     turbodl-native uninstall
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

const (
	hostName    = "com.turbodl.native"
	extensionID = "loafabaafjgjbfobghcldnkofbjjficn" // fijado con "key" en manifest.json
)

// version la pone build.ps1 a partir de manifest.json (-ldflags -X main.version=...).
var version = "dev"

func main() {
	args := os.Args[1:]
	// Chrome pasa el origen de la extensión como primer argumento.
	if len(args) > 0 && strings.HasPrefix(args[0], "chrome-extension://") {
		runHost()
		return
	}
	if len(args) == 0 {
		if launchedByDoubleClick() {
			setup() // motor + extensión + guía para cargarla en Chrome
			return
		}
		usage()
		return
	}
	switch args[0] {
	case "get", "descargar":
		os.Exit(cliGet(args[1:]))
	case "install", "instalar":
		var ids []string
		for i := 1; i < len(args); i++ {
			if args[i] == "--extension-id" && i+1 < len(args) {
				ids = append(ids, args[i+1])
				i++
			}
		}
		msg, err := install(ids)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Println(msg)
		if dir, err := installExtension(); err != nil {
			fmt.Fprintln(os.Stderr, "No se pudo copiar la extensión:", err)
		} else if dir != "" {
			fmt.Println("Extensión copiada en", dir)
			fmt.Println("Cárgala una vez en chrome://extensions → Modo de desarrollador → Cargar descomprimida.")
		}
	case "uninstall", "desinstalar":
		msg, err := uninstall()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Println(msg)
	case "status", "estado":
		printStatus()
	case "version", "--version":
		fmt.Println("turbodl-native", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`TurboDL · motor nativo ` + version + `

Uso:
  turbodl-native get [opciones] URL     descargar una URL
  TurboDL.exe (doble clic)               instalar el motor y dejar la extensión lista
  turbodl-native install                 lo mismo desde la terminal
  turbodl-native uninstall               quitar el motor (la extensión se quita en Chrome)
  turbodl-native status                  ver qué está instalado y dónde

Opciones de get:
  -c N            conexiones (por defecto 16, máximo 64)
  -o CARPETA      carpeta de destino (por defecto Descargas)
  -n NOMBRE       nombre del archivo
  -H "K: V"       cabecera extra (se puede repetir), p. ej. -H "Cookie: a=b"
  --referer URL   cabecera Referer
  --ua TEXTO      User-Agent (por defecto turbodl-native/VERSIÓN)
  --http2         permitir HTTP/2 (por defecto cada conexión es un TCP propio)
  --no-spread     no repartir conexiones entre las IPs del servidor

Ctrl+C pausa guardando el progreso; el mismo comando otra vez continúa.
`)
}

func cliGet(args []string) int {
	req := Request{Headers: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Falta el valor de %s\n", a)
				os.Exit(2)
			}
			i++
			return args[i]
		}
		switch a {
		case "-c":
			req.Connections, _ = strconv.Atoi(next())
		case "-o":
			req.Dir = next()
		case "-n":
			req.Filename = next()
		case "-H":
			k, v, ok := strings.Cut(next(), ":")
			if ok {
				req.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		case "--referer":
			req.Headers["Referer"] = next()
		case "--ua":
			req.Headers["User-Agent"] = next()
		case "--http2":
			req.HTTP2 = true
		case "--no-spread":
			req.NoSpread = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "Opción desconocida: %s\n", a)
				return 2
			}
			req.URL = a
		}
	}
	if req.URL == "" {
		usage()
		return 2
	}
	if req.Dir == "" {
		req.Dir = DownloadsDir()
	}

	d := NewDownload(req)
	start := time.Now()
	var last Progress
	d.OnInfo = func(i Info) {
		verb := "Descargando"
		if i.Resumed {
			verb = "Reanudando"
		}
		size := "tamaño desconocido"
		if i.Size >= 0 {
			size = fmtBytes(i.Size)
		}
		fmt.Fprintf(os.Stderr, "%s %s (%s)\n", verb, i.FinalPath, size)
		if !i.RangeOK {
			fmt.Fprintln(os.Stderr, "El servidor no admite rangos: una sola conexión.")
		}
	}
	d.OnNotice = func(s string) { fmt.Fprintf(os.Stderr, "\r\033[K%s\n", s) }
	d.OnProgress = func(p Progress) {
		last = p
		fmt.Fprintf(os.Stderr, "\r\033[K%s", progressLine(p))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	go func() { <-ctx.Done(); d.Pause() }()
	path, err := d.Run(context.Background())
	stop()
	fmt.Fprintln(os.Stderr)
	switch {
	case err == nil:
		secs := time.Since(start).Seconds()
		fmt.Fprintf(os.Stderr, "Terminado en %.1f s (media %s/s): %s\n", secs, fmtBytes(int64(float64(last.BytesDone)/secs)), path)
		return 0
	case errors.Is(err, ErrPaused):
		fmt.Fprintln(os.Stderr, "Pausada. Ejecuta el mismo comando para continuar.")
		return 130
	default:
		fmt.Fprintln(os.Stderr, "Error:", errorText(err))
		return 1
	}
}

// progressLine: "  45.3 %  1.2 GB / 2.7 GB  118.4 MB/s  16 con.  ETA 12 s  [██▓░░…]"
func progressLine(p Progress) string {
	var b strings.Builder
	if p.Size > 0 {
		fmt.Fprintf(&b, "%5.1f %%  %s / %s", float64(p.BytesDone)*100/float64(p.Size), fmtBytes(p.BytesDone), fmtBytes(p.Size))
	} else {
		fmt.Fprintf(&b, "%s", fmtBytes(p.BytesDone))
	}
	fmt.Fprintf(&b, "  %s/s  %d con.", fmtBytes(int64(p.Speed)), p.Connections)
	if p.ETA >= 0 {
		fmt.Fprintf(&b, "  ETA %s", (time.Duration(p.ETA) * time.Second).Round(time.Second))
	}
	if p.Size > 0 {
		b.WriteString("  " + segmentBar(p, 30))
	}
	return b.String()
}

// segmentBar dibuja qué partes del archivo están ya descargadas (estilo IDM).
func segmentBar(p Progress, width int) string {
	cells := make([]float64, width)
	cell := float64(p.Size) / float64(width)
	for _, s := range p.Segments {
		start, done := float64(s[0]), float64(s[2])
		for c := int(start / cell); c < width && float64(c)*cell < done; c++ {
			lo, hi := max(float64(c)*cell, start), min(float64(c+1)*cell, done)
			if hi > lo {
				cells[c] += (hi - lo) / cell
			}
		}
	}
	var b strings.Builder
	b.WriteByte('[')
	for _, f := range cells {
		switch {
		case f >= 0.99:
			b.WriteRune('█')
		case f >= 0.5:
			b.WriteRune('▓')
		case f > 0:
			b.WriteRune('▒')
		default:
			b.WriteRune('░')
		}
	}
	b.WriteByte(']')
	return b.String()
}
