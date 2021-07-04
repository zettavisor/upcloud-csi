package driver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var UpCloudApiEndpoint = "https://api.upcloud.com"
var UpCloudApiVersion = "1.3"

type UpCloudClient struct {
	token      string
	httpClient *http.Client
}

func NewUpCloudClient(token string) *UpCloudClient {
	return &UpCloudClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // big timeout for vol attach and detatch
		},
	}
}

func (client *UpCloudClient) getPath(apis ...string) string {
	u, err := url.Parse(UpCloudApiEndpoint)
	if err != nil {
		panic("error config UpCloudApiEndpoint " + UpCloudApiEndpoint)
	}
	u.Path = path.Join(u.Path, UpCloudApiVersion)
	for _, a := range apis {
		u.Path = path.Join(u.Path, a)
	}
	return u.String()
}

func (client *UpCloudClient) simpleHTTPGet(path ...string) ([]byte, error) {
	req, err := http.NewRequest("GET", client.getPath(path...), nil)
	if err != nil {
		return nil, errors.Wrap(err, "during create request")
	}
	req.Header.Set("authorization", "Basic "+client.token)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "during send request")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "during read body")
	}
	if resp.StatusCode >= 300 {
		fmt.Println("[ERROR] response body:\n", string(body))
		return nil, errors.New("error response status " + resp.Status)
	}
	return body, nil
}

func (client *UpCloudClient) ListStorages() (*UpCloudListStoragesResp, error) {
	body, err := client.simpleHTTPGet("storage", "private")
	if err != nil {
		return nil, err
	}
	var respData UpCloudListStoragesResp
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return nil, errors.Wrap(err, "during unmarshal body")
	}
	return &respData, nil
}

func (client *UpCloudClient) ListStoragesWithDetails() (*UpCloudListStoragesResp, error) {
	storages, err := client.ListStorages()
	if err != nil {
		return nil, err
	}
	for index := range storages.Storages.Storage {
		storage := storages.Storages.Storage[index]
		detail, err := client.StorageDetail(storage.UUID)
		if err != nil {
			return nil, err
		}
		storages.Storages.Storage[index].Details = detail
	}
	return storages, nil
}

func (client *UpCloudClient) CreateStorage(size int, tier, title, zone string) (*UpCloudCreateStorageResp, error) {
	reqbody := fmt.Sprintf(`{"storage":{"size":"%d","tier":"%s","title":"%s","zone":"%s"}}`, size, tier, title, zone)
	req, err := http.NewRequest("POST", client.getPath("storage"), strings.NewReader(
		reqbody,
	))
	if err != nil {
		return nil, errors.Wrap(err, "during create request")
	}
	req.Header.Set("authorization", "Basic "+client.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "during send request")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "during read body")
	}
	if resp.StatusCode >= 300 {
		fmt.Println("[ERROR] response body:\n", string(body))
		return nil, errors.New("error response status " + resp.Status)
	}
	var dat UpCloudCreateStorageResp
	err = json.Unmarshal(body, &dat)
	if err != nil {
		return nil, errors.Wrap(err, "during unmarshal body")
	}
	return &dat, nil
}

func (client *UpCloudClient) DeleteStorage(UUID string) error {
	req, err := http.NewRequest("DELETE", client.getPath("storage", UUID)+"?backups=delete", nil)
	if err != nil {
		return errors.Wrap(err, "during create request")
	}
	req.Header.Set("authorization", "Basic "+client.token)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "during send request")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return errors.New("error response status " + resp.Status)
	}
	return nil
}

func (client *UpCloudClient) StorageOP(serverID, volID, OP string) (*UpCloudStorageOPResp, error) {
	if OP != "attach" && OP != "detach" {
		return nil, errors.New("not recognise OP")
	}
	reqBody := fmt.Sprintf(`{"storage_device":{"address":"virtio","storage":"%s"}}`, volID)
	if OP == "detach" {
		reqBody = fmt.Sprintf(`{"storage_device":{"storage":"%s"}}`, volID)
	}
	req, err := http.NewRequest("POST", client.getPath("server", serverID, "storage", OP), strings.NewReader(
		reqBody,
	))
	if err != nil {
		return nil, errors.Wrap(err, "during create request")
	}
	req.Header.Set("authorization", "Basic "+client.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "during send request")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "during read body")
	}
	if resp.StatusCode >= 300 {
		fmt.Println("[ERROR] response body:\n", string(body))
		return nil, errors.New("error response status " + resp.Status)
	}
	var dat UpCloudStorageOPResp
	err = json.Unmarshal(body, &dat)
	if err != nil {
		return nil, errors.Wrap(err, "during unmarshal body")
	}
	return &dat, nil
}

type UpCloudStorageOPResp struct {
	Server struct {
		BootOrder   string `json:"boot_order"`
		CoreNumber  string `json:"core_number"`
		Created     int    `json:"created"`
		Firewall    string `json:"firewall"`
		Host        int64  `json:"host"`
		Hostname    string `json:"hostname"`
		IPAddresses struct {
			IPAddress []struct {
				Access     string `json:"access"`
				Address    string `json:"address"`
				Family     string `json:"family"`
				PartOfPlan string `json:"part_of_plan,omitempty"`
			} `json:"ip_address"`
		} `json:"ip_addresses"`
		License      int    `json:"license"`
		MemoryAmount string `json:"memory_amount"`
		Metadata     string `json:"metadata"`
		Networking   struct {
			Interfaces struct {
				Interface []struct {
					Bootable    string `json:"bootable"`
					Index       int    `json:"index"`
					IPAddresses struct {
						IPAddress []struct {
							Address  string `json:"address"`
							Family   string `json:"family"`
							Floating string `json:"floating"`
						} `json:"ip_address"`
					} `json:"ip_addresses"`
					Mac               string `json:"mac"`
					Network           string `json:"network"`
					SourceIPFiltering string `json:"source_ip_filtering"`
					Type              string `json:"type"`
				} `json:"interface"`
			} `json:"interfaces"`
		} `json:"networking"`
		NicModel             string `json:"nic_model"`
		Plan                 string `json:"plan"`
		PlanIpv4Bytes        string `json:"plan_ipv4_bytes"`
		PlanIpv6Bytes        string `json:"plan_ipv6_bytes"`
		RemoteAccessEnabled  string `json:"remote_access_enabled"`
		RemoteAccessPassword string `json:"remote_access_password"`
		RemoteAccessType     string `json:"remote_access_type"`
		SimpleBackup         string `json:"simple_backup"`
		State                string `json:"state"`
		StorageDevices       struct {
			StorageDevice []struct {
				Address      string `json:"address"`
				BootDisk     string `json:"boot_disk"`
				PartOfPlan   string `json:"part_of_plan,omitempty"`
				Storage      string `json:"storage"`
				StorageSize  int    `json:"storage_size"`
				StorageTier  string `json:"storage_tier"`
				StorageTitle string `json:"storage_title"`
				Type         string `json:"type"`
			} `json:"storage_device"`
		} `json:"storage_devices"`
		Tags struct {
			Tag []interface{} `json:"tag"`
		} `json:"tags"`
		Timezone   string `json:"timezone"`
		Title      string `json:"title"`
		UUID       string `json:"uuid"`
		VideoModel string `json:"video_model"`
		Zone       string `json:"zone"`
	} `json:"server"`
}

type UpCloudCreateStorageResp struct {
	Storage struct {
		Access     string `json:"access"`
		BackupRule struct {
		} `json:"backup_rule"`
		Backups struct {
			Backup []interface{} `json:"backup"`
		} `json:"backups"`
		Created time.Time `json:"created"`
		License int       `json:"license"`
		Servers struct {
			Server []interface{} `json:"server"`
		} `json:"servers"`
		Size  int    `json:"size"`
		State string `json:"state"`
		Tier  string `json:"tier"`
		Title string `json:"title"`
		Type  string `json:"type"`
		UUID  string `json:"uuid"`
		Zone  string `json:"zone"`
	} `json:"storage"`
}

func (client *UpCloudClient) StorageDetail(UUID string) (*UpCloudListStorageDetailsResp, error) {
	body, err := client.simpleHTTPGet("storage", UUID)
	if err != nil {
		return nil, err
	}
	var respData UpCloudListStorageDetailsResp
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return nil, errors.Wrap(err, "during unmarshal body")
	}
	return &respData, nil
}

type UpCloudListStoragesResp struct {
	Storages struct {
		Storage []struct {
			Access     string    `json:"access"`
			Created    time.Time `json:"created"`
			License    int       `json:"license"`
			Size       int       `json:"size"`
			State      string    `json:"state"`
			Tier       string    `json:"tier"`
			Title      string    `json:"title"`
			Type       string    `json:"type"`
			UUID       string    `json:"uuid"`
			Zone       string    `json:"zone"`
			PartOfPlan string    `json:"part_of_plan,omitempty"`
			Details    *UpCloudListStorageDetailsResp
		} `json:"storage"`
	} `json:"storages"`
}

type UpCloudListStorageDetailsResp struct {
	Storage struct {
		Access  string `json:"access"`
		License int    `json:"license"`
		Servers struct {
			Server []string `json:"server"`
		} `json:"servers"`
		Size  int    `json:"size"`
		State string `json:"state"`
		Tier  string `json:"tier"`
		Title string `json:"title"`
		Type  string `json:"type"`
		UUID  string `json:"uuid"`
		Zone  string `json:"zone"`
	} `json:"storage"`
}

func (client *UpCloudClient) ListServers() (*UpCloudListServerResp, error) {
	body, err := client.simpleHTTPGet("server")
	if err != nil {
		return nil, err
	}
	var respData UpCloudListServerResp
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return nil, errors.Wrap(err, "during unmarshal body")
	}
	return &respData, nil
}

type UpCloudListServerResp struct {
	Servers struct {
		Server []struct {
			CoreNumber    string `json:"core_number"`
			Hostname      string `json:"hostname"`
			License       int    `json:"license"`
			MemoryAmount  string `json:"memory_amount"`
			Plan          string `json:"plan"`
			PlanIvp4Bytes string `json:"plan_ivp4_bytes,omitempty"`
			PlanIpv6Bytes string `json:"plan_ipv6_bytes,omitempty"`
			State         string `json:"state"`
			Tags          struct {
				Tag []string `json:"tag"`
			} `json:"tags"`
			Title string `json:"title"`
			UUID  string `json:"uuid"`
			Zone  string `json:"zone"`
		} `json:"server"`
	} `json:"servers"`
}

func (client *UpCloudClient) ServerDetail(UUID string) (*UpCloudServerDetailResp, error) {
	body, err := client.simpleHTTPGet("server", UUID)
	if err != nil {
		return nil, err
	}
	var respData UpCloudServerDetailResp
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return nil, errors.Wrap(err, "during unmarshal body")
	}
	return &respData, nil
}

type UpCloudServerDetailResp struct {
	Server struct {
		BootOrder   string `json:"boot_order"`
		CoreNumber  string `json:"core_number"`
		Firewall    string `json:"firewall"`
		Host        int64  `json:"host"`
		Hostname    string `json:"hostname"`
		IPAddresses struct {
			IPAddress []struct {
				Access  string `json:"access"`
				Address string `json:"address"`
				Family  string `json:"family"`
			} `json:"ip_address"`
		} `json:"ip_addresses"`
		License      int    `json:"license"`
		MemoryAmount string `json:"memory_amount"`
		Networking   struct {
			Interfaces struct {
				Interface []struct {
					Index       int `json:"index"`
					IPAddresses struct {
						IPAddress []struct {
							Address  string `json:"address"`
							Family   string `json:"family"`
							Floating string `json:"floating"`
						} `json:"ip_address"`
					} `json:"ip_addresses"`
					Mac      string `json:"mac"`
					Network  string `json:"network"`
					Type     string `json:"type"`
					Bootable string `json:"bootable"`
				} `json:"interface"`
			} `json:"interfaces"`
		} `json:"networking"`
		NicModel       string `json:"nic_model"`
		Plan           string `json:"plan"`
		PlanIpv4Bytes  string `json:"plan_ipv4_bytes"`
		PlanIpv6Bytes  string `json:"plan_ipv6_bytes"`
		SimpleBackup   string `json:"simple_backup"`
		State          string `json:"state"`
		StorageDevices struct {
			StorageDevice []struct {
				Address      string `json:"address"`
				PartOfPlan   string `json:"part_of_plan"`
				Storage      string `json:"storage"`
				StorageSize  int    `json:"storage_size"`
				StorageTier  string `json:"storage_tier"`
				StorageTitle string `json:"storage_title"`
				Type         string `json:"type"`
				BootDisk     string `json:"boot_disk"`
			} `json:"storage_device"`
		} `json:"storage_devices"`
		Tags struct {
			Tag []string `json:"tag"`
		} `json:"tags"`
		Timezone             string `json:"timezone"`
		Title                string `json:"title"`
		UUID                 string `json:"uuid"`
		VideoModel           string `json:"video_model"`
		RemoteAccessEnabled  string `json:"remote_access_enabled"`
		RemoteAccessType     string `json:"remote_access_type"`
		RemoteAccessHost     string `json:"remote_access_host"`
		RemoteAccessPassword string `json:"remote_access_password"`
		RemoteAccessPort     string `json:"remote_access_port"`
		Zone                 string `json:"zone"`
	} `json:"server"`
}
