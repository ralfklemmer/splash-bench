package omlx

import (
	"context"
	"net/http"
)

// Model is the part of oMLX's model listing the kit uses.
type Model struct {
	ID              string           `json:"id"`
	Loaded          bool             `json:"loaded"`
	IsLoading       bool             `json:"is_loading"`
	ModelPath       string           `json:"model_path"`
	EstimatedSize   int64            `json:"estimated_size"`
	Settings        ModelSettings    `json:"settings"`
	ExternalEngines []ExternalEngine `json:"external_engines"`
}

// ModelSettings holds the per-model settings; ExternalEngine is nil when
// oMLX runs the model itself.
type ModelSettings struct {
	ExternalEngine *string `json:"external_engine"`
}

// ExternalEngine tells whether oMLX can hand a model to an external engine.
type ExternalEngine struct {
	ID         string `json:"id"`
	Compatible bool   `json:"compatible"`
	Reason     string `json:"reason"`
	// DraftRepo is the DFlash2 draft the engine still needs; empty when
	// none is missing or oMLX does not know the family.
	DraftRepo string `json:"draft_repo"`
}

// Models lists all models oMLX knows. It needs the API key.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var list struct {
		Models []Model `json:"models"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/admin/api/models", nil, &list); err != nil {
		return nil, err
	}
	return list.Models, nil
}

// Health checks that the server answers. It works without an API key.
func (c *Client) Health(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, "/health", nil, nil)
}
