package driver

import (
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	DefaultDriverName = "block.csi.upcloud.com"
	defaultTimeout    = 3 * time.Minute
)

// UpCloudDriver struct
type UpCloudDriver struct {
	drivername string
	nodeID     string
	region     string
	endpoint   string
	token      string

	publishVolumeID string
	mountID         string

	isController bool
	waitTimeout  time.Duration

	log     *logrus.Entry
	mounter Mounter

	version string

	upCloudClient *UpCloudClient
}

func NewDriver(endpoint, token, driverName, version, userAgent string, isController bool) (*UpCloudDriver, error) {
	if driverName == "" {
		driverName = DefaultDriverName
	}
	hostName := os.Getenv("KUBE_NODE_NAME")
	if hostName == "" {
		panic("KUBE_NODE_NAME is not set")
	}
	upCloudClient := NewUpCloudClient(token)
	servers, err := upCloudClient.ListServers()
	if err != nil {
		panic("error get servers" + err.Error())
	}
	hostID := ""
	region := ""
	exist := false
	for _, server := range servers.Servers.Server {
		if server.Hostname == hostName {
			exist = true
			hostID = server.UUID
			region = server.Zone
		}
	}
	if !exist {
		panic("server not found in cloud platform")
	}

	log := logrus.New().WithFields(logrus.Fields{
		"region":  hostID,
		"host_id": region,
		"version": version,
	})

	return &UpCloudDriver{
		drivername: driverName,
		nodeID:     hostID,
		region:     region,
		endpoint:   endpoint,
		token:      token,

		isController: isController,
		waitTimeout:  defaultTimeout,

		log:     log,
		mounter: NewMounter(log),

		version: version,

		upCloudClient: upCloudClient,
	}, nil
}

func (d *UpCloudDriver) Run() {
	server := NewNonBlockingGRPCServer()
	identity := NewUpCloudIdentityServer(d)
	controller := NewUpCloudControllerServer(d)
	node := NewUpCloudNodeDriver(d)

	server.Start(d.endpoint, identity, controller, node)
	server.Wait()
}
