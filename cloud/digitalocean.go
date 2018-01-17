package cloud

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	digo "github.com/dynport/gocloud/digitalocean"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	DigitalOceanStatusOff     = "off"
	DigitalOceanStatusNew     = "new"
	DigitalOceanStatusActive  = "active"
	DigitalOceanStatusArchive = "archive"
)

type doManager struct {
	account *digo.Account
}

type doSettings struct {
	ImageId  int `mapstructure:"image_id" json:"image_id" bson:"image_id"`
	SizeId   int `mapstructure:"size_id" json:"size_id" bson:"size_id"`
	RegionId int `mapstructure:"region_id" json:"region_id" bson:"region_id"`
	SSHKeyId int `mapstructure:"ssh_key_id" json:"ssh_key_id" bson:"ssh_key_id"`
}

var (
	// bson fields for the doSettings struct
	ImageIdKey  = bsonutil.MustHaveTag(doSettings{}, "ImageId")
	SizeIdKey   = bsonutil.MustHaveTag(doSettings{}, "SizeId")
	RegionIdKey = bsonutil.MustHaveTag(doSettings{}, "RegionId")
	SSHKeyIdKey = bsonutil.MustHaveTag(doSettings{}, "SSHKeyId")
)

//Validate checks that the settings from the config file are sane.
func (self *doSettings) Validate() error {
	if self.ImageId == 0 {
		return errors.New("ImageId must not be blank")
	}

	if self.SizeId == 0 {
		return errors.New("Size ID must not be blank")
	}

	if self.RegionId == 0 {
		return errors.New("Region must not be blank")
	}

	if self.SSHKeyId == 0 {
		return errors.New("SSH Key ID must not be blank")
	}

	return nil
}

func (_ *doManager) GetSettings() ProviderSettings {
	return &doSettings{}
}

//GetInstanceName returns a name to be used for an instance
func (*doManager) GetInstanceName(_d *distro.Distro) string {
	return fmt.Sprintf("droplet-%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

//SpawnHost creates a new droplet for the given distro.
func (digoMgr *doManager) SpawnHost(h *host.Host) (*host.Host, error) {
	if h.Distro.Provider != evergreen.ProviderNameDigitalOcean {
		return nil, errors.Errorf("Can't spawn instance of %s for distro %s: provider is %s",
			evergreen.ProviderNameDigitalOcean, h.Distro.Id, h.Distro.Provider)
	}

	digoSettings := &doSettings{}
	if err := mapstructure.Decode(h.Distro.ProviderSettings, digoSettings); err != nil {
		return nil, errors.Wrapf(err, "Error decoding params for distro %v", h.Distro.Id)
	}

	if err := digoSettings.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Invalid DigitalOcean settings in distro %v", h.Distro.Id)
	}

	dropletReq := &digo.Droplet{
		SizeId:   digoSettings.SizeId,
		ImageId:  digoSettings.ImageId,
		RegionId: digoSettings.RegionId,
		Name:     h.Id,
		SshKey:   digoSettings.SSHKeyId,
	}

	newDroplet, err := digoMgr.account.CreateDroplet(dropletReq)
	if err != nil {
		grip.Errorf("DigitalOcean create droplet API call failed for intent host '%v': %+v",
			h.Id, err)

		// remove the intent host document
		rmErr := h.Remove()
		if rmErr != nil {
			err = errors.Errorf("Could not remove intent host '%v': %+v",
				h.Id, rmErr)
			grip.Error(err)
			return nil, err
		}
		return nil, err
	}

	// the document is updated later in hostinit, rather than here
	h.Id = fmt.Sprintf("%v", newDroplet.Id)
	h.Host = newDroplet.IpAddress
	event.LogHostStarted(h.Id)

	return h, nil

}

//getDropletInfo is a helper function to retrieve metadata about a droplet by
//querying DigitalOcean's API directly.
func (digoMgr *doManager) getDropletInfo(dropletId int) (*digo.Droplet, error) {
	droplet := digo.Droplet{Id: dropletId}
	droplet.Account = digoMgr.account
	if err := droplet.Reload(); err != nil {
		return nil, errors.WithStack(err)
	}
	return &droplet, nil
}

//GetInstanceStatus returns a universal status code representing the state
//of a droplet.
func (digoMgr *doManager) GetInstanceStatus(host *host.Host) (CloudStatus, error) {
	hostIdAsInt, err := strconv.Atoi(host.Id)
	if err != nil {
		err = errors.Wrapf(err, "Can't get status of '%v': DigitalOcean host id's "+
			"must be integers", host.Id)
		grip.Error(err)
		return StatusUnknown, err

	}
	droplet, err := digoMgr.getDropletInfo(hostIdAsInt)
	if err != nil {
		return StatusUnknown, errors.Wrap(err, "Failed to get droplet info")
	}

	switch droplet.Status {
	case DigitalOceanStatusNew:
		return StatusInitializing, nil
	case DigitalOceanStatusActive:
		return StatusRunning, nil
	case DigitalOceanStatusArchive:
		return StatusStopped, nil
	case DigitalOceanStatusOff:
		return StatusTerminated, nil
	default:
		return StatusUnknown, nil
	}
}

//GetDNSName gets the DNS hostname of a droplet by reading it directly from
//the DigitalOcean API
func (digoMgr *doManager) GetDNSName(host *host.Host) (string, error) {
	hostIdAsInt, err := strconv.Atoi(host.Id)
	if err != nil {
		err = errors.Wrapf(err, "Can't get DNS name of '%v': DigitalOcean host id's must be integers",
			host.Id)
		grip.Error(err)
		return "", err
	}

	droplet, err := digoMgr.getDropletInfo(hostIdAsInt)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return droplet.IpAddress, nil

}

//CanSpawn returns if a given cloud provider supports spawning a new host
//dynamically. Always returns true for DigitalOcean.
func (digoMgr *doManager) CanSpawn() (bool, error) {
	return true, nil
}

//TerminateInstance destroys a droplet.
func (digoMgr *doManager) TerminateInstance(host *host.Host, user string) error {
	hostIdAsInt, err := strconv.Atoi(host.Id)
	if err != nil {
		err = errors.Wrapf(err, "Can't terminate '%v': DigitalOcean host id's must be integers", host.Id)
		grip.Error(err)
		return err
	}
	response, err := digoMgr.account.DestroyDroplet(hostIdAsInt)
	if err != nil {
		err = errors.Wrapf(err, "Failed to destroy droplet '%v'", host.Id)
		grip.Error(err)
		return err
	}

	if response.Status != "OK" {
		err = errors.Wrapf(err, "Failed to destroy droplet: '%+v'. message: %v",
			response.ErrorMessage)
		grip.Error(err)
		return err
	}

	return errors.WithStack(host.Terminate(evergreen.User))
}

//Configure populates a doManager by reading relevant settings from the
//config object.
func (digoMgr *doManager) Configure(settings *evergreen.Settings) error {
	digoMgr.account = digo.NewAccount(settings.Providers.DigitalOcean.ClientId,
		settings.Providers.DigitalOcean.Key)
	return nil
}

//IsSSHReachable checks if a droplet appears to be reachable via SSH by
//attempting to contact the host directly.
func (digoMgr *doManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := digoMgr.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, errors.WithStack(err)
	}

	ok, err := hostutil.CheckSSHResponse(context.TODO(), host, sshOpts)
	return ok, errors.WithStack(err)
}

//IsUp checks the droplet's state by querying the DigitalOcean API and
//returns true if the host should be available to connect with SSH.
func (digoMgr *doManager) IsUp(host *host.Host) (bool, error) {
	cloudStatus, err := digoMgr.GetInstanceStatus(host)
	if err != nil {
		return false, errors.WithStack(err)
	}
	if cloudStatus == StatusRunning {
		return true, nil
	}
	return false, nil
}

func (digoMgr *doManager) OnUp(host *host.Host) error {
	//Currently a no-op as DigitalOcean doesn't support tags.
	return nil
}

//GetSSHOptions returns an array of default SSH options for connecting to a
//droplet.
func (digoMgr *doManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, errors.New("No key specified for DigitalOcean host")
	}
	opts := []string{"-i", keyPath}
	for _, opt := range host.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host
func (digoMgr *doManager) TimeTilNextPayment(host *host.Host) time.Duration {
	now := time.Now()

	// the time since the host was created
	timeSinceCreation := now.Sub(host.CreationTime)

	// the hours since the host was created, rounded up
	hoursRoundedUp := time.Duration(math.Ceil(timeSinceCreation.Hours()))

	// the next round number of hours the host will have been up - the time
	// that the next payment will be due
	nextPaymentTime := host.CreationTime.Add(hoursRoundedUp)

	return nextPaymentTime.Sub(now)
}
