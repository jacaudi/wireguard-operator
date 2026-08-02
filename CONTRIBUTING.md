# Contributing

This project is built with [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder);
read about that first. Fork the repository, make your changes, and open a PR.

Planned work is scoped in [`docs/ROADMAP.md`](docs/ROADMAP.md), with each item tracked as
a GitHub issue.

## Getting started

```console
make test        # runs manifests, generate, fmt, vet, then the envtest suite
make manifests   # regenerate CRDs and RBAC after changing api/
make generate    # regenerate deepcopy functions after changing api/
```

`make test` regenerates before running, so a stale CRD or an unformatted file fails
locally rather than in CI. Never hand-edit `zz_generated.deepcopy.go` or anything under
`config/crd/bases` — regenerate instead. `release.yaml` and `bundle/` are produced at
release time and should not be edited by hand either.

## Engineering practices

These apply to every change.

### TDD

Write the test first, watch it fail for the right reason, then make it pass. A test that
has never failed has proven nothing.

For changes that add behaviour this is literal. For **pure refactors** it is not — there
is no new behaviour to drive out, so writing new tests first would be cargo-culting.
On a refactor it means: establish a green baseline before touching anything, keep it green
after every step, and if an existing test fails, the refactor changed behaviour. Fix the
code, never the assertion.

### DRY

The hardest-won lesson in this codebase: `internal/resources` and the inline reconciler
functions were the same code twice, and they silently drifted apart. Do not create a second
copy of anything — a constant, a rendering path, a default value. One source of truth,
referenced.

Note the limit: two things that look alike but change for different reasons are not
duplication. Do not collapse them to satisfy the acronym.

### 12-Factor

Most factors are satisfied by Kubernetes itself. The ones that bite here:

- **Config** — configuration comes from the CRD or the environment, never hardcoded in more
  than one place. A default belongs in the CRD's `+kubebuilder:default`, with any Go
  constant existing only as a nil-fallback.
- **Processes** — builders stay pure functions of their inputs. No caching on the
  reconciler, no hidden state between calls.
- **Logs** — event streams to stdout via `logr`. No log files, no rotation.
- **Disposability** — fast startup, graceful shutdown.

### YAGNI

Build what the issue asks for. Do not add a config knob because someone might want it, and
do not generalise for a second implementation that does not exist. If you are writing an
abstraction with exactly one caller, stop.

### KISS

Prefer the boring solution. If a reviewer needs the design doc open to understand the diff,
it is too clever. Explicit repetition of three short lines beats a helper that takes four
parameters to avoid it.

### Cold review before the PR

Before opening a PR, get a review from a reviewer with none of the context that produced
the change — a fresh session, colleague, or agent — given only:

- the diff
- the issue text, including its acceptance criteria
- the repository

and explicitly **not** the implementation plan, the discussion that produced it, or any
reasoning about why choices were made.

Ask it to answer:

1. Does the diff do what the acceptance criteria say, no more and no less?
2. Is there scope creep — anything changed that the issue did not ask for?
3. Is anything wrong, unclear, or surprising to someone seeing it for the first time?
4. Are the tests testing behaviour, or asserting on implementation details?

An author who has been reasoning about a change for an hour cannot see what is unexplained
in it. If the cold reviewer misreads the diff, that is a finding about the diff. Address
the findings, then open the PR.
