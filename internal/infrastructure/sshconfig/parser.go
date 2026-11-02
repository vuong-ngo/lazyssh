// ==============================================================================
// File: internal/infrastructure/sshconfig/parser.go
// Project: lazyssh
// Description: Line-by-line streaming parser for OpenSSH configuration files,
//              supporting Include resolution with cycle detection.
// ==============================================================================

package sshconfig

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vuong-ngo/lazyssh/internal/domain"
)

// DefaultConfigPath returns the canonical path to the user's primary SSH config.
// On Linux & macOS: ~/.ssh/config
// On Windows: %USERPROFILE%\.ssh\config
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating user home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// Parser handles parsing OpenSSH configuration files into an AST and domain Entities.
type Parser struct {
	visitedFiles map[string]struct{}
}

// NewParser creates a new Parser instance with an empty visited file registry.
func NewParser() *Parser {
	return &Parser{
		visitedFiles: make(map[string]struct{}),
	}
}

// ParseFile parses the SSH configuration file at the specified path.
func (p *Parser) ParseFile(filePath string) (*Document, []domain.Host, error) {
	expandedPath, err := expandHome(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("expanding path '%s': %w", filePath, err)
	}

	canonicalPath, err := filepath.EvalSymlinks(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			canonicalPath = expandedPath
		} else {
			return nil, nil, fmt.Errorf("evaluating symlinks for '%s': %w", expandedPath, err)
		}
	}

	if _, visited := p.visitedFiles[canonicalPath]; visited {
		// Prevent circular recursion in Include directives
		return &Document{}, nil, nil
	}
	p.visitedFiles[canonicalPath] = struct{}{}

	file, err := os.Open(expandedPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening ssh config '%s': %w", expandedPath, err)
	}
	defer file.Close()

	doc, hosts, err := p.Parse(file)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing '%s': %w", expandedPath, err)
	}

	// Annotate source file on parsed hosts
	for i := range hosts {
		hosts[i].SourceFile = expandedPath
	}

	// Resolve Include directives
	for _, node := range doc.Nodes {
		if directive, ok := node.(DirectiveNode); ok && strings.EqualFold(directive.Key, "Include") {
			includedHosts, err := p.resolveInclude(directive.Value, filepath.Dir(expandedPath))
			if err == nil {
				hosts = append(hosts, includedHosts...)
			}
		}
	}

	return doc, hosts, nil
}

// Parse reads an SSH config stream and returns its AST Document and parsed Hosts.
// Cross-platform guarantee: Automatically detects CRLF vs LF line endings.
func (p *Parser) Parse(r io.Reader) (*Document, []domain.Host, error) {
	br := bufio.NewReader(r)
	peekBytes, _ := br.Peek(4096)
	lineEnding := "\n"
	if bytes.Contains(peekBytes, []byte("\r\n")) {
		lineEnding = "\r\n"
	}

	scanner := bufio.NewScanner(br)
	doc := &Document{
		LineEnding: lineEnding,
	}
	var hosts []domain.Host

	var currentHost *domain.Host
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		rawLine := scanner.Text()
		trimmedLine := strings.TrimSpace(rawLine)

		// 1. Empty lines
		if trimmedLine == "" {
			doc.Nodes = append(doc.Nodes, EmptyLineNode{Raw: rawLine})
			continue
		}

		// 2. Comments
		if strings.HasPrefix(trimmedLine, "#") {
			doc.Nodes = append(doc.Nodes, CommentNode{Raw: rawLine})
			continue
		}

		// 3. Directive parsing
		directive := parseDirectiveLine(rawLine)
		doc.Nodes = append(doc.Nodes, directive)

		// Check for Host header
		if strings.EqualFold(directive.Key, "Host") {
			if currentHost != nil && currentHost.Alias != "" {
				hosts = append(hosts, *currentHost)
			}

			aliases := strings.Fields(directive.Value)
			if len(aliases) == 0 {
				currentHost = nil
				continue
			}

			// For simplicity in discrete host targeting, pick the primary alias
			// (Wildcards like '*' or '*.domain' will be stored but can be filtered)
			primaryAlias := aliases[0]
			currentHost = &domain.Host{
				Alias:           primaryAlias,
				ExtraDirectives: make(map[string]string),
				LineNumber:      lineNum,
			}
			continue
		}

		// Check for Match header (Match block terminates active Host block)
		if strings.EqualFold(directive.Key, "Match") {
			if currentHost != nil && currentHost.Alias != "" {
				hosts = append(hosts, *currentHost)
				currentHost = nil
			}
			continue
		}

		// If we are currently inside a Host block, populate its attributes
		if currentHost != nil {
			populateHostAttribute(currentHost, directive)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading config stream: %w", err)
	}

	if currentHost != nil && currentHost.Alias != "" {
		hosts = append(hosts, *currentHost)
	}

	return doc, hosts, nil
}

func parseDirectiveLine(raw string) DirectiveNode {
	leading := ""
	for i, r := range raw {
		if r != ' ' && r != '\t' {
			leading = raw[:i]
			break
		}
	}

	trimmed := strings.TrimSpace(raw)
	trailingComment := ""
	if commentIdx := strings.Index(trimmed, "#"); commentIdx != -1 {
		trailingComment = trimmed[commentIdx:]
		trimmed = strings.TrimSpace(trimmed[:commentIdx])
	}

	var key, val, sep string

	// Directives in OpenSSH can use whitespace or '=' (e.g. "Key Value", "Key=Value", "Key = Value")
	if eqIdx := strings.Index(trimmed, "="); eqIdx != -1 {
		beforeEq := strings.TrimSpace(trimmed[:eqIdx])
		// Check that there is only one word before '=' (the directive key)
		if !strings.ContainsAny(beforeEq, " \t") && beforeEq != "" {
			key = beforeEq
			val = strings.TrimSpace(trimmed[eqIdx+1:])
			sep = trimmed[len(beforeEq) : eqIdx+1]
			if strings.HasPrefix(trimmed[eqIdx+1:], " ") || strings.HasPrefix(trimmed[eqIdx+1:], "\t") {
				sep += " "
			}
			return DirectiveNode{
				LeadingWhitespace: leading,
				Key:               key,
				Separator:         sep,
				Value:             val,
				TrailingComment:   trailingComment,
			}
		}
	}

	// Default whitespace separator
	fields := strings.Fields(trimmed)
	if len(fields) > 0 {
		key = fields[0]
		keyEnd := strings.Index(trimmed, key) + len(key)
		rest := trimmed[keyEnd:]
		sep = " "
		val = strings.TrimSpace(rest)
	}

	return DirectiveNode{
		LeadingWhitespace: leading,
		Key:               key,
		Separator:         sep,
		Value:             val,
		TrailingComment:   trailingComment,
	}
}

func populateHostAttribute(h *domain.Host, d DirectiveNode) {
	val := cleanValue(d.Value)

	switch {
	case strings.EqualFold(d.Key, "HostName"):
		h.HostName = val
	case strings.EqualFold(d.Key, "User"):
		h.User = val
	case strings.EqualFold(d.Key, "Port"):
		if p, err := strconv.Atoi(val); err == nil {
			h.Port = p
		}
	case strings.EqualFold(d.Key, "IdentityFile"):
		h.IdentityFiles = append(h.IdentityFiles, val)
	case strings.EqualFold(d.Key, "ProxyJump"):
		h.ProxyJump = val
	case strings.EqualFold(d.Key, "ProxyCommand"):
		h.ProxyCommand = val
	case strings.EqualFold(d.Key, "ForwardAgent"):
		h.ForwardAgent = strings.EqualFold(val, "yes") || strings.EqualFold(val, "true")
	case strings.EqualFold(d.Key, "LocalForward"):
		if lf := parseForward(domain.ForwardLocal, val); lf != nil {
			h.LocalForwards = append(h.LocalForwards, *lf)
		}
	case strings.EqualFold(d.Key, "RemoteForward"):
		if rf := parseForward(domain.ForwardRemote, val); rf != nil {
			h.RemoteForwards = append(h.RemoteForwards, *rf)
		}
	case strings.EqualFold(d.Key, "DynamicForward"):
		h.DynamicForwards = append(h.DynamicForwards, val)
	default:
		if h.ExtraDirectives == nil {
			h.ExtraDirectives = make(map[string]string)
		}
		h.ExtraDirectives[d.Key] = val
	}
}

func parseForward(fType domain.ForwardType, val string) *domain.PortForward {
	fields := strings.Fields(val)
	if len(fields) == 0 {
		return nil
	}

	// Syntax A: LocalForward 8080 target:80
	// Syntax B: LocalForward bind_host:8080 target:80
	if len(fields) == 2 {
		bindPart := fields[0]
		targetPart := fields[1]

		bindHost := "127.0.0.1"
		bindPort := 0
		if strings.Contains(bindPart, ":") {
			parts := strings.Split(bindPart, ":")
			bindHost = parts[0]
			bindPort, _ = strconv.Atoi(parts[1])
		} else {
			bindPort, _ = strconv.Atoi(bindPart)
		}

		targetHost := "localhost"
		targetPort := 0
		if strings.Contains(targetPart, ":") {
			parts := strings.Split(targetPart, ":")
			targetHost = parts[0]
			targetPort, _ = strconv.Atoi(parts[1])
		}

		return &domain.PortForward{
			Type:       fType,
			BindHost:   bindHost,
			BindPort:   bindPort,
			TargetHost: targetHost,
			TargetPort: targetPort,
		}
	}
	return nil
}

func (p *Parser) resolveInclude(pattern string, baseDir string) ([]domain.Host, error) {
	pattern, err := expandHome(pattern)
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(baseDir, pattern)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing pattern '%s': %w", pattern, err)
	}

	var includedHosts []domain.Host
	for _, match := range matches {
		_, hosts, err := p.ParseFile(match)
		if err == nil {
			includedHosts = append(includedHosts, hosts...)
		}
	}

	return includedHosts, nil
}

func expandHome(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rel := filepath.FromSlash(path[2:])
		return filepath.Join(home, rel), nil
	}
	return filepath.Clean(path), nil
}

func cleanValue(v string) string {
	v = strings.TrimSpace(v)
	// Strip surrounding double quotes if present
	if strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") && len(v) >= 2 {
		return v[1 : len(v)-1]
	}
	return v
}
