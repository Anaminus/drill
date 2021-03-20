// The filesys package implements drill.Node for the io/fs package.
package filesys

import (
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/anaminus/drill"
)

// FS wraps an fs.FS to implement drill.Node.
//
// Included in FS are a number of FileHandlers. Any produced child FS will
// inherit these handlers.
//
// In order to descend into a file, the file must have an extension that matches
// one of these handlers (according Handlers.Match). If there is a match, the
// handler is called, returning the node returned by the handler.
type FS struct {
	fs.FS
	handlers Handlers
}

// NewFS returns an FS that wraps fsys, and includes a number of handlers.
// Returns an error if a handler pattern is malformed.
func NewFS(fsys fs.FS, handlers Handlers) (*FS, error) {
	for i, handler := range handlers {
		if _, err := path.Match(handler.Pattern, ""); err != nil {
			return nil, fmt.Errorf("handler %d, pattern %q: %w", i, handler.Pattern, err)
		}
	}
	f := FS{
		FS:       fsys,
		handlers: make(Handlers, len(handlers)),
	}
	copy(f.handlers, handlers)
	return &f, nil
}

// handle opens file name using the handlers of the FS.
func (f *FS) handle(name string) drill.Node {
	if f.handlers == nil {
		return nil
	}
	handler := f.handlers.Match(name)
	if handler == nil {
		return nil
	}
	return handler(f.FS, name)
}

// Handlers returns a list of the Handlers used by the node.
func (f *FS) Handlers() Handlers {
	handlers := make(Handlers, len(f.handlers))
	copy(handlers, f.handlers)
	return handlers
}

// Fragment returns the content of the file. Returns an empty string if the file
// does not exist, is a directory, or otherwise returns an error.
func (f *FS) Fragment() string {
	b, err := fs.ReadFile(f.FS, ".")
	if err != nil {
		return ""
	}
	return string(b)
}

// FragmentReader opens and returns the file.
func (f *FS) FragmentReader() (r io.ReadCloser, err error) {
	return f.Open(".")
}

// Len returns the number of files in the directory.
func (f *FS) Len() int {
	if f.FS == nil {
		return 0
	}
	sub, _ := fs.ReadDir(f.FS, ".")
	return len(sub)
}

// OrderedChild returns the file at index i from the results of fs.ReadDir.
func (f *FS) OrderedChild(i int) drill.Node {
	if f.FS == nil {
		return nil
	}
	subs, err := fs.ReadDir(f.FS, ".")
	if err != nil {
		return nil
	}
	if i = drill.Index(i, len(subs)); i < 0 {
		return nil
	}
	sub, err := fs.Sub(f.FS, subs[i].Name())
	if err != nil {
		return nil
	}
	return &FS{FS: sub, handlers: f.handlers}
}

// OrderedChildren returns each file in the directory.
func (f *FS) OrderedChildren() []drill.Node {
	if f.FS == nil {
		return nil
	}
	fsys, ok := f.FS.(fs.SubFS)
	if !ok {
		return nil
	}
	subs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	nodes := make([]drill.Node, 0, len(subs))
	for i, entry := range subs {
		if sub, err := fsys.Sub(entry.Name()); err == nil {
			nodes[i] = &FS{FS: sub}
		}
	}
	return nodes
}

// UnorderedChild returns the file matching name, or nil if it does not exist.
// Returns nil if the wrapped FS does not implement fs.SubFS.
func (f *FS) UnorderedChild(name string) drill.Node {
	if f.FS == nil {
		return nil
	}
	info, err := fs.Stat(f.FS, name)
	if err != nil {
		return f.handle(name)
	}
	if !info.IsDir() {
		return f.handle(name)
	}
	sub, err := fs.Sub(f.FS, name)
	if err != nil {
		return nil
	}
	return &FS{FS: sub, handlers: f.handlers}
}

// UnorderedChildren returns each file in the directory.
func (f *FS) UnorderedChildren() map[string]drill.Node {
	entries, err := fs.ReadDir(f.FS, ".")
	if err != nil {
		return map[string]drill.Node{}
	}
	children := make(map[string]drill.Node, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if sub, err := fs.Sub(f.FS, name); err == nil {
				children[name] = &FS{FS: sub, handlers: f.handlers}
			}
		} else {
			if sub := f.handle(name); sub != nil {
				children[name] = sub
			}
		}
	}
	return children
}

// Handler maps a glob pattern to a HandlerFunc.
type Handler struct {
	Pattern string
	Func    HandlerFunc
}

// Handlers is an ordered list of Handler values.
type Handlers []Handler

// Match returns the first Handler for which the glob pattern matches file.
// Returns nil if no matches were found.
func (hs Handlers) Match(file string) HandlerFunc {
	for _, handler := range hs {
		if ok, _ := path.Match(handler.Pattern, file); ok {
			return handler.Func
		}
	}
	return nil
}

// HandlerFunc produces a drill.Node from the given file, usually by calling
// fsys.Open(name), and interpreting the contents of the file.
type HandlerFunc func(fsys fs.FS, name string) drill.Node
