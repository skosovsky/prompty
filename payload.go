package prompty

import (
	"reflect"
	"text/template/parse"
)

// isNilNode returns true if node is nil or an interface holding a nil *[parse.ListNode].
func isNilNode(node parse.Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	for v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	return v.Kind() == reflect.Pointer && v.IsNil()
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

// extractVarsFromTree collects v2 input variable names from template parse tree.
// Examples:
//   - .Input.user_name -> "user_name"
//   - .LateVars.allowed_tools -> ignored (late-bound, not required input)
//   - .Tools -> ignored (reserved helper context)
func extractVarsFromTree(tree *parse.Tree) []string {
	if tree == nil || tree.Root == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	walkParseNodes(tree.Root, func(n parse.Node) {
		if fn, ok := n.(*parse.FieldNode); ok && len(fn.Ident) > 0 {
			name := fn.Ident[0]
			switch name {
			case "Tools", "LateVars":
				return
			case "Input":
				if len(fn.Ident) < 2 {
					return
				}
				name = fn.Ident[1]
			}
			if !seen[name] {
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
		if pm.optional {
			continue
		}
		for _, part := range pm.parts {
			for _, tmpl := range part.templates() {
				if tmpl == nil {
					continue
				}
				for _, name := range extractVarsFromTree(tmpl.Tree) {
					if !seen[name] {
						seen[name] = true
						out = append(out, name)
					}
				}
			}
		}
	}
	return out
}
