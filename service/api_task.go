package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/bookkeeping"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskrunner"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// StartTask is the handler function that retrieves the task from the request
// and acquires the global lock
// With the lock, it marks associated tasks, builds, and versions as started.
// It then updates the host document with relevant information, including the pid
// of the agent, and ensures that the host has the running task field set.
func (as *APIServer) StartTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	if !getGlobalLock(r.RemoteAddr, t.Id, TaskStartCaller) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(r.RemoteAddr, t.Id, TaskStartCaller)

	grip.Infoln("Marking task started:", t.Id)

	taskStartInfo := &apimodels.TaskStartRequest{}
	if err := util.ReadJSONInto(r.Body, taskStartInfo); err != nil {
		http.Error(w, fmt.Sprintf("Error reading task start request for %v: %v", t.Id, err), http.StatusBadRequest)
		return
	}

	if err := model.MarkStart(t.Id); err != nil {
		message := errors.Wrapf(err, "Error marking task '%s' started", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	h, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		message := errors.Wrapf(err, "Error finding host running task %s", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if h == nil {
		message := errors.Errorf("No host found running task %v", t.Id)
		if t.HostId != "" {
			message = errors.Errorf("No host found running task %s but task is said to be running on %s",
				t.Id, t.HostId)
		}

		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if err := h.SetTaskPid(taskStartInfo.Pid); err != nil {
		message := errors.Wrapf(err, "Error calling set pid on task %s", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Task %v started on host %v", t.Id, h.Id))
}

// EndTask creates test results from the request and the project config.
// It then acquires the lock, and with it, marks tasks as finished or inactive if aborted.
// If the task is a patch, it will alert the users based on failures
// It also updates the expected task duration of the task for scheduling.
func (as *APIServer) EndTask(w http.ResponseWriter, r *http.Request) {
	finishTime := time.Now()
	taskEndResponse := &apimodels.TaskEndResponse{}

	t := MustHaveTask(r)

	details := &apimodels.TaskEndDetail{}
	if err := util.ReadJSONInto(r.Body, details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if details.Status != evergreen.TaskSucceeded &&
		details.Status != evergreen.TaskFailed &&
		details.Status != evergreen.TaskUndispatched {
		msg := errors.Errorf("Invalid end status '%v' for task %v", details.Status, t.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	projectRef, err := model.FindOneProjectRef(t.Project)

	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}

	if projectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, errors.New("empty projectRef for task"))
		return
	}

	project, err := model.FindProject("", projectRef)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.WithStack(err))
		return
	}

	if !getGlobalLock(r.RemoteAddr, t.Id, EndTaskCaller) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(r.RemoteAddr, t.Id, EndTaskCaller)

	// mark task as finished
	err = model.MarkEnd(t.Id, APIServerLockTitle, finishTime, details, project, projectRef.DeactivatePrevious)
	if err != nil {
		message := errors.Wrapf(err, "Error calling mark finish on task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if t.Requester != evergreen.PatchVersionRequester {
		grip.Infoln("Processing alert triggers for task", t.Id)
		grip.Error(errors.Wrapf(alerts.RunTaskFailureTriggers(t.Id),
			"processing alert triggers for task %s", t.Id))
	} //TODO(EVG-223) process patch-specific triggers

	// if task was aborted, reset to inactive
	if details.Status == evergreen.TaskUndispatched {
		if err = model.SetActiveState(t.Id, "", false); err != nil {
			message := fmt.Sprintf("Error deactivating task after abort: %v", err)
			grip.Error(message)
			taskEndResponse.Message = message
			as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
			return
		}

		as.taskFinished(w, t, finishTime)
		return
	}

	// update the bookkeeping entry for the task
	err = bookkeeping.UpdateExpectedDuration(t, t.TimeTaken)
	if err != nil {
		grip.Errorln("Error updating expected duration:", err.Error())
	}

	// log the task as finished
	grip.Infof("Successfully marked task %s as finished", t.Id)

	// construct and return the appropriate response for the agent
	as.taskFinished(w, t, finishTime)
}

// validateTaskEndDetails returns true if the task is finished or undispatched
func validateTaskEndDetails(details *apimodels.TaskEndDetail) bool {
	return details.Status == evergreen.TaskSucceeded ||
		details.Status == evergreen.TaskFailed ||
		details.Status == evergreen.TaskUndispatched
}

// TODO: are there any other reasons why agent would exit?
// checkHostHealth creates a task response that is sent back to the agent after the task ends.
func checkHostHealth(h *host.Host, resp *apimodels.EndTaskResponse) {
	if h.Status == evergreen.HostDecommissioned || h.Status == evergreen.HostQuarantined {
		resp.ShouldExit = true
		resp.Message = fmt.Sprintf("host %s is in state %s and agent should exit", h.Id, h.Status)
	}
}

// NewEndTask creates test results from the request and the project config.
// It then acquires the lock, and with it, marks tasks as finished or inactive if aborted.
// If the task is a patch, it will alert the users based on failures
// It also updates the expected task duration of the task for scheduling.
// NOTE this should eventually become the default code path.
func (as *APIServer) newEndTask(w http.ResponseWriter, r *http.Request) {
	finishTime := time.Now()

	t := MustHaveTask(r)
	currentHost := MustHaveHost(r)

	details := &apimodels.TaskEndDetail{}
	endTaskResp := &apimodels.EndTaskResponse{}
	if err := util.ReadJSONInto(r.Body, details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if !validateTaskEndDetails(details) {
		msg := fmt.Errorf("Invalid end status '%v' for task %v", details.Status, t.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	projectRef, err := model.FindOneProjectRef(t.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}
	if projectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("empty projectRef for task"))
		return
	}

	project, err := model.FindProject("", projectRef)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// clear the running task on the host now that the task has finished
	if err = currentHost.ClearRunningTask(t.Id, time.Now()); err != nil {
		message := fmt.Errorf("error clearing running task %s for host %s : %v", t.Id, currentHost.Id, err)
		grip.Errorf(message.Error())
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	// mark task as finished
	err = model.MarkEnd(t.Id, APIServerLockTitle, finishTime, details,
		project, projectRef.DeactivatePrevious)
	if err != nil {
		message := fmt.Errorf("Error calling mark finish on task %v : %v", t.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}
	// the task was aborted if it is still in undispatched.
	// the active state should be inactive.
	if details.Status == evergreen.TaskUndispatched {
		if t.Activated {
			grip.Warningf("task %v is active and undispatched after being marked as finished", t.Id)
			return
		}
		message := fmt.Sprintf("task %v has been aborted and will not run", t.Id)
		grip.Infof(message)
		endTaskResp = &apimodels.EndTaskResponse{
			ShouldExit: true,
			Message:    message,
		}
		as.WriteJSON(w, http.StatusOK, endTaskResp)
		return
	}

	// task cost calculations have no impact on task results, so do them in their own goroutine
	go as.updateTaskCost(t, currentHost, finishTime)

	if t.Requester != evergreen.PatchVersionRequester {
		grip.Infoln("Processing alert triggers for task", t.Id)
		err = alerts.RunTaskFailureTriggers(t.Id)
		grip.ErrorWhenf(err != nil, "processing alert triggers for task %s: %+v", t.Id, err)
	}
	// TODO(EVG-223) process patch-specific triggers

	// update the bookkeeping entry for the task
	err = bookkeeping.UpdateExpectedDuration(t, t.TimeTaken)
	if err != nil {
		grip.Errorln("Error updating expected duration:", err)
	}

	checkHostHealth(currentHost, endTaskResp)
	grip.Infof("Successfully marked task %s as finished", t.Id)
	as.WriteJSON(w, http.StatusOK, endTaskResp)

}

// taskFinished constructs the appropriate response for each markEnd
// request the API server receives from an agent. The two possible responses are:
// 1. Inform the agent of another task to run
// 2. Inform the agent that it should terminate immediately
// The first case is the usual expected flow. The second case however, could
// occur for a number of reasons including:
// a. The version of the agent running on the remote machine is stale
// b. The host the agent is running on has been decommissioned
// c. There is no currently queued dispatchable and activated task
// In any of these aforementioned cases, the agent in question should terminate
// immediately and cease running any tasks on its host.
func (as *APIServer) taskFinished(w http.ResponseWriter, t *task.Task, finishTime time.Time) {
	taskEndResponse := &apimodels.TaskEndResponse{}

	// a. fetch the host this task just completed on to see if it's
	// now decommissioned
	host, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		message := fmt.Sprintf("Error locating host for task %v - set to %v: %v", t.Id,
			t.HostId, err)
		grip.Error(message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host == nil {
		message := fmt.Sprintf("Error finding host running for task %v - set to %v", t.Id,
			t.HostId)
		grip.Error(message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.Status == evergreen.HostDecommissioned || host.Status == evergreen.HostQuarantined {
		markHostRunningTaskFinished(host, t, "")
		message := fmt.Sprintf("Host %v - running %v - is in state '%v'. Agent will terminate",
			t.HostId, t.Id, host.Status)
		grip.Info(message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// task cost calculations have no impact on task results, so do them in their own goroutine
	go as.updateTaskCost(t, host, finishTime)

	// b. check if the agent needs to be rebuilt
	taskRunnerInstance := taskrunner.NewTaskRunner(&as.Settings)
	agentRevision, err := taskRunnerInstance.HostGateway.GetAgentRevision()
	if err != nil {
		markHostRunningTaskFinished(host, t, "")
		grip.Errorln("failed to get agent revision:", err)
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.AgentRevision != agentRevision {
		markHostRunningTaskFinished(host, t, "")
		message := fmt.Sprintf("Remote agent needs to be rebuilt")
		grip.Error(message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// c. fetch the task's distro queue to dispatch the next pending task
	nextTask, err := getNextDistroTask(t, host)
	if err != nil {
		markHostRunningTaskFinished(host, t, "")
		grip.Error(err)
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}
	if nextTask == nil {
		markHostRunningTaskFinished(host, t, "")
		taskEndResponse.Message = "No next task on queue"
	} else {
		taskEndResponse.Message = "Proceed with next task"
		taskEndResponse.RunNext = true
		taskEndResponse.TaskId = nextTask.Id
		taskEndResponse.TaskSecret = nextTask.Secret
		markHostRunningTaskFinished(host, t, nextTask.Id)
	}

	// give the agent the green light to keep churning
	as.WriteJSON(w, http.StatusOK, taskEndResponse)
}

// getNextDistroTask fetches the next task to run for the given distro and marks
// the task as dispatched in the given host's document
func getNextDistroTask(currentTask *task.Task, host *host.Host) (
	nextTask *task.Task, err error) {
	taskQueue, err := model.FindTaskQueueForDistro(currentTask.DistroId)
	if err != nil {
		return nil, errors.Wrapf(err, "Error locating distro queue (%v) for task '%v'",
			currentTask.DistroId, currentTask.Id)
	}

	if taskQueue == nil {
		return nil, errors.Errorf("Nil task queue found for task '%v's distro "+
			"queue - '%v'", currentTask.Id, currentTask.DistroId)
	}

	// dispatch the next task for this host
	nextTask, err = taskrunner.DispatchTaskForHost(taskQueue, host)
	if err != nil {
		return nil, errors.Wrapf(err, "Error dequeuing task for host %v", host.Id)
	}
	if nextTask == nil {
		return nil, nil
	}
	return nextTask, nil
}

// updateTaskCost determines a task's cost based on the host it ran on. Hosts that
// are unable to calculate their own costs will not set a task's Cost field. Errors
// are logged but not returned, since any number of API failures could happen and
// we shouldn't sacrifice a task's status for them.
func (as *APIServer) updateTaskCost(t *task.Task, h *host.Host, finishTime time.Time) {
	manager, err := providers.GetCloudManager(h.Provider, &as.Settings)
	if err != nil {
		grip.Errorf("Error loading provider for host %s cost calculation: %+v", t.HostId, err)
		return
	}
	if calc, ok := manager.(cloud.CloudCostCalculator); ok {
		grip.Infoln("Calculating cost for task:", t.Id)
		cost, err := calc.CostForDuration(h, t.StartTime, finishTime)
		if err != nil {
			grip.Errorf("calculating cost for task %s: %+v ", t.Id, err)
			return
		}
		if err := t.SetCost(cost); err != nil {
			grip.Errorf("Error updating cost for task %s: %+v ", t.Id, err)
			return
		}
	}
}

// markHostRunningTaskFinished updates the running task field in the host document.
// TODO: this should be taken out when the task runner no longer assigns tasks to the agent. (EVG-1591)
func markHostRunningTaskFinished(h *host.Host, t *task.Task, newTaskId string) {
	// clear the running task instead
	if newTaskId == "" {
		err := h.ClearRunningTask(t.Id, time.Now())
		if err != nil {
			grip.Errorf("error clearing task %s on host %s : %+v", t.Id, h.Id, err)
			return
		}
	}
	// update the given host's running_task field accordingly
	if ok, err := h.UpdateRunningTask(t.Id, newTaskId, time.Now()); err != nil || !ok {
		grip.Errorf("%s on host %s to '': %+v", t.Id, h.Id, err)
	}
}

// assignNextAvailableTask gets the next task from the queue and sets the running task field
// of currentHost.
func assignNextAvailableTask(taskQueue *model.TaskQueue, currentHost *host.Host) (*task.Task, error) {
	if currentHost.RunningTask != "" {
		return nil, errors.Errorf("Error host %v must have an unset running task field but has running task %v",
			currentHost.Id, currentHost.RunningTask)
	}
	// only proceed if there are pending tasks left
	for !taskQueue.IsEmpty() {
		nextTaskId := taskQueue.NextTask().Id

		nextTask, err := task.FindOne(task.ById(nextTaskId))
		if err != nil {
			return nil, err
		}
		if nextTask == nil {
			return nil, errors.New("nil task on the queue")
		}

		// dequeue the task from the queue
		if err = taskQueue.DequeueTask(nextTask.Id); err != nil {
			return nil, errors.Wrapf(err,
				"error pulling task with id %v from queue for distro %v",
				nextTask.Id, nextTask.DistroId)
		}

		// validate that the task can be run, if not fetch the next one in
		// the queue.
		if !nextTask.IsDispatchable() {
			grip.Warningf("Skipping task %s, which was "+
				"picked up to be run but is not runnable - "+
				"status (%s) activated (%t)", nextTask.Id, nextTask.Status,
				nextTask.Activated)
			continue
		}
		// attempt to update the host. TODO: double check Last task completed thing...
		// TODO: get rid of last task completed field in update running task.
		ok, err := currentHost.UpdateRunningTask(currentHost.LastTaskCompleted, nextTaskId, time.Now())

		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !ok {
			continue
		}
		// TODO: figure out where task secret is in this.
		return nextTask, nil
	}
	return nil, nil

}

// NextTask retrieves the next task's id given the host name and host secret by retrieving the task queue
// and popping the next task off the task queue.
func (as *APIServer) NextTask(w http.ResponseWriter, r *http.Request) {
	h := MustHaveHost(r)
	response := apimodels.NextTaskResponse{
		ShouldExit: false,
	}
	// if there is already a task assigned to the host send back that task
	if h.RunningTask != "" {
		t, err := task.FindOne(task.ById(h.RunningTask))
		if err != nil {
			err = errors.WithStack(err)
			grip.Error(err)
			as.WriteJSON(w, http.StatusInternalServerError,
				errors.Wrapf(err, "error getting running task %s", h.RunningTask))
			return
		}

		// if the task can be dispatched and activated dispatch it
		if t.IsDispatchable() {
			err := errors.WithStack(model.MarkTaskDispatched(t, h.Id, h.Distro.Id))
			if err != nil {
				grip.Error(err)
				as.WriteJSON(w, http.StatusInternalServerError,
					errors.Wrapf(err, "error while marking task %s as dispatched for host %s", t.Id, h.Id))
				return
			}
		}
		// if the task is activated return that task
		if t.Activated {
			response.TaskId = t.Id
			response.TaskSecret = t.Secret
			as.WriteJSON(w, http.StatusOK, response)
			return
		}
		// the task is not activated so the host's running task should be unset
		// so it can retrieve a new task.
		if err := h.ClearRunningTask(h.LastTaskCompleted, time.Now()); err != nil {
			err = errors.WithStack(err)
			grip.Error(err)
			as.WriteJSON(w, http.StatusInternalServerError, err)
			return
		}

		// return an empty
		grip.Infof("Unset running task field for inactive task %s on host %s", t.Id, h.Id)
		as.WriteJSON(w, http.StatusOK, response)
		return
	}

	// retrieve the next task off the task queue and attempt to assign it to the host.
	// If there is already a host that has the task, it will error
	taskQueue, err := model.FindTaskQueueForDistro(h.Distro.Id)
	if err != nil {
		err = errors.Wrapf(err, "Error locating distro queue (%v) for host '%v'", h.Distro.Id, h.Id)
		grip.Error(err)
		as.WriteJSON(w, http.StatusBadRequest, err)
		return
	}
	if taskQueue == nil {
		err = errors.Errorf("Nil task queue found for task '%v's distro queue - '%v'",
			h.Id, h.Distro.Id)
		grip.Error(err)
		as.WriteJSON(w, http.StatusBadRequest, err)
		return
	}
	// assign the task to a host and retrieve the task
	nextTask, err := assignNextAvailableTask(taskQueue, h)
	if err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		as.WriteJSON(w, http.StatusBadRequest, err)
		return
	}
	if nextTask == nil {
		// if the task is empty, still send it with an status ok and check it on the other side
		grip.Infof("no task to assign host %v", h.Id)
		as.WriteJSON(w, http.StatusOK, response)
		return
	}

	// mark the task as dispatched
	if err := model.MarkTaskDispatched(nextTask, h.Id, h.Distro.Id); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		as.WriteJSON(w, http.StatusInternalServerError, err)
		return
	}
	response.TaskId = nextTask.Id
	response.TaskSecret = nextTask.Secret
	grip.Infof("assigned task %s to host %s", nextTask.Id, h.Id)
	as.WriteJSON(w, http.StatusOK, response)
}
