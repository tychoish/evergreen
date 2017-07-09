package keyval

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	KeyValPluginName = "keyval"
	IncCommandName   = "inc"
	IncRoute         = "inc"
)

func init() {
	plugin.Publish(&KeyValPlugin{})
}

type KeyValPlugin struct{}

func (self *KeyValPlugin) Configure(map[string]interface{}) error {
	return nil
}

func (self *KeyValPlugin) Name() string {
	return KeyValPluginName
}

type IncCommand struct {
	Key         string `mapstructure:"key"`
	Destination string `mapstructure:"destination"`
}

func (self *IncCommand) Name() string {
	return IncCommandName
}

func (self *IncCommand) Plugin() string {
	return KeyValPluginName
}

// ParseParams validates the input to the IncCommand, returning an error
// if something is incorrect. Fulfills Command interface.
func (incCmd *IncCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, incCmd)
	if err != nil {
		return err
	}

	if incCmd.Key == "" || incCmd.Destination == "" {
		return fmt.Errorf("error parsing '%v' params: key and destination may not be blank",
			IncCommandName)
	}

	return nil
}

// Execute fetches the expansions from the API server
func (incCmd *IncCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	if err := plugin.ExpandValues(incCmd, conf.Expansions); err != nil {
		return err
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	keyVal := model.KeyVal{}
	err := comm.IncrementKey(ctx, td, &keyVal) //.TaskPostJSON(IncRoute, incCmd.Key)
	if err != nil {
		return errors.Wrapf(err, "problem incriminating key %s", incCmd.Key)
	}

	conf.Expansions.Put(incCmd.Destination, fmt.Sprintf("%d", keyVal.Value))
	return nil
}

func (self *KeyValPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == IncCommandName {
		return &IncCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}
