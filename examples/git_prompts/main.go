// Git prompts example: load a prompt from a Git repo (remoteregistry/git) and call Gemini.
// Run from this directory: go run .
// Requires GEMINI_API_KEY. Uses a temporary local git repo for demo; set GIT_REPO_URL to use your own repo.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty/adapter"
	geminiadapter "github.com/skosovsky/prompty/adapter/gemini"
	"github.com/skosovsky/prompty/parser/yaml"
	"github.com/skosovsky/prompty/remoteregistry"
	"github.com/skosovsky/prompty/remoteregistry/git"
)

func setupTempRepo(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp("", "prompty-git-prompts-*")
	if err != nil {
		return "", err
	}
	// Temp dir is left on disk for the lifetime of the process so file:// URLs remain valid.
	manifest := []byte(`id: demo
version: "1"
messages:
  - role: system
    content: "You answer briefly. Topic: {{ .topic }}."
  - role: user
    content: "{{ .question }}"
`)
	if err := os.WriteFile(filepath.Join(dir, "demo.yaml"), manifest, 0600); err != nil {
		return "", fmt.Errorf("WriteFile: %w", err)
	}
	for _, c := range []string{"git init", "git branch -M main", "git add .", "git commit -m init"} {
		cmd := exec.CommandContext(ctx, "sh", "-c", c)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s: %w %s", c, err, out)
		}
	}
	return "file://" + dir, nil
}

func run() error {
	ctx := context.Background()
	repoURL := os.Getenv("GIT_REPO_URL")
	if repoURL == "" {
		url, err := setupTempRepo(ctx)
		if err != nil {
			return fmt.Errorf("setupTempRepo: %w", err)
		}
		repoURL = url
	}
	fetcher, err := git.NewFetcher(repoURL)
	if err != nil {
		return fmt.Errorf("NewFetcher: %w", err)
	}
	defer func() { _ = fetcher.Close() }()

	reg, err := remoteregistry.New(fetcher, remoteregistry.WithParser(yaml.New()))
	if err != nil {
		return fmt.Errorf("remoteregistry.New: %w", err)
	}
	tpl, err := reg.GetTemplate(ctx, "demo")
	if err != nil {
		return fmt.Errorf("GetTemplate: %w", err)
	}
	type Payload struct {
		Topic    string `prompt:"topic"`
		Question string `prompt:"question"`
	}
	exec, err := tpl.FormatStruct(&Payload{Topic: "math", Question: "What is 3+3?"})
	if err != nil {
		return fmt.Errorf("FormatStruct: %w", err)
	}

	if os.Getenv("GEMINI_API_KEY") == "" {
		return errors.New("GEMINI_API_KEY is not set")
	}
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY")})
	if err != nil {
		return fmt.Errorf("genai NewClient: %w", err)
	}
	adp := geminiadapter.New(geminiadapter.WithClient(genaiClient))
	client := adapter.NewClient(adp)
	resp, err := client.Generate(ctx, exec)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	fmt.Println(resp.Text())
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
