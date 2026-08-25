package monitor

func MergeProbeHits(prev, curr []uintptr) []uintptr {
	if len(curr) == 0 {
		return nil
	}
	if len(prev) == 0 {
		return append([]uintptr(nil), curr...)
	}
	have := make(map[uintptr]struct{}, len(prev))
	for _, a := range prev {
		have[a] = struct{}{}
	}
	out := make([]uintptr, 0, len(curr))
	for _, a := range curr {
		if _, ok := have[a]; ok {
			out = append(out, a)
		}
	}
	if len(out) > 0 {
		return out
	}
	return append([]uintptr(nil), curr...)
}
