package route

import (
	"testing"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/stretchr/testify/suite"
)

type TaskMetricsSuite struct {
	data      data.MockMetricsConnector
	paginator PaginatorFunc
	suite.Suite
}

func TestTaskSystemMetricsSuite(t *testing.T) {
	s := new(TaskMetricsSuite)
	s.data = &MockMetricsConnector{}
	suite.Run(t, s)
}

func TestTaskProcessMetricsSuite(t *testing.T) {
	s := new(TaskMetricsSuite)
	s.data = &MockMetricsConnector{}
	suite.Run(t, s)
}

func (s *TaskMetricsSuite) SetupSuite() {}

func (s *TaskMetricsSuite) SetupTest() {}

func (s *TaskMetricsSuite) Test() {

}
