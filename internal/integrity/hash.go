package integrity

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func FileMatchesManifestHash(filePath, manifestHash string) (bool, error) {
	if manifestHash == "" {
		return false, nil
	}
	computed, err := NormalizedFileMD5(filePath)
	if err != nil {
		return false, err
	}
	return computed == manifestHash, nil
}

func NormalizedFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := md5.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if len(sum) < 2 {
		return "", fmt.Errorf("unexpected md5 length for %s", filePath)
	}
	return sum[:len(sum)-2] + "00", nil
}
