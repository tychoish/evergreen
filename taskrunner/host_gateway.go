package taskrunner

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	MakeShellTimeout  = 2 * time.Minute
	SCPTimeout        = 3 * time.Minute
	StartAgentTimeout = 2 * time.Minute
	agentFile         = "agent"
	pidFile           = ".pid"
)

// HostGateway is responsible for kicking off tasks on remote machines.
type HostGateway interface {
	// run the specified task on the specified host, return the revision of the
	// agent running the task on that host
	RunTaskOnHost(*evergreen.Settings, task.Task, host.Host) (string, error)
	// gets the current revision of the agent
	GetAgentRevision() (string, error)
}

// Implementation of the HostGateway that builds and copies over the MCI
// agent to run tasks.
type AgentHostGateway struct {
	// Destination directory for the agent executables
	ExecutablesDir string
	// Internal cache of the agent package's current git hash
	currentAgentHash string
}

// Start the task specified, on the host specified.  First runs any necessary
// preparation on the remote machine, then kicks off the agent process on the
// machine.
// Returns an error if any step along the way fails.
func (agbh *AgentHostGateway) RunTaskOnHost(settings *evergreen.Settings, taskToRun task.Task, hostObj host.Host) (string, error) {

	// get the host's SSH options
	cloudHost, err := providers.GetCloudHost(&hostObj, settings)
	if err != nil {
		return "", errors.Errorf("Failed to get cloud host for %v: %v", hostObj.Id, err)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return "", errors.Errorf("Error getting ssh options for host %v: %v", hostObj.Id, err)
	}

	// prep the remote host
	grip.Infof("Prepping remote host %v...", hostObj.Id)
	agentRevision, err := agbh.prepRemoteHost(hostObj, sshOptions)
	if err != nil {
		return "", errors.Errorf("error prepping remote host %v: %v", hostObj.Id, err)
	}
	grip.Infof("Prepping host %v finished successfully", hostObj.Id)

	// start the agent on the remote machine
	grip.Infof("Starting agent on host %v for task %v...", hostObj.Id, taskToRun.Id)

	// generate the host secret if none exists
	if hostObj.Secret == "" {
		if err := hostObj.CreateSecret(); err != nil {
			return "", errors.Errorf("creating secret for %v: %v", hostObj.Id, err)
		}
	}

	err = startAgentOnRemote(settings.ApiUrl, &taskToRun, &hostObj, sshOptions)
	if err != nil {
		return "", err
	}
	grip.Infof("Agent successfully started for task %v", taskToRun.Id)

	return agentRevision, nil
}

// Gets the git revision of the currently built agent
func (agbh *AgentHostGateway) GetAgentRevision() (string, error) {

	versionFile := filepath.Join(agbh.ExecutablesDir, "version")
	hashBytes, err := ioutil.ReadFile(versionFile)
	if err != nil {
		return "", errors.Errorf("error reading agent version file: %v", err)
	}

	return strings.TrimSpace(string(hashBytes)), nil
}

// executableSubPath returns the directory containing the compiled agents.
func executableSubPath(id string) (string, error) {

	// get the full distro info, so we can figure out the architecture
	d, err := distro.FindOne(distro.ById(id))
	if err != nil {
		return "", errors.Errorf("error finding distro %v: %v", id, err)
	}

	mainName := "main"
	if strings.HasPrefix(d.Arch, "windows") {
		mainName = "main.exe"
	}

	return filepath.Join(d.Arch, mainName), nil
}

func newCappedOutputLog() *util.CappedWriter {
	// store up to 1MB of streamed command output to print if a command fails
	return &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
}

// Prepare the remote machine to run a task.
// This consists of:
// 1. Creating the directories on the remote host where, according to the distro's settings,
//    the agent should be placed.
// 2. Copying the agent into that directory.
func (agbh *AgentHostGateway) prepRemoteHost(hostObj host.Host, sshOptions []string) (string, error) {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return "", errors.Errorf("error parsing ssh info %v: %v", hostObj.Host, err)
	}

	// first, create the necessary sandbox of directories on the remote machine
	mkdirOutput := newCappedOutputLog()
	makeShellCmd := &command.RemoteCommand{
		Id:             fmt.Sprintf("agent_mkdir-%v", rand.Int()),
		CmdString:      fmt.Sprintf("mkdir -m 777 -p %v", hostObj.Distro.WorkDir),
		Stdout:         mkdirOutput,
		Stderr:         mkdirOutput,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     false,
	}

	grip.Infof("Directories command: '%#v'", makeShellCmd)

	// run the make shell command with a timeout
	err = util.RunFunctionWithTimeout(makeShellCmd.Run, MakeShellTimeout)
	if err != nil {
		// if it timed out, kill the command
		if err == util.ErrTimedOut {
			makeShellCmd.Stop()
			return "", errors.Errorf("creating remote directories timed out: %v", mkdirOutput.String())
		}
		return "", errors.Errorf(
			"error creating directories on remote machine (%v): %v", err, mkdirOutput.String())
	}

	// third, copy over the correct agent binary to the remote machine
	execSubPath, err := executableSubPath(hostObj.Distro.Id)
	if err != nil {
		return "", errors.Errorf("error computing subpath to executable: %v", err)
	}

	scpAgentOutput := newCappedOutputLog()
	scpAgentCmd := &command.ScpCommand{
		Id:             fmt.Sprintf("scp%v", rand.Int()),
		Source:         filepath.Join(agbh.ExecutablesDir, execSubPath),
		Dest:           hostObj.Distro.WorkDir,
		Stdout:         scpAgentOutput,
		Stderr:         scpAgentOutput,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-P", hostInfo.Port}, sshOptions...),
	}

	// get the agent's revision before scp'ing over the executable
	preSCPAgentRevision, err := agbh.GetAgentRevision()
	grip.ErrorWhenln(err != nil, "Error getting pre scp agent revision:", err)

	// run the command to scp the agent with a timeout
	err = util.RunFunctionWithTimeout(scpAgentCmd.Run, SCPTimeout)
	if err != nil {
		if err == util.ErrTimedOut {
			scpAgentCmd.Stop()
			return "", errors.Errorf("scp-ing agent binary timed out: %v", scpAgentOutput.String())
		}
		return "", errors.Errorf(
			"error copying agent binary to remote machine (%v): %v", err, scpAgentOutput.String())
	}

	// get the agent's revision after scp'ing over the executable
	postSCPAgentRevision, err := agbh.GetAgentRevision()
	grip.ErrorWhenln(err != nil, "Error getting post scp agent revision:", err)

	grip.WarningWhenf(preSCPAgentRevision != postSCPAgentRevision,
		"Agent revision was %v before scp but is now %v. Using previous revision %v for host %v",
		preSCPAgentRevision, postSCPAgentRevision, preSCPAgentRevision, hostObj.Id)

	return preSCPAgentRevision, nil
}

// Start the agent process on the specified remote host, and have it run the specified task.
func startAgentOnRemote(apiURL string, task *task.Task, hostObj *host.Host, sshOptions []string) error {
	// the path to the agent binary on the remote machine
	pathToExecutable := filepath.Join(hostObj.Distro.WorkDir, "main")

	// build the command to run on the remote machine
	remoteCmd := fmt.Sprintf(
		`%v -api_server "%v" -task_id "%v" -task_secret "%v" -host_id "%v" -host_secret "%v" -log_prefix "%v" -https_cert "%v"`,
		pathToExecutable, apiURL, task.Id, task.Secret, hostObj.Id, hostObj.Secret,
		filepath.Join(hostObj.Distro.WorkDir, agentFile), "")
	grip.Info(remoteCmd)

	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return errors.Errorf("error parsing ssh info %v: %v", hostObj.Host, err)
	}

	// run the command to kick off the agent remotely
	var startAgentLog bytes.Buffer
	startAgentCmd := &command.RemoteCommand{
		Id:             fmt.Sprintf("startagent-%v", rand.Int()),
		CmdString:      remoteCmd,
		Stdout:         &startAgentLog,
		Stderr:         &startAgentLog,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     true,
	}

	// run the command to start the agent with a timeout
	err = util.RunFunctionWithTimeout(
		startAgentCmd.Run,
		StartAgentTimeout,
	)
	if err != nil {
		if err == util.ErrTimedOut {
			startAgentCmd.Stop()
			return errors.Errorf("starting agent timed out")
		}
		return errors.Errorf("error starting agent (%v): %v", hostObj.Id, err, startAgentLog.String())
	}

	return nil
}
