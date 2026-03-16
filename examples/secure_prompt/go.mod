module github.com/skosovsky/prompty/examples/secure_prompt

go 1.26.0

require (
	github.com/skosovsky/prompty v0.0.0
	github.com/skosovsky/prompty/parser/yaml v0.0.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace (
	github.com/skosovsky/prompty => ../..
	github.com/skosovsky/prompty/parser/yaml => ../../parser/yaml
)
