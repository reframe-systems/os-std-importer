# os-std-importer

A Go CLI tool that fetches all FeatureScript elements from the [Onshape Standard Library](https://cad.onshape.com/documents/12312312345abcabcabcdeff/w/a855e4161c814f2e9ab3698a) document and commits them into a local git repository. Designed to run on a schedule; exits immediately with no changes if the repo is already at the latest Onshape version.

## Output

The imported library is published at **[jnewth-reframe/os-std-mirror](https://github.com/jnewth-reframe/os-std-mirror/)**, maintained on two branches:

- **`with-versions`** — each commit mirrors an Onshape document version exactly, including `import-log.toml` (a history of all imports with version name and retrieval date). Commit messages include the Onshape version name.
- **`without-versions`** — identical file tree, but with FeatureScript version strings stripped and `import-log.toml` omitted. Useful for diffing purely on content changes across Onshape releases.

## How it works

1. Fetches the list of versions for the target Onshape document and identifies the latest.
2. Reads `import-log.toml` from the `with-versions` branch of the output repo. If the latest Onshape version is already recorded there, exits immediately — safe to call repeatedly.
3. Downloads all FeatureScript elements from that document version.
4. Writes `.fs` files into the output repo, appends an entry to `import-log.toml`, and commits to `with-versions`.
5. Checks out `without-versions`, copies the `.fs` files over, strips version strings, and commits (without `import-log.toml`).

The tool never creates branches or pushes — both branches must already exist, and pushing is the caller's responsibility.

## Build and run

```sh
go build    # produces ./os-std-importer
```

```sh
./os-std-importer [-settings=<file>] [-out=<dir>] [-dry-run] [-verbose] [onshape-doc-url]
```

```sh
# typical usage
./os-std-importer --settings=remote.json --out=../os-std-mirror

# check whether an update is available without downloading anything
./os-std-importer --settings=remote.json --out=../os-std-mirror -d
```

| Flag | Required | Description |
|------|----------|-------------|
| `--settings=<file>` | yes | Credentials/endpoint config; see `remote.json.template` |
| `--out=<dir>` | yes | Output git repo to commit into |
| `-d` | no | Dry run: check version only, no download or commit |
| `-v` | no | Verbose: print each element name as fetched |
| `--onshape-doc-url=<url>` | no | Target document (default: Onshape standard library) |

## Credentials

The settings file is gitignored. Copy `remote.json.template`, name it `remote.json` (the default), and fill in your keys:

| `useProxy` | Auth fields | Endpoint |
|-----------|-------------|----------|
| `false` | `accessKey` + `secretKey` | Onshape API directly |
| `true` | `onshapeKey` + `proxyKey` | Reframe proxy — set `proxyURL` to `https://onshape.reframe.quest` for production or `http://localhost:5080` for local dev |

## Pre-commit hook

A hook is included that blocks accidental commits of credential files:

```sh
cp hooks/pre-commit .git/hooks/pre-commit
```
