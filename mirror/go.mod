module 10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	10.1.1.220/cdm/cdm-center/services/cluster-manager v0.1.0-rc8.0.20230821051922-e5b9a61b38c0
	10.1.1.220/cdm/cdm-cloud/common v1.0.4
	10.1.1.220/cdm/cdm-disaster-recovery/common v1.0.1-0.20231108071121-cf6f011b0d38
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/google/uuid v1.3.0
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
	github.com/shirou/gopsutil v3.21.6+incompatible
	github.com/stretchr/testify v1.8.1
	github.com/tklauser/go-sysconf v0.3.7 // indirect
)
