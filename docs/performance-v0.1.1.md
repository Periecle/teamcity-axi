# Go v0.1.1 performance and release

v0.1.1 removes redundant local work while preserving the read-only v0.1 contract.
It includes the earlier post-release schema/hint improvements and adds renderer,
sanitizer, text-boundary, environment and portable-guidance changes. The previous
release and its reports remain available at v0.1.0; the earlier optimized source
comparison is retained in [performance.md](performance.md).

## Why the historical TS model session was faster

Historical TS completed the model sample at a 37.3-second median, released Go at
55.4 seconds, and the first optimized Go checkpoint at 44.3 seconds. Those are
whole model sessions, including reasoning and protocol round trips. They do not
measure only the implementation language. Scripted TOON workflows measured
303 ms in TS, 111 ms in released Go, and 103 ms in the first optimized Go sample.
Median cumulative tool execution in the optimized model sample was 92 ms; total
tool execution was 0.28% of total session time. Native-control session medians
also changed substantially between runs. Different full prompts, scheduling and
model backend state prevent a controlled language comparison.

The old Go traces contained help/schema discovery, repeated reads and unnecessary
expansion. Lazy schema compilation, precise truncation hints and first-read
commands addressed these observable causes: total model calls fell 30 to eight,
with one call per wrapper task, while all eight tasks remained successful.
Wrapper output was still larger than native output, and historical TS session
latency was still lower. These limitations are retained rather than dismissed.

## Additional changes in v0.1.1

- Share one detached JSON tree between initial envelope validation, payload
  validation and serialization, removing two repeated conversions on that path.
  Keep canonical number handling, omitempty, sorted JSON keys and both gates.
- Avoid allocating a text builder for clean valid UTF-8. Skip ANSI regex work
  when no escape is present, and replacement allocation when the exact secret
  regex does not match. Preserve both known-secret passes and all regex checks.
- Replace copied sanitizer path slices with equivalent root/meta protection
  state. No global output or secret cache is added.
- Bound excerpts at rune byte offsets, without materializing whole rune slices.
  Reuse the actual excerpt truncation result rather than sanitizing/counting the
  same input again. Fingerprint inputs and exact retrieval scopes are unchanged.
- Compile fixed header/version regexes once. Snapshot the command environment
  once for context and public rendering, and reuse its known secrets on fallback.
  Native session acquisition still performs its own version and authority checks.
- Reduce portable guidance from 6,721 to 4,625 bytes (31%), keeping first-read
  syntax, item/source identities, unknown/partial semantics, status/watch rules,
  untrusted-evidence handling and trusted configuration boundaries. The task
  corpus, task prompts, model effort, safety caps and grading are unchanged.

## In-process measurements

Three samples per benchmark and condition use Go 1.27.1, 150 ms per sample,
on the same Linux amd64 host. Baseline product source is `6f7a1f2`; the new
benchmark harness also runs against that baseline. All initial candidate samples
are retained. See [raw and parsed measurements](../evaluations/results/v0.1.1/microbenchmarks.json).

| Renderer | Baseline | v0.1.1 | Allocations before → after |
| --- | ---: | ---: | ---: |
| Exact status, JSON | 535 µs | 388 µs | 3291 → 1420 |
| Exact status, TOON | 619 µs | 436 µs | 3627 → 1754 |
| Partial failure, JSON | 968 µs | 629 µs | 6715 → 2697 |
| Partial failure, TOON | 1074 µs | 746 µs | 7357 → 3338 |
| Compacted notes, JSON | 1700 µs | 1203 µs | 13181 → 3858 |
| Compacted notes, TOON | 1711 µs | 1249 µs | 13265 → 3941 |

Rendering falls 27–35% and allocation counts 52–71% in these cases. Plain ASCII
and Unicode sanitizer allocations fall 29/27 to zero per call, with observed
median time improvements of 11%/14%. Control-bearing secret text remains fully
sanitized. Sequential host samples are exploratory, not a performance guarantee
or proof of whole-session benefit.

## Remaining opportunities and scope limits

Model startup/reasoning/round trips dominate the retained traces; changing model
or effort requires a separately controlled quality/latency evaluation. Native
CLI startup and independent evidence HTTP requests remain a cost. Persistent
caching, long-lived workers or direct HTTP transport would require freshness,
authority, cleanup and compatibility work. Required reads cannot be removed
merely to make the wrapper cheaper than a partial native workflow.

Byte-budget reduction still serializes each retained optional-change prefix;
optimizing it requires exact-boundary and nonmonotonic-format evidence. Some
normalization and sanitizer passes are retained deliberately: sanitization is
not idempotent for arbitrary short secrets, and normalization detaches caller
objects and preserves JSON semantics. No unsafe global secret cache, weakened
schema validation, reduced required evidence or larger budgets were introduced.

The local savings and preserved behavior justify this release without CPU
profiling or more complex architecture. Output-token parity with native and a
statistically controlled whole-session TS comparison remain separate unmet
objectives unless new measurements explicitly establish them.
