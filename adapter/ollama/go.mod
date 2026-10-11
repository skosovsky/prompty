module github.com/skosovsky/prompty/adapter/ollama

go 1.27.2

require (
	github.com/ollama/ollama v0.40.0
	github.com/skosovsky/prompty v0.17.1
	github.com/stretchr/testify v1.12.1
	go.uber.org/goleak v1.3.0
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.6.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/go-playground/validator/v10 v10.30.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/mailru/easyjson v0.9.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// v0.16.1 regressed the published v0.16.0 API and omitted a module.
retract v0.16.1
