package expansions

import (
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func init() {
	plugin.Publish(&ExpansionsPlugin{})
}

const (
	ExpansionsPluginName = "expansions"
	UpdateVarsCmdName    = "update"
)

// ExpansionsPlugin handles updating expansions in a task at runtime.
type ExpansionsPlugin struct{}

// Name fulfills the Plugin interface.
func (p *ExpansionsPlugin) Name() string {
	return ExpansionsPluginName
}

func (p *ExpansionsPlugin) Configure(map[string]interface{}) error {
	return nil
}

// NewCommand fulfills the Plugin interface.
func (p *ExpansionsPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == UpdateVarsCmdName {
		return &UpdateCommand{}, nil
	} else if cmdName == FetchVarsCmdname {
		return &FetchVarsCommand{}, nil
	}
	return nil, &plugin.ErrUnknownCommand{cmdName}
}

// UpdateCommand reads in a set of new expansions and updates the
// task's expansions at runtime. UpdateCommand can take a list
// of update expansion pairs and/or a file of expansion pairs
type UpdateCommand struct {
	// Key-value pairs for updating the task's parameters with
	Updates []PutCommandParams `mapstructure:"updates"`

	// Filename for a yaml file containing expansion updates
	// in the form of
	//   "expansion_key: expansions_value"
	YamlFile string `mapstructure:"file"`
}

// PutCommandParams are pairings of expansion names
// and the value they expand to
type PutCommandParams struct {
	// The name of the expansion
	Key string

	// The expanded value
	Value string

	// Can optionally concat a string to the end of the current value
	Concat string
}

func (c *UpdateCommand) Name() string {
	return UpdateVarsCmdName
}

func (c *UpdateCommand) Plugin() string {
	return ExpansionsPluginName
}

// ParseParams validates the input to the UpdateCommand, returning and error
// if something is incorrect. Fulfills Command interface.
func (c *UpdateCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return err
	}

	for _, item := range c.Updates {
		if item.Key == "" {
			return errors.Errorf("error parsing '%v' params: key must not be "+
				"a blank string", c.Name())
		}
	}

	return nil
}

func (c *UpdateCommand) ExecuteUpdates(ctx context.Context, conf *model.TaskConfig) error {
	for _, update := range c.Updates {
		if ctx.Err() != nil {
			return errors.New("operation aborted")
		}

		if update.Concat == "" {
			newValue, err := conf.Expansions.ExpandString(update.Value)

			if err != nil {
				return errors.WithStack(err)
			}
			conf.Expansions.Put(update.Key, newValue)
		} else {
			newValue, err := conf.Expansions.ExpandString(update.Concat)
			if err != nil {
				return errors.WithStack(err)
			}

			oldValue := conf.Expansions.Get(update.Key)
			conf.Expansions.Put(update.Key, oldValue+newValue)
		}
	}

	return nil
}

// Execute updates the expansions. Fulfills Command interface.
func (c *UpdateCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	err := c.ExecuteUpdates(ctx, conf)

	if err != nil {
		return errors.WithStack(err)
	}

	if c.YamlFile != "" {
		c.YamlFile, err = conf.Expansions.ExpandString(c.YamlFile)
		if err != nil {
			return errors.WithStack(err)
		}

		logger.Task().Infof("Updating expansions with keys from file: %s", c.YamlFile)
		filename := filepath.Join(conf.WorkDir, c.YamlFile)
		err := conf.Expansions.UpdateFromYaml(filename)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil

}
