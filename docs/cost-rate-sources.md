# Dated standard API rate evidence

Verified 2026-09-09. Amounts are USD per million tokens; these are list
rates, not subscription bills.

| Model | Earliest supported date | Input / output | Dated source |
|---|---|---|---|
| Sonnet 5 | 2026-06-30 | 2 / 10 | [Launch and August 10 amendment](https://www.anthropic.com/news/claude-sonnet-5): introductory rates became permanent; the proposed September 1 increase was cancelled. |
| Haiku 4.5 | 2025-10-15 | 1 / 5 | [Launch](https://www.anthropic.com/news/claude-haiku-4-5). The model ID's October 1 suffix is not its availability date. |

The provider's [dated cache announcement](https://claude.com/blog/prompt-caching)
states a general 1.25× write and 0.1× read policy. The migrated page displays
August 14, 2025 and a December 17, 2024 GA amendment; both precede these model
launches. Applying that policy to the dated base rates gives Sonnet 2.5/0.2
and Haiku 1.25/0.1 for five-minute writes/reads. The
[current pricing table](https://platform.claude.com/docs/en/about-claude/pricing)
confirms those values. This derivation is explicit, not a backdate of the
current table alone.

[Anthropic's dated cache-duration measurement](https://platform.claude.com/docs/en/about-claude/models/optimizing-for-cost-and-intelligence)
(reference 16) ran on August 23, 2026 on Sonnet 5 and Opus 5, using list
prices in effect at measurement. Its cache-duration section and pricing code
specify the 2× one-hour rate. Separate August 23 rows therefore add Sonnet's
$4 and Opus's $10 one-hour writes; this does not establish Haiku's earlier
one-hour price. Before August 23 those one-hour categories remain unknown. Other historical
models and service tiers remain unverified; eval time-window dollar joins
are excluded from the exact-session comparison.

[OpenAI's changelog](https://developers.openai.com/api/docs/changelog) dates
Sol's $4/$20 promotion to August 21, 2026, and Astra's release to September 3.
The [Sol model page](https://developers.openai.com/api/docs/models/gpt-5.6-sol)
and [Astra model page](https://developers.openai.com/api/docs/models/gpt-6-astra)
confirm current cache and long-context rates, but those dated changelog entries
do not specify the historical cache/long-context prices. Their complete rate
rows therefore keep the September 8 observation boundary; the release date
alone is not a complete dated rate table.


[Claude's September 1 release note](https://platform.claude.com/docs/en/release-notes/overview)
explicitly dates Fable 5.1's $10/$50 base and $0.25 cache-read price, with
unchanged cache writes. Reference 19 of the dated provider experiment above
records launch-snapshot writes of $12.50/$20. Its historical row starts on
September 1, not the private prelaunch experiment dates.

## November 2026 additions (verified 2026-11-20)

| Model | Effective | Input / cached / output | Dated source |
|---|---|---|---|
| Opus 5.5 | 2026-09-22 | 4 / 0.20 / 20 | [Release notes](https://platform.claude.com/docs/en/release-notes/overview), September 22: "$4 / $20 USD per MTok" |
| Sonnet 5.5 | 2026-09-28 | 2 / 0.20 / 10 | Release notes, October 7: reads cut "from $0.20 USD to $0.10 USD", "all other prices are unchanged" since launch |
| Sonnet 5.5 | 2026-10-07 | 2 / 0.10 / 10 | Same October 7 entry |
| Fable 5.2 | 2026-11-09 | 8 / 0.20 / 40 | Release notes, November 9: "$8 USD per MTok of input and $40 USD per MTok of output" |
| Haiku 5.5 | 2026-11-20 | 0.10 / 0.01 / 0.50; 5x over 100k | [Pricing table](https://platform.claude.com/docs/en/about-claude/pricing) only; the October 7 launch note gives no prices |
| Opus 4.5–4.8, Sonnet 4.5/4.6, Fable 5 | 2026-11-20 | per pricing table | Pricing table observation |
| gpt-6-sol / gpt-6-luna | 2026-09-22 | 2 / 0.20 / 10; 0.10 / 0.01 / 0.50 | [OpenAI changelog](https://developers.openai.com/api/docs/changelog), September 22 |
| gpt-6.1-sol | 2026-09-29 | 2 / 0.10 / 10, writes 2.50 | Changelog, September 29 |
| gpt-6.1-terra, gpt-6.2-sol, gpt-6.2-astra | 2026-11-20 | 1/0.05/5; 2/0.10/10; 8/0.40/40 | [Pricing page](https://developers.openai.com/api/docs/pricing) observation; their changelog entries give no rates |
| gemini-4-carbon | 2026-11-10 | 2 / 0.10 / 10 | [Carbon availability post](https://blog.google/innovation-and-ai/models-and-research/gemini-models/gemini-4-carbon-now-available/), November 10, plus the [Gemini API pricing page](https://aistudio.google.com/docs/pricing) "$0.10 through November 30" cache row |
| gemini-4-carbon | 2026-12-01 | 4 / 0.20 / 20 | Same post's footnote and the pricing page's scheduled December 1 price |
| gemini-4-neon | 2026-11-18 | 3 / 0.15 / 15 | [Neon launch post](https://blog.google/innovation-and-ai/models-and-research/gemini-models/introducing-gemini-4-neon/), November 18 |
| gemini-4-argon, gemini-4-hydrogen | 2026-11-20 | 2/0.10/10 (4/0.20/20 from Dec 1); 1.50/0.075/7.50 | Pricing page observation |

Derivations, stated rather than backdated:

- Claude cache writes for the dated rows apply the provider's dated 1.25x
  (five-minute) and 2x (one-hour) policy above; Opus 5.5 and Sonnet 5.5
  launched after the August 23 measurement that established 2x.
- Opus 5.5's 0.05x read and Fable 5.2's 0.025x read come from the pricing
  table's footnotes, not the launch notes. The October 7 note moved Sonnet 5.5
  to 0.05x "instead of 0.1x" and did not list Opus 5.5, which is consistent with
  Opus 5.5 already being 0.05x.
- OpenAI rows without a stated write price use 1.25x input, the ratio the
  6.1 Sol changelog entry states. Long-context rows follow the pricing page:
  over 272K input, 2x input and 1.5x output.
- Gemini 4 models support implicit caching only, so there is no write charge.
  `creation` equals `input` so any reported cache-creation tokens bill as input.
- `gemini-live` appears only as a `dot` test fixture model. It is not a Gemini
  API model ID (the API has `gemini-3.8-live`), so it stays unpriced.
- GPT-5.6 Sol's $4/$20 is promotional "until November 30, 2026"; its later
  rate is not published, so requests from December 1 keep the current row
  until a dated row is added.

## How traces use the table

`prices.toml` is embedded in the binary (`prices.go`), and the trace dashboard
prices each request from it. `hev cost` still reads the copy in `--dir`. A
transcript model ID matches its row exactly, or after dropping a provider
prefix, `[1m]`/`@snapshot` tag, `-YYYYMMDD` snapshot date and effort suffix
(`-low` … `-xhigh`, `-max`), so `gemini-4-neon-medium` bills as
`gemini-4-neon` and `claude-haiku-4-5` as `claude-haiku-4-5-20251001`. Version
digits are never dropped: `claude-opus-5-5` never bills as `claude-opus-5`.

A trace request whose model has no row costs $0, and its session reports
`rates_known: false`, so the dashboard shows the ≈ marker. A request before
the model's first dated row (or with no timestamp) is priced at the nearest
row and also marks the session approximate. `hev cost` stays strict: those
requests are unpriced. PI, and OpenCode when it records `cost` (including a
free model's `cost: 0`), keep the harness's own figure.
