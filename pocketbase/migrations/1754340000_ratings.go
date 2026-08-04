package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds message ratings.
//
// This exists to make one number honest. `auto_resolved` requires a defensible
// signal — the brief is explicit that a conversation ending is NOT evidence it
// was resolved — and until a visitor can say "that helped", the strongest
// signal available is absent and the metric is permanently zero.
//
// A rating is also the only per-answer quality data the product collects, and
// it is what the confidence floor will eventually be tuned against: a run of
// thumbs-down on answers that cleared the floor says the floor is too low far
// more reliably than any offline eval.
func init() {
	m.Register(ratingsUp, ratingsDown)
}

func ratingsUp(app core.App) error {
	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return err
	}

	messages.Fields.Add(
		&core.SelectField{Name: "rating", MaxSelect: 1, Values: []string{"up", "down"}},
	)
	if err := app.Save(messages); err != nil {
		return fmt.Errorf("messages.rating: %w", err)
	}

	return nil
}

func ratingsDown(app core.App) error {
	if messages, err := app.FindCollectionByNameOrId("messages"); err == nil {
		messages.Fields.RemoveByName("rating")
		if err := app.Save(messages); err != nil {
			return err
		}
	}
	return nil
}
