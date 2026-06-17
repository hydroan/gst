package controller

import (
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/ds/queue/circularbuffer"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/pkg/auditmanager"
	"go.uber.org/zap"
)

// TODO: 记录失败的操作.

/*
1.Model 层处理单个 types.Model, 功能: 数据预处理
2.Service 层处理多个 types.Model, 功能: 具体的业务逻辑
3.Database 层处理多个 types.Model, 功能: 数据库的增删改查,redis缓存等.
4.这三层都能对源数据进行修改, 因为:
  - Model 的实现对象必须是结构体指针
  - types.Service[M types.Model]: types.Service 泛型接口的类型约束是 types.Model 接口
  - types.Database[M types.Model]: types.Database 泛型接口的类型约束就是 types.Model 接口
  以上这三个条件自己慢慢体会吧.
5.用户自定义的 model:
  必须继承 model.Base 结构体, 因为这个结构体实现了 types.Model 接口
  用户只需要添加自己的字段和相应的 tag 和方法即可.
  如果想要给 types.Model 在数据库中创建对象的表, 请在 init() 中调用 register 函数注册一下即可, 比如 register[*Asset]()
  如果需要在创建表格的同时创建记录, 也可以通过 register 函数来做, 比如 register[*Asset](asset01, asset02, asset03)
  这里的 asset01, asset02, asset03 的类型是 *model.Asset.
6.用户自定义 service
  必须继承 service.base 结构体, 因为这个结构体实现了 types.Service[types.Model] 接口
  用户只需要覆盖默认的方法就行了
如果有额外的业务逻辑, 在 init() 中调用 register 函数注册一下自己定义的 service, 例如: register[*asset, *model.Asset](new(asset))
如果 service.Asset 有自定义字段, 可以这样: register[*asset, *model.Asset](&asset{SheetName: "资产类别清单"})

处理资源顺序:
    通用流程: Request -> ServiceBefore -> ModelBefore -> Database -> ModelAfter -> ServiceAfter -> Response.
	导入数据: Request -> ServiceBefore -> Import -> ModelBefore ->  Database -> ModelAfter -> ServiceAfter -> Response.
	导出数据: Request -> ServiceBefore -> ModelBefore -> Database -> ModelAfter -> ServiceAfter -> Export -> Response.

    Import 逻辑类似于 Update 逻辑
	Import 的 Model 的 UpdateBefore() 在 service 层里面处理, ServiceBefore 是可选的
	Export 逻辑类似于 List 逻辑, 只是比 Update 逻辑多了 Export 步骤

其他:
	1.记录操作日志也在 controller 层
*/

const ErrRequestBodyEmpty = "request body is empty"

const defaultLimit = 1000

var (
	// Global circular buffer for controller logger
	cb *circularbuffer.CircularBuffer[*modellogmgmt.OperationLog]

	// Global audit manager instance
	am *auditmanager.AuditManager
)

func Init() (err error) {
	// Initialize circular buffer
	if cb, err = circularbuffer.New(int(config.App.Server.CircularBuffer.SizeOperationLog), circularbuffer.WithSafe[*modellogmgmt.OperationLog]()); err != nil {
		return err
	}

	// Initialize audit manager
	am = auditmanager.New(&config.App.Audit, cb)

	// Consume operation log.
	go am.Consume()

	return nil
}

func Clean() {
	operationLogs := make([]*modellogmgmt.OperationLog, 0, config.App.Server.CircularBuffer.SizeOperationLog)
	for !cb.IsEmpty() {
		ol, _ := cb.Dequeue()
		operationLogs = append(operationLogs, ol)
	}
	if len(operationLogs) > 0 {
		if err := database.Database[*modellogmgmt.OperationLog](nil).WithLimit(-1).WithBatchSize(100).Create(operationLogs...); err != nil {
			zap.S().Error(err)
		}
	}
}
