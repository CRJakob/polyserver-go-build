package tracks

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	gametrack "polyserver/game/track"
)

func LoadTrack(path string) *gametrack.Track {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read track file %s: %v", path, err)
	}

	s := strings.TrimSpace(string(data))

	t, err := gametrack.DecodePolyTrack2(s)
	if err != nil {
		log.Fatalf("Failed to decode track %s: %v", path, err)
	}

	return t
}

func LoadAllTracks(dir string) (map[string]*gametrack.Track, []string) {
	out := map[string]*gametrack.Track{}
	names := []string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Could not read tracks directory %q: %v", dir, err)
		return out, names
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		base := strings.TrimSuffix(name, filepath.Ext(name))
		path := filepath.Join(dir, name)

		t := LoadTrack(path)

		out[base] = t
		names = append(names, base)

		log.Printf("Loaded track %s", base)
	}

	return out, names
}

func LoadTracksFromTop(dir string) (map[string]map[string]*gametrack.Track, map[string][]string) {
	out := map[string]map[string]*gametrack.Track{}
	names := map[string][]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Could not read tracks directory %q: %v", dir, err)
		return out, names
	}

	for _, e := range entries {
		if e.IsDir() {
			tracksMap, trackNames := LoadAllTracks(filepath.Join(dir, e.Name()))
			out[e.Name()] = tracksMap
			names[e.Name()] = trackNames
		}
		log.Println("File found at top level directory, skipping...")
	}

	return out, names
}
