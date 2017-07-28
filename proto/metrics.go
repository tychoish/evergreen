package proto

import (
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	sysInfoCollectorInterval = 30 * time.Second

	// The proc info collector collects stats at one interval for
	// a certain number of iterations and then falls back to a
	// second interval: these values configure those intervals.
	procInfoFirstInterval   = 5 * time.Second
	procInfoFirstIterations = 30
	procInfoSecondInterval  = 10 * time.Second
)

// metricsCollector holds the functionality for running two system
// information (metrics) collecting go-routines. The structure holds a TaskCommunicator object
type metricsCollector struct {
	comm client.Communicator
}

// start validates the struct and launches two go routines.
func (c *metricsCollector) start(ctx context.Context) error {
	if c.stop == nil {
		return errors.New("no channel specified")
	}

	if c.comm == nil {
		return errors.New("no task communicator specified")
	}

	go c.sysInfoCollector(ctx, sysInfoCollectorInterval)
	go c.processInfoCollector(ctx, procInfoFirstInterval, procInfoSecondInterval,
		procInfoFirstIterations)

	return nil
}

// processInfoCollector collects the process tree for the current
// process and all spawned processes on a specified interval and sends
// these data to the API server. The interval is controlled by the
// arguments to this method, which allow for collection at one interval
// for a number of iterations and a second interval for all subsequent
// iterations. The intention of these intervals is to collect data with a
// high granularity after beginning to run a task and with a lower
// granularity throughout the life of a commit.
func (c *metricsCollector) processInfoCollector(ctx context.Context,
	firstInterval, secondInterval time.Duration, numFirstIterations int) {

	grip.Info("starting process metrics collector")

	count := 0
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.Info("process metrics collector terminated.")
			return
		case <-timer.C:
			msgs := message.CollectProcessInfoSelfWithChildren()
			_, err := c.comm.TryTaskPost("process_info", msgs)
			grip.CatchNotice(err)
			grip.DebugWhen(sometimes.Fifth(), msgs)

			if count <= numFirstIterations {
				timer.Reset(firstInterval)
			} else {
				timer.Reset(secondInterval)
			}

			count++
		}
	}
}

// sysInfoCollector collects aggregated system stats on the specified
// interval as long as the metrics collector is running, and sends these
// data to the API server.
func (c *metricsCollector) sysInfoCollector(ctx context.Context, interval time.Duration) {
	grip.Info("starting system metrics collector")
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.Info("system metrics collector terminated.")
			return
		case <-timer.C:
			msg := message.CollectSystemInfo()
			_, err := c.comm.TryTaskPost("system_info", msg)
			grip.CatchNotice(err)
			grip.DebugWhen(sometimes.Fifth(), msg)

			timer.Reset(interval)
		}
	}
}
