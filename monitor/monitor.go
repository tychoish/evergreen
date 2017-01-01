package monitor

import (
	"fmt"
	"time"

	"github.com/tychoish/grip/slogger"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

var (
	// the functions the task monitor will run through to find tasks needing
	// to be cleaned up
	defaultTaskFlaggingFuncs = []taskFlaggingFunc{
		flagTimedOutHeartbeats,
	}

	// the functions the host monitor will run through to find hosts needing
	// to be terminated
	defaultHostFlaggingFuncs = []hostFlagger{
		{flagDecommissionedHosts, "decommissioned"},
		{flagUnreachableHosts, "unreachable"},
		{flagIdleHosts, "idle"},
		{flagExcessHosts, "excess"},
		{flagUnprovisionedHosts, "provision_timeout"},
		{flagProvisioningFailedHosts, "provision_failed"},
		{flagExpiredHosts, "expired"},
	}

	// the functions the host monitor will run through to do simpler checks
	defaultHostMonitoringFuncs = []hostMonitoringFunc{
		monitorReachability,
	}

	// the functions the notifier will use to build notifications that need
	// to be sent
	defaultNotificationBuilders = []notificationBuilder{
		spawnHostExpirationWarnings,
		slowProvisioningWarnings,
	}
)

// run all monitoring functions
func RunAllMonitoring(settings *evergreen.Settings) error {

	// load in all of the distros
	distros, err := distro.Find(db.Q{})
	if err != nil {
		return fmt.Errorf("error finding distros: %v", err)
	}

	// fetch the project refs, which we will use to get all of the projects
	projectRefs, err := model.FindAllProjectRefs()
	if err != nil {
		return fmt.Errorf("error loading in project refs: %v", err)
	}

	// turn the project refs into a map of the project id -> project
	projects := map[string]model.Project{}
	for _, ref := range projectRefs {
		// only monitor projects that are enabled
		if !ref.Enabled {
			continue
		}
		project, err := model.FindProject("", &ref)

		// continue on error to stop the whole monitoring process from
		// being held up
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "error finding project %v: %v",
				ref.Identifier, err)
			continue
		}

		if project == nil {
			evergreen.Logger.Logf(slogger.ERROR, "no project entry found for"+
				" ref %v", ref.Identifier)
			continue
		}

		projects[project.Identifier] = *project
	}

	// initialize the task monitor
	taskMonitor := &TaskMonitor{
		flaggingFuncs: defaultTaskFlaggingFuncs,
	}

	// clean up any necessary tasks
	errs := withGlobalLock("task cleanup",
		func() []error { return taskMonitor.CleanupTasks(projects) })
	for _, err := range errs {
		evergreen.Logger.Logf(slogger.ERROR, "Error cleaning up tasks: %v", err)
	}

	// initialize the host monitor
	hostMonitor := &HostMonitor{
		flaggingFuncs:   defaultHostFlaggingFuncs,
		monitoringFuncs: defaultHostMonitoringFuncs,
	}

	// clean up any necessary hosts
	errs = withGlobalLock("host cleanup",
		func() []error { return hostMonitor.CleanupHosts(distros, settings) })
	for _, err := range errs {
		evergreen.Logger.Logf(slogger.ERROR, "Error cleaning up hosts: %v", err)
	}

	// run monitoring checks
	errs = withGlobalLock("host monitoring",
		func() []error { return hostMonitor.RunMonitoringChecks(settings) })
	for _, err := range errs {
		evergreen.Logger.Logf(slogger.ERROR, "Error running host monitoring checks: %v", err)
	}

	// initialize the notifier
	notifier := &Notifier{
		notificationBuilders: defaultNotificationBuilders,
	}

	// send notifications
	errs = notifier.Notify(settings)
	for _, err := range errs {
		evergreen.Logger.Logf(slogger.ERROR, "Error sending notifications: %v", err)
	}

	// Do alerts for spawnhosts - collect all hosts expiring in the next 12 hours.
	// The trigger logic will filter out any hosts that aren't in a notification window, or have
	// already have alerts sent.
	now := time.Now()
	thresholdTime := now.Add(12 * time.Hour)
	expiringSoonHosts, err := host.Find(host.ByExpiringBetween(now, thresholdTime))
	if err != nil {
		return err
	}

	for _, h := range expiringSoonHosts {
		err := alerts.RunSpawnWarningTriggers(&h)

		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error queueing alert: %v", err)
		}
	}

	return nil

}

// withGlobalLock is a wrapper for grabbing the global lock for each segment of the monitor.
func withGlobalLock(name string, f func() []error) (errs []error) {
	evergreen.Logger.Logf(slogger.DEBUG, "Attempting to acquire global lock for monitor %v", name)
	// sleep for 1 second to give other spinning locks a chance to preempt this one
	time.Sleep(time.Second)
	acquired, err := db.WaitTillAcquireGlobalLock(name, db.LockTimeout)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error acquiring global lock for monitor", err)
		return []error{fmt.Errorf("error acquiring global lock for %v: %v", name, err)}
	}
	if !acquired {
		evergreen.Logger.Errorf(slogger.ERROR,
			"Timed out attempting to acquire global lock for monitor %v", name)
		return []error{fmt.Errorf("timed out acquiring global lock for monitor %v", name)}
	}
	defer func() {
		evergreen.Logger.Logf(slogger.DEBUG, "Releasing global lock for monitor %v", name)
		if err := db.ReleaseGlobalLock(name); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR,
				"Error releasing global lock for monitor %v: %v", name, err)
			errs = append(errs, fmt.Errorf("error releasing global lock for monitor %v: %v", name, err))
		} else {
			evergreen.Logger.Logf(slogger.DEBUG, "Released global lock for monitor %v", name)
		}
	}()
	evergreen.Logger.Logf(slogger.DEBUG, "Acquired global lock for monitor %v", name)
	errs = f()
	return errs
}
