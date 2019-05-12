package hcloud

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/state"
	"github.com/docker/machine/version"
	"github.com/hetznercloud/hcloud-go/hcloud"
)

const (
	driverName        = "hcloud"
	defaultImage      = "ubuntu-18.04"
	defaultServerType = "cx11"
)

type Driver struct {
	*drivers.BaseDriver
	serverId   int
	Image      string
	Location   string
	Datacenter string
	ServerType string
	hcloud     *hcloud.Client
}

func (d *Driver) GetSSHHostname() (string, error) {
	return d.GetIP()
}

func (d *Driver) GetURL() (string, error) {
	if err := drivers.MustBeRunning(d); err != nil {
		return "", err
	}

	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("tcp://%s", net.JoinHostPort(ip, "2376")), nil
}

func NewDriver(hostName, storePath string) *Driver {
	return &Driver{
		BaseDriver: &drivers.BaseDriver{
			MachineName: hostName,
			StorePath:   storePath,
		},
	}
}

func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	return []mcnflag.Flag{
		mcnflag.StringFlag{
			EnvVar: "HCLOUD_TOKEN",
			Name:   "hcloud-token",
			Usage:  "URL of host when no driver is selected",
		},
		mcnflag.StringFlag{
			EnvVar: "HCLOUD_IMAGE",
			Name:   "hcloud-image",
			Usage:  "URL of host when no driver is selected",
			Value:  defaultImage,
		},
		mcnflag.StringFlag{
			EnvVar: "HCLOUD_TYPE",
			Name:   "hcloud-type",
			Usage:  "URL of host when no driver is selected",
			Value:  defaultServerType,
		},
		mcnflag.StringFlag{
			EnvVar: "HCLOUD_LOCATION",
			Name:   "hcloud-location",
			Usage:  "URL of host when no driver is selected",
		},
		mcnflag.StringFlag{
			EnvVar: "HCLOUD_DATACENTER",
			Name:   "hcloud-datacenter",
			Usage:  "URL of host when no driver is selected",
		},
	}
}

func (d *Driver) Create() error {
	serverType, _, err := d.hcloud.ServerType.Get(context.TODO(), d.ServerType)
	if err != nil {
		return err
	}

	image, _, err := d.hcloud.Image.Get(context.TODO(), d.Image)
	if err != nil {
		return err
	}
	opts := hcloud.ServerCreateOpts{
		Name:       d.BaseDriver.MachineName,
		ServerType: serverType,
		Image:      image,
	}
	if d.Datacenter != "" {
		datacenter, _, err := d.hcloud.Datacenter.Get(context.TODO(), d.Image)
		if err != nil {
			return err
		}
		opts.Datacenter = datacenter
	}
	if d.Location != "" {
		location, _, err := d.hcloud.Location.Get(context.TODO(), d.Image)
		if err != nil {
			return err
		}
		opts.Location = location
	}
	resp, _, err := d.hcloud.Server.Create(context.TODO(), opts)
	d.waitOnAction(resp.Action)
	d.serverId = resp.Server.ID
	d.IPAddress = resp.Server.PublicNet.IPv4.IP.String()
	return nil
}

// DriverName returns the name of the driver
func (d *Driver) DriverName() string {
	return driverName
}

func (d *Driver) GetState() (state.State, error) {
	server, _, err := d.hcloud.Server.GetByID(context.TODO(), d.serverId)
	if err != nil {
		return state.Error, err
	}
	switch server.Status {
	case hcloud.ServerStatusRunning:
		return state.Running, nil
	case hcloud.ServerStatusOff:
		return state.Stopped, nil
	case hcloud.ServerStatusStopping:
		return state.Stopping, nil
	case hcloud.ServerStatusStarting:
	case hcloud.ServerStatusInitializing:
		return state.Starting, nil
	}
	return state.Running, nil
}

func (d *Driver) Kill() error {
	server, _, err := d.hcloud.Server.GetByID(context.TODO(), d.serverId)
	if err != nil {
		return err
	}
	action, _, err := d.hcloud.Server.Poweroff(context.TODO(), server)
	if err != nil {
		return err
	}
	return d.waitOnAction(action)
}

func (d *Driver) Remove() error {
	server, _, err := d.hcloud.Server.GetByID(context.TODO(), d.serverId)
	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
			log.Printf("HCloud server does not exists.")
			return nil
		}
		return err
	}
	_, err = d.hcloud.Server.Delete(context.TODO(), server)
	return err
}

func (d *Driver) Restart() error {
	server, _, err := d.hcloud.Server.GetByID(context.TODO(), d.serverId)
	if err != nil {
		return err
	}
	action, _, err := d.hcloud.Server.Reboot(context.TODO(), server)
	if err != nil {
		return err
	}
	return d.waitOnAction(action)
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	token := flags.String("hcloud-token")

	if token == "" {
		return fmt.Errorf("--hcloud-token option is required")
	}

	image := flags.String("hcloud-image")

	if image == "" {
		return fmt.Errorf("--hcloud-image option is required")
	}
	d.Image = image

	serverType := flags.String("hcloud-type")

	if serverType == "" {
		return fmt.Errorf("--hcloud-type option is required")
	}
	d.ServerType = serverType
	d.Datacenter = flags.String("hcloud-datacenter")
	d.Location = flags.String("hcloud-location")
	d.hcloud = hcloud.NewClient(hcloud.WithToken(token), hcloud.WithApplication("docker-machine", version.Version))
	return nil
}

func (d *Driver) Start() error {
	server, _, err := d.hcloud.Server.GetByID(context.TODO(), d.serverId)
	if err != nil {
		return err
	}
	action, _, err := d.hcloud.Server.Poweron(context.TODO(), server)
	if err != nil {
		return err
	}
	return d.waitOnAction(action)
}

func (d *Driver) Stop() error {
	server, _, err := d.hcloud.Server.GetByID(context.TODO(), d.serverId)
	if err != nil {
		return err
	}
	action, _, err := d.hcloud.Server.Shutdown(context.TODO(), server)
	if err != nil {
		return err
	}
	return d.waitOnAction(action)
}

func (d *Driver) waitOnAction(action *hcloud.Action) error {
	log.Printf("HCloud waiting for %q action to complete...", action.Command)
	_, errCh := d.hcloud.Action.WatchProgress(context.TODO(), action)
	if err := <-errCh; err != nil {
		return err
	}
	log.Printf("HCloud %q action succeeded", action.Command)
	return nil
}
