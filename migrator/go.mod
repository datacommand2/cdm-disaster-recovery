module github.com/datacommand2/cdm-disaster-recovery/migrator

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231130024214-fbee9b4bc6e4
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231128060710-080c7906e48b
	github.com/datacommand2/cdm-disaster-recovery/common v0.0.0-20231130024644-7eb6f8f56caf
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
)
