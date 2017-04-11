package plugin_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/expansions"
	"github.com/evergreen-ci/evergreen/plugin/builtin/shell"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/yaml.v2"
)

type MockPlugin struct{}

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func MockPluginEcho(w http.ResponseWriter, request *http.Request) {
	arg1 := mux.Vars(request)["param1"]
	arg2, err := strconv.Atoi(mux.Vars(request)["param2"])
	if err != nil {
		http.Error(w, "bad val for param2", http.StatusBadRequest)
		return
	}

	newTask := plugin.GetTask(request)
	if newTask != nil {
		//task should have been populated for us, by the API server
		plugin.WriteJSON(w, http.StatusOK, map[string]string{
			"echo": fmt.Sprintf("%v/%v/%v", arg1, arg2, newTask.Id),
		})
		return
	}
	http.Error(w, "couldn't get task from context", http.StatusInternalServerError)
}

func (mp *MockPlugin) Configure(conf map[string]interface{}) error {
	return nil
}

func (mp *MockPlugin) GetAPIHandler() http.Handler {
	r := mux.NewRouter()
	r.Path("/blah/{param1}/{param2}").Methods("GET").HandlerFunc(MockPluginEcho)
	return r
}

func (mp *MockPlugin) GetUIHandler() http.Handler {
	return nil
}

func (mp *MockPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return nil, nil
}

func (mp *MockPlugin) Name() string {
	return "mock"
}

func (mp *MockPlugin) NewCommand(commandName string) (plugin.Command, error) {
	if commandName != "foo" {
		return nil, &plugin.ErrUnknownCommand{commandName}
	}
	return &MockCommand{}, nil
}

type MockCommand struct {
	Param1 string
	Param2 int64
}

func (mc *MockCommand) Name() string {
	return "mock"
}

func (mc *MockCommand) Plugin() string {
	return "mock"
}

func (mc *MockCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, mc)
	if err != nil {
		return err
	}
	if mc.Param1 == "" {
		return errors.New("Param1 must be a non-blank string.")
	}
	if mc.Param2 == 0 {
		return errors.New("Param2 must be a non-zero integer.")
	}
	return nil
}

func (mc *MockCommand) Execute(logger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	resp, err := pluginCom.TaskGetJSON(fmt.Sprintf("blah/%s/%d", mc.Param1, mc.Param2))
	if err != nil {
		return err
	}

	if resp != nil {
		defer resp.Body.Close()
	}

	if resp == nil {
		return errors.New("Received nil HTTP response from api server")
	}

	jsonReply := map[string]string{}
	err = util.ReadJSONInto(resp.Body, &jsonReply)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("Got bad status code from API response: %v, body: %v", resp.StatusCode, jsonReply)
	}

	expectedEchoReply := fmt.Sprintf("%v/%v/%v", mc.Param1, mc.Param2, conf.Task.Id)
	if jsonReply["echo"] != expectedEchoReply {
		return errors.Errorf("Wrong echo reply! Wanted %v, got %v", expectedEchoReply, jsonReply["echo"])
	}
	return nil
}

func TestRegistry(t *testing.T) {
	Convey("With a SimpleRegistry", t, func() {
		Convey("Registering a plugin twice should return err", func() {
			registry := plugin.NewSimpleRegistry()
			err := registry.Register(&MockPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&shell.ShellPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&expansions.ExpansionsPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")
		})
		Convey("with a project file containing references to a valid plugin", func() {
			registry := plugin.NewSimpleRegistry()
			So(registry.Register(&MockPlugin{}), ShouldBeNil)
			So(registry.Register(&shell.ShellPlugin{}), ShouldBeNil)
			So(registry.Register(&expansions.ExpansionsPlugin{}), ShouldBeNil)

			data, err := ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(),
				"testdata", "plugin_project.yml"))
			testutil.HandleTestingErr(err, t, "failed to load test yaml file")
			project := &model.Project{}
			So(yaml.Unmarshal(data, project), ShouldBeNil)
			Convey("all commands in project file should load parse successfully", func() {
				for _, newTask := range project.Tasks {
					for _, command := range newTask.Commands {
						pluginCmds, err := registry.GetCommands(command, project.Functions)
						testutil.HandleTestingErr(err, t, "Got error getting plugin commands: %v")
						So(pluginCmds, ShouldNotBeNil)
						So(err, ShouldBeNil)
					}
				}
			})
		})
	})
}

func TestPluginFunctions(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchTask")
	Convey("With a SimpleRegistry", t, func() {
		Convey("with a project file containing functions", func() {
			registry := plugin.NewSimpleRegistry()
			err := registry.Register(&shell.ShellPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&expansions.ExpansionsPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")

			testServer, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
			defer testServer.Close()

			taskConfig, err := createTestConfig(filepath.Join(testutil.GetDirectoryOfFile(),
				"testdata", "plugin_project_functions.yml"), t)
			testutil.HandleTestingErr(err, t, "failed to create test config: %v", err)

			Convey("all commands in project file should parse successfully", func() {
				for _, newTask := range taskConfig.Project.Tasks {
					for _, command := range newTask.Commands {
						pluginCmd, err := registry.GetCommands(command, taskConfig.Project.Functions)
						testutil.HandleTestingErr(err, t, "Got error getting plugin command: %v")
						So(pluginCmd, ShouldNotBeNil)
						So(err, ShouldBeNil)
					}
				}
			})

			httpCom, err := comm.NewHTTPCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "", "", "", nil)
			So(err, ShouldBeNil)
			So(httpCom, ShouldNotBeNil)

			Convey("all commands in test project should execute successfully", func() {
				logger := agentutil.NewTestLogger(slogger.StdOutAppender())
				for _, newTask := range taskConfig.Project.Tasks {
					So(len(newTask.Commands), ShouldNotEqual, 0)
					for _, command := range newTask.Commands {
						pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
						testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
						So(pluginCmds, ShouldNotBeNil)
						So(err, ShouldBeNil)
						So(len(pluginCmds), ShouldEqual, 1)
						cmd := pluginCmds[0]
						pluginCom := &comm.TaskJSONCommunicator{cmd.Plugin(), httpCom}
						err = cmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
						So(err, ShouldBeNil)
					}
				}
			})
		})
	})
}

func TestPluginExecution(t *testing.T) {
	Convey("With a SimpleRegistry and test project file", t, func() {
		registry := plugin.NewSimpleRegistry()

		plugins := []plugin.CommandPlugin{&MockPlugin{}, &expansions.ExpansionsPlugin{}, &shell.ShellPlugin{}}
		apiPlugins := []plugin.APIPlugin{&MockPlugin{}, &expansions.ExpansionsPlugin{}}
		for _, p := range plugins {
			err := registry.Register(p)
			testutil.HandleTestingErr(err, t, "failed to register plugin")
		}

		testServer, err := service.CreateTestServer(testutil.TestConfig(), nil, apiPlugins)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		defer testServer.Close()

		httpCom, err := comm.NewHTTPCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "", "", "", nil)
		So(err, ShouldBeNil)
		So(httpCom, ShouldNotBeNil)

		pluginConfigPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_project.yml")
		taskConfig, err := createTestConfig(pluginConfigPath, t)
		testutil.HandleTestingErr(err, t, "failed to create test config: %v", err)

		logger := agentutil.NewTestLogger(slogger.StdOutAppender())

		Convey("all commands in test project should execute successfully", func() {
			for _, newTask := range taskConfig.Project.Tasks {
				So(len(newTask.Commands), ShouldNotEqual, 0)
				for _, command := range newTask.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					for _, c := range pluginCmds {
						pluginCom := &comm.TaskJSONCommunicator{c.Plugin(), httpCom}
						err = c.Execute(logger, pluginCom, taskConfig, make(chan bool))
						So(err, ShouldBeNil)
					}
				}
			}
		})
	})
}

// helper for generating a string of a size
func strOfLen(size int) string {
	b := bytes.Buffer{}
	for i := 0; i < size; i++ {
		b.WriteByte('a')
	}
	return b.String()
}

func TestAttachLargeResults(t *testing.T) {
	if runtime.Compiler == "gccgo" {
		// TODO: Remove skip when compiler is upgraded to include fix for bug https://github.com/golang/go/issues/12781
		t.Skip("skipping test to avoid httptest server bug")
	}
	testutil.HandleTestingErr(db.ClearCollections(task.Collection), t, "problem clearning collections")
	Convey("With a test task and server", t, func() {
		testServer, err := service.CreateTestServer(testutil.TestConfig(), nil, nil)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		defer testServer.Close()
		httpCom, err := comm.NewHTTPCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "", "", "", nil)
		So(err, ShouldBeNil)
		So(httpCom, ShouldNotBeNil)
		pluginCom := &comm.TaskJSONCommunicator{"test", httpCom}
		_, err = createTestConfig(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_project.yml"), t)
		testutil.HandleTestingErr(err, t, "failed to create test config: %v", err)

		Convey("a test log < 16 MB should be accepted", func() {
			id, err := pluginCom.TaskPostTestLog(&model.TestLog{
				Name:  "woah",
				Lines: []string{strOfLen(1024 * 1024 * 15)}, //15MB
			})
			So(id, ShouldNotEqual, "")
			So(err, ShouldBeNil)
		})
		Convey("a test log > 16 MB should error", func() {
			id, err := pluginCom.TaskPostTestLog(&model.TestLog{
				Name:  "woah",
				Lines: []string{strOfLen(1024 * 1024 * 17)}, //17MB
			})
			So(id, ShouldEqual, "")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "size exceeds")
		})
	})
}

func createTestConfig(filename string, t *testing.T) (*model.TaskConfig, error) {
	clearDataMsg := "Failed to clear test data collection"
	testutil.HandleTestingErr(
		db.ClearCollections(
			task.Collection, model.ProjectVarsCollection),
		t, clearDataMsg)

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	testProject := &model.Project{}
	err = yaml.Unmarshal(data, testProject)
	if err != nil {
		return nil, err
	}

	testProjectRef := &model.ProjectRef{
		Identifier: "mongodb-mongo-master",
		Owner:      "mongodb",
		Repo:       "mongo",
		RepoKind:   "github",
		Branch:     "master",
		Enabled:    true,
		BatchTime:  180,
	}

	workDir, err := ioutil.TempDir("", "plugintest_")
	if err != nil {
		return nil, err
	}

	testTask := &task.Task{
		Id:           "mocktaskid",
		BuildId:      "testBuildId",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  "test",
		HostId:       "testHost",
		Version:      "versionId",
		Secret:       "mocktasksecret",
		Status:       evergreen.TaskDispatched,
		Revision:     "cb91350bf017337a734dcd0321bf4e6c34990b6a",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	testutil.HandleTestingErr(testTask.Insert(), t, "failed to insert task")

	projectVars := &model.ProjectVars{
		Id: "mongodb-mongo-master",
		Vars: map[string]string{
			"abc": "xyz",
			"123": "456",
		},
	}
	_, err = projectVars.Upsert()
	testutil.HandleTestingErr(err, t, "failed to upsert project vars")
	testDistro := &distro.Distro{Id: "linux-64", WorkDir: workDir}
	testVersion := &version.Version{}
	return model.NewTaskConfig(testDistro, testVersion, testProject, testTask, testProjectRef)
}

func TestPluginSelfRegistration(t *testing.T) {
	Convey("Assuming the plugin collection has run its init functions", t, func() {
		So(len(plugin.CommandPlugins), ShouldBeGreaterThan, 0)
		nameMap := map[string]uint{}
		// count all occurrences of a plugin name
		for _, plugin := range plugin.CommandPlugins {
			nameMap[plugin.Name()] = nameMap[plugin.Name()] + 1
		}

		Convey("no plugin should be present in Published more than once", func() {
			for _, count := range nameMap {
				So(count, ShouldEqual, 1)
			}
		})

		Convey("some known default plugins should be present in the list", func() {
			// These use strings instead of consts from the plugin
			// packages, so we can avoid importing those packages
			// and make sure the registration from plugin/config
			// is actually happening
			So(nameMap["attach"], ShouldEqual, 1)
			So(nameMap["s3"], ShouldEqual, 1)
			So(nameMap["s3Copy"], ShouldEqual, 1)
			So(nameMap["archive"], ShouldEqual, 1)
			So(nameMap["expansions"], ShouldEqual, 1)
			So(nameMap["git"], ShouldEqual, 1)
			So(nameMap["shell"], ShouldEqual, 1)
		})
	})
}
