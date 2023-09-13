module github.com/sarchlab/zeonica

go 1.16

require (
	github.com/golang/mock v1.6.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.27.7
	github.com/sarchlab/akita/v3 v3.0.0-alpha.29
	github.com/tebeka/atexit v0.3.0
	gitlab.com/akita/akita/v2 v2.0.2
)

//replace gitlab.com/akita/akita/v2 => ../akita

//replace gitlab.com/akita/noc/v2 => ../noc
