# Decisions and deviations from the plan

Anything the implementation does differently from `nines-plan.md`, or where the plan left a choice open.

## Deviations

1. **License stays MIT.** The plan says Apache-2.0, but the repository was created with an MIT `LICENSE`. Changing a
   license is the owner's call, so it was left alone.
2. **Improvement potential is the exact gain.** Section 6 defines it as `I(X) * (1 - A_X)` and calls that "exactly the gain
   from making X perfect". That holds only for a component with a single variable. A component with
   `transient_fraction` owns several variables, and the product overstates the gain (in `testdata/models/checkout.yaml`
   it reports +0.0999 pp for `payments` where the real gain is +0.02 pp, because retries already absorb the
   transient failures). Confirmed with the project owner: levers are ranked by `P(X perfect) - P`. Birnbaum
   importance `I(X)` is still reported as `importance`. For single-variable components the two definitions agree
   (tested).
3. **Dependencies.** Only `cobra`, `yaml.v3` and `testify` (tests) were added. `pflag` and `mousetrap` come with cobra.
4. **Module path** is `github.com/marcelblanarik/nines`, matching the git remote.

## Choices the plan left open

- **Group variable.** A reference to a component in a group is ANDed with the group component's *hard* variable only
  (`G.hard`), and groups chain (a group may itself have a group). A group's failure is persistent by nature; if `G` also
  has `transient_fraction`, only its persistent part affects members.
- **Soft variables exist only when `transient_fraction > 0`.** With `r = 0` they would have probability 1, so they are
  not created and a component is one variable.
- **Point estimate of an uncertain availability** (used by `check` without `--uncertainty`, by levers, ceiling,
  solve): the midpoint for an interval, `good / total` for counts. The uncertainty run samples uniformly and from
  `Beta(good + 1, bad + 1)`, so its mean for counts is `(good + 1) / (total + 2)`, slightly different from the point
  estimate when `total` is small.
- **Operator variables** (`fallback.coverage`, `cache.hit`, `optional.untouched`) are fixed-probability variables with
  no component, so they never show up as levers or single points of failure.
- **Naive mode** gives every occurrence of a component, and every group reference, its own variables. If the naive BDD
  is too large the comparison is skipped with a warning.
- **Monte Carlo fallback** (BDD over `--max-bdd-nodes`): reports the estimate with a 95% Wilson interval and the
  ceiling by sampling. Levers, single points of failure, uncertainty and `explain`/`solve` need the BDD and are
  unavailable. The recommended tier uses the lower end of the interval.
- **Verdict float tolerance.** Comparisons against the target and tiers allow `1e-12`, so a model that computes exactly
  99.95% meets a 99.95% target.
- **`window`** is parsed as `Nd` and used for the downtime-minutes display (default 30d). It does not change any
  probability.
- **Validation stricter than the plan:** unknown YAML keys are errors (catches typos), YAML aliases are rejected,
  `target` must be below 100, `retry.attempts` is capped at 1000, `requires` is capped at depth 64 and 20,000 nodes,
  and counts must be integers with `0 <= good <= total`. These keep the validator safe on hostile input.
- **`--remove` semantics.** A removed component disappears from the requires tree: `all`/`any`/`k_of_n` lose that
  member (k_of_n keeps its k and errors if k > n); a node left with no members disappears; `retry`/`cached`/`optional`
  of it disappear; a fallback whose primary is removed disappears, and one whose fallback is removed becomes its
  primary. Components that were in the removed group lose the group. Removing something that is redundant therefore
  lowers availability, and removing a hard dependency raises it.
- **`nines fmt`** rewrites the model in canonical form and drops comments.
- **`solve`** rounds the printed percentage up (8 decimals) so pasting it back into the model still meets the target.
- **Cut sets** list order-2 pairs ranked by joint unavailability. They are skipped (with a warning) above 60
  components that appear in the model.
