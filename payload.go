package prompty

import (
	"reflect"
	"sync"
	"text/template/parse"
)

// isNilNode returns true if node is nil or an interface holding a nil pointer (e.g. *parse.ListNode).
func isNilNode(node parse.Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	return v.Kind() == reflect.Pointer && v.IsNil()
}

type payloadField struct {
	index     int
	tag       string
	isHistory bool
}

type payloadSchema struct {
	fields []payloadField
}

var payloadCache sync.Map // reflect.Type -> *payloadSchema

// chatMessageSliceType is cached reflect type for payload parser ([]ChatMessage history field).
var chatMessageSliceType = reflect.TypeFor[[]ChatMessage]()

func getPayloadFields(payload any) (vars map[string]any, history []ChatMessage, err error) {
	if payload == nil {
		return nil, nil, ErrInvalidPayload
	}
	typ := reflect.TypeOf(payload)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, nil, ErrInvalidPayload
	}
	v := reflect.ValueOf(payload)
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() {
		return nil, nil, ErrInvalidPayload
	}
	var schema *payloadSchema
	if cached, ok := payloadCache.Load(typ); ok {
		schema = cached.(*payloadSchema)
	} else {
		schema = &payloadSchema{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := f.Tag.Get("prompt")
			if tag == "" || tag == "-" {
				continue
			}
			if f.Type == chatMessageSliceType {
				schema.fields = append(schema.fields, payloadField{index: i, tag: tag, isHistory: true})
			} else {
				schema.fields = append(schema.fields, payloadField{index: i, tag: tag, isHistory: false})
			}
		}
		if len(schema.fields) == 0 {
			return nil, nil, ErrInvalidPayload
		}
		payloadCache.Store(typ, schema)
	}
	vars = make(map[string]any)
	for _, fi := range schema.fields {
		val := v.Field(fi.index)
		if fi.isHistory {
			if val.CanInterface() {
				if cm, ok := val.Interface().([]ChatMessage); ok {
					history = cm
				}
			}
			continue
		}
		if val.CanInterface() {
			vars[fi.tag] = val.Interface()
		}
	}
	if _, ok := vars["Tools"]; ok {
		return nil, nil, ErrReservedVariable
	}
	return vars, history, nil
}

func walkParseNodes(node parse.Node, visit func(parse.Node)) {
	if isNilNode(node) {
		return
	}
	visit(node)
	switch n := node.(type) {
	case *parse.ListNode:
		for _, c := range n.Nodes {
			walkParseNodes(c, visit)
		}
	case *parse.ActionNode:
		if n.Pipe != nil {
			walkParseNodes(n.Pipe, visit)
		}
	case *parse.PipeNode:
		for _, c := range n.Cmds {
			walkParseNodes(c, visit)
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			walkParseNodes(a, visit)
		}
	case *parse.IfNode:
		walkParseNodes(n.Pipe, visit)
		walkParseNodes(n.List, visit)
		walkParseNodes(n.ElseList, visit)
	case *parse.RangeNode:
		walkParseNodes(n.Pipe, visit)
		walkParseNodes(n.List, visit)
		walkParseNodes(n.ElseList, visit)
	case *parse.WithNode:
		walkParseNodes(n.Pipe, visit)
		walkParseNodes(n.List, visit)
		walkParseNodes(n.ElseList, visit)
	}
}

// extractVarsFromTree collects top-level variable names from template parse tree (e.g. .user_name -> "user_name").
func extractVarsFromTree(tree *parse.Tree) []string {
	if tree == nil || tree.Root == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	walkParseNodes(tree.Root, func(n parse.Node) {
		if fn, ok := n.(*parse.FieldNode); ok && len(fn.Ident) > 0 {
			name := fn.Ident[0]
			if name != "Tools" && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	})
	return out
}

func extractRequiredVarsFromParsed(parsed []parsedMessage) []string {
	seen := make(map[string]bool)
	var out []string
	for _, pm := range parsed {
		if pm.optional || pm.tpl == nil {
			continue
		}
		for _, name := range extractVarsFromTree(pm.tpl.Tree) {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}
