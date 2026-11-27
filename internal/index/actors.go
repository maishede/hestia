package index

import (
	"cmp"
	"slices"
	"strings"
)

type ActorSummary struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func actorFromName(name string) string {
	prefix, title, found := strings.Cut(name, "-")
	if !found || strings.TrimSpace(title) == "" {
		return ""
	}
	return strings.TrimSpace(prefix)
}

func (s *Store) Actors() []ActorSummary {
	s.mu.RLock()
	actors := make([]ActorSummary, 0, len(s.actorVideos))
	for name, ids := range s.actorVideos {
		actors = append(actors, ActorSummary{Name: name, Count: len(ids)})
	}
	s.mu.RUnlock()
	sortActors(actors)
	return actors
}

func sortActors(actors []ActorSummary) {
	slices.SortFunc(actors, func(a, b ActorSummary) int {
		return compareSort(cmp.Compare(a.Count, b.Count), "desc", a.Name, b.Name, a.Name, b.Name)
	})
}

func (s *Store) ActorVideos(actor string, page, size int, sortKey, order string) MediaPage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mediaPageLocked(s.actorVideos[actor], page, size, sortKey, order)
}
