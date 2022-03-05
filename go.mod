module github.com/sarchlab/zeonica

go 1.16

require (
	github.com/golang/mock v1.6.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.18.1
	github.com/tebeka/atexit v0.3.0
	gitlab.com/akita/akita/v2 v2.0.2
	gitlab.com/akita/noc/v2 v2.0.2
)

replace gitlab.com/akita/akita/v2 => ../akita

replace gitlab.com/akita/noc/v2 => ../noc
