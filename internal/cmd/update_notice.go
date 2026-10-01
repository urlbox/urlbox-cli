package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/update"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// updateCheckTimeout bounds the once-a-day release lookup. The check runs
// alongside the command, so this only adds latency when the command itself
// finishes sooner.
const updateCheckTimeout = 2 * time.Second

var (
	updateCurrentVersion = func() string { return version.Version }
	updateFetchLatest    = func(ctx context.Context) (string, error) {
		return update.FetchLatest(ctx, update.LatestReleaseURL)
	}
)

// SetUpdateCheckForTest pins the running version and the release source.
// Pair with t.Cleanup(ResetUpdateCheckForTest).
func SetUpdateCheckForTest(current string, fetch func(context.Context) (string, error)) {
	updateCurrentVersion = func() string { return current }
	updateFetchLatest = fetch
}

// ResetUpdateCheckForTest restores the build version and the GitHub source.
func ResetUpdateCheckForTest() {
	updateCurrentVersion = func() string { return version.Version }
	updateFetchLatest = func(ctx context.Context) (string, error) {
		return update.FetchLatest(ctx, update.LatestReleaseURL)
	}
}

type fetchResult struct {
	latest string
	err    error
}

// updateNotifier tells interactive users when a newer release exists. It
// starts from the root PersistentPreRunE (flags are parsed by then) and
// prints after the command has written its own output.
type updateNotifier struct {
	started bool
	state   update.State
	result  chan fetchResult
	cancel  context.CancelFunc
}

// attachUpdateNotifier chains the notifier's start onto root's existing
// PersistentPreRunE. No subcommand defines its own, so this runs for every
// command that gets as far as executing.
func attachUpdateNotifier(root *cobra.Command) *updateNotifier {
	n := &updateNotifier{}
	pre := root.PersistentPreRunE
	root.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		if pre != nil {
			if err := pre(c, args); err != nil {
				return err
			}
		}
		n.start(c)
		return nil
	}
	return n
}

// updateNoticeAllowed is the same bar as the post-render report hint — text
// output to a human terminal only — plus opt-outs. JSON, quiet, --jq and
// piped runs stay byte-identical for agents and scripts.
func updateNoticeAllowed(c *cobra.Command) bool {
	if os.Getenv("URLBOX_NO_UPDATE_NOTIFIER") != "" || os.Getenv("CI") != "" {
		return false
	}
	switch c.Name() {
	case "upgrade", "__complete", "__completeNoDesc", "completion":
		return false // upgrade runs its own check; completion output is machine-read
	}
	root := c.Root()
	if jq, _ := root.PersistentFlags().GetString("jq"); jq != "" {
		return false
	}
	formatFlag, _ := root.PersistentFlags().GetString("output-format")
	if output.ResolveFormat(formatFlag, c.OutOrStdout()) != output.FormatText {
		return false
	}
	if !isStderrTTY(c.ErrOrStderr()) {
		return false
	}
	return update.IsRelease(updateCurrentVersion())
}

func (n *updateNotifier) start(c *cobra.Command) {
	if !updateNoticeAllowed(c) {
		return
	}
	n.started = true
	n.state = update.LoadState(update.StatePath())
	if !n.state.Stale(time.Now()) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	n.cancel = cancel
	n.result = make(chan fetchResult, 1)
	go func() {
		latest, err := updateFetchLatest(ctx)
		n.result <- fetchResult{latest, err}
	}()
}

// finish waits for an in-flight check (bounded by updateCheckTimeout),
// records it, and prints the notice when a newer release exists. A failed
// check is silent and still recorded, so an offline machine waits a day
// before trying again.
func (n *updateNotifier) finish(stderr io.Writer) {
	if !n.started {
		return
	}
	latest := n.state.Latest
	if n.result != nil {
		res := <-n.result
		n.cancel()
		if res.err == nil {
			latest = res.latest
		}
		_ = update.SaveState(update.StatePath(), update.State{CheckedAt: time.Now(), Latest: latest})
	}
	current := updateCurrentVersion()
	if !update.IsNewer(current, latest) {
		return
	}
	styles := output.NewStylesForWriter(stderr)
	_, _ = fmt.Fprintln(stderr, styles.Muted.Render(update.Notice(current, latest)))
}
