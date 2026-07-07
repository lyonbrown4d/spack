package assetcache

import "github.com/prometheus/client_golang/prometheus"

func (c *Cache) Collectors() []prometheus.Collector {
	if c == nil {
		return nil
	}
	return []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "spack_asset_cache_entries_current",
			Help: "Current number of entries visible in the in-memory asset cache",
		}, func() float64 {
			return float64(c.EntryCount())
		}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "spack_asset_cache_bytes_current",
			Help: "Current byte cost used by the in-memory asset cache",
		}, func() float64 {
			return float64(c.CurrentBytes())
		}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "spack_asset_cache_max_bytes",
			Help: "Maximum byte cost configured for the in-memory asset cache",
		}, func() float64 {
			return float64(c.MaxBytes())
		}),
	}
}

func (c *Cache) EntryCount() int {
	if !c.Enabled() {
		return 0
	}
	count := 0
	c.cache.IterValues(func(entry *Entry) bool {
		if entry != nil {
			count++
		}
		return false
	})
	return count
}

func (c *Cache) CurrentBytes() int64 {
	if !c.Enabled() {
		return 0
	}
	used := c.cache.MaxCost() - c.cache.RemainingCost()
	if used < 0 {
		return 0
	}
	return used
}

func (c *Cache) MaxBytes() int64 {
	if !c.Enabled() {
		return 0
	}
	return c.cache.MaxCost()
}
