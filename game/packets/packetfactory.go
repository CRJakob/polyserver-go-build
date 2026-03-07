package gamepackets

import (
	"encoding/binary"
	"fmt"
	"log"
)

// PacketFactory helps create packets from raw data (for receiving)
type PacketFactory struct{}

func (f *PacketFactory) FromBytes(data []byte) (HostPacket, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty packet")
	}

	packetType := HostPacketType(data[0])

	switch packetType {
	case Pong:
		return PongPacket{
			PingId: data[1],
		}, nil
	case HostCarUpdate:
		carState, _, err := DecodeCarState(data[9:])
		if err != nil {
			log.Println("Error decoding car state: " + err.Error())
			return nil, err
		}
		return HostCarUpdatePacket{
			SessionID:    binary.LittleEndian.Uint32(data[1:5]),
			ResetCounter: binary.LittleEndian.Uint32(data[5:9]),
			CarState:     carState,
		}, nil
	case HostRecord:
		frameCount := make([]byte, 4)
		frameCount[0] = data[5]
		frameCount[1] = data[6]
		frameCount[2] = data[7]
		frameCount[3] = uint8(0)
		return HostRecordPacket{
			SessionID:   binary.LittleEndian.Uint32(data[1:5]),
			NumOfFrames: binary.LittleEndian.Uint32(frameCount),
		}, nil
	case HostCarReset:
		return HostCarResetPacket{
			SessionID:    binary.LittleEndian.Uint32(data[1:5]),
			ResetCounter: binary.LittleEndian.Uint32(data[5:9]),
		}, nil
	default:
		return nil, fmt.Errorf("unknown packet type: %s", packetType.String())
	}
}
