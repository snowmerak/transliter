package catalog

import transliter "github.com/snowmerak/translter/lib"

// Resolver adapts the built-in catalog to jobs.ModelResolver.
type Resolver struct{}

func (Resolver) Find(id transliter.ModelID) (transliter.Model, bool) {
	return Find(id)
}
