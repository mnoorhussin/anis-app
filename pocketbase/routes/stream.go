package routes

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/live"
)

// HistoryLimit is how much of a conversation is replayed when a widget
// connects. Enough that a visitor who reloads sees the thread they were in,
// not so much that a long conversation becomes a large first payload.
const HistoryLimit = 50

// heartbeatInterval keeps the connection alive through proxies that close idle
// sockets. A comment line is used rather than an event so a client never has
// to filter it out.
const heartbeatInterval = 25 * time.Second

// handleWidgetStream pushes new messages to a widget as they happen.
//
// This is what makes human takeover actually work: an agent replies in the
// inbox and the visitor sees it, without polling and without the widget
// carrying the PocketBase SDK.
//
// Authorisation is the interesting part. The widget key is public, so it
// cannot be what protects a conversation — anyone can read it out of a page's
// source, and with the key alone they would be able to read every conversation
// in that workspace. Access therefore requires BOTH the conversation id and
// the visitor id that opened it. Both are random and neither is published, so
// together they act as a bearer token for one anonymous thread. The visitor id
// is treated as a secret for exactly that reason: it is stored only in the
// visitor's own browser and never rendered anywhere.
func handleWidgetStream(deps Deps) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		workspace, err := resolveWidget(e)
		if err != nil {
			return e.NotFoundError("not found", nil)
		}
		applyWidgetCORS(e)

		conversationID := e.Request.URL.Query().Get("conversation")
		visitor := e.Request.URL.Query().Get("visitor")
		if conversationID == "" || visitor == "" {
			return e.BadRequestError("conversation and visitor are required", nil)
		}

		conversation, err := e.App.FindRecordById("conversations", conversationID)
		if err != nil {
			return e.NotFoundError("not found", nil)
		}
		// Same 404 for "no such conversation", "another workspace" and "not
		// your conversation". Distinguishing them would let someone with a
		// widget key probe for valid conversation ids.
		if conversation.GetString("workspace") != workspace.Id ||
			conversation.GetString("visitor") != visitor {
			return e.NotFoundError("not found", nil)
		}

		events, release, err := deps.Live.Subscribe(conversationID)
		if err != nil {
			return e.TooManyRequestsError("too many open connections", nil)
		}
		defer release()

		beginSSE(e)

		// Clear the write deadline. PocketBase's server sets a five-minute
		// WriteTimeout, which would sever a healthy stream mid-conversation —
		// and the symptom is an agent's reply that simply never arrives.
		rc := http.NewResponseController(e.Response)
		_ = rc.SetWriteDeadline(time.Time{})

		// Replay first, so a visitor who reloaded sees the thread rather than
		// an empty panel with a live connection attached to it.
		if err := sendHistory(e, conversationID); err != nil {
			return nil
		}
		if err := sse(e, "ready", map[string]any{
			"conversationId": conversationID,
			"status":         conversation.GetString("status"),
		}); err != nil {
			return nil
		}

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		ctx := e.Request.Context()
		for {
			select {
			case <-ctx.Done():
				// Visitor navigated away or closed the tab.
				return nil

			case ev, ok := <-events:
				if !ok {
					return nil
				}
				if err := sse(e, ev.Kind, ev); err != nil {
					return nil
				}

			case <-ticker.C:
				// A bare comment line: valid SSE, ignored by every client,
				// and enough to stop an idle connection being reaped.
				if _, err := e.Response.Write([]byte(": keep-alive\n\n")); err != nil {
					return nil
				}
				if err := e.Flush(); err != nil {
					return nil
				}
			}
		}
	}
}

// sendHistory replays the recent messages of a conversation.
func sendHistory(e *core.RequestEvent, conversationID string) error {
	msgs, err := e.App.FindRecordsByFilter("messages",
		"conversation = {:c}", "-created", HistoryLimit, 0,
		map[string]any{"c": conversationID})
	if err != nil {
		return err
	}
	// Query is newest-first; the widget renders oldest-first.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	items := make([]live.Event, 0, len(msgs))
	for _, m := range msgs {
		items = append(items, live.Event{
			Kind:    "message",
			ID:      m.Id,
			Role:    m.GetString("role"),
			Text:    m.GetString("text"),
			Outcome: m.GetString("outcome"),
			Created: m.GetDateTime("created").String(),
		})
	}

	return sse(e, "history", map[string]any{"messages": items})
}

// RegisterLiveHooks publishes message and status changes to connected widgets.
//
// Hung off record hooks rather than called from the handlers, so anything that
// writes a message — the chat route, an agent in the inbox, a future channel
// integration — reaches the visitor without each one remembering to publish.
func RegisterLiveHooks(app core.App, hub *live.Hub) {
	app.OnRecordAfterCreateSuccess("messages").BindFunc(func(e *core.RecordEvent) error {
		conversationID := e.Record.GetString("conversation")
		if conversationID == "" {
			return e.Next()
		}
		// Only a HUMAN reply is pushed live.
		//
		// The visitor's own message is already on screen, rendered
		// optimistically when they sent it. The assistant's reply already
		// arrived as streamed tokens on the request that asked for it —
		// publishing it here too would render it twice, once growing
		// character by character and once complete.
		//
		// Both still appear in the history replay, so a reload or a reconnect
		// shows the whole thread. The cost of this choice is that a visitor
		// with two tabs open sees their assistant replies in only one of them,
		// which is a trade worth making against duplicate bubbles for
		// everyone.
		if role := e.Record.GetString("role"); role == "human" {
			hub.Publish(conversationID, live.Event{
				Kind:    "message",
				ID:      e.Record.Id,
				Role:    role,
				Text:    e.Record.GetString("text"),
				Outcome: e.Record.GetString("outcome"),
				Created: e.Record.GetDateTime("created").String(),
			})
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("conversations").BindFunc(func(e *core.RecordEvent) error {
		// So a widget knows when a person took over, and when the assistant
		// was handed the conversation back.
		hub.Publish(e.Record.Id, live.Event{
			Kind:   "status",
			Status: e.Record.GetString("status"),
		})
		return e.Next()
	})
}
