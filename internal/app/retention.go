package app

import (
	"context"
	"sync"
	"time"

	"oubliette/internal/live"
	"oubliette/internal/store"
)

func purgePeriodically(storage *store.Store, hub *live.Hub, cfg Config) (stop func()) {
	purge(storage, hub, cfg)
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(cfg.PurgeEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				purge(storage, hub, cfg)
			}
		}
	}()
	return sync.OnceFunc(func() {
		close(done)
		<-stopped
	})
}

func purge(storage *store.Store, hub *live.Hub, cfg Config) {
	purged, err := storage.PurgeMessages(context.Background(), cfg.Now().Add(-cfg.AgeLimit))
	if err != nil {
		cfg.Log.Error("purging old Messages failed", "error", err)
		return
	}
	for _, message := range purged {
		hub.Removed(message.InboxID, message.ID)
	}
}
