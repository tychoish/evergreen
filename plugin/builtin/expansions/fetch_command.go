package expansions

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"golang.org/x/net/context"
)

const FetchVarsRoute = "fetch_vars"
const FetchVarsCmdname = "fetch"

// FetchVarsCommand pulls a set of vars (stored in the DB on the server side)
// and updates the agent's expansions map using the values it gets back
type FetchVarsCommand struct {
	Keys []FetchCommandParams `mapstructure:"keys" json:"keys"`
}

// FetchCommandParams is a pairing of remote key and local key values
type FetchCommandParams struct {
	// RemoteKey indicates which key in the projects vars map to use as the lvalue
	RemoteKey string `mapstructure:"remote_key" json:"remote_key"`

	// LocalKey indicates which key in the local expansions map to use as the rvalue
	LocalKey string `mapstructure:"local_key" json:"local_key"`
}

func (c *FetchVarsCommand) Name() string                                    { return FetchVarsCmdname }
func (c *FetchVarsCommand) Plugin() string                                  { return ExpansionsPluginName }
func (c *FetchVarsCommand) ParseParams(params map[string]interface{}) error { return nil }

// Execute fetches the expansions from the API server
func (c *FetchVarsCommand) Execute(ctx context.Context, client client.Communicator, conf *model.TaskConfig) error {
	logger := client.GetLoggerProducer(conf.Task.Id, conf.Task.Secret)
	logger.Task().Error("Expansions.fetch deprecated")
	return nil
}
