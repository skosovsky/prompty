package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ExampleNewFetcher() {
	dir, err := os.MkdirTemp("", "prompty-git-example-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for path, content := range map[string]string{
		"support_agent.yaml": `
id: support_agent
version: "1"
messages:
  - role: system
    content: "Hello {{ .user_name }}"
`,
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			panic(err)
		}
	}
	for _, c := range []string{"git init", "git branch -M main", "git add .", "git commit -m init"} {
		cmd := exec.Command("sh", "-c", c)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		if out, err := cmd.CombinedOutput(); err != nil {
			panic(fmt.Sprintf("%s: %v %s", c, err, out))
		}
	}
	g, err := NewFetcher("file://" + dir)
	if err != nil {
		panic(err)
	}
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	data, err := g.Fetch(ctx, "support_agent")
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.Contains(string(data), "support_agent"))
	fmt.Println(strings.Contains(string(data), "Hello"))
	// Output:
	// true
	// true
}
