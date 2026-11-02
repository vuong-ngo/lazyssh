// ==============================================================================
// File: internal/infrastructure/sshconfig/ast.go
// Project: lazyssh
// Description: Abstract Syntax Tree (AST) definitions for OpenSSH config files,
//              preserving comments, blank lines, and original indentation.
// ==============================================================================

package sshconfig

import (
	"strings"
)

// Node represents a single structural element in an SSH configuration file.
type Node interface {
	// String returns the exact textual representation of the node.
	String() string
}

// CommentNode represents a comment line starting with '#'.
type CommentNode struct {
	Raw string
}

func (c CommentNode) String() string {
	return c.Raw
}

// EmptyLineNode represents a blank or whitespace-only line.
type EmptyLineNode struct {
	Raw string
}

func (e EmptyLineNode) String() string {
	return e.Raw
}

// DirectiveNode represents a configuration key-value pair, preserving leading indentation.
type DirectiveNode struct {
	LeadingWhitespace string
	Key               string
	Separator         string // usually space or '='
	Value             string
	TrailingComment   string
}

func (d DirectiveNode) String() string {
	sep := d.Separator
	if sep == "" {
		sep = " "
	}
	result := d.LeadingWhitespace + d.Key + sep + d.Value
	if d.TrailingComment != "" {
		result += " " + d.TrailingComment
	}
	return result
}

// HostBlockNode groups a Host header with all its associated directives and comments.
type HostBlockNode struct {
	Header   DirectiveNode
	Children []Node
}

func (h HostBlockNode) String() string {
	var sb strings.Builder
	sb.WriteString(h.Header.String())
	for _, child := range h.Children {
		sb.WriteString("\n")
		sb.WriteString(child.String())
	}
	return sb.String()
}

// Document represents an entire parsed SSH config file preserving all AST nodes.
type Document struct {
	Nodes      []Node
	LineEnding string // "\n" (Unix/macOS) or "\r\n" (Windows CRLF)
}

// Serialize converts the entire AST back into its textual format,
// strictly honoring the detected line-ending convention (LF vs CRLF).
func (d *Document) Serialize() string {
	eol := d.LineEnding
	if eol == "" {
		eol = "\n"
	}
	var sb strings.Builder
	for i, node := range d.Nodes {
		if i > 0 {
			sb.WriteString(eol)
		}
		sb.WriteString(node.String())
	}
	if len(d.Nodes) > 0 {
		sb.WriteString(eol)
	}
	return sb.String()
}
