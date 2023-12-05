module github.com/datacommand2/cdm-disaster-recovery/migrator

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231205064454-47d81f9d15b3
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231205042820-988cc8ba20e0
	github.com/datacommand2/cdm-disaster-recovery/common v0.0.0-20231205065028-458a7411e43c
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
)
