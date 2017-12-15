package operations

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

const (
	confFlagName       = "conf"
	adminFlagsFlagName = "flags"
	pathFlagName       = "path"
	projectFlagName    = "project"
)

func addPathFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    pathFlagName,
		Aliases: []string{"filename", "file", "f"},
		Usage:   "path to an evergreen project configuration file",
	})
}

func serviceConfigFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    confFlagName,
		Aliases: []string{"c", "config"},
		Usage:   "path to the service configuration file",
		Default: evergreen.DefaultServiceConfigurationFileName,
	})
}

func addProjectFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    projectFlagName,
		Aliases: []string{"p"},
		Usage:   "specify the name of an existing evergreen project",
	})
}

func adminFlagFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  flagFlagName,
		Usage: "specify a flag to disable; may specify more than once",
	})
}
