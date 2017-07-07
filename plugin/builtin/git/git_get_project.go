package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// GitGetProjectCommand is a command that fetches source code from git for the project
// associated with the current task
type GitGetProjectCommand struct {
	// The root directory (locally) that the code should be checked out into.
	// Must be a valid non-blank directory name.
	Directory string `plugin:"expand"`

	// Revisions are the optional revisions associated with the modules of a project.
	// Note: If a module does not have a revision it will use the module's branch to get the project.
	Revisions map[string]string `plugin:"expand"`
}

func (c *GitGetProjectCommand) Name() string {
	return GetProjectCmdName
}

func (c *GitGetProjectCommand) Plugin() string {
	return GitPluginName
}

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (c *GitGetProjectCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return err
	}

	if c.Directory == "" {
		return errors.Errorf("error parsing '%v' params: value for directory "+
			"must not be blank", c.Name())
	}
	return nil
}

// Execute gets the source code required by the project
func (c *GitGetProjectCommand) Execute(ctx context.Context, client client.Communicator, conf *model.TaskConfig) error {

	// expand the github parameters before running the task
	if err := plugin.ExpandValues(c, conf.Expansions); err != nil {
		return err
	}

	location, err := conf.ProjectRef.Location()
	if err != nil {
		return err
	}

	logger := client.GetLoggerProducer(conf.Task.Id, conf.Task.Secret)

	gitCommands := []string{
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("rm -rf %s", c.Directory),
	}

	cloneCmd := fmt.Sprintf("git clone '%s' '%s'", location, c.Directory)
	if conf.ProjectRef.Branch != "" {
		cloneCmd = fmt.Sprintf("%s --branch '%s'", cloneCmd, conf.ProjectRef.Branch)
	}

	gitCommands = append(gitCommands,
		cloneCmd,
		fmt.Sprintf("cd %v; git reset --hard %s", c.Directory, conf.Task.Revision))

	cmdsJoined := strings.Join(gitCommands, "\n")

	fetchSourceCmd := &subprocess.LocalCommand{
		CmdString:        cmdsJoined,
		WorkingDirectory: conf.WorkDir,
		Stdout:           logger.TaskWriter(level.Info),
		Stderr:           logger.TaskWriter(level.Error),
		ScriptMode:       true,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errChan := make(chan error)
	go func() {
		logger.Execution().Info("Fetching source from git...")
		errChan <- fetchSourceCmd.Run()
	}()

	// wait until the command finishes or the stop channel is tripped
	select {
	case err := <-errChan:
		if err != nil {
			return errors.WithStack(err)
		}
	case <-ctx.Done():
		logger.Execution().Info("Got kill signal during git.get_project command")
		if fetchSourceCmd.Cmd != nil {
			logger.Execution().Infof("Stopping process: %d", fetchSourceCmd.Cmd.Process.Pid)
			if err := fetchSourceCmd.Stop(); err != nil {
				logger.Execution().Error("Error occurred stopping process: %v", err)
			}
		}
		return errors.New("Fetch command interrupted")
	}

	// Fetch source for the modules
	for _, moduleName := range conf.BuildVariant.Modules {
		if ctx.Err() != nil {
			return errors.New("git.get_project command aborted while applying modules")
		}
		logger.Execution().Infof("Fetching module: %s", moduleName)

		module, err := conf.Project.GetModuleByName(moduleName)
		if err != nil {
			logger.Execution().Errorf("Couldn't get module %s: %v", moduleName, err)
			continue
		}
		if module == nil {
			logger.Execution().Errorf("No module found for %s", moduleName)
			continue
		}

		moduleBase := filepath.Join(module.Prefix, module.Name)
		moduleDir := filepath.Join(conf.WorkDir, moduleBase, "/_")

		err = os.MkdirAll(moduleDir, 0755)
		if err != nil {
			return errors.WithStack(err)
		}
		// clear the destination
		err = os.RemoveAll(moduleDir)
		if err != nil {
			return errors.WithStack(err)
		}

		revision := c.Revisions[moduleName]

		// if there is no revision, then use the revision from the module, then branch name
		if revision == "" {
			if module.Ref != "" {
				revision = module.Ref
			} else {
				revision = module.Branch
			}
		}

		moduleCmds := []string{
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("set -o verbose"),
			fmt.Sprintf("git clone %v '%v'", module.Repo, filepath.ToSlash(moduleBase)),
			fmt.Sprintf("cd %v; git checkout '%v'", filepath.ToSlash(moduleBase), revision),
		}

		moduleFetchCmd := &subprocess.LocalCommand{
			CmdString:        strings.Join(moduleCmds, "\n"),
			WorkingDirectory: filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory)),
			Stdout:           logger.TaskWriter(level.Info),
			Stderr:           logger.TaskWriter(level.Error),
			ScriptMode:       true,
		}

		ctx, cancel := context.WithCancel(context.TODO())
		go func() {
			errChan <- moduleFetchCmd.Run()
		}()

		// wait until the command finishes or the stop channel is tripped
		select {
		case err := <-errChan:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			logger.Execution().Info("Got kill signal")
			if moduleFetchCmd.Cmd != nil {
				logger.Execution().Infof("Stopping process: %d", moduleFetchCmd.Cmd.Process.Pid)
				if err := moduleFetchCmd.Stop(); err != nil {
					logger.Execution().Errorf("Error occurred stopping process: %v", err)
				}
			}
			return errors.New("Fetch module command interrupted")
		}
	}

	//Apply patches if necessary
	if conf.Task.Requester != evergreen.PatchVersionRequester {
		return nil
	}

	go func() {
		logger.Execution().Info("Fetching patch.")
		patch, err := client.GetTaskPatch(ctx, conf.Task.Id, conf.Task.Secret)
		if err != nil {
			logger.Execution().Errorf("Failed to get patch: %v", err)
			errChan <- errors.Wrap(err, "Failed to get patch")
		}
		err = c.getPatchContents(ctx, client, logger, conf, patch)
		if err != nil {
			logger.Execution().Errorf("Failed to get patch contents: %v", err)
			errChan <- errors.Wrap(err, "Failed to get patch contents")
		}
		err = c.applyPatch(ctx, conf, patch, logger)
		if err != nil {
			logger.Execution().Infof("Failed to apply patch: %v", err)
			errChan <- errors.Wrap(err, "Failed to apply patch")
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		return errors.New("Patch command interrupted")
	}
}
