package live

import "sync"

const backlog = 64

type Hub struct {
	mu        sync.Mutex
	listeners map[string]map[chan string]struct{}
}

func NewHub() *Hub {
	return &Hub{listeners: map[string]map[chan string]struct{}{}}
}

func (h *Hub) Listen(inboxID string) (received <-chan string, stop func()) {
	listener := make(chan string, backlog)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.listeners[inboxID] == nil {
		h.listeners[inboxID] = map[chan string]struct{}{}
	}
	h.listeners[inboxID][listener] = struct{}{}
	return listener, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.listeners[inboxID], listener)
		if len(h.listeners[inboxID]) == 0 {
			delete(h.listeners, inboxID)
		}
	}
}

func (h *Hub) Received(inboxID, messageID string) {
	h.notify(inboxID, messageID)
}

func (h *Hub) ScopeChanged(inboxID string) {
	h.notify(inboxID, "")
}

func (h *Hub) notify(inboxID, messageID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for listener := range h.listeners[inboxID] {
		select {
		case listener <- messageID:
		default:
		}
	}
}
