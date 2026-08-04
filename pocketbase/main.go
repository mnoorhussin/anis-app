// Command anis is the Anis backend: PocketBase used as a Go framework, with
// sqlite-vec linked in for retrieval.
//
// It is one process and one SQLite file. It serves the REST/realtime API, the
// admin UI, authentication, file storage, and our own routes for chat,
// crawling, billing webhooks and escalation.
//
// Build and run:
//
//	go run . serve --http=127.0.0.1:8090
//	go build -tags no_default_driver -o anis .
//
// The `no_default_driver` tag is not cosmetic. Without it, PocketBase also
// links modernc.org/sqlite and would silently fall back to a SQLite build with
// no vector support if DBConnect were ever unset. With the tag, that mistake
// panics at startup instead of degrading quietly in production.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/osutils"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/billing"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/bootstrap"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/fetch"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/ingest"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/live"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/llm"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
	_ "github.com/mnoorhussin/anis-app/pocketbase/migrations"
	"github.com/mnoorhussin/anis-app/pocketbase/routes"
)

func main() {
	app := pocketbase.NewWithConfig(pocketbase.Config{
		// Route every connection through the sqlite-vec build.
		DBConnect: db.Connect,
	})

	// Go migrations, versioned in migrations/.
	//
	// Automigrate is enabled only under `go run`, i.e. on a developer machine.
	// It writes a new migration file whenever a collection is changed through
	// the admin UI, so schema changes arrive as reviewable files. On the
	// server it stays off: a production admin session must not be able to
	// author a migration that no one has seen.
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangGo,
		Automigrate:  osutils.IsProbablyGoRun(),
		Dir:          "migrations",
	})

	// Every new user gets an account, a workspace and an owner membership,
	// atomically with the user row itself.
	bootstrap.Register(app)

	// Sets the Stripe API key once. Absent credentials are fine — billing
	// endpoints report that they are unavailable rather than failing at boot,
	// so the product still runs for development and for a self-hoster.
	billing.Init()

	// Keeps the vec0 index in step with deleted content. Not optional: the
	// index has no foreign keys, so nothing else removes a deleted chunk's
	// vector and it would stay searchable.
	ingest.RegisterHooks(app)

	svc := &ingest.Service{Embedder: newEmbedder(app)}

	// Provider-agnostic by construction: nothing outside internal/llm imports
	// a vendor SDK, so the grounding prompt is one implementation rather than
	// one per provider.
	registry := llm.NewRegistry(newProvider(app))

	// In-process fan-out to connected widgets. Single binary, single hub —
	// see internal/live for why that is a deliberate fit and what would have
	// to change if it ever runs as more than one process.
	hub := live.New()
	routes.RegisterLiveHooks(app, hub)
	deps := routes.Deps{
		Ingest: svc,
		LLM:    registry,
		Live:   hub,
		Crawler: &ingest.Crawler{
			Ingest: svc,
			// A named User-Agent with a contact URL, so a site owner can
			// identify us and rate-limit or block us deliberately rather than
			// wondering what is hitting their server.
			Client: newFetchClient(app),
		},
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Fail fast rather than serving an assistant that cannot retrieve.
		// See db.AssertVecAvailable for why this is a real failure mode.
		if err := db.AssertVecAvailable(e.App.DB()); err != nil {
			return err
		}
		if err := routes.Register(e, deps); err != nil {
			return err
		}
		return e.Next()
	})

	// Serve the built dashboard from pb_public when present. In production
	// Caddy serves the SPA directly and this never fires; it exists so a
	// single binary is still a complete, runnable product for local use.
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			if !e.Router.HasRoute(http.MethodGet, "/{path...}") {
				// indexFallback=true so client-side routes deep-link correctly.
				e.Router.GET("/{path...}", apis.Static(os.DirFS(publicDir()), true))
			}
			return e.Next()
		},
		Priority: 999, // last, so our own routes always win
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// publicDir mirrors PocketBase's own default: relative to the working
// directory under `go run`, and next to the binary once built.
func publicDir() string {
	if osutils.IsProbablyGoRun() {
		return "./pb_public"
	}
	return filepath.Join(os.Args[0], "../pb_public")
}

// newEmbedder returns the configured embedding provider.
//
// The development fallback is gated behind an explicit environment variable
// rather than simply "no API key set", because a missing key in production is
// exactly when a silent fallback would do the most damage: the assistant would
// keep answering, from passages retrieved by a model with no semantic
// understanding at all. Failing to start is the correct behaviour there.
func newEmbedder(app core.App) rag.Embedder {
	if os.Getenv("ANIS_DEV_FAKE_EMBEDDINGS") == "1" {
		app.Logger().Warn("USING FAKE EMBEDDINGS - retrieval quality is meaningless. " +
			"Never set ANIS_DEV_FAKE_EMBEDDINGS in production.")
		return &rag.HashEmbedder{}
	}
	return &rag.Voyage{
		APIKey:    os.Getenv("VOYAGE_API_KEY"),
		ModelName: os.Getenv("ANIS_EMBEDDING_MODEL"),
	}
}

// crawlerUserAgent identifies AnisBot to the sites we fetch.
func crawlerUserAgent() string {
	if ua := os.Getenv("ANIS_CRAWLER_USER_AGENT"); ua != "" {
		return ua
	}
	return "AnisBot/1.0 (+https://anis.chat/bot)"
}

// newFetchClient builds the crawler's HTTP client.
//
// The private-address escape hatch is gated behind an explicit variable and
// warns on every startup, for the same reason as the fake embedder: it must be
// impossible to enable by accident. With it on, a customer could point the
// crawler at the machine's own cloud-metadata endpoint and read the
// credentials back out of their knowledge base.
func newFetchClient(app core.App) *fetch.Client {
	if os.Getenv("ANIS_ALLOW_PRIVATE_CRAWL") == "1" {
		app.Logger().Warn("SSRF PROTECTION DISABLED for the crawler - private and " +
			"link-local addresses are reachable. Never set ANIS_ALLOW_PRIVATE_CRAWL in production.")
		return fetch.NewAllowingPrivateAddresses(crawlerUserAgent(), 0)
	}
	return fetch.New(crawlerUserAgent(), 0)
}

// newProvider returns the configured LLM provider.
//
// Gated on an explicit flag rather than on a missing key, for the same reason
// as the other escape hatches: a missing key in production is exactly when a
// silent fallback does the most damage. Echo is safe to fall back to only
// because it produces no prose — see its documentation.
func newProvider(app core.App) llm.Provider {
	if os.Getenv("ANIS_DEV_ECHO_LLM") == "1" {
		app.Logger().Warn("USING THE DEV ECHO PROVIDER - no model is called and " +
			"replies are raw retrieved passages. Never set ANIS_DEV_ECHO_LLM in production.")
		return &llm.Echo{}
	}
	return &llm.Anthropic{
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:  os.Getenv("ANIS_LLM_MODEL"),
	}
}
