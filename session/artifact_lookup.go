package session

import "github.com/tstapler/stapler-squad/session/artifacts"

// FindInstanceByHistoryPath returns the title of the session whose JSONL
// history file matches filePath. Returns ("", false) if not found.
// HistoryFilePath is a public field set by HistoryLinker; safe to read here
// since this runs on each HistoryLinker callback, which is the same goroutine
// that sets the field.
func FindInstanceByHistoryPath(instances []*Instance, filePath string) (string, bool) {
	for _, inst := range instances {
		if inst.HistoryFilePath == filePath {
			return inst.Title, true
		}
	}
	return "", false
}

// InstanceInfoSlice converts a slice of live Instances to the lightweight
// InstanceInfo type used by ArtifactExtractor.SeedOffsets.
func InstanceInfoSlice(instances []*Instance) []artifacts.InstanceInfo {
	out := make([]artifacts.InstanceInfo, 0, len(instances))
	for _, inst := range instances {
		out = append(out, artifacts.InstanceInfo{
			Title:           inst.Title,
			HistoryFilePath: inst.HistoryFilePath,
		})
	}
	return out
}
