package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

// exampleHostFiles is a caller-owned local transport fixture. A production port
// owns authentication/egress and sends each upload once; it must reconcile an
// ambiguous upload error rather than retrying unknown acceptance.
type exampleHostFiles struct {
	binding prompty.BatchFileBinding
	closes  int
}

func (f *exampleHostFiles) Binding() prompty.BatchFileBinding { return f.binding }

func (f *exampleHostFiles) Upload(
	ctx context.Context,
	input prompty.BatchFileUpload,
) (*prompty.BatchFileUploadResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(input.Content, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errors.New("host upload limit exceeded")
	}
	return &prompty.BatchFileUploadResult{
		Resource:   prompty.BatchResourceRef{Kind: "input", ProviderID: "files/input"},
		SourceJSON: prompty.JSONDocument(`{"name":"files/input"}`), HTTPStatus: http.StatusOK,
	}, nil
}

func (f *exampleHostFiles) Download(
	ctx context.Context,
	_ prompty.BatchResourceRef,
) (*prompty.BatchFileDownload, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Ownership transfers to the batch operation, which closes once on all paths.
	return &prompty.BatchFileDownload{
		Body: &exampleHostDownload{
			Reader: strings.NewReader("{\"key\":\"one\",\"error\":{\"code\":3}}\n"),
			files:  f,
		},
		HTTPStatus: http.StatusOK,
	}, nil
}

type exampleHostDownload struct {
	io.Reader

	files *exampleHostFiles
}

func (b *exampleHostDownload) Close() error { b.files.closes++; return nil }

func ExampleNewFileBatches() {
	// Local fixtures only. A real host supplies its own upload/download transport,
	// authentication and cleanup policy; prompty does not install a file store.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
			return
		}
		_, _ = io.WriteString(
			w,
			`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/output"}}}`,
		)
	}))
	defer server.Close()
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"}})
	if err != nil {
		panic(err)
	}
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local file batch fixture; no live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityBatchSubmit:  prompty.CapabilitySupported,
			prompty.CapabilityBatchInspect: prompty.CapabilitySupported,
			prompty.CapabilityBatchResults: prompty.CapabilitySupported},
	}
	files := &exampleHostFiles{
		binding: prompty.BatchFileBinding{Provider: "gemini", Endpoint: server.URL, AccountBinding: "host-account"},
		closes:  0,
	}
	// This port does not declare BatchFileCaptureDescriber. Required original-source
	// capture needs an explicit honest host declaration; binding alone is not coverage.
	manager := NewFileBatches(
		"host-account",
		files,
		BatchFileOptions{},
		WithClient(client),
		WithModel("fixture"),
		WithProfiles(server.URL, profile),
	)
	input, options := inlineBatchFixtureInputs()
	input.Items, options.Bindings = input.Items[:1], options.Bindings[:1]
	submitted, err := manager.Submit(ctx, input)
	if err != nil {
		panic(err)
	}
	// The caller, not a background worker, chooses when to inspect and download.
	inspected, err := manager.Get(ctx, *submitted.Handle)
	if err != nil {
		panic(err)
	}
	results, err := manager.Results(ctx, *inspected.Handle, options)
	if err != nil {
		panic(err)
	}
	fmt.Println(results.Completeness.DownloadComplete, results.Completeness.CorrelationComplete)
	fmt.Println(results.Items[0].ID, results.Items[0].Outcome)
	fmt.Println("download closes:", files.closes)
	// Output:
	// true true
	// one error
	// download closes: 1
}
