package prompty

import (
	"maps"
	"sync"
)

//nolint:gochecknoglobals // pooled map projection for text/template (.Input field access)
var boundInputMapPool = sync.Pool{
	New: func() any { return make(map[string]any) },
}

func mergeBoundVarsWithPartials(tpl *ChatPromptTemplate, vars map[string]any) map[string]any {
	m, ok := boundInputMapPool.Get().(map[string]any)
	if !ok || m == nil {
		capHint := len(vars)
		if tpl != nil && tpl.PartialVariables != nil {
			capHint += len(tpl.PartialVariables)
		}
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
	if m == nil {
		return
	}
	clear(m)
	boundInputMapPool.Put(m)
}
