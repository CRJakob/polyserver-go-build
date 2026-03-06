package gametrack

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

const base62Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var decodeValues = [123]int{
	-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
	-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
	52, 53, 54, 55, 56, 57, 58, 59, 60, 61, -1, -1, -1, -1, -1, -1, -1, 0, 1, 2, 3, 4, 5, 6, 7, 8,
	9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, -1, -1, -1, -1, -1, -1, 26,
	27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
	51,
}

// DecodeBase62 decodes a Base62 string into bytes using the Polytrack algorithm
func DecodeBase62(input string) ([]byte, error) {
	outPos := 0
	bytesOut := make([]byte, 0)

	for i, ch := range input {
		if int(ch) >= len(decodeValues) {
			return nil, fmt.Errorf("invalid Base62 char at position %d: %c (code %d)", i, ch, ch)
		}
		charValue := decodeValues[ch]
		if charValue == -1 {
			return nil, fmt.Errorf("invalid Base62 char at position %d: %c (code %d)", i, ch, ch)
		}

		// 5 if charValue is 30 or 31, 6 otherwise
		valueLen := 6
		if (charValue & 30) == 30 { // Check if bits 1-4 are all 1 (30 = 0b11110)
			valueLen = 5
		}

		// Make sure we have enough space
		byteIndex := outPos / 8
		for byteIndex >= len(bytesOut) {
			bytesOut = append(bytesOut, 0)
		}

		decodeChars(&bytesOut, outPos, valueLen, byte(charValue), i == len(input)-1)
		outPos += valueLen
	}

	return bytesOut, nil
}

// ZlibDecompressToString decompresses zlib data and returns it as a UTF-8 string
func ZlibDecompressToString(data []byte) (string, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	return string(decompressed), nil
}

func decodeChars(bytes *[]byte, bitIndex int, valueLen int, charValue byte, isLast bool) {
	byteIndex := bitIndex / 8
	offset := bitIndex - 8*byteIndex

	// Write the portion that fits in the current byte
	// In Rust: ((char_value << offset) & 0xFF) as u8
	(*bytes)[byteIndex] |= (charValue << offset) & 0xFF

	// If the value spans to the next byte and this isn't the last character
	if offset > 8-valueLen && !isLast {
		// Ensure next byte exists
		for byteIndex+1 >= len(*bytes) {
			*bytes = append(*bytes, 0)
		}

		// In Rust: (char_value >> (8 - offset)) as u8
		// Note: In Rust, char_value is i32 but they cast to u8 after shift
		(*bytes)[byteIndex+1] |= charValue >> (8 - offset)
	}
}

// ZlibDecompress decompresses zlib-compressed data
func ZlibDecompress(data []byte) ([]byte, error) {

	// Try zlib first
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		fmt.Printf("zlib.NewReader error: %v\n", err)
		return nil, fmt.Errorf("zlib error: %w", err)
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decompression error: %w", err)
	}

	return decompressed, nil
}

// Helper function to calculate optimal byte size for a range
func calculateByteSize(rangeVal int32) int {
	if rangeVal <= 0 {
		return 1
	}
	// ceil(log2(rangeVal+1) / 8)
	bits := 0
	for rangeVal > 0 {
		bits++
		rangeVal >>= 1
	}
	bytes := (bits + 7) / 8
	if bytes < 1 {
		bytes = 1
	}
	if bytes > 4 {
		bytes = 4
	}
	return bytes
}

// Helper to write int32 as little-endian
func writeInt32(buf *bytes.Buffer, val int32) {
	v := uint32(val)
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 24))
}

// Helper to write uint32 as little-endian
func writeUint32(buf *bytes.Buffer, val uint32) {
	buf.WriteByte(byte(val))
	buf.WriteByte(byte(val >> 8))
	buf.WriteByte(byte(val >> 16))
	buf.WriteByte(byte(val >> 24))
}

// Helper to write uint16 as little-endian
func writeUint16(buf *bytes.Buffer, val uint16) {
	buf.WriteByte(byte(val))
	buf.WriteByte(byte(val >> 8))
}

// Helper to write an int with a specific number of bytes (little-endian)
func writeIntWithBytes(buf *bytes.Buffer, val int32, bytes int) {
	v := uint32(val)
	switch bytes {
	case 1:
		buf.WriteByte(byte(v))
	case 2:
		buf.WriteByte(byte(v))
		buf.WriteByte(byte(v >> 8))
	case 3:
		buf.WriteByte(byte(v))
		buf.WriteByte(byte(v >> 8))
		buf.WriteByte(byte(v >> 16))
	case 4:
		buf.WriteByte(byte(v))
		buf.WriteByte(byte(v >> 8))
		buf.WriteByte(byte(v >> 16))
		buf.WriteByte(byte(v >> 24))
	default:
		panic(fmt.Sprintf("invalid byte count: %d", bytes))
	}
}
