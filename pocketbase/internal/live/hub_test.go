package live

import (
	"sync"
	"testing"
	"time"
)

func recv(t *testing.T, ch <-chan Event) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for an event")
		return Event{}, false
	}
}

func TestDeliversToSubscribers(t *testing.T) {
	h := New()
	ch, release, err := h.Subscribe("c1")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	h.Publish("c1", Event{Kind: "message", Role: "human", Text: "أهلاً"})

	ev, _ := recv(t, ch)
	if ev.Role != "human" || ev.Text != "أهلاً" {
		t.Errorf("got %+v", ev)
	}
}

func TestScopedToOneConversation(t *testing.T) {
	h := New()
	a, releaseA, _ := h.Subscribe("c1")
	defer releaseA()
	b, releaseB, _ := h.Subscribe("c2")
	defer releaseB()

	h.Publish("c1", Event{Kind: "message", Text: "for c1"})

	if ev, _ := recv(t, a); ev.Text != "for c1" {
		t.Errorf("c1 subscriber got %q", ev.Text)
	}
	// A conversation's events must never reach another conversation's widget —
	// that would be one visitor reading another's chat.
	select {
	case ev := <-b:
		t.Errorf("c2 subscriber received an event for c1: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEveryoneOnAConversationGetsIt(t *testing.T) {
	h := New()
	// A visitor with two tabs open is ordinary.
	a, ra, _ := h.Subscribe("c1")
	defer ra()
	b, rb, _ := h.Subscribe("c1")
	defer rb()

	h.Publish("c1", Event{Kind: "message", Text: "hello"})

	for i, ch := range []<-chan Event{a, b} {
		if ev, _ := recv(t, ch); ev.Text != "hello" {
			t.Errorf("subscriber %d got %q", i, ev.Text)
		}
	}
}

// Publish runs inside a PocketBase record hook. Blocking there would hold a
// database write open behind the slowest browser connected.
func TestPublishNeverBlocksOnASlowSubscriber(t *testing.T) {
	h := New()
	_, release, _ := h.Subscribe("c1")
	defer release()

	done := make(chan struct{})
	go func() {
		// Far more than the buffer, with nothing reading.
		for range bufferPerSubscriber * 10 {
			h.Publish("c1", Event{Kind: "message", Text: "x"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a subscriber that was not reading")
	}

	if h.Dropped() == 0 {
		t.Error("expected dropped events to be counted, so a slow client is visible")
	}
}

func TestReleaseRemovesTheSubscriber(t *testing.T) {
	h := New()
	ch, release, _ := h.Subscribe("c1")

	if got := h.Subscribers("c1"); got != 1 {
		t.Fatalf("subscribers = %d, want 1", got)
	}
	release()

	if got := h.Subscribers("c1"); got != 0 {
		t.Errorf("subscribers = %d after release, want 0", got)
	}
	// The map entry must go too, or a busy site leaks one entry per
	// conversation forever.
	if got := h.Conversations(); got != 0 {
		t.Errorf("conversations = %d after release, want 0", got)
	}
	if _, open := <-ch; open {
		t.Error("channel should be closed after release")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	h := New()
	_, release, _ := h.Subscribe("c1")
	release()
	// A second call must not panic on a closed channel. Deferred cleanup plus
	// an explicit close on an error path makes double-release realistic.
	release()
}

func TestPublishAfterReleaseIsSafe(t *testing.T) {
	h := New()
	_, release, _ := h.Subscribe("c1")
	release()
	// Must not send on a closed channel.
	h.Publish("c1", Event{Kind: "message", Text: "late"})
}

func TestSubscriberLimit(t *testing.T) {
	h := New()
	var releases []func()
	for i := range MaxSubscribersPerConversation {
		_, r, err := h.Subscribe("c1")
		if err != nil {
			t.Fatalf("subscriber %d rejected early: %v", i, err)
		}
		releases = append(releases, r)
	}
	if _, _, err := h.Subscribe("c1"); err == nil {
		t.Error("expected the limit to be enforced — otherwise a client can hold sockets open against us")
	}
	for _, r := range releases {
		r()
	}
	// Space frees up once connections close.
	if _, r, err := h.Subscribe("c1"); err != nil {
		t.Errorf("could not subscribe after releasing: %v", err)
	} else {
		r()
	}
}

func TestConcurrentUse(t *testing.T) {
	h := New()
	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch, release, err := h.Subscribe("c1")
			if err != nil {
				return // hit the limit; fine
			}
			defer release()
			go h.Publish("c1", Event{Kind: "message", Text: "x"})
			select {
			case <-ch:
			case <-time.After(500 * time.Millisecond):
			}
		}(i)
	}
	wg.Wait()

	if got := h.Conversations(); got != 0 {
		t.Errorf("leaked %d conversation entries", got)
	}
}
