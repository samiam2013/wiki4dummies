package main

import (
	"sync"
	"time"
)

type resultCache struct {
	*sync.Map
}

func newResultCache() *resultCache {
	return &resultCache{&sync.Map{}}
}

func (rc *resultCache) get(key string) (SearchPageData, bool) {
	val, ok := rc.Load(key)
	if !ok {
		return SearchPageData{}, false
	}
	return val.(SearchPageData), true
}

func (rc *resultCache) set(key string, val SearchPageData, dur time.Duration) {
	rc.Store(key, val)
	_ = time.AfterFunc(dur, func() {
		rc.Delete(key)
	})
}
