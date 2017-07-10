package taskdata

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func init() {
	plugin.Publish(&TaskJSONPlugin{})
}

const (
	TaskJSONPluginName = "json"
	TaskJSONSend       = "send"
	TaskJSONGet        = "get"
	TaskJSONGetHistory = "get_history"
	TaskJSONHistory    = "history"
)

// TaskJSONPlugin handles thet
type TaskJSONPlugin struct{}

// Name implements Plugin Interface.
func (jsp *TaskJSONPlugin) Name() string {
	return TaskJSONPluginName
}

func (hwp *TaskJSONPlugin) GetUIHandler() http.Handler {
	r := mux.NewRouter()

	// version routes
	r.HandleFunc("/version", getVersion)
	r.HandleFunc("/version/{version_id}/{name}/", uiGetTasksForVersion)
	r.HandleFunc("/version/latest/{project_id}/{name}", uiGetTasksForLatestVersion)

	// task routes
	r.HandleFunc("/task/{task_id}/{name}/", uiGetTaskById)
	r.HandleFunc("/task/{task_id}/{name}/tags", uiGetTags)
	r.HandleFunc("/task/{task_id}/{name}/tag", uiHandleTaskTag).Methods("POST", "DELETE")

	r.HandleFunc("/tag/{project_id}/{tag}/{variant}/{task_name}/{name}", uiGetTaskJSONByTag)
	r.HandleFunc("/commit/{project_id}/{revision}/{variant}/{task_name}/{name}", uiGetCommit)
	r.HandleFunc("/history/{task_id}/{name}", uiGetTaskHistory)
	return r
}

func (jsp *TaskJSONPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required to fulfill the Plugin interface. This plugin
// does not have any UI hooks.
func (jsp *TaskJSONPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return &plugin.PanelConfig{}, nil
}

// NewCommand returns requested commands by name. Fulfills the Plugin interface.
func (jsp *TaskJSONPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == TaskJSONSend {
		return &TaskJSONSendCommand{}, nil
	} else if cmdName == TaskJSONGet {
		return &TaskJSONGetCommand{}, nil
	} else if cmdName == TaskJSONGetHistory {
		return &TaskJSONHistoryCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}

type TaskJSONSendCommand struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
}

func (tjsc *TaskJSONSendCommand) Name() string   { return "send" }
func (tjsc *TaskJSONSendCommand) Plugin() string { return "json" }

func (tjsc *TaskJSONSendCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, tjsc); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", tjsc.Name())
	}

	if tjsc.File == "" {
		return errors.New("'file' param must not be blank")
	}

	if tjsc.DataName == "" {
		return errors.New("'name' param must not be blank")
	}

	return nil
}

func (tjsc *TaskJSONSendCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	errChan := make(chan error)
	go func() {
		// attempt to open the file
		fileLoc := filepath.Join(conf.WorkDir, tjsc.File)
		jsonFile, err := os.Open(fileLoc)
		if err != nil {
			errChan <- errors.Wrap(err, "Couldn't open json file")
			return
		}

		jsonData := map[string]interface{}{}
		err = util.ReadJSONInto(jsonFile, &jsonData)
		if err != nil {
			errChan <- errors.Wrap(err, "File contained invalid json")
			return
		}

		retriablePost := util.RetriableFunc(
			func() error {
				logger.Task().Info("Posting JSON")
				resp, err := comm.TaskPostJSON(fmt.Sprintf("data/%v", tjsc.DataName), jsonData)
				if resp != nil {
					defer resp.Body.Close()
				}
				err = errors.WithStack(err)
				if err != nil {
					return util.RetriableError{err}
				}
				if resp.StatusCode != http.StatusOK {
					return util.RetriableError{errors.Errorf("unexpected status code %v", resp.StatusCode)}
				}
				return nil
			},
		)

		_, err = util.Retry(retriablePost, 10, 3*time.Second)
		errChan <- errors.WithStack(err)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			logger.Task().Errorf("Sending json data failed: %v", err)
		}
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received abort signal, stopping.")
		return nil
	}
}

type TaskJSONGetCommand struct {
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	TaskName string `mapstructure:"task" plugin:"expand"`
	Variant  string `mapstructure:"variant" plugin:"expand"`
}

func (jgc *TaskJSONGetCommand) Name() string   { return TaskJSONGet }
func (jgc *TaskJSONGetCommand) Plugin() string { return TaskJSONPluginName }
func (jgc *TaskJSONGetCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, jgc); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", jgc.Name())
	}
	if jgc.File == "" {
		return errors.New("JSON 'get' command must not have blank 'file' parameter")
	}
	if jgc.DataName == "" {
		return errors.New("JSON 'get' command must not have a blank 'name' param")
	}
	if jgc.TaskName == "" {
		return errors.New("JSON 'get' command must not have a blank 'task' param")
	}

	return nil
}

func (jgc *TaskJSONGetCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	err := errors.WithStack(plugin.ExpandValues(jgc, conf.Expansions))
	if err != nil {
		return err
	}

	if jgc.File != "" && !filepath.IsAbs(jgc.File) {
		jgc.File = filepath.Join(conf.WorkDir, jgc.File)
	}

	retriableGet := util.RetriableFunc(
		func() error {
			dataUrl := fmt.Sprintf("data/%s/%s", jgc.TaskName, jgc.DataName)
			if jgc.Variant != "" {
				dataUrl = fmt.Sprintf("data/%s/%s/%s", jgc.TaskName, jgc.DataName, jgc.Variant)
			}
			resp, err := comm.TaskGetJSON(dataUrl)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				logger.Execution().Warningf("Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}

			if resp.StatusCode == http.StatusOK {
				jsonBytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				return ioutil.WriteFile(jgc.File, jsonBytes, 0755)
			}
			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					return errors.New("No JSON data found")
				}
				return util.RetriableError{errors.Errorf("unexpected status code %v", resp.StatusCode)}
			}
			return nil
		},
	)

	_, err = util.Retry(retriableGet, 10, 3*time.Second)
	return errors.WithStack(err)
}

type TaskJSONHistoryCommand struct {
	Tags     bool   `mapstructure:"tags"`
	File     string `mapstructure:"file" plugin:"expand"`
	DataName string `mapstructure:"name" plugin:"expand"`
	TaskName string `mapstructure:"task" plugin:"expand"`
}

func (jgc *TaskJSONHistoryCommand) Name() string   { return TaskJSONHistory }
func (jgc *TaskJSONHistoryCommand) Plugin() string { return TaskJSONPluginName }
func (jgc *TaskJSONHistoryCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, jgc); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", jgc.Name())
	}
	if jgc.File == "" {
		return errors.New("JSON 'history' command must not have blank 'file' param")
	}
	if jgc.DataName == "" {
		return errors.New("JSON 'history command must not have blank 'name' param")
	}
	if jgc.TaskName == "" {
		return errors.New("JSON 'history command must not have blank 'task' param")
	}

	return nil
}

func (jgc *TaskJSONHistoryCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	err := errors.WithStack(plugin.ExpandValues(jgc, conf.Expansions))
	if err != nil {
		return err
	}

	if jgc.File != "" && !filepath.IsAbs(jgc.File) {
		jgc.File = filepath.Join(conf.WorkDir, jgc.File)
	}

	endpoint := fmt.Sprintf("history/%s/%s", jgc.TaskName, jgc.DataName)
	if jgc.Tags {
		endpoint = fmt.Sprintf("tags/%s/%s", jgc.TaskName, jgc.DataName)
	}

	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := comm.TaskGetJSON(endpoint)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				logger.Execution().Warningf("Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}

			if resp.StatusCode == http.StatusOK {
				jsonBytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				return ioutil.WriteFile(jgc.File, jsonBytes, 0755)
			}
			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					return errors.New("No JSON data found")
				}
				return util.RetriableError{errors.Errorf("unexpected status code %v", resp.StatusCode)}
			}
			return nil
		},
	)
	_, err = util.Retry(retriableGet, 10, 3*time.Second)
	return errors.WithStack(err)
}
