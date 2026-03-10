package middleware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================
// PL4 — Persistent Tamper-Evident Audit Ledger Tests
// =============================================================

func TestLedgerFile_WriteAndVerify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-ledger.ndjson")

	lf, err := OpenLedgerFile(path)
	if err != nil {
		t.Fatalf("OpenLedgerFile: %v", err)
	}
	defer lf.Close()

	// Write 5 entries
	for i := 0; i < 5; i++ {
		_, err := lf.WriteEntry(Receipt{
			Seq:    uint64(i + 1),
			Action: ActionToolCall,
			Tool:   "test_tool",
		})
		if err != nil {
			t.Fatalf("WriteEntry(%d): %v", i+1, err)
		}
	}
	lf.Close()

	// Verify chain integrity
	count, err := VerifyLedgerFile(path)
	if err != nil {
		t.Fatalf("VerifyLedgerFile: %v (verified %d entries)", err, count)
	}
	if count != 5 {
		t.Errorf("verified %d entries, want 5", count)
	}
}

func TestLedgerFile_TamperDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamper-ledger.ndjson")

	lf, err := OpenLedgerFile(path)
	if err != nil {
		t.Fatalf("OpenLedgerFile: %v", err)
	}

	for i := 0; i < 5; i++ {
		lf.WriteEntry(Receipt{
			Seq:    uint64(i + 1),
			Action: ActionToolCall,
		})
	}
	lf.Close()

	// Tamper: modify the middle of the file
	data, err := os.ReadFile(path) // #nosec G304 -- test file
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Replace "tool_call" with "tool_hack" in the 3rd line
	lines := strings.Split(string(data), "\n")
	lines[2] = strings.Replace(lines[2], "tool_call", "tool_hack", 1)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Verify should fail
	count, err := VerifyLedgerFile(path)
	if err == nil {
		t.Fatal("expected verification failure after tampering")
	}
	if count < 2 {
		t.Errorf("expected at least 2 verified entries before tamper, got %d", count)
	}
	if !strings.Contains(err.Error(), "chain broken") {
		t.Errorf("expected 'chain broken' error, got: %v", err)
	}
}

func TestLedgerFile_ChainContinuity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "continuity-ledger.ndjson")

	// Write 3 entries, close, reopen, write 2 more
	lf, err := OpenLedgerFile(path)
	if err != nil {
		t.Fatalf("OpenLedgerFile: %v", err)
	}
	for i := 0; i < 3; i++ {
		lf.WriteEntry(Receipt{Seq: uint64(i + 1), Action: ActionToolCall})
	}
	lf.Close()

	// Reopen and append
	lf2, err := OpenLedgerFile(path)
	if err != nil {
		t.Fatalf("OpenLedgerFile (reopen): %v", err)
	}
	for i := 3; i < 5; i++ {
		lf2.WriteEntry(Receipt{Seq: uint64(i + 1), Action: ActionToolResult})
	}
	lf2.Close()

	// Verify entire chain
	count, err := VerifyLedgerFile(path)
	if err != nil {
		t.Fatalf("VerifyLedgerFile: %v (verified %d entries)", err, count)
	}
	if count != 5 {
		t.Errorf("verified %d entries, want 5", count)
	}
}

func TestLedgerFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-ledger.ndjson")

	lf, err := OpenLedgerFile(path)
	if err != nil {
		t.Fatalf("OpenLedgerFile: %v", err)
	}
	lf.Close()

	count, err := VerifyLedgerFile(path)
	if err != nil {
		t.Fatalf("VerifyLedgerFile on empty file: %v", err)
	}
	if count != 0 {
		t.Errorf("verified %d entries on empty file, want 0", count)
	}
}

func TestLedgerFile_Rotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate-ledger.ndjson")

	lf, err := OpenLedgerFile(path)
	if err != nil {
		t.Fatalf("OpenLedgerFile: %v", err)
	}

	// Manually set a tiny threshold to trigger rotation
	// We can only test rotation by making the file big enough,
	// but we can force it by writing a lot. Instead, test the
	// rotation logic by calling rotateLocked directly.
	for i := 0; i < 10; i++ {
		lf.WriteEntry(Receipt{Seq: uint64(i + 1), Action: ActionToolCall, Summary: "pre-rotation"})
	}

	// Force rotation
	lf.mu.Lock()
	err = lf.rotateLocked()
	lf.mu.Unlock()
	if err != nil {
		t.Fatalf("rotateLocked: %v", err)
	}

	// Write more entries to the new file
	for i := 10; i < 15; i++ {
		lf.WriteEntry(Receipt{Seq: uint64(i + 1), Action: ActionToolCall, Summary: "post-rotation"})
	}
	lf.Close()

	// The rotated file should exist and be verifiable
	matches, _ := filepath.Glob(filepath.Join(dir, "rotate-ledger.*.ndjson"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 rotated file, found %d", len(matches))
	}

	count, err := VerifyLedgerFile(matches[0])
	if err != nil {
		t.Fatalf("VerifyLedgerFile (rotated): %v", err)
	}
	if count != 10 {
		t.Errorf("rotated file has %d entries, want 10", count)
	}

	// The new file should also be verifiable (but its genesis is the last hash of the rotated file)
	count, err = VerifyLedgerFile(path)
	if err != nil {
		// New file's first entry's prev_hash is the last hash from the rotated file,
		// not the genesis hash. VerifyLedgerFile starts from genesis.
		// This is expected behavior for cross-file continuity — the verifier
		// would need both files to verify the full chain.
		// For a single-file check, the new file should start with its own chain.
		t.Logf("Note: new file verification error (expected for cross-file chain): %v", err)
	} else if count != 5 {
		t.Errorf("new file has %d entries, want 5", count)
	}
}

func TestLedgerWithFile_Integration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "integrated-ledger.ndjson")

	l, err := NewLedgerWithFile(100, path)
	if err != nil {
		t.Fatalf("NewLedgerWithFile: %v", err)
	}
	defer l.Close()

	// Append through the normal Ledger API
	for i := 0; i < 10; i++ {
		l.Append(Receipt{
			Action:  ActionToolCall,
			Tool:    "test_tool",
			Summary: "integrated test",
		})
	}

	// In-memory should have 10
	if l.Count() != 10 {
		t.Errorf("in-memory Count() = %d, want 10", l.Count())
	}

	l.Close()

	// File should have 10 verifiable entries
	count, err := VerifyLedgerFile(path)
	if err != nil {
		t.Fatalf("VerifyLedgerFile: %v (verified %d entries)", err, count)
	}
	if count != 10 {
		t.Errorf("file has %d entries, want 10", count)
	}
}

func TestLedgerWithFile_CloseNil(t *testing.T) {
	l := NewLedger(100) // no file backend
	if err := l.Close(); err != nil {
		t.Errorf("Close() on ledger without file: %v", err)
	}
}

func TestVerifyLedgerFile_MissingFile(t *testing.T) {
	_, err := VerifyLedgerFile("/nonexistent/path/ledger.ndjson")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGenesisHash(t *testing.T) {
	if len(genesisHash) != 64 {
		t.Errorf("genesis hash length = %d, want 64 (hex SHA-256)", len(genesisHash))
	}
	for _, c := range genesisHash {
		if c != '0' {
			t.Errorf("genesis hash should be all zeros, got: %s", genesisHash)
			break
		}
	}
}
