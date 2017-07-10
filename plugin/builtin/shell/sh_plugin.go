package shell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func init() {
	plugin.Publish(&ShellPlugin{})
}

const (
	ShellPluginName = "shell"
	ShellExecCmd    = "exec"
	CleanupCmd      = "cleanup"
	TrackCmd        = "track"
)

// ShellPlugin runs arbitrary shell code on the agent's machine.
type ShellPlugin struct{}

// Name returns the name of the plugin. Required to fulfill
// the Plugin interface.
func (sp *ShellPlugin) Name() string {
	return ShellPluginName
}

// NewCommand returns the requested command, or returns an error
// if a non-existing command is requested.
func (sp *ShellPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == TrackCmd {
		return &TrackCommand{}, nil
	} else if cmdName == CleanupCmd {
		return &CleanupCommand{}, nil
	} else if cmdName == ShellExecCmd {
		return &ShellExecCommand{}, nil
	}
	return nil, errors.Errorf("no such command: %v", cmdName)
}

type TrackCommand struct{}

func (cc *TrackCommand) Name() string {
	return TrackCmd
}

func (cc *TrackCommand) Plugin() string {
	return ShellPluginName
}

func (cc *TrackCommand) ParseParams(params map[string]interface{}) error {
	return nil
}

// Execute starts the shell with its given parameters.
func (cc *TrackCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Warning("shell.track is deprecated. Process tracking is now enabled by default.")
	return nil
}

type CleanupCommand struct{}

func (cc *CleanupCommand) Name() string                                    { return CleanupCmd }
func (cc *CleanupCommand) Plugin() string                                  { return ShellPluginName }
func (cc *CleanupCommand) ParseParams(params map[string]interface{}) error { return nil }

// Execute starts the shell with its given parameters.
func (cc *CleanupCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Warning("shell.cleanup is deprecated. Process cleanup is now enabled by default.")
	return nil
}

// ShellExecCommand is responsible for running the shell code.
type ShellExecCommand struct {
	// Script is the shell code to be run on the agent machine.
	Script string `mapstructure:"script" plugin:"expand"`

	// Silent, if set to true, prevents shell code/output from being
	// logged to the agent's task logs. This can be used to avoid
	// exposing sensitive expansion parameters and keys.
	Silent bool `mapstructure:"silent"`

	// Shell describes the shell to execute the script contents
	// with. Defaults to "sh", but users can customize to
	// explicitly specify another shell.
	Shell string `mapstructure:"shell"`

	// Background, if set to true, prevents shell code/output from
	// waiting for the script to complete and immediately returns
	// to the caller
	Background bool `mapstructure:"background"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// SystemLog if set will write the shell command's output to the system logs, instead of the
	// task logs. This can be used to collect diagnostic data in the background of a running task.
	SystemLog bool `mapstructure:"system_log"`

	// ContinueOnError determines whether or not a failed return code
	// should cause the task to be marked as failed. Setting this to true
	// allows following commands to execute even if this shell command fails.
	ContinueOnError bool `mapstructure:"continue_on_err"`
}

func (_ *ShellExecCommand) Name() string   { return ShellExecCmd }
func (_ *ShellExecCommand) Plugin() string { return ShellPluginName }

// ParseParams reads in the command's parameters.
func (sec *ShellExecCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, sec)
	if err != nil {
		return errors.Wrapf(err, "error decoding %v params", sec.Name())
	}
	return nil
}

// Execute starts the shell with its given parameters.
func (sec *ShellExecCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Debug("Preparing script...")

	var logWriterInfo io.Writer
	var logWriterErr io.Writer

	if sec.SystemLog {
		logWriterInfo = logger.SystemWriter(level.Info)
		logWriterErr = logger.SystemWriter(level.Error)
	} else {
		logWriterInfo = logger.TaskWriter(level.Info)
		logWriterErr = logger.TaskWriter(level.Error)
	}

	localCmd := &subprocess.LocalCommand{
		CmdString:  sec.Script,
		Stdout:     logWriterInfo,
		Stderr:     logWriterErr,
		ScriptMode: true,
	}

	if sec.WorkingDir != "" {
		localCmd.WorkingDirectory = filepath.Join(conf.WorkDir, sec.WorkingDir)
	} else {
		localCmd.WorkingDirectory = conf.WorkDir
	}

	if sec.Shell != "" {
		localCmd.Shell = sec.Shell
	}

	err := localCmd.PrepToRun(conf.Expansions)
	if err != nil {
		return errors.Wrap(err, "Failed to apply expansions")
	}

	if sec.Silent {
		logger.Execution().Infof("Executing script with %s (source hidden)...",
			localCmd.Shell)
	} else {
		logger.Execution().Infof("Executing script with %s: %v",
			localCmd.Shell, localCmd.CmdString)
	}

	doneStatus := make(chan error)
	go func() {
		var err error
		env := os.Environ()
		env = append(env, fmt.Sprintf("EVR_TASK_ID=%v", conf.Task.Id), fmt.Sprintf("EVR_AGENT_PID=%v", os.Getpid()))
		localCmd.Environment = env
		err = localCmd.Start()

		if err != nil {
			logger.System().Debugf("error spawning shell process: %v", err)
		} else {
			logger.System().Debugf("spawned shell process with pid %d", localCmd.Cmd.Process.Pid)

			// Call the platform's process-tracking function. On some OSes this will be a noop,
			// on others this may need to do some additional work to track the process so that
			// it can be cleaned up later.
			trackProcess(conf.Task.Id, localCmd.Cmd.Process.Pid, logger)

			if sec.Background {
				logger.Execution().Debug("running command in the background")
				close(doneStatus)
			} else {
				select {
				case doneStatus <- localCmd.Cmd.Wait():
					logger.System().Debugf("shell process %d completed", localCmd.Cmd.Process.Pid)
				case <-ctx.Done():
					doneStatus <- localCmd.Stop()
					logger.System().Infof("shell process %d terminated", localCmd.Cmd.Process.Pid)
				}
			}
		}
	}()

	select {
	case err = <-doneStatus:
		if err != nil {
			if sec.ContinueOnError {
				logger.Execution().Infof("(ignoring) Script finished with error: %v", err)
				return nil
			}

			err = errors.Wrap(err, "script finished with error")
			logger.Execution().Info(err)
			return err
		}

		logger.Execution().Info("Script execution complete.")
	case <-ctx.Done():
		logger.Execution().Info("Got kill signal")

		// need to check command has started
		if localCmd.Cmd != nil {
			logger.Execution().Infof("Stopping process: %d", localCmd.Cmd.Process.Pid)

			// try and stop the process
			if err := localCmd.Stop(); err != nil {
				logger.Execution().Error(errors.Wrap(err, "error while stopping process"))
			}
		}

		return errors.New("shell command interrupted")
	}

	return nil
}

// envHasMarkers returns a bool indicating if both marker vars are found in an environment var list
func envHasMarkers(env []string, pidMarker, taskMarker string) bool {
	hasPidMarker := false
	hasTaskMarker := false
	for _, envVar := range env {
		if envVar == pidMarker {
			hasPidMarker = true
		}
		if envVar == taskMarker {
			hasTaskMarker = true
		}
	}
	return hasPidMarker && hasTaskMarker
}

// KillSpawnedProcs cleans up any tasks that were spawned by the given task.
func KillSpawnedProcs(taskId string, logger grip.Journaler) error {
	// Clean up all shell processes spawned during the execution of this task by this agent,
	// by calling the platform-specific "cleanup" function
	return cleanup(taskId, logger)
}
