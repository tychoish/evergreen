package plugin_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	slogger "github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
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
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

var Port = 8181

type MockPlugin struct {
}

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
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
		return fmt.Errorf("Param1 must be a non-blank string.")
	}
	if mc.Param2 == 0 {
		return fmt.Errorf("Param2 must be a non-zero integer.")
	}
	return nil
}

func (mc *MockCommand) Execute(logger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	resp, err := pluginCom.TaskGetJSON(fmt.Sprintf("blah/%s/%d", mc.Param1, mc.Param2))
	if resp != nil {
		defer resp.Body.Close()
	}

	if resp == nil {
		return fmt.Errorf("Received nil HTTP response from api server")
	}

	jsonReply := map[string]string{}
	err = util.ReadJSONInto(resp.Body, &jsonReply)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Got bad status code from API response: %v, body: %v", resp.StatusCode, jsonReply)
	}

	expectedEchoReply := fmt.Sprintf("%v/%v/%v", mc.Param1, mc.Param2, conf.Task.Id)
	if jsonReply["echo"] != expectedEchoReply {
		return fmt.Errorf("Wrong echo reply! Wanted %v, got %v", expectedEchoReply, jsonReply["echo"])
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
			registry.Register(&MockPlugin{})
			registry.Register(&shell.ShellPlugin{})
			registry.Register(&expansions.ExpansionsPlugin{})

			data, err := ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(),
				"testdata", "plugin_project.yml"))
			testutil.HandleTestingErr(err, t, "failed to load test yaml file")
			project := &model.Project{}
			err = yaml.Unmarshal(data, project)
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
	testConfig := evergreen.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchTask")
	Convey("With a SimpleRegistry", t, func() {
		Convey("with a project file containing functions", func() {
			registry := plugin.NewSimpleRegistry()
			err := registry.Register(&shell.ShellPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&expansions.ExpansionsPlugin{})
			testutil.HandleTestingErr(err, t, "Couldn't register plugin")

			testServer, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, false)
			testutil.HandleTestingErr(err, t, "Couldn't set up testing server")

			taskConfig, err := createTestConfig("testdata/plugin_project_functions.yml", t)
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

			httpCom, err := agent.NewHTTPCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "", nil)
			So(err, ShouldBeNil)
			So(httpCom, ShouldNotBeNil)

			Convey("all commands in test project should execute successfully", func() {
				sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
				logger := agent.NewTestLogger(sliceAppender)
				for _, newTask := range taskConfig.Project.Tasks {
					So(len(newTask.Commands), ShouldNotEqual, 0)
					for _, command := range newTask.Commands {
						pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
						testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
						So(pluginCmds, ShouldNotBeNil)
						So(err, ShouldBeNil)
						So(len(pluginCmds), ShouldEqual, 1)
						cmd := pluginCmds[0]
						pluginCom := &agent.TaskJSONCommunicator{cmd.Plugin(), httpCom}
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

		testServer, err := service.CreateTestServer(evergreen.TestConfig(), nil, apiPlugins, false)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")

		httpCom, err := agent.NewHTTPCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "", nil)
		So(err, ShouldBeNil)
		So(httpCom, ShouldNotBeNil)

		pluginConfigPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_project.yml")
		taskConfig, err := createTestConfig(pluginConfigPath, t)
		testutil.HandleTestingErr(err, t, "failed to create test config: %v", err)
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			for _, newTask := range taskConfig.Project.Tasks {
				So(len(newTask.Commands), ShouldNotEqual, 0)
				for _, command := range newTask.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					for _, c := range pluginCmds {
						pluginCom := &agent.TaskJSONCommunicator{c.Plugin(), httpCom}
						err = c.Execute(logger, pluginCom, taskConfig, make(chan bool))
						So(err, ShouldBeNil)
					}
				}
			}
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

func setupAPITestData(taskDisplayName string, isPatch bool, t *testing.T) (*task.Task, *build.Build, error) {
	//ignore errs here because the ns might just not exist.
	clearDataMsg := "Failed to clear test data collection"

	testutil.HandleTestingErr(
		db.ClearCollections(
			task.Collection, build.Collection, host.Collection,
			version.Collection, patch.Collection),
		t, clearDataMsg)

	testHost := &host.Host{
		Id:          "testHost",
		Host:        "testHost",
		RunningTask: "testTaskId",
		StartedBy:   evergreen.User,
	}
	testutil.HandleTestingErr(testHost.Insert(), t, "failed to insert host")

	newTask := &task.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "rhel55",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Secret:       "testTaskSecret",
		Status:       evergreen.TaskDispatched,
		Version:      "versionId",
		Requester:    evergreen.RepotrackerVersionRequester,
	}

	if isPatch {
		newTask.Requester = evergreen.PatchVersionRequester
	}

	testutil.HandleTestingErr(newTask.Insert(), t, "failed to insert task")

	v := &version.Version{
		Id:       "testVersionId",
		BuildIds: []string{newTask.BuildId},
	}
	testutil.HandleTestingErr(v.Insert(), t, "failed to insert version %v")
	if isPatch {
		mainPatchContent, err := ioutil.ReadFile("testdata/test.patch")
		testutil.HandleTestingErr(err, t, "failed to read test patch file %v")
		modulePatchContent, err := ioutil.ReadFile("testdata/testmodule.patch")
		testutil.HandleTestingErr(err, t, "failed to read test module patch file %v")

		p := &patch.Patch{
			Status:  evergreen.PatchCreated,
			Version: v.Id,
			Patches: []patch.ModulePatch{
				{
					ModuleName: "",
					Githash:    "cb91350bf017337a734dcd0321bf4e6c34990b6a",
					PatchSet:   patch.PatchSet{Patch: string(mainPatchContent)},
				},
				{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		testutil.HandleTestingErr(p.Insert(), t, "failed to insert version %v")

	}

	session, _, err := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(err, t, "couldn't get db session!")

	//Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).RemoveAll(bson.M{"t_id": newTask.Id})
	testutil.HandleTestingErr(err, t, "failed to remove logs")

	build := &build.Build{
		Id: "testBuildId",
		Tasks: []build.TaskCache{
			build.NewTaskCache(newTask.Id, newTask.DisplayName, true),
		},
		Version: "testVersionId",
	}

	testutil.HandleTestingErr(build.Insert(), t, "failed to insert build %v")
	return newTask, build, nil
}

func TestPluginSelfRegistration(t *testing.T) {
	Convey("Assuming the plugin collection has run its init functions", t, func() {
		So(len(plugin.CommandPlugins), ShouldBeGreaterThan, 0)
		nameMap := map[string]uint{}
		// count all occurances of a plugin name
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
