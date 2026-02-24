package storage

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
)

const maxHashReadSize = 1 * 1024 * 1024 // 1MB

// GetFileContentHash calculates MD5 hash of file content (first 1MB)
func GetFileContentHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()

	// Read only first 1MB for efficiency
	limitedReader := io.LimitReader(file, maxHashReadSize)
	if _, err := io.Copy(hash, limitedReader); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
