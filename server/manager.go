package server

import (
	"net/http"

	"github.com/HeapOfChaos/goondvr/entity"
)

var Manager IManager

type IManager interface {
	CreateChannel(conf *entity.ChannelConfig, shouldSave bool) error
	StopChannel(channelID string) error
	PauseChannel(channelID string) error
	ResumeChannel(channelID string) error
	ChannelInfo() []*entity.ChannelInfo
	// Publish pushes the given kind of update to the web UI. It never blocks and
	// reports false when the event could not be queued (slow browser), in which
	// case the caller should retry later.
	Publish(name string, ch *entity.ChannelInfo) bool
	Subscriber(w http.ResponseWriter, r *http.Request)
	LoadConfig() error
	SaveConfig() error
	Shutdown()
	GetChannelThumb(channelID string) string
	GetChannelLiveThumb(channelID string) string
	ReportCFBlock(username string)
	ResetCFBlock(username string)
	GetStats() StatsResponse
	// HasViewers reports whether any browser has the web UI open, so that work
	// done only for the sake of the UI (renewing previews) can be skipped.
	HasViewers() bool
}

// StatsResponse holds system stats returned by the /api/stats endpoint.
type StatsResponse struct {
	DiskPath       string  `json:"disk_path"`
	DiskUsedBytes  uint64  `json:"disk_used_bytes"`
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskPercent    float64 `json:"disk_percent"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	RecordingCount int     `json:"recording_count"`
}
