package arrclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/erkexzcx/justimport/internal/arrclient"
)

func TestCheckConnectivity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/system/status" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"appName": "Radarr",
			"version": "5.0.0",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	appName, version, err := client.CheckConnectivity(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appName != "Radarr" {
		t.Errorf("expected appName Radarr, got %s", appName)
	}
	if version != "5.0.0" {
		t.Errorf("expected version 5.0.0, got %s", version)
	}
}

func TestCheckConnectivity_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	_, _, err := client.CheckConnectivity(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := arrclient.QueueResponse{
			TotalRecords: 2,
			Records: []arrclient.QueueRecord{
				{
					ID:         1,
					Title:      "Movie.2020.1080p.mkv",
					DownloadID: "abc123",
					StatusMessages: []arrclient.StatusMessage{
						{
							Title:    "Manual import required",
							Messages: []string{"Release was matched to movie by ID."},
						},
					},
				},
				{
					ID:         2,
					Title:      "Another.Movie.2021.mkv",
					DownloadID: "def456",
					StatusMessages: []arrclient.StatusMessage{
						{Title: "Downloaded", Messages: nil},
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	records, err := client.GetQueue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].DownloadID != "abc123" {
		t.Errorf("expected downloadId abc123, got %s", records[0].DownloadID)
	}
	if records[0].StatusMessages[0].Title != "Manual import required" {
		t.Errorf("unexpected status message title: %s", records[0].StatusMessages[0].Title)
	}
}

func TestGetManualImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("downloadId") != "abc123" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		items := []arrclient.ManualImportItem{
			{
				ID:   1,
				Path: "/downloads/Movie.2020.1080p.mkv",
				Name: "Movie.2020.1080p.mkv",
				Size: 8000000000,
				Movie: &arrclient.MediaRef{
					ID:    5,
					Title: "Movie 2020",
				},
				Rejections: []arrclient.Rejection{},
			},
		}
		if err := json.NewEncoder(w).Encode(items); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	items, err := client.GetManualImport(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Path != "/downloads/Movie.2020.1080p.mkv" {
		t.Errorf("unexpected path: %s", items[0].Path)
	}
	if items[0].Movie == nil || items[0].Movie.Title != "Movie 2020" {
		t.Errorf("unexpected movie title")
	}
}

func TestPostManualImport(t *testing.T) {
	var receivedCmd arrclient.ManualImportCommand

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v3/command" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedCmd); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	item := arrclient.ManualImportItem{
		ID:   1,
		Path: "/downloads/Movie.2020.mkv",
		Movie: &arrclient.MediaRef{
			ID:    42,
			Title: "Movie 2020",
		},
		Quality:    json.RawMessage(`{"quality":{"id":7}}`),
		Languages:  json.RawMessage(`[{"id":1,"name":"English"}]`),
		DownloadID: "dl-abc",
		FolderName: "Movie.2020",
	}

	if err := client.PostManualImport(context.Background(), &item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedCmd.Name != "ManualImport" {
		t.Errorf("expected command name ManualImport, got %s", receivedCmd.Name)
	}
	if receivedCmd.ImportMode != "move" {
		t.Errorf("expected importMode move, got %s", receivedCmd.ImportMode)
	}
	if len(receivedCmd.Files) != 1 {
		t.Fatalf("expected 1 file in command, got %d", len(receivedCmd.Files))
	}

	f := receivedCmd.Files[0]
	if f.Path != "/downloads/Movie.2020.mkv" {
		t.Errorf("expected path /downloads/Movie.2020.mkv, got %s", f.Path)
	}
	if f.MovieID != 42 {
		t.Errorf("expected movieId 42, got %d", f.MovieID)
	}
	if f.DownloadID != "dl-abc" {
		t.Errorf("expected downloadId dl-abc, got %s", f.DownloadID)
	}
	if f.FolderName != "Movie.2020" {
		t.Errorf("expected folderName Movie.2020, got %s", f.FolderName)
	}
}

func TestPostManualImport_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	item := arrclient.ManualImportItem{ID: 1, Path: "/downloads/movie.mkv", Movie: &arrclient.MediaRef{ID: 1, Title: "M"}}

	if err := client.PostManualImport(context.Background(), &item); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClientName(t *testing.T) {
	client := arrclient.NewClient("http://localhost:7878/", "key", "radarr")
	if client.Name() != "Radarr (http://localhost:7878)" {
		t.Errorf("unexpected name: %s", client.Name())
	}
}

func TestClientName_DistinctPerInstance(t *testing.T) {
	hd := arrclient.NewClient("http://radarr:7878", "key-1", "radarr")
	uhd := arrclient.NewClient("http://radarr-4k:7878", "key-2", "radarr")

	if hd.Name() == uhd.Name() {
		t.Errorf("two Radarr instances must be distinguishable, both are %s", hd.Name())
	}
}

func TestJSONUnmarshal_QueueRecord(t *testing.T) {
	raw := `{
		"id": 42,
		"title": "White.Noise.2.2007.1080p.mkv",
		"downloadId": "XYZ789",
		"statusMessages": [
			{
				"title": "Found matching movie via grab history, but release was matched to movie by ID. Manual Import required.",
				"messages": ["Release was matched to movie by ID"]
			}
		]
	}`

	var record arrclient.QueueRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.ID != 42 {
		t.Errorf("expected id 42, got %d", record.ID)
	}
	if record.DownloadID != "XYZ789" {
		t.Errorf("expected downloadId XYZ789, got %s", record.DownloadID)
	}
	if len(record.StatusMessages) != 1 {
		t.Fatalf("expected 1 status message, got %d", len(record.StatusMessages))
	}
}

func TestJSONUnmarshal_ManualImportItem(t *testing.T) {
	raw := `{
		"id": 1,
		"path": "/downloads/White.Noise.2.2007.1080p.mkv",
		"name": "White.Noise.2.2007.1080p.mkv",
		"size": 9000000000,
		"movie": {"id": 5, "title": "White Noise 2: The Light"},
		"rejections": [],
		"downloadId": "XYZ789",
		"quality": {"quality": {"id": 7, "name": "Bluray-1080p"}},
		"languages": [{"id": 1, "name": "English"}]
	}`

	var item arrclient.ManualImportItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Path != "/downloads/White.Noise.2.2007.1080p.mkv" {
		t.Errorf("unexpected path: %s", item.Path)
	}
	if item.Movie == nil || item.Movie.Title != "White Noise 2: The Light" {
		t.Errorf("unexpected movie title")
	}
	if item.Movie.ID != 5 {
		t.Errorf("expected movie id 5, got %d", item.Movie.ID)
	}
	if item.Size != 9000000000 {
		t.Errorf("unexpected size: %d", item.Size)
	}
	if len(item.Rejections) != 0 {
		t.Errorf("expected 0 rejections, got %d", len(item.Rejections))
	}
}

func TestGetQueue_RadarrParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeUnknownMovieItems") != "true" {
			t.Errorf("expected includeUnknownMovieItems=true for radarr")
		}
		if r.URL.Query().Get("includeUnknownSeriesItems") != "" {
			t.Errorf("unexpected includeUnknownSeriesItems param for radarr")
		}
		if err := json.NewEncoder(w).Encode(arrclient.QueueResponse{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "key", "radarr")
	if _, err := client.GetQueue(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetQueue_SonarrParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeUnknownSeriesItems") != "true" {
			t.Errorf("expected includeUnknownSeriesItems=true for sonarr")
		}
		if r.URL.Query().Get("includeUnknownMovieItems") != "" {
			t.Errorf("unexpected includeUnknownMovieItems param for sonarr")
		}
		if err := json.NewEncoder(w).Encode(arrclient.QueueResponse{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "key", "sonarr")
	if _, err := client.GetQueue(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetManualImport_URLEncodesDownloadID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The downloadId should be properly URL-encoded, so the query parser
		// should return the original value with special characters.
		got := r.URL.Query().Get("downloadId")
		if got != "abc&evil=true" {
			t.Errorf("expected downloadId 'abc&evil=true', got %q", got)
		}
		if err := json.NewEncoder(w).Encode([]arrclient.ManualImportItem{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "radarr")
	_, err := client.GetManualImport(context.Background(), "abc&evil=true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostManualImport_SonarrCommand(t *testing.T) {
	var receivedCmd arrclient.ManualImportCommand

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/command" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedCmd); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := arrclient.NewClient(server.URL, "test-key", "sonarr")
	item := arrclient.ManualImportItem{
		ID:   1,
		Path: "/downloads/Show.S01E01.mkv",
		Series: &arrclient.MediaRef{
			ID:    10,
			Title: "The Show",
		},
		Episodes: []arrclient.EpisodeRef{
			{ID: 100},
			{ID: 101},
		},
		Quality:     json.RawMessage(`{"quality":{"id":4}}`),
		Languages:   json.RawMessage(`[{"id":1,"name":"English"}]`),
		DownloadID:  "dl-xyz",
		FolderName:  "Show.S01",
		ReleaseType: json.RawMessage(`0`),
	}

	if err := client.PostManualImport(context.Background(), &item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedCmd.Name != "ManualImport" {
		t.Errorf("expected command name ManualImport, got %s", receivedCmd.Name)
	}
	if len(receivedCmd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(receivedCmd.Files))
	}

	f := receivedCmd.Files[0]
	if f.SeriesID != 10 {
		t.Errorf("expected seriesId 10, got %d", f.SeriesID)
	}
	if f.MovieID != 0 {
		t.Errorf("expected movieId 0 for sonarr, got %d", f.MovieID)
	}
	if len(f.EpisodeIDs) != 2 || f.EpisodeIDs[0] != 100 || f.EpisodeIDs[1] != 101 {
		t.Errorf("unexpected episodeIds: %v", f.EpisodeIDs)
	}
	if f.DownloadID != "dl-xyz" {
		t.Errorf("expected downloadId dl-xyz, got %s", f.DownloadID)
	}
}
