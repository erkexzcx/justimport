package arrclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for a single Radarr or Sonarr instance.
type Client struct {
	baseURL    string
	apiKey     string
	appType    string // "radarr" or "sonarr": selects API behaviour
	label      string // human-readable identity of this instance, see Name
	httpClient *http.Client
}

// NewClient creates a new Client for the given *arr instance.
// appType is "radarr" or "sonarr" and selects the instance's API behaviour.
func NewClient(baseURL, apiKey, appType string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")

	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		appType: appType,
		label:   instanceLabel(appType, baseURL),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// instanceLabel builds the human-readable identity of an instance. Several
// instances of the same app can run side by side, so the URL is part of the
// label — Name must be unique per instance, as the importer keys state on it.
func instanceLabel(appType, baseURL string) string {
	if appType == "" {
		return baseURL
	}

	return strings.ToUpper(appType[:1]) + appType[1:] + " (" + baseURL + ")"
}

// Name returns the instance label, e.g. "Radarr (http://radarr4k:7878)".
func (c *Client) Name() string {
	return c.label
}

// maxResponseBytes is the maximum response body size the client will read (10 MB).
const maxResponseBytes = 10 * 1024 * 1024

func (c *Client) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck // response body close error is intentionally ignored

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	err = json.NewDecoder(limited).Decode(out)
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// CheckConnectivity verifies the connection to the instance by fetching system status.
// Returns the app name and version on success.
func (c *Client) CheckConnectivity(ctx context.Context) (string, string, error) {
	var status struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}

	if err := c.doGet(ctx, "/api/v3/system/status", &status); err != nil {
		return "", "", err
	}

	return status.AppName, status.Version, nil
}

// GetQueue fetches all queue records from the instance.
func (c *Client) GetQueue(ctx context.Context) ([]QueueRecord, error) {
	var resp QueueResponse

	// Radarr uses includeUnknownMovieItems; Sonarr uses includeUnknownSeriesItems.
	param := "includeUnknownMovieItems"
	if c.appType == "sonarr" {
		param = "includeUnknownSeriesItems"
	}

	path := fmt.Sprintf("/api/v3/queue?pageSize=1000&%s=true", param)
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}

	return resp.Records, nil
}

// GetManualImport fetches the list of files available for manual import for the given download ID.
func (c *Client) GetManualImport(ctx context.Context, downloadID string) ([]ManualImportItem, error) {
	var items []ManualImportItem

	path := "/api/v3/manualimport?downloadId=" + url.QueryEscape(downloadID)
	if err := c.doGet(ctx, path, &items); err != nil {
		return nil, err
	}

	return items, nil
}

// PostManualImport sends a manual import command for the given item via POST /api/v3/command.
func (c *Client) PostManualImport(ctx context.Context, item *ManualImportItem) error {
	file := ManualImportFile{
		Path:         item.Path,
		FolderName:   item.FolderName,
		Quality:      item.Quality,
		Languages:    item.Languages,
		ReleaseGroup: item.ReleaseGroup,
		IndexerFlags: item.IndexerFlags,
		DownloadID:   item.DownloadID,
	}

	if item.Movie != nil {
		file.MovieID = item.Movie.ID
	}

	if item.Series != nil {
		file.SeriesID = item.Series.ID
		file.ReleaseType = item.ReleaseType

		for _, ep := range item.Episodes {
			file.EpisodeIDs = append(file.EpisodeIDs, ep.ID)
		}
	}

	cmd := ManualImportCommand{
		Name:       "ManualImport",
		Files:      []ManualImportFile{file},
		ImportMode: "move",
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v3/command", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck // response body close error is intentionally ignored

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d for manual import command", resp.StatusCode)
	}

	return nil
}
