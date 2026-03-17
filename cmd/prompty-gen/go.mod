module github.com/skosovsky/prompty/cmd/prompty-gen

go 1.26.0

require (
	github.com/dave/jennifer v1.7.0
	github.com/skosovsky/prompty v0.0.0
	github.com/skosovsky/prompty/parser/yaml v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/skosovsky/prompty => ../..

replace github.com/skosovsky/prompty/parser/yaml => ../../parser/yaml
