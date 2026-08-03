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

	"github.com/mnoorhussin/anis-app/pocketbase/internal/db"
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

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Fail fast rather than serving an assistant that cannot retrieve.
		// See db.AssertVecAvailable for why this is a real failure mode.
		if err := db.AssertVecAvailable(e.App.DB()); err != nil {
			return err
		}
		routes.Register(e)
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
