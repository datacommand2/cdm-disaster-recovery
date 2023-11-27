module github.com/datacommand2/cdm-disaster-recovery/mirror

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231127012344-99c2aa35af23
    github.com/datacommand2/cdm-cloud/common v0.0.0-20231124062432-069e9eb1c852
	github.com/datacommand2/cdm-disaster-recovery/common v0.0.0-20231127004446-b287a4110507
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/google/uuid v1.3.0
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
	github.com/shirou/gopsutil v3.21.6+incompatible
	github.com/stretchr/testify v1.8.1
	github.com/tklauser/go-sysconf v0.3.7 // indirect
)
