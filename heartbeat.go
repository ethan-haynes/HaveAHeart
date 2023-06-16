package heartbeat

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	defaultExpiration = 5 * time.Minute
	cleanupInterval   = 10 * time.Second
)

var (
	cache            sync.Map
	cleanupOnce      sync.Once
	cleanupWg        sync.WaitGroup
	cleanupStopChan  chan struct{}
)

type heartbeatInfo struct {
	timestamp time.Time
	timer     *time.Timer
}

func HandleHeartbeatRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entry, ok := cache.Load(path)
	if !ok {
		entry = addNewEntry(path)
	}

	info := entry.(*heartbeatInfo)
	info.timestamp = time.Now()
	info.timer.Reset(defaultExpiration)

	w.WriteHeader(http.StatusOK)
}

func addNewEntry(path string) *heartbeatInfo {
	info := &heartbeatInfo{
		timestamp: time.Now(),
		timer:     time.NewTimer(defaultExpiration),
	}
	go func() {
		<-info.timer.C
		log.Printf("Missed heartbeat for path %s\n", path)
		// send email, SMS, or other notification to alert someone of the missed heartbeat
		cache.Delete(path)
	}()

	cache.Store(path, info)

	return info
}

func cleanupExpiredEntries() {
	ticker := time.NewTicker(cleanupInterval)
	for {
		select {
		case <-ticker.C:
			deletedPaths := make([]string, 0) // To store expired paths for deletion

			cache.Range(func(key, value interface{}) bool {
				info := value.(*heartbeatInfo)
				if time.Since(info.timestamp) > defaultExpiration {
					log.Printf("Removing expired entry for path %s\n", key)
					deletedPaths = append(deletedPaths, key.(string))
				}
				return true
			})

			// Delete the expired paths outside of the cache.Range loop
			for _, path := range deletedPaths {
				cache.Delete(path)
			}

		case <-cleanupStopChan:
			ticker.Stop()
			cleanupWg.Done()
			return
		}

		cleanupWg.Done()
	}
}

func init() {
	cleanupOnce.Do(func() {
		cleanupWg.Add(1)
		cleanupStopChan = make(chan struct{})
		go cleanupExpiredEntries()
	})

	go handleTerminationSignal()
}

func handleTerminationSignal() {
	terminationChan := make(chan os.Signal, 1)
	signal.Notify(terminationChan, os.Interrupt, syscall.SIGTERM)
	<-terminationChan

	// Call PerformCleanup when termination signal is received
	PerformCleanup()

	// Close the cleanupStopChan to terminate the cleanup loop
	close(cleanupStopChan)

	// Terminate the program
	os.Exit(0)
}

// PerformCleanup performs the cleanup process and waits for its completion.
// It should be called when the server is about to exit.
func PerformCleanup() {
	cleanupWg.Wait()
}
