module github.com/datacommand2/cdm-disaster-recovery/manager

go 1.14

require (
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231116073359-755996e851e1
	github.com/datacommand2/cdm-cloud/services/identity v0.0.0-20231116073359-755996e851e1
	github.com/datacommand2/cdm-cloud/services/scheduler v0.0.0-20231116073359-755996e851e1
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.4.0
	github.com/jinzhu/copier v0.4.0
	github.com/jinzhu/gorm v1.9.16
	github.com/micro/go-micro/v2 v2.9.1
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.4 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231106174013-bbf56f31fb17
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0
)
