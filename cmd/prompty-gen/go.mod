module github.com/skosovsky/prompty/cmd/prompty-gen

go 1.26.0

require (
	github.com/dave/jennifer v1.7.0
	github.com/skosovsky/prompty v0.5.1
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/kr/text v0.2.0 // indirect

// Replace with local prompty so parser/yaml and manifest resolve from the same module.
replace github.com/skosovsky/prompty => ../..
