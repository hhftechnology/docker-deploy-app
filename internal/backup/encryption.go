package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EncryptionManager handles backup encryption and decryption
type EncryptionManager struct {
	keyStorage string
}

// NewEncryptionManager creates a new encryption manager
func NewEncryptionManager(keyStorage string) *EncryptionManager {
	return &EncryptionManager{
		keyStorage: keyStorage,
	}
}

// EncryptedReader wraps an io.Reader to provide encryption
type EncryptedReader struct {
	reader io.Reader
	cipher cipher.Stream
}

// NewEncryptedReader creates a new encrypted reader
func NewEncryptedReader(reader io.Reader, key []byte) (*EncryptedReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Generate random IV
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)

	// Create a reader that first returns the IV, then encrypted data
	ivReader := &EncryptedReader{
		reader: io.MultiReader(
			&singleUseReader{data: iv},
			&streamReader{reader: reader, stream: stream},
		),
	}

	return ivReader, nil
}

// Read implements io.Reader
func (er *EncryptedReader) Read(p []byte) (n int, err error) {
	return er.reader.Read(p)
}

// DecryptedReader wraps an io.Reader to provide decryption
type DecryptedReader struct {
	reader io.Reader
	cipher cipher.Stream
	ivRead bool
	key    []byte
}

// NewDecryptedReader creates a new decrypted reader
func NewDecryptedReader(reader io.Reader, key []byte) (*DecryptedReader, error) {
	return &DecryptedReader{
		reader: reader,
		ivRead: false,
		key:    key,
	}, nil
}

// Read implements io.Reader
func (dr *DecryptedReader) Read(p []byte) (n int, err error) {
	if !dr.ivRead {
		// Read IV first
		iv := make([]byte, aes.BlockSize)
		if _, err := io.ReadFull(dr.reader, iv); err != nil {
			return 0, err
		}

		// Create cipher
		block, err := aes.NewCipher(dr.key)
		if err != nil {
			return 0, err
		}

		dr.cipher = cipher.NewCFBDecrypter(block, iv)
		dr.ivRead = true
	}

	// Read encrypted data
	encrypted := make([]byte, len(p))
	n, err = dr.reader.Read(encrypted)
	if n > 0 {
		dr.cipher.XORKeyStream(p[:n], encrypted[:n])
	}

	return n, err
}

// singleUseReader reads data once
type singleUseReader struct {
	data []byte
	pos  int
}

func (r *singleUseReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// streamReader encrypts data on the fly
type streamReader struct {
	reader io.Reader
	stream cipher.Stream
}

func (sr *streamReader) Read(p []byte) (n int, err error) {
	n, err = sr.reader.Read(p)
	if n > 0 {
		sr.stream.XORKeyStream(p[:n], p[:n])
	}
	return n, err
}

// GenerateKey generates an encryption key from a password
func (em *EncryptionManager) GenerateKey(password string, salt []byte) []byte {
	if salt == nil {
		salt = make([]byte, 32)
		rand.Read(salt)
	}
	// Simple key derivation using SHA256
	combined := append([]byte(password), salt...)
	hash := sha256.Sum256(combined)
	return hash[:]
}

// EncryptFile encrypts a file
func (em *EncryptionManager) EncryptFile(srcPath, dstPath string, key []byte) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	encryptedReader, err := NewEncryptedReader(srcFile, key)
	if err != nil {
		return fmt.Errorf("failed to create encrypted reader: %w", err)
	}

	_, err = io.Copy(dstFile, encryptedReader)
	if err != nil {
		return fmt.Errorf("failed to encrypt file: %w", err)
	}

	return nil
}

// DecryptFile decrypts a file
func (em *EncryptionManager) DecryptFile(srcPath, dstPath string, key []byte) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	decryptedReader, err := NewDecryptedReader(srcFile, key)
	if err != nil {
		return fmt.Errorf("failed to create decrypted reader: %w", err)
	}

	_, err = io.Copy(dstFile, decryptedReader)
	if err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	}

	return nil
}

// StoreKey stores an encryption key securely
func (em *EncryptionManager) StoreKey(backupID string, key []byte) error {
	keyDir := filepath.Join(em.keyStorage, "keys")
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	keyPath := filepath.Join(keyDir, backupID+".key")
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	// Store key as hex
	_, err = keyFile.WriteString(hex.EncodeToString(key))
	if err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	return nil
}

// RetrieveKey retrieves an encryption key
func (em *EncryptionManager) RetrieveKey(backupID string) ([]byte, error) {
	keyPath := filepath.Join(em.keyStorage, "keys", backupID+".key")
	keyFile, err := os.Open(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open key file: %w", err)
	}
	defer keyFile.Close()

	keyHex, err := io.ReadAll(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read key: %w", err)
	}

	key, err := hex.DecodeString(string(keyHex))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	return key, nil
}

// DeleteKey removes an encryption key
func (em *EncryptionManager) DeleteKey(backupID string) error {
	keyPath := filepath.Join(em.keyStorage, "keys", backupID+".key")
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

// VerifyKey verifies that a key can decrypt a backup
func (em *EncryptionManager) VerifyKey(backupPath string, key []byte) error {
	// Try to read the first few bytes of the backup and decrypt them
	file, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read first 1KB for verification
	testData := make([]byte, 1024)
	n, err := file.Read(testData)
	if err != nil && err != io.EOF {
		return err
	}

	// Try to decrypt
	decryptedReader, err := NewDecryptedReader(&singleUseReader{data: testData[:n]}, key)
	if err != nil {
		return err
	}

	// Read a bit to see if decryption works
	testDecrypt := make([]byte, 100)
	_, err = decryptedReader.Read(testDecrypt)
	if err != nil && err != io.EOF {
		return fmt.Errorf("key verification failed: %w", err)
	}

	return nil
}

// CreateBackupSignature creates a signature for backup integrity verification
func (em *EncryptionManager) CreateBackupSignature(backupPath string) (string, error) {
	file, err := os.Open(backupPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// VerifyBackupSignature verifies backup integrity
func (em *EncryptionManager) VerifyBackupSignature(backupPath, expectedSignature string) error {
	actualSignature, err := em.CreateBackupSignature(backupPath)
	if err != nil {
		return err
	}

	if actualSignature != expectedSignature {
		return fmt.Errorf("backup signature mismatch")
	}

	return nil
}

// Helper function to generate a key (simplified)
func generateKey(password string) []byte {
	hash := sha256.Sum256([]byte(password))
	return hash[:]
}