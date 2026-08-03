package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/chunk"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/crawl"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/fetch"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/lang"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/rag"
)

// Crawler runs website ingestion in the background.
//
// Crawling a 200-page site takes minutes, so it cannot happen inside the HTTP
// request that starts it. The source row is created immediately in `queued`
// and the customer watches it move through `fetching` → `processing` →
// `ready`, which is why every one of those states exists in the schema.
type Crawler struct {
	Ingest  *Service
	Client  *fetch.Client
	MaxPage int

	// Bounded concurrency. Each crawl holds memory for a page frontier and
	// makes outbound requests under our own IP; letting an unbounded number
	// run gets AnisBot rate-limited or blocked, and can exhaust the box.
	sem  chan struct{}
	once sync.Once
}

// MaxConcurrentCrawls across the whole process.
const MaxConcurrentCrawls = 3

// CrawlTimeout bounds one whole site crawl. A site that responds slowly to
// every request must not hold a worker forever.
const CrawlTimeout = 10 * time.Minute

func (c *Crawler) init() {
	c.once.Do(func() { c.sem = make(chan struct{}, MaxConcurrentCrawls) })
}

// Start validates the URL, creates the source, and crawls in the background.
//
// The returned record is the `queued` source, so the dashboard can show it at
// once. Validation happens here rather than in the worker so a mistyped
// address is an immediate, comprehensible error instead of a source that sits
// queued and then fails.
func (c *Crawler) Start(app core.App, workspaceID, rawURL string, limits Limits) (*core.Record, error) {
	c.init()

	u, err := c.Client.CheckURL(rawURL)
	if err != nil {
		return nil, err
	}

	sourcesCol, err := app.FindCollectionByNameOrId("sources")
	if err != nil {
		return nil, err
	}

	source := core.NewRecord(sourcesCol)
	source.Set("workspace", workspaceID)
	source.Set("type", "website")
	source.Set("title", u.Hostname())
	source.Set("url", u.String())
	source.Set("status", "queued")
	if err := app.Save(source); err != nil {
		return nil, fmt.Errorf("ingest: create source: %w", err)
	}

	go c.run(app, source.Id, u.String(), workspaceID, limits)

	return source, nil
}

// Refresh re-crawls an existing website source.
//
// The old chunks are deleted first, which also removes their vectors via the
// delete hook. Doing it up front rather than at the end means a refresh that
// fails leaves the source empty and visibly `failed`, instead of quietly
// serving stale content the customer believes they have replaced.
func (c *Crawler) Refresh(app core.App, source *core.Record, limits Limits) error {
	c.init()

	if source.GetString("type") != "website" {
		return errors.New("ingest: only website sources can be refreshed")
	}
	url := source.GetString("url")
	if url == "" {
		return errors.New("ingest: source has no URL")
	}

	if err := deleteChunksOf(app, source); err != nil {
		return err
	}

	source.Set("status", "queued")
	source.Set("error", "")
	source.Set("pages", 0)
	if err := app.Save(source); err != nil {
		return err
	}

	go c.run(app, source.Id, url, source.GetString("workspace"), limits)
	return nil
}

// Limits are the plan ceilings that apply to one ingestion.
type Limits struct {
	Chunks int
	Pages  int
}

func (c *Crawler) run(app core.App, sourceID, rawURL, workspaceID string, limits Limits) {
	// The goroutine outlives the HTTP request that started it, so it gets its
	// own bounded context rather than inheriting one that is already cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), CrawlTimeout)
	defer cancel()

	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		c.fail(app, sourceID, "the crawler was busy for too long; please try again")
		return
	}

	source, err := app.FindRecordById("sources", sourceID)
	if err != nil {
		// Deleted while queued. Nothing to report to.
		return
	}
	source.Set("status", "fetching")
	if err := app.Save(source); err != nil {
		app.Logger().Error("crawl: could not mark source as fetching", "source", sourceID, "error", err)
	}

	maxPages := limits.Pages
	if maxPages <= 0 {
		maxPages = c.MaxPage
	}

	result, err := crawl.Site(ctx, rawURL, crawl.Options{
		Client:   c.Client,
		MaxPages: maxPages,
	})
	if err != nil {
		c.fail(app, sourceID, shortReason(err))
		return
	}
	if result.RobotsBlocked {
		// A distinct, actionable message. "0 pages found" would send the
		// customer looking for a problem on our side.
		c.fail(app, sourceID, "this site's robots.txt asks crawlers not to index it, "+
			"so we have not. Add the pages as text instead, or allow AnisBot in robots.txt.")
		return
	}
	if len(result.Docs) == 0 {
		c.fail(app, sourceID, describeEmptyCrawl(result))
		return
	}

	c.setStatus(app, sourceID, "processing")

	if err := c.index(ctx, app, sourceID, workspaceID, result, limits.Chunks); err != nil {
		c.fail(app, sourceID, shortReason(err))
		return
	}
}

// index chunks every crawled page and writes them with their vectors.
func (c *Crawler) index(
	ctx context.Context,
	app core.App,
	sourceID, workspaceID string,
	result *crawl.Result,
	chunkLimit int,
) error {
	// Each page is chunked separately, and its title is prepended to every
	// chunk. Without that, a chunk from the returns page and a chunk from the
	// shipping page are indistinguishable once retrieved — the title is often
	// the only thing saying which policy a passage belongs to.
	var chunks []chunk.Chunk
	for _, doc := range result.Docs {
		for _, ch := range chunk.Text(doc.Text) {
			ch.Text = doc.Title + "\n\n" + ch.Text
			ch.Position = len(chunks)
			chunks = append(chunks, ch)
		}
	}
	if len(chunks) == 0 {
		return errors.New("the pages we could read contained no usable text")
	}

	existing, err := rag.CountWorkspace(app.DB(), workspaceID)
	if err != nil {
		return err
	}
	if chunkLimit > 0 && existing+len(chunks) > chunkLimit {
		return fmt.Errorf("%w: this site produced %d passages, which would exceed the plan limit of %d",
			ErrChunkLimit, len(chunks), chunkLimit)
	}

	texts := make([]string, len(chunks))
	for i, ch := range chunks {
		texts[i] = ch.Text
	}
	vectors, err := c.Ingest.Embedder.Embed(ctx, texts, rag.InputDocument)
	if err != nil {
		return fmt.Errorf("could not build the search index: %w", err)
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("got %d vectors for %d passages", len(vectors), len(chunks))
	}

	chunksCol, err := app.FindCollectionByNameOrId("chunks")
	if err != nil {
		return err
	}

	return app.RunInTransaction(func(txApp core.App) error {
		entries := make([]rag.IndexEntry, 0, len(chunks))
		for i, ch := range chunks {
			rec := core.NewRecord(chunksCol)
			rec.Set("workspace", workspaceID)
			rec.Set("source", sourceID)
			rec.Set("text", ch.Text)
			rec.Set("text_normalized", lang.Normalize(ch.Text))
			rec.Set("position", ch.Position)
			if err := txApp.Save(rec); err != nil {
				return err
			}
			entries = append(entries, rag.IndexEntry{
				ChunkID:     rec.Id,
				WorkspaceID: workspaceID,
				Embedding:   vectors[i],
			})
		}
		if err := rag.Insert(txApp.DB(), entries, c.Ingest.Embedder.Dimensions()); err != nil {
			return err
		}

		source, err := txApp.FindRecordById("sources", sourceID)
		if err != nil {
			return err
		}
		source.Set("status", "ready")
		source.Set("error", skippedSummary(result))
		source.Set("pages", len(result.Docs))
		source.Set("last_refreshed", time.Now().UTC().Format(time.RFC3339))
		return txApp.Save(source)
	})
}

// skippedSummary tells the customer which pages were left out and why.
//
// Stored even on success, because "we indexed 12 of your 20 pages" is
// information they need — the eight missing ones are usually the reason an
// answer they expected never comes.
func skippedSummary(result *crawl.Result) string {
	if len(result.Skipped) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d page(s) were not indexed: ", len(result.Skipped))
	for i, s := range result.Skipped {
		if i == 5 {
			fmt.Fprintf(&b, "and %d more", len(result.Skipped)-5)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s (%s)", s.URL, s.Reason)
	}
	return truncate(b.String(), 2000)
}

func describeEmptyCrawl(result *crawl.Result) string {
	if len(result.Skipped) == 0 {
		return "we could not read any pages from this address"
	}
	return "no pages could be indexed. " + skippedSummary(result)
}

func (c *Crawler) setStatus(app core.App, sourceID, status string) {
	source, err := app.FindRecordById("sources", sourceID)
	if err != nil {
		return
	}
	source.Set("status", status)
	if err := app.Save(source); err != nil {
		app.Logger().Error("crawl: could not update status", "source", sourceID, "error", err)
	}
}

func (c *Crawler) fail(app core.App, sourceID, reason string) {
	source, err := app.FindRecordById("sources", sourceID)
	if err != nil {
		return
	}
	source.Set("status", "failed")
	source.Set("error", truncate(reason, 2000))
	if err := app.Save(source); err != nil {
		app.Logger().Error("crawl: could not record failure", "source", sourceID, "error", err)
	}
}

// deleteChunksOf removes a source's chunks. Their vectors go with them via the
// delete hook in hooks.go.
func deleteChunksOf(app core.App, source *core.Record) error {
	chunks, err := app.FindRecordsByFilter("chunks", "source = {:s}", "", 0, 0,
		map[string]any{"s": source.Id})
	if err != nil {
		return err
	}
	for _, ch := range chunks {
		if err := app.Delete(ch); err != nil {
			return err
		}
	}
	return nil
}

func shortReason(err error) string {
	msg := err.Error()
	// Strip Go's error-wrapping prefixes; the customer needs the cause, not
	// our call stack rendered as text.
	for _, prefix := range []string{"ingest: ", "crawl: ", "fetch: ", "extract: "} {
		msg = strings.ReplaceAll(msg, prefix, "")
	}
	return truncate(msg, 2000)
}
