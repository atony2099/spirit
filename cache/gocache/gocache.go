package gocache

import (
	"time"

	"github.com/go-spirit/spirit/cache"
	gocache "github.com/patrickmn/go-cache"
)

type GoCache struct {
	gocache *gocache.Cache
}

func init() {
	cache.RegisterCache("go-cache", NewGoCache)
}

func NewGoCache(opts ...cache.Option) (c cache.Cache, err error) {

	cacheOpts := cache.Options{}

	for _, o := range opts {
		o(&cacheOpts)
	}

	var defaultExpiration, cleanupInterval time.Duration

	if cacheOpts.Config != nil {
		defaultExpiration = cacheOpts.Config.GetTimeDuration("expiration", gocache.DefaultExpiration)
		cleanupInterval = cacheOpts.Config.GetTimeDuration("cleanup-interval", 10*time.Minute)
	} else {
		defaultExpiration = gocache.DefaultExpiration
		cleanupInterval = 10 * time.Minute
	}

	goCache := &GoCache{
		gocache: gocache.New(defaultExpiration, cleanupInterval),
	}

	c = goCache

	return
}

func (p *GoCache) Set(k string, v interface{}) {
	p.gocache.SetDefault(k, v)
	return
}

func (p *GoCache) Get(k string) (interface{}, bool) {
	return p.gocache.Get(k)
}

func (p *GoCache) Delete(k string) {
	p.gocache.Delete(k)
}

func (p *GoCache) Increment(k string, delta int64) (v int64, err error) {
	v, err = p.gocache.IncrementInt64(k, delta)
	return
}

func (p *GoCache) Decrement(k string, delta int64) (v int64, err error) {
	v, err = p.gocache.DecrementInt64(k, delta)
	return
}

func (p *GoCache) Flush() {
	p.gocache.Flush()
}

func (p *GoCache) IsLocal() bool {
	return true
}

func (p *GoCache) CanStoreInterface() bool {
	return true
}
