package repotracker

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/slogger"
)

type Runner struct{}

const (
	RunnerName  = "repotracker"
	Description = "poll version control for new commits"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		err = fmt.Errorf("Error acquiring global lock: %+v", err)
		grip.Error(err)
		return err
	}

	if !lockAcquired {
		err = errors.New("Timed out acquiring global lock")
		grip.Error(err)
		return err
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Running repository tracker with db “%v”", config.Database.DB)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		err = fmt.Errorf("Error finding tracked projects %+v", err)
		grip.Error(err)
		return err
	}

	numNewRepoRevisionsToFetch := config.RepoTracker.NumNewRepoRevisionsToFetch
	if numNewRepoRevisionsToFetch <= 0 {
		numNewRepoRevisionsToFetch = DefaultNumNewRepoRevisionsToFetch
	}

	var wg sync.WaitGroup
	wg.Add(len(allProjects))
	for _, projectRef := range allProjects {
		go func(projectRef model.ProjectRef) {
			defer wg.Done()

			tracker := &RepoTracker{
				config,
				&projectRef,
				NewGithubRepositoryPoller(&projectRef, config.Credentials["github"]),
			}

			err = tracker.FetchRevisions(numNewRepoRevisionsToFetch)
			if err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error fetching revisions: %v", err)
			}
		}(projectRef)
	}
	wg.Wait()

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		err = fmt.Errorf("Error updating process status: %+v", err)
		grip.Error(err)
		return err
	}
	evergreen.Logger.Logf(slogger.INFO, "Repository tracker took %v to run", runtime)
	return nil
}
