package channel

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/server"
)

// uiMinInterval is the minimum time between two pushes of the same kind
// (info / thumb / log) of one channel to the web UI. Recording fires a signal
// on every segment (several per second with LL-HLS), and every push makes the
// browser rebuild part of the page, so bursts are coalesced and only the
// latest state is sent. (A variable only so tests can shorten it.)
var uiMinInterval = 2 * time.Second

// maxLogLines is the number of log lines kept per channel for the web UI.
const maxLogLines = 100

// Channel represents a channel instance.
type Channel struct {
	// LogCh, UpdateCh and ThumbCh are "something changed" signals for the
	// Publisher. They hold at most one pending signal and are written with a
	// non-blocking send (see signal), so neither the recording loop nor the
	// loggers can ever be stalled by a slow or frozen browser.
	LogCh    chan bool
	UpdateCh chan bool
	ThumbCh  chan bool
	done     chan struct{}
	// publisherExited is closed when the Publisher goroutine has returned.
	publisherExited chan struct{}

	IsOnline            bool
	StreamedAt          int64
	Duration            float64 // Seconds
	Filesize            int     // Bytes
	TotalDiskUsageBytes int64   // Total bytes across all recordings for this channel
	Sequence            int
	FileExt             string // ".ts" or ".mp4", set per-stream
	RoomTitle           string
	Gender              string
	NumViewers          int
	EdgeRegion          string
	SummaryCardImage    string
	LiveThumbURL        string
	CFBlockCount        int

	logsMu sync.RWMutex
	Logs   []string

	fileMu                  sync.RWMutex
	finalizeMu              sync.Mutex
	finalizeWG              sync.WaitGroup
	finalizeCount           int
	monitorMu               sync.Mutex
	monitorCtx              context.Context    // context of the current monitor run (created by startMonitorLocked)
	monitorCancel           context.CancelFunc // cancels monitorCtx; both guarded by monitorMu
	monitorRunning          bool
	monitorRestartRequested bool
	monitorRunID            uint64
	monitorDone             chan struct{}
	doneOnce                sync.Once

	File           *os.File
	mp4InitSegment []byte
	Config         *entity.ChannelConfig
}

// New creates a new channel instance with the given manager and configuration.
func New(conf *entity.ChannelConfig) *Channel {
	ch := &Channel{
		LogCh:            make(chan bool, 1),
		UpdateCh:         make(chan bool, 1),
		ThumbCh:          make(chan bool, 1),
		done:             make(chan struct{}),
		publisherExited:  make(chan struct{}),
		Config:           conf,
		monitorCancel:    func() {},
		RoomTitle:        conf.RoomTitle,
		Gender:           conf.Gender,
		SummaryCardImage: conf.SummaryCardImage,
		StreamedAt:       conf.StreamedAt,
	}
	go ch.Publisher()

	return ch
}

// signal raises a coalescing "something changed" flag. It never blocks: if a
// signal is already pending the new one is merged into it.
func signal(c chan bool) {
	select {
	case c <- true:
	default:
	}
}

// Publisher turns the change signals of the channel into SSE events for the
// web UI.
//
// Each kind of event (info, thumb, log) is sent at most once per
// uiMinInterval. The first change after a quiet period is sent immediately;
// changes that arrive during the cool-down are merged and sent once it ends,
// always carrying the latest state. If the SSE queue is full (browser too slow
// to keep up) the push is retried later instead of blocking.
func (ch *Channel) Publisher() {
	defer close(ch.publisherExited)

	events := [...]entity.Event{entity.EventUpdate, entity.EventThumb, entity.EventLog}
	var (
		pending [len(events)]bool
		last    [len(events)]time.Time
		wake    <-chan time.Time
	)

	for {
		select {
		case <-ch.done:
			return
		case <-ch.UpdateCh:
			pending[0] = true
		case <-ch.ThumbCh:
			pending[1] = true
		case <-ch.LogCh:
			pending[2] = true
		case <-wake:
		}

		var (
			info *entity.ChannelInfo
			wait time.Duration = -1
			now                = time.Now()
		)
		for i, evt := range events {
			if !pending[i] {
				continue
			}
			if remaining := uiMinInterval - now.Sub(last[i]); remaining > 0 {
				if wait < 0 || remaining < wait {
					wait = remaining
				}
				continue
			}
			if info == nil {
				info = ch.ExportInfo()
			}
			// Even when the queue is full we restart the cool-down, so a stuck
			// browser is retried every uiMinInterval rather than in a hot loop.
			last[i] = now
			if server.Manager.Publish(evt, info) {
				pending[i] = false
			} else if wait < 0 || uiMinInterval < wait {
				wait = uiMinInterval
			}
		}

		wake = nil
		if wait >= 0 {
			wake = time.After(wait)
		}
	}
}

// Info logs an informational message to both the browser log and stdout.
func (ch *Channel) Info(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	ch.sendLog(fmt.Sprintf("%s [INFO] %s", time.Now().Format("15:04"), msg))
	log.Printf(" INFO [%s] %s", ch.Config.Username, msg)
}

// Verbose logs a message to the browser log always, and to stdout only when --debug is enabled.
// Use this for high-frequency events (e.g. per-segment updates) that would clutter the console.
func (ch *Channel) Verbose(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	ch.sendLog(fmt.Sprintf("%s [INFO] %s", time.Now().Format("15:04"), msg))
	if server.Config.Debug {
		log.Printf(" INFO [%s] %s", ch.Config.Username, msg)
	}
}

// Error logs an error message to both the browser log and stdout.
func (ch *Channel) Error(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	ch.sendLog(fmt.Sprintf("%s [ERROR] %s", time.Now().Format("15:04"), msg))
	log.Printf("ERROR [%s] %s", ch.Config.Username, msg)
}

// ExportInfo exports the channel information as a ChannelInfo struct.
func (ch *Channel) ExportInfo() *entity.ChannelInfo {
	var filename string
	ch.fileMu.RLock()
	if ch.File != nil {
		filename = ch.File.Name()
	}
	duration := ch.Duration
	filesize := ch.Filesize
	totalDiskUsageBytes := ch.TotalDiskUsageBytes
	ch.fileMu.RUnlock()
	var streamedAt string
	if ch.StreamedAt != 0 {
		streamedAt = time.Unix(ch.StreamedAt, 0).Format("2006-01-02 15:04 AM")
	}
	ch.logsMu.RLock()
	logs := make([]string, len(ch.Logs))
	copy(logs, ch.Logs)
	ch.logsMu.RUnlock()

	siteDomain := server.Config.Domain // default to Chaturbate domain
	if ch.Config.Site == "stripchat" {
		siteDomain = "https://stripchat.com/"
	}

	site := ch.Config.Site
	if site == "" {
		site = "chaturbate"
	}

	return &entity.ChannelInfo{
		ChannelID:        entity.ChannelID(ch.Config.Site, ch.Config.Username),
		IsOnline:         ch.IsOnline,
		IsPaused:         ch.isPaused(), // Pause/Resume write it under monitorMu
		Username:         ch.Config.Username,
		MaxDuration:      internal.FormatDuration(float64(ch.Config.MaxDuration * 60)), // MaxDuration from config is in minutes
		MaxFilesize:      internal.FormatFilesize(ch.Config.MaxFilesize * 1024 * 1024), // MaxFilesize from config is in MB
		StreamedAt:       streamedAt,
		CreatedAt:        ch.Config.CreatedAt,
		Duration:         internal.FormatDuration(duration),
		Filesize:         internal.FormatFilesize(filesize),
		TotalDiskUsage:   internal.FormatFilesize(int(totalDiskUsageBytes)),
		Filename:         filename,
		Logs:             logs,
		GlobalConfig:     server.Config,
		RoomTitle:        ch.RoomTitle,
		Gender:           ch.Gender,
		NumViewers:       ch.NumViewers,
		EdgeRegion:       ch.EdgeRegion,
		SummaryCardImage: ch.SummaryCardImage,
		LiveThumbURL:     ch.LiveThumbURL,
		Site:             site,
		SiteDomain:       siteDomain,
	}
}

// Pause pauses the channel and cancels the context.
func (ch *Channel) Pause() {
	// Stop the monitoring loop, this also updates `ch.IsOnline` to false
	// `context.Canceled` → `ch.Monitor()` → `onRetry` → `ch.UpdateOnlineStatus(false)`.
	ch.monitorMu.Lock()
	ch.monitorRestartRequested = false
	ch.Config.IsPaused = true
	cancel := ch.monitorCancel
	ch.monitorMu.Unlock()
	cancel()

	ch.Update()
	ch.Info("channel paused")
}

// Stop stops the channel and cancels the context.
func (ch *Channel) Stop() {
	// Stop the monitoring loop
	ch.monitorMu.Lock()
	ch.monitorRestartRequested = false
	ch.Config.IsPaused = true
	cancel := ch.monitorCancel
	ch.monitorMu.Unlock()
	cancel()
	ch.waitForMonitorStop()
	ch.waitForFinalizations()

	ch.Info("channel stopped")
	ch.stopPublisher()
}

// Resume resumes the channel monitoring.
//
// `startSeq` is used to prevent all channels from starting at the same time, preventing TooManyRequests errors.
// It's only be used when program starting and trying to resume all channels at once.
func (ch *Channel) Resume(startSeq int) {
	go func() {
		if startSeq > 0 {
			select {
			case <-time.After(time.Duration(startSeq) * time.Second):
			case <-ch.done: // channel was removed while waiting for its turn
				return
			}
			// The user may have paused the channel while it was waiting for its
			// turn; starting it now would silently undo that.
			if ch.isPaused() {
				return
			}
		}
		runID, ok := ch.requestMonitorStart()
		if !ok {
			return
		}
		ch.Update()
		ch.Info("channel resumed")
		ch.Monitor(runID)
	}()
}

// UpdateOnlineStatus updates the online status of the channel.
func (ch *Channel) UpdateOnlineStatus(isOnline bool) {
	ch.IsOnline = isOnline
	if !isOnline {
		ch.NumViewers = 0
	}
	ch.UpdateThumb()
	ch.Update()
}

// UpdateThumb sends a thumbnail-only update signal to the publisher.
func (ch *Channel) UpdateThumb() {
	signal(ch.ThumbCh)
}

// requestMonitorStart starts a monitor immediately when possible, or records
// a pending restart if a previous monitor is still shutting down.
func (ch *Channel) requestMonitorStart() (uint64, bool) {
	ch.monitorMu.Lock()
	defer ch.monitorMu.Unlock()

	if ch.monitorRunning {
		if ch.Config.IsPaused {
			ch.monitorRestartRequested = true
		}
		return 0, false
	}

	return ch.startMonitorLocked(), true
}

// finishMonitor clears the running flag when a monitor loop exits.
func (ch *Channel) finishMonitor() {
	ch.monitorMu.Lock()
	done := ch.monitorDone
	shouldRestart := ch.monitorRestartRequested
	ch.monitorRunning = false
	ch.monitorRestartRequested = false
	var runID uint64
	if shouldRestart {
		runID = ch.startMonitorLocked()
	} else {
		ch.monitorDone = nil
	}
	ch.monitorMu.Unlock()

	if done != nil {
		close(done)
	}

	if shouldRestart {
		ch.Update()
		ch.Info("channel resumed")
		go ch.Monitor(runID)
	}
}

// startMonitorLocked marks a monitor as active and allocates a new run ID.
// monitorMu must already be held by the caller.
func (ch *Channel) startMonitorLocked() uint64 {
	// A fresh context for every run, created here (not later inside Monitor) so
	// that a Pause arriving at any moment after the start cancels *this* run.
	ch.monitorCancel() // release the previous run's context
	ch.monitorCtx, ch.monitorCancel = context.WithCancel(context.Background())
	ch.Config.IsPaused = false
	ch.monitorRunning = true
	ch.monitorRestartRequested = false
	ch.monitorDone = make(chan struct{})
	ch.monitorRunID++
	return ch.monitorRunID
}

// isPaused reports whether the channel is currently paused.
func (ch *Channel) isPaused() bool {
	ch.monitorMu.Lock()
	defer ch.monitorMu.Unlock()
	return ch.Config.IsPaused
}

// runContext returns the context of the current monitor run.
func (ch *Channel) runContext() context.Context {
	ch.monitorMu.Lock()
	defer ch.monitorMu.Unlock()
	return ch.monitorCtx
}

// stopPublisher signals the publisher goroutine to exit. It is only used when
// a channel is being permanently stopped and removed, not for pause/resume.
func (ch *Channel) stopPublisher() {
	ch.doneOnce.Do(func() {
		close(ch.done)
	})
}

// sendLog stores a log line (keeping the last maxLogLines) and tells the
// publisher that the log changed, unless the channel has been permanently shut
// down. It never blocks on the web UI.
func (ch *Channel) sendLog(msg string) {
	select {
	case <-ch.done:
		return
	default:
	}

	ch.logsMu.Lock()
	ch.Logs = append(ch.Logs, msg)
	if len(ch.Logs) > maxLogLines {
		ch.Logs = ch.Logs[len(ch.Logs)-maxLogLines:]
	}
	ch.logsMu.Unlock()

	signal(ch.LogCh)
}

// waitForMonitorStop blocks until the current monitor run has finished cleanup.
func (ch *Channel) waitForMonitorStop() {
	ch.monitorMu.Lock()
	done := ch.monitorDone
	ch.monitorMu.Unlock()

	if done != nil {
		<-done
	}
}

func (ch *Channel) startFinalization() {
	ch.finalizeMu.Lock()
	ch.finalizeCount++
	ch.finalizeWG.Add(1)
	ch.finalizeMu.Unlock()
}

func (ch *Channel) finishFinalization() {
	ch.finalizeMu.Lock()
	ch.finalizeCount--
	ch.finalizeMu.Unlock()
	ch.finalizeWG.Done()
}

func (ch *Channel) waitForFinalizations() int {
	ch.finalizeMu.Lock()
	pending := ch.finalizeCount
	ch.finalizeMu.Unlock()

	if pending > 0 {
		ch.Info("waiting for %d recording finalization task(s)", pending)
		ch.finalizeWG.Wait()
	}
	return pending
}
