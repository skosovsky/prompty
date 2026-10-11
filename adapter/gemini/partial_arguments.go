package gemini

import (
	"encoding/json"
	"maps"

	"github.com/skosovsky/prompty"
)

type partialArguments struct{ root *argumentNode }

type argumentNode struct {
	members  map[string]*argumentNode
	elements map[int]*argumentNode
	value    any
	leaf     bool
	open     bool
}

// accept is transactional: an invalid later field must not alter prior state.
func (a *partialArguments) accept(fragments []json.RawMessage) error {
	root := cloneArgumentNode(a.root)
	if root == nil {
		root = &argumentNode{
			elements: nil,
			value:    nil,
			leaf:     false,
			open:     false,
			members:  make(map[string]*argumentNode),
		}
	}
	for _, fragment := range fragments {
		argument, err := decodePartialArgument(fragment)
		if err != nil {
			return err
		}
		steps, err := parseArgumentPath(argument.path)
		if err != nil {
			return err
		}
		leaf, err := argumentTarget(root, steps)
		if err != nil {
			return err
		}
		if err = leaf.accept(argument); err != nil {
			return err
		}
	}
	a.root = root
	return nil
}

//nolint:nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (n *argumentNode) accept(argument partialArgument) error {
	if n.members != nil || n.elements != nil {
		return prompty.ErrStreamProtocol
	}
	if n.leaf {
		if !n.open {
			return prompty.ErrStreamProtocol
		}
		previous, ok := n.value.(string)
		if !ok {
			return prompty.ErrStreamProtocol
		}
		if argument.present {
			next, stringValue := argument.value.(string)
			if !stringValue {
				return prompty.ErrStreamProtocol
			}
			n.value = previous + next
		}
	} else {
		if !argument.present {
			return prompty.ErrStreamProtocol
		}
		n.value, n.leaf = argument.value, true
	}
	n.open = argument.continues
	return nil
}

//nolint:gocognit,nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func argumentTarget(root *argumentNode, steps []argumentPathStep) (*argumentNode, error) {
	node := root
	for _, step := range steps {
		if node.leaf {
			return nil, prompty.ErrStreamProtocol
		}
		if step.array {
			if node.members != nil {
				return nil, prompty.ErrStreamProtocol
			}
			if node.elements == nil {
				node.elements = make(map[int]*argumentNode)
			}
			child := node.elements[step.index]
			if child == nil {
				child = &argumentNode{members: nil, elements: nil, value: nil, leaf: false, open: false}
				node.elements[step.index] = child
			}
			node = child
		} else {
			if node.elements != nil {
				return nil, prompty.ErrStreamProtocol
			}
			if node.members == nil {
				node.members = make(map[string]*argumentNode)
			}
			child := node.members[step.name]
			if child == nil {
				child = &argumentNode{members: nil, elements: nil, value: nil, leaf: false, open: false}
				node.members[step.name] = child
			}
			node = child
		}
	}
	return node, nil
}

func (a *partialArguments) complete() (string, error) {
	if a.root == nil {
		return "{}", nil
	}
	value, err := materializeArgumentNode(a.root)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", prompty.ErrIncompleteToolCallArgs
	}
	return string(data), nil
}

func materializeArgumentNode(node *argumentNode) (any, error) {
	if node.open {
		return nil, prompty.ErrIncompleteToolCallArgs
	}
	if node.leaf {
		return node.value, nil
	}
	if node.members != nil {
		value := make(map[string]any, len(node.members))
		for key, child := range node.members {
			materialized, err := materializeArgumentNode(child)
			if err != nil {
				return nil, err
			}
			value[key] = materialized
		}
		return value, nil
	}
	if node.elements != nil {
		// Sparse construction allocates by actual fragments, not untrusted index.
		// A contiguous array is required; absent entries are not fabricated nulls.
		value := make([]any, len(node.elements))
		for index, child := range node.elements {
			if index >= len(value) {
				return nil, prompty.ErrIncompleteToolCallArgs
			}
			materialized, err := materializeArgumentNode(child)
			if err != nil {
				return nil, err
			}
			value[index] = materialized
		}
		return value, nil
	}
	return nil, prompty.ErrIncompleteToolCallArgs
}

func cloneArgumentNode(node *argumentNode) *argumentNode {
	if node == nil {
		return nil
	}
	clone := *node
	clone.members = maps.Clone(node.members)
	clone.elements = maps.Clone(node.elements)
	for key, child := range clone.members {
		clone.members[key] = cloneArgumentNode(child)
	}
	for index, child := range clone.elements {
		clone.elements[index] = cloneArgumentNode(child)
	}
	return &clone
}
