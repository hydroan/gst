package service

/*
service 必须继承 base[types.Model]
service 如果使用自定义 Logger, 则必须匿名继承, 例如
type asset {
	base[*model.Asset]
	types.Logger
}
*/
