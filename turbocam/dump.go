package main

// Diagnóstico: con TURBOCAM_DUMP=carpeta, se guarda el H.264 tal cual llega de cada
// móvil (los primeros 30 MB por conexión) y un registro con el formato del
// codificador del móvil y del decodificador del PC. Sirve para ver con ffmpeg si un
// vídeo raro viene así del móvil o lo estropea el PC.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var dumpDir = os.Getenv("TURBOCAM_DUMP")

var dumpMu sync.Mutex
var dumpFiles = map[string]*os.File{}
var dumpSize = map[string]int{}

func dumpH264(id string, rtc bool, data []byte) {
	if dumpDir == "" {
		return
	}
	name := id + "-ws.h264"
	if rtc {
		name = id + "-rtc.h264"
	}
	dumpMu.Lock()
	defer dumpMu.Unlock()
	f := dumpFiles[name]
	if f == nil {
		os.MkdirAll(dumpDir, 0o755)
		var err error
		if f, err = os.Create(filepath.Join(dumpDir, name)); err != nil {
			return
		}
		dumpFiles[name] = f
	}
	if dumpSize[name] < 30<<20 {
		f.Write(data)
		dumpSize[name] += len(data)
	}
}

func dumpLog(id, line string) {
	if dumpDir == "" {
		return
	}
	dumpMu.Lock()
	defer dumpMu.Unlock()
	os.MkdirAll(dumpDir, 0o755)
	f, err := os.OpenFile(filepath.Join(dumpDir, id+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), line)
	f.Close()
}
