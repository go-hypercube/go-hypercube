package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/stretchr/testify/require"
)

func TestEvery(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	shutdown := false
	t.Cleanup(func() {
		if !shutdown {
			require.NoError(t, sc.Shutdown())
		}
	})
	require.ErrorIs(t, sc.Every("0 0 1 1 *", nil), ErrNilTask)
	require.ErrorIs(t, sc.Every("invalid", func(context.Context) error { return nil }), gocron.ErrCronJobParse)
	require.Empty(t, sc.Jobs())
	started := make(chan context.Context, 1)
	require.NoError(t, sc.Every("0 0 1 1 *", func(ctx context.Context) error {
		started <- ctx
		<-ctx.Done()
		return ctx.Err()
	}))
	sc.Start()
	require.NoError(t, sc.Jobs()[0].RunNow())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case taskCtx := <-started:
		shutdown = true
		require.NoError(t, sc.Shutdown())
		require.ErrorIs(t, taskCtx.Err(), context.Canceled)
	case <-ctx.Done():
		t.Fatal("direct scheduled task did not run")
	}
}

func TestNewWrapsErrors(t *testing.T) {
	sc, err := New(gocron.WithStopTimeout(-time.Second))
	require.Nil(t, sc)
	require.ErrorIs(t, err, gocron.ErrWithStopTimeoutZeroOrNegative)
}
