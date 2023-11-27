module github.com/datacommand2/cdm-disaster-recovery/migrator

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231127012344-99c2aa35af23
    github.com/datacommand2/cdm-cloud/common v0.0.0-20231124062432-069e9eb1c852
    github.com/datacommand2/cdm-disaster-recovery/common v0.0.0-20231127004446-b287a4110507
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
)