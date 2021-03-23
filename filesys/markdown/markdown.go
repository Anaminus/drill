// The markdown package implements filesys.Handler for the Markdown format.
//
// Markdown is parsed and rendered with the goldmark package. As such, it
// contains implementations of various goldmark interfaces, which may be used
// for custom parsers and renderers.
package markdown

import (
	"errors"
	"io"
	"io/fs"
	"strings"

	"github.com/anaminus/drill"
	"github.com/anaminus/drill/filesys"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// NewHandler returns a filesys.HandlerFunc that handles the Markdown format.
//
// The content of a Markdown file is divided into a tree of sections, delimited
// by headings. A section can be drilled into, with the name corresponding to
// the content of the section's heading.
//
// A section contains all the content that follows a heading, up to the next
// heading of the same level. Content that follows the section's heading, and
// precedes the end of the section or the first sub-heading, is contained within
// an "orphaned" section. The name of this section is an empty string, and will
// be the first child section.
func NewHandler(options ...goldmark.Option) filesys.HandlerFunc {
	return func(fsys fs.FS, name string) drill.Node {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil
		}
		options = append(options,
			goldmark.WithParserOptions(
				parser.WithASTTransformers(
					util.Prioritized(NewSectionTransformer(), 2000),
				),
			),
			goldmark.WithRendererOptions(
				renderer.WithNodeRenderers(
					util.Prioritized(NewSectionRenderer(), 2000),
				),
			),
		)
		md := goldmark.New(options...)
		parser := md.Parser()
		root := parser.Parse(text.NewReader(b))
		node := NewNode(root, b, md.Renderer())
		return node
	}
}

// Node implements drill.Node.
type Node struct {
	root     *Node
	section  ast.Node
	source   []byte
	renderer renderer.Renderer
}

// NewNode returns a Node that wraps the given ast.Node, source, and renderer.
// root is assumed to be an ast.Document or a Section.
//
// The node created by NewNode is treated as the root. Nodes that derive from
// the root will point back to the root.
func NewNode(root ast.Node, source []byte, renderer renderer.Renderer) *Node {
	node := &Node{
		section:  root,
		source:   source,
		renderer: renderer,
	}
	node.root = node
	return node
}

// derive returns a Node that wraps section, using the same source and renderer.
func (n *Node) derive(section ast.Node) *Node {
	d := *n
	d.section = section
	return &d
}

// Root returns the original root Node from which the current node derives, or
// the node itself, if it is the root.
func (n *Node) Root() *Node {
	if n.root == nil {
		return n
	}
	return n.root
}

// Node returns the wrapped ast.Node.
func (n *Node) Node() ast.Node {
	return n.section
}

// WalkChildSections traverses each child node that is a Section. Stops if walk
// returns true.
func (n *Node) WalkChildSections(walk func(child *Section) bool) {
	ast.Walk(n.section, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if child == n.section {
			return ast.WalkContinue, nil
		}
		if section, ok := child.(*Section); ok {
			if walk(section) {
				return ast.WalkStop, nil
			}
		}
		return ast.WalkSkipChildren, nil
	})
}

// Fragment renders the wrapped node and returns the result as a string. Returns
// an empty string if an error occurs, or the node has no renderer.
func (n *Node) Fragment() string {
	if n.renderer == nil {
		return ""
	}
	var buf strings.Builder
	if err := n.renderer.Render(&buf, n.source, n.section); err != nil {
		return ""
	}
	return buf.String()
}

// render renders n to w.
func render(n *Node, w *io.PipeWriter) {
	if err := n.renderer.Render(w, n.source, n.section); err != nil {
		w.CloseWithError(err)
	}
	w.Close()
}

// FragmentReader returns a ReadCloser that renders the wrapped node.
func (n *Node) FragmentReader() (r io.ReadCloser, err error) {
	if n.renderer == nil {
		return nil, errors.New("no renderer")
	}
	r, w := io.Pipe()
	go render(n, w)
	return r, nil
}

// Len returns the number of child Sections.
func (n *Node) Len() int {
	var count int
	n.WalkChildSections(func(section *Section) bool {
		count++
		return false
	})
	return count
}

// OrderedChild returns a Node that wraps the ordered child Section at index i.
// Returns nil if the index is out of bounds.
func (n *Node) OrderedChild(i int) drill.Node {
	if i = drill.Index(i, n.Len()); i < 0 {
		return nil
	}
	var count int
	var section ast.Node
	n.WalkChildSections(func(child *Section) bool {
		if count == i {
			section = child
			return true
		}
		count++
		return false
	})
	if section == nil {
		return nil
	}
	return n.derive(section)
}

// OrderedChildren returns a list of Nodes that wrap each ordered child Section.
func (n *Node) OrderedChildren() []drill.Node {
	var sections []drill.Node
	n.WalkChildSections(func(child *Section) bool {
		sections = append(sections, n.derive(child))
		return false
	})
	return sections
}

// UnorderedChild returns a Node that wraps the unordered child Section whose
// Name is equal to name.
func (n *Node) UnorderedChild(name string) drill.Node {
	var section ast.Node
	n.WalkChildSections(func(child *Section) bool {
		if child.Name == name {
			section = child
			return true
		}
		return false
	})
	if section == nil {
		return nil
	}
	return n.derive(section)
}

// UnorderedChildren returns a map of names to Nodes that wrap each unordered
// child Section.
func (n *Node) UnorderedChildren() map[string]drill.Node {
	sections := map[string]drill.Node{}
	n.WalkChildSections(func(child *Section) bool {
		sections[child.Name] = n.derive(child)
		return false
	})
	return sections
}

// Descend recursively descends into the unordered child sections matching each
// given name. Returns nil if a child could not be found at any point.
func (n *Node) Descend(names ...string) drill.Node {
	for _, name := range names {
		var ok bool
		n.WalkChildSections(func(section *Section) bool {
			if section.Name != name {
				return false
			}
			ok = true
			n = n.derive(section)
			return true
		})
		if !ok {
			return nil
		}
	}
	return n
}

// Query recursively descends into the child nodes that match the given queries.
// A query is either a string or an int. If an int, then the next node is
// acquired using the OrderedChild method of the current node. If a string, then
// the next node is acquired using the UnorderedChild method of the current
// node. Returns nil if a child could not be found at any point.
func (n *Node) Query(queries ...interface{}) drill.Node {
	for _, query := range queries {
		switch q := query.(type) {
		case string:
			var section ast.Node
			n.WalkChildSections(func(child *Section) bool {
				if child.Name == q {
					section = child
					return true
				}
				return false
			})
			if section == nil {
				return nil
			}
			n = n.derive(section)
		case int:
			if q = drill.Index(q, n.Len()); q < 0 {
				return nil
			}
			var count int
			var section ast.Node
			n.WalkChildSections(func(child *Section) bool {
				if count == q {
					section = child
					return true
				}
				count++
				return false
			})
			if section == nil {
				return nil
			}
			n = n.derive(section)
		default:
			return nil
		}
	}
	return n
}
