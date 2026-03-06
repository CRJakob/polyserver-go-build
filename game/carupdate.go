package game

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	gamepackets "polyserver/game/packets"
)

// CarUpdateBatcher handles batching and splitting of car updates
type CarUpdateBatcher struct {
	maxChunkSize int // yn - 5 from JS
	sessionID    uint32
	Server       GameServer
}

func NewCarUpdateBatcher(sessionID uint32) *CarUpdateBatcher {
	return &CarUpdateBatcher{
		maxChunkSize: 16384 - 5, // Subtract header size (1 byte type + 4 bytes sessionID)
		sessionID:    sessionID,
	}
}

// encodeCarStateExtended encodes a CarStateExtended to bytes with the 8-byte header
func encodeCarStateExtended(extended *CarStateExtended) ([]byte, error) {
	// Encode the car state first
	carStateData, err := extended.CarState.EncodeCarState()
	if err != nil {
		return nil, fmt.Errorf("failed to encode car state: %w", err)
	}

	// Create buffer: 8 bytes header + car state data
	buf := make([]byte, 8+len(carStateData))

	// Write ID (4 bytes, little-endian)
	binary.LittleEndian.PutUint32(buf[0:4], extended.ID)

	// Write ResetCounter (4 bytes, little-endian)
	binary.LittleEndian.PutUint32(buf[4:8], extended.ResetCounter)

	// Write car state data
	copy(buf[8:], carStateData)

	return buf, nil
}

// SendCarUpdates attempts to send a batch of car states, splitting if necessary
func (b *CarUpdateBatcher) SendCarUpdates(player *Player, carStates []*CarStateExtended) error {
	if len(carStates) == 0 {
		return nil
	}

	// Encode all car states to bytes with their headers
	var stateBytes [][]byte
	for _, state := range carStates {
		data, err := encodeCarStateExtended(state)
		if err != nil {
			return fmt.Errorf("failed to encode car state: %w", err)
		}
		stateBytes = append(stateBytes, data)
	}

	// Combine all car states into one byte array
	combined := combineByteSlices(stateBytes)

	// Compress with zlib settings (level 9)
	compressed, err := compressWithSettings(combined)
	if err != nil {
		return fmt.Errorf("failed to compress car states: %w", err)
	}

	// Check if compressed data fits in a single packet
	if len(compressed) <= b.maxChunkSize {
		// Send as one packet
		return b.sendSinglePacket(player, compressed)
	}

	// Too big - split and try recursively
	return b.splitAndSend(player, carStates)
}

// sendSinglePacket sends one compressed batch with header
func (b *CarUpdateBatcher) sendSinglePacket(player *Player, compressed []byte) error {
	// Create packet: [type][sessionID (4 bytes)][compressed data]
	packet := make([]byte, 5+len(compressed))

	packet[0] = byte(gamepackets.PlayerCarUpdate)
	binary.LittleEndian.PutUint32(packet[1:5], b.sessionID)
	copy(packet[5:], compressed)

	return player.Session.UnreliableDC.Send(packet)
}

// splitAndSend splits the car states and tries again recursively
func (b *CarUpdateBatcher) splitAndSend(player *Player, carStates []*CarStateExtended) error {
	if len(carStates) <= 1 {
		return fmt.Errorf("cannot split car update data further - single item still too large")
	}

	// Split in half
	mid := len(carStates) / 2
	firstHalf := carStates[:mid]
	secondHalf := carStates[mid:]

	// Recursively try to send each half
	if err := b.SendCarUpdates(player, firstHalf); err != nil {
		return fmt.Errorf("failed to send first half: %w", err)
	}

	if err := b.SendCarUpdates(player, secondHalf); err != nil {
		return fmt.Errorf("failed to send second half: %w", err)
	}

	return nil
}

// Helper functions
func combineByteSlices(slices [][]byte) []byte {
	totalLen := 0
	for _, slice := range slices {
		totalLen += len(slice)
	}

	result := make([]byte, totalLen)
	offset := 0
	for _, slice := range slices {
		copy(result[offset:], slice)
		offset += len(slice)
	}
	return result
}

func compressWithSettings(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	writer, err := zlib.NewWriterLevel(&buf, zlib.DefaultCompression)
	if err != nil {
		return nil, err
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
