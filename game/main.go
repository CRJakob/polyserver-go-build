package game

import (
	"fmt"
	"log"

	gamepackets "polyserver/game/packets"
	"polyserver/signaling"
	webrtc_session "polyserver/webrtc"
	"sync"
	"time"
)

type GameServer struct {
	SignalingServer *signaling.WebRTCServer
	Players         []*Player
	playersLock     sync.Mutex
	Factory         gamepackets.PacketFactory
	GameSession     *GameSession
	Batcher         *CarUpdateBatcher
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

	for _, player := range server.Players {
		player.SendUnreliable(gamepackets.PingDataPacket{
			HostID:      0,
			PlayerPings: pings,
		})
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
	server.playersLock.Lock()
	defer server.playersLock.Unlock()
	for _, player := range server.Players {
		var unsentCarStates []*CarStateExtended
		// So others can't modify data while we're reading it
		player.CSLock.Lock()
		for _, p := range server.Players {
			if player != p && len(p.UnsentCarStates) > 0 {
				for _, carState := range p.UnsentCarStates {
					unsentCarStates = append(unsentCarStates, &CarStateExtended{
						ID:           p.ID,
						ResetCounter: p.ResetCounter,
						CarState:     carState,
					})
				}
			}
		}

		// I wana look into this more, i think it would be nice
		// to have it multithreaded
		go server.Batcher.SendCarUpdates(player, unsentCarStates)
		player.CSLock.Unlock()
	}
}
