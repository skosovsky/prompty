// Git prompts example: load a prompt from a Git repo (remoteregistry/git) and call Gemini.
// Run from this directory: go run .
// Requires GEMINI_API_KEY. Uses a temporary local git repo for demo; set GIT_REPO_URL to use your own repo.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/skosovsky/prompty/adapter"
	geminiadapter "github.com/skosovsky/prompty/adapter/gemini"
	"github.com/skosovsky/prompty/remoteregistry"
	"github.com/skosovsky/prompty/remoteregistry/git"
	"google.golang.org/genai"
)

func main() {
	repoURL := os.Getenv("GIT_REPO_URL")
	if repoURL == "" {
		dir, err := os.MkdirTemp("", "prompty-git-prompts-*")
		if err != nil {
			log.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(dir)
		manifest := []byte(`id: demo
version: "1"
messages:
  - role: system
    content: "You answer briefly. Topic: {{ .topic }}."
  - role: user
    content: "{{ .question }}"
`)
		if err := os.WriteFile(filepath.Join(dir, "demo.yaml"), manifest, 0644); err != nil {
			log.Fatalf("WriteFile: %v", err)
		}
		for _, c := range []string{"git init", "git branch -M main", "git add .", "git commit -m init"} {
			cmd := exec.Command("sh", "-c", c)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
			if out, err := cmd.CombinedOutput(); err != nil {
				log.Fatalf("%s: %v %s", c, err, out)
			}
		}
		repoURL = "file://" + dir
	}
	fetcher, err := git.NewFetcher(repoURL)
	if err != nil {
		log.Fatalf("NewFetcher: %v", err)
	}
	defer func() { _ = fetcher.Close() }()

	reg := remoteregistry.New(fetcher)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "demo")
	if err != nil {
		log.Fatalf("GetTemplate: %v", err)
	}
	type Payload struct {
		Topic    string `prompt:"topic"`
		Question string `prompt:"question"`
	}
	exec, err := tpl.FormatStruct(ctx, &Payload{Topic: "math", Question: "What is 3+3?"})
	if err != nil {
		log.Fatalf("FormatStruct: %v", err)
	}

	adp := geminiadapter.New()
	req, err := adp.TranslateTyped(ctx, exec)
	if err != nil {
		log.Fatalf("Translate: %v", err)
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Fatal("GEMINI_API_KEY is not set")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: os.Getenv("GEMINI_API_KEY")})
	if err != nil {
		log.Fatalf("genai NewClient: %v", err)
	}
	model := "gemini-2.0-flash"
	resp, err := client.Models.GenerateContent(ctx, model, req.Contents, req.Config)
	if err != nil {
		log.Fatalf("GenerateContent: %v", err)
	}
	parts, err := adp.ParseResponse(ctx, resp)
	if err != nil {
		log.Fatalf("ParseResponse: %v", err)
	}
	fmt.Println(adapter.TextFromParts(parts))
}
