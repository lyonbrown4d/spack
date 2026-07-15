package server

import "github.com/lyonbrown4d/spack/internal/config"

type preparedBodyBudget struct {
	max  int64
	used int64
}

func newPreparedBodyBudget(cfg *config.Config) *preparedBodyBudget {
	if cfg == nil || !cfg.HTTP.MemoryCache.Enabled() {
		return &preparedBodyBudget{}
	}
	return &preparedBodyBudget{max: cfg.HTTP.MemoryCache.MaxCost()}
}

func (b *preparedBodyBudget) Reserve(size int64) (int64, bool) {
	if b == nil || b.max <= 0 || size < 0 {
		return 0, false
	}
	reserved := normalizedPreparedBodyBudgetSize(size)
	if b.used > b.max-reserved {
		return 0, false
	}
	b.used += reserved
	return reserved, true
}

func (b *preparedBodyBudget) Release(size int64) {
	if b == nil || size <= 0 {
		return
	}
	b.used -= size
	if b.used < 0 {
		b.used = 0
	}
}

func (b *preparedBodyBudget) Adjust(reserved, actual int64) bool {
	if b == nil || reserved <= 0 {
		return false
	}
	actual = normalizedPreparedBodyBudgetSize(actual)
	if actual == reserved {
		return true
	}
	if actual < reserved {
		b.Release(reserved - actual)
		return true
	}
	extra := actual - reserved
	if b.used > b.max-extra {
		b.Release(reserved)
		return false
	}
	b.used += extra
	return true
}

func normalizedPreparedBodyBudgetSize(size int64) int64 {
	if size <= 0 {
		return 1
	}
	return size
}
