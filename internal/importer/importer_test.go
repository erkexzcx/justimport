package importer_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/erkexzcx/justimport/internal/arrclient"
	"github.com/erkexzcx/justimport/internal/importer"
)

func init() {
	// Suppress log output during tests.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// mockClient is a test double for importer.ArrClient.
type mockClient struct {
	name            string
	queueRecords    []arrclient.QueueRecord
	queueErr        error
	manualImport    []arrclient.ManualImportItem
	manualImportErr error
	postCalled      int
	postErr         error
}

func (m *mockClient) Name() string { return m.name }

func (m *mockClient) GetQueue(_ context.Context) ([]arrclient.QueueRecord, error) {
	return m.queueRecords, m.queueErr
}

func (m *mockClient) GetManualImport(_ context.Context, _ string) ([]arrclient.ManualImportItem, error) {
	return m.manualImport, m.manualImportErr
}

func (m *mockClient) PostManualImport(_ context.Context, _ *arrclient.ManualImportItem) error {
	m.postCalled++
	return m.postErr
}

// recordWithManualImport builds a QueueRecord that requires manual import.
func recordWithManualImport(id int, title, downloadID string) arrclient.QueueRecord {
	return arrclient.QueueRecord{
		ID:         id,
		Title:      title,
		DownloadID: downloadID,
		StatusMessages: []arrclient.StatusMessage{
			{Title: "Manual import required", Messages: []string{}},
		},
	}
}

func movieItem(path, movieTitle string) arrclient.ManualImportItem {
	return arrclient.ManualImportItem{
		ID:         1,
		Path:       path,
		Movie:      &arrclient.MediaRef{Title: movieTitle},
		Rejections: []arrclient.Rejection{},
	}
}

// ---------------------------------------------------------------------------
// filterItems / isSampleFile (tested via importer behaviour)
// ---------------------------------------------------------------------------

func TestFilterItems_ExcludesSampleByFilename(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Some.Release", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Some.Release/Sample.mkv", "Some Movie"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, true)
	imp.Run(cancelledContext(), 0)

	// Sample file only → 0 files after filtering → SKIPPED (not imported).
	if client.postCalled != 0 {
		t.Errorf("expected no POST calls, got %d", client.postCalled)
	}
}

func TestFilterItems_ExcludesSampleInDirectory(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Some.Release", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Samples/movie.mkv", "Some Movie"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, true)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls for sample directory, got %d", client.postCalled)
	}
}

func TestFilterItems_KeepsNonSampleFile(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.1080p.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.1080p.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	// 1 non-sample file with no rejections and a matched movie → should import.
	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Zero / multiple files
// ---------------------------------------------------------------------------

func TestProcessItem_ZeroFiles(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Some.Release", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls for 0 files, got %d", client.postCalled)
	}
}

func TestProcessItem_MultipleFiles(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.Pack", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.Pack/Movie1.mkv", "Movie 1"),
			movieItem("/downloads/Movie.Pack/Movie2.mkv", "Movie 2"),
			movieItem("/downloads/Movie.Pack/Movie3.mkv", "Movie 3"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls for 3 files, got %d", client.postCalled)
	}
}

func TestProcessItem_SampleFilteredLeavesOneFile(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.1080p", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.1080p/Movie.2020.1080p.mkv", "Movie 2020"),
			movieItem("/downloads/Movie.2020.1080p/Sample.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	// After filtering out sample, exactly 1 file remains → should import.
	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call after sample filtering, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Rejections
// ---------------------------------------------------------------------------

func TestProcessItem_FileWithRejections(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.1080p.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			{
				ID:    1,
				Path:  "/downloads/Movie.2020.1080p.mkv",
				Movie: &arrclient.MediaRef{Title: "Movie 2020"},
				Rejections: []arrclient.Rejection{
					{Reason: "Quality cutoff not met", Type: "permanent"},
				},
			},
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls for rejected file, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Unmatched media
// ---------------------------------------------------------------------------

func TestProcessItem_NoMovieOrSeries(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Unknown.Release.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			{
				ID:         1,
				Path:       "/downloads/Unknown.Release.mkv",
				Movie:      nil,
				Series:     nil,
				Rejections: []arrclient.Rejection{},
			},
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls for unmatched file, got %d", client.postCalled)
	}
}

func TestProcessItem_SeriesMatch(t *testing.T) {
	client := &mockClient{
		name: "sonarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Show.S01E01.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			{
				ID:         1,
				Path:       "/downloads/Show.S01E01.mkv",
				Series:     &arrclient.MediaRef{Title: "The Show"},
				Rejections: []arrclient.Rejection{},
			},
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call for series match, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Dry run
// ---------------------------------------------------------------------------

func TestDryRun_DoesNotPost(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.1080p.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.1080p.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, true) // dryRun=true
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls in dry run mode, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Deduplication
// ---------------------------------------------------------------------------

func TestDeduplication_SameDownloadIDNotProcessedTwice(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.1080p.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.1080p.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)

	// Poll twice — second poll should not re-import.
	imp.Run(cancelledContext(), 0)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 1 {
		t.Errorf("expected exactly 1 POST call (dedup), got %d", client.postCalled)
	}
}

func TestDeduplication_DifferentDownloadIDsProcessedIndependently(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.One.mkv", "dl1"),
			recordWithManualImport(2, "Movie.Two.mkv", "dl2"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.mkv", "Movie One"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	// Both dl1 and dl2 should be processed on first poll.
	if client.postCalled != 2 {
		t.Errorf("expected 2 POST calls for 2 different downloads, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// needsManualImport detection
// ---------------------------------------------------------------------------

func TestNeedsManualImport_TitleMatch(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			{
				ID:         1,
				Title:      "Movie.2020",
				DownloadID: "dl1",
				StatusMessages: []arrclient.StatusMessage{
					{Title: "Manual import required", Messages: []string{}},
				},
			},
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call for 'manual import required' title, got %d", client.postCalled)
	}
}

func TestNeedsManualImport_MessageMatch(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			{
				ID:         1,
				Title:      "Movie.2020",
				DownloadID: "dl1",
				StatusMessages: []arrclient.StatusMessage{
					{
						Title:    "Some other title",
						Messages: []string{"release was matched to movie by id"},
					},
				},
			},
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call for message match, got %d", client.postCalled)
	}
}

func TestNeedsManualImport_MatchedToSeriesByID(t *testing.T) {
	client := &mockClient{
		name: "sonarr",
		queueRecords: []arrclient.QueueRecord{
			{
				ID:         1,
				Title:      "Show.2026.S01E01",
				DownloadID: "dl1",
				StatusMessages: []arrclient.StatusMessage{
					{
						Title:    "Some other title",
						Messages: []string{"release was matched to series by id"},
					},
				},
			},
		},
		manualImport: []arrclient.ManualImportItem{
			{
				ID:         1,
				Path:       "/downloads/Show.2026.S01E01.mkv",
				Series:     &arrclient.MediaRef{Title: "The Show"},
				Rejections: []arrclient.Rejection{},
			},
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call for 'matched to series by id' message, got %d", client.postCalled)
	}
}

func TestNeedsManualImport_NoMatchSkipsItem(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			{
				ID:         1,
				Title:      "Movie.2020",
				DownloadID: "dl1",
				StatusMessages: []arrclient.StatusMessage{
					{Title: "Downloaded", Messages: []string{"All good"}},
				},
			},
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls for non-manual-import record, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func TestPollInstance_QueueError(t *testing.T) {
	client := &mockClient{
		name:     "radarr",
		queueErr: errors.New("connection refused"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0) // should not panic
}

func TestProcessItem_ManualImportError(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
		},
		manualImportErr: errors.New("API error"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0) // should not panic

	if client.postCalled != 0 {
		t.Errorf("expected no POST calls when manual import fetch fails, got %d", client.postCalled)
	}
}

func TestProcessItem_PostError(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
		postErr: errors.New("server error"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)
	imp.Run(cancelledContext(), 0)

	if client.postCalled != 1 {
		t.Errorf("expected 1 POST attempt even when it fails, got %d", client.postCalled)
	}
}

func TestProcessItem_PostErrorNotMarkedAsSeen(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
		postErr: errors.New("server error"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)

	// First poll: POST fails.
	imp.Run(cancelledContext(), 0)
	if client.postCalled != 1 {
		t.Fatalf("expected 1 POST attempt, got %d", client.postCalled)
	}

	// Second poll: item should be retried because it was not marked as seen.
	imp.Run(cancelledContext(), 0)
	if client.postCalled != 2 {
		t.Errorf("expected 2 POST attempts (retry after failure), got %d", client.postCalled)
	}
}

func TestProcessItem_ManualImportErrorNotMarkedAsSeen(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
		},
		manualImportErr: errors.New("API error"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)

	// First poll: GetManualImport fails.
	imp.Run(cancelledContext(), 0)

	// Clear error for second poll — should be retried.
	client.manualImportErr = nil
	client.manualImport = []arrclient.ManualImportItem{
		movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
	}

	imp.Run(cancelledContext(), 0)
	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call after retry, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Reconnect / seen-map pruning
// ---------------------------------------------------------------------------

func TestSeenPruning_ItemRemovedFromQueue(t *testing.T) {
	client := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
	}

	imp := importer.New([]importer.ArrClient{client}, false)

	// First poll: item is processed and marked as seen.
	imp.Run(cancelledContext(), 0)
	if client.postCalled != 1 {
		t.Fatalf("expected 1 POST call, got %d", client.postCalled)
	}

	// Item leaves queue (e.g. import completed, or service restarted with empty queue).
	client.queueRecords = nil

	// Second poll: seen map should be pruned since item is no longer in queue.
	imp.Run(cancelledContext(), 0)

	// Item returns to queue (e.g. re-downloaded, or service restarted with item back).
	client.queueRecords = []arrclient.QueueRecord{
		recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
	}

	// Third poll: item should be re-processed because it was pruned from seen.
	imp.Run(cancelledContext(), 0)
	if client.postCalled != 2 {
		t.Errorf("expected 2 POST calls (re-process after pruning), got %d", client.postCalled)
	}
}

func TestSeenPruning_SkippedWhenClientFails(t *testing.T) {
	radarr := &mockClient{
		name: "radarr",
		queueRecords: []arrclient.QueueRecord{
			recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
		},
		manualImport: []arrclient.ManualImportItem{
			movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
		},
	}
	sonarr := &mockClient{
		name:     "sonarr",
		queueErr: errors.New("connection refused"),
	}

	imp := importer.New([]importer.ArrClient{radarr, sonarr}, false)

	// First poll: radarr succeeds, sonarr fails.
	imp.Run(cancelledContext(), 0)
	if radarr.postCalled != 1 {
		t.Fatalf("expected 1 POST call for radarr, got %d", radarr.postCalled)
	}

	// Radarr's item leaves queue, but sonarr is still failing.
	radarr.queueRecords = nil

	// Second poll: pruning should be skipped because sonarr failed.
	imp.Run(cancelledContext(), 0)

	// Radarr's item returns — should NOT be re-processed because seen was not pruned.
	radarr.queueRecords = []arrclient.QueueRecord{
		recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
	}

	imp.Run(cancelledContext(), 0)
	if radarr.postCalled != 1 {
		t.Errorf("expected 1 POST call (no pruning while sonarr down), got %d", radarr.postCalled)
	}
}

func TestReconnect_QueueErrorThenSuccess(t *testing.T) {
	client := &mockClient{
		name:     "radarr",
		queueErr: errors.New("connection refused"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)

	// First poll: connection fails.
	imp.Run(cancelledContext(), 0)

	// Fix the connection and provide queue data.
	client.queueErr = nil
	client.queueRecords = []arrclient.QueueRecord{
		recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
	}
	client.manualImport = []arrclient.ManualImportItem{
		movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
	}

	// Second poll: should reconnect and process items.
	imp.Run(cancelledContext(), 0)
	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call after reconnect, got %d", client.postCalled)
	}
}

func TestReconnect_MultipleFailuresThenRecovery(t *testing.T) {
	client := &mockClient{
		name:     "radarr",
		queueErr: errors.New("connection refused"),
	}

	imp := importer.New([]importer.ArrClient{client}, false)

	// Multiple failures should not panic or break state.
	imp.Run(cancelledContext(), 0)
	imp.Run(cancelledContext(), 0)
	imp.Run(cancelledContext(), 0)

	// Recovery.
	client.queueErr = nil
	client.queueRecords = []arrclient.QueueRecord{
		recordWithManualImport(1, "Movie.2020.mkv", "dl1"),
	}
	client.manualImport = []arrclient.ManualImportItem{
		movieItem("/downloads/Movie.2020.mkv", "Movie 2020"),
	}

	imp.Run(cancelledContext(), 0)
	if client.postCalled != 1 {
		t.Errorf("expected 1 POST call after recovery, got %d", client.postCalled)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// cancelledContext returns a context that is already cancelled.
// This causes importer.Run to perform exactly one poll and return.
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
