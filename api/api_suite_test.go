package api

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen  -write_package_comment=false -package=$GOPACKAGE -destination=mock_cgra_test.go github.com/sarchlab/zeonica/cgra Device,Tile
//go:generate mockgen -write_package_comment=false -package=$GOPACKAGE -destination=mock_sim_test.go github.com/sarchlab/akita/v4/sim Port,Engine
func TestApi(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Api Suite")
}
