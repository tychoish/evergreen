package evergreen

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

var (
	// Should be specified with -ldflags at build time
	BuildRevision = ""

	// Commandline Version String; used to control auto-updating.
	ClientVersion = "e4185327b58797d5ebcc88f092b33e319d8c8521" // in the future this should probably be a date, but this is the legacy value
)

const (
	// DefaultConfFile is the default config file path for Evergreen.
	DefaultConfFile = "/etc/mci_settings.yml"
)

// AuthUser configures a user for our Naive authentication setup.
type AuthUser struct {
	Username    string `yaml:"username"`
	DisplayName string `yaml:"display_name"`
	Password    string `yaml:"password"`
	Email       string `yaml:"email"`
}

// NaiveAuthConfig contains a list of AuthUsers from the settings file.
type NaiveAuthConfig struct {
	Users []*AuthUser
}

// CrowdConfig holds settings for interacting with Atlassian Crowd.
type CrowdConfig struct {
	Username string
	Password string
	Urlroot  string
}

// GithubAuthConfig holds settings for interacting with Github Authentication including the
// ClientID, ClientSecret and CallbackUri which are given when registering the application
// Furthermore,
type GithubAuthConfig struct {
	ClientId     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Users        []string `yaml:"users"`
	Organization string   `yaml:"organization"`
}

// AuthConfig has a pointer to either a CrowConfig or a NaiveAuthConfig.
type AuthConfig struct {
	Crowd  *CrowdConfig      `yaml:"crowd"`
	Naive  *NaiveAuthConfig  `yaml:"naive"`
	Github *GithubAuthConfig `yaml:"github"`
}

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int
	MaxRepoRevisionsToSearch   int
	LogFile                    string
}

type ClientBinary struct {
	Arch string `yaml:"arch" json:"arch"`
	OS   string `yaml:"os" json:"os"`
	URL  string `yaml:"url" json:"url"`
}

type ClientConfig struct {
	ClientBinaries []ClientBinary `yaml:"client_binaries"`
	LatestRevision string         `yaml:"latest_revision"`
}

// APIConfig holds relevant encryption and log settings for the API server.
type APIConfig struct {
	LogFile         string
	HttpListenAddr  string
	HttpsListenAddr string
	HttpsKey        string
	HttpsCert       string
}

// UIConfig holds relevant settings for the UI server.
type UIConfig struct {
	Url            string
	HelpUrl        string
	HttpListenAddr string
	// Secret to encrypt session storage
	Secret  string
	LogFile string
	// Default project to assume when none specified, e.g. when using
	// the /waterfall route use this project, while /waterfall/other-project
	// then use `other-project`
	DefaultProject string
	// Cache results of template compilation, so you don't have to re-read files
	// on every request. Note that if this is true, changes to HTML templates
	// won't take effect until server restart.
	CacheTemplates bool
	// SecureCookies sets the "secure" flag on user tokens. Evergreen
	// does not yet natively support SSL UI connections, but this option
	// is available, for example, for deployments behind HTTPS load balancers.
	SecureCookies bool
}

// MonitorConfig holds logging settings for the monitor process.
type MonitorConfig struct {
	LogFile string
}

// RunnerConfig holds logging and timing settings for the runner process.
type RunnerConfig struct {
	LogFile         string
	IntervalSeconds int64
}

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	LogFile           string
	SSHTimeoutSeconds int64
}

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	LogFile string
	SMTP    *SMTPConfig `yaml:"smtp"`
}

// SMTPConfig holds SMTP email settings.
type SMTPConfig struct {
	Server     string   `yaml:"server"`
	Port       int      `yaml:"port"`
	UseSSL     bool     `yaml:"use_ssl"`
	Username   string   `yaml:"username"`
	Password   string   `yaml:"password"`
	From       string   `yaml:"from"`
	AdminEmail []string `yaml:"admin_email"`
}

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	LogFile     string
	MergeToggle int
}

// TaskRunnerConfig holds logging settings for the scheduler process.
type TaskRunnerConfig struct {
	LogFile string
}

// CloudProviders stores configuration settings for the supported cloud host providers.
type CloudProviders struct {
	AWS          AWSConfig          `yaml:"aws"`
	DigitalOcean DigitalOceanConfig `yaml:"digitalocean"`
}

// AWSConfig stores auth info for Amazon Web Services.
type AWSConfig struct {
	Secret string `yaml:"aws_secret"`
	Id     string `yaml:"aws_id"`
}

// DigitalOceanConfig stores auth info for Digital Ocean.
type DigitalOceanConfig struct {
	ClientId string `yaml:"client_id"`
	Key      string `yaml:"key"`
}

// JiraConfig stores auth info for interacting with Atlassian Jira.
type JiraConfig struct {
	Host     string
	Username string
	Password string
}

// PluginConfig holds plugin-specific settings, which are handled.
// manually by their respective plugins
type PluginConfig map[string]map[string]interface{}

type AlertsConfig struct {
	LogFile string
	SMTP    *SMTPConfig `yaml:"smtp"`
}
type WriteConcern struct {
	W        int    `yaml:"w"`
	WMode    string `yaml:"wmode"`
	WTimeout int    `yaml:"wtimeout"`
	FSync    bool   `yaml:"fsync"`
	J        bool   `yaml:"j"`
}

type DBSettings struct {
	Url                  string       `yaml:"url"`
	SSL                  bool         `yaml:"ssl"`
	DB                   string       `yaml:"db"`
	WriteConcernSettings WriteConcern `yaml:"write_concern"`
}

// Settings contains all configuration settings for running Evergreen.
type Settings struct {
	Database            DBSettings        `yaml:"database"`
	WriteConcern        WriteConcern      `yaml:"write_concern"`
	ConfigDir           string            `yaml:"configdir"`
	ApiUrl              string            `yaml:"api_url"`
	AgentExecutablesDir string            `yaml:"agentexecutablesdir"`
	ClientBinariesDir   string            `yaml:"client_binaries_dir"`
	SuperUsers          []string          `yaml:"superusers"`
	Jira                JiraConfig        `yaml:"jira"`
	Providers           CloudProviders    `yaml:"providers"`
	Keys                map[string]string `yaml:"keys"`
	Credentials         map[string]string `yaml:"credentials"`
	AuthConfig          AuthConfig        `yaml:"auth"`
	RepoTracker         RepoTrackerConfig `yaml:"repotracker"`
	Monitor             MonitorConfig     `yaml:"monitor"`
	Api                 APIConfig         `yaml:"api"`
	Alerts              AlertsConfig      `yaml:"alerts"`
	Ui                  UIConfig          `yaml:"ui"`
	HostInit            HostInitConfig    `yaml:"hostinit"`
	Notify              NotifyConfig      `yaml:"notify"`
	Runner              RunnerConfig      `yaml:"runner"`
	Scheduler           SchedulerConfig   `yaml:"scheduler"`
	TaskRunner          TaskRunnerConfig  `yaml:"taskrunner"`
	Expansions          map[string]string `yaml:"expansions"`
	Plugins             PluginConfig      `yaml:"plugins"`
	IsProd              bool              `yaml:"isprod"`
}

// NewSettings builds an in-memory representation of the given settings file.
func NewSettings(filename string) (*Settings, error) {
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	settings := &Settings{}
	err = yaml.Unmarshal(configData, settings)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// Validate checks the settings and returns nil if the config is valid,
// or an error with a message explaining why otherwise.
func (settings *Settings) Validate(validators []ConfigValidator) error {
	for _, validator := range validators {
		err := validator(settings)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetSettingsOrExit loads the evergreen settings file,
// or exits with a non-0 code if any errors occur.
func GetSettingsOrExit() *Settings {
	settings, err := GetSettings()
	if err != nil {
		// don't use the logger here, since it needs settings to initialize
		fmt.Fprintf(os.Stderr, "Error reading config file: %v", err)
		os.Exit(1)
	}
	return settings
}

// GetSettings returns Evergreen Settings or an error.
func GetSettings() (*Settings, error) {
	var settingsPath = flag.String("conf", DefaultConfFile, "path to config file")
	flag.Parse()
	settings, err := NewSettings(*settingsPath)
	if err != nil {
		return nil, err
	}
	err = settings.Validate(ConfigValidationRules)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// ConfigValidator is a type of function that checks the settings
// struct for any errors or missing required fields.
type ConfigValidator func(settings *Settings) error

// ConfigValidationRules is the set of all ConfigValidator functions.
var ConfigValidationRules = []ConfigValidator{
	func(settings *Settings) error {
		if settings.Database.Url == "" || settings.Database.DB == "" {
			return fmt.Errorf("DBUrl and DB must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.ApiUrl == "" {
			return fmt.Errorf("API hostname must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.Secret == "" {
			return fmt.Errorf("UI Secret must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.ConfigDir == "" {
			return fmt.Errorf("Config directory must not be empty")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.DefaultProject == "" {
			return fmt.Errorf("You must specify a default project in UI")
		}
		return nil
	},

	func(settings *Settings) error {
		if settings.Ui.Url == "" {
			return fmt.Errorf("You must specify a default UI url")
		}
		return nil
	},

	func(settings *Settings) error {
		notifyConfig := settings.Notify.SMTP

		if notifyConfig == nil {
			return nil
		}

		if notifyConfig.Server == "" || notifyConfig.Port == 0 {
			return fmt.Errorf("You must specify a SMTP server and port")
		}

		if len(notifyConfig.AdminEmail) == 0 {
			return fmt.Errorf("You must specify at least one admin_email")
		}

		if notifyConfig.From == "" {
			return fmt.Errorf("You must specify a from address")
		}

		return nil
	},

	func(settings *Settings) error {
		if settings.AuthConfig.Crowd == nil && settings.AuthConfig.Naive == nil && settings.AuthConfig.Github == nil {
			return fmt.Errorf("You must specify one form of authentication")
		}
		if settings.AuthConfig.Naive != nil {
			used := map[string]bool{}
			for _, x := range settings.AuthConfig.Naive.Users {
				if used[x.Username] {
					return fmt.Errorf("Duplicate user in list")
				}
				used[x.Username] = true
			}
		}
		if settings.AuthConfig.Github != nil {
			if settings.AuthConfig.Github.Users == nil && settings.AuthConfig.Github.Organization == "" {
				return fmt.Errorf("Must specify either a set of users or an organization for Github Authentication")
			}
		}
		return nil
	},
}
