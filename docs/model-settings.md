# Hy-MT2-30B-A3B model settings

This document separates settings published by the model authors from
project-level experimental profiles. Values with different provenance must not
be presented together as official recommendations.

## Official model-card settings

The inference sections of the
[Hy-MT2-30B-A3B model card](https://huggingface.co/tencent/Hy-MT2-30B-A3B)
and the
[GGUF model card](https://huggingface.co/tencent/Hy-MT2-30B-A3B-GGUF)
publish the following settings for 30B-A3B (checked 2026-07-25):

| Setting | Published value |
| --- | ---: |
| `temperature` | `0.7` |
| `top_p` | `1.0` |
| `top_k` | `-1` |
| `repetition_penalty` | `1.0` |
| `max_tokens` | `4096` |
| Default system prompt | None |
| Stop sequence | No separate value published |

The Transformers example places the translation prompt in one user message,
applies the model chat template, and uses `max_new_tokens=4096`.

## Project experimental profiles

The profiles below are not Tencent recommendations. They are starting points
for measuring repeatability and format-contract compliance. Evaluate them by
language pair, document type, inference backend, and quantization.

| Profile | temperature | top_p | top_k | repetition penalty | Maximum output tokens |
| --- | ---: | ---: | ---: | ---: | ---: |
| Published baseline | 0.7 | 1.0 | -1 | 1.0 | 4096 |
| Low-variance candidate | 0.1 | 1.0 | Disabled | 1.0 | Request-specific, capped at 4096 |
| Greedy comparison | 0.0 | 1.0 | Disabled | 1.0 | Request-specific, capped at 4096 |

Operational cautions:

- The value that disables `top_k` may be `-1`, `0`, or an omitted field,
  depending on the backend. An inference adapter must translate this semantic
  setting into the backend-specific representation.
- `temperature=0` does not guarantee bit-for-bit determinism. Parallel
  execution, quantization, hardware, and backend versions can affect output.
- Do not assume a fixed source-to-target token ratio. Measure expansion by
  language pair and format when calculating output limits.
- Increasing repetition penalty can remove legitimate repeated sentences,
  keys, or delimiters. Keep `1.0` in the initial comparison profiles.

## Stop sequences

The default project profile does not add a custom stop sequence. Source data
may contain common separator strings or Markdown fences, so an application
stop string can truncate a valid translation.

Use the chat template's EOS behavior and a maximum output-token limit. Record
any stop tokens inserted automatically by the backend.

If an application requires a custom stop sequence:

1. Verify that it does not occur in the source or generated prompt.
2. Use a value that cannot appear in valid structured output.
3. Treat truncated JSON, YAML, HTML, or XML as a validation failure.

## Comparison plan

Capture actual model output for the cases in `lib/testdata/cases.json`, then
measure these dimensions separately:

- structural-contract pass rate;
- translation quality by language pair;
- glossary compliance;
- average and worst-case output token count;
- variation across repeated runs;
- differences between original weights and GGUF quantizations.
