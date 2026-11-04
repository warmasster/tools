package netmon

import (
	"net/netip"
	"testing"
	"time"
	"unsafe"

	"turbomon/internal/etw"
)

func fakeRecord(id uint16, pid, size uint32, d, s [4]byte, dport, sport uint16) *etw.Record {
	b := make([]byte, 20)
	*(*uint32)(unsafe.Pointer(&b[0])) = pid
	*(*uint32)(unsafe.Pointer(&b[4])) = size
	copy(b[8:], d[:])
	copy(b[12:], s[:])
	b[16], b[17] = byte(dport>>8), byte(dport)
	b[18], b[19] = byte(sport>>8), byte(sport)
	r := &etw.Record{UserData: unsafe.Pointer(&b[0]), UserDataLength: uint16(len(b))}
	r.Header.ID = id
	return r
}

func TestHandleAndServer(t *testing.T) {
	m := New()
	server := [4]byte{185, 50, 104, 12}
	local := [4]byte{192, 168, 1, 20}
	for i := 0; i < 50; i++ {
		m.Handle(fakeRecord(42, 1234, 120, server, local, 27015, 50000)) // envía al servidor
		m.Handle(fakeRecord(43, 1234, 300, local, server, 50000, 27015)) // recibe del servidor
	}
	m.Handle(fakeRecord(11, 99, 1<<20, local, [4]byte{151, 101, 1, 1}, 50001, 443)) // otra descarga
	srv, ok := m.GameServer(1234)
	if !ok || srv.Addr != netip.MustParseAddrPort("185.50.104.12:27015") || !srv.UDP {
		t.Fatalf("servidor: %+v %v", srv, ok)
	}
	time.Sleep(10 * time.Millisecond)
	rates := m.Rates()
	if len(rates) != 2 || rates[0].PID != 99 {
		t.Fatalf("tasas: %+v", rates)
	}
}

func TestSeries(t *testing.T) {
	s := NewSeries(10)
	for _, v := range []float64{20, 22, 21, 80, 20} {
		s.Add(v, true)
	}
	s.Add(0, false)
	st := s.Stats()
	if st.Avg < 32 || st.Avg > 33 || st.Loss < 16 || st.Loss > 17 || st.P95 != 80 {
		t.Fatalf("%+v", st)
	}
}

// Ping de verdad a 1.1.1.1 (si hay internet).
func TestPingReal(t *testing.T) {
	if testing.Short() {
		t.Skip("prueba corta")
	}
	d, err := Ping(netip.MustParseAddr("1.1.1.1"), time.Second)
	t.Logf("ping 1.1.1.1: %v %v", d, err)
}
