package spackbundle

import "runtime"

func bundleFileParallelism(total int) int {
	if total < 2 {
		return 1
	}
	workers := runtime.GOMAXPROCS(0)*2 + 1
	return min(total, max(1, workers))
}
