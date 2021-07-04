package driver

import (
	"context"
	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

const (
	minVolumeSizeInBytes      int64 = 1 * giB
	maxVolumeSizeInBytes      int64 = 10 * tiB
	defaultVolumeSizeInBytes  int64 = 10 * giB
	volumeStatusCheckRetries        = 15
	volumeStatusCheckInterval       = 1
)

var (
	supportedVolCapabilities = &csi.VolumeCapability_AccessMode{
		Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
)

var _ csi.ControllerServer = &UpCloudControllerServer{}

type UpCloudControllerServer struct {
	Driver *UpCloudDriver
}

func NewUpCloudControllerServer(driver *UpCloudDriver) *UpCloudControllerServer {
	return &UpCloudControllerServer{Driver: driver}
}

// CreateVolume provisions a new volume on behalf of the user
func (c *UpCloudControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volName := req.Name
	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name is missing")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume Capabilities is missing")
	}

	// Validate
	if !isValidCapability(req.VolumeCapabilities) {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume Volume capability is not compatible: %v", req)
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-name":  volName,
		"capabilities": req.VolumeCapabilities,
	}).Info("Create Volume: called")

	// check that the volume doesnt already exist
	storages, err := c.Driver.upCloudClient.ListStorages()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume Volume: List volume error %s", err.Error())
	}
	for _, storage := range storages.Storages.Storage {
		if storage.Title == volName {
			return &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      storage.UUID,
					CapacityBytes: int64(storage.Size) * giB,
				},
			}, nil
		}
	}

	// if applicable, create volume
	size, err := getStorageBytes(req.CapacityRange)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid volume capacity range: %v", err)
	}
	newStorage, err := c.Driver.upCloudClient.CreateStorage(int(size/giB), req.Parameters["tier"], volName, c.Driver.region)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error create volume: %s", err.Error())
	}

	res := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      newStorage.Storage.UUID,
			CapacityBytes: size,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						"region": c.Driver.region,
					},
				},
			},
		},
	}

	c.Driver.log.WithFields(logrus.Fields{
		"size":        size,
		"volume-id":   newStorage.Storage.UUID,
		"volume-name": volName,
		"volume-size": int(size / giB),
	}).Info("Create Volume: created volume")

	return res, nil
}

func (c *UpCloudControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume VolumeID is missing")
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
	}).Info("Delete volume: called")

	exists := false
	storages, err := c.Driver.upCloudClient.ListStorages()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Delete Volume: List volume error %s", err.Error())
	}
	for _, storage := range storages.Storages.Storage {
		if storage.UUID == req.VolumeId {
			exists = true
		}
	}
	if !exists {
		return &csi.DeleteVolumeResponse{}, nil
	}
	// TODO: detach just to be safe

	err = c.Driver.upCloudClient.DeleteStorage(req.VolumeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Delete Volume: %s", err.Error())
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (c *UpCloudControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID is missing")
	}

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID is missing")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume VolumeCapability is missing")
	}

	if req.Readonly {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume read only is not currently supported")
	}
	_, err := c.Driver.upCloudClient.StorageDetail(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume read volume error: "+err.Error())
	}

	serverDetail, err := c.Driver.upCloudClient.ServerDetail(req.NodeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume read server error: "+err.Error())
	}

	mountID := ""
	alreadyMounted := false
	for _, dev := range serverDetail.Server.StorageDevices.StorageDevice {
		if req.VolumeId == dev.Storage {
			alreadyMounted = true
			mountID = dev.Address
		}
	}
	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Publish Volume: called")
	if alreadyMounted {
		return &csi.ControllerPublishVolumeResponse{
			PublishContext: map[string]string{
				req.VolumeId: mountID,
			},
		}, nil
	}

	serverInfo, err := c.Driver.upCloudClient.StorageOP(req.NodeId, req.VolumeId, "attach")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerPublishVolume : %s", err.Error())
	}

	for _, dev := range serverInfo.Server.StorageDevices.StorageDevice {
		if dev.Storage == req.VolumeId {
			mountID = dev.Address
		}
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			req.VolumeId: mountID,
		},
	}, nil
}

func (c *UpCloudControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID is missing")
	}

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Node ID is missing")
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Publish Unpublish: called")

	_, err := c.Driver.upCloudClient.StorageDetail(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume read volume error: "+err.Error())
	}

	serverDetail, err := c.Driver.upCloudClient.ServerDetail(req.NodeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume read server error: "+err.Error())
	}

	alreadyMounted := false
	for _, dev := range serverDetail.Server.StorageDevices.StorageDevice {
		if req.VolumeId == dev.Storage {
			alreadyMounted = true
		}
	}
	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Unublish Volume: unpublished")

	if !alreadyMounted {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	_, err = c.Driver.upCloudClient.StorageOP(req.NodeId, req.VolumeId, "detach")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerUnpublishVolume: %s", err.Error())
	}
	now := time.Now()
	for {
		vol, err := c.Driver.upCloudClient.StorageDetail(req.VolumeId)
		if err != nil {
			return nil, status.Error(codes.Internal, "ControllerUnpublishVolume read volume error: "+err.Error())
		}
		if len(vol.Storage.Servers.Server) == 0 {
			break
		}
		if time.Duration(time.Since(now).Seconds()) > 5*time.Second {
			return nil, status.Errorf(codes.Internal, "ControllerUnpublishVolume: waiting operation timeout")
		}
		time.Sleep(5 * time.Second)
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks if requested capabilities are supported
func (c *UpCloudControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID is missing")
	}

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities is missing")
	}

	storages, err := c.Driver.upCloudClient.ListStorages()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume Volume: List volume error %s", err.Error())
	}
	exists := false
	for _, storage := range storages.Storages.Storage {
		if storage.UUID == req.VolumeId {
			exists = true
		}
	}
	if !exists {
		return nil, status.Errorf(codes.NotFound, "cannot get volume: %v", err.Error())
	}

	res := &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: supportedVolCapabilities,
				},
			},
		},
	}

	return res, nil
}

func (c *UpCloudControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	if req.StartingToken != "" {
		_, err := strconv.Atoi(req.StartingToken)
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "ListVolumes starting_token is invalid: %s", err)
		}
	}

	var entries []*csi.ListVolumesResponse_Entry
	storages, err := c.Driver.upCloudClient.ListStorages()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume Volume: List volume error %s", err.Error())
	}
	for _, storage := range storages.Storages.Storage {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      storage.UUID,
				CapacityBytes: int64(storage.Size) * giB,
			},
		})
	}

	res := &csi.ListVolumesResponse{
		Entries: entries,
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volumes": entries,
	}).Info("List Volumes")
	return res, nil
}

func (c *UpCloudControllerServer) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities get capabilities of the controller
func (c *UpCloudControllerServer) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	capability := func(capability csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capability,
				},
			},
		}
	}

	var capabilities []*csi.ControllerServiceCapability
	for _, caps := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
	} {
		capabilities = append(capabilities, capability(caps))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: capabilities,
	}

	c.Driver.log.WithFields(logrus.Fields{
		"response": resp,
		"method":   "controller-get-capabilities",
	})

	return resp, nil
}

func (c *UpCloudControllerServer) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *UpCloudControllerServer) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *UpCloudControllerServer) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *UpCloudControllerServer) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// This relates to being able to get health checks on a PV. We do not have this
func (c *UpCloudControllerServer) ControllerGetVolume(ctx context.Context, request *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func isValidCapability(caps []*csi.VolumeCapability) bool {
	for _, capacity := range caps {
		if capacity == nil {
			return false
		}

		accessMode := capacity.GetAccessMode()
		if accessMode == nil {
			return false
		}

		if accessMode.GetMode() != supportedVolCapabilities.GetMode() {
			return false
		}

		accessType := capacity.GetAccessType()
		switch accessType.(type) {
		case *csi.VolumeCapability_Block:
		case *csi.VolumeCapability_Mount:
		default:
			return false
		}
	}
	return true
}

// getStorageBytes returns storage size in bytes
func getStorageBytes(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return defaultVolumeSizeInBytes, nil
	}

	capacity := capRange.GetRequiredBytes()
	return capacity, nil
}
