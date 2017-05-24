package route

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type taskMetricsArgs struct {
	task string
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the system information for a task
//
//    /tassk/{task_id}/metrics/system

func getTaskSystemMetricsManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     evergreen.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &taskSystemMetricsHandler{},
			},
		},
	}
}

type taskSystemMetricsHandler struct {
	PaginationExecutor
}

func (p *taskSystemMetricsHandler) Handler() RequestHandler {
	return &taskSystemMetricsHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       taskSystemMetricsPaginator,
	}}
}

func (p *taskSystemMetricsHandler) ParseAndValidate(r *http.Request) error {
	p.Args = taskMetricsArgs{task: mux.Vars(r)["task_id"]}

	return p.PaginationExecutor.ParseAndValidate(r)
}

func taskSystemMetricsPaginator(key string, limit int, args interface{}, sc servicecontext.ServiceContext) ([]model.Model, *PageResult, error) {
	task := args.(*taskMetricsArgs).task
	grip.Infoln("getting results for task:", task)

	ts, err := time.ParseInLocation(model.APITimeFormat, key, time.FixedZone("", 0))
	if err != nil {
		return []model.Model{}, nil, apiv3.APIError{
			Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	// fetch required data from the service layer
	data, err := sc.FindTaskSystemMetrics(task, ts, limit*2, 1)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err

	}
	prevData, err := sc.FindTaskSystemMetrics(task, ts, limit, -1)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// populate the page info structure
	pages := &PageResult{}
	if len(data) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(data[limit].Base.Time).String(),
			Limit:    len(data) - limit,
		}
	}
	if len(prevData) > 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(data[0].Base.Time).String(),
			Limit:    len(prevData),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		data = data[:limit]
	}

	models := make([]model.Model, len(data))
	for idx, info := range data {
		sysinfoModel := &model.APISystemMetrics{}
		if err = sysinfoModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, apiv3.APIError{
				Message:    "problem converting metrics document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models[idx] = sysinfoModel
	}

	return models, pages, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the process tree for a task
//
//    /tassk/{task_id}/metrics/process

func getTaskProcessMetricsManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     evergreen.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &taskSystemMetricsHandler{},
			},
		},
	}
}

type taskProcessMetricsHandler struct {
	PaginationExecutor
}

func (p *taskProcessMetricsHandler) Handler() RequestHandler {
	return &taskProcessMetricsHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       taskProcessMetricsPaginator,
	}}
}

func (p *taskProcessMetricsHandler) ParseAndValidate(r *http.Request) error {
	p.Args = taskMetricsArgs{task: mux.Vars(r)["task_id"]}

	return p.PaginationExecutor.ParseAndValidate(r)
}

func taskProcessMetricsPaginator(key string, limit int, args interface{}, sc servicecontext.ServiceContext) ([]model.Model, *PageResult, error) {
	task := args.(*taskMetricsArgs).task
	grip.Infoln("getting results for task:", task)

	ts, err := time.ParseInLocation(model.APITimeFormat, key, time.FixedZone("", 0))
	if err != nil {
		return []model.Model{}, nil, apiv3.APIError{
			Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}

	// fetch required data from the service layer
	data, err := sc.FindTaskProcessMetrics(task, ts, limit*2, 1)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "database error")
		}
		return []model.Model{}, nil, err

	}
	prevData, err := sc.FindTaskProcessMetrics(task, ts, limit, -1)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// populate the page info structure
	pages := &PageResult{}
	if len(data) > limit && len(data[limit]) > 1 {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(data[limit][0].Base.Time).String(),
			Limit:    len(data) - limit,
		}
	}
	if len(prevData) > 1 && len(data[0]) > 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(data[0][0].Base.Time).String(),
			Limit:    len(prevData),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		data = data[:limit]
	}

	models := make([]model.Model, len(data))
	for idx, info := range data {
		procModel := &model.APIProcessMetrics{}
		if err = procModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, apiv3.APIError{
				Message:    "problem converting metrics document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models[idx] = procModel
	}

	return models, pages, nil
}
