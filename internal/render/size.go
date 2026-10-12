package render

import "strconv"

const sizeUnits = "KMGTPE"

// appendSize appends a byte count the way a person reads it: "512B", "1.5KiB", "2.1MiB".
func appendSize(dst []byte, n int) []byte {
	v := uint64(max(n, 0))
	if v < 1024 {
		return append(strconv.AppendUint(dst, v, 10), 'B')
	}
	unit, name := uint64(1024), 0
	// 1023.95 KiB rounds to 1024.0, which reads better as 1.0 MiB.
	for tenths(v, unit) >= 10240 && name < len(sizeUnits)-1 {
		unit *= 1024
		name++
	}
	t := tenths(v, unit)
	dst = strconv.AppendUint(dst, t/10, 10)
	return append(dst, '.', byte('0'+t%10), sizeUnits[name], 'i', 'B')
}

// tenths returns v in tenths of unit, rounded half up. The whole part goes first, so that v*10 cannot overflow.
func tenths(v, unit uint64) uint64 {
	return v/unit*10 + (v%unit*10+unit/2)/unit
}
