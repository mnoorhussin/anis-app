package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/answer"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/lang"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/llm"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/prompt"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/usage"
)

type widgetMessageRequest struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
	// Visitor is an opaque id the widget keeps in localStorage, so a returning
	// visitor's conversations group together. Never a personal identifier.
	Visitor string `json:"visitor"`
}

// MaxQuestionRunes bounds one question. Long enough for a detailed support
// query, short enough that pasting a book does not become an embedding bill.
const MaxQuestionRunes = 2000

// handleWidgetMessage answers a visitor's question, streaming the reply.
//
// The order below is the correctness story of the whole product, so it is
// written out rather than left implicit:
//
//  1. Resolve the key and check the Origin.
//  2. Check the plan allowance and the spending cap BEFORE spending anything.
//  3. Detect the language.
//  4. Embed and retrieve, scoped to the workspace the KEY named — never a
//     workspace id from the request body.
//  5. If nothing clears the confidence floor, refuse WITHOUT calling the
//     model, and record a knowledge gap.
//  6. Otherwise stream, recording which passages were used.
//  7. Meter exactly one reply, and only if one was generated.
func handleWidgetMessage(deps Deps) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		workspace, err := resolveWidget(e)
		if err != nil {
			return e.NotFoundError("not found", nil)
		}
		applyWidgetCORS(e)

		var req widgetMessageRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("could not read the request", err)
		}
		question := strings.TrimSpace(req.Text)
		if question == "" {
			return e.BadRequestError("empty message", nil)
		}
		if len([]rune(question)) > MaxQuestionRunes {
			question = string([]rune(question)[:MaxQuestionRunes])
		}

		account, err := e.App.FindRecordById("accounts", workspace.GetString("account"))
		if err != nil {
			return e.InternalServerError("workspace has no account", err)
		}

		// --- 2. Allowance and cap, before any spend ------------------------
		verdict, err := usage.Check(e.App, account)
		if err != nil {
			return e.InternalServerError("could not check usage", err)
		}

		// --- 3. Language ----------------------------------------------------
		detected := lang.Detect(question, "en")

		conversation, err := findOrCreateConversation(e.App, workspace.Id, req, detected.Language)
		if err != nil {
			return e.InternalServerError("could not open the conversation", err)
		}
		if _, err := saveMessage(e.App, workspace.Id, conversation.Id, "user", question, nil, "", 0); err != nil {
			return e.InternalServerError("could not save the message", err)
		}

		// A person is handling this conversation, or is about to. The
		// assistant must not answer over them.
		//
		// This is a hard rule from the brief, and it is enforced here rather
		// than in the widget because the endpoint is public: hiding the
		// composer would not stop a direct call. The visitor's message is
		// still saved — that is the whole point, the agent needs to see what
		// they said — and it appears in the inbox in real time.
		if status := conversation.GetString("status"); status == "human" || status == "escalated" {
			e.App.Logger().Info("assistant stayed silent; a person has the conversation",
				"conversation", conversation.Id, "status", status)
			return streamSilence(e, conversation)
		}

		// Over the cap: no generation at all. The visitor is offered a person
		// rather than shown our billing state — they did nothing wrong, and
		// the business's spending is not their business.
		if !verdict.Allowed {
			e.App.Logger().Warn("widget reply blocked by usage limits",
				"workspace", workspace.Id, "reason", verdict.Reason)
			return streamRefusal(e, conversation, workspace.Id, detected.Language,
				answer.OutcomeBlocked, 0, 0)
		}

		// --- 4. Retrieve -----------------------------------------------------
		floor := workspaceFloor(workspace)

		vectors, err := deps.Ingest.Embedder.Embed(e.Request.Context(), []string{question}, rag.InputQuery)
		if err != nil || len(vectors) != 1 {
			return e.InternalServerError("could not search the knowledge base", err)
		}

		retriever := &rag.VecRetriever{DB: e.App.DB(), Dimensions: deps.Ingest.Embedder.Dimensions()}
		// Retrieve more than we will use, so the floor has something to reject
		// and TopSimilarity is meaningful even when everything is rejected.
		chunks, err := retriever.Search(e.Request.Context(), workspace.Id, vectors[0], answer.MaxPassages*2)
		if err != nil {
			return e.InternalServerError("could not search the knowledge base", err)
		}

		// --- 5. Decide -------------------------------------------------------
		decision := answer.Decide(chunks, floor)

		// Logged on every message so the floor can be fitted to real traffic
		// rather than argued about. This is the raw material for tuning.
		e.App.Logger().Info("widget answer decision",
			"workspace", workspace.Id,
			"language", detected.Language,
			"outcome", string(decision.Outcome),
			"top_similarity", decision.TopSimilarity,
			"floor", decision.Floor,
			"passages", len(decision.Passages),
			"retrieved", len(chunks))

		if decision.Outcome != answer.OutcomeAnswered {
			// The refusal is a constant, produced WITHOUT calling the model.
			// Asking a model to decline is asking it to improvise, and the one
			// answer we must never risk being improvised is "I don't know".
			recordKnowledgeGap(e.App, workspace.Id, question, detected.Language, decision.TopSimilarity)
			return streamRefusal(e, conversation, workspace.Id, detected.Language,
				decision.Outcome, decision.TopSimilarity, decision.Floor)
		}

		// --- 6. Generate ------------------------------------------------------
		return streamAnswer(e, deps, workspace, account, conversation, question, detected, decision, verdict)
	}
}

// workspaceFloor reads the confidence floor for a workspace.
//
// A parameter, not a constant, so it can be tuned per workspace and swept
// during evaluation. The default is UNVALIDATED — see answer.DefaultFloor.
func workspaceFloor(workspace *core.Record) float64 {
	if v, ok := WidgetConfigMap(workspace)["confidenceFloor"].(float64); ok && v > 0 && v <= 1 {
		return v
	}
	return answer.DefaultFloor
}

/* -------------------------------------------------------------------------
 * Server-sent events
 * ---------------------------------------------------------------------- */

// sse writes one event. The caller must have set the headers already.
func sse(e *core.RequestEvent, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(e.Response, "event: %s\ndata: %s\n\n", event, body); err != nil {
		return err
	}
	// Flush per event. Without this the reply arrives in one lump at the end
	// and streaming buys nothing.
	return e.Flush()
}

func beginSSE(e *core.RequestEvent) {
	h := e.Response.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	// Belt and braces for proxies that buffer by default. Caddy is configured
	// with flush_interval -1, but a customer may sit behind their own CDN.
	h.Set("X-Accel-Buffering", "no")
	e.Response.WriteHeader(http.StatusOK)
}

// streamSilence acknowledges a message without answering it.
//
// Sent when a person has the conversation. No token event at all: an
// "an agent will reply shortly" line would be the product speaking on the
// agent's behalf, and if nobody is actually online it becomes a promise we
// did not keep. The widget shows the message as delivered and waits — which
// is what a person on the other end would expect from any chat.
//
// Nothing is metered: no reply was generated.
func streamSilence(e *core.RequestEvent, conversation *core.Record) error {
	beginSSE(e)
	return sse(e, "done", map[string]any{
		"conversationId": conversation.Id,
		"outcome":        "human",
		"offerHandoff":   false,
		// Tells the widget a person is handling it, so it can say so once
		// rather than leaving the visitor watching an empty panel.
		"withHuman": true,
	})
}

// streamRefusal sends the constant refusal and records it.
func streamRefusal(
	e *core.RequestEvent,
	conversation *core.Record,
	workspaceID, language string,
	outcome answer.Outcome,
	similarity, floor float64,
) error {
	text := prompt.Refusal(language)

	beginSSE(e)
	if err := sse(e, "token", map[string]any{"text": text}); err != nil {
		return nil // the visitor navigated away; nothing to report
	}

	// A refusal is not rateable — there is nothing to say was helpful — so its
	// id is not sent to the widget.
	if _, err := saveMessage(e.App, workspaceID, conversation.Id, "assistant", text,
		nil, string(outcome), similarity); err != nil {
		e.App.Logger().Error("could not save refusal", "error", err)
	}

	// A refusal is NOT a billable reply. Metering it would charge a customer
	// for the assistant declining to answer.
	return sse(e, "done", map[string]any{
		"conversationId": conversation.Id,
		"outcome":        string(outcome),
		// The widget uses this to offer a handoff, which is the whole point of
		// refusing well.
		"offerHandoff": true,
	})
}

// streamAnswer generates and streams a grounded reply.
func streamAnswer(
	e *core.RequestEvent,
	deps Deps,
	workspace, account, conversation *core.Record,
	question string,
	detected lang.Detection,
	decision answer.Decision,
	verdict usage.Verdict,
) error {
	system := prompt.System(prompt.Options{
		AssistantName: workspace.GetString("name"),
		BusinessName:  workspace.GetString("name"),
		Language:      detected.Language,
		Mixed:         detected.Mixed,
		Chunks:        decision.Passages,
	})

	history, err := recentTurns(e.App, conversation.Id)
	if err != nil {
		e.App.Logger().Error("could not load history", "error", err)
	}

	// Per-workspace provider selection exists so a privacy-sensitive customer
	// can be pinned to an EU-hosted model; unset means the default.
	provider := deps.LLM.For(workspace.GetString("llm_provider"))
	stream, wait := provider.Stream(e.Request.Context(), llmRequest(system, history, question))

	beginSSE(e)

	var reply strings.Builder
	for chunk := range stream {
		reply.WriteString(chunk.Text)
		if err := sse(e, "token", map[string]any{"text": chunk.Text}); err != nil {
			// Visitor disconnected. Stop writing, but still record what was
			// generated — it was paid for either way.
			break
		}
	}

	if err := wait(); err != nil {
		e.App.Logger().Error("generation failed", "workspace", workspace.Id, "error", err)
		if reply.Len() == 0 {
			// Nothing was produced, so nothing is metered and the visitor gets
			// the refusal rather than an error they cannot act on.
			return streamRefusal(e, conversation, workspace.Id, detected.Language,
				answer.OutcomeBlocked, decision.TopSimilarity, decision.Floor)
		}
	}

	text := reply.String()
	saved, err := saveMessage(e.App, workspace.Id, conversation.Id, "assistant", text,
		decision.SourceIDs(), string(answer.OutcomeAnswered), decision.TopSimilarity)
	if err != nil {
		e.App.Logger().Error("could not save reply", "error", err)
	}

	// --- 7. Meter, only now that a reply exists ---------------------------
	if err := usage.Record(e.App, account.Id, verdict.Overage); err != nil {
		e.App.Logger().Error("could not record usage", "account", account.Id, "error", err)
	}

	done := map[string]any{
		"conversationId": conversation.Id,
		"outcome":        string(answer.OutcomeAnswered),
		"offerHandoff":   false,
	}
	// The widget needs the stored id to attach a rating to this reply. Only
	// an answered reply carries one: a refusal has nothing to rate.
	if saved != nil {
		done["messageId"] = saved.Id
	}
	return sse(e, "done", done)
}

/* -------------------------------------------------------------------------
 * Persistence
 * ---------------------------------------------------------------------- */

func findOrCreateConversation(
	app core.App,
	workspaceID string,
	req widgetMessageRequest,
	language string,
) (*core.Record, error) {
	if req.ConversationID != "" {
		c, err := app.FindRecordById("conversations", req.ConversationID)
		// The conversation must belong to the workspace the KEY named. Without
		// this check a visitor could append to another business's conversation
		// by guessing an id.
		if err == nil && c.GetString("workspace") == workspaceID {
			return c, nil
		}
	}

	col, err := app.FindCollectionByNameOrId("conversations")
	if err != nil {
		return nil, err
	}
	c := core.NewRecord(col)
	c.Set("workspace", workspaceID)
	c.Set("channel", "website")
	c.Set("language", language)
	c.Set("status", "active")
	c.Set("visitor", req.Visitor)
	return c, app.Save(c)
}

func saveMessage(
	app core.App,
	workspaceID, conversationID, role, text string,
	sources []string,
	outcome string,
	confidence float64,
) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return nil, err
	}
	m := core.NewRecord(col)
	m.Set("workspace", workspaceID)
	m.Set("conversation", conversationID)
	m.Set("role", role)
	m.Set("text", text)
	if sources != nil {
		m.Set("sources_used", sources)
	}
	if outcome != "" {
		m.Set("outcome", outcome)
	}
	m.Set("confidence", confidence)
	if err := app.Save(m); err != nil {
		return nil, err
	}
	return m, nil
}

// HistoryTurns kept in context. Small on purpose: support conversations are
// short, and every extra turn is tokens on every subsequent reply.
const HistoryTurns = 6

func recentTurns(app core.App, conversationID string) ([]*core.Record, error) {
	msgs, err := app.FindRecordsByFilter("messages",
		"conversation = {:c}", "-created", HistoryTurns, 0,
		map[string]any{"c": conversationID})
	if err != nil {
		return nil, err
	}
	// Query returns newest first; the model needs oldest first.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// recordKnowledgeGap notes a question the sources could not answer.
//
// Grouped by normalised question so "هل الشحن مجاني" and "هل الشحن مجاني؟"
// are one gap with a count of two, not two gaps of one — the count is what
// makes the report worth reading.
func recordKnowledgeGap(app core.App, workspaceID, question, language string, similarity float64) {
	normalized := lang.Normalize(question)
	if normalized == "" {
		return
	}

	existing, err := app.FindRecordsByFilter("knowledge_gaps",
		"workspace = {:w} && question_normalized = {:q}", "", 1, 0,
		map[string]any{"w": workspaceID, "q": normalized})
	if err != nil {
		app.Logger().Error("could not look up knowledge gap", "error", err)
		return
	}

	if len(existing) > 0 {
		g := existing[0]
		g.Set("count", g.GetInt("count")+1)
		// Keep the best similarity ever seen: it says whether the content is
		// missing entirely or merely below the floor.
		if similarity > g.GetFloat("best_similarity") {
			g.Set("best_similarity", similarity)
		}
		if err := app.Save(g); err != nil {
			app.Logger().Error("could not update knowledge gap", "error", err)
		}
		return
	}

	col, err := app.FindCollectionByNameOrId("knowledge_gaps")
	if err != nil {
		return
	}
	g := core.NewRecord(col)
	g.Set("workspace", workspaceID)
	g.Set("question", question)
	g.Set("question_normalized", normalized)
	g.Set("language", language)
	g.Set("count", 1)
	g.Set("best_similarity", similarity)
	g.Set("answered", false)
	if err := app.Save(g); err != nil {
		app.Logger().Error("could not record knowledge gap", "error", err)
	}
}

// llmRequest assembles the model call from the grounded system prompt and the
// recent turns.
//
// Temperature is low and fixed. This is not a creative task: the reply must
// restate what the passages say, and sampling variety here shows up as
// invented detail.
func llmRequest(system string, history []*core.Record, question string) llm.Request {
	msgs := make([]llm.Message, 0, len(history)+1)
	for _, m := range history {
		role := llm.RoleUser
		// A human agent's message is presented to the model as the assistant's
		// own, because from the visitor's side it was the assistant speaking.
		if r := m.GetString("role"); r == "assistant" || r == "human" {
			role = llm.RoleAssistant
		}
		text := m.GetString("text")
		if strings.TrimSpace(text) == "" {
			continue
		}
		msgs = append(msgs, llm.Message{Role: role, Text: text})
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Text: question})

	return llm.Request{
		System:  system,
		History: msgs,
		// Support answers are short. A cap here is also a cost control: a
		// runaway generation is an expensive bad answer.
		MaxTokens:   600,
		Temperature: 0.1,
	}
}
