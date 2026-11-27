package server

import (
	"context"
	"errors"
	"time"

	"hestia/internal/index"
)

var (
	ErrCoverGenerationRunning = errors.New("cover generation is already running")
	ErrCoverScanInProgress    = errors.New("media library scan is in progress")
)

type CoverProgress struct {
	Total     int          `json:"total"`
	Processed int          `json:"processed"`
	Generated int          `json:"generated"`
	Cached    int          `json:"cached"`
	Skipped   int          `json:"skipped"`
	Failed    int          `json:"failed"`
	Done      bool         `json:"done"`
	Issues    []CoverIssue `json:"issues,omitempty"` // New failures since the previous report.
}

type CoverIssue struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

const (
	coverSourceMissing = "sourceMissing"
	coverImageRead     = "imageRead"
	coverVideoFrame    = "videoFrame"
	coverCacheWrite    = "cacheWrite"
	coverOtherError    = "other"
)

type thumbnailFailure struct {
	reason string
	err    error
}

func (failure thumbnailFailure) Error() string { return failure.err.Error() }
func (failure thumbnailFailure) Unwrap() error { return failure.err }

func coverFailureReason(err error) string {
	var failure thumbnailFailure
	if errors.As(err, &failure) {
		return failure.reason
	}
	if errors.Is(err, errFolderDeleting) {
		return coverSourceMissing
	}
	return coverOtherError
}

// GenerateCovers processes one cover at a time as part of the caller's request.
// A canceled request or an error reporting progress stops the run.
func (s *Server) GenerateCovers(ctx context.Context, report func(CoverProgress) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.coverGeneration.TryLock() {
		return ErrCoverGenerationRunning
	}
	defer s.coverGeneration.Unlock()

	ids, ready := s.store.CoverMediaIDs()
	if !ready {
		return ErrCoverScanInProgress
	}
	progress := CoverProgress{Total: len(ids)}
	if err := report(progress); err != nil {
		return err
	}
	lastReport := time.Now()
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		var result thumbnail
		path, media, err := s.store.ResolveMedia(id)
		if err != nil {
			path = id
			err = thumbnailFailure{coverSourceMissing, err}
		} else {
			result, err = s.generateCover(ctx, path, media)
		}
		if canceled := ctx.Err(); canceled != nil {
			return canceled
		}
		switch {
		case err != nil:
			progress.Failed++
			reason := coverFailureReason(err)
			progress.Issues = append(progress.Issues, CoverIssue{Path: path, Reason: reason})
			if s.logger != nil {
				s.logger.Printf("封面未生成 (%s): %s: %v", reason, path, err)
			}
		case result.cached:
			progress.Cached++
		case len(result.data) != 0:
			progress.Generated++
		default:
			progress.Skipped++
		}
		progress.Processed++
		if progress.Processed < progress.Total && time.Since(lastReport) >= 100*time.Millisecond {
			if err := report(progress); err != nil {
				return err
			}
			progress.Issues = nil
			lastReport = time.Now()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	progress.Done = true
	return report(progress)
}

func (s *Server) generateCover(ctx context.Context, abs string, m index.Media) (thumbnail, error) {
	switch {
	case m.Kind == index.KindImage && canResizeCoverImage(m.Ext):
		return s.imageThumbnail(ctx, abs, index.CardCoverWidth, m, true)
	case m.Kind == index.KindVideo:
		return s.videoThumbnail(ctx, abs, m, true)
	default:
		return thumbnail{}, nil
	}
}
