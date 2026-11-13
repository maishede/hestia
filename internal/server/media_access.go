package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

var errMediaInUse = errors.New("文件仍在读取，请稍后重试")
var errFolderDeleting = errors.New("文件夹正在删除或改名")

type mediaRead struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type mediaAccess struct {
	mu      sync.Mutex
	reads   map[string]map[*mediaRead]struct{}
	blocked map[string]bool
}

func (s *Server) beginMediaRead(w http.ResponseWriter, r *http.Request, folderID string) (context.Context, func(), bool) {
	if r.Context().Err() != nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(r.Context())
	read := &mediaRead{cancel: cancel, done: make(chan struct{})}
	access := &s.mediaAccess
	access.mu.Lock()
	if access.blocked[folderID] {
		access.mu.Unlock()
		cancel()
		errJSON(w, http.StatusConflict, errFolderDeleting.Error())
		return nil, nil, false
	}
	if access.reads == nil {
		access.reads = make(map[string]map[*mediaRead]struct{})
	}
	if access.reads[folderID] == nil {
		access.reads[folderID] = make(map[*mediaRead]struct{})
	}
	access.reads[folderID][read] = struct{}{}
	access.mu.Unlock()
	return ctx, func() {
		cancel()
		access.mu.Lock()
		delete(access.reads[folderID], read)
		if len(access.reads[folderID]) == 0 {
			delete(access.reads, folderID)
		}
		close(read.done)
		access.mu.Unlock()
	}, true
}

func mediaReadCanceled(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if ctx.Err() == nil {
		return false
	}
	if r.Context().Err() == nil {
		errJSON(w, http.StatusConflict, errFolderDeleting.Error())
	}
	return true
}

// Deletion changes the paths of the whole subtree. Keep reads blocked until its
// filesystem changes and reindexing have finished, then release even on error.
func (s *Server) prepareFolderDeletion(ctx context.Context, folderPath string, folderIDs []string) (func(), error) {
	access := &s.mediaAccess
	access.mu.Lock()
	for _, id := range folderIDs {
		if access.blocked[id] {
			access.mu.Unlock()
			return nil, errFolderDeleting
		}
	}
	if access.blocked == nil {
		access.blocked = make(map[string]bool)
	}
	var reads []*mediaRead
	for _, id := range folderIDs {
		access.blocked[id] = true
		for read := range access.reads[id] {
			reads = append(reads, read)
		}
	}
	access.mu.Unlock()

	release := func() {
		access.mu.Lock()
		defer access.mu.Unlock()
		for _, id := range folderIDs {
			delete(access.blocked, id)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, read := range reads {
		read.cancel()
	}
	if s.tr != nil {
		affected := make(map[string]bool, len(folderIDs))
		for _, id := range folderIDs {
			affected[id] = true
		}
		matches := func(mediaID, sourcePath string) bool {
			if sourcePath != "" {
				rel, err := filepath.Rel(folderPath, sourcePath)
				if err == nil && filepath.IsLocal(rel) {
					return true
				}
			}
			media, exists := s.store.GetMedia(mediaID)
			return exists && affected[media.FolderID]
		}
		if err := s.tr.stopMatching(ctx, matches); err != nil {
			release()
			return nil, errMediaInUse
		}
	}
	for _, read := range reads {
		select {
		case <-read.done:
		case <-ctx.Done():
			release()
			return nil, errMediaInUse
		}
	}
	if ctx.Err() != nil {
		release()
		return nil, errMediaInUse
	}
	return release, nil
}
