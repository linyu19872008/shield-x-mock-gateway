module github.com/linyu19872008/shield-x-mock-gateway

go 1.26.4

require (
	github.com/linyu19872008/shield-x-client-go v0.1.1
	gopkg.in/yaml.v3 v3.0.1
)

// v0.1.0 used the pre-cmd/mock-gateway layout and produced the wrong binary name.
retract v0.1.0

// 本地联调：使用同工作区的 client-go（发布前移除或改为正式版本）。
replace github.com/linyu19872008/shield-x-client-go => ../shield-x-client-go
