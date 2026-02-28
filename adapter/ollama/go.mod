module github.com/skosovsky/prompty/adapter/ollama

go 1.26.0

require (
	github.com/ollama/ollama v0.17.4
	github.com/skosovsky/prompty v0.0.0
	github.com/stretchr/testify v1.11.1
	go.uber.org/goleak v1.3.0
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/skosovsky/prompty => ../..
