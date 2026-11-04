package stutter

import (
	"math/rand"
	"sort"
	"testing"
)

func TestPeriod(t *testing.T) {
	var times []uint32
	for k := uint32(1); k*30+7 < 900; k++ {
		times = append(times, k*30+7)
	}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 8; i++ {
		times = append(times, uint32(r.Intn(900)))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	p, fit := Period(times, 900)
	t.Logf("periodo %d, encaja %.0f %%", p, fit)
	if p != 30 && p != 29 && p != 31 {
		t.Fatalf("periodo %d", p)
	}
	var rnd []uint32
	for i := 0; i < 25; i++ {
		rnd = append(rnd, uint32(r.Intn(900)))
	}
	sort.Slice(rnd, func(i, j int) bool { return rnd[i] < rnd[j] })
	if p, _ := Period(rnd, 900); p != 0 {
		t.Fatalf("al azar ha salido periodo %d", p)
	}
}

// Simula una partida a 144 FPS en la que «fondo.exe» se dispara cada 20 s y
// justo entonces hay un tirón.
func TestAnalyzer(t *testing.T) {
	const freq = 10_000_000
	a := New(freq, 0)
	ts := int64(0)
	frame := int64(freq / 144)
	r := rand.New(rand.NewSource(2))
	for sec := 1; sec <= 600; sec++ {
		var ms []float32
		var tss []int64
		for ts+frame <= int64(sec)*freq {
			d := frame
			if sec%20 == 0 && len(ms) == 50 {
				d = freq * 45 / 1000 // tirón de 45 ms
			} else if r.Intn(4000) == 0 {
				d = freq * 30 / 1000 // alguno suelto
			}
			ts += d
			ms = append(ms, float32(float64(d)*1000/freq))
			tss = append(tss, ts)
		}
		a.Frames(ms, tss)
		ctx := Context{Procs: []Proc{{Name: "chrome.exe", CPU: 2 + r.Float64()}}}
		if sec%20 == 0 {
			ctx.Procs = append(ctx.Procs, Proc{Name: "fondo.exe", CPU: 6})
		} else if r.Intn(30) == 0 {
			ctx.Procs = append(ctx.Procs, Proc{Name: "fondo.exe", CPU: 0.6})
		}
		a.Second(int64(sec)*freq, ctx)
	}
	rep := a.Report()
	t.Logf("%+v", rep.Suspects)
	t.Logf("tirones %d (%d s con tirón) en %d s, %.1f/min, peor %.1f ms, periodo %d (%.0f %%)", rep.Hitches, rep.StutterSecs, rep.Secs, rep.PerMin, rep.Worst, rep.Period, rep.PeriodFit)
	if rep.Hitches < 29 {
		t.Fatalf("tirones %d", rep.Hitches)
	}
	if len(rep.Suspects) == 0 || rep.Suspects[0].Name != "fondo.exe" {
		t.Fatalf("culpable: %+v", rep.Suspects)
	}
	for _, s := range rep.Suspects {
		if s.Name == "chrome.exe" {
			t.Fatal("chrome no tiene la culpa")
		}
	}
	if rep.Period != 20 {
		t.Fatalf("periodo %d", rep.Period)
	}
}

func TestIsHitch(t *testing.T) {
	cases := []struct {
		ms, med float32
		want    bool
	}{{33, 16.7, false}, {34, 16.7, true}, {15, 6.9, true}, {12, 6.9, false}, {300, 150, false}}
	for _, c := range cases {
		if IsHitch(c.ms, c.med) != c.want {
			t.Errorf("%v sobre %v", c.ms, c.med)
		}
	}
}
