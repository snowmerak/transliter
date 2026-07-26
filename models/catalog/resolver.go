package catalog

import transliter "github.com/snowmerak/transliter/lib"

// Resolver adapts the built-in catalog to jobs.ModelResolver and restapi.Catalog.
type Resolver struct{}

func (Resolver) All() []transliter.Model {
	return All()
}

func (Resolver) Find(id transliter.ModelID) (transliter.Model, bool) {
	return Find(id)
}
