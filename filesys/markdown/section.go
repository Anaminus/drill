package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Section is an ast.Node that groups nodes into sections based on Headings.
//
// A Section without a Heading is an "orphaned" section. This section contains
// the nodes that follow the heading of the parent section, and precede the
// first sub-heading.
type Section struct {
	ast.BaseBlock

	Heading *ast.Heading
	Name    string
}

func (n *Section) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindSection is the ast.NodeKind corresponding to Section.
var KindSection = ast.NewNodeKind("Section")

func (n *Section) Kind() ast.NodeKind {
	return KindSection
}

// NewSection returns an initialized Section node.
func NewSection() *Section {
	return &Section{
		BaseBlock: ast.BaseBlock{},
	}
}

// sectionTransformer satisfies the parser.ASTTransformer interface.
type sectionTransformer struct{}

// NewSectionTransformer returns a parser.ASTTransformer that inserts Sections
// into the tree.
func NewSectionTransformer() parser.ASTTransformer {
	return &sectionTransformer{}
}

func compileSection(parent ast.Node, source []byte, level int) {
	var subs []*Section
	section := NewSection()
	for current := parent.FirstChild(); current != nil; {
		next := current.NextSibling()
		if heading, ok := current.(*ast.Heading); ok && heading.Level == level {
			if section.Heading != nil || section.ChildCount() > 0 {
				parent.InsertBefore(parent, heading, section)
				subs = append(subs, section)
			}
			// Begin next section.
			section = NewSection()
			section.Heading = heading
			id, _ := heading.AttributeString("section")
			if id, ok := id.([]byte); ok {
				section.Name = string(id)
			} else {
				id, _ := heading.AttributeString("id")
				if id, ok := id.([]byte); ok {
					section.Name = string(id)
				} else {
					section.Name = string(heading.Text(source))
				}
			}
		} else {
			section.AppendChild(section, current)
		}
		current = next
	}
	if section.Heading != nil || section.ChildCount() > 0 {
		parent.AppendChild(parent, section)
		subs = append(subs, section)
	}
	if level >= 6 {
		return
	}
	for _, section := range subs {
		if section.Heading != nil {
			compileSection(section, source, level+1)
		}
	}
}

// Transform recursively inserts Section nodes into the tree of doc. A section
// is delimited by Heading nodes. The list of nodes in the document becomes a
// tree of sections.
func (s sectionTransformer) Transform(doc *ast.Document, r text.Reader, pc parser.Context) {
	compileSection(doc, r.Source(), 1)
	return
}

// sectionRenderer satisfies various renderer interfaces.
type sectionRenderer struct{}

// NewSectionRenderer returns a renderer.NodeRenderer that renders a Section
// node.
func NewSectionRenderer() renderer.NodeRenderer {
	return sectionRenderer{}
}

// RegisterFuncs registers KindSection with renderSection.
func (s sectionRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindSection, s.renderSection)
}

// renderSection renders a Section node by simply falling through to the
// children of section.
func (sectionRenderer) renderSection(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}
