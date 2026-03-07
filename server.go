package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"polyserver/game"
	gamepackets "polyserver/game/packets"
	gametrack "polyserver/game/track"
	"polyserver/signaling"
	"polyserver/tracks"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
)

func setupLogging() {
	file, err := os.OpenFile(
		"polyserver.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	multi := io.MultiWriter(os.Stdout, file)

	log.SetOutput(multi)

	// Optional: include date + time + file:line
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// Auto Playlist Settings
var (
	autoRotateMu       sync.Mutex
	autoRotateEnabled  bool
	autoRotateFolder   string
	autoRotateInterval int // seconds
	autoRotateNextTime time.Time
	autoRotateIndex    int
	autoRotateState    string = "Stopped"
)

func runServer() {

	tracksDir := flag.String("tracks", "tracks", "track directory")
	controlPort := flag.Int("control-port", 9090, "internal control port")

	flag.Parse()

	log.Println("Game server starting...")

	tracksMap, trackNames := tracks.LoadTracksFromTop(*tracksDir)
	if len(trackNames) == 0 {
		log.Fatal("No tracks found")
	}

	var defaultTrack *gametrack.Track
	for k := range tracksMap {
		for j := range tracksMap[k] {
			log.Printf("Default map: %s/%s\n", k, j)
			defaultTrack = tracksMap[k][j]
			break
		}
		break
	}

	server := signaling.NewServer()

	if err := server.Connect(); err != nil {
		log.Fatal(err)
	}
	go server.Start()

	gameServer := game.NewServer(server)

	gameServer.UpdateGameSession(game.GameSession{
		SessionID:        0,
		GameMode:         game.Competitive,
		SwitchingSession: false,
		CurrentTrack:     defaultTrack,
		MaxPlayers:       200,
		Propagated:       false,
	})

	if err := server.CreateInvite(nil); err != nil {
		log.Fatalf("Failed to create invite: %v", err)
	}

	log.Println("Initial invite:", server.CurrentInvite)

	// ---- CONTROL API ----

	app := fiber.New()

	app.Get("/status", func(c *fiber.Ctx) error {

		currentName := ""
		currentDir := ""
		currentSession, err := json.Marshal(game.GameSession{
			SessionID:        gameServer.GameSession.SessionID,
			GameMode:         gameServer.GameSession.GameMode,
			SwitchingSession: gameServer.GameSession.SwitchingSession,
			MaxPlayers:       gameServer.GameSession.MaxPlayers,
			Propagated:       gameServer.GameSession.Propagated,
		})
		if err != nil {
			log.Println("Error marshalling session: " + err.Error())
		}
		for dirName := range tracksMap {
			for name, t := range tracksMap[dirName] {
				if t == gameServer.GameSession.CurrentTrack {
					currentName = name
					currentDir = dirName
					break
				}
			}
		}

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		autoRotateMu.Lock()
		arEnabled := autoRotateEnabled
		arFolder := autoRotateFolder
		arInterval := autoRotateInterval
		arState := autoRotateState
		arTimeLeft := int(time.Until(autoRotateNextTime).Seconds())
		autoRotateMu.Unlock()

		return c.JSON(fiber.Map{
			"invite":     server.CurrentInvite,
			"inviteKey":  server.CurrentInviteKey,
			"timeoutIn":  (time.Second * time.Duration(server.InviteTimeout.Unix()-time.Now().Unix())).String(),
			"tracks":     trackNames,
			"current":    currentName,
			"currentDir": currentDir,
			"session":    string(currentSession),
			"stats": fiber.Map{
				"goroutines": runtime.NumGoroutine(),
				"memoryAlloc": memStats.Alloc,
				"bytesSent": atomic.LoadUint64(&gameServer.BytesSent),
				"bytesReceived": atomic.LoadUint64(&gameServer.BytesReceived),
				"tickTime": atomic.LoadInt64(&gameServer.CurrentTickTime),
			},
			"autorotate": fiber.Map{
				"enabled": arEnabled,
				"folder": arFolder,
				"interval": arInterval,
				"state": arState,
				"timeLeft": arTimeLeft,
			},
		})
	})

	app.Post("/invite", func(c *fiber.Ctx) error {

		type Req struct {
			Regenerate bool    `json:"regenerate"`
			Key        *string `json:"key"`
		}

		var req Req
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Invalid body")
		}
		var key *string
		if req.Regenerate {
			key = server.CurrentInviteKey
		} else {
			key = req.Key
		}

		if err := server.CreateInvite(key); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		return c.JSON(fiber.Map{
			"invite":    server.CurrentInvite,
			"key":       server.CurrentInviteKey,
			"timeoutIn": (time.Second * time.Duration(server.InviteTimeout.Unix()-time.Now().Unix())).String(),
		})
	})

	app.Post("/kick", func(c *fiber.Ctx) error {

		type Req struct {
			ID uint32 `json:"id"`
		}

		var req Req
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Invalid body")
		}

		for _, player := range gameServer.Players {
			if player.ID == req.ID {
				log.Println("Kicked player: ", player.Nickname)
				player.Send(gamepackets.KickPlayerPacket{})
				for _, p := range gameServer.Players {
					p.Send(gamepackets.RemovePlayerPacket{
						ID:       player.ID,
						IsKicked: true,
					})
				}
				time.AfterFunc(1*time.Second, func() {
					player.Session.Peer.Close()
				})
				break
			}
		}

		return c.SendStatus(204)
	})

	app.Post("/session/end", func(c *fiber.Ctx) error {
		if gameServer.GameSession.SwitchingSession {
			log.Println("Can't end session, already ended.")
			return c.SendStatus(400)
		}
		log.Println("Ending session...")
		gameServer.GameSession.SwitchingSession = true
		gameServer.GameSession.Propagated = false
		for _, player := range gameServer.Players {
			player.Send(gamepackets.EndSessionPacket{})
		}
		return c.SendStatus(204)
	})

	app.Post("/session/start", func(c *fiber.Ctx) error {
		if !gameServer.GameSession.SwitchingSession {
			log.Println("Can't start session, already started.")
			return c.SendStatus(400)
		}
		log.Println("Starting session...")
		gameServer.GameSession.SwitchingSession = false
		for _, player := range gameServer.Players {
			player.StartNewSession()
		}
		return c.SendStatus(204)
	})

	app.Post("/session/set", func(c *fiber.Ctx) error {

		type Req struct {
			GameMode   game.GameMode `json:"gamemode"`
			TrackDir   string        `json:"trackDir"`
			Track      string        `json:"track"`
			MaxPlayers int           `json:"maxPlayers"`
		}

		var req Req
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Invalid body")
		}
		t, ok := tracksMap[req.TrackDir][req.Track]

		if !ok {
			log.Println("Track " + req.Track + " not found.")
			return c.SendStatus(400)
		}

		gameServer.UpdateGameSession(game.GameSession{
			GameMode:         req.GameMode,
			SwitchingSession: true,
			CurrentTrack:     t,
			MaxPlayers:       req.MaxPlayers,
		})
		log.Println("Got new session data...")

		return c.SendStatus(204)
	})

	app.Post("/reloadTracks", func(c *fiber.Ctx) error {
		log.Println("Reloading tracks...")
		tracksMap, trackNames = tracks.LoadTracksFromTop(*tracksDir)
		return c.SendStatus(204)
	})

	app.Post("/autorotate/start", func(c *fiber.Ctx) error {
		type Req struct {
			Folder   string `json:"folder"`
			Interval int    `json:"interval"`
		}
		var req Req
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Invalid body")
		}

		autoRotateMu.Lock()
		autoRotateEnabled = true
		autoRotateFolder = req.Folder
		autoRotateInterval = req.Interval
		autoRotateState = "Playing"
		autoRotateNextTime = time.Now().Add(time.Duration(req.Interval) * time.Second)
		autoRotateIndex = -1 // Reset sequence to try to pick map 0 on next map transition
		autoRotateMu.Unlock()

		return c.SendStatus(204)
	})

	app.Post("/autorotate/stop", func(c *fiber.Ctx) error {
		autoRotateMu.Lock()
		autoRotateEnabled = false
		autoRotateState = "Stopped"
		autoRotateMu.Unlock()
		return c.SendStatus(204)
	})

	app.Post("/autorotate/skip", func(c *fiber.Ctx) error {
		autoRotateMu.Lock()
		if autoRotateEnabled && autoRotateState == "Playing" {
			autoRotateNextTime = time.Now() // Force trigger next tick execution
		}
		autoRotateMu.Unlock()
		return c.SendStatus(204)
	})

	app.Get("/players", func(c *fiber.Ctx) error {

		list := []fiber.Map{}
		for _, p := range gameServer.Players {

			timeStr := "-"
			if p.NumberOfFrames != nil {
				seconds := float64(*p.NumberOfFrames) / 1000.0
				timeStr = fmt.Sprintf("%.3fs", seconds)
			}

			list = append(list, fiber.Map{
				"id":   p.ID,
				"name": p.Nickname,
				"time": timeStr,
				"ping": p.Ping,
			})
		}

		return c.JSON(fiber.Map{
			"players": list,
		})
	})

	addr := "127.0.0.1:" + strconv.Itoa(*controlPort)

	go func() {
		log.Println("Control API running on", addr)
		if err := app.Listen(addr); err != nil {
			log.Println(err)
		}
	}()

	select {} // keep server alive
}
