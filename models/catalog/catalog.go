// Package catalog exposes the built-in model implementations.
package catalog

import (
	transliter "github.com/snowmerak/translter/lib"
	hymt2v1p8b "github.com/snowmerak/translter/models/hymt2/v1p8b"
	hymt2v30ba3b "github.com/snowmerak/translter/models/hymt2/v30ba3b"
	hymt2v7b "github.com/snowmerak/translter/models/hymt2/v7b"
	gemmav12b "github.com/snowmerak/translter/models/translategemma/v12b"
	gemmav27b "github.com/snowmerak/translter/models/translategemma/v27b"
	gemmav4b "github.com/snowmerak/translter/models/translategemma/v4b"
)

// All returns a new slice containing every built-in model.
func All() []transliter.Model {
	return []transliter.Model{
		hymt2v1p8b.New(),
		hymt2v7b.New(),
		hymt2v30ba3b.New(),
		gemmav4b.New(),
		gemmav12b.New(),
		gemmav27b.New(),
	}
}

// Find returns the built-in model with id.
func Find(id transliter.ModelID) (transliter.Model, bool) {
	for _, model := range All() {
		if model.Descriptor().ID == id {
			return model, true
		}
	}
	return nil, false
}
