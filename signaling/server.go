package signaling

import (
	"encoding/json"
	"fmt"
	"log"
	"polyserver/config"
	webrtc_session "polyserver/webrtc"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type WebRTCServer struct {
	Conn             *websocket.Conn
	ConnLock         sync.Mutex
	CurrentInvite    string
	CurrentInviteKey *string
	InviteTimeout    time.Time
	SessionLock      sync.Mutex
	Sessions         map[string]*webrtc_session.PeerSession
	ClientCount      uint32

	OnOpen  func(joinPacket JoinInvite, session *webrtc_session.PeerSession)
	OnClose func(sessionId string)
}

func NewServer() *WebRTCServer {
	return &WebRTCServer{
		Sessions:    make(map[string]*webrtc_session.PeerSession),
		ClientCount: 1,
	}
}

func (s *WebRTCServer) Connect() error {
	if s.Conn != nil {
		s.Conn.Close()
	}

	conn, _, err := websocket.DefaultDialer.Dial(config.WebsocketUrl, nil)
	if err != nil {
		return err
	}

	s.Conn = conn
	return nil
}

func (s *WebRTCServer) RegenerateInvite(newInvite bool) error {
	if err := s.Connect(); err != nil {
		return err
	}

	go s.Start()
	key := s.CurrentInviteKey
	if newInvite {
		key = nil
	}

	return s.CreateInvite(key)
}

func (s *WebRTCServer) CreateInvite(key *string) error {
	if s.Conn == nil {
		return fmt.Errorf("not connected")
	}

	payload := map[string]interface{}{
		"version": config.PolyVersion,
		"type":    "createInvite",
		"key":     key,
	}

	data, _ := json.Marshal(payload)

	return s.Conn.WriteMessage(websocket.TextMessage, data)
}

func (s *WebRTCServer) Start() {
	for {
		_, message, err := s.Conn.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			err := s.RegenerateInvite(false)
			if err != nil {
				log.Panicln("Unable to restart ws: " + err.Error())
			}
			return
		}

		s.route(message)
	}
}

func (s *WebRTCServer) handleCreateInvite(p CreateInviteResponse) {
	s.CurrentInvite = p.InviteCode
	s.CurrentInviteKey = &p.InviteKey
	s.InviteTimeout = time.Now().Add(time.Millisecond * time.Duration(p.TimeoutMilliseconds))
	log.Println("Invite code:", s.CurrentInvite)
	log.Println("Invite key:", *s.CurrentInviteKey)
	log.Println("Will timeout at:", s.InviteTimeout.String())
}

func (s *WebRTCServer) onConnectionClosed(sessionId string) {
	s.SessionLock.Lock()
	defer s.SessionLock.Unlock()
	for k := range s.Sessions {
		if k == sessionId {
			log.Printf("Removing %v from Sessions...\n", sessionId)
			s.OnClose(sessionId)
			delete(s.Sessions, k)
			break
		}
	}
}

func (s *WebRTCServer) handleJoinInvite(p JoinInvite) {
	log.Println("User is joining:", p.Nickname)
	iceServers := make([]webrtc.ICEServer, 0)

	for _, iceServer := range p.IceServers {
		log.Println("Server:", iceServer.URLs)
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       []string{iceServer.URLs},
			Username:   iceServer.Username,
			Credential: iceServer.Credential,
		})
	}

	session, answer, err := webrtc_session.NewPeerSession(
		p.Session,
		p.Offer,
		s.OnIceCandidateServer,
		iceServers,
		s.onConnectionClosed,
	)
	if err != nil {
		log.Println("failed to create session:", err)
		return
	}

	s.SessionLock.Lock()
	s.Sessions[p.Session] = session
	s.SessionLock.Unlock()

	session.ReliableDC.OnOpen(func() {
		s.OnOpen(p, session)
	})

	log.Println("Created session:", p.Session)
	log.Println("State:", session.Peer.ConnectionState().String())
	joinPacket, _ := json.Marshal(AcceptJoinPacket{
		Type:                    "acceptJoin",
		Version:                 config.PolyVersion,
		Session:                 p.Session,
		Mods:                    config.LoadedMods,
		IsModsVanillaCompatible: config.AcceptVanillaClients,
		CliendId:                s.ClientCount,
		Answer:                  answer,
	})
	s.ClientCount++
	log.Println("Answering...")

	s.send([]byte(joinPacket))
}

func (s *WebRTCServer) handleICE(p IceCandidateResponse) {
	s.SessionLock.Lock()
	session, ok := s.Sessions[p.Session]
	s.SessionLock.Unlock()

	if !ok {
		log.Println("unknown session:", p.Session)
		return
	}

	err := session.AddICECandidate(p.Candidate)
	if err != nil {
		log.Println("failed to add ICE:", err)
	}
	log.Println("Ice:", p.Candidate)
}

func (s *WebRTCServer) OnIceCandidateServer(candidate []byte, session string) error {
	var iceCandidate IceCandidate
	err := json.Unmarshal(candidate, &iceCandidate)
	if err != nil {
		return err
	}
	icePacket, err := json.Marshal(IceCandidatePacket{
		Type:      "iceCandidate",
		Candidate: iceCandidate,
		Version:   config.PolyVersion,
		Session:   session,
	})
	if err != nil {
		return err
	}
	return s.send(icePacket)
}

func (s *WebRTCServer) send(data []byte) error {
	s.ConnLock.Lock()
	defer s.ConnLock.Unlock()
	return s.Conn.WriteMessage(1, data)
}
