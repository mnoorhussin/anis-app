package routes

import (
	"net/http"
	"strconv"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/analytics"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/chunk"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/ingest"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/plans"
)

type rateRequest struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	Rating         string `json:"rating"`
	Visitor        string `json:"visitor"`
}

// handleWidgetRate records a visitor's thumbs up or down on a reply.
//
// This is the strongest resolution signal the product has. The brief is
// explicit that a conversation ending proves nothing, so without a visitor
// actually saying "that helped", `auto_resolved` would be permanently zero and
// the headline dashboard number would be meaningless.
//
// A thumbs UP marks the conversation auto-resolved with the `rated_helpful`
// signal. A thumbs DOWN marks nothing — it is recorded against the message for
// quality review, but an unhelpful answer is not evidence of any outcome, and
// inventing one would be exactly the dishonesty this design avoids.
func handleWidgetRate(e *core.RequestEvent) error {
	workspace, err := resolveWidget(e)
	if err != nil {
		return e.NotFoundError("not found", nil)
	}
	applyWidgetCORS(e)

	var req rateRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("could not read the request", err)
	}
	if req.Rating != "up" && req.Rating != "down" {
		return e.BadRequestError("rating must be up or down", nil)
	}

	conversation, err := e.App.FindRecordById("conversations", req.ConversationID)
	if err != nil {
		return e.NotFoundError("not found", nil)
	}
	// Same ownership rule as the stream: the widget key is public, so the
	// conversation id AND the visitor id together are what authorise this.
	// Without the visitor check, anyone with the key could rate — and
	// therefore resolve — another visitor's conversation.
	if conversation.GetString("workspace") != workspace.Id ||
		conversation.GetString("visitor") != req.Visitor {
		return e.NotFoundError("not found", nil)
	}

	message, err := e.App.FindRecordById("messages", req.MessageID)
	if err != nil || message.GetString("conversation") != conversation.Id {
		return e.NotFoundError("not found", nil)
	}
	// Only the assistant's own replies are rateable. Rating a visitor's own
	// message is meaningless, and rating a human agent's reply would put a
	// number on a colleague rather than on the product.
	if role := message.GetString("role"); role != "assistant" {
		return e.BadRequestError("only an assistant reply can be rated", nil)
	}

	message.Set("rating", req.Rating)
	if err := e.App.Save(message); err != nil {
		return e.InternalServerError("could not save the rating", err)
	}

	if req.Rating == "up" && conversation.GetString("status") == "active" {
		conversation.Set("status", "auto_resolved")
		conversation.Set("resolution_signal", "rated_helpful")
		if err := e.App.Save(conversation); err != nil {
			e.App.Logger().Error("could not mark the conversation resolved", "error", err)
		}
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// handleAnalytics returns the workspace's numbers.
func handleAnalytics(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("sign in required", nil)
	}
	workspace, err := workspaceForMember(e.App, e.Auth.Id, e.Request.URL.Query().Get("workspace"))
	if err != nil {
		return e.NotFoundError("workspace not found", nil)
	}

	days := 30
	if v := e.Request.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	summary, err := analytics.Compute(e.App, workspace.Id, days)
	if err != nil {
		// Logged as well as returned: the response deliberately hides the
		// cause from the customer, which also hid it from us.
		e.App.Logger().Error("analytics failed", "workspace", workspace.Id, "error", err)
		return e.InternalServerError("could not compute analytics", err)
	}
	return e.JSON(http.StatusOK, summary)
}

type answerGapRequest struct {
	GapID  string `json:"gapId"`
	Answer string `json:"answer"`
}

// handleAnswerGap turns an unanswered question into knowledge.
//
// The whole point of the knowledge-gap report: a question the assistant could
// not answer becomes an FAQ entry in one step, so the report is a queue of work
// rather than a list of complaints.
//
// The gap is only marked answered once ingestion has actually succeeded —
// otherwise a failed embedding would quietly clear the report while the
// assistant still could not answer the question.
func handleAnswerGap(deps Deps) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("sign in required", nil)
		}

		var req answerGapRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("could not read the request", err)
		}
		if req.Answer == "" {
			return e.BadRequestError("an answer is required", nil)
		}

		gap, err := e.App.FindRecordById("knowledge_gaps", req.GapID)
		if err != nil {
			return e.NotFoundError("not found", nil)
		}
		// Membership is verified against the gap's OWN workspace, read from
		// the record rather than the request.
		workspace, err := workspaceForMember(e.App, e.Auth.Id, gap.GetString("workspace"))
		if err != nil || workspace.Id != gap.GetString("workspace") {
			return e.NotFoundError("not found", nil)
		}

		account, err := e.App.FindRecordById("accounts", workspace.GetString("account"))
		if err != nil {
			return e.InternalServerError("workspace has no account", err)
		}
		limits := plans.For(account.GetString("plan"))

		question := gap.GetString("question")
		source, err := deps.Ingest.Run(e.Request.Context(), e.App, ingest.Request{
			WorkspaceID: workspace.Id,
			Title:       question,
			Type:        "faq",
			Pairs:       []chunk.QA{{Question: question, Answer: req.Answer}},
			ChunkLimit:  limits.ChunksPerWorkspace,
		})
		if err != nil {
			return e.JSON(http.StatusBadRequest, map[string]any{
				"error":  err.Error(),
				"source": sourceView(source),
			})
		}

		gap.Set("answered", true)
		if err := e.App.Save(gap); err != nil {
			e.App.Logger().Error("could not mark the gap answered", "error", err)
		}

		return e.JSON(http.StatusOK, map[string]any{"ok": true, "source": sourceView(source)})
	}
}
