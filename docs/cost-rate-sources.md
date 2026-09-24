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
