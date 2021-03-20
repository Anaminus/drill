// The drill package contains primitives for implementing tree structures that
// can be drilled into.
package drill

import "io"

// Node is implemented by any type that can produce a fragment.
type Node interface {
	// Fragment returns the data represented by the node. Returns an empty
	// string if the node has no data, or an error occurs.
	Fragment() string
}

// ReaderNode extends a Node by producing a fragment as a reader.
type ReaderNode interface {
	Node
	FragmentReader() (r io.ReadCloser, err error)
}

// OrderedBranch extends Node by containing ordered child nodes.
type OrderedBranch interface {
	Node
	// Len returns the number of ordered child nodes. Should not return a value
	// less than 0.
	Len() int
	// OrderedChild returns the child node at index i. i should be expected to
	// have the same boundary rules as implemented by the Index function.
	// Returns nil if i is out of bounds, or Len returns 0.
	OrderedChild(i int) Node
	// OrderedChildren returns a retainable list of ordered child nodes.
	OrderedChildren() []Node
}

// UnorderedBranch extends Node by containing unordered child nodes.
type UnorderedBranch interface {
	Node
	// UnorderedChild returns the child of the node that matches name. Returns
	// nil if the child could not be found.
	UnorderedChild(name string) Node
	// UnorderedChildren returns a retainable map of names to child nodes.
	UnorderedChildren() map[string]Node
}

// Descender extends a Node to descend into the unordered descendants of the
// node.
type Descender interface {
	Node
	Descend(names ...string) Node
}

// Queryer extends Node to descend into the descendants of the node.
type Queryer interface {
	Node
	Query(queries ...interface{}) Node
}

// Descend recursively descends into the unordered child nodes matching each
// given name. Returns nil if a child could not be found at any point.
//
// If a node implements Descender, then the Descend method is called with the
// remaining names.
func Descend(n Node, names ...string) Node {
	for i, name := range names {
		switch v := n.(type) {
		case Descender:
			return v.Descend(names[i:]...)
		case UnorderedBranch:
			n = v.UnorderedChild(name)
		default:
			return nil
		}
	}
	return n
}

// Query recursively descends into the child nodes that match the given queries.
// A query is either a string or an int. If an int, and the current node is an
// OrderedBranch, then the next node is acquired using the OrderedChild method
// of the current node. If a string, and the current node is an UnorderedBranch,
// then the next node is acquired using the UnorderedChild method of the current
// node. Returns nil if a child could not be found at any point.
//
// If a node implements Queryer, then the Query method is called with the
// remaining queries.
func Query(n Node, queries ...interface{}) Node {
	for i, query := range queries {
		switch v := n.(type) {
		case Queryer:
			return v.Query(queries[i:]...)
		}
		if n == nil {
			return nil
		}
		switch q := query.(type) {
		case string:
			u, ok := n.(UnorderedBranch)
			if !ok {
				return nil
			}
			n = u.UnorderedChild(q)
		case int:
			o, ok := n.(OrderedBranch)
			if !ok {
				return nil
			}
			n = o.OrderedChild(q)
		default:
			return nil
		}
	}
	return n
}

func descendants(d *[]Node, n OrderedBranch) {
	for _, child := range n.OrderedChildren() {
		if child == nil {
			continue
		}
		*d = append(*d, child)
		if o, ok := n.(OrderedBranch); ok {
			descendants(d, o)
		}
	}
}

// Descendants returns a list of all the descendants of the node. If a node does
// not implement OrderedBranch, then its children are skipped.
func Descendants(n Node) []Node {
	d := []Node{}
	if n == nil {
		return d
	}
	if o, ok := n.(OrderedBranch); ok {
		descendants(&d, o)
	}
	return d
}

// Index returns i such that, if it is less than 0, it wraps around to len, so
// that -1 returns the index of the last node, and so on. Returns a value less
// than 0 if i is out of bounds, or if len is less than or equal to 0.
//
// Index may be used to implement the i parameter of OrderedBranch.OrderedChild.
func Index(i, len int) int {
	if len <= 0 {
		return -1
	}
	if i < 0 {
		i += len
	}
	if i < 0 || i >= len {
		return -1
	}
	return i
}
