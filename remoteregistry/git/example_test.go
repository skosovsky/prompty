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
    content: "Hello {{ .Input.user_name }}"
`,
	} {
		full := filepath.Join(dir, path)
		if mkErr := os.MkdirAll(filepath.Dir(full), 0755); mkErr != nil {
			panic(mkErr)
		}
		if writeErr := os.WriteFile(full, []byte(content), 0644); writeErr != nil {
			panic(writeErr)
		}
	}
	for _, c := range []string{"git init", "git branch -M main", "git add .", "git commit -m init"} {
		cmd := exec.Command("sh", "-c", c)
		cmd.Dir = dir
		cmd.Env = gitTestEnv()
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			panic(fmt.Sprintf("%s: %v %s", c, runErr, out))
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
