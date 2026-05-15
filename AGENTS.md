# AGENTS.md

This file provides guidance to coding agents (e.g. Claude Code, claude.ai/code) when working with code in this repository.

## Repository purpose

Go module `go.klusters.dev/capi-config` — a CLI that rewrites Cluster API (CAPI) cluster YAML to inject AppsCode-specific network/IAM configuration before the manifests are applied. One subcommand per CAPI provider: AWS (CAPA), Azure (CAPZ), GCP (CAPG), and "K" / KubeVirt (CAPK). Each command reads a stream of unstructured CAPI objects (Cluster, MachinePool, the provider-specific control-plane/managed-machine-pool kinds), patches the relevant fields (VPC CIDRs, subnet IDs, IAM role annotations, etc.), and writes the result back.

The produced binary is `capi-config`. It is meant to run as a step in the cluster-creation pipeline — not as a long-running operator.

## Architecture

- `cmd/capi-config/` — entry point (`main.go`, `version.go`).
- `pkg/cmds/` — Cobra command tree:
  - `root.go` — registers per-provider commands.
  - `completion.go` — shell completion.
- `pkg/cmds/config/` — one file per provider, plus shared helpers:
  - `capa.go` — AWS (CAPA). Patches `AWSManagedControlPlane`, `AWSManagedMachinePool`, `MachinePool`, `Cluster`. Uses `eks.amazonaws.com/controlplane-role` / `eks.amazonaws.com/machinepool-role` annotations.
  - `capz.go` — Azure.
  - `capg.go` — GCP.
  - `capk.go` — KubeVirt.
  - `common.go` — shared YAML stream/unstructured plumbing built on `kmodules.xyz/client-go/tools/parser`.
  - `constants.go` — shared kind/annotation strings.
- `hack/`, `Makefile` — AppsCode build harness (everything runs inside `ghcr.io/appscode/golang-dev`). This binary ships for **5 platforms**: linux amd64/arm/arm64 plus `windows/amd64`, `darwin/amd64`, and `darwin/arm64` (see `BIN_PLATFORMS` in the Makefile).
- `vendor/` — checked-in deps.

The CLI doesn't connect to a cluster; it transforms YAML on stdin/files. Provider-specific patching strategies are isolated to their respective files in `pkg/cmds/config/`.

## Common commands

All Make targets run inside `ghcr.io/appscode/golang-dev` — Docker must be running.

- `make ci` — CI pipeline: `verify check-license lint build unit-tests`.
- `make build` — build the binary for the host OS/ARCH into `bin/<os>_<arch>/capi-config`.
- `make all-build` — build for every `BIN_PLATFORMS` entry, including the Windows and macOS targets that are unusual for AppsCode operators.
- `make fmt` — gofmt + goimports.
- `make lint` — golangci-lint.
- `make unit-tests` / `make test` — Go unit tests.
- `make verify` — `verify-gen verify-modules`; `go mod tidy && go mod vendor` must leave the tree clean.
- `make add-license` / `make check-license` — manage license headers.

Run a single Go test (requires a local Go toolchain):

```
go test ./pkg/cmds/config/... -run TestName -v
```

There is no container target — this CLI does not ship as an image.

## Conventions

- Module path is `go.klusters.dev/capi-config` (vanity URL); imports must use that, not the GitHub URL.
- License: **AppsCode Community License 1.0.0** (`LICENSE.md`); new files need the standard AppsCode header (`make add-license`).
- Sign off commits (`git commit -s`); contributions follow the DCO (`DCO`, `CONTRIBUTING.md`).
- Vendor directory is checked in — `go mod tidy && go mod vendor` must leave the tree clean (enforced by `verify-modules`).
- Add a new provider by dropping a `pkg/cmds/config/cap<x>.go` file and registering it in `pkg/cmds/root.go` next to the existing `NewCmdCAP{A,G,K,Z}` calls. Keep provider code self-contained — share only via `common.go` / `constants.go`.
- All YAML manipulation goes through `kmodules.xyz/client-go/tools/parser` and `unstructured.SetNestedMap` — do **not** use ad-hoc `text/template` or string replacement.
- Kind/annotation strings used to identify CAPI objects are part of the user contract — keep them in `constants.go`, not inline.
- The CLI ships as a host binary on linux/windows/darwin; do not pull in linux-only deps.
