# 聊天历史消息存储调研与当前项目建议

调研日期：2026-10-18。范围：消息权威存储、会话排序、历史分页、断线补拉、幂等和后续扩容。结论：**继续使用 PostgreSQL 17；保留现有会话内连续序号与事务 outbox，优先减少重复索引并验证真实分页路径。当前没有足够证据支持迁移到 ScyllaDB/Cassandra、MongoDB 或 Redis Streams 作为消息主存储。**

## 证据与限制

本文区分三类陈述：“仓库事实”来自实际代码和文档；“外部事实”来自已读取的一手文档；“判断/建议”是将技术特性应用于本项目的推断。没有对四种数据库做同负载基准测试，不给出吞吐提升、延迟倍数或生产容量承诺。

已尝试 `web_search`，但当前工具未配置搜索凭据，报错 `DeepSeek search has no API key for "DEEPSEEK_API_KEY"`。后续直接抓取官方文档；Discord 原站无法连接，使用 ScyllaDB 官方明确署名 Discord 工程师、标注来自 Discord 博客的原文节选核对工程案例。以下链接用于追溯技术事实；不能把本报告理解为对所有近期产品版本和商业条款的穷尽检索。

## 1. 当前项目实际需要什么

以下均为仓库事实：

- [项目说明](../../README.md)将项目定位为 Go + PostgreSQL + Redis + Vue 的单体 IM 基础闭环；群成员上限 1000，当前仅支持一个服务实例。Redis 维护短期在线标记，尚不负责跨实例广播。群人数上限和本机长连接测试不等于消息历史规模或生产容量证明。
- [消息模型](../../backend/internal/model/message.go)包含独立消息 ID、`conversation_id`、`sender_id`、`client_msg_id`、`seq`、正文、类型和时间。客户端补拉依赖会话内 `seq`，不是全局时间排序。
- [发送逻辑](../../backend/internal/logic/chat_message.go)在同一 PostgreSQL 事务内校验成员、检查 `(sender_id, client_msg_id)` 幂等、更新 `conversations.next_seq`、插入消息和 outbox。对同一幂等键更换内容或会话会返回冲突。同一会话的序号分配因更新会话行而串行，失败事务回滚不会消耗该会话序号。
- [初始表结构](../../database/migrations/000001_initial.up.sql)同时声明 `UNIQUE (conversation_id, seq)` 和 `messages_history_idx ON messages(conversation_id, seq DESC)`；还有消息主键和发送者幂等唯一约束。
- [历史与补拉逻辑](../../backend/internal/logic/chat_history.go)已实现键集分页：历史使用 `conversation_id=$1 AND seq<$2 AND seq<=$3 ORDER BY seq DESC LIMIT $4`，查询后反转为升序返回；同步使用 `seq>$2 AND seq<=$3 ORDER BY seq LIMIT $4`。每页上限 100；第三个参数是鉴权查询得到的会话最新序号。因此后端已经避免深 `OFFSET`，无需重写一套分页。
- [数据库说明](../../database/README.md)明确：用户已读游标和每设备接收游标分离、单调前进；outbox 发布状态不等于设备确认；消息和 outbox 的保留策略尚未定义。

对项目的判断：现阶段最需要维护的是“消息、连续会话序号、幂等记录、可补拉事件一起提交”的可靠性边界。只把消息正文迁往另一数据库，会把原来的单库事务变成跨库一致性问题。当前没有历史条数、磁盘增长、最热会话写入率和线上查询延迟数据，不能仅由 1000 人群聊推断 PostgreSQL 不够用。

## 2. 技术选择比较

| 方案 | 官方资料能确认的能力 | 与当前项目的匹配判断 | 当前选择 |
| --- | --- | --- | --- |
| PostgreSQL 17 | 事务、唯一约束、复合 B-tree、正反向索引扫描、声明式分区 | 直接承接现有消息/序号/成员/outbox 事务与查询；改动与运维成本最低 | 继续作为权威存储，做小步验证和优化 |
| ScyllaDB / Cassandra | 以分区键定位数据、以 clustering key 排序并做分区内范围读取；可按会话和时间/序号桶建模 | 大规模、稳定查询形态的消息存储候选；需要重新设计幂等、序号、权限和跨分区事务边界 | 暂不引入；先有真实瓶颈证据 |
| MongoDB 普通集合 | 单文档原子、多文档事务、复合索引和 bucket 模式 | 可以实现聊天存储，但并未消除本项目跨实体事务与幂等需求；迁移成本缺乏已验证收益 | 暂不迁移；时间序列集合尤其不适合直接替换 |
| Redis Streams | 追加日志、ID 范围读取、阻塞读取、消费者组、重投和裁剪 | 适合未来的事件分发或可重建热缓存；消费者组确认并不是聊天成员设备的接收确认 | 不作为历史权威库，不用 XACK 替换 received 游标 |

## 3. PostgreSQL：当前方案已经采用了关键实践

### 3.1 连续会话序号比时间戳更贴合接收协议

仓库的 `conversations.next_seq=next_seq+1` 是事务内会话行更新。它给出了同一会话的明确顺序，能够支持客户端“从已连续接收的位置继续补拉”。全局 UUID、Snowflake 或时间戳可以作为消息身份，但不能直接替代这个协议：时钟、并发和消息 ID 的空洞不能自动证明某设备已收齐中间消息。

判断：保留当前序号；不要把它改为会话外的 PostgreSQL `SEQUENCE` 或 Redis `INCR` 后便宣称仍然连续。PostgreSQL 的普通 sequence 在事务回滚后不会回收已取出的值，这是[官方文档](https://www.postgresql.org/docs/17/functions-sequence.html)明确的语义。热点会话的行锁可能成为写入瓶颈，但需先测量等待时间、事务时长和热点分布；换存储引擎也不会自动消除应用要求的严格序列化。

### 3.2 复合索引可以支持两个分页方向

外部事实：[多列索引文档](https://www.postgresql.org/docs/17/indexes-multicolumn.html)说明前导列等值条件和后续列范围条件可以限制 B-tree 扫描范围；[索引排序文档](https://www.postgresql.org/docs/17/indexes-ordering.html)说明 B-tree 可正向和反向扫描，索引顺序匹配 `ORDER BY ... LIMIT` 时可以避免对所有候选行排序。

本项目查询固定 `conversation_id`，只按 `seq` 排序。由 `UNIQUE (conversation_id, seq)` 创建的默认升序唯一 B-tree 因而可用于 `seq ASC` 和 `seq DESC`。虽然两个索引在多列整体排序定义上不同，但在本项目“第一列等值、第二列排序”的查询上，单独的降序索引很可能没有额外必要。

建议：新增可回滚迁移移除 `messages_history_idx`，保留唯一约束索引。对已有未版本化数据库使用兼容的 `DROP INDEX IF EXISTS`。先检查实际索引定义和执行计划；不要移除承担唯一约束的索引，也不要改写已发布的初始迁移来代替升级迁移。预期收益是少维护一份索引及相应存储/WAL，具体节约多少必须实测。

验证应在真实 PostgreSQL 上覆盖历史倒序、同步正序、边界游标、分页期间并发追加；对去掉该索引后的代表性数据运行 `EXPLAIN (ANALYZE, BUFFERS)`，确认范围索引扫描和返回顺序。小表默认选择顺序扫描可能完全合理；若测试中关闭顺序扫描，只能证明剩余索引具备访问能力，不能证明生产优化器总会选它或延迟已经改善。

迁移风险：普通 `DROP INDEX` 的锁可能影响繁忙库，应安排独立迁移与维护窗口；`DROP INDEX CONCURRENTLY` 不能在事务块中运行，不能直接塞入当前事务式迁移执行器。[官方 DROP INDEX 文档](https://www.postgresql.org/docs/17/sql-dropindex.html)说明这些限制。回滚重建索引也有耗时和锁成本，不能仅以“有 down 文件”理解为无成本回滚。

### 3.3 游标稳定性与前端实际加载成本

建议继续使用排他的 `before_seq`/`after_seq`，并以实际返回消息的最小/最大序号推进游标。向后分页时新消息只追加到更大序号，不会使已经确认的旧页位置像 offset 一样整体移动；当前按页读出的 `lastSeq` 上界还能限制该页同步范围。这里是基于只追加路径的判断，不是对未来消息删除/编辑的一致性承诺。

必须区分“后端有历史分页”与“前端真正按需读历史”。当前前端从 0 开始全量同步，`loadOlder` 主要展开已在内存中的消息；换数据库无法修复首次加载随历史量增长的网络与内存成本。后续可将最新窗口和向前翻页接入后端 history API，同时设计 IndexedDB 等持久本地缓存、连续同步位置和确认恢复。**不能只跳到最新页便把 `received_seq` 确认到最新值**，否则会把未收过的旧消息跳过，破坏项目现有保证。本轮适合记录边界，不适合未经协议设计就缩短同步路径。

### 3.4 分区应由保留和容量证据驱动

外部事实：[PostgreSQL 17 分区文档](https://www.postgresql.org/docs/17/ddl-partitioning.html)说明分区裁剪依赖分区键可推导的条件，并要求分区表的唯一/主键约束包含全部分区键列。

- **按时间 RANGE 分区**：有利于按月/季度整批摘除旧数据；但当前查询只有会话与序号，没有时间范围，不能假设会裁剪大量分区。现有 `PRIMARY KEY(id)`、`UNIQUE(sender_id, client_msg_id)`、`UNIQUE(conversation_id, seq)` 都未包含时间分区键，直接分区会遇到唯一性语义问题。把时间机械地加到唯一键里会放宽原幂等范围，不能当作等价改造。
- **按 conversation_id HASH 分区**：可与当前会话查询配合裁剪，但 `PRIMARY KEY(id)` 和全发送者幂等键仍不包含分区键，需要另行设计。它也不会将一个热点会话自动拆成多份，更不等同于跨机器分片。
- **桶化/冷归档**：将来可用序号范围或时间桶定位历史，需定义桶边界、跨桶分页、全局消息/幂等注册表与原事务的关系；归档还涉及读路由、鉴权和恢复验证。

当前建议不直接改分区。先采集 messages/outbox 的堆和索引体积、日增量、最热会话写入率、分页 P95/P99、锁等待、WAL、autovacuum 和备份恢复耗时，再判断是否确有分区收益。不能为所有项目指定一个未经验证的统一“几千万条就必须分区”阈值。

## 4. ScyllaDB/Cassandra：Discord 经验的可迁移部分

外部事实：[Cassandra 逻辑数据建模](https://cassandra.apache.org/doc/latest/cassandra/developing/data-modeling/data-modeling_logical.html)强调围绕查询设计分区和表；[ScyllaDB 表定义文档](https://docs.scylladb.com/manual/stable/cql/ddl.html)解释同分区副本定位、clustering 顺序、范围读取，以及超大分区可能形成热点。该文档也明确分区内更新的原子/隔离性质不能简单推广到跨分区操作。

Discord 工程师 Bo Ingram 的[原文](https://discord.com/blog/how-discord-stores-trillions-of-messages)在本次环境无法直连，但 [ScyllaDB 官方刊载的署名节选](https://www.scylladb.com/discord-scylladb-blog-video/)可读，并提供以下一手工程细节：

```sql
PRIMARY KEY ((channel_id, bucket), message_id)
WITH CLUSTERING ORDER BY (message_id DESC)
```

Discord 使用会话加静态时间窗口作为复合分区键，以时间可排序 ID 作为分区内排序键。文章回顾 2017 年的 12 个 Cassandra 节点和 2022 年初存储万亿级消息的 177 个节点；遇到热点分区、compaction 积压和 JVM GC 带来的维护/尾延迟问题。文章说明其在迁往 ScyllaDB 前，也验证并推动改善了反向查询；还明确承认换成 ScyllaDB 后热点分区仍可能存在。以上数字仅描述 Discord 自身案例，不是本项目容量或 ScyllaDB 相对 PostgreSQL 的性能证据。

对项目的判断：可以借鉴“限制分区大小、两种扫描方向都验证、避免热点负载放大、迁移前验证数据”的方法；不应直接借鉴其集群规模和数据库选择。当前系统同时需要会话行递增、成员权限、跨会话发送者幂等和 outbox 事务。若改用 `((conversation_id,bucket),seq)`，这些数据未必共处一个分区；需设计 LWT/幂等注册、序号分配、事件一致性和失败恢复，不能把 CQL batch 当作 PostgreSQL 跨表事务的同义词。[ScyllaDB 更新语义](https://docs.scylladb.com/manual/stable/cql/dml.html)还说明默认写入冲突按时间戳决议及 LWT 的作用，这与“同幂等键不同正文必须报冲突”不是自动等价的行为。

重新评估条件：经优化和代表性压测后，历史读写的增长、磁盘/恢复成本或多地域需求确实超过单 PostgreSQL 的可接受边界；团队具备 repair、compaction、故障演练和数据迁移运维能力；且已完成跨存储一致性设计。届时优先评估可从权威事件重建的历史读取投影，避免首先拆散发送事务。

## 5. MongoDB：普通文档可用，时间序列集合有不匹配之处

外部事实：[MongoDB 事务文档](https://www.mongodb.com/docs/manual/core/transactions.md)说明单文档操作原子，并支持跨文档/集合/数据库/分片事务，事务保证依赖读写 concern；不能泛称 MongoDB 不支持事务。[bucket 模式](https://www.mongodb.com/docs/manual/data-modeling/design-patterns/group-data/bucket-pattern.md)支持把同一实体的一组记录存入有界桶，甚至按展示页建桶，明确避免无限增长数组。

判断：普通集合一条消息一文档、以 `{conversation_id,seq}` 建复合索引，可以承接当前访问模式；但为序号、幂等和 outbox 维护多实体事务仍有实现与运维成本。每个会话塞进一个无限数组会造成文档增长和热点；有界桶又增加并发封桶、跨桶游标、修改/删除与幂等检查复杂度。对已有成熟关系模型，没有已验证收益支撑这次迁移。

[MongoDB 时间序列限制](https://www.mongodb.com/docs/manual/core/timeseries/timeseries-limitations.md)明确：不支持唯一索引，不能在事务中写入时间序列集合，更新有字段限制，也不支持 change streams。聊天数据虽带时间戳，却还承担幂等、事务及将来的撤回/编辑需求，因此不建议把时间序列集合当消息主库。这里的限制仅针对时间序列集合，不能套用到普通集合。

## 6. Redis Streams：事件投递与历史权威存储职责不同

外部事实：[Redis Streams 文档](https://redis.io/docs/latest/develop/data-types/streams/index.html.md)提供 XADD、XRANGE/XREVRANGE、XREAD、消费者组和裁剪等能力。一个消费者组中的消息分配给组内消费者处理；同一个组不是向每台网关或每位成员各广播一份。XACK 是消费者处理确认，不会证明某聊天成员的设备已经收到，更不会证明用户已读。

[Redis 持久化文档](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)列出 RDB/AOF 的持久化权衡：仅 RDB 可能丢失最近快照后的数据，AOF everysec 在故障时可能丢失约一秒写入；不能把“用了 Stream”自动等同于持久化承诺。最新文档含 Redis 8.x 新命令和裁剪语义，而本项目使用 Redis 7，不应直接使用新版本才有的功能。

判断：后续跨实例投递可评估“PG 事务 outbox → Streams → 网关”，消费端按事件 ID 幂等，发布失败可重试；但 PG 与 Redis 没有由现有代码提供的共同事务，必须允许重复投递并保持数据库补拉。网关广播需自行设计路由/独立消费者组等拓扑，不能只开一个共享组就承诺每网关收到全部事件。长期历史放在 Redis 还要承担内存成本、裁剪、备份恢复、权限和索引重建等问题，当前没理由替换 PG。

## 7. 建议落地顺序与风险边界

1. **本轮合适的增量**：保留数据库与协议；新增升级/回滚迁移移除已核验的重复历史索引，补充真实 PG 历史/同步游标测试及剩余索引访问验证。更新数据库说明，写明为什么一个唯一索引足以支持两种方向。实现与测试结果由实际代码变更记录提供，本调研不把建议写成已通过的测试结果。
2. **下一步观测**：建立消息与 outbox 增长、分页延迟、会话锁等待、未发布事件最老年龄、设备补拉积压和恢复时间指标。负载应包含大量不同会话、少数热点会话、长历史向前翻页、连续追加和故障重试，不能仅做空库或单页查询测试。
3. **明确 outbox 清理**：当前 outbox 存储事件 JSON/成员快照，消息正文存在于权威表和事件 payload，已发布行若一直保留也会持续膨胀。为“已发布且超过明确保留期”的行设计小批量删除、监控及 vacuum；不能删除未发布行。发布不等于成员接收，设备恢复仍靠 messages，因此 outbox 的清理条件与历史消息保留必须分别设计。
4. **再设计按需历史和本地缓存**：协调最新窗口、旧页、连续接收确认、刷新恢复和跨设备语义；未经这一设计，不改变目前从 0 补齐的安全边界。
5. **最后考虑归档/分区/分布式投影**：保留策略必须同时定义设备落后于最早保留序号时的重置/全量恢复接口。直接删除旧消息会让当前按 `next_seq` 判断 `has_more` 和连续接收的假设失效，不能只加一个定时 DELETE。分区上线需迁移回填、校验、索引/唯一性方案、跨桶分页和可实际演练的回滚。

当前选择的价值在于保留已经实现的可靠性语义，并以可测量的增量降低存储与查询成本。何时更换平台，应由真实数据规模、热点和故障恢复证据决定。

## 8. 本轮实际落地与验证

已新增 `000006_message_history_index` up/down 迁移：移除重复历史索引，保留会话序号唯一索引，并提供重建旧索引的 SQL。已增加仅需 PostgreSQL 的历史存储集成测试，更新旧库迁移版本数量断言及数据库运行说明。

本地验证环境为 Go 1.26.4、Windows、独立临时 PostgreSQL 18.6；没有连接或修改现有业务数据库。仓库 CI 的目标仍为 PostgreSQL 17，本地结果不冒充已执行过 PG17 CI。

- 1205 条目标会话历史与另一会话的数据同时存在；每页读取 37 条，页间持续追加新消息，旧历史无重复、无缺口，正向补拉包含追加内容。
- 真实执行验证非成员访问拒绝、会话隔离、最早历史空页和最新同步空页。
- 默认优化器下执行 `EXPLAIN (ANALYZE, BUFFERS)`：同步使用 `Index Scan using messages_conversation_id_seq_key`，历史使用 `Index Scan Backward using messages_conversation_id_seq_key`，两者没有独立 Sort。没有关闭顺序扫描或强制计划；结果仅对应测试数据分布。
- 带数据执行 down 重建旧索引，再由实际迁移执行器升级；验证重复应用和消息总数保持不变。
- `go test -v ./...` 与 `go vet ./...` 通过。依赖 Redis 的既有 API 集成测试因未设置 `IM_TEST_REDIS_ADDR` 而跳过，新增两项历史存储集成测试实际执行通过。

本轮不承诺具体吞吐或磁盘节约比例；收益是停止维护一份已确认冗余的索引。繁忙现有库上线仍需遵循上文迁移锁与维护窗口说明。
