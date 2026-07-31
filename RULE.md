# ysr Branch Rules

## 主分支硬约束

- 本项目唯一的默认主分支是 `ysr`。
- 永远禁止合并到 `main` 分支。
- 永远禁止直接向 `main` 分支提交内容。

## Upstream Integration

- By default, do not merge `origin/main` or local `main` into `ysr`.
- An upstream merge into `ysr` is permitted only when the repository owner explicitly requests that merge in the current task.
- Do not infer permission to merge from new upstream commits, a request to continue work, or completion of another task.
- An approved merge must leave local `main` unchanged unless the repository owner explicitly requests otherwise.

## Channel Type IDs

- ysr-specific channel types must use the reserved range `200-299`.
- Do not allocate new ysr channel types in upstream or legacy ranges.
- Any change to an existing ysr channel type ID must include a transactional data migration for both `channels.type` and persisted task platform values, plus regression coverage.

## 开发约束

以下约束适用于整个项目；根目录及各子目录中的 `AGENTS.md` 可针对其适用范围补充或细化规则，但不得削弱本文档中的约束。

### 文档语言

- 新增文档，以及对现有文档新增或改写的内容，必须使用简体中文。
- 既有内容无需仅因本规则进行翻译。
- 代码标识符、命令、配置键、API 或协议名称、品牌、专有名词、第三方原文、许可证文本、外部模板及多语言翻译内容可保留原语言。

### 变更范围

- 变更必须限定在完成当前需求所必需的范围内。
- 禁止夹带无关的重构、格式化、依赖升级、文件改名或目录整理。
- 完成需求确实需要额外变更时，必须说明原因及影响范围。

### 兼容性

- 未经明确要求，不得引入破坏性变更，包括公开 API、配置项、环境变量、持久化数据格式、数据库结构和外部协议行为的非兼容变化。
- 明确需要破坏性变更时，必须记录影响范围，并提供可执行的迁移步骤和回退方法。

### 测试与验证

- 新增代码行为或修复缺陷时，必须新增或更新能够保护对应行为的测试。
- 完成变更前，必须运行与变更相关的测试、静态检查和构建检查；不得在未取得最新验证结果时宣称变更已经完成或通过。
- 无法执行检查或检查未通过时，必须明确说明具体项目、原因和剩余风险，不得隐瞒或将其描述为已通过。

### 依赖管理

- 新增依赖前，必须确认现有依赖和标准库无法合理满足需求，并评估其维护状态、许可、体积和安全风险。
- 依赖及锁文件必须使用项目规定的包管理工具更新，禁止手工修改锁文件。
- 禁止在与当前需求无关的情况下升级或替换依赖。

### 安全与隐私

- 禁止提交密钥、令牌、密码、私钥、真实个人数据或其他敏感信息。
- 外部输入必须在进入受信任边界前完成必要的校验和限制，不得仅依赖客户端校验。
- 日志、错误响应、测试数据和示例不得泄露凭据、个人数据或敏感内部信息。

### 错误处理

- 禁止静默忽略错误；错误必须被处理、向上返回或按项目约定记录。
- 错误处理必须保留足够的上下文以便排查，同时不得向非授权对象暴露敏感实现细节。
- 不得以空结果、默认成功或无说明的降级掩盖失败。

### 生成文件

- 禁止直接手工修改自动生成文件；必须修改对应源文件，并通过项目规定的生成流程更新产物。
- 生成产物必须与源文件在同一变更中保持一致，且不得包含与当前需求无关的重新生成内容。

### 文档同步

- 公开接口、配置项、命令、部署方式或开发流程发生变化时，必须在同一变更中更新相关项目文档。
- 文档中的示例、路径、链接和命令必须与当前项目保持一致并可实际使用。

### 重大改动的设计与计划文档

- 涉及架构调整、跨模块功能、公开 API、配置或数据结构、兼容性、安全、计费，或者需要迁移与回退方案的改动，均属于重大改动。
- 重大改动必须同时提供设计决策文档和可执行计划文档。设计决策文档存放于 `docs/superpowers/specs/`，文件名使用 `YYYY-MM-DD-主题-design.md`；计划文档存放于 `docs/superpowers/plans/`，文件名使用 `YYYY-MM-DD-主题.md`。
- 必须先完成设计决策文档，再基于已确定的设计编写可执行计划。设计文档必须说明目标、范围、关键决策、备选方案、影响和验证策略；计划文档必须拆分实施步骤，明确涉及文件、执行顺序和验证方式。
- 开始代码实现前，两份文档必须齐备、主题对应且内容一致，计划文档必须明确引用对应的设计文档。
- 实现过程中设计决策发生变化时，必须先同步更新设计文档和计划文档，确认二者一致后方可继续编码。
