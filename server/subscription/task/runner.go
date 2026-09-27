package task

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/archive"
	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/config"
	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/internal/downloaders"
	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/internal/kv"
	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/internal/queue"
	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/subscription/domain"
	"github.com/robfig/cron/v3"
)

type TaskRunner interface {
	Submit(subcription *domain.Subscription) error
	Spawner(ctx context.Context)
	StopTask(id string) error
	Recoverer()
}

type monitorTask struct {
	Done         chan struct{}
	Schedule     cron.Schedule
	Subscription *domain.Subscription
}

type CronTaskRunner struct {
	mq *queue.MessageQueue
	db *kv.Store

	tasks   chan monitorTask
	running map[string]*monitorTask
}

func NewCronTaskRunner(mq *queue.MessageQueue, db *kv.Store) TaskRunner {
	return &CronTaskRunner{
		mq:      mq,
		db:      db,
		tasks:   make(chan monitorTask),
		running: make(map[string]*monitorTask),
	}
}

var argsSplitterRe = regexp.MustCompile(`(?mi)[^\s"']+|"([^"]*)"|'([^']*)'`)

// subscription arguments are stored as one string, e.g.
// "--proxy socks5://10.0.0.1:3000/ -t mkv": split them into yt-dlp arguments.
func splitArgs(params string) []string {
	return argsSplitterRe.FindAllString(params, -1)
}

func (t *CronTaskRunner) Submit(subcription *domain.Subscription) error {
	schedule, err := cron.ParseStandard(subcription.CronExpr)
	if err != nil {
		return err
	}

	job := monitorTask{
		Done:         make(chan struct{}),
		Schedule:     schedule,
		Subscription: subcription,
	}

	t.tasks <- job

	return nil
}

// Handles the entire lifecylce of a monitor job.
func (t *CronTaskRunner) Spawner(ctx context.Context) {
	for req := range t.tasks {
		t.running[req.Subscription.Id] = &req // keep track of the current job

		go func() {
			ctx, cancel := context.WithCancel(ctx) // inject into the job's context a cancellation singal
			fetcherEvents := t.doFetch(ctx, &req)  // retrieve the channel of events of the job

			for {
				select {
				case <-req.Done:
					slog.Info("stopping cron job and removing schedule", slog.String("url", req.Subscription.URL))
					cancel()
					return
				case <-fetcherEvents:
					slog.Info("finished monitoring channel", slog.String("url", req.Subscription.URL))
				}
			}
		}()
	}
}

// Stop a currently scheduled job
func (t *CronTaskRunner) StopTask(id string) error {
	task := t.running[id]
	if task != nil {
		t.running[id].Done <- struct{}{}
		delete(t.running, id)
	}
	return nil
}

// Start a fetcher and notify on a channel when a fetcher has completed
func (t *CronTaskRunner) doFetch(ctx context.Context, req *monitorTask) <-chan struct{} {
	completed := make(chan struct{})

	// generator func
	go func() {
		for {
			sleepFor := t.fetcher(ctx, req)
			completed <- struct{}{}

			time.Sleep(sleepFor)
		}
	}()

	return completed
}

// Perform the retrieval of the latest video of the channel.
// Returns a time.Duration containing the amount of time to the next schedule.
func (t *CronTaskRunner) fetcher(ctx context.Context, req *monitorTask) time.Duration {
	slog.Info("fetching latest video for channel", slog.String("channel", req.Subscription.URL))

	nextSchedule := time.Until(req.Schedule.Next(time.Now()))

	// The subscription arguments are used for the query as well: a proxy (or a
	// cookies file) is needed to reach the channel page just as much as it is
	// needed to download the video itself.
	args := append(
		[]string{"-I1", "--flat-playlist", "--print", "webpage_url"},
		append(splitArgs(req.Subscription.Params), req.Subscription.URL)...,
	)

	cmd := exec.CommandContext(ctx, config.Instance().Paths.DownloaderPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()
	if err != nil {
		// Never drop the schedule: the loop has to keep polling, otherwise a
		// single failure (no network, expired cookies, ...) stops the
		// subscription forever.
		slog.Error(
			"failed to fetch the latest video for channel",
			slog.String("channel", req.Subscription.URL),
			slog.String("error", err.Error()),
			slog.String("stderr", lastLine(stderr.String())),
			slog.Any("retry_in", nextSchedule),
		)
		return nextSchedule
	}

	latestVideoURL := strings.TrimSpace(string(stdout))

	if latestVideoURL == "" {
		slog.Warn("no video found for channel", slog.String("channel", req.Subscription.URL))
		return nextSchedule
	}

	// if the download exists there's not point in sending it into the message queue.
	exists, err := archive.DownloadExists(ctx, latestVideoURL, splitArgs(req.Subscription.Params)...)
	if err != nil {
		slog.Warn(
			"could not check the archive, queueing the download anyway",
			slog.String("url", latestVideoURL),
			slog.String("error", err.Error()),
		)
	}
	if exists {
		return nextSchedule
	}

	// TODO: autoremove hook
	d := downloaders.NewGenericDownload(
		latestVideoURL,
		append(
			splitArgs(req.Subscription.Params),
			[]string{
				"--break-on-existing",
				"--download-archive",
				filepath.Join(config.Instance().Dir(), "archive.txt"),
			}...),
	)

	t.db.Set(d)     // give it an id
	t.mq.Publish(d) // send it to the message queue waiting to be processed

	slog.Info(
		"cron task runner next schedule",
		slog.String("url", req.Subscription.URL),
		slog.Any("duration", nextSchedule),
	)

	return nextSchedule
}

func (t *CronTaskRunner) Recoverer() {
	panic("unimplemented")
}

// lastLine returns the last non empty line of s.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}
