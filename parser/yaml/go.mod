module github.com/skosovsky/prompty/parser/yaml

go 1.26.0

require (
	github.com/skosovsky/prompty v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/kr/text v0.2.0 // indirect

replace github.com/skosovsky/prompty => ../..
