module 10.1.1.220/cdm/cdm-disaster-recovery/daemons/migrator

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	10.1.1.220/cdm/cdm-center/services/cluster-manager v0.1.0-rc8.0.20230907055220-0d9795bc2b70
	10.1.1.220/cdm/cdm-cloud/common v1.0.4
	10.1.1.220/cdm/cdm-disaster-recovery/common v0.0.0-20230907052223-cda694444826
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
)