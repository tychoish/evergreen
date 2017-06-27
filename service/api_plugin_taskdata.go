package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/gorilla/mux"
)

func (as *APIServer) getTaskJSONTagsForTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]

	tagged, err := model.GetTaskJSONTagsForTask(t.Project, t.BuildVariant, taskName, name)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, tagged)
}
