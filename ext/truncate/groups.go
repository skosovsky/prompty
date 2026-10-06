package truncate

import "github.com/skosovsky/prompty"

type messageGraph struct {
	parent    []int
	protected []bool
}

func newGraph(size int) *messageGraph {
	graph := &messageGraph{parent: make([]int, size), protected: make([]bool, size)}
	for i := range size {
		graph.parent[i] = i
	}
	return graph
}
func (g *messageGraph) root(index int) int {
	for g.parent[index] != index {
		g.parent[index] = g.parent[g.parent[index]]
		index = g.parent[index]
	}
	return index
}
func (g *messageGraph) join(left, right int) { g.parent[g.root(right)] = g.root(left) }

func removalGroups(messages []prompty.ChatMessage, policy Protection) ([]Group, []int, error) {
	graph := newGraph(len(messages))
	boundary := 0
	for i, message := range messages {
		if message.Role == prompty.RoleUser {
			boundary = i
		}
	}
	if policy.CurrentTurnStart != nil {
		boundary = *policy.CurrentTurnStart
	}
	if boundary < 0 || boundary > len(messages) {
		return nil, nil, ErrInvalidInput
	}
	for i, message := range messages {
		graph.protected[i] = i >= boundary || message.Role == prompty.RoleSystem ||
			message.Role == prompty.RoleDeveloper
	}
	if err := applyProtection(graph, policy, len(messages)); err != nil {
		return nil, nil, err
	}
	linkTurns(graph, messages, boundary)
	linkExchanges(graph, messages)
	linkOpaqueState(graph, messages)
	return graph.groups()
}

func linkTurns(graph *messageGraph, messages []prompty.ChatMessage, boundary int) {
	start := -1
	for i, message := range messages {
		if i == boundary {
			start = -1
		}
		if message.Role == prompty.RoleSystem || message.Role == prompty.RoleDeveloper {
			continue
		}
		if start < 0 || message.Role == prompty.RoleUser {
			start = i
		}
		graph.join(start, i)
	}
}

// Opaque dependencies are not decoded by this generic extension. All state-
// bearing messages form one conservative group, preserving/removing them whole.
func linkOpaqueState(graph *messageGraph, messages []prompty.ChatMessage) {
	first := -1
	for i, message := range messages {
		if len(message.ProviderState) == 0 {
			continue
		}
		if first < 0 {
			first = i
		}
		graph.join(first, i)
	}
}

type exchange struct {
	indices   []int
	calls     int
	results   int
	name      string
	ambiguous bool
}

func linkExchanges(graph *messageGraph, messages []prompty.ChatMessage) {
	exchanges := make(map[string]*exchange)
	for i, message := range messages {
		for _, part := range message.Content {
			registerPartExchange(graph, exchanges, i, part)
		}
	}
	for _, item := range exchanges {
		for _, index := range item.indices {
			graph.join(item.indices[0], index)
			if item.calls != 1 || item.results != 1 || item.ambiguous {
				graph.protected[index] = true
			}
		}
	}
}
func registerExchange(graph *messageGraph, exchanges map[string]*exchange, index int, id, name string, call bool) {
	if id == "" {
		graph.protected[index] = true
		return
	}
	item := exchanges[id]
	if item == nil {
		item = &exchange{name: name, indices: nil, calls: 0, results: 0, ambiguous: false}
		exchanges[id] = item
	}
	item.indices = append(item.indices, index)
	if name == "" || item.name != name {
		item.ambiguous = true
	}
	if call {
		item.calls++
	} else {
		item.results++
	}
}
func (g *messageGraph) groups() ([]Group, []int, error) {
	protectedRoots := make(map[int]bool)
	for i, protected := range g.protected {
		if protected {
			protectedRoots[g.root(i)] = true
		}
	}
	positions := make(map[int]int)
	var groups []Group
	var protected []int
	for i := range g.parent {
		root := g.root(i)
		if protectedRoots[root] {
			protected = append(protected, i)
			continue
		}
		position, exists := positions[root]
		if !exists {
			position = len(groups)
			positions[root] = position
			groups = append(groups, Group{ID: position, Messages: nil})
		}
		groups[position].Messages = append(groups[position].Messages, i)
	}
	return groups, protected, nil
}

func applyProtection(graph *messageGraph, policy Protection, size int) error {
	for _, index := range policy.Protected {
		if index < 0 || index >= size {
			return ErrInvalidInput
		}
		graph.protected[index] = true
	}
	for _, dependency := range policy.Dependencies {
		for _, index := range dependency {
			if index < 0 || index >= size {
				return ErrInvalidInput
			}
		}
		for i := 1; i < len(dependency); i++ {
			graph.join(dependency[0], dependency[i])
		}
	}
	return nil
}

func registerPartExchange(graph *messageGraph, exchanges map[string]*exchange, index int, part prompty.ContentPart) {
	switch value := part.(type) {
	case prompty.ToolCallPart:
		registerExchange(graph, exchanges, index, value.ID, value.Name, true)
	case *prompty.ToolCallPart:
		if value != nil {
			registerExchange(graph, exchanges, index, value.ID, value.Name, true)
		}
	case prompty.ToolResultPart:
		registerExchange(graph, exchanges, index, value.ToolCallID, value.Name, false)
	case *prompty.ToolResultPart:
		if value != nil {
			registerExchange(graph, exchanges, index, value.ToolCallID, value.Name, false)
		}
	}
}
