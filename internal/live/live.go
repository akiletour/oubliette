package live

import "sync"

const backlog = 64

type Event struct {
	Name      string
	MessageID string
}

type Hub struct {
	mu        sync.Mutex
	listeners map[string]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{listeners: map[string]map[chan Event]struct{}{}}
}

func (h *Hub) Listen(inboxID string) (events <-chan Event, stop func()) {
	listener := make(chan Event, backlog)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.listeners[inboxID] == nil {
		h.listeners[inboxID] = map[chan Event]struct{}{}
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
	h.notify(inboxID, Event{Name: "received", MessageID: messageID})
}

func (h *Hub) Removed(inboxID, messageID string) {
	h.notify(inboxID, Event{Name: "removed", MessageID: messageID})
}

func (h *Hub) Emptied(inboxID string) {
	h.notify(inboxID, Event{Name: "emptied"})
}

func (h *Hub) ScopeChanged(inboxID string) {
	h.notify(inboxID, Event{})
}

func (h *Hub) notify(inboxID string, event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for listener := range h.listeners[inboxID] {
		select {
		case listener <- event:
		default:
		}
	}
}
