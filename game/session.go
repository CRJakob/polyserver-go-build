package game

import gametrack "polyserver/game/track"

type GameSession struct {
	SessionID        uint32           `json:"sessionId"`
	GameMode         GameMode         `json:"gamemode"`
	SwitchingSession bool             `json:"switchingSession"`
	CurrentTrack     *gametrack.Track `json:"currentTrack"`
	MaxPlayers       int              `json:"maxPlayers"`
	Propagated       bool             `json:"propagated"`
}

// TODO: Proper session switching
