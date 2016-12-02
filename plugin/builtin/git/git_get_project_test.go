package git_test

import (
	"os"
	"path/filepath"
	"testing"

	slogger "github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/git"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func TestGitPlugin(t *testing.T) {
	testConfig := evergreen.TestConfig()
	Convey("With git plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, false)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)

		taskConfig, err := plugintest.CreateTestConfig(
			filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_clone.yml"),
			t)
		testutil.HandleTestingErr(err, t, "failed to create test config")
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agentutil.NewTestLogger(sliceAppender)
		Convey("all commands in test project should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &comm.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
				}
			}
			err = os.RemoveAll(taskConfig.WorkDir)
			testutil.HandleTestingErr(err, t, "Couldn't clean up test temp dir")
		})
	})
}
