# Module Development Guide

本文档记录内置模块开发约定，尤其是会通过 `gg module copy <name>` 复制到业务项目中的模块。

## Service 文件边界

### 公用函数不要放在具体 action service 文件中

开发可复制模块时，多个 service action 共享的函数、类型、常量不要放在某一个具体 action 的 service 文件里，这类代码应放到独立 helper 文件中。

原因是 `gg module copy` 会根据 action service 文件的依赖关系复制 helper 文件。如果一个未被当前 DSL `Service(true)` 声明的 action service 文件里放了公用函数，它可能会被当成 helper 复制到业务项目；随后 `gg gen --prune` 又会因为当前模型没有对应 service target，把这个文件识别成应清理的 service 文件。

简单规则：

- action service 文件只放该 action 自己的 service struct、action 方法、hook 方法和强绑定该 action 的私有逻辑。
- 多个 action 共享的逻辑必须放到非 action service 文件中。
- 复制模块前要确认 `gg module copy <name>` 产出的 helper files 不包含未被 DSL action 声明的 action service 文件。
