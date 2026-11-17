package site

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/HeapOfChaos/goondvr/internal"
)

// StreamInfo holds live stream data returned by a site when a channel is online.
type StreamInfo struct {
	HLSURL           string
	RoomTitle        string
	Gender           string
	NumViewers       int
	SummaryCardImage string
	LiveThumbURL     string // live-updating thumbnail URL; empty = use platform default (e.g. mmcdn)
	CDNReferer       string // Referer/Origin to use for CDN media requests; empty defaults to Chaturbate
	MouflonPDKey     string // Stripchat MOUFLON v2 decryption key; empty if not applicable
}

// LiveThumbSource is implemented by sites whose live preview address is a
// snapshot that goes out of date: the preview only changes if the site is asked
// for the address of a newer one. (Sites whose preview address never changes,
// such as Chaturbate, do not implement it.)
type LiveThumbSource interface {
	// FetchLiveThumbURL returns the address of the latest live preview. It must
	// be cheap: it is called about once a minute for every channel on screen.
	FetchLiveThumbURL(ctx context.Context, req *internal.Req, username string) (string, error)
}

// Availability is the answer of a site to "is this channel broadcasting right
// now?", without any intention of recording it.
type Availability struct {
	Live         bool   // broadcasting in public, i.e. it could be recorded
	NumViewers   int    // 0 when unknown
	LiveThumbURL string // address of the newest preview snapshot, when the site works that way
}

// AvailabilitySource is implemented by sites that can say whether a channel is
// broadcasting with one light request. The web UI uses it to move the channels
// that are on air (but paused, i.e. not being recorded) to the top of the list.
type AvailabilitySource interface {
	// FetchAvailability must be cheap and must never use the browser fallback:
	// it runs for every paused channel, one at a time, only while somebody has
	// the web UI open. An error means "could not tell" (the previous answer is
	// kept); a channel that is offline, private or hidden is not an error.
	FetchAvailability(ctx context.Context, req *internal.Req, username string) (Availability, error)
}

// ErrListNotConfigured is returned by OnlineListSource.ListOnline when the list
// cannot be used until the user configures it (for example, an affiliate id).
// It is not a failure: the manager just keeps using the per-channel method.
var ErrListNotConfigured = errors.New("online list is not configured")

// Presence is what a site says about one channel in its list of rooms on the air.
type Presence struct {
	Live       bool     // broadcasting in public, i.e. it could be recorded
	Show       string   // the site's own word for the show: public, private, group, hidden, away...
	NumViewers int      // 0 when unknown
	Subject    string   // room subject, when the list carries it
	Tags       []string // the room's tags (lower case), when the list carries them
}

// OnlineListSource is implemented by sites that can say, with one request (or a
// few pages of one list), which of many channels are on the air. This is the
// main way the manager keeps the "on air" mark of every channel up to date
// without a browser being open; the per-channel AvailabilitySource is the
// fallback when it fails.
type OnlineListSource interface {
	// ListOnline returns the presence of the wanted channels that are on the air,
	// keyed by lower-case username. want holds lower-case usernames (never empty).
	// A channel that is not in the answer is off the air, so any failure, and any
	// answer that cannot be trusted to be complete, must be an error; Cloudflare
	// blocks are reported as internal.ErrCloudflareBlocked. It must never use the
	// browser fallback.
	ListOnline(ctx context.Context, req *internal.Req, want map[string]struct{}) (map[string]Presence, error)
}

// Site defines the interface a streaming platform must implement.
type Site interface {
	// FetchStream returns StreamInfo if the channel is live, nil if offline.
	// Any non-nil error is a transient failure (retry). Offline is not an error — return nil, nil.
	FetchStream(ctx context.Context, req *internal.Req, username string) (*StreamInfo, error)
	// FetchLastBroadcast returns Unix timestamp of last broadcast, or 0 if unknown.
	FetchLastBroadcast(ctx context.Context, req *internal.Req, username string) (int64, error)
}

// Renamer is implemented by sites that send the old address of a renamed account to the new one.
type Renamer interface {
	// ResolveRename returns the new name of the account when the site redirects the address of username
	// to another account that exists, and "" when it does not. An error means "could not tell".
	ResolveRename(ctx context.Context, req *internal.Req, username string) (string, error)
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{2,64}$`)

// RenameFromRedirect asks for the page of username (pageURL) without following redirects. If the site sends it to
// the page of a different account on the same host, and exists says that account is real, that account is the
// new name. A redirect that only changes the case, leads elsewhere (login, age check, home page...) or leads to
// something that is not an account is not a rename: a wrong answer here would rename a channel for good.
func RenameFromRedirect(ctx context.Context, req *internal.Req, pageURL, username string, exists func(ctx context.Context, name string) error) (string, error) {
	loc, err := req.RedirectLocation(ctx, pageURL)
	if err != nil || loc == "" {
		return "", err
	}
	from, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	to, err := from.Parse(loc) // loc may be relative
	if err != nil || !strings.EqualFold(to.Host, from.Host) {
		return "", nil
	}
	name := strings.Trim(to.Path, "/")
	if !usernameRe.MatchString(name) || strings.EqualFold(name, username) {
		return "", nil
	}
	if err := exists(ctx, name); err != nil {
		return "", nil
	}
	return name, nil
}
