package auth

import (
	"sync"
	"time"
)

var (
	// Global token cache instance (initialized lazily)
	globalTokenCache *TokenCache
	tokenCacheOnce   sync.Once

	// Global token client instance (initialized lazily)
	globalTokenClient *TokenClient
	tokenClientOnce   sync.Once
)

// GetTokenCache returns the global token cache instance (initialized lazily)
func GetTokenCache() *TokenCache {
	tokenCacheOnce.Do(func() {
		globalTokenCache = NewTokenCache(1000) // Default size: 1000 entries
		// Start cleanup goroutine (runs every 5 minutes)
		globalTokenCache.StartCleanup(5 * time.Minute)
	})
	return globalTokenCache
}

// GetTokenClient returns the global token client instance (initialized lazily)
func GetTokenClient() *TokenClient {
	tokenClientOnce.Do(func() {
		globalTokenClient = NewTokenClient()
	})
	return globalTokenClient
}

// TokenCache provides in-memory caching for Azure tokens
type TokenCache struct {
	mu      sync.RWMutex
	cache   map[string]*cacheEntry
	maxSize int
}

type cacheEntry struct {
	token      string
	expiresAt  time.Time
	lastAccess time.Time
}

// NewTokenCache creates a new token cache
func NewTokenCache(maxSize int) *TokenCache {
	if maxSize <= 0 {
		maxSize = 1000 // Default size
	}
	return &TokenCache{
		cache:   make(map[string]*cacheEntry),
		maxSize: maxSize,
	}
}

// Get retrieves a token from cache
func (c *TokenCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return "", false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Don't delete here (let cleanup handle it)
		return "", false
	}

	entry.lastAccess = time.Now()
	return entry.token, true
}

// Set stores a token in cache
func (c *TokenCache) Set(key string, token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if cache is full (LRU)
	if len(c.cache) >= c.maxSize {
		c.evictLRU()
	}

	c.cache[key] = &cacheEntry{
		token:      token,
		expiresAt:  expiresAt,
		lastAccess: time.Now(),
	}
}

// evictLRU removes least recently used entry
func (c *TokenCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.cache {
		if oldestKey == "" || entry.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastAccess
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// Cleanup removes expired entries
func (c *TokenCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			delete(c.cache, key)
		}
	}
}

// StartCleanup starts a background goroutine that periodically cleans up expired entries
func (c *TokenCache) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			c.Cleanup()
		}
	}()
}
