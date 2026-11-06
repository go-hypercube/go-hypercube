// Package scheduler runs scheduled callbacks in the current process, independently
// of the framework app, queue drivers, and job registry.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

// Scheduler uses gocron's scheduling and lifecycle controls.
// Its owner must call Shutdown, even if Start is never called.
// RemoveJob removes a job by ID; RemoveByTags removes jobs by namespace or other tags.
type Scheduler struct {
	gocron.Scheduler
	// Serializes configured registration and namespace-scoped removal.
	mu sync.Mutex
}

// New creates a stopped scheduler using the supplied gocron options.
func New(options ...gocron.SchedulerOption) (*Scheduler, error) {
	s, err := gocron.NewScheduler(options...)
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}
	return &Scheduler{Scheduler: s}, nil
}

// ErrNilTask indicates that a scheduled callback is nil.
var ErrNilTask = errors.New("scheduled task must not be nil")

// ErrEmptyNamespace indicates that no schedule owner was supplied.
var ErrEmptyNamespace = errors.New("scheduled job namespace must not be empty")

// ErrEmptyName indicates that no schedule name was supplied.
var ErrEmptyName = errors.New("scheduled job name must not be empty")

// ErrReservedTag indicates that a caller supplied a namespace ownership tag.
var ErrReservedTag = errors.New("namespace tags are reserved for schedule ownership")

// ErrDuplicateID indicates that a cron already uses the requested identifier.
var ErrDuplicateID = errors.New("scheduled job ID already exists")

// JobConfig describes a cron's identity and execution options.
// ID is optional and defaults to a generated UUID. Name and Namespace are required.
// Use the host namespace or a plugin namespace; ownership is stored as a
// reserved "namespace:<name>" tag.
type JobConfig struct {
	ID        uuid.UUID
	Name      string
	Namespace string
	Tags      []string
	// Options configures execution, for example WithContext or WithSingletonMode.
	// ID, Name, and Tags above take precedence over metadata options here.
	Options []gocron.JobOption
}

// NamespaceTag returns the reserved tag identifying a schedule's owner.
func NamespaceTag(namespace string) string { return "namespace:" + namespace }

// Schedule registers a cron and returns its handle for inspection and removal.
// The task context is canceled on removal or shutdown; callbacks must observe
// cancellation to stop promptly.
func (sc *Scheduler) Schedule(crontab string, task func(context.Context) error, config JobConfig) (gocron.Job, error) {
	if task == nil {
		return nil, ErrNilTask
	}
	if config.Namespace == "" {
		return nil, ErrEmptyNamespace
	}
	if config.Name == "" {
		return nil, ErrEmptyName
	}
	if config.ID == uuid.Nil {
		config.ID = uuid.New()
	}
	for _, tag := range config.Tags {
		if strings.HasPrefix(tag, "namespace:") {
			return nil, ErrReservedTag
		}
	}
	tags := append([]string{NamespaceTag(config.Namespace)}, config.Tags...)
	options := append([]gocron.JobOption(nil), config.Options...)
	options = append(options, gocron.WithIdentifier(config.ID), gocron.WithName(config.Name), gocron.WithTags(tags...))
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for _, scheduled := range sc.Jobs() {
		if scheduled.ID() == config.ID {
			return nil, fmt.Errorf("schedule task %q: %w", config.Name, ErrDuplicateID)
		}
	}
	scheduled, err := sc.NewJob(gocron.CronJob(crontab, false), gocron.NewTask(func(ctx context.Context) {
		if err := task(ctx); err != nil {
			slog.Error("scheduled task failed", "id", config.ID, "name", config.Name,
				"namespace", config.Namespace, "cron", crontab, "err", err)
		}
	}), options...)
	if err != nil {
		return nil, fmt.Errorf("schedule task %q in namespace %q with cron %q: %w", config.Name, config.Namespace, crontab, err)
	}
	return scheduled, nil
}

// JobsInNamespace lists the cron handles owned by namespace.
func (sc *Scheduler) JobsInNamespace(namespace string) []gocron.Job {
	var jobs []gocron.Job
	for _, scheduled := range sc.Jobs() {
		if slices.Contains(scheduled.Tags(), NamespaceTag(namespace)) {
			jobs = append(jobs, scheduled)
		}
	}
	return jobs
}

// RemoveJobInNamespace removes a job only if it belongs to namespace.
// Missing IDs and IDs owned by another namespace return gocron.ErrJobNotFound.
func (sc *Scheduler) RemoveJobInNamespace(namespace string, id uuid.UUID) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for _, scheduled := range sc.JobsInNamespace(namespace) {
		if scheduled.ID() == id {
			return sc.RemoveJob(id)
		}
	}
	return gocron.ErrJobNotFound
}
