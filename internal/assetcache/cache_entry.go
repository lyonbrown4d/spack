package assetcache

import (
	"errors"

	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/samber/oops"
)

type Entry struct {
	Body       []byte
	Attachment any
}

type entryLoadOptions struct {
	attachment        any
	attachmentMatches func(any) bool
	wait              bool
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
	return c.getEntryWithRequest(path, request, entryLoadOptions{wait: true})
}

func (c *Cache) GetEntryForServe(
	path string,
	request cachepolicy.MemoryRequest,
	attachment any,
	attachmentMatches func(any) bool,
) (Entry, bool, error) {
	return c.getEntryWithRequest(path, request, entryLoadOptions{
		attachment:        attachment,
		attachmentMatches: attachmentMatches,
	})
}

func (c *Cache) getEntryWithRequest(path string, request cachepolicy.MemoryRequest, options entryLoadOptions) (Entry, bool, error) {
	if !c.Enabled() {
		return Entry{}, false, oops.In("assetcache").Owner("entry").Wrap(errors.New("memory cache is disabled"))
	}

	if entry, found := c.cache.Get(path); found && entry != nil {
		c.addCounter(metricAssetCacheHits, 1)
		return c.ensureAttachment(path, request, *entry, options), true, nil
	}
	c.addCounter(metricAssetCacheMisses, 1)

	result, err := c.loadEntry(path, request, options)
	if err != nil {
		return Entry{}, false, err
	}
	entry := c.ensureAttachment(path, request, result.entry, options)
	return entry, result.found, nil
}

func (c *Cache) ensureAttachment(path string, request cachepolicy.MemoryRequest, entry Entry, options entryLoadOptions) Entry {
	if options.attachment == nil || attachmentMatches(entry.Attachment, options) {
		return entry
	}
	entry.Attachment = options.attachment
	c.storeEntry(path, entry, request, false)
	return entry
}

func attachmentMatches(existing any, options entryLoadOptions) bool {
	if options.attachmentMatches != nil {
		return options.attachmentMatches(existing)
	}
	return existing != nil
}

func (c *Cache) Attach(path string, request cachepolicy.MemoryRequest, attachment any) bool {
	if !c.Enabled() {
		return false
	}

	entry, found := c.cache.Get(path)
	if !found || entry == nil {
		return false
	}
	return c.storeEntry(path, Entry{Body: entry.Body, Attachment: attachment}, request, true)
}

func (c *Cache) loadEntry(path string, request cachepolicy.MemoryRequest, options entryLoadOptions) (cacheLoadResult, error) {
	value, err, _ := c.loader.Do(path, func() (any, error) {
		return c.loadEntryOnce(path, request, options)
	})
	if err != nil {
		return cacheLoadResult{}, oops.Wrapf(err, "load cache entry")
	}

	result, ok := value.(cacheLoadResult)
	if !ok {
		return cacheLoadResult{}, oops.Errorf("unexpected cache load result %T", value)
	}
	return result, nil
}

func (c *Cache) loadEntryOnce(path string, request cachepolicy.MemoryRequest, options entryLoadOptions) (cacheLoadResult, error) {
	if entry, found := c.cache.Get(path); found && entry != nil {
		return cacheLoadResult{entry: *entry, found: true}, nil
	}

	entry, cached, err := c.readAndCachePath(path, request, options.attachment, options.wait)
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
