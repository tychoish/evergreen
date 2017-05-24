package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/gorilla/mux"
)

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a ServiceContext then attaches each of the main functions for
// the api to the router.
func AttachHandler(root *mux.Router, superUsers []string, URL, prefix string) http.Handler {
	sc := &servicecontext.DBServiceContext{}

	sc.SetURL(URL)
	sc.SetPrefix(prefix)
	sc.SetSuperUsers(superUsers)
	return GetHandler(root, sc)
}

// GetHandler builds each of the functions that this api implements and then
// registers them on the given router. It then returns the given router as an
// http handler which can be given more functions.
func GetHandler(r *mux.Router, sc servicecontext.ServiceContext) http.Handler {
	getPlaceHolderManager("/", 2).Register(r, sc)
	getHostRouteManager("/hosts", 2).Register(r, sc)
	getTaskRouteManager("/tasks/{task_id}", 2).Register(r, sc)
	getTestRouteManager("/tasks/{task_id}/tests", 2).Register(r, sc)
	getTaskSystemMetricsManager("/tasks/{task_id}/metrics/system", 2).Register(r, sc)
	getTaskProcessMetricsManager("/tasks/{task_id}/metrics/process", 2).Register(r, sc)
	getTaskRestartRouteManager("/tasks/{task_id}/restart", 2).Register(r, sc)
	getTasksByProjectAndCommitRouteManager("/projects/{project_id}/revisions/{commit_hash}/tasks", 2).Register(r, sc)
	getTasksByBuildRouteManager("/builds/{build_id}/tasks", 2).Register(r, sc)

	return r
}

type PlaceHolderRequestHandler struct{}

func getPlaceHolderManager(route string, version int) *RouteManager {
	// PLACE HOLDER ROUTE DEFINITION
	// make object
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &PlaceHolderRequestHandler{},
				MethodType:     evergreen.MethodGet,
			},
		},
		Version: version,
	}
}

func (p *PlaceHolderRequestHandler) Handler() RequestHandler {
	return &PlaceHolderRequestHandler{}
}

func (p *PlaceHolderRequestHandler) ParseAndValidate(r *http.Request) error {
	return nil
}
func (p *PlaceHolderRequestHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	return ResponseData{}, apiv3.APIError{
		StatusCode: http.StatusOK,
		Message:    "this is a placeholder for now",
	}
}
