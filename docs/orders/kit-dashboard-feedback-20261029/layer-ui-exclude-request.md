# Dashboard exclusion and optional layer-ui follow-up

Kit now provides separate Exclude project, Exclude model, Exclude harness and
Exclude host cards using the unchanged verified createValueSelect component.
Select hevmind-promo in Exclude project; remove its selected-value chip to undo.
The active filter button shows the selection, and URL state survives reload.
No layer-ui change blocks project exclusion. Tool exclusion is not implemented.

## Kit API contract

/api/sessions, /api/stats and /api/values accept repeated exclude_project,
exclude_model, exclude_harness and exclude_host parameters. For example:
`/api/sessions?window=30d&exclude_project=hevmind-promo`.
Project values accept a project name or a literal repository URL. Exclusions
combine with AND across facets, remove any of the repeated values, and win over
inclusions. Missing attributes do not match excluded values. Picker counts ignore
the facet's own inclusion but retain every exclusion, including its own, so they
count only surviving sessions. Selected excluded values remain removable chips
even when absent from the returned options. Reload and trace links preserve all
parameters. Clear filters removes exclusions too.

## Optional upstream component enhancement

To combine include and exclude in one picker, extend createValueSelect with
`excludedValue: string[] = []` and
`onFilterChange({value: string[], excludedValue: string[]})`.
Existing `value` and `onChange` must remain backward compatible when this mode
is unused. Provide an accessible Include/Exclude action on each option, distinct
selected and excluded chips (excluded chips say “Not <value>”), individual removal,
and Clear all for both arrays. Moving a value between modes removes it from the
other array. Labels and summaries must explain OR inclusions and excluded values;
never claim an excluded selection is an inclusion. Preserve excluded chips across
loading, empty option results, pagination, errors and search. Support keyboard
operation and disabled state. Kit would map excludedValue to exclude_<facet>.
Tool exclusion additionally needs a verified store array-negation predicate and
matching cached predicate in kit before it can be offered.

Upstream tests: include/exclude toggles and callback payload; moving between modes;
chip removal and clear; keyboard and accessibility labels; disabled state;
selected/excluded values absent from options; async errors and stale responses;
counts remain server supplied; existing inclusion-only clients unchanged. Rebuild
with npm run build:runtime, then import via scripts/import-layer-ui.py; never edit
kit's staged files. Kit integration acceptance: exclude hevmind-promo, verify list,
stats and counts, reload, remove the chip, and verify the original totals return.
