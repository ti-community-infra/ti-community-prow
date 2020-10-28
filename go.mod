module github.com/tidb-community-bots/ti-community-prow

go 1.14

replace k8s.io/client-go => k8s.io/client-go v0.18.9

require (
	github.com/gin-gonic/gin v1.6.3
	github.com/sirupsen/logrus v1.7.0
	gotest.tools v2.2.0+incompatible
	k8s.io/apimachinery v0.18.9
	k8s.io/test-infra v0.0.0-20201002205105-9b61c9650606
	sigs.k8s.io/yaml v1.2.0
)
