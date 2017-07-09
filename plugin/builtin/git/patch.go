package git

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// GitApplyPatchCommand is deprecated. Its functionality is now a part of GitGetProjectCommand.
type GitApplyPatchCommand struct{}

func (*GitApplyPatchCommand) Name() string                                    { return ApplyPatchCmdName }
func (*GitApplyPatchCommand) Plugin() string                                  { return GitPluginName }
func (*GitApplyPatchCommand) ParseParams(params map[string]interface{}) error { return nil }
func (*GitApplyPatchCommand) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Task().Warning("git.apply_patch is deprecated. Patches are applied in git.get_project.")
	return nil
}

// getPatchContents() dereferences any patch files that are stored externally, fetching them from
// the API server, and setting them into the patch object.
func (c GitGetProjectCommand) getPatchContents(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig, patch *patch.Patch) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	for i, patchPart := range patch.Patches {
		// If the patch isn't stored externally, no need to do anything.
		if patchPart.PatchSet.PatchFileId == "" {
			continue
		}

		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		// otherwise, fetch the contents and load it into the patch object
		logger.Execution().Infof("Fetching patch contents for %s", patchPart.PatchSet.PatchFileId)

		result, err := comm.GetPatchFile(ctx, td, patchPart.PatchSet.PatchFileId)
		if err != nil {
			return errors.Wrapf(err, "problem getting patch file")
		}

		patch.Patches[i].PatchSet.Patch = string(result)
	}
	return nil
}

// GetPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed. If the patch is empty it will not apply the patch.
func GetPatchCommands(modulePatch patch.ModulePatch, dir, patchPath string) []string {
	patchCommands := []string{
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("ls"),
		fmt.Sprintf("cd '%s'", dir),
		fmt.Sprintf("git reset --hard '%s'", modulePatch.Githash),
	}
	if modulePatch.PatchSet.Patch == "" {
		return patchCommands
	}
	return append(patchCommands, []string{
		fmt.Sprintf("git apply --check --whitespace=fix '%v'", patchPath),
		fmt.Sprintf("git apply --stat '%v'", patchPath),
		fmt.Sprintf("git apply --whitespace=fix < '%v'", patchPath),
	}...)
}

// applyPatch is used by the agent to copy patch data onto disk
// and then call the necessary git commands to apply the patch file
func (c *GitGetProjectCommand) applyPatch(ctx context.Context, logger client.LoggerProducer,
	conf *model.TaskConfig, p *patch.Patch) error {

	// patch sets and contain multiple patches, some of them for modules
	for _, patchPart := range p.Patches {
		if ctx.Err() != nil {
			return errors.New("apply patch operation canceled")
		}

		var dir string
		if patchPart.ModuleName == "" {
			// if patch is not part of a module, just apply patch against src root
			dir = c.Directory
			logger.Execution().Info("Applying patch with git...")
		} else {
			// if patch is part of a module, apply patch in module root
			module, err := conf.Project.GetModuleByName(patchPart.ModuleName)
			if err != nil {
				return errors.Wrap(err, "Error getting module")
			}
			if module == nil {
				return errors.Errorf("Module '%s' not found", patchPart.ModuleName)
			}

			// skip the module if this build variant does not use it
			if !util.SliceContains(conf.BuildVariant.Modules, module.Name) {
				logger.Execution().Infof(
					"Skipping patch for module %v: the current build variant does not use it",
					module.Name)
				continue
			}

			dir = filepath.Join(c.Directory, module.Prefix, module.Name)
			logger.Execution().Info("Applying module patch with git...")
		}

		// create a temporary folder and store patch files on disk,
		// for later use in shell script
		tempFile, err := ioutil.TempFile("", "mcipatch_")
		if err != nil {
			return errors.WithStack(err)
		}
		defer tempFile.Close()
		_, err = io.WriteString(tempFile, patchPart.PatchSet.Patch)
		if err != nil {
			return errors.WithStack(err)
		}
		tempAbsPath := tempFile.Name()

		// this applies the patch using the patch files in the temp directory
		patchCommandStrings := GetPatchCommands(patchPart, dir, tempAbsPath)
		cmdsJoined := strings.Join(patchCommandStrings, "\n")
		patchCmd := &subprocess.LocalCommand{
			CmdString:        cmdsJoined,
			WorkingDirectory: conf.WorkDir,
			Stdout:           logger.TaskWriter(level.Info),
			Stderr:           logger.TaskWriter(level.Error),
			ScriptMode:       true,
		}

		if err = patchCmd.Run(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
