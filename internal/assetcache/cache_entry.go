package assetcache

import (
	"errors"
	"fmt"

	"github.com/lyonbrown4d/spack/internal/cachepolicy"
)

type Entry struct {
	Body       []byte
	Attachment any
}

func (c *Cache) GetOrLoadWithRequest(path string, request cachepolicy.MemoryRequest) ([]byte, bool, error) {
	entry, found, err := c.GetEntryWithRequest(path, request)
	if err != nil {
		return nil, false, err
	}
	return entry.Body, found, nil
}

func (c *Cache) GetCachedEntry(path string) (Entry, bool) {
	if !c.Enabled() {
		return Entry{}, false
	}

	entry, found := c.cache.Get(path)
	if !found || entry == nil {
		return Entry{}, false
	}
	return *entry, true
}

func (c *Cache) GetEntryWithRequest(path string, request cachepolicy.MemoryRequest) (Entry, bool, error) {
	if !c.Enabled() {
		return Entry{}, false, errors.New("memory cache is disabled")
	}

	if entry, found := c.cache.Get(path); found && entry != nil {
		c.addCounter(metricAssetCacheHits, 1)
		return *entry, true, nil
	}
	c.addCounter(metricAssetCacheMisses, 1)

	result, err := c.loadEntry(path, request)
	if err != nil {
		return Entry{}, false, err
	}
	return result.entry, result.found, nil
}

func (c *Cache) Attach(path string, request cachepolicy.MemoryRequest, attachment any) bool {
	if !c.Enabled() {
		return false
	}

	entry, found := c.cache.Get(path)
	if !found || entry == nil {
		return false
	}
	return c.storeEntry(path, Entry{Body: entry.Body, Attachment: attachment}, request)
}

func (c *Cache) loadEntry(path string, request cachepolicy.MemoryRequest) (cacheLoadResult, error) {
	value, err, _ := c.loader.Do(path, func() (any, error) {
		return c.loadEntryOnce(path, request)
	})
	if err != nil {
		return cacheLoadResult{}, fmt.Errorf("load cache entry: %w", err)
	}

	result, ok := value.(cacheLoadResult)
	if !ok {
		return cacheLoadResult{}, fmt.Errorf("unexpected cache load result %T", value)
	}
	return result, nil
}

func (c *Cache) loadEntryOnce(path string, request cachepolicy.MemoryRequest) (cacheLoadResult, error) {
	if entry, found := c.cache.Get(path); found && entry != nil {
		return cacheLoadResult{entry: *entry, found: true}, nil
	}

	entry, cached, err := c.readAndCachePath(path, request)
	if err != nil {
		c.addCounter(metricAssetCacheLoadErrors, 1)
		return cacheLoadResult{}, err
	}
	if cached {
		c.addCounter(metricAssetCacheFills, 1)
		c.addCounter(metricAssetCacheFillBytes, int64(len(entry.Body)))
	}
	return cacheLoadResult{entry: entry, found: false}, nil
}
