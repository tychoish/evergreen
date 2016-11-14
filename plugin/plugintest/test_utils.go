package plugintest

import (
	"io"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	slogger "github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

type MockLogger struct{}

func (_ *MockLogger) Flush()                                                                   {}
func (_ *MockLogger) LogLocal(level slogger.Level, messageFmt string, args ...interface{})     {}
func (_ *MockLogger) LogExecution(level slogger.Level, messageFmt string, args ...interface{}) {}
func (_ *MockLogger) LogTask(level slogger.Level, messageFmt string, args ...interface{})      {}
func (_ *MockLogger) LogSystem(level slogger.Level, messageFmt string, args ...interface{})    {}
func (_ *MockLogger) GetTaskLogWriter(level slogger.Level) io.Writer                           { return ioutil.Discard }
func (_ *MockLogger) GetSystemLogWriter(level slogger.Level) io.Writer                         { return ioutil.Discard }

func CreateTestConfig(filename string, t *testing.T) (*model.TaskConfig, error) {
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, model.ProjectVarsCollection),
		t, "Failed to clear test data collection")

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	testProject := &model.Project{}

	err = yaml.Unmarshal(data, testProject)
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
		Secret:       "mocktasksecret",
		Status:       evergreen.TaskDispatched,
		Version:      "versionId",
		Revision:     "cb91350bf017337a734dcd0321bf4e6c34990b6a",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	testutil.HandleTestingErr(testTask.Insert(), t, "failed to insert task")
	testutil.HandleTestingErr(err, t, "failed to upsert project ref")

	projectVars := &model.ProjectVars{
		Id: "mongodb-mongo-master",
		Vars: map[string]string{
			"abc": "xyz",
			"123": "456",
		},
	}

	projectRef := &model.ProjectRef{
		Owner:       "mongodb",
		Repo:        "mongo",
		Branch:      "master",
		RepoKind:    "github",
		Enabled:     true,
		Private:     false,
		BatchTime:   0,
		RemotePath:  "etc/evergreen.yml",
		Identifier:  "mongodb-mongo-master",
		DisplayName: "mongodb-mongo-master",
		LocalConfig: string(data),
	}
	err = projectRef.Upsert()
	testutil.HandleTestingErr(err, t, "failed to upsert project ref")
	projectRef.Upsert()
	_, err = projectVars.Upsert()
	testutil.HandleTestingErr(err, t, "failed to upsert project vars")

	workDir, err := ioutil.TempDir("", "plugintest_")
	testutil.HandleTestingErr(err, t, "failed to get working directory: %v")
	testDistro := &distro.Distro{Id: "linux-64", WorkDir: workDir}
	testVersion := &version.Version{}
	return model.NewTaskConfig(testDistro, testVersion, testProject, testTask, projectRef)
}

func TestAgentCommunicator(taskId string, taskSecret string, apiRootUrl string) *comm.HTTPCommunicator {
	agentCommunicator, err := comm.NewHTTPCommunicator(apiRootUrl, taskId, taskSecret, "", nil)
	if err != nil {
		panic(err)
	}
	agentCommunicator.MaxAttempts = 3
	agentCommunicator.RetrySleep = 100 * time.Millisecond
	return agentCommunicator
}

func SetupAPITestData(taskDisplayName string, isPatch bool, t *testing.T) (*task.Task, *build.Build, error) {
	//ignore errs here because the ns might just not exist.
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, build.Collection,
			host.Collection, version.Collection, patch.Collection),
		t, "Failed to clear test collections")

	testHost := &host.Host{
		Id:          "testHost",
		Host:        "testHost",
		RunningTask: "testTaskId",
		StartedBy:   evergreen.User,
	}
	testutil.HandleTestingErr(testHost.Insert(), t, "failed to insert host")

	task := &task.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "rhel55",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Version:      "testVersionId",
		Secret:       "testTaskSecret",
		Status:       evergreen.TaskDispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
	}

	if isPatch {
		task.Requester = evergreen.PatchVersionRequester
	}

	testutil.HandleTestingErr(task.Insert(), t, "failed to insert task")

	version := &version.Version{Id: "testVersionId", BuildIds: []string{task.BuildId}}
	testutil.HandleTestingErr(version.Insert(), t, "failed to insert version %v")
	if isPatch {

		modulePatchContent, err := ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "testmodule.patch"))
		testutil.HandleTestingErr(err, t, "failed to read test module patch file %v")

		patch := &patch.Patch{
			Status:  evergreen.PatchCreated,
			Version: version.Id,
			Patches: []patch.ModulePatch{
				{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		testutil.HandleTestingErr(patch.Insert(), t, "failed to insert patch %v")

	}

	session, _, err := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(err, t, "couldn't get db session!")

	//Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).RemoveAll(bson.M{"t_id": task.Id})
	testutil.HandleTestingErr(err, t, "failed to remove logs")

	build := &build.Build{Id: "testBuildId", Tasks: []build.TaskCache{build.NewTaskCache(task.Id, task.DisplayName, true)}}

	testutil.HandleTestingErr(build.Insert(), t, "failed to insert build %v")
	return task, build, nil
}
