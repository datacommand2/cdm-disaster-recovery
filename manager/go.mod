module github.com/datacommand2/cdm-disaster-recovery/manager

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231130024214-fbee9b4bc6e4
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231128060710-080c7906e48b
	github.com/datacommand2/cdm-cloud/services/identity v0.0.0-20231129020632-c054325b27f7
	github.com/datacommand2/cdm-disaster-recovery/common v0.0.0-20231204013449-9f6a80c77da4
	github.com/golang/protobuf v1.5.2
	github.com/google/uuid v1.4.0
	github.com/jinzhu/copier v0.3.4
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
	github.com/pkg/errors v0.9.1
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c
	google.golang.org/grpc v1.44.0
	google.golang.org/protobuf v1.28.1
)
