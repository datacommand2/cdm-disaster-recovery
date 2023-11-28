module github.com/datacommand2/cdm-disaster-recovery/mirror

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231127065214-299269158e0f
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231127061122-07e02be5bd0c
	github.com/datacommand2/cdm-disaster-recovery/common v0.0.0-20231128001023-78919424bb16
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
	github.com/shirou/gopsutil v3.21.6+incompatible
	github.com/tklauser/go-sysconf v0.3.7 // indirect
)
