// Package live fans new messages out to connected widgets.
//
// One process, one hub, in memory. That is a deliberate fit to the deployment:
// Anis runs as a single Go binary on a single box, so there is no second
// instance for a subscriber to be connected to. If the product ever runs more
// than one process this must move to something shared — and the failure would
// be silent (a visitor simply never sees a reply), so the constraint is
// written down here rather than discovered later.
package live

import (
	"sync"
	"sync/atomic"
)

// Event is something a widget needs to know about.
type Event struct {
	// Kind is "message" or "status".
	Kind string `json:"kind"`
	// ID of the message, so a client can discard a duplicate after reconnecting.
	ID string `json:"id,omitempty"`
	// Role of a message: user, assistant or human.
	Role string `json:"role,omitempty"`
	Text string `json:"text,omitempty"`
	// Outcome of an assistant message: answered, refused, blocked. Carried
	// through history so a replay does not lose why a reply was what it was —
	// the widget keys its handoff offer off a refusal, and without this a
	// reconnect silently removes the offer.
	Outcome string `json:"outcome,omitempty"`
	// Status of the conversation, for a status event.
	Status  string `json:"status,omitempty"`
	Created string `json:"created,omitempty"`
}

// bufferPerSubscriber is how far a slow client may fall behind before events
// are dropped for it.
//
// Dropping is the right failure: the alternative is blocking the publisher,
// which would make one visitor on a bad connection stall the agent's reply to
// everyone else. A client that misses events reconnects and re-reads history.
const bufferPerSubscriber = 16

// MaxSubscribersPerConversation bounds a single conversation.
//
// A visitor with several tabs open is normal; a few hundred connections on one
// conversation is someone holding sockets open against us.
const MaxSubscribersPerConversation = 8

// Hub routes events to the widgets watching each conversation.
type Hub struct {
	mu sync.RWMutex
	// conversation id -> set of subscriber channels
	subs map[string]map[chan Event]struct{}

	// Observability for the health of the fan-out. Dropped events are the
	// signal that a subscriber is too slow or a buffer is too small.
	dropped atomic.Int64
}

func New() *Hub {
	return &Hub{subs: map[string]map[chan Event]struct{}{}}
}

// ErrTooManySubscribers is returned when a conversation is at its connection
// limit.
type ErrTooManySubscribers struct{}

func (ErrTooManySubscribers) Error() string { return "live: too many subscribers" }

// Subscribe returns a channel of events for one conversation, and a function
// to release it.
//
// The unsubscribe function MUST be called — it is what closes the channel and
// removes the entry. A caller that forgets leaks a goroutine and a map entry
// per connection, which on a long-lived SSE endpoint is an outage measured in
// days rather than a bug.
func (h *Hub) Subscribe(conversationID string) (<-chan Event, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	set, ok := h.subs[conversationID]
	if !ok {
		set = map[chan Event]struct{}{}
		h.subs[conversationID] = set
	}
	if len(set) >= MaxSubscribersPerConversation {
		return nil, nil, ErrTooManySubscribers{}
	}

	ch := make(chan Event, bufferPerSubscriber)
	set[ch] = struct{}{}

	var once sync.Once
	release := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if set, ok := h.subs[conversationID]; ok {
				delete(set, ch)
				if len(set) == 0 {
					delete(h.subs, conversationID)
				}
			}
			close(ch)
		})
	}

	return ch, release, nil
}

// Publish delivers an event to everyone watching a conversation.
//
// Never blocks. A subscriber whose buffer is full is skipped rather than
// waited on, because this runs inside a PocketBase record hook — blocking here
// would hold a database write open behind the slowest browser connected.
func (h *Hub) Publish(conversationID string, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.subs[conversationID] {
		select {
		case ch <- ev:
		default:
			h.dropped.Add(1)
		}
	}
}

// Dropped reports how many events could not be delivered because a subscriber
// was too far behind. Non-zero means someone missed a reply until they
// reconnected.
func (h *Hub) Dropped() int64 { return h.dropped.Load() }

// Subscribers reports the number of connections on a conversation.
func (h *Hub) Subscribers(conversationID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[conversationID])
}

// Conversations reports how many conversations have at least one subscriber.
// A number that only ever grows means release is not being called.
func (h *Hub) Conversations() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
