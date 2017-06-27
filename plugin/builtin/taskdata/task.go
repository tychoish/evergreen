package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
)

// uiGetTaskById sends back a JSONTask with the corresponding task id.
func uiGetTaskById(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	name := mux.Vars(r)["name"]
	jsonForTask, err := model.GetTaskJSONById(taskId, name)
	if err != nil {
		if err != mgo.ErrNotFound {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTask)
}

func apiGetTaskByName(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]

	jsonForTask, err := model.GetTaskJSONByName(t.Version, t.BuildId, taskName, name)
	if err != nil {
		if err == mgo.ErrNotFound {
			plugin.WriteJSON(w, http.StatusNotFound, nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		plugin.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTask.Data)
}

// apiGetTaskForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func apiGetTaskForVariant(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]
	variantId := mux.Vars(r)["variant"]
	// Find the task for the other variant, if it exists

	jsonForTask, err := model.GetTaskJSONForVariant(t.Version, variantId, taskName, name)

	if err != nil {
		if err == mgo.ErrNotFound {
			plugin.WriteJSON(w, http.StatusNotFound, nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		plugin.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTask.Data)
}

func apiInsertTask(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	rawData := map[string]interface{}{}
	err := util.ReadJSONInto(util.NewRequestReader(r), &rawData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = model.InsertTaskJSON(t, name, rawData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, "ok")
	return
}
