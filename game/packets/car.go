package gamepackets

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

type CarState struct {
	Frames                   uint32
	SpeedKmh                 float32
	HasStarted               bool
	FinishFrames             *uint32
	NextCheckpointIndex      uint16
	HasCheckpointToRespawnAt bool
	Position                 Vector3
	Quaternion               Quaternion
	CollisionImpulses        []float32
	WheelContact             [4]*WheelContact
	WheelSuspensionLength    [4]float32
	WheelSuspensionVelocity  [4]float32
	WheelDeltaRotation       [4]float32
	WheelSkidInfo            [4]float32
	Steering                 float32
	BrakeLightEnabled        bool
	Controls                 Controls
}

type Vector3 struct {
	X, Y, Z float32
}

type Quaternion struct {
	X, Y, Z, W float32
}

type WheelContact struct {
	Position Vector3
	Normal   Vector3
}

type Controls struct {
	Up    bool
	Right bool
	Down  bool
	Left  bool
	Reset bool
}

func DecodeCarState(data []byte) (*CarState, int, error) {
	const errMsg = "CarState data is too short"

	offset := 0

	// Read frames (3 bytes, little-endian)
	if len(data) < offset+3 {
		return nil, 0, fmt.Errorf(errMsg)
	}

	frames := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16
	offset += 3

	// Read speed (float32, little-endian)
	if len(data) < offset+4 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	speed := math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// Read flags (1 byte)
	if len(data) < offset+1 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	flags := data[offset]
	hasStarted := flags&1 != 0

	hasCheckpointToRespawnAt := (flags & 4) != 0

	wheelContactFlags := [4]bool{
		(flags & 8) != 0,
		(flags & 16) != 0,
		(flags & 32) != 0,
		(flags & 64) != 0,
	}
	offset++

	// Read finish frames (optional, 3 bytes if present)
	var finishFrames *uint32
	if (flags & 2) != 0 { // o in JS (2 & flags)
		if len(data) < offset+3 {
			return nil, 0, fmt.Errorf(errMsg)
		}
		ff := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16
		finishFrames = &ff
		offset += 3
	}

	// Read next checkpoint index (uint16, little-endian)
	if len(data) < offset+2 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	nextCheckpointIndex := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Read position (Vector3, 3 floats, 12 bytes)
	if len(data) < offset+12 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	position := Vector3{
		X: math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4])),
		Y: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+4 : offset+8])),
		Z: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8 : offset+12])),
	}
	offset += 12

	// Read quaternion (4 floats, 16 bytes)
	if len(data) < offset+16 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	quaternion := Quaternion{
		X: math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4])),
		Y: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+4 : offset+8])),
		Z: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8 : offset+12])),
		W: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+12 : offset+16])),
	}
	offset += 16

	// Read number of collision impulses
	if len(data) < offset+1 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	numImpulses := data[offset]
	offset++

	if numImpulses > 4 {
		return nil, 0, fmt.Errorf("number of collision impulses exceeds maximum allowed: %d", numImpulses)
	}

	// Read collision impulses
	collisionImpulses := make([]float32, numImpulses)
	for i := 0; i < int(numImpulses); i++ {
		if len(data) < offset+4 {
			return nil, 0, fmt.Errorf(errMsg)
		}
		collisionImpulses[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	// Read wheel contacts (optional per wheel)
	wheelContact := [4]*WheelContact{nil, nil, nil, nil}
	for i := 0; i < 4; i++ {
		if wheelContactFlags[i] {
			if len(data) < offset+24 { // 2 vectors * 12 bytes
				return nil, 0, fmt.Errorf(errMsg)
			}

			position := Vector3{
				X: math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4])),
				Y: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+4 : offset+8])),
				Z: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8 : offset+12])),
			}
			offset += 12

			normal := Vector3{
				X: math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4])),
				Y: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+4 : offset+8])),
				Z: math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8 : offset+12])),
			}
			offset += 12

			wheelContact[i] = &WheelContact{
				Position: position,
				Normal:   normal,
			}
		}
	}

	// Read wheel suspension lengths (4 floats)
	var wheelSuspensionLength [4]float32
	for i := 0; i < 4; i++ {
		if len(data) < offset+4 {
			return nil, 0, fmt.Errorf(errMsg)
		}
		wheelSuspensionLength[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	// Read wheel suspension velocities (4 floats)
	var wheelSuspensionVelocity [4]float32
	for i := 0; i < 4; i++ {
		if len(data) < offset+4 {
			return nil, 0, fmt.Errorf(errMsg)
		}
		wheelSuspensionVelocity[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	// Read wheel delta rotations (4 floats)
	var wheelDeltaRotation [4]float32
	for i := 0; i < 4; i++ {
		if len(data) < offset+4 {
			return nil, 0, fmt.Errorf(errMsg)
		}
		wheelDeltaRotation[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	// Read wheel skid info (4 floats)
	var wheelSkidInfo [4]float32
	for i := 0; i < 4; i++ {
		if len(data) < offset+4 {
			return nil, 0, fmt.Errorf(errMsg)
		}
		wheelSkidInfo[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	// Read steering
	if len(data) < offset+4 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	steering := math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// Read final flags
	if len(data) < offset+1 {
		return nil, 0, fmt.Errorf(errMsg)
	}
	finalFlags := data[offset]
	controls := Controls{
		Up:    finalFlags&1 != 0,
		Right: finalFlags&2 != 0,
		Down:  finalFlags&4 != 0,
		Left:  finalFlags&8 != 0,
		Reset: finalFlags&16 != 0,
	}
	brakeLightEnabled := finalFlags&32 != 0
	offset++

	carState := &CarState{
		Frames:                   frames,
		SpeedKmh:                 speed,
		HasStarted:               hasStarted,
		FinishFrames:             finishFrames,
		NextCheckpointIndex:      nextCheckpointIndex,
		HasCheckpointToRespawnAt: hasCheckpointToRespawnAt,
		Position:                 position,
		Quaternion:               quaternion,
		CollisionImpulses:        collisionImpulses,
		WheelContact:             wheelContact,
		WheelSuspensionLength:    wheelSuspensionLength,
		WheelSuspensionVelocity:  wheelSuspensionVelocity,
		WheelDeltaRotation:       wheelDeltaRotation,
		WheelSkidInfo:            wheelSkidInfo,
		Steering:                 steering,
		BrakeLightEnabled:        brakeLightEnabled,
		Controls:                 controls,
	}

	return carState, offset, nil
}

// EncodeCarState encodes a CarState to bytes
func (cs *CarState) EncodeCarState() ([]byte, error) {
	// Calculate total size
	size := cs.calculateSize()
	buf := make([]byte, size)
	offset := 0

	// Write frames (3 bytes, little-endian)
	binary.LittleEndian.PutUint32(buf[offset:offset+4], cs.Frames)
	offset += 3

	// Write speedKmh (float32, 4 bytes)
	binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(cs.SpeedKmh))
	offset += 4

	// Write flags byte
	flags := byte(0)
	if cs.HasStarted {
		flags |= 1 << 0
	}
	if cs.FinishFrames != nil {
		flags |= 1 << 1
	}
	if cs.HasCheckpointToRespawnAt {
		flags |= 1 << 2
	}
	for i := 0; i < 4; i++ {
		if i < len(cs.WheelContact) && cs.WheelContact[i] != nil {
			flags |= 1 << (3 + i)
		}
	}
	buf[offset] = flags
	offset++

	// Write finishFrames (optional, 3 bytes)
	if cs.FinishFrames != nil {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], *cs.FinishFrames)
		offset += 3
	}

	// Write nextCheckpointIndex (uint16, 2 bytes)
	binary.LittleEndian.PutUint16(buf[offset:offset+2], cs.NextCheckpointIndex)
	offset += 2

	// Write position (3 floats, 12 bytes)
	binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(cs.Position.X))
	binary.LittleEndian.PutUint32(buf[offset+4:offset+8], math.Float32bits(cs.Position.Y))
	binary.LittleEndian.PutUint32(buf[offset+8:offset+12], math.Float32bits(cs.Position.Z))
	offset += 12

	// Write quaternion (4 floats, 16 bytes)
	binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(cs.Quaternion.X))
	binary.LittleEndian.PutUint32(buf[offset+4:offset+8], math.Float32bits(cs.Quaternion.Y))
	binary.LittleEndian.PutUint32(buf[offset+8:offset+12], math.Float32bits(cs.Quaternion.Z))
	binary.LittleEndian.PutUint32(buf[offset+12:offset+16], math.Float32bits(cs.Quaternion.W))
	offset += 16

	// Write number of collision impulses (1 byte)
	if len(cs.CollisionImpulses) > 255 {
		return nil, fmt.Errorf("too many collision impulses: %d", len(cs.CollisionImpulses))
	}
	buf[offset] = byte(len(cs.CollisionImpulses))
	offset++

	// Write collision impulses (4 bytes each)
	for i, impulse := range cs.CollisionImpulses {
		binary.LittleEndian.PutUint32(buf[offset+i*4:offset+(i+1)*4], math.Float32bits(impulse))
	}
	offset += 4 * len(cs.CollisionImpulses)

	// Write wheel contacts (optional, 24 bytes each)
	for i := 0; i < 4; i++ {
		if i < len(cs.WheelContact) && cs.WheelContact[i] != nil {
			wc := cs.WheelContact[i]

			// Write position
			binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(wc.Position.X))
			binary.LittleEndian.PutUint32(buf[offset+4:offset+8], math.Float32bits(wc.Position.Y))
			binary.LittleEndian.PutUint32(buf[offset+8:offset+12], math.Float32bits(wc.Position.Z))
			offset += 12

			// Write normal
			binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(wc.Normal.X))
			binary.LittleEndian.PutUint32(buf[offset+4:offset+8], math.Float32bits(wc.Normal.Y))
			binary.LittleEndian.PutUint32(buf[offset+8:offset+12], math.Float32bits(wc.Normal.Z))
			offset += 12
		}
	}

	// Write wheelSuspensionLength (4 floats, 16 bytes)
	for i, val := range cs.WheelSuspensionLength {
		binary.LittleEndian.PutUint32(buf[offset+i*4:offset+(i+1)*4], math.Float32bits(val))
	}
	offset += 16

	// Write wheelSuspensionVelocity (4 floats, 16 bytes)
	for i, val := range cs.WheelSuspensionVelocity {
		binary.LittleEndian.PutUint32(buf[offset+i*4:offset+(i+1)*4], math.Float32bits(val))
	}
	offset += 16

	// Write wheelDeltaRotation (4 floats, 16 bytes)
	for i, val := range cs.WheelDeltaRotation {
		binary.LittleEndian.PutUint32(buf[offset+i*4:offset+(i+1)*4], math.Float32bits(val))
	}
	offset += 16

	// Write wheelSkidInfo (4 floats, 16 bytes)
	for i, val := range cs.WheelSkidInfo {
		binary.LittleEndian.PutUint32(buf[offset+i*4:offset+(i+1)*4], math.Float32bits(val))
	}
	offset += 16

	// Write steering (float32, 4 bytes)
	binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(cs.Steering))
	offset += 4

	// Write final flags byte
	finalFlags := byte(0)
	if cs.Controls.Up {
		finalFlags |= 1 << 0
	}
	if cs.Controls.Right {
		finalFlags |= 1 << 1
	}
	if cs.Controls.Down {
		finalFlags |= 1 << 2
	}
	if cs.Controls.Left {
		finalFlags |= 1 << 3
	}
	if cs.Controls.Reset {
		finalFlags |= 1 << 4
	}
	if cs.BrakeLightEnabled {
		finalFlags |= 1 << 5
	}
	buf[offset] = finalFlags
	offset++

	return buf[:offset], nil
}

// calculateSize pre-calculates the required buffer size
func (cs *CarState) calculateSize() int {
	size := 0

	// Base fixed fields
	size += 3 // frames (3 bytes)
	size += 4 // speedKmh
	size += 1 // flags byte

	// Optional finishFrames
	if cs.FinishFrames != nil {
		size += 3
	}

	// More fixed fields
	size += 2  // nextCheckpointIndex
	size += 12 // position
	size += 16 // quaternion

	// Collision impulses
	size += 1                             // count byte
	size += 4 * len(cs.CollisionImpulses) // impulses

	// Wheel contacts (optional)
	for i := 0; i < 4; i++ {
		if i < len(cs.WheelContact) && cs.WheelContact[i] != nil {
			size += 24 // 2 vectors * 12 bytes
		}
	}

	// Wheel arrays (always 4 floats each)
	size += 16 // wheelSuspensionLength
	size += 16 // wheelSuspensionVelocity
	size += 16 // wheelDeltaRotation
	size += 16 // wheelSkidInfo

	// Final fields
	size += 4 // steering
	size += 1 // final flags

	return size
}

// HostCarUpdatePacket represents a car state update for a specific session
type HostCarUpdatePacket struct {
	SessionID    uint32
	ResetCounter uint32
	CarState     *CarState
}

// Constants for CarUpdate packet
const (
	CarUpdateHeaderSize = 8 // 4 bytes sessionID + 4 bytes resetCounter
)

func (p HostCarUpdatePacket) Type() HostPacketType {
	return HostCarUpdate
}

func (p HostCarUpdatePacket) Marshal() ([]byte, error) {
	if p.CarState == nil {
		return nil, fmt.Errorf("car state cannot be nil")
	}

	// Encode the car state first to know its size
	carStateData, err := p.CarState.EncodeCarState()
	if err != nil {
		return nil, fmt.Errorf("failed to encode car state: %w", err)
	}

	// Total size: 1 (type) + header (8) + car state data
	buf := make([]byte, 1+CarUpdateHeaderSize+len(carStateData))

	// Write packet type
	buf[0] = byte(HostCarUpdate)

	// Write session ID (uint32, little-endian)
	binary.LittleEndian.PutUint32(buf[1:5], p.SessionID)

	// Write reset counter (uint32, little-endian)
	binary.LittleEndian.PutUint32(buf[5:9], p.ResetCounter)

	// Write car state data
	copy(buf[9:], carStateData)

	return buf, nil
}

type CarStyle struct {
	Pattern uint8 // this.pattern (byte at index 1)
	Rims    uint8 // this.rims (byte at index 2)
	Exhaust uint8 // this.exhaust (byte at index 3)

	// These are 3-byte values (24-bit) stored at specific offsets
	Color1 uint32 // r in JS - stored at offset 4-6 (3 bytes)
	Color2 uint32 // a in JS - stored at offset 7-9 (3 bytes)
	Color3 uint32 // s in JS - stored at offset 10-12 (3 bytes)
	Color4 uint32 // o in JS - stored at offset 13-15 (3 bytes)
}

func (cs *CarStyle) EncodeCarStyle() []byte {
	buf := make([]byte, 16)

	buf[0] = 0 // Header
	buf[1] = cs.Pattern
	buf[2] = cs.Rims
	buf[3] = cs.Exhaust

	// Color1 (3 bytes, little-endian)
	buf[4] = byte(cs.Color1)
	buf[5] = byte(cs.Color1 >> 8)
	buf[6] = byte(cs.Color1 >> 16)

	// Color2 (3 bytes)
	buf[7] = byte(cs.Color2)
	buf[8] = byte(cs.Color2 >> 8)
	buf[9] = byte(cs.Color2 >> 16)

	// Color3 (3 bytes)
	buf[10] = byte(cs.Color3)
	buf[11] = byte(cs.Color3 >> 8)
	buf[12] = byte(cs.Color3 >> 16)

	// Color4 (3 bytes)
	buf[13] = byte(cs.Color4)
	buf[14] = byte(cs.Color4 >> 8)
	buf[15] = byte(cs.Color4 >> 16)

	return buf
}

func DeserializeCarStyle(data []byte) (*CarStyle, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("car style data too short: %d bytes", len(data))
	}

	if data[0] != 0 {
		return nil, fmt.Errorf("invalid car style header: %d", data[0])
	}

	return &CarStyle{
		Pattern: data[1],
		Rims:    data[2],
		Exhaust: data[3],
		Color1:  uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16,
		Color2:  uint32(data[7]) | uint32(data[8])<<8 | uint32(data[9])<<16,
		Color3:  uint32(data[10]) | uint32(data[11])<<8 | uint32(data[12])<<16,
		Color4:  uint32(data[13]) | uint32(data[14])<<8 | uint32(data[15])<<16,
	}, nil
}

func FromBase64String(encoded string) (*CarStyle, error) {
	if encoded == "" {
		return DefaultCarStyle(), nil
	}

	// Try with URL encoding (no padding)
	binaryData, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(encoded)
	if err != nil {
		// Try with URL encoding and auto-padding
		binaryData, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 URL: %w", err)
		}
	}
	return DeserializeCarStyle(binaryData)
}

func (cs *CarStyle) ToBase64String() string {
	binaryData := cs.EncodeCarStyle()
	// Use URL-safe encoding with - and _ (matches JS)
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(binaryData)
}

func DefaultCarStyle() *CarStyle {
	return &CarStyle{
		Pattern: 0,
		Rims:    0,
		Exhaust: 0,
		Color1:  5592405, // 0x555555
		Color2:  5592405,
		Color3:  5592405,
		Color4:  5592405,
	}
}
