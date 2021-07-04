package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
)

var _ csi.IdentityServer = &UpCloudIdentityServer{}

// UpCloudIdentityServer
type UpCloudIdentityServer struct {
	Driver *UpCloudDriver
}

func NewUpCloudIdentityServer(driver *UpCloudDriver) *UpCloudIdentityServer {
	return &UpCloudIdentityServer{driver}
}

// GetPluginInfo returns basic plugin data
func (upCloudIdentity *UpCloudIdentityServer) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	upCloudIdentity.Driver.log.Info("UpCloudIdentityServer.GetPluginInfo called")

	res := &csi.GetPluginInfoResponse{
		Name:          upCloudIdentity.Driver.drivername,
		VendorVersion: upCloudIdentity.Driver.version,
	}
	return res, nil
}

// GetPluginCapabilities returns plugins available capabilities
func (upCloudIdentity *UpCloudIdentityServer) GetPluginCapabilities(_ context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	upCloudIdentity.Driver.log.Infof("upCloudIdentityServer.GetPluginCapabilities called with request : %v", req)

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

func (upCloudIdentity *UpCloudIdentityServer) Probe(_ context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	upCloudIdentity.Driver.log.Infof("upCloudIdentityServer.Probe called with request : %v", req)

	return &csi.ProbeResponse{
		Ready: &wrappers.BoolValue{Value: true},
	}, nil
}
