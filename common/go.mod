module github.com/datacommand2/cdm-disaster-recovery/common

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231130024214-fbee9b4bc6e4
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231128060710-080c7906e48b
	github.com/datacommand2/cdm-cloud/services/identity v0.0.0-20231129020632-c054325b27f7
	github.com/datacommand2/cdm-disaster-recovery/manager v0.0.0-20231129232117-0168f1f49c94
	github.com/google/uuid v1.4.0
	github.com/jinzhu/gorm v1.9.16
	github.com/lestrrat-go/jsref v0.0.0-20211028120858-c0bcbb5abf20
	github.com/stretchr/testify v1.7.0
)
