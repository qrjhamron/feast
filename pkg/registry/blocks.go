package registry

import (
	"fmt"
	"github.com/qrjhamron/feast/pkg/world"
)

func BlockNameFromStateID(id int32) (string, bool) {
	name := world.ResolveBlockName(id)
	if name != "" {
		return name, true
	}
	if id == 0 {
		return "air", true
	}
	return fmt.Sprintf("block_%d", id), false
}
