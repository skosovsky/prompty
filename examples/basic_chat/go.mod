module github.com/skosovsky/prompty/examples/basic_chat

go 1.26.0

require (
	github.com/openai/openai-go/v3 v3.24.0
	github.com/skosovsky/prompty v0.0.0
	github.com/skosovsky/prompty/adapter/openai v0.0.0
)

require (
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/skosovsky/prompty => ../..
	github.com/skosovsky/prompty/adapter/openai => ../../adapter/openai
)
