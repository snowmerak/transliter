# Model-specific settings

Each built-in model package owns its generation options. This document keeps
official recommendations, official examples, and project experiments
explicitly separated.

Sources were checked on 2026-07-25:

- [Hy-MT2-30B-A3B](https://huggingface.co/tencent/Hy-MT2-30B-A3B)
- [Hy-MT2-30B-A3B-GGUF](https://huggingface.co/tencent/Hy-MT2-30B-A3B-GGUF)
- [TranslateGemma 4B IT](https://huggingface.co/google/translategemma-4b-it)
- [TranslateGemma technical report](https://arxiv.org/abs/2601.09012)

## Option provenance

`GenerationOptions.Provenance` has three possible values:

| Value | Meaning |
| --- | --- |
| `official_recommendation` | The model authors explicitly recommend these values |
| `official_example` | The values appear in an official usage example but are not presented as general recommendations |
| `project_experimental` | A project-defined comparison profile requiring evaluation |

Adapters must omit nil option fields. They must not substitute backend defaults
and then report those defaults as model recommendations.

## Hy-MT2 official recommendations

Tencent publishes one profile for 1.8B and 7B, and a different profile for
30B-A3B:

| Model | temperature | top_p | top_k | repetition penalty | max output tokens |
| --- | ---: | ---: | ---: | ---: | ---: |
| Hy-MT2 1.8B | 0.7 | 0.6 | 20 | 1.05 | 4096 |
| Hy-MT2 7B | 0.7 | 0.6 | 20 | 1.05 | 4096 |
| Hy-MT2 30B-A3B | 0.7 | 1.0 | -1 | 1.0 | 4096 |

All three packages return
`ProvenanceOfficialRecommendation` for `ProfileOfficial`.

Tencent also states that the models have no default system prompt. No separate
stop sequence is published.

## Hy-MT2 deterministic experiment

`ProfileDeterministic` changes temperature to `0.1` and preserves the remaining
size-specific official values. It returns
`ProvenanceProjectExperimental`.

This is not a Tencent recommendation. Compare it against the official profile
for every language pair, format, backend, and quantization.

## TranslateGemma official example

TranslateGemma is designed for a strict chat template. Its official direct
initialization example uses:

```text
do_sample=false
max_new_tokens=200
```

The model card presents `200` as an example value, not a universal maximum or
recommended production default. Therefore `ProfileOfficial` returns
`ProvenanceOfficialExample`, not `ProvenanceOfficialRecommendation`.

Temperature, top-p, top-k, and repetition penalty remain nil because the
official direct example does not specify them.

TranslateGemma's model card states a total input context of 2K tokens. The
capability metadata reports `MaxInputTokens: 2048`.

## TranslateGemma deterministic profile

The project deterministic profile keeps:

```text
do_sample=false
max_output_tokens=200
```

but marks the result `ProvenanceProjectExperimental` so applications do not
confuse a project default with an official recommendation.

The application should calculate an output budget from its workload and
backend constraints rather than treating 200 as sufficient for arbitrary
documents.

## Backend normalization

Inference adapters must map the neutral option fields carefully:

- a disabled `top_k` may be represented as `-1`, `0`, or an omitted field;
- `MaxOutputTokens` may map to `max_tokens` or `max_new_tokens`;
- `DoSample=false` may need to suppress temperature and sampling fields;
- stop tokens inserted by a chat template differ from application stop
  strings;
- GGUF and original-weight backends can produce different output under the
  same nominal settings.

## Stop sequences

No built-in profile adds a custom stop string. Source data can contain common
separators or Markdown fences, so an application-defined stop value can
truncate valid output.

If a runtime adds one:

1. verify that it does not occur in source content or model input;
2. use a value that cannot appear in valid structured output;
3. treat truncated JSON, YAML, HTML, or XML as a validation failure.

## Comparison plan

Capture actual output for `lib/testdata/cases.json` and measure:

- structural-contract pass rate;
- quality by model, size, language pair, and prompt kind;
- glossary compliance;
- average and worst-case output tokens;
- variation across repeated runs;
- original weights versus GGUF quantization.
