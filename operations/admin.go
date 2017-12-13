package operations

import (
	"context"
	"errors"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/urfave/cli"
)

func Admin() cli.Command {
	return cli.Command{
		Name:  "admin",
		Usage: "site administration for an evergreen deployment",
		Subcommands: []cli.Command{
			adminSetBanner(false),
			adminDisableService(),
			adminEnableService(),
		},
	}
}

func adminSetBanner(disableNetworkForTest bool) cli.Command {
	const (
		messageFlagName = "message"
		clearFlagName   = "clear"
		themeFlagName   = "theme"
	)

	return cli.Command{
		Name:    "banner",
		Aliases: []string{"set-banner"},
		Usage:   "modify the contents of the site-wide display banner",
		Flags: clientConfigFlags(
			cli.StringFlag{
				Name:    messageFlagName,
				Aliases: []string{"m"},
				Usage:   "content of new message",
			},
			cli.StringFlag{
				Name:    themeFlagName,
				Aliases: []string{"t"},
				Usage:   "color theme to use for banner",
			},
			cli.BoolFlag{
				Name:  clearFlagName,
				Usage: "clear the content of the banner",
			}),
		Before: requireConfig(
			func(c *cli.Context) error {
				if c.String(messageFlagName) != "" && c.Bool(clearFlagName) {
					return errors.New("cannot specify a message and the 'clear' option at the same time")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.String(messageFlagName) == "" && !c.Bool(clearFlagName) {
					return errors.New("cannot specify a message and the 'clear' option at the same time")
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			themeName := c.String(themeFlagName)
			msgContent := c.String(messageFlagName)
			confPath := c.String(confFlagName)

			var theme admin.BannerTheme
			var ok bool
			if themeName != "" {
				if ok, theme = admin.IsValidBannerTheme(themeName); !ok {
					return fmt.Errorf("%s is not a valid banner theme", themeName)
				}
			}

			var err error
			confPath, err := findConfigFilePath(confPath)
			if err != nil {
				return errors.Wrap(err, "problem finding configuration file")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSetttings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			if disableNetworkForTest {
				return nil
			}

			client := conf.GetRestCommunicator(ctx)
			defer client.Close()

			client.SetAPIUser(settings.User)
			client.SetAPIKey(settings.APIKey)

			return errors.Wrap(client.SetBannerMessage(ctx, msgContent, theme),
				"problem setting the site-wide banner message")
		},
	}
}

func adminDisableService() cli.Command {
	return cli.Command{
		Name:  "disable-service",
		Usage: "disable a background service",
	}
}
func adminEnableService() cli.Command {}
