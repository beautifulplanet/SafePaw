// =============================================================
// SafePaw Gateway — Persistent Tamper-Evident Audit Ledger (PL4)
// =============================================================
// Adds SHA-256 hash-chained NDJSON persistence to the receipt ledger.
//
// Each line is a JSON object containing all Receipt fields plus:
//   prev_hash: SHA-256 hash of the previous line (hex)
//
// The first entry in a file uses a genesis hash (all zeros).
// To verify integrity: read line-by-line, hash each line, and
// confirm the next line's prev_hash matches.
//
// Rotation: when the file exceeds 100MB, it is renamed to
// <name>.<timestamp>.ndjson and a new file is started. The new
// file's genesis hash is the final hash of the rotated file,
// creating continuity across rotations.
// =============================================================

package middleware

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// ledgerMaxFileSize is the rotation threshold (100MB).
	ledgerMaxFileSize = 100 * 1024 * 1024

	// genesisHash is the prev_hash for the first entry in a chain.
	genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

// PersistentEntry is the on-disk format: Receipt + hash chain fields.
type PersistentEntry struct {
	Receipt
	PrevHash string `json:"prev_hash"`
}

// LedgerFile manages the NDJSON file with hash-chain integrity.
type LedgerFile struct {
	mu       sync.Mutex
	path     string   // current file path
	file     *os.File // open file handle
	prevHash string   // SHA-256 hash of the last written line
	size     int64    // current file size in bytes
}

// OpenLedgerFile opens or creates a persistent ledger file.
// If the file already exists, reads the last hash to continue the chain.
func OpenLedgerFile(path string) (*LedgerFile, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create ledger directory: %w", err)
	}

	info, err := os.Stat(path)
	var size int64
	prevHash := genesisHash
	if err == nil {
		size = info.Size()
		// Read the last line to recover the chain hash
		if size > 0 {
			lastHash, err := readLastHash(path)
			if err != nil {
				return nil, fmt.Errorf("recover chain hash: %w", err)
			}
			prevHash = lastHash
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- operator-configured path
	if err != nil {
		return nil, fmt.Errorf("open ledger file: %w", err)
	}

	return &LedgerFile{
		path:     path,
		file:     f,
		prevHash: prevHash,
		size:     size,
	}, nil
}

// WriteEntry appends a hash-chained receipt to the NDJSON file.
// Returns the SHA-256 hash of the written line.
func (lf *LedgerFile) WriteEntry(r Receipt) (string, error) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	// Check rotation before writing
	if lf.size >= ledgerMaxFileSize {
		if err := lf.rotateLocked(); err != nil {
			log.Printf("[LEDGER] Rotation failed: %v", err)
			// Continue writing to the current file — don't lose data
		}
	}

	entry := PersistentEntry{
		Receipt:  r,
		PrevHash: lf.prevHash,
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("marshal ledger entry: %w", err)
	}
	line = append(line, '\n')

	n, err := lf.file.Write(line)
	if err != nil {
		return "", fmt.Errorf("write ledger entry: %w", err)
	}
	lf.size += int64(n)

	// Compute hash of the line (without trailing newline) for chain
	hash := sha256Hex(line[:len(line)-1])
	lf.prevHash = hash

	return hash, nil
}

// Close closes the underlying file.
func (lf *LedgerFile) Close() error {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	if lf.file != nil {
		return lf.file.Close()
	}
	return nil
}

// Path returns the current file path.
func (lf *LedgerFile) Path() string {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	return lf.path
}

// rotateLocked renames the current file and opens a new one.
// Caller must hold lf.mu.
func (lf *LedgerFile) rotateLocked() error {
	// Close current file
	if err := lf.file.Close(); err != nil {
		return fmt.Errorf("close for rotation: %w", err)
	}

	// Rename: /path/to/ledger.ndjson -> /path/to/ledger.20260310T120000Z.ndjson
	ext := filepath.Ext(lf.path)
	base := strings.TrimSuffix(lf.path, ext)
	rotatedPath := fmt.Sprintf("%s.%s%s", base, time.Now().UTC().Format("20060102T150405Z"), ext)
	if err := os.Rename(lf.path, rotatedPath); err != nil {
		// Re-open the original file for continued writing
		var reOpenErr error
		lf.file, reOpenErr = os.OpenFile(lf.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- operator-configured path
		if reOpenErr != nil {
			return fmt.Errorf("rename failed (%v) and reopen failed: %w", err, reOpenErr)
		}
		return fmt.Errorf("rename for rotation: %w", err)
	}

	log.Printf("[LEDGER] Rotated: %s -> %s (prev_hash carried forward)", lf.path, rotatedPath)

	// Open new file — chain continues from the last hash
	f, err := os.OpenFile(lf.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- operator-configured path
	if err != nil {
		return fmt.Errorf("open new ledger after rotation: %w", err)
	}
	lf.file = f
	lf.size = 0
	// prevHash is NOT reset — carries forward for cross-file continuity

	return nil
}

// VerifyLedgerFile reads an NDJSON ledger file and verifies its hash chain.
// Returns (entryCount, nil) on success, or (entriesVerified, error) on failure.
func VerifyLedgerFile(path string) (int, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-provided path for verification
	if err != nil {
		return 0, fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()

	var count int
	prevHash := genesisHash // expected prev_hash for the first entry

	scanner := bufio.NewScanner(f)
	// Allow lines up to 1MB (receipts shouldn't be this large, but be safe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		count++

		// Parse the entry to check its prev_hash
		var entry PersistentEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return count - 1, fmt.Errorf("line %d: invalid JSON: %w", count, err)
		}

		// Verify chain: this entry's prev_hash must match the hash of the previous line
		if entry.PrevHash != prevHash {
			return count - 1, fmt.Errorf("line %d: chain broken: prev_hash=%s expected=%s",
				count, entry.PrevHash, prevHash)
		}

		// This line's hash becomes the expected prev_hash for the next line
		prevHash = sha256Hex(line)
	}

	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("scan error: %w", err)
	}

	return count, nil
}

// readLastHash reads the last non-empty line of the file and computes its hash.
func readLastHash(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- internal ledger file
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lastLine []byte
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) > 0 {
			lastLine = make([]byte, len(line))
			copy(lastLine, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if lastLine == nil {
		return genesisHash, nil
	}
	return sha256Hex(lastLine), nil
}

// sha256Hex returns the lowercase hex-encoded SHA-256 hash of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
