package prompty

import (
	"maps"
	"sync"
)

const maxPooledInputEntries = 1024

//nolint:gochecknoglobals // pooled map projection for text/template (.Input field access)
var boundInputMapPool = sync.Pool{
	New: func() any { return make(map[string]any) },
}

func mergeBoundVarsWithPartials(tpl *ChatPromptTemplate, vars map[string]any) map[string]any {
	capHint := len(vars)
	if tpl != nil {
		capHint += len(tpl.PartialVariables)
	}
	var m map[string]any
	if capHint <= maxPooledInputEntries {
		m, _ = boundInputMapPool.Get().(map[string]any)
	}
	if m == nil {
		m = make(map[string]any, capHint)
	}
	clear(m)
	maps.Copy(m, vars)
	if tpl != nil && tpl.PartialVariables != nil {
		for k, v := range tpl.PartialVariables {
			if _, exists := m[k]; !exists {
				m[k] = v
			}
		}
	}
	return m
}

func releaseBoundInputMap(m map[string]any) {
	if m == nil || len(m) > maxPooledInputEntries {
		return
	}
	clear(m)
	boundInputMapPool.Put(m)
}
