package noise

// En spin_amd64.s.
func spin(dur, thr1, thr2, thr3 uint64) (stolen, maxGap, n1, n2, n3 uint64)
func tsc() uint64
