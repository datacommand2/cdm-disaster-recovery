module github.com/datacommand2/cdm-disaster-recovery/common

go 1.14

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/datacommand2/cdm-center/cluster-manager v0.0.0-20231205064454-47d81f9d15b3
	github.com/datacommand2/cdm-cloud/common v0.0.0-20231205042820-988cc8ba20e0
	github.com/datacommand2/cdm-cloud/services/identity v0.0.0-20231205055758-216c42add282
	github.com/datacommand2/cdm-disaster-recovery/manager v0.0.0-20231205063538-005af25e81e4
	github.com/google/uuid v1.4.0
	github.com/jinzhu/gorm v1.9.16
	github.com/lestrrat-go/jsref v0.0.0-20211028120858-c0bcbb5abf20
	github.com/stretchr/testify v1.7.0
)
