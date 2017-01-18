package scheduler

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/slogger"
)

// Runner runs the scheduler process.
type Runner struct{}

const (
	RunnerName  = "scheduler"
	Description = "queue tasks for execution and allocate hosts"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Starting scheduler at time %v", startTime)

	schedulerInstance := &Scheduler{
		config,
		&DBTaskFinder{},
		&CmpBasedTaskPrioritizer{},
		&DBTaskDurationEstimator{},
		&DBTaskQueuePersister{},
		&DurationBasedHostAllocator{},
	}

	if err := schedulerInstance.Schedule(); err != nil {
		err = fmt.Errorf("Error running scheduler: %+v", err)
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Scheduler took %v to run", runtime)
	return nil
}
