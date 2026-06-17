package model

/*

1.覆盖默认的 ID:
  - 给新的 primaryKey 字段增加 gorm tag "gorm:primaryKey"
  - 覆盖默认的 ID 并设置 json 和 gorm tag 为 "-"
    ID string `json:"-" gorm:"-"`
  - 覆盖默认的 SetID() 方法
    func(* FeishuUser)SetID(...string){}
2.model 结构体对象的字段如果要通过 query parameter 作为查询参数的话, 需要增加 schema tag
3.自己定义的 model 继承 Base model 时必须时匿名继承,并且不能加 json tag
  否则在 gin ShouldBindJson 和 json.Unmarshal 时会出问题
4.如果自定义了 ID 的规则, 那么记住几点:
  1.一定不要在自定义 Model 中增加 ID 字段,
  2.不要重写 GetD() 和 SetID() 方法
5.如果要 UpdatePartial, 修改的字段如果是基本类型,比如 int, string 等, 如果修改的值是默认值(zero value),
  那么必须该类型改成指针类型,否则无法修改.
6.外键只允许多集关系的表,并且这种表的 children 也是自己
7.直接完全继承另外一个model,则可以完成操作另外一个model的数据库表,参考 CasbinRule
6.重写 SetID() 函数为一个空的函数, 可以让 ID 只为 integer 并且为自增类型, 参考 CasbinRule
7.尽量别在 model 的结构体字段上加上 binding:"required" tag
8.model尽量不要调用外层,只允许访问 database.Database 层, 因为有很多其他的库会用到 model 层, 这样可以防止循环依赖.
9.model可以手动注册"自动创建数据库表",可以在 init 中调用 Register, 或者注册路由 router.Register 来手动创建数据库表
10.如何使用该库: 作为第三方库导入进来 import; 或者 git clone 当前整个项目到 internal 目录中, 在自己的项目调用这个后端框架.
11:如果 create before 检查到有相同的资源在创建但是不想创建,则可以设置相同的ID.这样就可以只更新资源而不重复创建资源了.
12.为了防止创建相同的 name 的 role， 可以在 CreateBefore 使用 util.HashID(r.Name) 设置ID，这样如果 role.Name 相同则只是更新
13.如果model的结构体很多字段，并把 database 中的 WithBatchSize 调小一些(默认1000)

rbac
	g hybfkuf admin                  // hybfkuf 属于 admin 组
	g user1 admin                    // user1 属于 admin 组
	p admin /api/asset/asset GET     // admin 组允许 GET
	p admin /api/asset/asset POST    // admin 组允许 POST
	p admin /api/asset/asset PATCH   // admin 组允许 PATCH
	p admin /api/asset/asset DELETE  // admin 组允许 DELETE

	Group -> Policy: 给角色组设置策略(group 就是 role)
	Group <- User: 向角色组中添加和删除用户


// - Model 层的 Hooker 比 Service 层的 Hooker 用的更多.
// - Model 层的 ListAfter 可能是只查询最顶层的 Menu,并不能拿到所有的 Menu
// - Model 层的 DeleteBefore/DeleteAfter 前端传递过来的很可能只是一个 ID, 其他字段为空
//   如果要通过检查改资源的其他字段来进行操作,需要先通过数据库 Get 到该资源.
// - Modal, ListAfter 如果检查字段并更新当前资源, 则会出发 UpdateBefore,UpdateAfter hook, 小心循环调用
// - Modal.UpdateXXX 可能是 UpdatePartial, 所有很多字段都是空的, 需要先 Get 一下
//   因为你不能保证前端使用的是 PUT(全量更新) 还是 PATCH(部分更新), 最好还是自己从数据库立main Get 一下.
// - Model.GetBefore 时，如果要使用 GetBefore hook，需要在 new 完之后，把 ID值设置上，
//   例如：user := new(User) user.ID = "myid", 要不然一个空空的 user 是无法出发有效的 hook 的。



使用 service 层而不是 model 层的情况
- service 层有前端的一些信息: model 拿不到
- service 的 expandFields 的效率比 model 块
  model 每次只能展开一个, service 层可以一次性展开所有

使用 model 层而不是 service 层的情况
- ListAfter 来设置某个字段时, 如果做了权限控制,则没有权限的人是看不到这些资源的,也无法触发 ListAfter 钩子

*/
