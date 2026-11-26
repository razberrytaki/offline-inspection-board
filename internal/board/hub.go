package board

import "sync"

type Event struct {
	ID       string
	RootID   string
	Revision int32
}

type Hub struct {
	mu          sync.Mutex
	subscribers map[chan Event]struct{}
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscribers == nil {
		h.subscribers = make(map[chan Event]struct{})
	}
	events := make(chan Event, 64)
	h.subscribers[events] = struct{}{}
	return events, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, subscribed := h.subscribers[events]; subscribed {
			delete(h.subscribers, events)
			close(events)
		}
	}
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for events := range h.subscribers {
		select {
		case events <- event:
		default:
			// 적체된 구독을 닫아 저장을 막지 않고 구독자가 다시 조회할 수 있게 한다.
			delete(h.subscribers, events)
			close(events)
		}
	}
}
