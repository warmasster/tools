package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFilterData(t *testing.T) {
	b := filterData()
	if binary.LittleEndian.Uint32(b) != 2 || binary.LittleEndian.Uint32(b[8:]) != 1 {
		t.Fatal("cabecera REGFILTER2 incorrecta")
	}
	if string(b[16:20]) != "0pi3" || string(b[44:48]) != "0ty3" {
		t.Fatal("firmas de patilla o tipo incorrectas")
	}
	off := func(at int) []byte { o := binary.LittleEndian.Uint32(b[at:]); return b[o : o+16] }
	// Los desplazamientos llevan a los GUID correctos.
	if !bytes.Equal(off(40), guidCapture) || !bytes.Equal(off(52), guidVideo) || !bytes.Equal(off(56), guidNull) {
		t.Fatal("los desplazamientos de FilterData no apuntan a sus GUID")
	}
	// MEDIATYPE_Video en memoria empieza por «vids» (igual que en los filtros de Windows).
	if string(guidVideo[:4]) != "vids" {
		t.Errorf("GUID mal convertido: % x", guidVideo)
	}
}

func TestParseHotkey(t *testing.T) {
	ok := map[string][2]uint32{
		"Ctrl+Alt+P":    {modCtrl | modAlt, 'P'},
		"Ctrl+Mayús+F9": {modCtrl | modShift, 0x78},
		"Win+Alt+1":     {modWin | modAlt, '1'},
		"Alt+Espacio":   {modAlt, 0x20},
	}
	for s, want := range ok {
		m, vk, err := parseHotkey(s)
		if err != nil || m != want[0] || vk != want[1] {
			t.Errorf("%s: %x %x %v", s, m, vk, err)
		}
	}
	for _, s := range []string{"P", "Ctrl+", "Ctrl+Alt+Ñ", "Hiper+P", "Ctrl+F25"} {
		if _, _, err := parseHotkey(s); err == nil {
			t.Errorf("%q debería fallar", s)
		}
	}
}

func TestAdjustNormalized(t *testing.T) {
	a := Adjust{Zoom: 50, Brightness: 9, Contrast: 0.1, Saturation: -1, Rotate: 45, Fit: "raro", Facing: "x"}.normalized()
	if a.Zoom != 5 || a.Brightness != 1.5 || a.Contrast != .5 || a.Saturation != 1 || a.Rotate != 0 || a.Fit != "cover" || a.Facing != "user" {
		t.Errorf("%+v", a)
	}
	// Los valores a cero (ajuste nuevo) se quedan en lo normal, no en el mínimo.
	z := Adjust{}.normalized()
	if z.Zoom != 1 || z.Brightness != 1 || z.Contrast != 1 {
		t.Errorf("%+v", z)
	}
}

func TestSceneNormalized(t *testing.T) {
	s := Scene{Kind: "?", PipSize: 3, PipCorner: "x", Region: Region{W: 10, H: 10}}.normalized()
	if s.Kind != "phone" || s.PipSize != .3 || s.PipCorner != "br" || s.Region.W != 0 {
		t.Errorf("%+v", s)
	}
	pip := Scene{Kind: "pip", Main: "screen", Pip: "abc"}
	if !pip.uses("screen") || !pip.uses("abc") || pip.uses("otro") {
		t.Error("uses no reconoce las fuentes de la escena")
	}
}

func TestCleanInputs(t *testing.T) {
	if cleanID("abc") != "" || cleanID("abcdefgh12") == "" || cleanID("abcdefgh/../x") != "" {
		t.Error("cleanID")
	}
	if got := cleanName("  Pixel\x00 8\n ", 60); got != "Pixel 8" {
		t.Errorf("cleanName = %q", got)
	}
}
