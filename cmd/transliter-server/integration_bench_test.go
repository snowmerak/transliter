package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/snowmerak/transliter/lib/inference/openai"
	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/restapi"
	"github.com/snowmerak/transliter/models/catalog"
)

// Shared bench targets (Hy-MT2 ∩ TranslateGemma friendly set). Source is English.
var benchTargets = []struct {
	name     string
	language string
}{
	{"ko", "Korean"},
	{"ja", "Japanese"},
	{"zh", "Chinese"},
	{"zh-hant", "Traditional Chinese"},
	{"fr", "French"},
	{"de", "German"},
	{"es", "Spanish"},
	{"pt", "Portuguese"},
	{"ru", "Russian"},
	{"ar", "Arabic"},
	{"th", "Thai"},
	{"vi", "Vietnamese"},
	{"hi", "Hindi"},
	{"tr", "Turkish"},
	{"it", "Italian"},
}

const (
	benchSource   = "The service is ready. Please review the attached summary and confirm the next steps before Friday."
	benchWarmRuns = 2 // full language set passes after first request
)

type benchSample struct {
	Model       string
	Provider    string
	Target      string
	Phase       string // first | warm
	Pass        int    // 0=first, 1..=warm pass index
	Wall        time.Duration
	Infer       time.Duration // CompletedAt - StartedAt
	PromptTok   int
	OutputTok   int
	TotalTok    int
	Translation string
	Err         string
}

func TestIntegrationLanguageBench(t *testing.T) {
	available := requireIntegrationModels(t)

	authenticator, err := jobs.NewStaticAuthenticator(map[string]string{
		"alice": integrationAPIKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backends, err := buildBackends(ctx, serverConfig{
		QueueBackend:       "nats-embedded",
		StoreBackend:       "sqlite",
		SQLitePath:         filepath.Join(t.TempDir(), "jobs.db"),
		NATSEmbeddedMemory: true,
		JobTimeout:         5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backends.Close()

	client, err := openai.New(openai.Config{
		BaseURL: integrationBaseURL,
		Model:   "hy-mt2-1.8b",
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	processor := jobs.NewTranslationProcessor(catalog.Resolver{}, client)
	schedulerDone := make(chan error, 1)
	scheduler := &jobs.Scheduler{
		Queue:       backends.queue,
		Store:       backends.store,
		Processor:   processor,
		Concurrency: 1,
		JobTimeout:  5 * time.Minute,
	}
	go func() { schedulerDone <- scheduler.Run(ctx) }()
	defer func() {
		cancel()
		<-schedulerDone
	}()

	handler := &restapi.Handler{
		Authenticator: authenticator,
		Queue:         backends.queue,
		Store:         backends.store,
		Catalog:       catalog.Resolver{},
		Retention:     time.Hour,
		Validate:      validateJobRequest,
	}
	routes, err := handler.Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()
	t.Cleanup(func() { freeOMLXModelSlot(t) })

	var samples []benchSample
	modelsRun := 0

	cases := append([]struct {
		name               string
		catalogModel       string
		providerCandidates []string
	}{}, integrationModelCases...)
	if os.Getenv("TRANSLITER_INTEGRATION_HY_EXTENDED") == "1" {
		cases = append(cases, hyExtendedDiagnosticCases...)
	} else {
		t.Log("extended Hy bench cases skipped; set TRANSLITER_INTEGRATION_HY_EXTENDED=1 to include")
	}
	if os.Getenv("TRANSLITER_INTEGRATION_TRANSLATEGEMMA") == "1" {
		cases = append(cases, translateGemmaDiagnosticCases...)
	} else {
		t.Log("TranslateGemma bench cases skipped; set TRANSLITER_INTEGRATION_TRANSLATEGEMMA=1 to include")
	}

	for _, tc := range cases {
		providerModel, ok := resolveProviderModel(available, tc.providerCandidates)
		if !ok {
			t.Logf("skip %s: provider not mounted (candidates=%v)", tc.name, tc.providerCandidates)
			continue
		}
		modelsRun++

		t.Run(tc.name, func(t *testing.T) {
			// Unload every resident model first so first_request includes cold load
			// and large models do not OOM against leftover weights.
			freeOMLXModelSlot(t)
			t.Cleanup(func() { freeOMLXModelSlot(t) })

			first := runBenchJob(t, server.URL, tc.catalogModel, providerModel, "Korean", "first", 0)
			samples = append(samples, first)
			t.Logf(
				"first_request wall=%s infer=%s tokens=%d/%d/%d out=%q err=%q",
				first.Wall.Round(time.Millisecond),
				first.Infer.Round(time.Millisecond),
				first.PromptTok,
				first.OutputTok,
				first.TotalTok,
				truncateRunes(first.Translation, 80),
				first.Err,
			)
			if first.Err != "" {
				t.Fatalf("first request failed: %s", first.Err)
			}

			for pass := 1; pass <= benchWarmRuns; pass++ {
				t.Run(fmt.Sprintf("warm%d", pass), func(t *testing.T) {
					for _, target := range benchTargets {
						sample := runBenchJob(
							t,
							server.URL,
							tc.catalogModel,
							providerModel,
							target.language,
							"warm",
							pass,
						)
						samples = append(samples, sample)
						if sample.Err != "" {
							t.Errorf("%s: %s", target.name, sample.Err)
							continue
						}
						t.Logf(
							"%-8s wall=%8s infer=%8s tok=%d/%d/%d out=%q",
							target.name,
							sample.Wall.Round(time.Millisecond),
							sample.Infer.Round(time.Millisecond),
							sample.PromptTok,
							sample.OutputTok,
							sample.TotalTok,
							truncateRunes(sample.Translation, 60),
						)
					}
				})
			}
		})
	}

	if modelsRun == 0 {
		t.Fatal("no catalog provider models available on inference server")
	}

	printBenchSummary(t, samples)
}

func runBenchJob(
	t *testing.T,
	baseURL, catalogModel, providerModel, targetLang, phase string,
	pass int,
) benchSample {
	t.Helper()

	sample := benchSample{
		Model:    catalogModel,
		Provider: providerModel,
		Target:   targetLang,
		Phase:    phase,
		Pass:     pass,
	}

	createBody := fmt.Sprintf(`{
		"model_catalog": %q,
		"model": %q,
		"profile": "official",
		"translation": {
			"source": %q,
			"source_language": "English",
			"target_language": %q,
			"kind": "text",
			"glossary": {}
		}
	}`, catalogModel, providerModel, benchSource, targetLang)

	start := time.Now()
	create := integrationAPIRequest(
		t,
		http.MethodPost,
		baseURL+"/v1/jobs",
		integrationAPIKey,
		createBody,
	)
	if create.StatusCode != http.StatusAccepted {
		sample.Wall = time.Since(start)
		sample.Err = fmt.Sprintf("create status=%d body=%s", create.StatusCode, integrationReadBody(t, create))
		return sample
	}

	var created jobs.Job
	integrationDecodeBody(t, create, &created)
	if created.ID == "" {
		sample.Wall = time.Since(start)
		sample.Err = "empty job id"
		return sample
	}

	var completed jobs.Job
	deadline := time.Now().Add(5 * time.Minute)
	for {
		response := integrationAPIRequest(
			t,
			http.MethodGet,
			baseURL+"/v1/jobs/"+created.ID,
			integrationAPIKey,
			"",
		)
		if response.StatusCode != http.StatusOK {
			sample.Wall = time.Since(start)
			sample.Err = fmt.Sprintf("get status=%d body=%s", response.StatusCode, integrationReadBody(t, response))
			return sample
		}
		integrationDecodeBody(t, response, &completed)
		switch completed.Status {
		case jobs.StatusSucceeded, jobs.StatusFailed:
			sample.Wall = time.Since(start)
			if completed.StartedAt != nil && completed.CompletedAt != nil {
				sample.Infer = completed.CompletedAt.Sub(*completed.StartedAt)
			}
			if completed.Status == jobs.StatusFailed {
				sample.Err = completed.Error
				if sample.Err == "" {
					sample.Err = "job failed"
				}
				return sample
			}
			if completed.Result == nil || strings.TrimSpace(completed.Result.Translation) == "" {
				sample.Err = "succeeded with empty translation"
				return sample
			}
			sample.Translation = completed.Result.Translation
			sample.PromptTok = completed.Result.PromptTokens
			sample.OutputTok = completed.Result.OutputTokens
			sample.TotalTok = completed.Result.TotalTokens
			return sample
		}
		if time.Now().After(deadline) {
			sample.Wall = time.Since(start)
			sample.Err = fmt.Sprintf("timeout status=%s", completed.Status)
			return sample
		}
		// Poll often; reported latency uses StartedAt/CompletedAt, not poll cadence.
		time.Sleep(25 * time.Millisecond)
	}
}

func printBenchSummary(t *testing.T, samples []benchSample) {
	t.Helper()

	type key struct {
		model  string
		target string
	}
	type agg struct {
		infers []time.Duration
		walls  []time.Duration
		outs   []string
	}

	firstByModel := map[string]benchSample{}
	warm := map[key]*agg{}

	for _, s := range samples {
		if s.Err != "" {
			continue
		}
		if s.Phase == "first" {
			firstByModel[s.Model] = s
			continue
		}
		k := key{model: s.Model, target: s.Target}
		a := warm[k]
		if a == nil {
			a = &agg{}
			warm[k] = a
		}
		if s.Infer > 0 {
			a.infers = append(a.infers, s.Infer)
		}
		if s.Wall > 0 {
			a.walls = append(a.walls, s.Wall)
		}
		a.outs = append(a.outs, s.Translation)
	}

	models := make([]string, 0, len(firstByModel))
	seen := map[string]struct{}{}
	for _, s := range samples {
		if _, ok := seen[s.Model]; ok {
			continue
		}
		seen[s.Model] = struct{}{}
		models = append(models, s.Model)
	}
	sort.Strings(models)

	t.Log("=== FIRST REQUEST (after oMLX unload of all loaded models) ===")
	t.Logf("%-20s %-28s %10s %10s %s", "catalog", "provider", "infer", "wall", "out")
	for _, model := range models {
		s, ok := firstByModel[model]
		if !ok {
			continue
		}
		t.Logf(
			"%-20s %-28s %10s %10s %q",
			s.Model,
			s.Provider,
			s.Infer.Round(time.Millisecond),
			s.Wall.Round(time.Millisecond),
			truncateRunes(s.Translation, 48),
		)
	}

	t.Log("=== WARM (infer = CompletedAt-StartedAt; avg over warm passes only) ===")
	t.Logf("%-20s %-22s %10s %10s %10s %s", "catalog", "target", "avg_infer", "min", "max", "sample_out")

	targets := make([]string, 0, len(benchTargets))
	for _, target := range benchTargets {
		targets = append(targets, target.language)
	}

	for _, model := range models {
		var all []time.Duration
		for _, target := range targets {
			a := warm[key{model: model, target: target}]
			if a == nil || len(a.infers) == 0 {
				t.Logf("%-20s %-22s %10s", model, target, "n/a")
				continue
			}
			avg := avgDuration(a.infers)
			minV, maxV := minMaxDuration(a.infers)
			all = append(all, a.infers...)
			out := ""
			if len(a.outs) > 0 {
				out = a.outs[len(a.outs)-1]
			}
			t.Logf(
				"%-20s %-22s %10s %10s %10s %q",
				model,
				target,
				avg.Round(time.Millisecond),
				minV.Round(time.Millisecond),
				maxV.Round(time.Millisecond),
				truncateRunes(out, 40),
			)
		}
		if len(all) > 0 {
			avg := avgDuration(all)
			minV, maxV := minMaxDuration(all)
			t.Logf(
				"%-20s %-22s %10s %10s %10s  (n=%d)",
				model,
				"*ALL*",
				avg.Round(time.Millisecond),
				minV.Round(time.Millisecond),
				maxV.Round(time.Millisecond),
				len(all),
			)
		}
	}
}

func avgDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	var sum time.Duration
	for _, value := range values {
		sum += value
	}
	return sum / time.Duration(len(values))
}

func minMaxDuration(values []time.Duration) (time.Duration, time.Duration) {
	minV, maxV := values[0], values[0]
	for _, value := range values[1:] {
		if value < minV {
			minV = value
		}
		if value > maxV {
			maxV = value
		}
	}
	return minV, maxV
}

func truncateRunes(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}
