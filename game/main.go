package game

import (
	"fmt"
	"log"

	gamepackets "polyserver/game/packets"
	"polyserver/signaling"
	webrtc_session "polyserver/webrtc"
	"sync"
	"sync/atomic"
	"time"
)

type GameServer struct {
	SignalingServer *signaling.WebRTCServer
	Players         []*Player
	playersLock     sync.Mutex
	Factory         gamepackets.PacketFactory
	GameSession     *GameSession
	Batcher         *CarUpdateBatcher

	// Profiling Stats
	BytesSent       uint64
	BytesReceived   uint64
	CurrentTickTime int64
}

type GameMode uint8

const (
	Casual GameMode = iota
	Competitive
)

func (gm GameMode) String() string {
	switch gm {
	case Casual:
		return "Casual"
	case Competitive:
		return "Competitive"
	default:
		return fmt.Sprintf("Unknown(%d)", gm)
	}
}

func NewServer(signalingServer *signaling.WebRTCServer) *GameServer {
	server := &GameServer{
		SignalingServer: signalingServer,
		Players:         make([]*Player, 0),
		Factory:         gamepackets.PacketFactory{},
		GameSession:     &GameSession{},
	}

	signalingServer.OnOpen = server.onPlayerJoin
	signalingServer.OnClose = server.onPlayerDisconnect

	schedule(server.sendPings, time.Second)

	server.Batcher = NewCarUpdateBatcher(server.GameSession.SessionID)

	schedule(server.UpdateCarStates, 100*time.Millisecond)

	return server
}

func (s *GameServer) UpdateGameSession(gs GameSession) {
	s.GameSession.SessionID++
	s.GameSession.GameMode = gs.GameMode
	s.GameSession.SwitchingSession = gs.SwitchingSession
	s.GameSession.CurrentTrack = gs.CurrentTrack
	s.GameSession.MaxPlayers = gs.MaxPlayers
	s.Batcher.sessionID = s.GameSession.SessionID
	if s.GameSession.Propagated {
		// Don't let low IQ individuals shoot themselves in the foot
		log.Println("Cannot send map twice in one switch!")
		return
	}
	for _, player := range s.Players {
		player.SendTrack()
	}
	s.GameSession.Propagated = true
}

//
// PLAYER JOIN
//

func (server *GameServer) onPlayerJoin(p signaling.JoinInvite, session *webrtc_session.PeerSession) {

	log.Println("Creating player " + p.Nickname)

	carStyle, err := gamepackets.FromBase64String(p.CarStyle)
	if err != nil {
		carStyle = gamepackets.DefaultCarStyle()
		log.Println("Failed fromBase64String:", err)
	}

	newPlayer := NewPlayer(&Player{
		Server:                  server,
		Session:                 session,
		IsKicked:                false,
		ID:                      server.SignalingServer.ClientCount - 1,
		Mods:                    p.Mods,
		IsModsVanillaCompatible: p.IsModsVanillaCompatible,
		Nickname:                p.Nickname,
		CountryCode:             p.CountryCode,
		ResetCounter:            0,
		CarStyle:                carStyle,
		NumberOfFrames:          nil,
		Ping:                    0,
		PingIdCounter:           0,
		PingPackages:            make([]PingPackage, 0),
		UnsentCarStates:         make([]gamepackets.CarState, 0),
	})

	newPlayer.Send(gamepackets.EndSessionPacket{})
	newPlayer.SendTrack()
	newPlayer.StartNewSession()
	if server.GameSession.SwitchingSession {
		newPlayer.Send(gamepackets.EndSessionPacket{})
	}

	// Send existing players to the new player
	server.playersLock.Lock()
	for _, player := range server.Players {
		newPlayer.SendPlayerUpdate(player)
	}
	server.playersLock.Unlock()

	server.propagateUpdate(newPlayer)

	server.playersLock.Lock()
	server.Players = append(server.Players, newPlayer)
	server.playersLock.Unlock()
}

//
// PLAYER DISCONNECT
//

func (server *GameServer) onPlayerDisconnect(sessionId string) {
	server.playersLock.Lock()
	defer server.playersLock.Unlock()

	var playerId uint32
	index := -1

	for i, player := range server.Players {
		if player.Session.SessionID == sessionId {
			log.Println("Removing player " + player.Nickname)
			playerId = player.ID
			index = i
			break
		}
	}

	if index >= 0 {
		server.Players = append(server.Players[:index], server.Players[index+1:]...)
	}

	for _, player := range server.Players {
		if player.ID == playerId {
			continue
		}
		player.Send(gamepackets.RemovePlayerPacket{
			ID:       playerId,
			IsKicked: false,
		})
	}
}

//
// SCHEDULER
//

func schedule(f func(), interval time.Duration) *time.Ticker {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			f()
		}
	}()
	return ticker
}

//
// PING SYSTEM
//

func (server *GameServer) sendPings() {
	server.playersLock.Lock()
	for _, player := range server.Players {
		player.SendPing()
	}
	server.playersLock.Unlock()

	server.sendPingDatas()
}

func (server *GameServer) sendPingDatas() {
	pings := server.getPlayerPings()

	packet := gamepackets.PingDataPacket{
		HostID:      0,
		PlayerPings: pings,
	}

	data, err := packet.Marshal()
	if err != nil {
		log.Println("Error marshaling ping data:", err)
		return
	}

	server.playersLock.Lock()
	defer server.playersLock.Unlock()
	for _, player := range server.Players {
		player.SendRawUnreliable(data)
	}
}

func (server *GameServer) getPlayerPings() []gamepackets.PlayerPing {

	pings := make([]gamepackets.PlayerPing, 0, len(server.Players))

	for _, player := range server.Players {
		pings = append(pings, gamepackets.PlayerPing{
			PlayerID: player.ID,
			Ping:     uint16(player.Ping),
		})
	}

	return pings
}

//
// PLAYER UPDATE PROPAGATION
//

func (server *GameServer) propagateUpdate(p *Player) {
	server.playersLock.Lock()
	defer server.playersLock.Unlock()
	for _, player := range server.Players {
		log.Printf("Sending player %s to %s", p.Nickname, player.Nickname)
		player.SendPlayerUpdate(p)
	}
}

//
// CAR STATE DISTRIBUTION
//

type CarStateExtended struct {
	ID           uint32
	ResetCounter uint32
	CarState     gamepackets.CarState
}

func (server *GameServer) UpdateCarStates() {
	startTime := time.Now()

	// First gather all updates while holding playersLock to get a snapshot of players
	server.playersLock.Lock()
	playersCopy := make([]*Player, len(server.Players))
	copy(playersCopy, server.Players)
	server.playersLock.Unlock()

	// For each player, collect their unsent car states, and clear them to prevent memory leak
	playerStates := make(map[uint32][][]byte)
	for _, p := range playersCopy {
		p.CSLock.Lock()
		if len(p.UnsentCarStates) > 0 {
			var states [][]byte
			for _, carState := range p.UnsentCarStates {
				encoded, err := encodeCarStateExtended(&CarStateExtended{
					ID:           p.ID,
					ResetCounter: p.ResetCounter,
					CarState:     carState,
				})
				if err == nil {
					states = append(states, encoded)
				}
			}
			playerStates[p.ID] = states
			p.UnsentCarStates = p.UnsentCarStates[:0] // Clear slice to stop infinite growth
		}
		p.CSLock.Unlock()
	}

	// Now distribute states to everyone else
	var wg sync.WaitGroup
	for _, player := range playersCopy {
		var unsentCarStates [][]byte
		for playerID, states := range playerStates {
			if playerID != player.ID {
				unsentCarStates = append(unsentCarStates, states...)
			}
		}

		wg.Add(1)
		go func(p *Player, states [][]byte) {
			defer wg.Done()
			server.Batcher.SendCarUpdates(p, states)
		}(player, unsentCarStates)
	}
	wg.Wait()
	atomic.StoreInt64(&server.CurrentTickTime, time.Since(startTime).Microseconds())
}
