// Easyconf helps the user to config a dataflow array easily in Zeonica.
package easyconf

import (
	"math"

	"github.com/sarchlab/zeonica/confignew"
	"github.com/sarchlab/zeonica/dummy"
)

func CreateFourSideArray(n int) {

	nameIDBinding := confignew.NewNameIDBinding()

	nameIDBinding.BindRegisterFile(32)

	northID := nameIDBinding.RegisterName("north")
	southID := nameIDBinding.RegisterName("south")
	westID := nameIDBinding.RegisterName("west")
	eastID := nameIDBinding.RegisterName("east")

	cores := make([]*dummy.DummyTile, n*n)
	for i := 0; i < n*n; i++ {
		cores = append(cores, dummy.DummyCreateAndRegiTile())
	}
	bufs := make([]*dummy.DummyBuffer, 4*n*(n-1))
	for i := 0; i < 4*n*(n-1); i++ {
		bufs = append(bufs, dummy.DummyCreateAndRegiBuffer())
	}
	ConnectCoresFromFourSidesElastic(cores, bufs, northID, southID, westID, eastID)

}

func ConnectCoresFromFourSidesElastic(cores []*dummy.DummyTile, bufs []*dummy.DummyBuffer, northID int, southID int, westID int, eastID int) {
	core_num := len(cores)
	sqrt_core_num := int(math.Sqrt(float64(core_num)))
	if sqrt_core_num*sqrt_core_num != core_num {
		panic("The number of cores is not a square number.")
	}
	if len(bufs) != 4*sqrt_core_num*(sqrt_core_num-1) {
		panic("The number of buffers is not correct. There should be 4*N*(N-1) buffers.")
	}
	buf_num := 0
	for i := 0; i < core_num; i++ {
		core := cores[i]
		if i-sqrt_core_num >= 0 {
			core.IDbinding.BindIDAndImpl(northID, bufs[buf_num])
			buf_num++
		} else {
			core.IDbinding.BindIDAndImpl(northID, dummy.NonExistComponent)
		}
		if i-1 >= 0 {
			core.IDbinding.BindIDAndImpl(westID, bufs[buf_num])
			buf_num++
		} else {
			core.IDbinding.BindIDAndImpl(westID, dummy.NonExistComponent)
		}
		if i+1 < core_num {
			core.IDbinding.BindIDAndImpl(eastID, bufs[buf_num])
			buf_num++
		} else {
			core.IDbinding.BindIDAndImpl(eastID, dummy.NonExistComponent)
		}
		if i+sqrt_core_num < core_num {
			core.IDbinding.BindIDAndImpl(southID, bufs[buf_num])
			buf_num++
		} else {
			core.IDbinding.BindIDAndImpl(southID, dummy.NonExistComponent)
		}
	}
}

// 0 1 2 3
// 4 5 6 7
// 8 9 10 11
// 12 13 14 15
