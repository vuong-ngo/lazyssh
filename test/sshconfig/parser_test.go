// ==============================================================================
// File: test/sshconfig/parser_test.go
// Project: lazyssh
// Description: Exhaustive integration and unit tests for SSH config parser AST,
//              directive case-insensitivity, Match block isolation, Include cycle
//              detection, and atomic file persistence.
// Note: Uses 100% pure Go standard library (testing, os, filepath) with zero external plugins.
// ==============================================================================

package sshconfig_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/sshconfig"
)

const complexSSHConfig = `# Top-level configuration header
# Author: System Engineer

Host prod-api
    HostName 192.168.1.120
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519_prod
    ProxyJump bastion-edge
    LocalForward 8080 localhost:80
    ForwardAgent yes
    # Inline comment inside block
    ServerAliveInterval 60

Host staging-db
    HostName db.staging.internal
    User postgres
    Port 5432
`

func TestParser_ParseComplexConfig(t *testing.T) {
	p := sshconfig.NewParser()
	doc, hosts, err := p.Parse(strings.NewReader(complexSSHConfig))
	if err != nil {
		t.Fatalf("unexpected error parsing complex config: %v", err)
	}

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	// 1. Verify Host 1
	h1 := hosts[0]
	if h1.Alias != "prod-api" {
		t.Errorf("host 1: expected alias 'prod-api', got '%s'", h1.Alias)
	}
	if h1.HostName != "192.168.1.120" {
		t.Errorf("host 1: expected HostName '192.168.1.120', got '%s'", h1.HostName)
	}
	if h1.User != "deploy" {
		t.Errorf("host 1: expected User 'deploy', got '%s'", h1.User)
	}
	if h1.Port != 2222 {
		t.Errorf("host 1: expected Port 2222, got %d", h1.Port)
	}
	if len(h1.IdentityFiles) != 1 || h1.IdentityFiles[0] != "~/.ssh/id_ed25519_prod" {
		t.Errorf("host 1: expected IdentityFile '~/.ssh/id_ed25519_prod', got %v", h1.IdentityFiles)
	}
	if h1.ProxyJump != "bastion-edge" {
		t.Errorf("host 1: expected ProxyJump 'bastion-edge', got '%s'", h1.ProxyJump)
	}
	if !h1.ForwardAgent {
		t.Errorf("host 1: expected ForwardAgent true, got false")
	}
	if h1.ExtraDirectives["ServerAliveInterval"] != "60" {
		t.Errorf("host 1: expected ServerAliveInterval 60, got '%s'", h1.ExtraDirectives["ServerAliveInterval"])
	}
	if len(h1.LocalForwards) != 1 {
		t.Fatalf("host 1: expected 1 LocalForward, got %d", len(h1.LocalForwards))
	}
	lf := h1.LocalForwards[0]
	if lf.Type != domain.ForwardLocal || lf.BindPort != 8080 || lf.TargetHost != "localhost" || lf.TargetPort != 80 {
		t.Errorf("host 1: unexpected LocalForward %+v", lf)
	}

	// 2. Verify Host 2
	h2 := hosts[1]
	if h2.Alias != "staging-db" {
		t.Errorf("host 2: expected alias 'staging-db', got '%s'", h2.Alias)
	}
	if h2.Port != 5432 {
		t.Errorf("host 2: expected Port 5432, got %d", h2.Port)
	}

	// 3. Round-trip AST serialization test (ensures 100% fidelity preservation)
	serialized := doc.Serialize()
	if serialized != complexSSHConfig {
		t.Errorf("AST serialization mismatch!\n--- EXPECTED ---\n%s\n--- GOT ---\n%s", complexSSHConfig, serialized)
	}
}

func TestParser_EmptyStream(t *testing.T) {
	p := sshconfig.NewParser()
	doc, hosts, err := p.Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error parsing empty stream: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts from empty stream, got %d", len(hosts))
	}
	if doc.Serialize() != "" {
		t.Errorf("expected empty serialization, got '%s'", doc.Serialize())
	}
}

func TestParser_DirectiveCaseInsensitivityAndSeparators(t *testing.T) {
	config := `host mixed-case
    hostname=10.0.0.5
    user = ec2-user
    PORT = 8022
    IDENTITYFILE = ~/.ssh/custom_key
    FORWARDAGENT = YES
`
	p := sshconfig.NewParser()
	_, hosts, err := p.Parse(strings.NewReader(config))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	h := hosts[0]
	if h.Alias != "mixed-case" {
		t.Errorf("expected alias 'mixed-case', got '%s'", h.Alias)
	}
	if h.HostName != "10.0.0.5" {
		t.Errorf("expected HostName '10.0.0.5', got '%s'", h.HostName)
	}
	if h.User != "ec2-user" {
		t.Errorf("expected User 'ec2-user', got '%s'", h.User)
	}
	if h.Port != 8022 {
		t.Errorf("expected Port 8022, got %d", h.Port)
	}
	if len(h.IdentityFiles) != 1 || h.IdentityFiles[0] != "~/.ssh/custom_key" {
		t.Errorf("expected IdentityFile '~/.ssh/custom_key', got %v", h.IdentityFiles)
	}
	if !h.ForwardAgent {
		t.Errorf("expected ForwardAgent true from YES, got false")
	}
}

func TestParser_MatchBlockIsolation(t *testing.T) {
	config := `Host server-1
    HostName 192.168.1.1
    User admin

Match host *.local exec "check-network"
    User override-user
`
	p := sshconfig.NewParser()
	_, hosts, err := p.Parse(strings.NewReader(config))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}

	// Directives inside Match block must NOT leak into server-1
	if hosts[0].User != "admin" {
		t.Errorf("expected user 'admin', but got '%s' (Match block leaked!)", hosts[0].User)
	}
}

func TestParser_CircularIncludeDetection(t *testing.T) {
	tempDir := t.TempDir()

	fileA := filepath.Join(tempDir, "fileA.conf")
	fileB := filepath.Join(tempDir, "fileB.conf")

	// File A includes File B; File B includes File A
	contentA := "Include " + fileB + "\nHost host-from-a\n    HostName 1.1.1.1\n"
	contentB := "Include " + fileA + "\nHost host-from-b\n    HostName 2.2.2.2\n"

	if err := os.WriteFile(fileA, []byte(contentA), 0600); err != nil {
		t.Fatalf("writing fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte(contentB), 0600); err != nil {
		t.Fatalf("writing fileB: %v", err)
	}

	p := sshconfig.NewParser()
	// Must terminate cleanly without stack overflow or infinite loop
	_, hosts, err := p.ParseFile(fileA)
	if err != nil {
		t.Fatalf("unexpected error parsing circular include: %v", err)
	}

	if len(hosts) == 0 {
		t.Errorf("expected hosts to be resolved despite circular include, got 0")
	}
}

func TestParser_NonExistentFile(t *testing.T) {
	p := sshconfig.NewParser()
	_, _, err := p.ParseFile("/non/existent/path/config")
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func TestWriter_WriteAtomicAndPermissions(t *testing.T) {
	tempDir := t.TempDir()
	// Nested directory to test directory auto-creation with 0700 permissions
	nestedDir := filepath.Join(tempDir, "nested", ".ssh")
	targetPath := filepath.Join(nestedDir, "config")

	p := sshconfig.NewParser()
	doc, _, err := p.Parse(strings.NewReader(complexSSHConfig))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	w := sshconfig.NewWriter()
	if err := w.WriteAtomic(targetPath, doc); err != nil {
		t.Fatalf("unexpected error in WriteAtomic: %v", err)
	}

	// 1. Verify parent directory permissions (POSIX 0700 on Linux/macOS)
	dirInfo, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm() != 0700 {
		t.Errorf("expected directory perm 0700 on POSIX, got %v", dirInfo.Mode().Perm())
	}

	// 2. Verify file permissions (POSIX 0600 on Linux/macOS)
	fileInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if runtime.GOOS != "windows" && fileInfo.Mode().Perm() != 0600 {
		t.Errorf("expected file perm 0600 on POSIX, got %v", fileInfo.Mode().Perm())
	}

	// 3. Verify content fidelity
	readBytes, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read back written file: %v", err)
	}
	if string(readBytes) != complexSSHConfig {
		t.Errorf("read-back content mismatch!\n--- GOT ---\n%s", string(readBytes))
	}
}

func TestParser_CRLFLineEndingPreservation(t *testing.T) {
	// Windows standard CRLF (\r\n) line endings
	crlfConfig := "# Windows CRLF Config\r\n\r\nHost win-server\r\n    HostName 10.0.0.99\r\n    User Administrator\r\n"

	p := sshconfig.NewParser()
	doc, hosts, err := p.Parse(strings.NewReader(crlfConfig))
	if err != nil {
		t.Fatalf("unexpected parse error for CRLF: %v", err)
	}

	if len(hosts) != 1 || hosts[0].Alias != "win-server" {
		t.Fatalf("expected 1 host 'win-server', got %+v", hosts)
	}

	serialized := doc.Serialize()
	if serialized != crlfConfig {
		t.Errorf("expected CRLF line endings to be 100%% preserved on serialization!\n--- EXPECTED (%d bytes) ---\n%q\n--- GOT (%d bytes) ---\n%q",
			len(crlfConfig), crlfConfig, len(serialized), serialized)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path, err := sshconfig.DefaultConfigPath()
	if err != nil {
		t.Fatalf("unexpected error finding default config path: %v", err)
	}

	if path == "" {
		t.Errorf("expected non-empty default config path")
	}

	expectedSuffix := filepath.Join(".ssh", "config")
	if !strings.HasSuffix(path, expectedSuffix) {
		t.Errorf("expected path to end with '%s', got '%s'", expectedSuffix, path)
	}
}
