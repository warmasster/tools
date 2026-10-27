package main

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestProtocolMatchesHeader comprueba que shm.go y camera/protocol.h dicen lo mismo:
// si no, la cámara leería basura.
func TestProtocolMatchesHeader(t *testing.T) {
	f, err := os.Open("camera/protocol.h")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defs := map[string]string{}
	re := regexp.MustCompile(`^#define\s+(TC_\w+)\s+(.+?)\s*(/\*.*)?$`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			defs[m[1]] = m[2]
		}
	}
	num := func(name string) int {
		v, ok := defs[name]
		if !ok {
			t.Fatalf("falta %s en protocol.h", name)
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 0, 64)
		if err != nil {
			t.Fatalf("%s no es un número: %q", name, v)
		}
		return int(n)
	}
	str := func(name string) string {
		v := defs[name]
		v = strings.TrimPrefix(v, "L")
		u, err := strconv.Unquote(v)
		if err != nil {
			t.Fatalf("%s no es una cadena: %q", name, v)
		}
		return u
	}

	if got := str("TC_MAP_NAME"); got != `Local\TurboCAM.Video.1` {
		t.Errorf("TC_MAP_NAME = %q", got)
	}
	if got := str("TC_MAP_SDDL"); got != tcMapSDDL {
		t.Errorf("TC_MAP_SDDL = %q, Go = %q", got, tcMapSDDL)
	}
	checks := map[string]int{
		"TC_MAGIC": tcMagic, "TC_VERSION": tcVersion, "TC_MAX_W": tcMaxW, "TC_MAX_H": tcMaxH,
		"TC_SLOTS": tcSlots, "TC_MAX_CONSUMERS": tcMaxConsumers,
		"TC_STATE_OFF": tcStateOff, "TC_STATE_WAITING": tcStateWaiting, "TC_STATE_LIVE": tcStateLive,
		"TC_STATE_PAUSED": tcStatePaused, "TC_STATE_COVER": tcStateCover,
		"TC_HEARTBEAT_TIMEOUT_MS": tcHeartbeatMsec, "TC_CONSUMER_TIMEOUT_MS": tcConsumerTimeoutMs,
		"TC_OFF_MAGIC": tcOffMagic, "TC_OFF_VERSION": tcOffVersion, "TC_OFF_STATE": tcOffState,
		"TC_OFF_LATEST": tcOffLatest, "TC_OFF_HEARTBEAT": tcOffHeartbeat, "TC_OFF_FPS": tcOffFps,
		"TC_OFF_WIDTH": tcOffWidth, "TC_OFF_HEIGHT": tcOffHeight,
		"TC_CONS_BASE": tcConsBase, "TC_CONS_SIZE": tcConsSize, "TC_CONS_PID": tcConsPid,
		"TC_CONS_SEEN": tcConsSeen, "TC_CONS_WIDTH": tcConsWidth, "TC_CONS_HEIGHT": tcConsHeight,
		"TC_CONS_FPS": tcConsFps, "TC_CONS_EXE": tcConsExe, "TC_CONS_EXE_CHARS": tcConsExeChars,
		"TC_SLOT_BASE": tcSlotBase, "TC_SLOT_SEQ": tcSlotSeq, "TC_SLOT_WIDTH": tcSlotWidth,
		"TC_SLOT_HEIGHT": tcSlotHeight, "TC_SLOT_DATA": tcSlotData, "TC_SLOT_STRIDE": tcSlotStride,
	}
	for name, want := range checks {
		if got := num(name); got != want {
			t.Errorf("%s: protocol.h = %d, Go = %d", name, got, want)
		}
	}

	// Coherencia interna del formato.
	if tcConsExe+2*tcConsExeChars > tcConsSize {
		t.Error("el nombre del ejecutable se sale de su ranura de consumidor")
	}
	if tcConsBase+tcMaxConsumers*tcConsSize > tcSlotBase {
		t.Error("los consumidores pisan las ranuras de imagen")
	}
	if tcSlotStride < tcSlotData+tcFrameBytes || tcSlotStride%4096 != 0 {
		t.Error("TC_SLOT_STRIDE no cabe un fotograma o no está alineado a 4 KB")
	}
}
