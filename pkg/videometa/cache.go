package videometa

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

type CacheKeyInput struct {
	URL           string
	ETag          string
	LastModified  string
	ContentLength int64
}

func CacheKey(input CacheKeyInput) string {
	payload := input.URL + "\x00" + input.ETag + "\x00" + input.LastModified + "\x00" + strconv.FormatInt(input.ContentLength, 10)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

type cacheEntry struct {
	key       string
	metadata  Metadata
	expiresAt time.Time
}

type Cache struct {
	mutex    sync.Mutex
	capacity int
	entries  map[string]*list.Element
	order    *list.List
	now      func() time.Time
}

func NewCache(capacity int) *Cache {
	return newCache(capacity, time.Now)
}

func newCache(capacity int, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{
		capacity: capacity,
		entries:  make(map[string]*list.Element),
		order:    list.New(),
		now:      now,
	}
}

func (c *Cache) Get(key string) (Metadata, bool) {
	if c == nil || c.capacity <= 0 {
		return Metadata{}, false
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return Metadata{}, false
	}
	entry := element.Value.(*cacheEntry)
	if !c.now().Before(entry.expiresAt) {
		c.remove(element)
		return Metadata{}, false
	}
	c.order.MoveToFront(element)
	return entry.metadata, true
}

func (c *Cache) Set(key string, metadata Metadata, ttl time.Duration) {
	if c == nil || c.capacity <= 0 || ttl <= 0 {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	expiresAt := c.now().Add(ttl)
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*cacheEntry)
		entry.metadata = metadata
		entry.expiresAt = expiresAt
		c.order.MoveToFront(element)
		return
	}
	element := c.order.PushFront(&cacheEntry{key: key, metadata: metadata, expiresAt: expiresAt})
	c.entries[key] = element
	for c.order.Len() > c.capacity {
		c.remove(c.order.Back())
	}
}

func (c *Cache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*cacheEntry)
	delete(c.entries, entry.key)
	c.order.Remove(element)
}
