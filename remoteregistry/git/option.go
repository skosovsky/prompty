package git

// Option configures Fetcher.
type Option func(*Fetcher)

// WithBranch sets the branch to clone (e.g. "main"). Default is "main".
func WithBranch(branch string) Option {
	return func(g *Fetcher) {
		g.branch = branch
	}
}

// WithDir sets the subdirectory within the repo to read manifests from (e.g. "prompts").
// Default is "" (repo root).
func WithDir(dir string) Option {
	return func(g *Fetcher) {
		g.dir = dir
	}
}

// WithDepth sets the clone depth (number of commits). Default is 1 (shallow clone).
// Use 0 for full clone.
func WithDepth(depth int) Option {
	return func(g *Fetcher) {
		g.depth = depth
	}
}

// WithAuth sets the token for HTTPS auth (e.g. GitHub/GitLab personal access token).
// Used as BasicAuth username "x-access-token" with password token.
func WithAuth(token string) Option {
	return func(g *Fetcher) {
		g.authToken = token
	}
}
