package restapi

import (
	"net/http"

	transliter "github.com/snowmerak/transliter/lib"
)

// Catalog finds built-in model adapters for discovery and preview endpoints.
type Catalog interface {
	All() []transliter.Model
	Find(transliter.ModelID) (transliter.Model, bool)
}

var knownProfiles = []transliter.OptionProfile{
	transliter.ProfileOfficial,
	transliter.ProfileDeterministic,
}

type modelCatalogSummary struct {
	ID             transliter.ModelID         `json:"id"`
	Family         string                     `json:"family"`
	Parameters     string                     `json:"parameters,omitempty"`
	Repository     string                     `json:"repository,omitempty"`
	GGUFRepository string                     `json:"gguf_repository,omitempty"`
	Capabilities   modelCatalogCapabilities   `json:"capabilities"`
	Profiles       []transliter.OptionProfile `json:"profiles"`
	Languages      []transliter.Language      `json:"languages"`
}

type modelCatalogCapabilities struct {
	PromptKinds            []transliter.PromptKind     `json:"prompt_kinds"`
	RequiresSourceLanguage bool                        `json:"requires_source_language"`
	StructuredUserContent  bool                        `json:"structured_user_content"`
	MaxInputTokens         int                         `json:"max_input_tokens,omitempty"`
	AuxiliaryFields        modelCatalogAuxiliaryFields `json:"auxiliary_fields"`
}

type modelCatalogAuxiliaryFields struct {
	Glossary               bool `json:"glossary"`
	Style                  bool `json:"style"`
	Audience               bool `json:"audience"`
	TranslatableAttributes bool `json:"translatable_attributes"`
	Delimiters             bool `json:"delimiters"`
}

type modelCatalogDetail struct {
	modelCatalogSummary
	ProfileOptions map[transliter.OptionProfile]transliter.GenerationOptions `json:"profile_options"`
}

type modelCatalogPreviewRequest struct {
	Profile     transliter.OptionProfile      `json:"profile"`
	Translation transliter.TranslationRequest `json:"translation"`
}

type modelCatalogPreviewResponse struct {
	ModelCatalog transliter.ModelID           `json:"model_catalog"`
	Profile      transliter.OptionProfile     `json:"profile"`
	Options      transliter.GenerationOptions `json:"options"`
	Input        transliter.ModelInput        `json:"input"`
}

func (handler *Handler) listModelCatalogs(writer http.ResponseWriter, _ *http.Request) {
	catalog, ok := handler.catalog()
	if !ok {
		writeError(writer, http.StatusServiceUnavailable, "model catalog is unavailable")
		return
	}
	models := catalog.All()
	items := make([]modelCatalogSummary, 0, len(models))
	for _, model := range models {
		items = append(items, summarizeModelCatalog(model))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"model_catalogs": items})
}

func (handler *Handler) getModelCatalog(writer http.ResponseWriter, request *http.Request) {
	catalog, ok := handler.catalog()
	if !ok {
		writeError(writer, http.StatusServiceUnavailable, "model catalog is unavailable")
		return
	}
	model, ok := catalog.Find(transliter.ModelID(request.PathValue("id")))
	if !ok {
		writeError(writer, http.StatusNotFound, "model catalog not found")
		return
	}
	detail, err := detailModelCatalog(model)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "model catalog options unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, detail)
}

func (handler *Handler) previewModelCatalog(writer http.ResponseWriter, request *http.Request) {
	catalog, ok := handler.catalog()
	if !ok {
		writeError(writer, http.StatusServiceUnavailable, "model catalog is unavailable")
		return
	}
	model, ok := catalog.Find(transliter.ModelID(request.PathValue("id")))
	if !ok {
		writeError(writer, http.StatusNotFound, "model catalog not found")
		return
	}

	var input modelCatalogPreviewRequest
	if err := decodeJSON(writer, request, handler.MaxBodyBytes, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := transliter.ValidateTranslationRequest(input.Translation); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	profile := input.Profile
	if profile == "" {
		profile = transliter.ProfileOfficial
	}
	options, err := model.Options(profile)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
		return
	}
	built, err := model.BuildInput(input.Translation)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, modelCatalogPreviewResponse{
		ModelCatalog: model.Descriptor().ID,
		Profile:      profile,
		Options:      options,
		Input:        built,
	})
}

func (handler *Handler) catalog() (Catalog, bool) {
	if handler == nil || handler.Catalog == nil {
		return nil, false
	}
	return handler.Catalog, true
}

func summarizeModelCatalog(model transliter.Model) modelCatalogSummary {
	descriptor := model.Descriptor()
	capabilities := model.Capabilities()
	return modelCatalogSummary{
		ID:             descriptor.ID,
		Family:         descriptor.Family,
		Parameters:     descriptor.Parameters,
		Repository:     descriptor.Repository,
		GGUFRepository: descriptor.GGUFRepository,
		Capabilities: modelCatalogCapabilities{
			PromptKinds:            append([]transliter.PromptKind(nil), capabilities.PromptKinds...),
			RequiresSourceLanguage: capabilities.RequiresSourceLanguage,
			StructuredUserContent:  capabilities.StructuredUserContent,
			MaxInputTokens:         capabilities.MaxInputTokens,
			AuxiliaryFields: modelCatalogAuxiliaryFields{
				Glossary:               capabilities.Glossary,
				Style:                  capabilities.Style,
				Audience:               capabilities.Audience,
				TranslatableAttributes: capabilities.TranslatableAttributes,
				Delimiters:             capabilities.Delimiters,
			},
		},
		Profiles:  supportedProfiles(model),
		Languages: supportedModelLanguages(model),
	}
}

func detailModelCatalog(model transliter.Model) (modelCatalogDetail, error) {
	summary := summarizeModelCatalog(model)
	options := make(map[transliter.OptionProfile]transliter.GenerationOptions, len(summary.Profiles))
	for _, profile := range summary.Profiles {
		value, err := model.Options(profile)
		if err != nil {
			return modelCatalogDetail{}, err
		}
		options[profile] = value
	}
	return modelCatalogDetail{
		modelCatalogSummary: summary,
		ProfileOptions:      options,
	}, nil
}

func supportedProfiles(model transliter.Model) []transliter.OptionProfile {
	profiles := make([]transliter.OptionProfile, 0, len(knownProfiles))
	for _, profile := range knownProfiles {
		if _, err := model.Options(profile); err == nil {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

func supportedModelLanguages(model transliter.Model) []transliter.Language {
	all := transliter.SupportedLanguages()
	languages := make([]transliter.Language, 0, len(all))
	for _, language := range all {
		if model.SupportsLanguage(language) {
			languages = append(languages, language)
		}
	}
	return languages
}
