package job

import (
	"context"
	"testing"

	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSchedulingUnavailable(t *testing.T) {
	app := NewAppForJob(&Options{})
	require.ErrorIs(t, app.Schedule("0 0 1 1 *", func(context.Context) error { return nil }, scheduler.JobConfig{Name: "task"}), ErrSchedulerUnavailable)
	require.ErrorIs(t, app.ScheduleDispatch("0 0 1 1 *", "follow-up", nil, DispatchConfig{}, scheduler.JobConfig{Name: "task"}), ErrSchedulerUnavailable)
	require.Nil(t, app.ScheduledJobs())
	require.ErrorIs(t, app.RemoveScheduledJob(uuid.New()), ErrSchedulerUnavailable)
}
