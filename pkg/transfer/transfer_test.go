package transfer

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// 1. Test AES-256-GCM Encryption & Decryption
func TestAES256GCMEncryption(t *testing.T) {
	key := make([]byte, 32) // 256-bit AES key
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	plaintext := []byte("LANShare High Performance Secret Transfer Payload 12345!")

	ciphertext, err := EncryptData(key, plaintext)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatalf("Ciphertext should not equal plaintext")
	}

	decrypted, err := DecryptData(key, ciphertext)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("Decrypted data mismatch: expected %s, got %s", string(plaintext), string(decrypted))
	}
}

// 2. Test Buffer Pool Memory Recycling
func TestBufferPool(t *testing.T) {
	bufPtr1 := BufferPool.Get().(*[]byte)
	if len(*bufPtr1) != ChunkSize {
		t.Fatalf("Expected buffer size %d, got %d", ChunkSize, len(*bufPtr1))
	}

	// Fill with test data
	(*bufPtr1)[0] = 0xAA
	BufferPool.Put(bufPtr1)

	bufPtr2 := BufferPool.Get().(*[]byte)
	if len(*bufPtr2) != ChunkSize {
		t.Fatalf("Expected recycled buffer size %d, got %d", ChunkSize, len(*bufPtr2))
	}
	BufferPool.Put(bufPtr2)
}

// 3. Test Resumable File Writing (Append & Offset seek)
func TestResumableFileWriter(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "resumable_test.bin")

	part1 := []byte("PART1_BYTES_0_TO_10_")
	part2 := []byte("PART2_BYTES_10_TO_20")

	// Phase 1: Write Part 1
	err := os.WriteFile(filePath, part1, 0644)
	if err != nil {
		t.Fatalf("Failed to write part 1: %v", err)
	}

	// Phase 2: Resume Write Part 2 from offset len(part1)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("Failed to open file for append: %v", err)
	}

	_, err = f.Write(part2)
	f.Close()
	if err != nil {
		t.Fatalf("Failed to append part 2: %v", err)
	}

	// Verify total file content
	finalData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read final file: %v", err)
	}

	expected := append(part1, part2...)
	if !bytes.Equal(finalData, expected) {
		t.Fatalf("Resumable file content mismatch: expected %s, got %s", string(expected), string(finalData))
	}
}
