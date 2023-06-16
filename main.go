package heartbeat

import (
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	defaultExpiration = 5 * time.Minute
	cleanupInterval   = 10 * time.Second
)

type HeartbeatMiddleware struct {
	cache     sync.Map
	paths     []string
	heartbeat http.Handler
}

type heartbeatInfo struct {
	timestamp time.Time
	timer     *time.Timer
}

func NewHeartbeatMiddleware(paths []string, heartbeat http.Handler) *HeartbeatMiddleware {
	middleware := &HeartbeatMiddleware{
		cache:     sync.Map{},
		paths:     paths,
		heartbeat: heartbeat,
	}
	go middleware.cleanupExpiredEntries()
	return middleware
}

func (m *HeartbeatMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.Handler) {
	path := r.URL.Path

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if contains(m.paths, path) {
		entry, ok := m.cache.Load(path)
		if !ok {
			entry = m.addNewEntry(path)
		}

		info := entry.(*heartbeatInfo)
		info.timestamp = time.Now()
		info.timer.Reset(defaultExpiration)
	}

	m.heartbeat.ServeHTTP(w, r)
	next.ServeHTTP(w, r)
}

func (m *HeartbeatMiddleware) addNewEntry(path string) *heartbeatInfo {
	info := &heartbeatInfo{
		timestamp: time.Now(),
		timer:     time.NewTimer(defaultExpiration),
	}
	go func() {
		<-info.timer.C
		log.Printf("Missed heartbeat for path %s\n", path)
		// send email, SMS, or other notification to alert someone of the missed heartbeat
		m.cache.Delete(path)
	}()

	m.cache.Store(path, info)
	return info
}

func (m *HeartbeatMiddleware) cleanupExpiredEntries() {
	ticker := time.NewTicker(cleanupInterval)
	for range ticker.C {
		m.cache.Range(func(key, value interface{}) bool {
			info := value.(*heartbeatInfo)
			if time.Since(info.timestamp) > defaultExpiration {
				log.Printf("Removing expired entry for path %s\n", key)
				m.cache.Delete(key)
			}
			return true
		})
	}
}

func contains(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}
