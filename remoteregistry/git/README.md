# Git registry for prompty

Provides a `remoteregistry.Fetcher` that reads YAML prompt manifests from a Git repository. The repo is cloned (or pulled if already present) on first use; you pass this Fetcher to `remoteregistry.New` to get a `Registry` with TTL cache.

## Install

```bash
go get github.com/skosovsky/prompty/remoteregistry/git
```

## Configuration

- **Repo URL:** required in `NewFetcher(repoURL, opts...)`. Use HTTPS; for private repos use `WithAuth(token)` (e.g. GitHub/GitLab personal access token). Auth is sent as BasicAuth with username `x-access-token` and password equal to the token.
- **Cache:** the Fetcher does not implement TTL itself; pass the Fetcher to `remoteregistry.New(fetcher, remoteregistry.WithTTL(d))` to control cache duration. Use `Evict`/`EvictAll` and `Close()` on the `remoteregistry.Registry` for cleanup.

## Capabilities

- **Usage:** `git.NewFetcher(repoURL, opts...)` returns a `*git.Fetcher` implementing `remoteregistry.Fetcher`. Pass it to `remoteregistry.New(fetcher, opts...)` to get a `prompty.Registry`. Resolve templates by id; manifest resolution is `{dir}/{id}.yaml` or `{dir}/{id}.yml` (see `WithDir`).
- **Clone behavior:** on first `Fetch`, the repo is cloned (or opened and pulled if `WithCloneDir` was used and the directory already contains a clone). Shallow clone by default (depth 1); use `WithDepth(0)` for a full clone.
- **Options:**
  - `WithBranch(branch)` — branch to clone/pull (default `main`).
  - `WithDir(subdir)` — subdirectory inside the repo to read manifests from (default repo root).
  - `WithDepth(depth)` — clone depth; 1 = shallow, 0 = full clone.
  - `WithAuth(token)` — HTTPS auth (e.g. personal access token).
  - `WithCloneDir(dir)` — persistent directory for the clone. If `dir` already has a `.git`, the Fetcher uses it and runs pull; otherwise it clones into `dir`. `Close()` does not remove this directory.
- **Resources:** call `Close()` on the Fetcher (or the registry that holds it) to release the clone. When not using `WithCloneDir`, the temporary clone is removed on `Close()`.

See [pkg.go.dev](https://pkg.go.dev/github.com/skosovsky/prompty/remoteregistry/git) for the full API.
