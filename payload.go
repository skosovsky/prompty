package prompty

import (
	"maps"
	"reflect"
	"slices"
	"text/template"
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

// templatePath contains possible origins in the invocation context. A nil path
// is a computed/local value; selecting its members adds no root dependency.
type templatePath [][]string

// shadowed retains the outer binding as it was immediately before a local :=.
// Assignments before that declaration escape; assignments to the local do not.
type templateScope struct {
	values   map[string]templatePath
	shadowed map[string]templatePath
}

func (s *templateScope) fork(inheritDeclarations bool) *templateScope {
	shadowed := make(map[string]templatePath)
	if inheritDeclarations {
		shadowed = maps.Clone(s.shadowed)
	}
	return &templateScope{values: maps.Clone(s.values), shadowed: shadowed}
}

func (s *templateScope) bind(name string, value templatePath, assign bool) {
	if old, exists := s.values[name]; exists && !assign {
		if _, local := s.shadowed[name]; !local {
			s.shadowed[name] = old
		}
	}
	s.values[name] = value
}

func (s *templateScope) outerValue(name string) templatePath {
	if value, local := s.shadowed[name]; local {
		return value
	}
	return s.values[name]
}

type templateVarExtractor struct {
	seen   map[string]bool
	out    []string
	lookup func(string) *parse.Tree
	active map[string]bool
}

func extractVarsFromTree(tree *parse.Tree) []string { return extractTemplateVars(tree, nil) }

func extractTemplateVars(tree *parse.Tree, lookup func(string) *parse.Tree) []string {
	e := templateVarExtractor{seen: make(map[string]bool), out: nil, lookup: lookup, active: make(map[string]bool)}
	root := templatePath{nil}
	if tree != nil {
		e.walk(
			tree.Root,
			root,
			&templateScope{values: map[string]templatePath{"$": root}, shadowed: make(map[string]templatePath)},
		)
	}
	return e.out
}

func (e *templateVarExtractor) selectPath(origins templatePath, keys []string) templatePath {
	var out templatePath
	for _, origin := range origins {
		path := append(append([]string(nil), origin...), keys...)
		out = append(out, path)
		if len(path) == 0 {
			continue
		}
		name := path[0]
		switch name {
		case "Tools", "LateVars":
			continue
		case "Input":
			if len(path) < 2 {
				continue
			}
			name = path[1]
		}
		if !e.seen[name] {
			e.seen[name] = true
			e.out = append(e.out, name)
		}
	}
	return out
}

func (e *templateVarExtractor) expression(
	node parse.Node,
	dot templatePath,
	vars *templateScope,
) templatePath {
	if isNilNode(node) {
		return nil
	}
	switch n := node.(type) {
	case *parse.DotNode:
		return dot
	case *parse.FieldNode:
		return e.selectPath(dot, n.Ident)
	case *parse.VariableNode:
		return e.selectPath(vars.values[n.Ident[0]], n.Ident[1:])
	case *parse.ChainNode:
		return e.selectPath(e.expression(n.Node, dot, vars), n.Field)
	case *parse.PipeNode:
		var result templatePath
		for _, cmd := range n.Cmds {
			result = e.expression(cmd, dot, vars)
		}
		for _, decl := range n.Decl {
			vars.bind(decl.Ident[0], result, n.IsAssign)
		}
		return result
	case *parse.CommandNode:
		return e.command(n, dot, vars)
	}
	return nil
}

func (e *templateVarExtractor) command(
	cmd *parse.CommandNode,
	dot templatePath,
	vars *templateScope,
) templatePath {
	values := make([]templatePath, len(cmd.Args))
	for index, arg := range cmd.Args {
		values[index] = e.expression(arg, dot, vars)
	}
	if len(values) == 1 {
		return values[0]
	}
	function, ok := cmd.Args[0].(*parse.IdentifierNode)
	if !ok {
		return nil
	}
	switch function.Ident {
	case "index":
		value := values[1]
		for _, arg := range cmd.Args[2:] {
			switch key := arg.(type) {
			case *parse.StringNode:
				value = e.selectPath(value, []string{key.Text})
			case *parse.NumberNode:
				value = rangeElementPath(value)
			default:
				return nil
			}
		}
		return value
	case "and", "or":
		var out templatePath
		for _, value := range values[1:] {
			out = append(out, value...)
		}
		return out
	}
	return nil
}

func rangeElementPath(origins templatePath) templatePath {
	var out templatePath
	for _, path := range origins {
		if len(path) == 0 || (len(path) == 1 && path[0] == "Input") {
			continue
		}
		out = append(out, path)
	}
	return out
}

// Merge only outer variables; declarations inside a control action do not escape.
func mergeTemplateScopes(outer, left, right *templateScope) {
	for name := range outer.values {
		var merged templatePath
		for _, path := range append(append(templatePath(nil), left.outerValue(name)...), right.outerValue(name)...) {
			if !slices.ContainsFunc(merged, func(other []string) bool { return slices.Equal(path, other) }) {
				merged = append(merged, path)
			}
		}
		outer.values[name] = merged
	}
}

func constantTemplateBool(pipe *parse.PipeNode) (bool, bool) {
	if len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return false, false
	}
	value, ok := pipe.Cmds[0].Args[0].(*parse.BoolNode)
	if !ok {
		return false, false
	}
	return value.True, true
}

func (e *templateVarExtractor) branch(
	pipe *parse.PipeNode,
	list, otherwise *parse.ListNode,
	dot templatePath,
	vars *templateScope,
	with bool,
) {
	scope := vars.fork(false)
	value := e.expression(pipe, dot, scope)
	body := scope.fork(true)
	other := scope.fork(true)
	bodyDot := dot
	if with {
		bodyDot = value
	}
	if truth, constant := constantTemplateBool(pipe); constant {
		if truth {
			e.walk(list, bodyDot, body)
			mergeTemplateScopes(vars, body, body)
		} else {
			e.walk(otherwise, dot, other)
			mergeTemplateScopes(vars, other, other)
		}
		return
	}
	e.walk(list, bodyDot, body)
	e.walk(otherwise, dot, other)
	mergeTemplateScopes(vars, body, other)
}

func (e *templateVarExtractor) walk(node parse.Node, dot templatePath, vars *templateScope) {
	if isNilNode(node) {
		return
	}
	switch n := node.(type) {
	case *parse.ListNode:
		for _, child := range n.Nodes {
			e.walk(child, dot, vars)
		}
	case *parse.ActionNode:
		e.expression(n.Pipe, dot, vars)
	case *parse.IfNode:
		e.branch(n.Pipe, n.List, n.ElseList, dot, vars, false)
	case *parse.WithNode:
		e.branch(n.Pipe, n.List, n.ElseList, dot, vars, true)
	case *parse.RangeNode:
		scope := vars.fork(false)
		value := rangeElementPath(e.expression(n.Pipe, dot, scope))
		body := scope.fork(true)
		for index, decl := range n.Pipe.Decl {
			body.values[decl.Ident[0]] = value
			if len(n.Pipe.Decl) == 2 && index == 0 {
				body.values[decl.Ident[0]] = nil
			}
		}
		other := scope.fork(true)
		e.walk(n.List, value, body)
		e.walk(n.ElseList, dot, other)
		mergeTemplateScopes(vars, body, other)
	case *parse.TemplateNode:
		value := e.expression(n.Pipe, dot, vars)
		if e.lookup == nil || e.active[n.Name] {
			return
		}
		if tree := e.lookup(n.Name); tree != nil {
			e.active[n.Name] = true
			e.walk(
				tree.Root,
				value,
				&templateScope{values: map[string]templatePath{"$": value}, shadowed: make(map[string]templatePath)},
			)
			delete(e.active, n.Name)
		}
	}
}

func extractVarsFromTemplate(tmpl *template.Template) []string {
	return extractTemplateVars(tmpl.Tree, func(name string) *parse.Tree {
		if partial := tmpl.Lookup(name); partial != nil {
			return partial.Tree
		}
		return nil
	})
}

func extractRequiredVarsFromParsed(parsed []parsedMessage) []string {
	seen := make(map[string]bool)
	var out []string
	for _, pm := range parsed {
		if pm.optional {
			continue
		}
		for _, name := range requiredVarsFromMessage(pm) {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

func requiredVarsFromMessage(message parsedMessage) []string {
	var names []string
	for _, part := range message.parts {
		for _, tmpl := range part.templates() {
			if tmpl != nil {
				names = append(names, extractVarsFromTemplate(tmpl)...)
			}
		}
	}
	return names
}
