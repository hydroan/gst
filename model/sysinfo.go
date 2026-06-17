package model

import (
	"github.com/cockroachdb/errors"
	"gorm.io/datatypes"
)

type SysInfo struct {
	Node    Node    `json:"node" gorm:"embedded;embeddedPrefix:node_"`
	OS      OS      `json:"os" gorm:"embedded;embeddedPrefix:os_"`
	Kernel  Kernel  `json:"kernel" gorm:"embedded;embeddedPrefix:kernel_"`
	Product Product `json:"product" gorm:"embedded;embeddedPrefix:product_"`
	Board   Board   `json:"board" gorm:"embedded;embeddedPrefix:board_"`
	Chassis Chassis `json:"chassis" gorm:"embedded;embeddedPrefix:chassis_"`
	BIOS    BIOS    `json:"bios" gorm:"embedded;embeddedPrefix:bios_"`
	CPU     CPU     `json:"cpu" gorm:"embedded;embeddedPrefix:cpu_"`
	Memory  Memory  `json:"memory" gorm:"embedded;embeddedPrefix:memory_"`

	Storages StorageDevices `json:"storages,omitempty"`
	Networks NetworkDevices `json:"networks,omitempty"`

	Base
}

func (si *SysInfo) CreateBefore() error {
	if len(si.ID) == 0 {
		if len(si.Node.MachineID) == 0 {
			return errors.New("machine id is empty")
		}
		si.ID = si.Node.MachineID
	}
	return nil
}

func (si *SysInfo) UpdateBefore() error { return si.CreateBefore() }

type Node struct {
	Hostname   string `json:"hostname,omitempty"`
	MachineID  string `json:"machineid,omitempty"`
	Hypervisor string `json:"hypervisor,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
}

type OS struct {
	Name         string `json:"name,omitempty"`
	Vendor       string `json:"vendor,omitempty"`
	Version      string `json:"version,omitempty"`
	Release      string `json:"release,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

type Kernel struct {
	Release      string `json:"release,omitempty"`
	Version      string `json:"version,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}
type Product struct {
	Name    string `json:"name,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
	Version string `json:"version,omitempty"`
	Serial  string `json:"serial,omitempty"`
	UUID    string `json:"uuid,omitempty"`
	SKU     string `json:"sku,omitempty"`
}
type Board struct {
	Name     string `json:"name,omitempty"`
	Vendor   string `json:"vendor,omitempty"`
	Version  string `json:"version,omitempty"`
	Serial   string `json:"serial,omitempty"`
	AssetTag string `json:"assettag,omitempty"`
}
type Chassis struct {
	Type     uint   `json:"type,omitempty"`
	Vendor   string `json:"vendor,omitempty"`
	Version  string `json:"version,omitempty"`
	Serial   string `json:"serial,omitempty"`
	AssetTag string `json:"assettag,omitempty"`
}
type BIOS struct {
	Vendor  string `json:"vendor,omitempty"`
	Version string `json:"version,omitempty"`
	Date    string `json:"date,omitempty"`
}
type CPU struct {
	Vendor  string `json:"vendor,omitempty"`
	Model   string `json:"model,omitempty"`
	Speed   uint   `json:"speed,omitempty"`   // CPU clock rate in MHz
	Cache   uint   `json:"cache,omitempty"`   // CPU cache size in KB
	Cpus    uint   `json:"cpus,omitempty"`    // number of physical CPUs
	Cores   uint   `json:"cores,omitempty"`   // number of physical CPU cores
	Threads uint   `json:"threads,omitempty"` // number of logical (HT) CPU cores
}
type Memory struct {
	Type  string `json:"type,omitempty"`
	Speed uint   `json:"speed,omitempty"` // RAM data rate in MT/s
	Size  uint   `json:"size,omitempty"`  // RAM size in MB
}
type StorageDevice struct {
	Name   string `json:"name,omitempty"`
	Driver string `json:"driver,omitempty"`
	Vendor string `json:"vendor,omitempty"`
	Model  string `json:"model,omitempty"`
	Serial string `json:"serial,omitempty"`
	Size   uint   `json:"size,omitempty"` // device size in GB
}
type NetworkDevice struct {
	Name       string `json:"name,omitempty"`
	Driver     string `json:"driver,omitempty"`
	MACAddress string `json:"macaddress,omitempty"`
	Port       string `json:"port,omitempty"`
	Speed      uint   `json:"speed,omitempty"` // device max supported speed in Mbps
}
type (
	StorageDevices = datatypes.JSONSlice[StorageDevice]
	NetworkDevices = datatypes.JSONSlice[NetworkDevice]
)
