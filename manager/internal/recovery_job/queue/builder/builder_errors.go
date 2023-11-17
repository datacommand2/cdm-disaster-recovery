package builder

import "github.com/datacommand2/cdm-cloud/common/errors"

// errTaskBuildSkipped Task 생성을 건너뜀
var errTaskBuildSkipped = errors.New("skip task build")
