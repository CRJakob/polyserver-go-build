package game

import (
	"fmt"
	"log"
	gamepackets "polyserver/game/packets"
	webrtc_session "polyserver/webrtc"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

type Player struct {
	Session                 *webrtc_session.PeerSession
	Server                  *GameServer
	IsKicked                bool
	ID                      uint32
	Mods                    []string
	IsModsVanillaCompatible bool
	Nickname                string
	CountryCode             *string
	ResetCounter            uint32
	CarStyle                *gamepackets.CarStyle
	NumberOfFrames          *uint32
	Ping                    int
	PingIdCounter           uint8
	PingPackages            []PingPackage
	PPLock                  sync.Mutex
	UnsentCarStates         []gamepackets.CarState
	CSLock                  sync.Mutex
}

type PingPackage struct {
	PingId   int
	SentTime time.Time
}

func NewPlayer(p *Player) *Player {
	p.Session.ReliableDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		p.HandleMessage(msg.Data)
	})
	p.Session.UnreliableDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		p.HandleMessage(msg.Data)
	})
	return p
}

func (player *Player) HandleMessage(data []byte) {
	packet, err := player.Server.Factory.FromBytes(data)
	if err != nil {
		log.Println("Error from packet: " + err.Error())
		return
	}
	switch packet.Type() {
	case gamepackets.Pong:
		pongPacket, _ := packet.(gamepackets.PongPacket)
		// log.Printf("Received pong: %v", +pongPacket.PingId)
		player.PPLock.Lock()
		defer player.PPLock.Unlock()
		for index, pingPacket := range player.PingPackages {
			if pingPacket.PingId == int(pongPacket.PingId) {
				player.Ping = int(time.Now().UnixMilli() - pingPacket.SentTime.UnixMilli())
				player.PingPackages = append(player.PingPackages[:index], player.PingPackages[index+1:]...)
				break
			}
		}
	case gamepackets.HostCarUpdate:
		updatePacket, _ := packet.(gamepackets.HostCarUpdatePacket)
		// log.Printf("Update packet received from %v: %v\n", updatePacket.SessionID, updatePacket)
		if updatePacket.SessionID == player.Server.GameSession.SessionID {
			player.CSLock.Lock()
			if updatePacket.ResetCounter > player.ResetCounter {
				player.ResetCounter = updatePacket.ResetCounter
				player.UnsentCarStates = make([]gamepackets.CarState, 0)
			}
			if updatePacket.ResetCounter == player.ResetCounter {
				player.UnsentCarStates = append(player.UnsentCarStates, *updatePacket.CarState)
			}
			player.CSLock.Unlock()
		}
	case gamepackets.HostRecord:
		recordPacket, _ := packet.(gamepackets.HostRecordPacket)
		if player.Server.GameSession.SessionID == recordPacket.SessionID {
			if recordPacket.NumOfFrames > uint32(5999999) {
				maxFrames := uint32(5999999)
				player.NumberOfFrames = &maxFrames
			} else {
				player.NumberOfFrames = &recordPacket.NumOfFrames
			}

			for _, p := range player.Server.Players {
				if p.ID != player.ID {
					p.SendPlayerUpdate(player)
				}
			}
		}
	case gamepackets.HostCarReset:
		resetPacket, _ := packet.(gamepackets.HostCarResetPacket)
		if resetPacket.SessionID == player.Server.GameSession.SessionID && resetPacket.ResetCounter > player.ResetCounter {
			player.ResetCounter = resetPacket.ResetCounter

			player.CSLock.Lock()
			player.UnsentCarStates = make([]gamepackets.CarState, 0)
			player.CSLock.Unlock()

			for _, p := range player.Server.Players {
				if p.ID != player.ID {
					p.Send(gamepackets.PlayerCarResetPacket{
						ID:           player.ID,
						ResetCounter: resetPacket.ResetCounter,
					})
				}
			}
		}
		log.Printf("Reset packet: %v\n", resetPacket)
	}
}

func (player *Player) Send(packet gamepackets.PlayerPacket) error {
	data, err := packet.Marshal()
	// log.Printf("Sending %s to %s", packet.Type(), player.Nickname)
	if err != nil {
		return fmt.Errorf("failed to marshal %s packet: %w", packet.Type(), err)
	}

	return player.Session.ReliableDC.Send(data)
}

func (player *Player) SendUnreliable(packet gamepackets.PlayerPacket) error {
	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal %s packet: %w", packet.Type(), err)
	}

	return player.Session.UnreliableDC.Send(data)
}

func (player *Player) SendTrack() error {
	// Send track ID
	trackId, err := player.Server.GameSession.CurrentTrack.GetTrackID()
	if err != nil {
		return fmt.Errorf("failed to get track ID: %w", err)
	}

	if err := player.Send(gamepackets.TrackIDPacket{TrackID: trackId}); err != nil {
		return fmt.Errorf("failed to send track ID: %w", err)
	}

	// Get the exported track string (base62 encoded)
	// This should be the same as n.toExportString(t) in JS
	trackString := player.Server.GameSession.CurrentTrack.ExportString

	// Send track data in chunks of 16383 bytes
	for offset := 0; offset < len(trackString); offset += 16383 {
		// Calculate chunk length (min of remaining data and chunk size)
		chunkEnd := offset + 16383
		if chunkEnd > len(trackString) {
			chunkEnd = len(trackString)
		}
		chunkLen := chunkEnd - offset

		// Create packet with 1 byte header + chunk data
		packet := make([]byte, 1+chunkLen)
		packet[0] = byte(gamepackets.TrackChunk)

		// Copy string characters directly (they're ASCII/base62)
		copy(packet[1:], trackString[offset:chunkEnd])

		// Send the raw packet
		if err := player.Session.ReliableDC.Send(packet); err != nil {
			return fmt.Errorf("failed to send chunk at offset %d: %w", offset, err)
		}
	}

	return nil
}

func (player *Player) StartNewSession() {
	player.Send(gamepackets.NewSessionPacket{
		SessionID:  player.Server.GameSession.SessionID,
		GameMode:   uint8(player.Server.GameSession.GameMode),
		MaxPlayers: uint8(player.Server.GameSession.MaxPlayers),
	})
}

func (player *Player) SendPing() {
	player.PingIdCounter++
	player.SendUnreliable(gamepackets.PingPacket{
		PingId: player.PingIdCounter,
	})
	player.PPLock.Lock()
	defer player.PPLock.Unlock()
	player.PingPackages = append(player.PingPackages, PingPackage{
		PingId:   int(player.PingIdCounter),
		SentTime: time.Now(),
	})
	if len(player.PingPackages) > 10 {
		player.PingPackages = append(player.PingPackages[:0], player.PingPackages[1:]...)
	}
}

func (player *Player) SendPlayerUpdate(p *Player) {
	err := player.Send(gamepackets.PlayerUpdatePacket{
		ID:          p.ID,
		Nickname:    p.Nickname,
		CountryCode: p.CountryCode,
		CarStyle:    p.CarStyle,
		NumFrames:   p.NumberOfFrames,
	})
	if err != nil {
		log.Println("Error sending player update: " + err.Error())
	}
}
