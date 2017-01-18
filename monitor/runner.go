package monitor

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/slogger"
)

type Runner struct{}

const (
	RunnerName  = "monitor"
	Description = "track and clean up expired hosts and tasks"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Starting monitor at time %v", startTime)

	if err := RunAllMonitoring(config); err != nil {
		err = fmt.Errorf("error running monitor: %v", err)
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Monitor took %v to run", runtime)
	return nil
}
