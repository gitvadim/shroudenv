package bootstrap

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type ValueGenerator interface {
	Generate(schema GeneratorSchema) (string, error)
}

type RandomStringGenerator struct{}

func (g *RandomStringGenerator) Generate(schema GeneratorSchema) (string, error) {
	length := schema.Length
	if length <= 0 {
		length = 16 // default length
	}

	var charset string
	cType := strings.ToLower(strings.TrimSpace(schema.Charset))
	switch cType {
	case "", "alphanumeric":
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	case "numeric":
		charset = "0123456789"
	case "alphabet":
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	default:
		// Custom charset
		charset = schema.Charset
	}

	if len(charset) == 0 {
		return "", errors.New("charset cannot be empty for random string generator")
	}

	charLen := len(charset)
	maxVal := 256 - (256 % charLen)

	result := make([]byte, length)

	// Rejection-sampling: to ensure unbiased selection, we discard bytes >= maxVal.
	// We size the buffer to max(length*2, 64) to ensure a reasonable minimum size,
	// reducing the probability of multiple outer loop iterations even for large charsets.
	bufSize := length * 2
	if bufSize < 64 {
		bufSize = 64
	}
	buf := make([]byte, bufSize)
	resultIdx := 0

	for resultIdx < length {
		_, err := rand.Read(buf)
		if err != nil {
			return "", fmt.Errorf("failed to read secure random bytes: %w", err)
		}
		for _, b := range buf {
			if int(b) < maxVal {
				result[resultIdx] = charset[int(b)%charLen]
				resultIdx++
				if resultIdx >= length {
					break
				}
			}
		}
	}

	return string(result), nil
}

type RandomBytesGenerator struct{}

func (g *RandomBytesGenerator) Generate(schema GeneratorSchema) (string, error) {
	length := schema.Length
	if length <= 0 {
		length = 32 // default length in bytes
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to read secure random bytes: %w", err)
	}

	encoding := strings.ToLower(strings.TrimSpace(schema.Encoding))
	switch encoding {
	case "", "hex":
		return hex.EncodeToString(bytes), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(bytes), nil
	default:
		return "", fmt.Errorf("unsupported encoding %q for random bytes generator", schema.Encoding)
	}
}

type UUIDGenerator struct{}

func (g *UUIDGenerator) Generate(schema GeneratorSchema) (string, error) {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for UUID: %w", err)
	}

	// Set version 4 (bits 12-15 of time_hi_and_version to 0100)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant 10xx (bits 6-7 of clock_seq_hi_and_reserved to 10)
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:],
	), nil
}

// Generate resolves a GeneratorSchema statelessly.
func Generate(schema GeneratorSchema) (string, error) {
	var gen ValueGenerator
	gType := strings.ToLower(strings.TrimSpace(schema.Type))
	switch gType {
	case "random_string":
		gen = &RandomStringGenerator{}
	case "random_bytes":
		gen = &RandomBytesGenerator{}
	case "uuid":
		gen = &UUIDGenerator{}
	default:
		return "", fmt.Errorf("unsupported generator type %q", schema.Type)
	}
	return gen.Generate(schema)
}
