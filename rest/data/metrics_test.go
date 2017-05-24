package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type MetricsConnectorSuite struct {
	ctx *DBMetricsConnector
	suite.Suite
}

func TestMetricsConnectorSuite(t *testing.T) {
	suite.Run(t, new(MetricsConnectorSuite))
}

func (s *MetricsConnectorSuite) SetupSuite() {
	testutil.ConfigureIntegrationTest(s.T(), testConfig, "TestFindTaskById")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
}

func (s *MetricsConnectorSuite) SetupTest() {
	s.ctx = &DBMetricsConnector{}
	s.Require().NoError(db.Clear(event.EventTaskSystemInfo))
	s.Require().NoError(db.Clear(event.EventTaskProcessInfo))
}

func (s *MetricsConnectorSuite) TestSystemsResultsShouldBeEmpty() {
	sys, err := s.ctx.FindTaskSystemMetrics("foo", time.Now(), 100, -1)
	s.NoError(err)
	s.Len(sys, 0)
}

func (s *MetricsConnectorSuite) TestProcessMetricsShouldBeEmpty() {
	procs, err := s.ctx.FindTaskProcessMetrics("foo", time.Now(), 100, -1)
	s.NoError(err)
	s.Len(procs, 0)
}

func (s *MetricsConnectorSuite) TestProcessLimitingFunctionalityConstrainsResults() {
	msgs := []*message.ProcessInfo{}
	for _, m := range message.CollectProcessInfoSelfWithChildren() {
		msgs = append(msgs, m.(*message.ProcessInfo))
		msgs = append(msgs, m.(*message.ProcessInfo))
		msgs = append(msgs, m.(*message.ProcessInfo))
		msgs = append(msgs, m.(*message.ProcessInfo))
		msgs = append(msgs, m.(*message.ProcessInfo))
		msgs = append(msgs, m.(*message.ProcessInfo))
	}
	event.LogTaskProcessData("foo", msgs)
	procs, err := s.ctx.FindTaskProcessMetrics("foo", time.Now().Add(-2*time.Hour), 1, 1)
	s.NoError(err)

	s.Len(procs, 1, "len of %d", len(msgs))
}
