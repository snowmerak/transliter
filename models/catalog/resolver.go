package catalog

import transliter "github.com/snowmerak/transliter/lib"

// Resolver adapts the built-in catalog to jobs.ModelResolver.
type Resolver struct{}

func (Resolver) Find(id transliter.ModelID) (transliter.Model, bool) {
	return Find(id)
}
