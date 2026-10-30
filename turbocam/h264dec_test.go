package main

import (
	"bytes"
	"crypto/md5"
	"os"
	"runtime"
	"testing"

	"github.com/pion/ice/v4"
)

// splitAnnexB parte un flujo H.264 en unidades de acceso (fotogramas): cada una
// empieza en el primer NAL que no sea de corte (SPS, PPS, SEI, AUD) tras un corte.
func splitAnnexB(b []byte) [][]byte {
	var starts []int
	for i := 0; i+3 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			s := i
			if i > 0 && b[i-1] == 0 {
				s = i - 1
			}
			starts = append(starts, s)
			i += 2
		}
	}
	var out [][]byte
	cur, seenSlice := -1, false
	for _, s := range starts {
		j := s + 3
		if b[s+2] == 0 {
			j = s + 4
		}
		t := b[j] & 0x1f
		isSlice := t == 1 || t == 5
		if cur < 0 {
			cur = s
		} else if seenSlice && (!isSlice || b[j+1]&0x80 != 0) {
			out = append(out, b[cur:s])
			cur, seenSlice = s, false
		}
		if isSlice {
			seenSlice = true
		}
	}
	if cur >= 0 {
		out = append(out, b[cur:])
	}
	return out
}

func decodeFile(t *testing.T, name string) []*imageInfo {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	comInitThread()
	d, err := newH264Decoder()
	if err != nil {
		// Windows Server (el de GitHub Actions) puede no traer Media Foundation.
		t.Skipf("sin decodificador H.264 en este Windows: %v", err)
	}
	defer d.close()
	var got []*imageInfo
	for _, au := range splitAnnexB(data) {
		imgs, err := d.decode(au)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range imgs {
			got = append(got, &imageInfo{m.Rect.Dx(), m.Rect.Dy(), m.Y[100*m.YStride+45], m.Cb[50*m.CStride+22], m.Cr[50*m.CStride+22],
				m.Y[100*m.YStride+137], m.Cb[50*m.CStride+68], m.Cr[50*m.CStride+68]})
		}
	}
	return got
}

type imageInfo struct {
	w, h         int
	y0, cb0, cr0 byte
	y1, cb1, cr1 byte
}

func near(a, b byte) bool { return int(a)-int(b) < 6 && int(b)-int(a) < 6 }

func TestH264Decode(t *testing.T) {
	got := decodeFile(t, "360p.h264")
	if len(got) != 20 {
		t.Fatalf("salen %d fotogramas, quería 20 (uno por cada uno que entra)", len(got))
	}
	f := got[0]
	if f.w != 640 || f.h != 360 {
		t.Fatalf("tamaño %dx%d, quería 640x360", f.w, f.h)
	}
	// Barras de color: gris (180,128,128) y amarillo (162,44,142), según ffmpeg.
	if !near(f.y0, 180) || !near(f.cb0, 128) || !near(f.cr0, 128) || !near(f.y1, 162) || !near(f.cb1, 44) || !near(f.cr1, 142) {
		t.Errorf("colores raros: %+v", *f)
	}
}

func TestH264DecodeCrops1080(t *testing.T) {
	got := decodeFile(t, "1080p.h264")
	if len(got) != 3 {
		t.Fatalf("salen %d fotogramas, quería 3", len(got))
	}
	if got[0].w != 1920 || got[0].h != 1080 {
		t.Fatalf("tamaño %dx%d, quería 1920x1080 (sin el relleno hasta 1088)", got[0].w, got[0].h)
	}
}

func TestSplitAnnexB(t *testing.T) {
	data, _ := os.ReadFile("testdata/360p.h264")
	aus := splitAnnexB(data)
	if len(aus) != 20 || !bytes.Equal(bytes.Join(aus, nil), data) {
		t.Fatalf("%d unidades", len(aus))
	}
}

func TestStartBitrateInAnswer(t *testing.T) {
	sdp := "v=0\r\na=rtpmap:102 H264/90000\r\na=fmtp:102 packetization-mode=1;profile-level-id=42e01f\r\na=rtpmap:103 rtx/90000\r\na=fmtp:103 apt=102\r\n"
	got := withStartBitrate(sdp, 2000)
	if !bytes.Contains([]byte(got), []byte("profile-level-id=42e01f;x-google-start-bitrate=2000\r\n")) || bytes.Contains([]byte(got), []byte("apt=102;x-google")) {
		t.Fatalf("SDP:\n%s", got)
	}
	if !h264IsKey([]byte{0, 0, 0, 1, 0x67, 1, 0, 0, 1, 0x65, 2}) || h264IsKey([]byte{0, 0, 1, 0x41, 9}) {
		t.Fatal("h264IsKey")
	}
}

// TestH264Moves: cada fotograma decodificado es distinto (con un vídeo en movimiento).
// Si el decodificador reutiliza la memoria de salida, sale siempre el primero.
func TestH264Moves(t *testing.T) {
	data, err := os.ReadFile("testdata/mov.h264")
	if err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	comInitThread()
	d, err := newH264Decoder()
	if err != nil {
		t.Skipf("sin decodificador H.264 en este Windows: %v", err)
	}
	defer d.close()
	seen := map[[16]byte]bool{}
	n := 0
	for _, au := range splitAnnexB(data) {
		imgs, err := d.decode(au)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range imgs {
			seen[md5.Sum(m.Y)] = true
			n++
		}
	}
	if n != 30 || len(seen) < 28 {
		t.Fatalf("%d fotogramas, %d distintos: la imagen se queda congelada", n, len(seen))
	}
}

func TestPublicCandidate(t *testing.T) {
	c := publicCandidate("92.59.1.2", 47852, 47852)
	cand, err := ice.UnmarshalCandidate(c)
	if err != nil {
		t.Fatalf("%s: %v", c, err)
	}
	if cand.Type() != ice.CandidateTypeServerReflexive || cand.Address() != "92.59.1.2" || cand.Port() != 47852 {
		t.Fatalf("candidato raro: %s", cand)
	}
	if m := sdpMid("v=0\r\nm=video 9 UDP/TLS/RTP/SAVPF 102\r\na=mid:0\r\n"); m != "0" {
		t.Fatalf("mid %q", m)
	}
}
