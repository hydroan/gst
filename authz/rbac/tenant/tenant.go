package tenant

var modelData = []byte(`
[request_definition]
r = tenant, sub, obj, act

[policy_definition]
p = tenant, sub, obj, act, eft

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = (g(r.sub, "super_admin", "*") || r.tenant == p.tenant) &&
    g(r.sub, p.sub, p.tenant) &&
    keyMatch3(r.obj, p.obj) &&
    r.act == p.act
`)

func Init() error {
	_ = modelData
	return nil
}
