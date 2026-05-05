# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A single-file Go CLI tool that fetches all FeatureScript elements from the Onshape Standard Library document, writes them as `.fs` files into a local git repository, and commits the result on two branches: `with-versions` (tracks Onshape document version names) and `without-versions` (tracks only file content, no version metadata). The tool is designed to be run on a schedule; it exits early with a message if the repo is already at the latest Onshape version, making it safe to call repeatedly without producing spurious commits.

## Build and run

```sh
go build         # produces ./os-std-importer binary
```

```sh
./os-std-importer --settings=<file> --out=<dir> [-d] [-v] [--onshape-doc-url=<url>]
# e.g.: ./os-std-importer --settings=local.json --out=../os-std
# e.g.: ./os-std-importer --settings=remote.json --out=../os-std -d
```

- `--settings` and `--out` are required.
- `--onshape-doc-url` is optional; defaults to the canonical Onshape Standard Library document URL.
- `-d` checks the latest Onshape version and reports whether an update is needed, but does not download files or create commits.
- `-v` prints each element name as it is fetched.

## Credentials

The settings file (gitignored) selects the API endpoint. `-settings` defaults to `remote.json`; see `remote.json.template` for all fields.

- `useProxy: false` — direct Onshape API; requires `accessKey` + `secretKey`
- `useProxy: true` — via a Reframe proxy; requires `onshapeKey` + `proxyKey` and a `proxyURL` (production: `https://onshape.reframe.quest`, local dev: `http://localhost:5080`)

The pre-commit hook in `hooks/pre-commit` blocks commits that include credential files. To install: `cp hooks/pre-commit .git/hooks/pre-commit`.

## Two-branch flow

The tool maintains two branches in the `-out` repo:

- **`with-versions`** — each commit appends an entry to `import-log.toml` (a TOML history file) just before the commit. The commit message includes the version name.
- **`without-versions`** — a parallel branch that receives an identical file tree but without `import-log.toml`. Useful for diffing purely on content changes, ignoring version label noise.

Both branches must already exist in the repo before the tool is run. The tool does NOT create branches and does NOT push to any remote — pushing is the caller's responsibility.

## `import-log.toml` contract

`import-log.toml` lives at the root of the `-out` repo and records a history of every import run as an array of TOML tables, each with `version` and `retrieved` fields:

```toml
# Onshape Standard Library import history.
# Maintained by os-std-importer; do not edit manually.

[[entry]]
  version = "Start"
  retrieved = "2026-01-01"

[[entry]]
  version = "1945.0"
  retrieved = "2026-05-05"
```

At startup, the tool reads the last entry's `version` from `import-log.toml` on the `with-versions` branch (via `git show with-versions:import-log.toml`). If it matches the latest Onshape document version name, the tool exits immediately with:

```
Repo is at or ahead of Onshape document version
```

On each successful import, a new entry is appended before the `with-versions` commit.

## Architecture

Everything lives in `main.go` (single `main` package). Git operations are performed by shelling out to `git`. API calls use `net/http` directly with basic auth.

**Dependencies:**
- `github.com/jnewth/goutil` — provides `Verify`, `Verboseln`, and `Verbosef` helpers used throughout.
- `github.com/BurntSushi/toml` — reads and writes `import-log.toml`.

**Key responsibilities of `main.go`:**
- Parse flags and load credentials via `loadSettings`
- Resolve the target Onshape document (default or user-supplied URL)
- Fetch the list of document versions; extract the latest version name
- Read the last entry from `import-log.toml` on `with-versions`; exit early if already current
- Fetch all FeatureScript elements from the document
- Write `.fs` files into `-out`, then commit on `with-versions` (with `import-log.toml`) and `without-versions` (without `import-log.toml`)
- `-dry-run` short-circuits after the version check, before any file I/O or git operations
