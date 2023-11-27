module github.com/datacommand2/cdm-disaster-recovery/common

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231127061122-07e02be5bd0c
	github.com/datacommand2/cdm-cloud/services/identity v0.0.0-20231127061639-e680b139acd3
	github.com/google/uuid v1.3.0
	github.com/jinzhu/gorm v1.9.16
	github.com/lestrrat-go/jsref v0.0.0-20211028120858-c0bcbb5abf20
)
