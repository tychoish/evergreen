package taskrunner

import (
	"time"

	"github.com/tychoish/grip/slogger"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
)

type Runner struct{}

const (
	RunnerName  = "taskrunner"
	Description = "run queued tasks on available hosts"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Starting taskrunner at time %v", startTime)
	if err := NewTaskRunner(config).Run(); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "error running taskrunner: %v", err)
	}
	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Taskrunner took %v to run", runtime)
	return nil
}
